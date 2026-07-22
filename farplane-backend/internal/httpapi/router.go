package httpapi

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// New builds the Gin engine with middleware and API routes for the local SPA.
func New() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"http://localhost:3000"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
	}))

	r.GET("/health", handleHealth)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/hello", handleHello)
	}

	return r
}
