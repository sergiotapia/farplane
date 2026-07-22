package httpapi

import (
	"context"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farplane/farplane/farplane-backend/internal/config"
	"github.com/farplane/farplane/farplane-backend/internal/githubapp"
	"github.com/farplane/farplane/farplane-backend/internal/lanehub"
	dockerruntime "github.com/farplane/farplane/farplane-backend/internal/runtime/docker"
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
	var rt = dockerruntime.New()
	hub := lanehub.New()
	api := newAPI(cfg, st, rt, hub)
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

		v1.GET("/lane-invites/:token", api.handleGetLaneInvite)
		v1.POST("/lane-invites/:token/signup", authLimiter.middleware(), api.handleSignupLaneInvite)

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

			authed.GET("/lane-templates", api.handleListLaneTemplates)
			authed.POST("/lane-templates", api.handleCreateLaneTemplate)
			authed.GET("/lane-templates/:id", api.handleGetLaneTemplate)
			authed.PATCH("/lane-templates/:id", api.handleUpdateLaneTemplate)
			authed.DELETE("/lane-templates/:id", api.handleDeleteLaneTemplate)
			authed.POST("/lane-templates/:id/fork", api.handleForkLaneTemplate)
			authed.POST("/lane-templates/:id/validate", api.handleValidateLaneTemplate)

			authed.GET("/secrets", api.handleListSecrets)
			authed.PUT("/secrets/:name", api.handleSetSecret)
			authed.DELETE("/secrets/:name", api.handleClearSecret)

			authed.GET("/lane-agents", api.handleListLaneAgents)

			authed.GET("/projects/:id/lanes", api.handleListProjectLanes)
			authed.POST("/projects/:id/lanes", api.handleCreateLane)
			authed.GET("/lanes/:id", api.handleGetLane)
			authed.PATCH("/lanes/:id", api.handlePatchLane)
			authed.GET("/lanes/:id/messages", api.handleListLaneMessages)
			authed.POST("/lanes/:id/messages", api.handlePostLaneMessage)
			authed.GET("/lanes/:id/ws", api.handleLaneWebSocket)

			authed.GET("/lanes/:id/participants", api.handleListLaneParticipants)
			authed.POST("/lanes/:id/invites", api.handleCreateLaneInvite)
			authed.GET("/lanes/:id/invites", api.handleListLaneInvites)
			authed.DELETE("/lanes/:id/participants/:user_id", api.handleKickLaneParticipant)
			authed.POST("/lane-invites/:token/accept", api.handleAcceptLaneInvite)

			authed.GET("/organization-members", api.handleListOrganizationMembers)
		}
	}

	return r
}
