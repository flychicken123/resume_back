package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type exitEventRequest struct {
	PagePath  string `json:"page_path" binding:"required"`
	PageTitle string `json:"page_title"`
	Step      string `json:"step"`
	Reason    string `json:"reason"`
	UserEmail string `json:"user_email"`
}

func TrackExitEvent(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req exitEventRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid exit event payload"})
			return
		}

		pagePath := strings.TrimSpace(req.PagePath)
		if pagePath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "page_path is required"})
			return
		}

		userAgent := c.Request.UserAgent()
		_, err := db.Exec(
			`INSERT INTO exit_events (user_email, page_path, page_title, step, reason, user_agent)
             VALUES ($1, $2, $3, $4, $5, $6)`,
			strings.TrimSpace(req.UserEmail),
			pagePath,
			strings.TrimSpace(req.PageTitle),
			strings.TrimSpace(req.Step),
			strings.TrimSpace(req.Reason),
			userAgent,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record exit event"})
			return
		}

		c.Status(http.StatusNoContent)
	}
}
