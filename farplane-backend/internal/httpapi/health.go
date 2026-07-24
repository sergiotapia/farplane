package httpapi

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// handleHealth is liveness only: the process is up.
func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{jsonKeyStatus: "ok"})
}

// handleReady is readiness: Postgres must accept a ping.
func handleReady(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if pool == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				jsonKeyStatus: "unavailable",
				"database":    "missing",
			})

			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			log.Printf("ready: database ping failed: %v", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				jsonKeyStatus: "unavailable",
				"database":    "down",
			})

			return
		}

		c.JSON(http.StatusOK, gin.H{
			jsonKeyStatus: "ok",
			"database":    "up",
		})
	}
}
