# Hermes

Hermes reads scanned vendor Statements of Account (SOA). You upload the PDF in the browser. It OCRs every page, extracts the rows and totals, checks the arithmetic and flags the rows that need a person to look at them.

It has three processes you run locally, plus two backing services:

| Process | What it is | Port |
|---|---|---|
| `hermes-cv/server.py` | The OCR pipeline (Python, Tesseract), called by the API | 8090 |
| `hermes-web/cmd/api` | The Go API: uploads, the processing queue, status events | 8080 |
| `hermes-web/web` | The React frontend (Vite dev server) | 5173 |
| Postgres | Documents, extracted rows, and the queue | 5432 |
| S3-compatible storage | The uploaded PDFs (RustFS locally) | 9000 |

```
browser ──► Vite :5173 ──/api──► Go API :8080 ──► Postgres
                                   │   ▲
                                   │   └── presigned URL ── S3 storage :9000
                                   ▼
                             OCR sidecar :8090
```

## Prerequisites

- **Go** 1.26+
- **Node** 24+ and **pnpm**
- **Python** 3.14+. Older versions work, but cancelling a job then stops only the pages not yet started.
- **Tesseract** 5 with English data, on your `PATH`:
  - Arch: `pacman -S tesseract tesseract-data-eng`
  - Debian/Ubuntu: `apt install tesseract-ocr`
  - macOS: `brew install tesseract`
