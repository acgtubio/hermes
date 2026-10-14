package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/acgtubio/hermes/internal/api"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func newTestRouter(t *testing.T, pingErr error) http.Handler {
	t.Helper()
	h, err := NewRouter(New(fakePinger{pingErr}, slog.New(slog.NewTextHandler(io.Discard, nil))))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestGetHealth(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantCode   int
		wantStatus api.HealthStatus
	}{
		{"ok", nil, http.StatusOK, api.HealthStatusOk},
		{"db down", errors.New("connection refused"), http.StatusServiceUnavailable, api.HealthStatusUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newTestRouter(t, tt.pingErr).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

			if rec.Code != tt.wantCode {
				t.Fatalf("status code = %d, want %d", rec.Code, tt.wantCode)
			}
			var body api.Health
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", body.Status, tt.wantStatus)
			}
		})
	}
}

func TestUnknownRouteIsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want 404", rec.Code)
	}
}
