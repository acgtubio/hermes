// Package events wakes the SSE streams watching a document when it changes. A wake carries no
// data: the stream re-reads the document, so it always sends committed, current state and a burst
// of changes coalesces into one read.
package events

import (
	"sync"

	"github.com/google/uuid"
)

type Hub struct {
	mu   sync.Mutex
	subs map[uuid.UUID]map[chan struct{}]struct{}
}

func NewHub() *Hub { return &Hub{subs: map[uuid.UUID]map[chan struct{}]struct{}{}} }

// Subscribe returns a channel that receives a value whenever document id may have changed, and a
// function that unsubscribes. Subscribe before reading the document, so no change is missed.
func (h *Hub) Subscribe(id uuid.UUID) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[id] == nil {
		h.subs[id] = map[chan struct{}]struct{}{}
	}
	h.subs[id][ch] = struct{}{}
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subs[id], ch)
		if len(h.subs[id]) == 0 {
			delete(h.subs, id)
		}
	}
}

// Publish wakes every subscriber of id. It never blocks: a subscriber that hasn't consumed its
// last wake simply stays woken.
func (h *Hub) Publish(id uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[id] {
		wake(ch)
	}
}

// PublishAll wakes every subscriber, e.g. after notifications may have been missed.
func (h *Hub) PublishAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, chs := range h.subs {
		for ch := range chs {
			wake(ch)
		}
	}
}

// Subscribers is the number of open subscriptions, for tests and diagnostics.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, chs := range h.subs {
		n += len(chs)
	}
	return n
}

func wake(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
