package server

import (
	"context"
	"log/slog"

	"github.com/acgtubio/hermes/internal/api"
)

// Pinger is the subset of *pgxpool.Pool the health check needs.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Server implements api.StrictServerInterface.
type Server struct {
	db  Pinger
	log *slog.Logger
}

var _ api.StrictServerInterface = (*Server)(nil)

func New(db Pinger, log *slog.Logger) *Server {
	return &Server{db: db, log: log}
}

func (s *Server) GetHealth(ctx context.Context, _ api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	if err := s.db.Ping(ctx); err != nil {
		s.log.WarnContext(ctx, "health check failed", "err", err)
		msg := "database unreachable"
		return api.GetHealth503JSONResponse{Status: api.HealthStatusUnavailable, Error: &msg}, nil
	}
	return api.GetHealth200JSONResponse{Status: api.HealthStatusOk}, nil
}
