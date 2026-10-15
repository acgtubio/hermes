package main

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
	"go.uber.org/zap/zapcore"
)

// config is owned by this binary. Internal packages receive plain values
// (URLs, ports, clients) rather than importing this struct.
type config struct {
	Port            int           `default:"8080"`
	DatabaseURL     string        `split_words:"true" required:"true"`
	ShutdownTimeout time.Duration `split_words:"true" default:"10s"`
	LogLevel        zapcore.Level `split_words:"true" default:"info"`

	// S3-compatible object storage for uploaded PDFs (MinIO in dev).
	S3Endpoint  string `envconfig:"S3_ENDPOINT" default:"localhost:9000"`
	S3AccessKey string `envconfig:"S3_ACCESS_KEY" required:"true"`
	S3SecretKey string `envconfig:"S3_SECRET_KEY" required:"true"`
	S3Bucket    string `envconfig:"S3_BUCKET" default:"hermes-documents"`
	S3UseSSL    bool   `envconfig:"S3_USE_SSL" default:"false"`

	// The OCR pipeline sidecar (hermes-cv/server.py). It fetches PDFs from presigned URLs on
	// S3Endpoint, so that endpoint must be reachable from the sidecar under the same host name.
	PipelineURL     string        `split_words:"true" default:"http://localhost:8090"`
	PipelineTimeout time.Duration `split_words:"true" default:"10m"`

	MaxUploadMB int64 `envconfig:"MAX_UPLOAD_MB" default:"100"`
}

// loadConfig reads HERMES_PORT, HERMES_DATABASE_URL, HERMES_SHUTDOWN_TIMEOUT, HERMES_LOG_LEVEL,
// HERMES_S3_{ENDPOINT,ACCESS_KEY,SECRET_KEY,BUCKET,USE_SSL}, HERMES_PIPELINE_{URL,TIMEOUT} and
// HERMES_MAX_UPLOAD_MB.
func loadConfig() (config, error) {
	var cfg config
	if err := envconfig.Process("hermes", &cfg); err != nil {
		return config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}
