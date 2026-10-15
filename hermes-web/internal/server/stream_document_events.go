package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/acgtubio/hermes/internal/api"
	"github.com/acgtubio/hermes/internal/database"
	"github.com/acgtubio/hermes/internal/logging"
)

// keepAlive is how often an idle stream sends a comment, so proxies don't drop it.
var keepAlive = 20 * time.Second

// StreamDocumentEvents sends the document's state now and after every status change, until it's
// ready or failed. It subscribes before the first read, so no change can slip in between.
func (s *Server) StreamDocumentEvents(ctx context.Context, req api.StreamDocumentEventsRequestObject) (api.StreamDocumentEventsResponseObject, error) {
	wake, unsubscribe := s.Events.Subscribe(req.Id)
	doc, err := s.Docs.Get(ctx, req.Id)
	if err != nil {
		unsubscribe()
		if errors.Is(err, database.ErrNotFound) {
			return api.StreamDocumentEvents404JSONResponse{ErrorJSONResponse: api.ErrorJSONResponse{Message: "document not found"}}, nil
		}
		return nil, err
	}
	return &documentEventStream{s: s, ctx: ctx, id: req.Id, first: doc, wake: wake, unsubscribe: unsubscribe}, nil
}

// documentEventStream implements the generated response interface itself: it has to clear the
// connection's deadlines and set SSE headers, which the generated text/event-stream type can't.
type documentEventStream struct {
	s           *Server
	ctx         context.Context
	id          uuid.UUID
	first       database.Document
	wake        <-chan struct{}
	unsubscribe func()
}

func (e *documentEventStream) VisitStreamDocumentEventsResponse(w http.ResponseWriter) error {
	defer e.unsubscribe()
	log := logging.Ctx(e.ctx, e.s.Log).With(zap.Stringer("document_id", e.id))

	// The server's WriteTimeout would cut the stream, and its ReadTimeout would end the request
	// (the connection's background read hits the deadline and cancels the context).
	rc := http.NewResponseController(w)
	if err := errors.Join(rc.SetWriteDeadline(time.Time{}), rc.SetReadDeadline(time.Time{})); err != nil {
		log.Warn("can't lift deadlines; the server's timeouts will cut this stream", zap.Error(err))
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // nginx: don't buffer the stream
	w.WriteHeader(http.StatusOK)

	send := func(format string, args ...any) error {
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			return err
		}
		return rc.Flush()
	}
	var last []byte
	sendDoc := func(d database.Document) (done bool, err error) {
		b, err := json.Marshal(toAPI(d))
		if err != nil {
			return false, err
		}
		done = d.Status == database.StatusReady || d.Status == database.StatusFailed
		if !bytes.Equal(b, last) {
			if err := send("event: document\ndata: %s\n\n", b); err != nil {
				return false, err
			}
			last = b
		}
		if done {
			return true, send("event: end\ndata: {}\n\n")
		}
		return false, nil
	}

	log.Debug("event stream opened")
	defer log.Debug("event stream closed")
	if err := send("retry: 3000\n\n"); err != nil {
		return nil // the client is gone; nothing to report
	}
	if done, err := sendDoc(e.first); done || err != nil {
		return nil
	}
	ping := time.NewTicker(keepAlive)
	defer ping.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return nil
		case <-e.s.streams.Done(): // shutting down; the client reconnects to another instance
			return nil
		case <-ping.C:
			if send(": ping\n\n") != nil {
				return nil
			}
		case <-e.wake:
			d, err := e.s.Docs.Get(e.ctx, e.id)
			if err != nil {
				if e.ctx.Err() == nil {
					log.Error("read document for event stream", zap.Error(err))
				}
				return nil // the client reconnects and starts again from the current state
			}
			if done, err := sendDoc(d); done || err != nil {
				return nil
			}
		}
	}
}
