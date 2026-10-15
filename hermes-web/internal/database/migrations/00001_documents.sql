-- +goose Up
CREATE TYPE document_status AS ENUM ('queued', 'processing', 'ready', 'failed');

-- One uploaded PDF. Doubles as the processing queue: the worker claims the oldest queued row.
CREATE TABLE documents (
  id           uuid PRIMARY KEY,
  filename     text NOT NULL,
  object_key   text NOT NULL,
  sha256       text NOT NULL UNIQUE,
  size_bytes   bigint NOT NULL,
  status       document_status NOT NULL DEFAULT 'queued',
  error        text,
  page_count   int,
  created_at   timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz
);
CREATE INDEX documents_queued_idx ON documents (created_at) WHERE status = 'queued';

-- Typed columns are NULL when the cell failed its check; the raw read is always in cells:
-- {field: {text, conf, bbox, status, first_read?}}. No arithmetic CHECK, since rows with failed
-- cells are stored on purpose so they can be reviewed.
CREATE TABLE soa_rows (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  document_id         uuid NOT NULL REFERENCES documents ON DELETE CASCADE,
  page                int NOT NULL,
  row_index           int NOT NULL,
  doc_date            date,
  po_number           text,
  service_entry_sheet text,
  reference_no        text,
  doc_header_text     text,
  description         text,
  sales               numeric(14,2),
  vat                 numeric(14,2),
  total_sales_w_vat   numeric(14,2),
  withholding_tax     numeric(14,2),
  net_amount          numeric(14,2),
  cells               jsonb NOT NULL,
  needs_review        boolean NOT NULL,
  UNIQUE (document_id, page, row_index)
);
CREATE INDEX soa_rows_po_number_idx ON soa_rows (po_number);
CREATE INDEX soa_rows_service_entry_sheet_idx ON soa_rows (service_entry_sheet);
CREATE INDEX soa_rows_needs_review_idx ON soa_rows (document_id) WHERE needs_review;

CREATE TABLE soa_totals (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  document_id       uuid NOT NULL REFERENCES documents ON DELETE CASCADE,
  page              int NOT NULL,
  kind              text NOT NULL CHECK (kind IN ('subtotal', 'footer')),
  after_row         int,
  sales             numeric(14,2),
  vat               numeric(14,2),
  total_sales_w_vat numeric(14,2),
  withholding_tax   numeric(14,2),
  net_amount        numeric(14,2),
  cells             jsonb NOT NULL,
  needs_review      boolean NOT NULL
);
CREATE INDEX soa_totals_document_idx ON soa_totals (document_id, page);

-- +goose Down
DROP TABLE soa_totals;
DROP TABLE soa_rows;
DROP TABLE documents;
DROP TYPE document_status;
