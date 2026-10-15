-- +goose Up
-- Pipeline progress while a document is processing: pages_done of pages_total, written by the worker
-- as the sidecar reports each page. The notify trigger also fires on pages_done, so event streams
-- (on any instance) send progress the same way they send status changes.
ALTER TABLE documents ADD COLUMN pages_done int, ADD COLUMN pages_total int;

DROP TRIGGER documents_notify ON documents;
CREATE TRIGGER documents_notify
  AFTER INSERT OR UPDATE OF status, pages_done ON documents
  FOR EACH ROW EXECUTE FUNCTION documents_notify();

-- +goose Down
DROP TRIGGER documents_notify ON documents;
CREATE TRIGGER documents_notify
  AFTER INSERT OR UPDATE OF status ON documents
  FOR EACH ROW EXECUTE FUNCTION documents_notify();
ALTER TABLE documents DROP COLUMN pages_done, DROP COLUMN pages_total;
