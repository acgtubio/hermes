package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/acgtubio/hermes/internal/database"
	"github.com/acgtubio/hermes/internal/pipeline"
	"github.com/acgtubio/hermes/internal/soa"
)

type fakeQueue struct {
	completed, failed, requeued []uuid.UUID
	failMsg                     string
}

func (q *fakeQueue) ClaimNext(context.Context) (database.Document, bool, error) {
	return database.Document{}, false, nil
}
func (q *fakeQueue) Complete(_ context.Context, id uuid.UUID, _ soa.Document) error {
	q.completed = append(q.completed, id)
	return nil
}
func (q *fakeQueue) Fail(_ context.Context, id uuid.UUID, msg string) error {
	q.failed, q.failMsg = append(q.failed, id), msg
	return nil
}
func (q *fakeQueue) Requeue(_ context.Context, id uuid.UUID) error {
	q.requeued = append(q.requeued, id)
	return nil
}

type fakeProc struct{ err error }

func (p fakeProc) Process(_ context.Context, url string) (soa.Document, error) {
	if url != "https://objects/documents/x.pdf?signed" {
		return soa.Document{}, fmt.Errorf("unexpected url %q", url)
	}
	return soa.Document{PageCount: 1}, p.err
}

type fakePresigner struct{}

func (fakePresigner) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://objects/" + key + "?signed", nil
}

func TestProcess(t *testing.T) {
	doc := database.Document{ID: uuid.New(), ObjectKey: "documents/x.pdf"}
	tests := []struct {
		name                       string
		err                        error
		completed, failed, requeue int
		retry                      bool
	}{
		{"success", nil, 1, 0, 0, false},
		{"pipeline error", errors.New("pipeline: broken pdf (HTTP 422)"), 0, 1, 0, false},
		{"sidecar busy", fmt.Errorf("%w: sidecar busy", pipeline.ErrRetryable), 0, 0, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &fakeQueue{}
			w := New(q, fakeProc{tt.err}, fakePresigner{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			retry := w.process(context.Background(), doc)
			if retry != tt.retry || len(q.completed) != tt.completed || len(q.failed) != tt.failed || len(q.requeued) != tt.requeue {
				t.Errorf("retry %v, completed %d, failed %d, requeued %d; want %v, %d, %d, %d",
					retry, len(q.completed), len(q.failed), len(q.requeued), tt.retry, tt.completed, tt.failed, tt.requeue)
			}
			if tt.failed > 0 && q.failMsg != tt.err.Error() {
				t.Errorf("fail message %q, want %q", q.failMsg, tt.err)
			}
		})
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := New(&fakeQueue{}, fakeProc{}, fakePresigner{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	w.Wake()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run didn't return after cancel")
	}
}
