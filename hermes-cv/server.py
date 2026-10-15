"""Pipeline sidecar for the Go API: python server.py   (port from HERMES_PIPELINE_PORT, default 8090)

POST /process {"url": "<presigned GET of the PDF>"} downloads the PDF, runs run.process on it, and returns
the document JSON. One document at a time: the pipeline already uses every core, so a request that
arrives mid-job gets 503 and the caller retries. GET /health reports liveness. The sidecar holds no
storage credentials; the URL is all it gets."""
import json, os, shutil, sys, tempfile, threading, traceback, urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import run

busy = threading.Lock()
DOWNLOAD_TIMEOUT = 120  # seconds, for the PDF fetch from object storage

def log(*args):
    print(*args, file=sys.stderr, flush=True)

class Handler(BaseHTTPRequestHandler):
    def reply(self, code, body):
        b = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)

    def do_GET(self):
        if self.path != "/health":
            return self.reply(404, {"error": "not found"})
        self.reply(200, {"status": "busy" if busy.locked() else "ok"})

    def do_POST(self):
        if self.path != "/process":
            return self.reply(404, {"error": "not found"})
        try:
            url = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))))["url"]
        except (ValueError, KeyError, TypeError):
            return self.reply(400, {"error": 'body must be {"url": "..."}'})
        if not busy.acquire(blocking=False):
            return self.reply(503, {"error": "busy with another document"})
        work = tempfile.mkdtemp(prefix="hermes-")
        try:
            src = os.path.join(work, "doc.pdf")
            with urllib.request.urlopen(url, timeout=DOWNLOAD_TIMEOUT) as r, open(src, "wb") as f:
                shutil.copyfileobj(r, f)
            self.reply(200, run.process(src, os.path.join(work, "out"), log=log))
        except Exception as e:  # anything the pipeline raises fails this document, not the sidecar
            log(traceback.format_exc())
            self.reply(422, {"error": f"{type(e).__name__}: {e}"})
        finally:
            shutil.rmtree(work, ignore_errors=True)
            busy.release()

if __name__ == "__main__":
    port = int(os.environ.get("HERMES_PIPELINE_PORT", "8090"))
    log(f"pipeline sidecar on :{port}")
    ThreadingHTTPServer(("", port), Handler).serve_forever()
