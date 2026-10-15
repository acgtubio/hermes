// Package logging builds the service's zap logger and attaches request-scoped fields to it.
package logging

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/acgtubio/hermes/internal/trace"
)

// New returns a JSON logger on stdout at level, with ISO 8601 timestamps under "time".
func New(level zapcore.Level) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)
	cfg.OutputPaths = []string{"stdout"}
	cfg.EncoderConfig.TimeKey = "time"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	return cfg.Build()
}

// Ctx returns log with the context's trace ID attached. Use it for every log call that has a
// context, so the line can be tied back to the request (or document) that caused it.
func Ctx(ctx context.Context, log *zap.Logger) *zap.Logger {
	if id := trace.FromContext(ctx); id != "" {
		return log.With(zap.String("trace_id", id))
	}
	return log
}
