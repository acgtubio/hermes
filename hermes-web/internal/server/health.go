package server

import (
	"context"

	"github.com/acgtubio/hermes/internal/api"
)

func (s *Server) GetHealth(ctx context.Context, _ api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	if err := s.DB.Ping(ctx); err != nil {
		s.Log.WarnContext(ctx, "health check failed", "err", err)
		msg := "database unreachable"
		return api.GetHealth503JSONResponse{Status: api.HealthStatusUnavailable, Error: &msg}, nil
	}
	return api.GetHealth200JSONResponse{Status: api.HealthStatusOk}, nil
}
