package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/acgtubio/hermes/internal/database"
)

// Shared test helpers and fakes. Each handler's tests live next to it, in <operation>_test.go.

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

type fakeDocs struct {
	mu    sync.Mutex
	byID  map[uuid.UUID]database.Document
	bySum map[string]uuid.UUID
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
	d := database.Document{ID: n.ID, Filename: n.Filename, ObjectKey: n.ObjectKey, Status: database.StatusQueued, CreatedAt: time.Now()}
	f.byID[n.ID], f.bySum[n.SHA256] = d, n.ID
	return d, nil
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

// newRouterWith fills in a discarding logger and an empty store for deps left nil.
func newRouterWith(t *testing.T, d Deps) http.Handler {
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
	if d.MaxUpload == 0 {
		d.MaxUpload = 1 << 20
	}
	d.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	h, err := NewRouter(New(d))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestUnknownRouteIsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want 404", rec.Code)
	}
}
