package server

import (
	"context"
	"io"
	"log/slog"

	"github.com/google/uuid"

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
	Wake      func() // called after a document is queued, e.g. (*worker.Worker).Wake
	MaxUpload int64  // bytes
	Log       *slog.Logger
}

// Server implements api.StrictServerInterface.
type Server struct {
	Deps
}

var _ api.StrictServerInterface = (*Server)(nil)

func New(d Deps) *Server {
	if d.Wake == nil {
		d.Wake = func() {}
	}
	return &Server{Deps: d}
}

func toAPI(d database.Document) api.Document {
	return api.Document{
		Id: d.ID, Filename: d.Filename, Status: api.DocumentStatus(d.Status), Error: d.Error,
		PageCount: d.PageCount, RowCount: d.RowCount, NeedsReviewCount: d.NeedsReviewCount,
		CreatedAt: d.CreatedAt, ProcessedAt: d.ProcessedAt,
	}
}
