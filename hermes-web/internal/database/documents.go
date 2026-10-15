package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/acgtubio/hermes/internal/soa"
)

// Document statuses, matching the document_status enum.
const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusReady      = "ready"
	StatusFailed     = "failed"
)

var ErrNotFound = errors.New("document not found")

// DuplicateError is returned by Create when a file with the same SHA-256 was already uploaded.
type DuplicateError struct{ ID uuid.UUID }

func (e *DuplicateError) Error() string { return fmt.Sprintf("already uploaded as document %s", e.ID) }

type NewDocument struct {
	ID        uuid.UUID
	Filename  string
	ObjectKey string
	SHA256    string
	Size      int64
	TraceID   string // of the upload request; "" if it had none
}

type Document struct {
	ID          uuid.UUID
	Filename    string
	ObjectKey   string
	Status      string
	Error       *string
	PageCount   *int
	CreatedAt   time.Time
	ProcessedAt *time.Time
	TraceID     string
	// Pipeline progress: set while processing (see SetProgress), cleared when (re)queued.
	PagesDone, PagesTotal *int
	// Set by Get once the document is ready.
	RowCount, NeedsReviewCount *int
}

// Documents stores uploaded documents and their parsed rows. The documents table is also the
// processing queue (ClaimNext / Complete / Fail / Requeue).
type Documents struct{ pool *pgxpool.Pool }

func NewDocuments(pool *pgxpool.Pool) *Documents { return &Documents{pool: pool} }

const documentCols = `id, filename, object_key, status, error, page_count, created_at, processed_at, coalesce(trace_id, ''), pages_done, pages_total`

func scanDocument(row pgx.Row) (Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.Filename, &d.ObjectKey, &d.Status, &d.Error, &d.PageCount, &d.CreatedAt, &d.ProcessedAt, &d.TraceID,
		&d.PagesDone, &d.PagesTotal)
	return d, err
}

// Create queues a new document. Returns *DuplicateError if its SHA-256 is already stored.
func (s *Documents) Create(ctx context.Context, n NewDocument) (Document, error) {
	d, err := scanDocument(s.pool.QueryRow(ctx, `
		INSERT INTO documents (id, filename, object_key, sha256, size_bytes, trace_id) VALUES ($1, $2, $3, $4, $5, nullif($6, ''))
		ON CONFLICT (sha256) DO NOTHING
		RETURNING `+documentCols, n.ID, n.Filename, n.ObjectKey, n.SHA256, n.Size, n.TraceID))
	if errors.Is(err, pgx.ErrNoRows) {
		var id uuid.UUID
		if err := s.pool.QueryRow(ctx, `SELECT id FROM documents WHERE sha256 = $1`, n.SHA256).Scan(&id); err != nil {
			return Document{}, fmt.Errorf("find duplicate: %w", err)
		}
		return Document{}, &DuplicateError{ID: id}
	}
	if err != nil {
		return Document{}, fmt.Errorf("insert document: %w", err)
	}
	return d, nil
}

func (s *Documents) Get(ctx context.Context, id uuid.UUID) (Document, error) {
	d, err := scanDocument(s.pool.QueryRow(ctx, `SELECT `+documentCols+` FROM documents WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("get document: %w", err)
	}
	if d.Status == StatusReady {
		var rows, review int
		if err := s.pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE needs_review) FROM soa_rows WHERE document_id = $1`, id).
			Scan(&rows, &review); err != nil {
			return Document{}, fmt.Errorf("count rows: %w", err)
		}
		d.RowCount, d.NeedsReviewCount = &rows, &review
	}
	return d, nil
}

// ClaimNext marks the oldest queued document as processing and returns it; ok is false when the
// queue is empty. SKIP LOCKED keeps concurrent claimers from taking the same document.
func (s *Documents) ClaimNext(ctx context.Context) (Document, bool, error) {
	d, err := scanDocument(s.pool.QueryRow(ctx, `
		UPDATE documents SET status = 'processing', pages_done = NULL, pages_total = NULL
		WHERE id = (SELECT id FROM documents WHERE status = 'queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING `+documentCols))
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, fmt.Errorf("claim document: %w", err)
	}
	return d, true, nil
}

// Complete stores the parsed rows and totals and marks the document ready, in one transaction.
func (s *Documents) Complete(ctx context.Context, id uuid.UUID, doc soa.Document) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"soa_rows"}, []string{
			"document_id", "page", "row_index", "doc_date", "po_number", "service_entry_sheet", "reference_no",
			"doc_header_text", "description", "sales", "vat", "total_sales_w_vat", "withholding_tax", "net_amount",
			"cells", "needs_review",
		}, pgx.CopyFromSlice(len(doc.Rows), func(i int) ([]any, error) {
			r := doc.Rows[i]
			return []any{id, r.Page, r.RowIndex, r.DocDate, r.PONumber, r.ServiceEntrySheet, r.ReferenceNo,
				r.DocHeaderText, r.Description, r.Sales, r.VAT, r.TotalSalesWithVAT, r.WithholdingTax, r.NetAmount,
				[]byte(r.Cells), r.NeedsReview}, nil
		}))
		if err != nil {
			return fmt.Errorf("copy rows: %w", err)
		}
		_, err = tx.CopyFrom(ctx, pgx.Identifier{"soa_totals"}, []string{
			"document_id", "page", "kind", "after_row", "sales", "vat", "total_sales_w_vat", "withholding_tax",
			"net_amount", "cells", "needs_review",
		}, pgx.CopyFromSlice(len(doc.Totals), func(i int) ([]any, error) {
			t := doc.Totals[i]
			return []any{id, t.Page, t.Kind, t.AfterRow, t.Sales, t.VAT, t.TotalSalesWithVAT, t.WithholdingTax,
				t.NetAmount, []byte(t.Cells), t.NeedsReview}, nil
		}))
		if err != nil {
			return fmt.Errorf("copy totals: %w", err)
		}
		_, err = tx.Exec(ctx, `UPDATE documents SET status = 'ready', error = NULL, page_count = $2, processed_at = now() WHERE id = $1`,
			id, doc.PageCount)
		return err
	})
}

func (s *Documents) Fail(ctx context.Context, id uuid.UUID, msg string) error {
	_, err := s.pool.Exec(ctx, `UPDATE documents SET status = 'failed', error = $2, processed_at = now() WHERE id = $1`, id, msg)
	return err
}

// SetProgress records that pagesDone of pagesTotal pages are processed. Only a processing document
// is updated, so a late report can't overwrite a finished or requeued one. The notify trigger turns
// each update into an event-stream update.
func (s *Documents) SetProgress(ctx context.Context, id uuid.UUID, pagesDone, pagesTotal int) error {
	_, err := s.pool.Exec(ctx, `UPDATE documents SET pages_done = $2, pages_total = $3 WHERE id = $1 AND status = 'processing'`,
		id, pagesDone, pagesTotal)
	return err
}

// Requeue puts a claimed document back in the queue, e.g. when the pipeline was busy.
func (s *Documents) Requeue(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE documents SET status = 'queued', pages_done = NULL, pages_total = NULL
		WHERE id = $1 AND status = 'processing'`, id)
	return err
}

// RequeueStale requeues documents left processing by a previous run that stopped mid-job.
// Call it once at startup; it assumes a single API instance runs the worker.
func (s *Documents) RequeueStale(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE documents SET status = 'queued', pages_done = NULL, pages_total = NULL WHERE status = 'processing'`)
	return tag.RowsAffected(), err
}
