package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/acgtubio/hermes/internal/database"
	"github.com/acgtubio/hermes/internal/pipeline"
	"github.com/acgtubio/hermes/internal/server"
	"github.com/acgtubio/hermes/internal/storage"
	"github.com/acgtubio/hermes/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

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
		log.Info("requeued documents left processing by the last run", "count", n)
	}
	w := worker.New(docs, pipeline.New(cfg.PipelineURL, cfg.PipelineTimeout), objects, log)
	workerDone := make(chan struct{})
	go func() { w.Run(ctx); close(workerDone) }()
	defer func() { stop(); <-workerDone }() // let the worker finish its DB calls before the pool closes

	router, err := server.NewRouter(server.New(server.Deps{
		DB: pool, Docs: docs, Objects: objects, Wake: w.Wake, MaxUpload: cfg.MaxUploadMB << 20, Log: log,
	}))
	if err != nil {
		return err
	}
	srv := server.NewHTTPServer(cfg.Port, router)

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr)
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
