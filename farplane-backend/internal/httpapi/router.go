package httpapi

import (
	"context"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farplane/farplane/farplane-backend/internal/config"
	"github.com/farplane/farplane/farplane-backend/internal/githubapp"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

// Option configures the API server (used by tests to inject a GitHub App client).
type Option func(*api)

// WithGitHubApp injects a GitHub App client (typically a fake in tests).
func WithGitHubApp(client GitHubApp) Option {
	return func(a *api) {
		a.githubMu.Lock()
		defer a.githubMu.Unlock()
		a.github = client
		a.githubForced = true
	}
}

// WithManifestConvert injects a fake manifest code exchange (tests).
func WithManifestConvert(fn func(ctx context.Context, code string) (githubapp.ManifestApp, error)) Option {
	return func(a *api) {
		a.manifestConvert = fn
	}
}

// New builds the Gin engine with middleware and API routes for the local SPA.
// pool may be nil in unit tests that do not hit the database.
func New(pool *pgxpool.Pool, cfg config.Config, opts ...Option) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.Use(cors.New(cors.Config{
		AllowOrigins:     spaOrigins(cfg),
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Farplane-Setup-Token"},
		AllowCredentials: true,
	}))

	r.GET("/health", handleHealth)
	r.GET("/ready", handleReady(pool))

	var st *store.Store
	if pool != nil {
		st = store.New(pool)
	}
	api := newAPI(cfg, st)
	for _, opt := range opts {
		opt(api)
	}
	authLimiter := newIPRateLimiter(30, time.Minute)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/hello", handleHello)

		v1.GET("/setup/status", api.handleSetupStatus)
		v1.POST("/setup", authLimiter.middleware(), api.handleSetup)

		v1.GET("/auth/google/start", api.handleGoogleStart)
		v1.GET("/auth/google/callback", api.handleGoogleCallback)
		v1.POST("/auth/login", authLimiter.middleware(), api.handleLogin)
		v1.POST("/auth/logout", api.handleLogout)

		v1.GET("/github/callback", api.handleGitHubCallback)
		v1.GET("/github/app/manifest/callback", api.handleGitHubManifestCallback)
		v1.POST("/github/webhook", api.handleGitHubWebhook)

		authed := v1.Group("/")
		authed.Use(api.sessionOptional(), api.requireSession())
		{
			authed.GET("/me", api.handleMe)

			authed.POST("/github/app/manifest/start", api.handleGitHubManifestStart)
			authed.POST("/github/install/start", api.handleGitHubInstallStart)
			authed.GET("/github/installations", api.handleListGitHubInstallations)
			authed.DELETE("/github/installations/:id", api.handleDisconnectGitHubInstallation)
			authed.GET("/github/repositories", api.handleListGitHubRepositories)

			authed.GET("/projects", api.handleListProjects)
			authed.POST("/projects", api.handleCreateProject)
			authed.GET("/projects/:id", api.handleGetProject)
		}
	}

	return r
}
