package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/acgtubio/hermes/internal/database"
	"github.com/acgtubio/hermes/internal/events"
	"github.com/acgtubio/hermes/internal/logging"
	"github.com/acgtubio/hermes/internal/pipeline"
	"github.com/acgtubio/hermes/internal/server"
	"github.com/acgtubio/hermes/internal/storage"
	"github.com/acgtubio/hermes/internal/worker"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

// run logs its own fatal error: through zap once the logger exists, to stderr before that.
func run() (err error) {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return err
	}
	log, err := logging.New(cfg.LogLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal: build logger:", err)
		return err
	}
	defer func() {
		if err != nil {
			log.Error("fatal", zap.Error(err))
		}
		_ = log.Sync()
	}()
	zap.ReplaceGlobals(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		return err
	}
	objects, err := storage.New(ctx, cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UseSSL)
	if err != nil {
		return err
	}
	docs := database.NewDocuments(pool)

	if n, err := docs.RequeueStale(ctx); err != nil {
		return err
	} else if n > 0 {
		log.Info("requeued documents left processing by the last run", zap.Int64("count", n))
	}
	w := worker.New(docs, pipeline.New(cfg.PipelineURL, cfg.PipelineTimeout), objects, log)
	workerDone := make(chan struct{})
	go func() { w.Run(ctx); close(workerDone) }()
	defer func() { stop(); <-workerDone }() // let the worker finish its DB calls before the pool closes

	// Status changes reach SSE streams via Postgres NOTIFY (the documents_notify trigger), so they
	// arrive whichever instance or process made the change.
	hub := events.NewHub()
	listenDone := make(chan struct{})
	go func() {
		database.Listen(ctx, pool, database.DocumentEvents, func(payload string) {
			if id, err := uuid.Parse(payload); err == nil {
				hub.Publish(id)
			}
		}, hub.PublishAll, log)
		close(listenDone)
	}()
	defer func() { stop(); <-listenDone }()

	handlers := server.New(server.Deps{
		DB: pool, Docs: docs, Objects: objects, Events: hub, Wake: w.Wake, MaxUpload: cfg.MaxUploadMB << 20, Log: log,
	})
	router, err := server.NewRouter(handlers)
	if err != nil {
		return err
	}
	srv := server.NewHTTPServer(cfg.Port, router)
	srv.RegisterOnShutdown(handlers.StopStreams)

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Info("stopped")
	return nil
}
