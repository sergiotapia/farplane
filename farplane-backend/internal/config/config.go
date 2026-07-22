package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	defaultAddr              = ":8080"
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	// Zero disables write deadlines so Lane WebSockets are not cut off mid-stream.
	defaultWriteTimeout      = 0
	defaultIdleTimeout       = 60 * time.Second
	defaultShutdownTimeout   = 10 * time.Second
	defaultAppBaseURL        = "http://localhost:3000"
	defaultAppAPIBaseURL     = "http://localhost:8080"
	defaultGoogleRedirectURL = "http://localhost:8080/api/v1/auth/google/callback"
	defaultGitHubCallbackURL = "http://localhost:8080/api/v1/github/callback"
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
	AppAPIBaseURL       string // public URL of this API (manifest webhooks/callbacks)
	GoogleClientID      string
	GoogleClientSecret  string
	GoogleRedirectURL   string
	SessionSecret       string
	SessionCookieSecure bool
	SessionTTL          time.Duration
	// SetupToken, when non-empty, must be sent to complete first-time setup.
	SetupToken string

	// GitHub App (org/repo connect; not sign-in). Optional env override;
	// self-hosted installs normally create the App via the manifest flow and
	// store credentials in Postgres.
	GitHubAppID            int64
	GitHubAppSlug          string
	GitHubAppPrivateKeyPEM string
	GitHubAppWebhookSecret string
	GitHubAppClientID      string
	GitHubAppClientSecret  string
	GitHubAppCallbackURL   string

	// Runtime selects the Lane computer backend: "docker" (default). Sprites later.
	Runtime string
}

// GoogleOAuthConfigured reports whether Google sign-in can run.
func (c Config) GoogleOAuthConfigured() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != "" && c.GoogleRedirectURL != ""
}

// GitHubAppConfigured reports whether GitHub App install and API calls can run.
func (c Config) GitHubAppConfigured() bool {
	return c.GitHubAppID > 0 &&
		c.GitHubAppSlug != "" &&
		c.GitHubAppPrivateKeyPEM != "" &&
		c.GitHubAppWebhookSecret != "" &&
		c.GitHubAppCallbackURL != ""
}

// Load reads ADDR, PORT, APP_ENV, DATABASE_URL, GIN_MODE, and auth-related env vars.
// Default listen address is :8080. ADDR wins when set. A leading ":" on PORT is stripped.
//
// Load also reads a `.env` file when present (does not override variables already
// set in the process environment). It checks the current working directory and
// the repo root one level up (so `make backend` from farplane-backend works).
//
// DATABASE_URL defaults to the local farplane_dev DSN only when APP_ENV is empty,
// "local", "dev", or "development". Other environments require DATABASE_URL.
//
// SESSION_SECRET defaults only in local/dev; other environments require it.
func Load() (Config, error) {
	loadDotEnv()

	cfg := Config{
		Addr:                 defaultAddr,
		AppEnv:               strings.TrimSpace(os.Getenv("APP_ENV")),
		GinMode:              os.Getenv("GIN_MODE"),
		ReadHeaderTimeout:    defaultReadHeaderTimeout,
		ReadTimeout:          defaultReadTimeout,
		WriteTimeout:         defaultWriteTimeout,
		IdleTimeout:          defaultIdleTimeout,
		ShutdownTimeout:      defaultShutdownTimeout,
		AppBaseURL:           defaultAppBaseURL,
		AppAPIBaseURL:        defaultAppAPIBaseURL,
		GoogleRedirectURL:    defaultGoogleRedirectURL,
		GitHubAppCallbackURL: defaultGitHubCallbackURL,
		SessionTTL:           30 * 24 * time.Hour,
		Runtime:              "docker",
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
	if v := strings.TrimSpace(os.Getenv("APP_API_BASE_URL")); v != "" {
		cfg.AppAPIBaseURL = strings.TrimRight(v, "/")
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

	if v := strings.TrimSpace(os.Getenv("GITHUB_APP_ID")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			return Config{}, fmt.Errorf("GITHUB_APP_ID: must be a positive integer")
		}
		cfg.GitHubAppID = id
	}
	cfg.GitHubAppSlug = strings.TrimSpace(os.Getenv("GITHUB_APP_SLUG"))
	cfg.GitHubAppWebhookSecret = strings.TrimSpace(os.Getenv("GITHUB_APP_WEBHOOK_SECRET"))
	cfg.GitHubAppClientID = strings.TrimSpace(os.Getenv("GITHUB_APP_CLIENT_ID"))
	cfg.GitHubAppClientSecret = strings.TrimSpace(os.Getenv("GITHUB_APP_CLIENT_SECRET"))
	if v := strings.TrimSpace(os.Getenv("GITHUB_APP_CALLBACK_URL")); v != "" {
		cfg.GitHubAppCallbackURL = v
	}
	pem, err := loadGitHubAppPrivateKeyPEM()
	if err != nil {
		return Config{}, err
	}
	cfg.GitHubAppPrivateKeyPEM = pem

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

	if v := strings.TrimSpace(os.Getenv("RUNTIME")); v != "" {
		cfg.Runtime = strings.ToLower(v)
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

func loadDotEnv() {
	candidates := []string{".env"}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "..", ".env"))
	}
	for _, path := range candidates {
		_ = godotenv.Load(path)
	}
}

// IsPublicAPIBaseURL reports whether url is acceptable for GitHub App webhooks.
// GitHub requires a public host; we also require HTTPS so the manifest code
// (which yields the App private key) is not sent over cleartext.
func IsPublicAPIBaseURL(raw string) bool {
	u := strings.ToLower(strings.TrimSpace(raw))
	if u == "" || !strings.HasPrefix(u, "https://") {
		return false
	}
	if strings.Contains(u, "localhost") || strings.Contains(u, "127.0.0.1") || strings.Contains(u, "[::1]") {
		return false
	}
	return true
}

// loadGitHubAppPrivateKeyPEM reads GITHUB_APP_PRIVATE_KEY or GITHUB_APP_PRIVATE_KEY_PATH.
// Inline PEM may use literal \n for multiline env values.
func loadGitHubAppPrivateKeyPEM() (string, error) {
	if path := strings.TrimSpace(os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("GITHUB_APP_PRIVATE_KEY_PATH: %w", err)
		}
		return string(raw), nil
	}
	pem := strings.TrimSpace(os.Getenv("GITHUB_APP_PRIVATE_KEY"))
	if pem == "" {
		return "", nil
	}
	return strings.ReplaceAll(pem, `\n`, "\n"), nil
}
