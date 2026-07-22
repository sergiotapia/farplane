package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultAddr              = ":8080"
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultIdleTimeout       = 60 * time.Second
	defaultShutdownTimeout   = 10 * time.Second
	// Local-only default. Used when APP_ENV is empty, local, or development.
	defaultDatabaseURL = "postgres://postgres:postgres@127.0.0.1:5432/farplane_dev?sslmode=disable"
)

// Config holds process settings read from the environment.
type Config struct {
	Addr              string
	AppEnv            string
	DatabaseURL       string
	GinMode           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// Load reads ADDR, PORT, APP_ENV, DATABASE_URL, and GIN_MODE.
// Default listen address is :8080. ADDR wins when set. A leading ":" on PORT is stripped.
//
// DATABASE_URL defaults to the local farplane_dev DSN only when APP_ENV is empty,
// "local", "dev", or "development". Other environments require DATABASE_URL.
func Load() (Config, error) {
	cfg := Config{
		Addr:              defaultAddr,
		AppEnv:            strings.TrimSpace(os.Getenv("APP_ENV")),
		GinMode:           os.Getenv("GIN_MODE"),
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		ShutdownTimeout:   defaultShutdownTimeout,
	}

	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		cfg.DatabaseURL = databaseURL
	} else if allowsDefaultDatabaseURL(cfg.AppEnv) {
		cfg.DatabaseURL = defaultDatabaseURL
	} else {
		return Config{}, fmt.Errorf("DATABASE_URL is required when APP_ENV=%q", cfg.AppEnv)
	}

	if addr := os.Getenv("ADDR"); addr != "" {
		cfg.Addr = addr
		return cfg, nil
	}

	if port := os.Getenv("PORT"); port != "" {
		cfg.Addr = ":" + strings.TrimPrefix(port, ":")
	}

	return cfg, nil
}

func allowsDefaultDatabaseURL(appEnv string) bool {
	switch strings.ToLower(appEnv) {
	case "", "local", "dev", "development":
		return true
	default:
		return false
	}
}
