package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/acgtubio/hermes/internal/api"
	"github.com/acgtubio/hermes/internal/database"
)

func TestGetDocument(t *testing.T) {
	docs := newFakeDocs()
	d, _ := docs.Create(context.Background(), database.NewDocument{ID: uuid.New(), Filename: "a.pdf", SHA256: "x"})
	h := newRouterWith(t, Deps{Docs: docs})

	for _, tt := range []struct {
		path     string
		wantCode int
	}{
		{"/api/documents/" + d.ID.String(), http.StatusOK},
		{"/api/documents/" + uuid.NewString(), http.StatusNotFound},
		{"/api/documents/not-a-uuid", http.StatusBadRequest},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if rec.Code != tt.wantCode {
			t.Errorf("GET %s = %d, want %d", tt.path, rec.Code, tt.wantCode)
		}
	}
}

func TestGetDocumentTraceID(t *testing.T) {
	docs := newFakeDocs()
	const tid = "4bf92f3577b34da6a3ce929d0e0e4736"
	with, _ := docs.Create(context.Background(), database.NewDocument{ID: uuid.New(), Filename: "a.pdf", SHA256: "x", TraceID: tid})
	without, _ := docs.Create(context.Background(), database.NewDocument{ID: uuid.New(), Filename: "b.pdf", SHA256: "y"})
	h := newRouterWith(t, Deps{Docs: docs})

	for _, tt := range []struct {
		id   uuid.UUID
		want *string
	}{{with.ID, ptr(tid)}, {without.ID, nil}} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/documents/"+tt.id.String(), nil))
		var got api.Document
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("GET %s: %v (%s)", tt.id, err, rec.Body)
		}
		if (got.TraceId == nil) != (tt.want == nil) || (got.TraceId != nil && *got.TraceId != *tt.want) {
			t.Errorf("GET %s: trace_id = %v, want %v", tt.id, got.TraceId, tt.want)
		}
	}
}

func ptr[T any](v T) *T { return &v }
