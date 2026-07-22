package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
