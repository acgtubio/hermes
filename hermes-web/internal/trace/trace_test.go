package trace

import (
	"context"
	"testing"
)

func TestFromTraceparent(t *testing.T) {
	tests := []struct {
		header string
		want   string
		ok     bool
	}{
		{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "4bf92f3577b34da6a3ce929d0e0e4736", true},
		{" 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00 ", "4bf92f3577b34da6a3ce929d0e0e4736", true},
		{"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "", false}, // unknown version
		{"00-4bf92f3577b34da6a3ce929d0e0e473-00f067aa0ba902b7-01", "", false},  // short trace id
		{"00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01", "", false}, // uppercase isn't allowed
		{"00-4bf92f3577b34da6a3ce929d0e0e47zz-00f067aa0ba902b7-01", "", false}, // not hex
		{"00-00000000000000000000000000000000-00f067aa0ba902b7-01", "", false}, // all-zero trace id
		{"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", "", false}, // all-zero parent id
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := FromTraceparent(tt.header)
		if got != tt.want || ok != tt.ok {
			t.Errorf("FromTraceparent(%q) = %q, %v; want %q, %v", tt.header, got, ok, tt.want, tt.ok)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	id := New()
	if got, ok := FromTraceparent(Traceparent(id)); !ok || got != id {
		t.Errorf("Traceparent(%q) parsed back as %q, %v", id, got, ok)
	}
	if New() == id {
		t.Error("New returned the same ID twice")
	}
	if got := FromContext(WithID(context.Background(), id)); got != id {
		t.Errorf("FromContext = %q, want %q", got, id)
	}
	if got := FromContext(context.Background()); got != "" {
		t.Errorf("FromContext on an empty context = %q, want empty", got)
	}
}
