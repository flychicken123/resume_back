package middleware

import (
	"net/http"
	"resumeai/services"

	"github.com/gin-gonic/gin"
)

// CheckSubscriptionLimit middleware to check if user can generate resume
func CheckSubscriptionLimit(stripeService *services.StripeService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from context (set by auth middleware)
		userID := c.GetInt("user_id")
		if userID == 0 {
			// For non-authenticated users, allow with basic limit check
			// You might want to track by IP or session
			c.Next()
			return
		}

		// Check if user can generate more resumes
		canGenerate, remaining, resetDate, err := stripeService.CheckUsageLimit(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to check subscription limit",
			})
			c.Abort()
			return
		}

		if !canGenerate {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Resume generation limit reached",
				"remaining":   remaining,
				"reset_date":  resetDate,
				"upgrade_url": "/pricing",
				"message":     "You've reached your resume generation limit. Please upgrade your plan or wait for the limit to reset.",
			})
			c.Abort()
			return
		}

		// Store usage info in context for later use
		c.Set("remaining_resumes", remaining)
		c.Set("reset_date", resetDate)

		c.Next()

		// If the request was successful (2xx status), increment usage
		if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			// Increment usage counter
			go func() {
				if err := stripeService.IncrementUsage(userID); err != nil {
					// Log error but don't fail the request
					println("Failed to increment usage:", err.Error())
				}
			}()
		}
	}
}

// RequireSubscription middleware to require specific subscription level
func RequireSubscription(minPlan string) gin.HandlerFunc {
	planLevels := map[string]int{
		"free":     0,
		"premium":  1,
		"ultimate": 2,
	}

	return func(c *gin.Context) {
		userID := c.GetInt("userID")
		if userID == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		// Get user's current plan from context or database
		// This is a simplified version - you'd typically query the database
		userPlan := c.GetString("user_plan")
		if userPlan == "" {
			userPlan = "free" // Default to free
		}

		minLevel := planLevels[minPlan]
		userLevel := planLevels[userPlan]

		if userLevel < minLevel {
			c.JSON(http.StatusForbidden, gin.H{
				"error":       "Subscription upgrade required",
				"required":    minPlan,
				"current":     userPlan,
				"upgrade_url": "/pricing",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
