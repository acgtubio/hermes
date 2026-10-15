package events

import (
	"testing"

	"github.com/google/uuid"
)

func woken(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestHub(t *testing.T) {
	h := NewHub()
	a, b := uuid.New(), uuid.New()
	a1, unsubA1 := h.Subscribe(a)
	a2, _ := h.Subscribe(a)
	b1, _ := h.Subscribe(b)

	h.Publish(a)
	h.Publish(a) // coalesces with the first: one wake
	if !woken(a1) || !woken(a2) {
		t.Error("subscribers of a weren't woken")
	}
	if woken(a1) {
		t.Error("two publishes woke a1 twice, want them coalesced")
	}
	if woken(b1) {
		t.Error("publishing a woke b's subscriber")
	}

	unsubA1()
	h.Publish(a)
	if woken(a1) {
		t.Error("unsubscribed channel was woken")
	}
	if !woken(a2) {
		t.Error("remaining subscriber of a wasn't woken")
	}

	h.PublishAll()
	if !woken(a2) || !woken(b1) {
		t.Error("PublishAll didn't wake every subscriber")
	}
	if n := h.Subscribers(); n != 2 {
		t.Errorf("Subscribers() = %d, want 2", n)
	}
}
