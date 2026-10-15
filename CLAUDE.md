# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository layout

Early-stage project with two independent parts (no git repo, no shared build):

- `hermes-cv/` — Python prototype scripts that OCR a scanned tabular document (an invoice/withholding-tax listing) into structured rows and validate them.
- `hermes-web/` — Go backend (module `github.com/acgtubio/hermes`, go 1.26) plus `web/`, a React 19 + TypeScript + Vite frontend that is still the stock Vite template. Dependencies already include `@tanstack/react-router` and `@tanstack/react-query`, though neither is wired up yet.

## hermes-web (Go backend)

Run from `hermes-web/`:

```sh
go generate ./...                                   # regenerate internal/api/api.gen.go from api/openapi.yaml
HERMES_DATABASE_URL=postgres://... HERMES_S3_ACCESS_KEY=... HERMES_S3_SECRET_KEY=... go run ./cmd/api  # serves on :8080
go test ./...
go test ./internal/server -run TestGetHealth         # single test
```

The API is spec-first with oapi-codegen's strict server:
- `api/openapi.yaml` is the source of truth.
- oapi-codegen is pinned as a `tool` in go.mod and runs via `go tool oapi-codegen`. It is driven by `internal/api/generate.go` and `oapi-codegen.yaml`.
- To add an endpoint:
  1. Edit the spec.
  2. Run `go generate ./...`.
  3. Implement the new method on `*server.Server` in its own file, named after the operation: `internal/server/<operation>.go`, e.g. `get_document.go`. Put its tests in `<operation>_test.go`. The compile-time assertion `var _ api.StrictServerInterface` in `handlers.go` fails the build until you do.
- `internal/server/handlers.go` holds what the handlers share: `Server`, `Deps`, `New`, the dependency interfaces and `toAPI`. Shared test fakes go in `handlers_test.go`.
- Never hand-edit `api.gen.go`.
- Enum constants are type-prefixed, e.g. `api.HealthStatusOk`.

Request flow:
- `cmd/api/main.go` wires everything up and owns `ListenAndServe` and graceful shutdown.
- `internal/server.NewRouter` mounts the generated handlers under `/api` on chi.
- Requests are validated against the embedded spec by `nethttp-middleware`. Its `Prefix: "/api"` option stands in for the spec's `servers` entry.
- `/api/openapi.json` serves the embedded spec.
- `POST /api/documents` skips request-body validation (`uploadRoute` in `handlers.go`), because kin-openapi `io.ReadAll`s the body and uploads are up to `HERMES_MAX_UPLOAD_MB`. The handler streams the multipart part straight to object storage instead. Every other route is validated in full.

Document processing:
1. The browser uploads a PDF to `POST /api/documents`. The handler streams it to `documents/<id>.pdf` in the bucket, hashing as it goes. It inserts a `queued` row in `documents` and returns `202`. The same SHA-256 uploaded again returns `409`.
2. `internal/worker` runs in-process. It claims the oldest queued document (`FOR UPDATE SKIP LOCKED`) and hands the sidecar (`hermes-cv/server.py`) a 15-minute presigned GET URL.
3. It stores the result in one transaction (`soa_rows`, `soa_totals`) and marks the document `ready`. If the pipeline errors, the document is marked `failed` with the error.
4. The browser polls `GET /api/documents/{id}`.

Details:
- The `documents` table is the queue; there's no Redis. One document runs at a time, because the pipeline already uses every core.
- If the sidecar is busy (`503`) or down, the document is requeued and retried every 10 s.
- At startup, `RequeueStale` requeues documents left `processing`. That assumes a single API instance.
- `internal/pipeline` converts the sidecar's JSON to `soa` types:
  - Typed columns are NULL when a cell is `failed` or doesn't parse. The raw cell (`text`, `conf`, `bbox`, `status`, `first_read`) is always kept in the `cells` JSONB.
  - `needs_review` means a cell failed or was inferred.
  - The amount format rule matches `validate.num` in hermes-cv. Keep them in step.

