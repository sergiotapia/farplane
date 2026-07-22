package httpapi

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farplane/farplane/farplane-backend/internal/config"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

// New builds the Gin engine with middleware and API routes for the local SPA.
// pool may be nil in unit tests that do not hit the database.
func New(pool *pgxpool.Pool, cfg config.Config) *gin.Engine {
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

		authed := v1.Group("/")
		authed.Use(api.sessionOptional(), api.requireSession())
		{
			authed.GET("/me", api.handleMe)
		}
	}

	return r
}
