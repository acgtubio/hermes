package server

import (
	"context"

	"go.uber.org/zap"

	"github.com/acgtubio/hermes/internal/api"
	"github.com/acgtubio/hermes/internal/logging"
)

func (s *Server) GetHealth(ctx context.Context, _ api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	if err := s.DB.Ping(ctx); err != nil {
		logging.Ctx(ctx, s.Log).Warn("health check failed", zap.Error(err))
		msg := "database unreachable"
		return api.GetHealth503JSONResponse{Status: api.HealthStatusUnavailable, Error: &msg}, nil
	}
	return api.GetHealth200JSONResponse{Status: api.HealthStatusOk}, nil
}
