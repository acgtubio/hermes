# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository layout

## Local development

The repo root has `Taskfile.yml` (Task 3; on Arch the binary is `go-task`), `compose.yaml` and `.env.example`. `README.md` covers the user-facing steps.

Everyday commands:
- `task setup` installs everything. It's idempotent, because it fingerprints `requirements.txt`, `go.sum` and `pnpm-lock.yaml`.
- `task dev` runs `infra:up`, then `run`. That's the sidecar, the API and Vite in one prefixed stream (`go-task --parallel --output prefixed cv api web`).
- `task check` runs the Go tests, gofmt, go vet, oxlint and both builds.

How it's wired:
- **Compose services:** `compose.yaml` holds only the backing services: Postgres 17 and RustFS. The app processes run on the host.
- **Podman or Docker:** `COMPOSE` resolves to `podman compose` when podman is installed, else `docker compose`.
  - Keep `compose.yaml` to features both support.
  - RustFS's image has no curl, so it has no healthcheck. Instead, `infra:up` polls its port, because the API exits if it can't create its bucket at startup.
- **Settings:** they load from `dotenv: ['.env', '.env.example']`. The shell beats `.env`, which beats `.env.example`; the earlier file wins.
  - `compose.yaml` reads the same `HERMES_PG_*`/`HERMES_S3_*` variables, with matching `${VAR:-default}` fallbacks.
  - A new setting goes in `.env.example`, and in `compose.yaml` if a container needs it.
- **Task quirks:**
  - Task has no per-command `dir`, so work in another directory gets its own sub-task (`lint:api`, `build:web`).
  - `output` is only global, which is why the prefixed output is requested on the nested `--parallel` call rather than set file-wide.

Early-stage project with two independent parts (no shared build):

- `hermes-cv/` — Python prototype scripts that OCR a scanned tabular document (an invoice/withholding-tax listing) into structured rows and validate them.
- `hermes-web/` — Go backend (module `github.com/acgtubio/hermes`, go 1.26) plus `web/`, a React 19 + TypeScript + Vite frontend using TanStack Router (file-based routes) and TanStack Query.

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
4. The browser follows `GET /api/documents/{id}/events` (SSE), or polls `GET /api/documents/{id}`.

Details:
- The `documents` table is the queue; there's no Redis. One document runs at a time, because the pipeline already uses every core.
- If the sidecar is busy (`503`) or down, the document is requeued and retried every 10 s.
- At startup, `RequeueStale` requeues documents left `processing`. That assumes a single API instance.
- `internal/pipeline` converts the sidecar's JSON to `soa` types:
  - Typed columns are NULL when a cell is `failed` or doesn't parse. The raw cell (`text`, `conf`, `bbox`, `status`, `first_read`) is always kept in the `cells` JSONB.
  - `needs_review` means a cell failed or was inferred.
  - The amount format rule matches `validate.num` in hermes-cv. Keep them in step.

Status events (SSE):
- `GET /api/documents/{id}/events` sends a `document` event on connect, after each status change and, while processing, after each finished page (`pages_done` of `pages_total`). Its data is the same JSON as `GET /documents/{id}`. After `ready` or `failed` it sends `end` and closes. It sends a `: ping` every 20 s.
- The `documents_notify` trigger (migrations 00003 and 00004) is the single source of change events. It runs `pg_notify('document_events', id)` on insert and on every update of `status` or `pages_done`, so code that changes a status needn't notify, and manual SQL updates reach the streams too.
- `database.Listen` holds one hijacked LISTEN connection per API instance and feeds `events.Hub`. That works whichever instance or process made the change. On reconnect it wakes every stream (`PublishAll`), since notifications sent while it was down are lost.
- Wake-ups carry no data. The stream re-reads the row and sends it only if it changed, so bursts coalesce, and a reconnecting browser just gets the current state again. There are no event IDs to replay.
- A stream must subscribe *before* its first read (`StreamDocumentEvents` does), or a change can slip in between.
- The stream implements the generated response interface itself. It clears the connection's read and write deadlines with `http.ResponseController`, because `WriteTimeout` (30 s) would cut it and `ReadTimeout` would cancel its context.
- Progress is stored, not just relayed. The sidecar streams it to the worker, and the worker writes it to `documents.pages_done`/`pages_total` (`SetProgress`). The trigger then turns each write into an event on whichever instance the browser is connected to, and a reconnecting browser gets the current count. `ClaimNext`, `Requeue` and `RequeueStale` clear it.
- `main.go` registers `StopStreams` with `RegisterOnShutdown`. Without it, `Shutdown` would wait out its timeout on open streams.