- **Podman or Docker** with Compose, for Postgres and the object store. Podman is used when it's installed. `podman compose` needs a compose provider (`docker-compose` or `podman-compose`).
- **[Task](https://taskfile.dev)** 3. On Arch the package installs the binary as `go-task`, so read `task` as `go-task` below.

## Quick start

```sh
task setup     # Python venv, Go modules and pnpm packages (safe to re-run; skips what's up to date)
task dev       # Postgres + storage, then the OCR sidecar, the API and the frontend
```

Then open http://localhost:5173. `task dev` runs the three app processes in one terminal, with each line prefixed `[cv]`, `[api]` or `[web]`.

- **Ctrl+C** stops the sidecar, the API and the frontend.
- **`task infra:down`** stops Postgres and storage, keeping their data.
- **`task infra:reset`** deletes their data.

The database tables and the bucket are created on first start: the API applies its migrations and creates the bucket itself.

Run `task` to list everything. The ones you'll use most:

| Task | |
|---|---|
| `task dev` | Start everything |
| `task infra:up` / `infra:down` / `infra:reset` | Backing services only (`compose.yaml`) |
| `task infra:logs`, `task infra:ps` | Their logs and status |
| `task cv`, `task api`, `task web` | One app process, for running each in its own terminal |
| `task check` | Go tests, gofmt, go vet, oxlint, and both builds |
| `task generate` | Regenerate the API server code after editing `api/openapi.yaml` |
| `task cv:run -- statement.pdf out/` | Run the OCR pipeline on one file, without the API |

### Settings

The defaults are in `.env.example`: credentials, ports and the API's connection settings. To change any of them, copy the file to `.env` (which is git-ignored) and edit it there. Values in `.env`, or already set in your shell, override the defaults.

`compose.yaml` reads the same variables. So if you move Postgres to port 5433, set both `HERMES_PG_PORT=5433` and the port in `HERMES_DATABASE_URL`.

> MinIO images can no longer be pulled from Docker Hub or quay.io, which is why RustFS is used. Any S3-compatible store works, such as Garage, SeaweedFS or AWS S3.

## Without Task

The same setup by hand, one terminal per process. Run these from the repo root.

```sh
# 1. Postgres and storage
podman compose up -d --wait            # or: docker compose up -d --wait

# 2. OCR sidecar, on :8090
cd hermes-cv
python -m venv .venv && .venv/bin/pip install -r requirements.txt
.venv/bin/python server.py

# 3. API, on :8080 (new terminal, from the repo root)
cd hermes-web
export HERMES_DATABASE_URL=postgres://hermes:hermes@localhost:5432/hermes
export HERMES_S3_ACCESS_KEY=hermes HERMES_S3_SECRET_KEY=hermes-secret
go run ./cmd/api

# 4. Frontend, on :5173 (new terminal, from the repo root)
cd hermes-web/web
pnpm install && pnpm dev
```

Check each one:
- `curl localhost:8090/health` returns `{"status": "ok"}`.
- `curl localhost:8080/api/health` returns `{"status":"ok"}`.
- The dev server proxies `/api` to the API.

## Try it

1. Open http://localhost:5173.
2. Choose **Statement of Account**, then **Upload a statement**.
3. Upload an SOA PDF.

The document page follows the processing live, one page at a time. A 28-page statement takes about a minute.

Without the browser:

```sh
curl -F 'file=@"statement.pdf"' localhost:8080/api/documents     # 202, returns the document's id
curl -N localhost:8080/api/documents/<id>/events                 # live status (server-sent events)
curl localhost:8080/api/documents/<id>                           # current status and counts
```

Uploading the same file twice returns `409` with the earlier document's id.

## Configuration

The API reads `HERMES_*` environment variables. Only the first three in this list have no default.

| Variable | Default | |
|---|---|---|
| `HERMES_DATABASE_URL` | (required) | Postgres connection URL |
| `HERMES_S3_ACCESS_KEY` | (required) | Object store credentials |
| `HERMES_S3_SECRET_KEY` | (required) | |
| `HERMES_PORT` | `8080` | |
| `HERMES_S3_ENDPOINT` | `localhost:9000` | The sidecar must reach the store under this same host name, because presigned URLs are signed for it |
| `HERMES_S3_BUCKET` | `hermes-documents` | Created if missing |
| `HERMES_S3_USE_SSL` | `false` | |
| `HERMES_PIPELINE_URL` | `http://localhost:8090` | The OCR sidecar |
| `HERMES_PIPELINE_TIMEOUT` | `10m` | |
| `HERMES_MAX_UPLOAD_MB` | `100` | If you change it, change `MAX_UPLOAD_MB` in `web/src/features/soa/uploadErrors.ts` too |
| `HERMES_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `HERMES_SHUTDOWN_TIMEOUT` | `10s` | |

The sidecar's port is set by `HERMES_PIPELINE_PORT` (default `8090`).

## Development

```sh
task check              # Go tests, gofmt, go vet, oxlint, and type-check + build the frontend
task generate           # after editing hermes-web/api/openapi.yaml
task cv:run -- statement.pdf out/   # OCR one file; writes out/document.json and per-page intermediates
```

`CLAUDE.md` describes the architecture and conventions in depth.

## Troubleshooting

- **A document stays queued.** The API retries the sidecar every 10 s. Check that `server.py` is running and that `HERMES_PIPELINE_URL` points at it.
- **A document fails with a download error.** The sidecar couldn't fetch the PDF from the object store. It must reach the store at the host in `HERMES_S3_ENDPOINT`. That matters when the sidecar runs in a container, where `localhost` means the container itself.
- **`tesseract` not found.** Install it (see Prerequisites) and make sure it's on the `PATH` of the shell that runs `server.py`.
- **A port is already in use.** Set a different port in `.env`: `HERMES_PG_PORT` or `HERMES_S3_PORT`, along with the matching `HERMES_DATABASE_URL` or `HERMES_S3_ENDPOINT`.
- **`task infra:up` fails with "looking up compose provider".** Podman needs a compose provider. Install `docker-compose` or `podman-compose`.

## Stopping

- **Stop the app processes:** Ctrl+C in the `task dev` terminal.
- **Stop Postgres and storage:** `task infra:down`. Their data is kept in compose volumes.
- **Delete that data:** `task infra:reset`.

Uploaded statements are financial documents. Keep sample PDFs out of git: `.gitignore` already excludes `hermes-cv/sample-files/*.pdf`.
