package middleware

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"resumeai/services"
)

// ExperimentBucket attaches an experiment variant to the Gin context and response headers
// so handlers can branch backend logic without re-implementing assignment.
func ExperimentBucket(experimentKey string, svc *services.ExperimentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIdentifier := resolveExperimentUser(c, "", "")
		result, err := svc.AssignVariant(c.Request.Context(), experimentKey, userIdentifier, c.Request.URL.Path, false)
		if err != nil {
			// Do not block the request if experiment lookup fails
			c.Next()
			return
		}

		// Persist the identifier for stickiness across requests
		persistExperimentUser(c, userIdentifier)

		c.Set(fmt.Sprintf("experiment.%s.variant", experimentKey), result.Variant.VariantKey)
		c.Set(fmt.Sprintf("experiment.%s.assignment", experimentKey), result.Assignment)
		c.Header("X-Experiment-Key", experimentKey)
		c.Header("X-Experiment-Variant", result.Variant.VariantKey)
		c.Header("X-Experiment-User", userIdentifier)

		c.Next()
	}
}

func resolveExperimentUser(c *gin.Context, explicitUser string, anonymousID string) string {
	if val, ok := c.Get("user_id"); ok {
		return fmt.Sprintf("user:%v", val)
	}
	trimmed := strings.TrimSpace(explicitUser)
	if trimmed != "" {
		return trimmed
	}
	anon := strings.TrimSpace(anonymousID)
	if anon != "" {
		return anon
	}
	header := strings.TrimSpace(c.GetHeader("X-Experiment-User"))
	if header != "" {
		return header
	}
	if cookie, err := c.Cookie("ab_user_id"); err == nil && strings.TrimSpace(cookie) != "" {
		return cookie
	}
	return fmt.Sprintf("anon:%s", uuid.NewString())
}

func persistExperimentUser(c *gin.Context, id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	c.SetCookie("ab_user_id", id, 60*60*24*60, "/", "", false, true)
}
