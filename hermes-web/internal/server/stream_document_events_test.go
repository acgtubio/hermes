package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/acgtubio/hermes/internal/api"
	"github.com/acgtubio/hermes/internal/database"
	"github.com/acgtubio/hermes/internal/events"
)

// sseReader reads a text/event-stream one event (or comment) at a time.
type sseReader struct {
	t *testing.T
	r *bufio.Reader
}

type sseEvent struct {
	name, data, comment string
}

// next returns the next event or comment, failing the test if none arrives within a second.
func (s sseReader) next() sseEvent {
	s.t.Helper()
	got := make(chan sseEvent, 1)
	go func() {
		var e sseEvent
		for {
			line, err := s.r.ReadString('\n')
			if err != nil {
				got <- sseEvent{name: "EOF"}
				return
			}
			line = strings.TrimSuffix(line, "\n")
			switch {
			case line == "" && (e.name != "" || e.data != "" || e.comment != ""):
				got <- e
				return
			case strings.HasPrefix(line, "event: "):
				e.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				e.data = strings.TrimPrefix(line, "data: ")
			case strings.HasPrefix(line, ": "):
				e.comment = strings.TrimPrefix(line, ": ")
			case strings.HasPrefix(line, "retry: "):
				e.name = "retry"
			}
		}
	}()
	select {
	case e := <-got:
		return e
	case <-time.After(time.Second):
		s.t.Fatal("no event within 1s")
		return sseEvent{}
	}
}

func (s sseReader) nextDocument() api.Document {
	s.t.Helper()
	e := s.next()
	if e.name != "document" {
		s.t.Fatalf("got event %+v, want a document event", e)
	}
	var d api.Document
	if err := json.Unmarshal([]byte(e.data), &d); err != nil {
		s.t.Fatalf("document event data %q: %v", e.data, err)
	}
	return d
}

func openStream(t *testing.T, srv *httptest.Server, id string) (*http.Response, sseReader) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/documents/"+id+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp, sseReader{t, bufio.NewReader(resp.Body)}
}

func TestStreamDocumentEvents(t *testing.T) {
	docs, hub := newFakeDocs(), events.NewHub()
	d, _ := docs.Create(context.Background(), database.NewDocument{ID: uuid.New(), Filename: "a.pdf", SHA256: "x"})
	srv := httptest.NewServer(newRouterWith(t, Deps{Docs: docs, Events: hub}))
	defer srv.Close()

	resp, events := openStream(t, srv, d.ID.String())
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d, content type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if e := events.next(); e.name != "retry" {
		t.Fatalf("first event %+v, want retry", e)
	}
	if got := events.nextDocument(); got.Status != api.DocumentStatusQueued || got.Id != d.ID {
		t.Fatalf("snapshot = %+v, want the queued document", got)
	}

	docs.update(d.ID, func(d *database.Document) { d.Status = database.StatusProcessing })
	hub.Publish(d.ID)
	hub.Publish(d.ID) // no change since: must not send a duplicate
	if got := events.nextDocument(); got.Status != api.DocumentStatusProcessing {
		t.Fatalf("after processing: %+v", got)
	}

	for _, done := range []int{0, 1, 2} { // the worker recording pipeline progress
		done, total := done, 2
		docs.update(d.ID, func(d *database.Document) { d.PagesDone, d.PagesTotal = &done, &total })
		hub.Publish(d.ID)
		got := events.nextDocument()
		if got.PagesDone == nil || *got.PagesDone != done || got.PagesTotal == nil || *got.PagesTotal != 2 {
			t.Fatalf("progress event = %+v, want %d of 2 pages", got, done)
		}
	}

	rows, review := 423, 8
	docs.update(d.ID, func(d *database.Document) {
		d.Status, d.RowCount, d.NeedsReviewCount = database.StatusReady, &rows, &review
	})
	hub.Publish(d.ID)
	got := events.nextDocument()
	if got.Status != api.DocumentStatusReady || got.RowCount == nil || *got.RowCount != 423 {
		t.Fatalf("after ready: %+v", got)
	}
	if e := events.next(); e.name != "end" {
		t.Fatalf("after ready: event %+v, want end", e)
	}
	if e := events.next(); e.name != "EOF" {
		t.Fatalf("stream still open after end: %+v", e)
	}
	waitFor(t, func() bool { return hub.Subscribers() == 0 }, "stream to unsubscribe")
}

func TestStreamDocumentEventsAlreadyDone(t *testing.T) {
	docs := newFakeDocs()
	d, _ := docs.Create(context.Background(), database.NewDocument{ID: uuid.New(), Filename: "a.pdf", SHA256: "x"})
	docs.update(d.ID, func(d *database.Document) { d.Status = database.StatusFailed })
	srv := httptest.NewServer(newRouterWith(t, Deps{Docs: docs}))
	defer srv.Close()

	_, events := openStream(t, srv, d.ID.String())
	events.next() // retry
	if got := events.nextDocument(); got.Status != api.DocumentStatusFailed {
		t.Fatalf("snapshot = %+v", got)
	}
	if e := events.next(); e.name != "end" {
		t.Fatalf("event %+v, want end straight after a terminal snapshot", e)
	}
}

func TestStreamDocumentEventsKeepAliveAndDisconnect(t *testing.T) {
	defer func(d time.Duration) { keepAlive = d }(keepAlive)
	keepAlive = 50 * time.Millisecond
	docs, hub := newFakeDocs(), events.NewHub()
	d, _ := docs.Create(context.Background(), database.NewDocument{ID: uuid.New(), Filename: "a.pdf", SHA256: "x"})
	srv := httptest.NewServer(newRouterWith(t, Deps{Docs: docs, Events: hub}))
	defer srv.Close()

	resp, events := openStream(t, srv, d.ID.String())
	events.next() // retry
	events.nextDocument()
	if e := events.next(); e.comment != "ping" {
		t.Fatalf("idle stream sent %+v, want a ping", e)
	}
	resp.Body.Close()
	waitFor(t, func() bool { return hub.Subscribers() == 0 }, "disconnected stream to unsubscribe")
}

func TestStreamDocumentEventsStopStreams(t *testing.T) {
	docs := newFakeDocs()
	d, _ := docs.Create(context.Background(), database.NewDocument{ID: uuid.New(), Filename: "a.pdf", SHA256: "x"})
	s := New(Deps{DB: fakePinger{}, Docs: docs, Events: events.NewHub(), Objects: newFakeObjects(), Log: testLogger()})
	h, err := NewRouter(s)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	_, events := openStream(t, srv, d.ID.String())
	events.next() // retry
	events.nextDocument()
	s.StopStreams()
	if e := events.next(); e.name != "EOF" {
		t.Fatalf("stream still open after StopStreams: %+v", e)
	}
}

func TestStreamDocumentEventsErrors(t *testing.T) {
	hub := events.NewHub()
	srv := httptest.NewServer(newRouterWith(t, Deps{Events: hub}))
	defer srv.Close()
	for _, tt := range []struct {
		id   string
		want int
	}{
		{uuid.NewString(), http.StatusNotFound},
		{"not-a-uuid", http.StatusBadRequest},
	} {
		resp, err := http.Get(srv.URL + "/api/documents/" + tt.id + "/events")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tt.want || resp.Header.Get("Content-Type") == "text/event-stream" {
			t.Errorf("GET events for %s = %d %q (%s), want %d and no stream", tt.id, resp.StatusCode, resp.Header.Get("Content-Type"), body, tt.want)
		}
	}
	if n := hub.Subscribers(); n != 0 {
		t.Errorf("%d subscriptions left after rejected streams", n)
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}
