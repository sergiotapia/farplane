package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func handleHello(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "farplane"})
}
