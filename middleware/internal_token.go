package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequireInternalToken validates a shared secret header for internal/ops endpoints.
// When the env var is empty, the endpoint is treated as disabled and returns 404.
func RequireInternalToken(envVarName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		secret := strings.TrimSpace(os.Getenv(envVarName))
		if secret == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			c.Abort()
			return
		}

		token := strings.TrimSpace(c.GetHeader("X-Internal-Token"))
		if token == "" || token != secret {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			c.Abort()
			return
		}

		c.Next()
	}
}

