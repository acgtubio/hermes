-- +goose Up
-- Every change to a document's status is announced on the document_events channel, with the document
-- id as the payload. NOTIFY is delivered on commit, so listeners only ever see committed states.
-- The trigger is the single source of these events: code that changes a status needn't notify.
-- +goose StatementBegin
CREATE FUNCTION documents_notify() RETURNS trigger AS $$
BEGIN
  PERFORM pg_notify('document_events', NEW.id::text);
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER documents_notify
  AFTER INSERT OR UPDATE OF status ON documents
  FOR EACH ROW EXECUTE FUNCTION documents_notify();

-- +goose Down
DROP TRIGGER documents_notify ON documents;
DROP FUNCTION documents_notify();