Database and storage:
- Migrations are goose SQL files in `internal/database/migrations/NNNNN_name.sql`. They're embedded and applied at startup by `database.Migrate`.
- Object storage goes through `internal/storage`, which uses only the plain S3 API through minio-go. Keep it that way: MinIO's free distribution has been scaled back (images are no longer pullable from Docker Hub or quay.io). RustFS, Garage, SeaweedFS and AWS S3 all work as replacements. For dev, `docker.io/rustfs/rustfs` with `RUSTFS_ACCESS_KEY`/`RUSTFS_SECRET_KEY` works on :9000.
- Presigned URLs are signed for `HERMES_S3_ENDPOINT`'s host, so the sidecar must reach the store under that same name.

Configuration:
- Config is owned by the binary: `cmd/api/config.go`, using envconfig with the `HERMES_` prefix. The variables are:
  - `HERMES_PORT` (8080)
  - `HERMES_DATABASE_URL` (required)
  - `HERMES_SHUTDOWN_TIMEOUT` (10s)
  - `HERMES_LOG_LEVEL` (info; a zap level: debug, info, warn, error)
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

Logging and trace IDs:
- Logging uses zap (`*zap.Logger`), built by `logging.New`: JSON on stdout. Log with typed fields (`zap.Error(err)`, `zap.Stringer("document_id", id)`), not key/value pairs.
- Whenever a context is available, log through `logging.Ctx(ctx, log)`, which adds the `trace_id` field.
- Tests use `zap.NewNop()`, or `zaptest/observer` to assert on log entries (see `newObservedRouter`).
- Trace IDs use the W3C trace-context format (`internal/trace`):
  - The `traceIDs` middleware takes the ID from an incoming `traceparent` header, or generates one. It's put in the context and returned as `X-Trace-Id`.
  - An upload's trace ID is stored on its `documents` row (`trace_id`). The worker processes the document under it and forwards it to the sidecar in a `traceparent` header.
  - The `Document` JSON returns it as `trace_id`, on `GET /documents/{id}` and in the SSE events, so the UI can show it on a failed document.
  - So the upload request, the job's logs and the sidecar's `[trace <id>]` lines can all be found by one ID. A document without a stored ID gets a fresh one when processed.
  - This is not OpenTelemetry (there are no spans or exporter), but the format stays compatible if OTel is added later.

## hermes-web/web (frontend)

Uses pnpm. Run from `hermes-web/web/`:

```sh
pnpm install
pnpm dev       # Vite dev server; /api is proxied to http://localhost:8080 (the Go backend)
pnpm build     # tsc -b && vite build
pnpm lint      # oxlint (not ESLint), config in .oxlintrc.json
```

Architecture: one SPA split by feature (a modular monolith, not micro frontends). The landing page (`/`) lists the features, and each feature is its own route subtree.

```
src/
  main.tsx          app shell: the one QueryClient and router, nothing feature-specific
  routes/           TanStack Router file routes; thin, they only render a feature's pages
    __root.tsx      AppLayout around <Outlet />; router context { queryClient }
    index.tsx       / : landing page, the tools list
    soa/route.tsx   the /soa section (its crumb); index.tsx /soa, upload.tsx /soa/upload,
                    documents/$documentId.tsx /soa/documents/{id} (live status)
  features/<name>/  everything the feature owns: pages, hooks, CSS and its API client
  shared/           only the UI kit (shared/ui) and the layout (shared/layout)
  routeTree.gen.ts  generated by the router plugin; committed, never hand-edited
```

Rules:
- **Features never import from each other.** They may import from `shared/`.
- **Each feature keeps its own API client:** fetch calls, query keys, hooks, and types for just the operations it uses (`features/soa/api.ts`). Don't move API code to `shared/` until a second feature actually needs it. Copying a small helper once is fine. Extract it when duplication actually shows up.
- **App-level wiring isn't shared code.** The `QueryClient` and router live in `main.tsx`.
- **Each feature is downloaded only when opened.** The router plugin's `autoCodeSplitting` splits every route's component into its own chunk, so the landing page doesn't load SOA code. Keep route files importing from their own feature only, so this split holds.
- **Adding a feature:**
  1. Create `features/<name>/`.
  2. Add `routes/<name>/` files.
  3. Add a `LinkListRow` for it in `routes/index.tsx`.
