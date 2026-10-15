package worker

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/acgtubio/hermes/internal/database"
	"github.com/acgtubio/hermes/internal/pipeline"
	"github.com/acgtubio/hermes/internal/soa"
	"github.com/acgtubio/hermes/internal/trace"
)

type fakeQueue struct {
	completed, failed, requeued []uuid.UUID
	failMsg                     string
	progress                    []pipeline.Progress
}

func (q *fakeQueue) SetProgress(_ context.Context, _ uuid.UUID, done, total int) error {
	q.progress = append(q.progress, pipeline.Progress{PagesDone: done, PageCount: total})
	return nil
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

// fakeProc records the trace ID the worker processed the document under.
type fakeProc struct {
	err     error
	traceID *string
}

func (p fakeProc) Process(ctx context.Context, url string, onProgress func(pipeline.Progress)) (soa.Document, error) {
	if url != "https://objects/documents/x.pdf?signed" {
		return soa.Document{}, fmt.Errorf("unexpected url %q", url)
	}
	if p.traceID != nil {
		*p.traceID = trace.FromContext(ctx)
	}
	onProgress(pipeline.Progress{PagesDone: 0, PageCount: 2})
	onProgress(pipeline.Progress{PagesDone: 2, PageCount: 2})
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
			w := New(q, fakeProc{err: tt.err}, fakePresigner{}, zap.NewNop())
			retry := w.process(context.Background(), doc)
			if retry != tt.retry || len(q.completed) != tt.completed || len(q.failed) != tt.failed || len(q.requeued) != tt.requeue {
				t.Errorf("retry %v, completed %d, failed %d, requeued %d; want %v, %d, %d, %d",
					retry, len(q.completed), len(q.failed), len(q.requeued), tt.retry, tt.completed, tt.failed, tt.requeue)
			}
			if want := []pipeline.Progress{{PagesDone: 0, PageCount: 2}, {PagesDone: 2, PageCount: 2}}; !reflect.DeepEqual(q.progress, want) {
				t.Errorf("progress recorded %v, want %v", q.progress, want)
			}
			if tt.failed > 0 && q.failMsg != tt.err.Error() {
				t.Errorf("fail message %q, want %q", q.failMsg, tt.err)
			}
		})
	}
}

func TestProcessUsesUploadTraceID(t *testing.T) {
	for _, upload := range []string{"4bf92f3577b34da6a3ce929d0e0e4736", ""} {
		var got string
		w := New(&fakeQueue{}, fakeProc{traceID: &got}, fakePresigner{}, zap.NewNop())
		w.process(context.Background(), database.Document{ID: uuid.New(), ObjectKey: "documents/x.pdf", TraceID: upload})
		switch {
		case upload != "" && got != upload:
			t.Errorf("processed under trace %q, want the upload's %q", got, upload)
		case upload == "" && len(got) != 32:
			t.Errorf("document without a trace ID processed under %q, want a generated one", got)
		}
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := New(&fakeQueue{}, fakeProc{}, fakePresigner{}, zap.NewNop())
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
