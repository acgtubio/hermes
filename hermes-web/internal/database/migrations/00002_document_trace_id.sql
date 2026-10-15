-- +goose Up
-- The trace ID of the upload request, so the worker's and sidecar's logs for the document carry it.
ALTER TABLE documents ADD COLUMN trace_id text;

-- +goose Down
ALTER TABLE documents DROP COLUMN trace_id;
