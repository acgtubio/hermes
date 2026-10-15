package server

import (
	"context"
	"io"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/acgtubio/hermes/internal/api"
	"github.com/acgtubio/hermes/internal/database"
)

// Each strict-server operation is implemented in its own file, named after the operation
// (health.go, create_document.go, get_document.go). This file holds what they share.

// Pinger is the subset of *pgxpool.Pool the health check needs.
type Pinger interface {
	Ping(ctx context.Context) error
}

// DocumentStore is the subset of *database.Documents the handlers need.
type DocumentStore interface {
	Create(ctx context.Context, d database.NewDocument) (database.Document, error)
	Get(ctx context.Context, id uuid.UUID) (database.Document, error)
}

// Subscriber is the subset of *events.Hub the event stream needs.
type Subscriber interface {
	Subscribe(id uuid.UUID) (<-chan struct{}, func())
}

// ObjectStore is the subset of *storage.S3 the handlers need.
type ObjectStore interface {
	Put(ctx context.Context, key, contentType string, r io.Reader) error
	Delete(ctx context.Context, key string) error
}

// Deps are the Server's dependencies.
type Deps struct {
	DB        Pinger
	Docs      DocumentStore
	Objects   ObjectStore
	Events    Subscriber
	Wake      func() // called after a document is queued, e.g. (*worker.Worker).Wake
	MaxUpload int64  // bytes
	Log       *zap.Logger
}

// Server implements api.StrictServerInterface.
type Server struct {
	Deps
	streams     context.Context // cancelled by StopStreams
	stopStreams context.CancelFunc
}

var _ api.StrictServerInterface = (*Server)(nil)

func New(d Deps) *Server {
	if d.Wake == nil {
		d.Wake = func() {}
	}
	streams, stop := context.WithCancel(context.Background())
	return &Server{Deps: d, streams: streams, stopStreams: stop}
}

// StopStreams ends every open event stream. Register it with (*http.Server).RegisterOnShutdown:
// Shutdown waits for connections to go idle, and a stream never does.
func (s *Server) StopStreams() { s.stopStreams() }

func toAPI(d database.Document) api.Document {
	doc := api.Document{
		Id: d.ID, Filename: d.Filename, Status: api.DocumentStatus(d.Status), Error: d.Error,
		PageCount: d.PageCount, PagesDone: d.PagesDone, PagesTotal: d.PagesTotal,
		RowCount: d.RowCount, NeedsReviewCount: d.NeedsReviewCount,
		CreatedAt: d.CreatedAt, ProcessedAt: d.ProcessedAt,
	}
	if d.TraceID != "" {
		doc.TraceId = &d.TraceID
	}
	return doc
}
