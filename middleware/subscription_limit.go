package middleware

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CheckResumeLimitMiddleware checks if user can generate a resume based on their subscription limits
func CheckResumeLimitMiddleware(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from context (set by auth middleware)
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":     "User not authenticated",
				"needsAuth": true,
			})
			c.Abort()
			return
		}

		userIDInt, ok := userID.(int)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID"})
			c.Abort()
			return
		}

		// Check resume limit using the database function
		var canGenerate sql.NullBool
		var remaining sql.NullInt64
		var resetDate sql.NullTime

		err := db.QueryRow(`
			SELECT can_generate, remaining_count, reset_date
			FROM check_resume_limit($1)
		`, userIDInt).Scan(&canGenerate, &remaining, &resetDate)

		if err != nil {
			fmt.Printf("Error checking resume limit: %v\n", err)
			// If there's an error, allow the request but log it
			c.Next()
			return
		}

		// Handle null values - if null, allow by default
		if !canGenerate.Valid {
			canGenerate.Bool = true
			canGenerate.Valid = true
		}
		if !remaining.Valid {
			remaining.Int64 = 1
			remaining.Valid = true
		}

		if !canGenerate.Bool {
			// Get user's plan details for the error message
			var planName string
			var resumeLimit int
			var resumePeriod string

			err = db.QueryRow(`
				SELECT sp.display_name, sp.resume_limit, sp.resume_period
				FROM users u
				LEFT JOIN subscription_plans sp ON sp.id = COALESCE(u.subscription_plan_id, 1)
				WHERE u.id = $1
			`, userIDInt).Scan(&planName, &resumeLimit, &resumePeriod)

			if err != nil {
				planName = "Free"
				resumeLimit = 1
				resumePeriod = "weekly"
			}

			message := fmt.Sprintf("You have reached your %s limit of %d resume(s) per %s. ",
				planName, resumeLimit, resumePeriod)

			if resetDate.Valid {
				message += fmt.Sprintf("Your limit will reset on %s.",
					resetDate.Time.Format("January 2, 2006"))
			}

			c.JSON(http.StatusForbidden, gin.H{
				"error":        message,
				"limitReached": true,
				"plan":         planName,
				"limit":        resumeLimit,
				"period":       resumePeriod,
				"remaining":    0,
				"resetDate":    resetDate.Time,
			})
			c.Abort()
			return
		}

		// Store the usage info in context for potential use
		c.Set("resume_limit_remaining", int(remaining.Int64))
		c.Set("resume_limit_reset_date", resetDate.Time)

		// After successful generation, increment usage
		c.Next()

		// Check if the request was successful (2xx status)
		if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			// Increment the usage count
			_, err := db.Exec(`SELECT increment_resume_usage($1)`, userIDInt)
			if err != nil {
				fmt.Printf("Warning: Failed to increment resume usage: %v\n", err)
			} else {
				fmt.Printf("Resume usage incremented for user %d\n", userIDInt)
			}
		}
	}
}
