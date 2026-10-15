// Package trace carries a trace ID through a request and everything it causes: the HTTP
// request, the background job for an uploaded document, and the pipeline sidecar call.
// IDs use the W3C trace-context format (32 hex characters, sent in a traceparent header),
// so they stay compatible if OpenTelemetry is added later.
package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// Header is the W3C trace-context request header.
const Header = "traceparent"

const zeroID = "00000000000000000000000000000000"

// New returns a random trace ID.
func New() string { return randomHex(16) }

// FromTraceparent extracts the trace ID from a traceparent header value
// ("00-<32 hex trace id>-<16 hex parent id>-<2 hex flags>"). ok is false if it's malformed.
func FromTraceparent(h string) (id string, ok bool) {
	parts := strings.Split(strings.TrimSpace(h), "-")
	if len(parts) != 4 || parts[0] != "00" || !isHex(parts[1], 32) || !isHex(parts[2], 16) || !isHex(parts[3], 2) {
		return "", false
	}
	if parts[1] == zeroID || parts[2] == "0000000000000000" {
		return "", false
	}
	return parts[1], true
}

// Traceparent formats a header value for an outgoing call in trace id, with a fresh parent ID.
func Traceparent(id string) string { return "00-" + id + "-" + randomHex(8) + "-01" }

type ctxKey struct{}

func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the context's trace ID, or "" if it has none.
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // never fails (crypto/rand panics on an unusable source)
	return hex.EncodeToString(b)
}

func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}
