package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
	"go.uber.org/zap"

	"github.com/acgtubio/hermes/internal/api"
	"github.com/acgtubio/hermes/internal/logging"
	"github.com/acgtubio/hermes/internal/trace"
)

const apiPrefix = "/api"

// NewRouter mounts the generated OpenAPI handlers under /api, with request
// validation against the embedded spec.
func NewRouter(s *Server) (http.Handler, error) {
	spec, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load openapi spec: %w", err)
	}
	// Routing is done by chi under apiPrefix; the validator strips that prefix
	// instead of matching against the spec's servers list.
	spec.Servers = nil

	r := chi.NewRouter()
	r.Use(traceIDs, middleware.RealIP, requestLogger(s.Log), middleware.Recoverer)

	r.Route(apiPrefix, func(r chi.Router) {
		r.Get("/openapi.json", serveSpec)
		r.Group(func(r chi.Router) {
			validator := func(skipBody bool) func(http.Handler) http.Handler {
				return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
					Options:              openapi3filter.Options{ExcludeRequestBody: skipBody},
					Prefix:               apiPrefix,
					DoNotValidateServers: true,
				})
			}
			r.Use(selectMiddleware(uploadRoute, validator(true), validator(false)))
			api.HandlerFromMux(api.NewStrictHandler(s, nil), r)
		})
	})
	return r, nil
}

// selectMiddleware applies a to requests matching pick and b to the rest.
func selectMiddleware(pick func(*http.Request) bool, a, b func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		ha, hb := a(next), b(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pick(r) {
				ha.ServeHTTP(w, r)
			} else {
				hb.ServeHTTP(w, r)
			}
		})
	}
}

// NewHTTPServer wraps the router in an http.Server with conservative timeouts. ReadTimeout covers
// the whole request body, so it allows a large PDF upload over a slow link.
func NewHTTPServer(port int, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func serveSpec(w http.ResponseWriter, _ *http.Request) {
	b, err := api.GetSpecJSON()
	if err != nil {
		http.Error(w, "spec unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

// TraceIDHeader returns the request's trace ID to the caller, so a failure can be found in the logs.
const TraceIDHeader = "X-Trace-Id"

// traceIDs gives each request a trace ID: the caller's, from a W3C traceparent header, or a new
// one. It's put in the context (see logging.Ctx) and echoed in the X-Trace-Id response header.
func traceIDs(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := trace.FromTraceparent(r.Header.Get(trace.Header))
		if !ok {
			id = trace.New()
		}
		w.Header().Set(TraceIDHeader, id)
		next.ServeHTTP(w, r.WithContext(trace.WithID(r.Context(), id)))
	})
}

func requestLogger(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			logging.Ctx(r.Context(), log).Info("request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.Status()),
				zap.Int("bytes", ww.BytesWritten()),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}