- **The route tree is generated.** The router plugin writes `routeTree.gen.ts` whenever Vite runs, in dev or build. `tsc` needs that file, so after adding or renaming a route file, run `pnpm dev` or `pnpm exec vite build` once before `pnpm build`.
- **Lint exception for routes:** `react/only-export-components` is off for `src/routes/`. Route files export `Route`, and the router plugin handles their hot reload.

Design system (the approved canvas: https://claude.ai/artifact/A7tWcEUkQ66wpuk7hw9u9d):
- **Tokens:** colour tokens live on `:root` in `src/index.css`, with dark values under `prefers-color-scheme`. Use the tokens; never write a hex value in component CSS. The colour rules:
  - **ink** is for text, the logo and primary buttons.
  - **accent** (blue) is only for links, progress, focus and the current step.
  - **ok/warn/danger** are only for status.
  - **line** is for dividers and panels, and **control** is for control edges (3:1).
  - **Green means nothing is left to check.** A ready document with rows to review gets the amber *Needs review* badge.
- **No decorative motion:** no transitions, animations, shadows or gradients. The progress bar jumps to each value.
- **Type:** IBM Plex Sans (400/500/600), self-hosted via `@fontsource`. Plex Mono is only for machine IDs such as trace IDs. Numbers use `tabular-nums`. Copy is in sentence case.
- **UI kit:** components live in `shared/ui/` (one `ui.css`): `Button`/`ButtonLink`, `Panel`, `Badge`, `Steps`, `ProgressBar`, `Notice`, `Facts`, `LinkList`, `FileRow`, `DropZone`, plus the icons.
  - `ButtonLink` and `LinkListRow` come from `createLink`, so they take typed `to`/`params`. That's also why they're whitelisted in `.oxlintrc.json`.
  - Feature-specific layout goes in the feature's own CSS (`features/soa/soa.css`).
- **Phone width:** at under 600 px the header shows only the logo, buttons in `.soa-actions` go full width, steps use two columns, and facts stack.
- **Header crumbs:** the header is the route path (`AppLayout`, `useMatches`). A route names its step with `staticData: { crumb }`, or with a loader that returns `{ crumb }`, which wins.
  - The document route's loader runs `queryClient.ensureQueryData` with the feature's own `documentKeys`/`getDocument`. That primes the cache `useDocument` reads, and names the crumb after the file. Its `errorComponent` handles unknown IDs.

The SOA feature:
- **Types are maintained by hand.** `features/soa/api.ts` copies the spec's `Document` type. Keep them in step until TS types are generated from `api/openapi.yaml`. When that happens, generate per feature, not into `shared/`.
- **Live status comes from SSE.** `useDocument` (`features/soa/hooks.ts`) opens an `EventSource` on `/api/documents/{id}/events` while the document isn't ready or failed. It writes each `document` event into the query cache, and closes the stream on `end`; without that close, EventSource would reconnect forever.
- **Duplicates link to the earlier upload.** On a `409`, the upload page parses the existing document's ID out of the error message (`ApiError.duplicateOf`) and links to it.
- **Upload error wording:** it lives in `features/soa/uploadErrors.ts`, shared by the browser-side checks (type, size) and the server's 413/415. `MAX_UPLOAD_MB` must match the API's `HERMES_MAX_UPLOAD_MB`.
- **The failed state shows the document's `trace_id`.** This is the upload's ID, which the job's logs also carry, with a copy button. Don't use an `X-Trace-Id` response header for this, because that names only the request that read the page.
- **Two buttons wait on missing endpoints.** There's no "Try again" on failed documents, because re-uploading returns 409 and there's no retry endpoint yet. There's no "Review rows" either, because there's no rows endpoint yet. Add each one with its endpoint.

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
- `POST /process {"url": ...}` downloads the PDF from the presigned URL. It answers with newline-delimited JSON:
  - `{"type": "progress", "pages_done", "page_count"}`, once the pages are rendered (0) and then per page;
  - then either `{"type": "result", "document"}` or `{"type": "error", "error"}`.

  Failures before the stream starts keep their status codes: 400, 503 when busy, 422 when the download fails. `pipeline.Client` reads the stream with a `json.Decoder`, because the result line is the whole document, megabytes long.
- It handles one job at a time and returns `503` while busy.
- When the caller disconnects, the next progress write fails and `run.process` stops the pool's workers (`terminate_workers`, Python 3.14+). The sidecar is then free within seconds, instead of OCRing the rest of the document. Tesseract calls already running finish on their own. On older Python only the pages not yet handed to workers are cancelled.
- It holds no storage credentials.
- It prefixes every log line with `[trace <id>]`, taken from the caller's `traceparent` header (`-` if there isn't one). Its error bodies include `trace_id`.

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
