package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireAdmin ensures that the current authenticated user has admin privileges before
// allowing the request to proceed.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdminValue, exists := c.Get("is_admin")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}

		isAdmin, ok := isAdminValue.(bool)
		if !ok || !isAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}

		c.Next()
	}
}
