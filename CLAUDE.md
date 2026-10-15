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

It processes one document type: the vendor's Statement of Account (SOA). Page 1 is a cover summary, and the following pages are a ruled table (Doc Date … Net Amount), scanned portrait with the table rotated 90°. Samples are in `sample-files/` (Jun 16–30, Jul 16–31 and Sep 1–15 2026). Check every change against all of them. They differ in ways a single sample hides: page orientation, edge shadows, warped paper, handwritten ticks and pencil marks, and totals above a million. Keep thresholds relative (to image size or text height) rather than tuned to one file.

Install with `pip install -r requirements.txt`; the `tesseract` binary must also be on PATH.

`python run.py <doc.pdf|scan.png> [outdir]` runs everything:
- It writes each page's intermediates to `<outdir>/page-NN/` (default `out/`).
- It skips pages with no table rows (the cover).
- It validates totals across pages.

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
   - Row amounts: re-OCRed the same way, kept only if the row arithmetic then holds.
   - Subtotal and footer amounts: kept once they parse; the cross-page check verifies them.
6. `python validate.py [rows.json ...]` checks the following, using `Decimal` throughout:
   - field formats (`FORMATS`). Amounts group only the last thousands, e.g. `1001,579.43`; that's how the report prints them, not an OCR slip.
   - sales + vat = total
   - total − withholding = net
   - withholding = 2% of sales, rounded half-up
   - cells with OCR conf < 80
   - subtotals against the rows in their group (groups can span pages)
   - footers. Each page's NET AMOUNT is a **running total** across the document, so pass every page's `rows.json` in order. Given a single page, mismatches are expected and say so.

Key coupling to keep in mind when editing:
- Bboxes in `rows.json` are relative to the table crop. `table.json` (`x`, `y`, `w`) holds the crop origin and width, which `reread.py` adds back.
- The `rows.COLUMNS` order matches `EDGES` (11 columns, 10 edges). Columns from index 6 onward are the amount columns, and `validate.AMOUNTS` lists the same fields.
