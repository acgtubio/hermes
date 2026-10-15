package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/acgtubio/hermes/internal/database"
	"github.com/acgtubio/hermes/internal/events"
)

// Shared test helpers and fakes. Each handler's tests live next to it, in <operation>_test.go.

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

type fakeDocs struct {
	mu      sync.Mutex
	byID    map[uuid.UUID]database.Document
	bySum   map[string]uuid.UUID
	created []database.NewDocument
}

func newFakeDocs() *fakeDocs {
	return &fakeDocs{byID: map[uuid.UUID]database.Document{}, bySum: map[string]uuid.UUID{}}
}

func (f *fakeDocs) Create(_ context.Context, n database.NewDocument) (database.Document, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id, ok := f.bySum[n.SHA256]; ok {
		return database.Document{}, &database.DuplicateError{ID: id}
	}
	d := database.Document{ID: n.ID, Filename: n.Filename, ObjectKey: n.ObjectKey, Status: database.StatusQueued,
		CreatedAt: time.Now(), TraceID: n.TraceID}
	f.byID[n.ID], f.bySum[n.SHA256] = d, n.ID
	f.created = append(f.created, n)
	return d, nil
}

// update changes a stored document, as the worker would.
func (f *fakeDocs) update(id uuid.UUID, change func(*database.Document)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d := f.byID[id]
	change(&d)
	f.byID[id] = d
}

func (f *fakeDocs) Get(_ context.Context, id uuid.UUID) (database.Document, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.byID[id]
	if !ok {
		return database.Document{}, database.ErrNotFound
	}
	return d, nil
}

type fakeObjects struct {
	mu   sync.Mutex
	objs map[string][]byte
}

func newFakeObjects() *fakeObjects { return &fakeObjects{objs: map[string][]byte{}} }

func (f *fakeObjects) Put(_ context.Context, key, _ string, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objs[key] = b
	return nil
}

func (f *fakeObjects) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objs, key)
	return nil
}

func newTestRouter(t *testing.T, pingErr error) http.Handler {
	t.Helper()
	return newRouterWith(t, Deps{DB: fakePinger{pingErr}})
}

// newRouterWith fills in an empty store and fakes for deps left nil, and a discarding logger.
func newRouterWith(t *testing.T, d Deps) http.Handler {
	t.Helper()
	h, _ := newObservedRouter(t, d)
	return h
}

// newObservedRouter is newRouterWith, also returning the log entries the router writes.
func newObservedRouter(t *testing.T, d Deps) (http.Handler, *observer.ObservedLogs) {
	t.Helper()
	if d.DB == nil {
		d.DB = fakePinger{}
	}
	if d.Docs == nil {
		d.Docs = newFakeDocs()
	}
	if d.Objects == nil {
		d.Objects = newFakeObjects()
	}
	if d.Events == nil {
		d.Events = events.NewHub()
	}
	if d.MaxUpload == 0 {
		d.MaxUpload = 1 << 20
	}
	core, logs := observer.New(zap.DebugLevel)
	d.Log = zap.New(core)
	h, err := NewRouter(New(d))
	if err != nil {
		t.Fatal(err)
	}
	return h, logs
}

func testLogger() *zap.Logger { return zap.NewNop() }

func TestUnknownRouteIsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want 404", rec.Code)
	}
}
