package config

import (
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
)

// Config holds process settings read from the environment.
type Config struct {
	Addr              string
	GinMode           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// Load reads ADDR, PORT, and GIN_MODE. Default listen address is :8080.
// ADDR wins when set. A leading ":" on PORT is stripped.
func Load() Config {
	cfg := Config{
		Addr:              defaultAddr,
		GinMode:           os.Getenv("GIN_MODE"),
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		ShutdownTimeout:   defaultShutdownTimeout,
	}

	if addr := os.Getenv("ADDR"); addr != "" {
		cfg.Addr = addr
		return cfg
	}

	if port := os.Getenv("PORT"); port != "" {
		cfg.Addr = ":" + strings.TrimPrefix(port, ":")
	}

	return cfg
}
