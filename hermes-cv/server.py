"""Pipeline sidecar for the Go API: python server.py   (port from HERMES_PIPELINE_PORT, default 8090)

POST /process {"url": "<presigned GET of the PDF>"} downloads the PDF and runs run.process on it. The
response is newline-delimited JSON (application/x-ndjson), one message per line:
  {"type": "progress", "pages_done": 0, "page_count": 28}   once the pages are rendered, then per page
  {"type": "result", "document": {...}}                      last, on success
  {"type": "error", "error": "..."}                          last, if the pipeline fails mid-stream
Failures before the stream starts keep their status codes: 400 bad body, 503 busy, 422 download failed.
One document at a time: the pipeline already uses every core, so a request that arrives mid-job gets
503 and the caller retries. GET /health reports liveness. The sidecar holds no storage credentials;
the URL is all it gets.

The caller's W3C traceparent header carries the document's trace ID (the upload request's). Every log
line for the job is prefixed with it, so the sidecar's output lines up with the Go API's trace_id."""
import json, os, re, shutil, sys, tempfile, threading, traceback, urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import run

busy = threading.Lock()
DOWNLOAD_TIMEOUT = 120  # seconds, for the PDF fetch from object storage

TRACEPARENT = re.compile(r"00-([0-9a-f]{32})-([0-9a-f]{16})-[0-9a-f]{2}")

def log(*args):
    print(*args, file=sys.stderr, flush=True)

def trace_id(header):
    """The trace ID from a traceparent header, or "-" if it's missing or malformed (or all zeros)."""
    m = TRACEPARENT.fullmatch((header or "").strip())
    return m.group(1) if m and m.group(1).strip("0") and m.group(2).strip("0") else "-"

class Handler(BaseHTTPRequestHandler):
    def reply(self, code, body):
        b = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)

    def send_line(self, msg):
        self.wfile.write(json.dumps(msg).encode() + b"\n")
        self.wfile.flush()

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
        tid = trace_id(self.headers.get("traceparent"))
        def tlog(*args):  # prefix every line: page notes and tracebacks span several
            for line in " ".join(map(str, args)).splitlines() or [""]:
                log(f"[trace {tid}]", line)
        if not busy.acquire(blocking=False):
            tlog("busy with another document, rejected")
            return self.reply(503, {"error": "busy with another document", "trace_id": tid})
        work, streaming = tempfile.mkdtemp(prefix="hermes-"), False
        try:
            tlog("processing")
            src = os.path.join(work, "doc.pdf")
            with urllib.request.urlopen(url, timeout=DOWNLOAD_TIMEOUT) as r, open(src, "wb") as f:
                shutil.copyfileobj(r, f)
            # No Content-Length: the HTTP/1.0 response body runs until the connection closes.
            self.send_response(200)
            self.send_header("Content-Type", "application/x-ndjson")
            self.end_headers()
            streaming = True
            progress = lambda done, total: self.send_line({"type": "progress", "pages_done": done, "page_count": total})
            doc = run.process(src, os.path.join(work, "out"), log=tlog, on_progress=progress)
            self.send_line({"type": "result", "document": doc})
        except (BrokenPipeError, ConnectionResetError):
            tlog("caller disconnected")  # e.g. the API restarted; it requeues the document
        except Exception as e:  # anything the pipeline raises fails this document, not the sidecar
            tlog(traceback.format_exc())
            err = {"error": f"{type(e).__name__}: {e}", "trace_id": tid}
            try:
                self.send_line({"type": "error", **err}) if streaming else self.reply(422, err)
            except OSError:
                pass
        finally:
            shutil.rmtree(work, ignore_errors=True)
            busy.release()

if __name__ == "__main__":
    port = int(os.environ.get("HERMES_PIPELINE_PORT", "8090"))
    log(f"pipeline sidecar on :{port}")
    ThreadingHTTPServer(("", port), Handler).serve_forever()
