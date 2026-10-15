package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

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