Database and storage:
- Migrations are goose SQL files in `internal/database/migrations/NNNNN_name.sql`. They're embedded and applied at startup by `database.Migrate`.
- Object storage goes through `internal/storage`, which uses only the plain S3 API through minio-go. Keep it that way: MinIO's free distribution has been scaled back (images are no longer pullable from Docker Hub or quay.io). RustFS, Garage, SeaweedFS and AWS S3 all work as replacements. For dev, `docker.io/rustfs/rustfs` with `RUSTFS_ACCESS_KEY`/`RUSTFS_SECRET_KEY` works on :9000.
- Presigned URLs are signed for `HERMES_S3_ENDPOINT`'s host, so the sidecar must reach the store under that same name.

Configuration:
- Config is owned by the binary: `cmd/api/config.go`, using envconfig with the `HERMES_` prefix. The variables are:
  - `HERMES_PORT` (8080)
  - `HERMES_DATABASE_URL` (required)
  - `HERMES_SHUTDOWN_TIMEOUT` (10s)
  - `HERMES_LOG_LEVEL` (info)
  - `HERMES_S3_ENDPOINT` (localhost:9000)
  - `HERMES_S3_ACCESS_KEY`, `HERMES_S3_SECRET_KEY` (required)
  - `HERMES_S3_BUCKET` (hermes-documents; created if missing)
  - `HERMES_S3_USE_SSL` (false)
  - `HERMES_PIPELINE_URL` (http://localhost:8090)
  - `HERMES_PIPELINE_TIMEOUT` (10m)
  - `HERMES_MAX_UPLOAD_MB` (100)
- Internal packages must not import config. They take plain values such as `database.New(ctx, url)`.
- To add a new dependency (e.g. Redis): add a config field, construct the client in `main.go`, and pass it into `server.New` through `server.Deps`.
- Handlers and the worker depend on small interfaces rather than concrete clients, so tests can use fakes. Examples: `server.Pinger`, `server.DocumentStore`, `server.ObjectStore`, `worker.Queue`.

## hermes-web/web (frontend)

Uses pnpm. Run from `hermes-web/web/`:

```sh
pnpm install
pnpm dev       # Vite dev server; /api is proxied to http://localhost:8080 (the Go backend)
pnpm build     # tsc -b && vite build
pnpm lint      # oxlint (not ESLint), config in .oxlintrc.json
```

There is no test runner configured. The TS config is strict about unused locals/params and uses `verbatimModuleSyntax` + `erasableSyntaxOnly`, so use `import type` for type-only imports and avoid enums/namespaces/parameter properties.

## hermes-cv (OCR pipeline)

It processes one document type: the vendor's Statement of Account (SOA). Page 1 is a cover summary, and the following pages are a ruled table (Doc Date … Net Amount), scanned portrait with the table rotated 90°. Samples are in `sample-files/` (Jun 16–30, Jul 16–31 and Sep 1–15 2026). Check every change against all of them. They differ in ways a single sample hides: page orientation, edge shadows, warped paper, handwritten ticks and pencil marks, and totals above a million. Keep thresholds relative (to image size or text height) rather than tuned to one file.

Install with `pip install -r requirements.txt`; the `tesseract` binary must also be on PATH.

`python run.py <doc.pdf|scan.png> [outdir]` runs everything (`run.process`):
- It writes each page's intermediates to `<outdir>/page-NN/` (default `out/`).
- It skips pages with no table rows (the cover).
- It validates totals across pages.
- It writes the whole document to `<outdir>/document.json`: `{page_count, pages: [{page, rows, subtotals, footer}], flags, totals}`.

`python server.py` is the sidecar the Go API calls. It listens on `HERMES_PIPELINE_PORT`, default 8090.
- `POST /process {"url": ...}` downloads the PDF from the presigned URL and returns `run.process`'s document.
- It handles one job at a time and returns `503` while busy.
- A caller that disconnects doesn't cancel the job in progress.
- It holds no storage credentials.

The stage scripts can also be run one at a time. Run them in order from one working directory; each stage hands off to the next through files in the current directory:

1. `python ingest.py <doc.pdf> [outdir]` → `page-NN.png`. Renders each page at the embedded scan's native resolution, so pixels aren't resampled. The page's `/Rotate` is applied, so the output is upright as displayed.
2. `python normalize.py <page.png>` → `norm.png`. It does three things:
   - fixes coarse rotation with Tesseract OSD (`--psm 0`);
   - whitens scanner-edge shadows: dark blobs that touch the border within `EDGE_BAND`;
   - deskews by searching ±5° for the angle that makes the horizontal ink profile sharpest. This doesn't depend on ruling lines.
3. `python ocr_body.py` → `table.json` and `words.csv`.
   - `find_table` locates the table from its long horizontal rules, using a short kernel so curved rules on warped pages survive. The crop runs from the top rule to the image bottom; the scan often cuts the table off.
   - `clean` flattens the background and whitens faint scanner streaks. The image stays grayscale: binarizing hurt accuracy, and despeckling deletes decimal points.
   - OCR runs with `--psm 6`. Low-confidence punctuation-only tokens are dropped as scan noise.
4. `python rows.py` → `rows.json`.
   - Columns come from `EDGES`, fixed fractions of the table width. The SOA layout is fixed, and per-page gap detection broke on stray marks.
   - Words are grouped into lines using Tesseract's own line IDs (`block_num`/`par_num`/`line_num`). On warped pages a row drifts by more than its height, so grouping by `cy` splits it.
   - A row starts on a line whose first word is a `dd.mm.yyyy` date; other lines are continuations of the previous row.
   - A dateless line with ≥3 amounts is a totals line: the last one is the NET AMOUNT footer, earlier ones are `subtotals`. Text after the footer (the signature block) is ignored.
5. `python reread.py` repairs cells that fail a check. Repaired cells keep `first_read`, and `run.py` does this automatically.
   - ID fields: if exactly one token matches the format (a handwritten tick read as `4`), it keeps that token. Otherwise it re-OCRs the cell as a single line with the field's character whitelist and keeps the result only if it matches the format.
   - Row amounts: re-OCRed the same way, kept only if the row arithmetic then holds. If no re-read fixes the row, `validate.infer` recomputes each amount from the other four and applies the result only when exactly one cell makes the row hold. This covers unreadable cells and misreads that still parse (`1,639.00` for `1,539.00`).
   - Subtotal and footer amounts: kept once they parse; the cross-page check verifies them.
   - Every cell gets a `status`, the signal the backend should use to warn users. `conf` stays the first-pass OCR confidence; it's a secondary hint and is often high on misreads.
     - `ok`: passed its field's check (row arithmetic for amounts, `FORMATS` for IDs/date, the cross-page sums for subtotal/footer cells)
     - `repaired`: a re-read passed
     - `inferred`: computed from the row's arithmetic, not read
     - `failed`: still fails its check
     - `unchecked`: no check exists (`text`)

     Subtotal and footer statuses are set by `check_totals`, so only `run.py` writes them.
6. `python validate.py [rows.json ...]` checks the following, using `Decimal` throughout:
   - field formats (`FORMATS`). Amounts group only the last thousands, e.g. `1001,579.43`; that's how the report prints them, not an OCR slip.
   - sales + vat = total
   - total − withholding = net
   - withholding = 2% of sales, rounded half-up
   - cells with OCR conf < 80
   - subtotals against the rows in their group (groups can span pages). Sums are per column, so an unreadable cell leaves only its own column short.
   - footers. Each page's NET AMOUNT is a **running total** across the document, so pass every page's `rows.json` in order. Given a single page, mismatches are expected and say so.

Key coupling to keep in mind when editing:
- Bboxes in `rows.json` are relative to the table crop. `table.json` (`x`, `y`, `w`) holds the crop origin and width, which `reread.py` adds back.
- The `rows.COLUMNS` order matches `EDGES` (11 columns, 10 edges). Columns from index 6 onward are the amount columns, and `validate.AMOUNTS` lists the same fields.
