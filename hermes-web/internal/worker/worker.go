// Package worker processes queued documents one at a time: it claims the oldest queued document,
// has the pipeline sidecar parse it, and stores the result. One at a time because the pipeline
// already uses every core.
package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/acgtubio/hermes/internal/database"
	"github.com/acgtubio/hermes/internal/pipeline"
	"github.com/acgtubio/hermes/internal/soa"
)

// Queue is the subset of *database.Documents the worker needs.
type Queue interface {
	ClaimNext(ctx context.Context) (database.Document, bool, error)
	Complete(ctx context.Context, id uuid.UUID, doc soa.Document) error
	Fail(ctx context.Context, id uuid.UUID, msg string) error
	Requeue(ctx context.Context, id uuid.UUID) error
}

type Processor interface {
	Process(ctx context.Context, pdfURL string) (soa.Document, error)
}

type Presigner interface {
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

const (
	idlePoll = 5 * time.Second  // queue check when nothing woke the worker
	backoff  = 10 * time.Second // after a retryable failure (sidecar busy or down)
	urlTTL   = 15 * time.Minute // the sidecar fetches the PDF right away; this only needs to cover a slow start
)

type Worker struct {
	queue   Queue
	proc    Processor
	objects Presigner
	log     *slog.Logger
	wake    chan struct{}
}

func New(q Queue, p Processor, objects Presigner, log *slog.Logger) *Worker {
	return &Worker{queue: q, proc: p, objects: objects, log: log, wake: make(chan struct{}, 1)}
}

// Wake tells the worker a document was queued, so it doesn't wait for the next poll.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run processes documents until ctx is cancelled. A document in flight at shutdown stays
// processing and is requeued by database.RequeueStale on the next start.
func (w *Worker) Run(ctx context.Context) {
	for ctx.Err() == nil {
		doc, ok, err := w.queue.ClaimNext(ctx)
		switch {
		case err != nil:
			if ctx.Err() == nil {
				w.log.ErrorContext(ctx, "claim document", "err", err)
			}
			w.sleep(ctx, backoff, false)
		case !ok:
			w.sleep(ctx, idlePoll, true)
		case w.process(ctx, doc):
			w.sleep(ctx, backoff, false)
		}
	}
}

// process handles one claimed document and reports whether it was requeued for a retry.
func (w *Worker) process(ctx context.Context, doc database.Document) (retry bool) {
	log := w.log.With("document_id", doc.ID)
	log.InfoContext(ctx, "processing document", "filename", doc.Filename)
	start := time.Now()
	url, err := w.objects.PresignGet(ctx, doc.ObjectKey, urlTTL)
	var result soa.Document
	if err == nil {
		result, err = w.proc.Process(ctx, url)
	}
	if ctx.Err() != nil {
		return false
	}
	switch {
	case errors.Is(err, pipeline.ErrRetryable):
		log.WarnContext(ctx, "pipeline unavailable, requeueing", "err", err)
		if err := w.queue.Requeue(ctx, doc.ID); err != nil {
			log.ErrorContext(ctx, "requeue document", "err", err)
		}
		return true
	case err == nil:
		err = w.queue.Complete(ctx, doc.ID, result)
		if err == nil {
			log.InfoContext(ctx, "document ready", "rows", len(result.Rows), "duration", time.Since(start))
			return false
		}
	}
	log.ErrorContext(ctx, "document failed", "err", err)
	if err := w.queue.Fail(ctx, doc.ID, err.Error()); err != nil {
		log.ErrorContext(ctx, "mark document failed", "err", err)
	}
	return false
}

func (w *Worker) sleep(ctx context.Context, d time.Duration, wakeable bool) {
	t := time.NewTimer(d)
	defer t.Stop()
	var wake <-chan struct{}
	if wakeable {
		wake = w.wake
	}
	select {
	case <-ctx.Done():
	case <-t.C:
	case <-wake:
	}
}
