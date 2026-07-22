package config

import (
	"fmt"
	"os"
	"strconv"
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
	defaultAppBaseURL        = "http://localhost:3000"
	defaultGoogleRedirectURL = "http://localhost:8080/api/v1/auth/google/callback"
	// Local-only default. Used when APP_ENV is empty, local, or development.
	defaultDatabaseURL = "postgres://postgres:postgres@127.0.0.1:5432/farplane_dev?sslmode=disable"
	// Local-only signing key for OAuth state. Override in real installs.
	defaultSessionSecret = "farplane-local-session-secret-change-me"
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

	// Auth / first-time setup
	AppBaseURL          string
	GoogleClientID      string
	GoogleClientSecret  string
	GoogleRedirectURL   string
	SessionSecret       string
	SessionCookieSecure bool
	SessionTTL          time.Duration
	// SetupToken, when non-empty, must be sent to complete first-time setup.
	SetupToken string
}

// GoogleOAuthConfigured reports whether Google sign-in can run.
func (c Config) GoogleOAuthConfigured() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != "" && c.GoogleRedirectURL != ""
}

// Load reads ADDR, PORT, APP_ENV, DATABASE_URL, GIN_MODE, and auth-related env vars.
// Default listen address is :8080. ADDR wins when set. A leading ":" on PORT is stripped.
//
// DATABASE_URL defaults to the local farplane_dev DSN only when APP_ENV is empty,
// "local", "dev", or "development". Other environments require DATABASE_URL.
//
// SESSION_SECRET defaults only in local/dev; other environments require it.
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
		AppBaseURL:        defaultAppBaseURL,
		GoogleRedirectURL: defaultGoogleRedirectURL,
		SessionTTL:        30 * 24 * time.Hour,
	}

	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		cfg.DatabaseURL = databaseURL
	} else if allowsDefaultDatabaseURL(cfg.AppEnv) {
		cfg.DatabaseURL = defaultDatabaseURL
	} else {
		return Config{}, fmt.Errorf("DATABASE_URL is required when APP_ENV=%q", cfg.AppEnv)
	}

	if v := strings.TrimSpace(os.Getenv("APP_BASE_URL")); v != "" {
		cfg.AppBaseURL = strings.TrimRight(v, "/")
	}
	cfg.GoogleClientID = strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	cfg.GoogleClientSecret = strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET"))
	if v := strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URL")); v != "" {
		cfg.GoogleRedirectURL = v
	}

	if secret := os.Getenv("SESSION_SECRET"); secret != "" {
		cfg.SessionSecret = secret
	} else if allowsDefaultDatabaseURL(cfg.AppEnv) {
		cfg.SessionSecret = defaultSessionSecret
	} else {
		return Config{}, fmt.Errorf("SESSION_SECRET is required when APP_ENV=%q", cfg.AppEnv)
	}

	cfg.SetupToken = strings.TrimSpace(os.Getenv("SETUP_TOKEN"))

	if v := strings.TrimSpace(os.Getenv("SESSION_COOKIE_SECURE")); v != "" {
		secure, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("SESSION_COOKIE_SECURE: %w", err)
		}
		cfg.SessionCookieSecure = secure
	} else {
		// Secure cookies by default outside local/dev HTTP.
		cfg.SessionCookieSecure = !allowsDefaultDatabaseURL(cfg.AppEnv)
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
