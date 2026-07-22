package config

import (
	"os"
)

// Config holds process settings read from the environment.
type Config struct {
	Addr string
}

// Load reads ADDR and PORT. Default listen address is :8080.
func Load() Config {
	if addr := os.Getenv("ADDR"); addr != "" {
		return Config{Addr: addr}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return Config{Addr: ":" + port}
}
