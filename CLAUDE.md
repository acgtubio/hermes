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
HERMES_DATABASE_URL=postgres://... go run ./cmd/api  # serves on :8080
go test ./...
go test ./internal/server -run TestGetHealth         # single test
```

The API is spec-first with oapi-codegen's strict server:
- `api/openapi.yaml` is the source of truth.
- oapi-codegen is pinned as a `tool` in go.mod and runs via `go tool oapi-codegen`. It is driven by `internal/api/generate.go` and `oapi-codegen.yaml`.
- To add an endpoint:
  1. Edit the spec.
  2. Run `go generate ./...`.
  3. Implement the new method on `*server.Server` in `internal/server/handlers.go`. The compile-time assertion `var _ api.StrictServerInterface` fails the build until you do.
- Never hand-edit `api.gen.go`.
- Enum constants are type-prefixed, e.g. `api.HealthStatusOk`.

Request flow:
- `cmd/api/main.go` wires everything up and owns `ListenAndServe` and graceful shutdown.
- `internal/server.NewRouter` mounts the generated handlers under `/api` on chi.
- Requests are validated against the embedded spec by `nethttp-middleware`. Its `Prefix: "/api"` option stands in for the spec's `servers` entry.
- `/api/openapi.json` serves the embedded spec.

Configuration:
- Config is owned by the binary: `cmd/api/config.go`, using envconfig with the `HERMES_` prefix. The variables are `HERMES_PORT` (8080), `HERMES_DATABASE_URL` (required), `HERMES_SHUTDOWN_TIMEOUT` (10s) and `HERMES_LOG_LEVEL` (info).
- Internal packages must not import config. They take plain values such as `database.New(ctx, url)`.
- To add a new dependency (e.g. Redis): add a config field, construct the client in `main.go`, and pass it into `server.New`.
- Handlers depend on small interfaces (e.g. `server.Pinger`) rather than `*pgxpool.Pool`, so tests can use fakes.

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

Install with `pip install -r requirements.txt`; the `tesseract` binary must also be on PATH. They are meant to be run in order from one working directory, and each stage hands off to the next through files in the current directory:

1. `python normalize.py <scan.png>` → `norm.png`. Fixes coarse rotation with Tesseract OSD (`--psm 0`), then deskews using the median angle of long horizontal ruling lines (Hough).
2. `python ocr_body.py` → `words.csv`. Crops the table body and runs word-level OCR (`--psm 6`). Adds the centre columns `cx`/`cy`.
3. `python rows.py` → `rows.json`. Infers column boundaries from vertical gaps of at least 25px with no word coverage, groups words into lines by `cy`, and starts a record on each line whose first word is a `dd.mm.yyyy` date. Other lines are continuations and get appended to the previous record. A line containing `NET` and `AMOUNT` is parsed as the totals footer.
4. `python validate.py` reads `rows.json` and checks the following:
   - field formats (10-digit PO/SES numbers, the date)
   - sales + vat = total
   - total − withholding = net
   - withholding = 2% of sales
   - cells with OCR conf < 80
   - column sums against the footer

   It uses `Decimal` throughout.
5. `python reread.py` (optional) re-OCRs selected cells from their bboxes using single-line mode and per-field character whitelists, and reports where that disagrees with the first pass.

Key coupling to keep in mind when editing:
- Crop constants are hardcoded for one specific scan layout: `X0=380, X1=3384, BODY_TOP=566` in `ocr_body.py` and `reread.py`, and `W=3004` (= `X1−X0`) in `rows.py`. Bboxes in `rows.json` are relative to that crop, which is why `reread.py` adds `X0`/`BODY_TOP` back.
- `rows.py`'s `names` list maps column index to field name in order (`doc_date … net_amount`). It assumes gap detection finds exactly 10 gaps, i.e. 11 columns. Indices ≥ 6 are the numeric amount columns used for the footer.
