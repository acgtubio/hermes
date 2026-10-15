package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

var traceIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestTraceIDs(t *testing.T) {
	const incoming = "4bf92f3577b34da6a3ce929d0e0e4736"
	tests := []struct {
		name, traceparent, want string
	}{
		{"caller's traceparent", "00-" + incoming + "-00f067aa0ba902b7-01", incoming},
		{"none sent", "", ""},
		{"malformed", "garbage", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, logs := newObservedRouter(t, Deps{})
			req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			if tt.traceparent != "" {
				req.Header.Set("traceparent", tt.traceparent)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			got := rec.Header().Get(TraceIDHeader)
			if tt.want != "" && got != tt.want {
				t.Errorf("%s = %q, want the caller's %q", TraceIDHeader, got, tt.want)
			}
			if !traceIDRe.MatchString(got) {
				t.Errorf("%s = %q, want 32 hex characters", TraceIDHeader, got)
			}
			entries := logs.FilterMessage("request").All()
			if len(entries) != 1 || entries[0].ContextMap()["trace_id"] != got {
				t.Errorf("request log entries %v, want one with trace_id %s", entries, got)
			}
		})
	}
}
