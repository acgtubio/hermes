package main

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// config is owned by this binary. Internal packages receive plain values
// (URLs, ports, clients) rather than importing this struct.
type config struct {
	Port            int           `default:"8080"`
	DatabaseURL     string        `split_words:"true" required:"true"`
	ShutdownTimeout time.Duration `split_words:"true" default:"10s"`
	LogLevel        slog.Level    `split_words:"true" default:"info"`
}

// loadConfig reads HERMES_PORT, HERMES_DATABASE_URL, HERMES_SHUTDOWN_TIMEOUT and HERMES_LOG_LEVEL.
func loadConfig() (config, error) {
	var cfg config
	if err := envconfig.Process("hermes", &cfg); err != nil {
		return config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}
