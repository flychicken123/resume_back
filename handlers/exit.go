package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type exitEventRequest struct {
	PagePath         string `json:"page_path" binding:"required"`
	PageTitle        string `json:"page_title"`
	Step             string `json:"step"`
	Reason           string `json:"reason"`
	UserEmail        string `json:"user_email"`
	PreviousPagePath string `json:"previous_page_path"`
	SessionDuration  *int64 `json:"session_duration_ms"`
	PageDuration     *int64 `json:"page_duration_ms"`
	LastStepDelta    *int64 `json:"last_step_delta_ms"`
	Referrer         string `json:"referrer"`
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
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			reason = "unknown"
		}

		previousPath := strings.TrimSpace(req.PreviousPagePath)
		referrer := strings.TrimSpace(req.Referrer)

		_, err := db.Exec(
			`INSERT INTO exit_events (
			    user_email,
			    page_path,
			    page_title,
			    step,
			    reason,
			    user_agent,
			    previous_page_path,
			    session_duration_ms,
			    page_duration_ms,
			    last_step_delta_ms,
			    referrer
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			strings.TrimSpace(req.UserEmail),
			pagePath,
			strings.TrimSpace(req.PageTitle),
			strings.TrimSpace(req.Step),
			reason,
			userAgent,
			nullableString(previousPath),
			nullableInt64(req.SessionDuration),
			nullableInt64(req.PageDuration),
			nullableInt64(req.LastStepDelta),
			nullableString(referrer),
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record exit event"})
			return
		}

		c.Status(http.StatusNoContent)
	}
}

func nullableInt64(value *int64) interface{} {
	if value == nil {
		return nil
	}
	if *value <= 0 {
		return nil
	}
	return *value
}

func nullableString(value string) interface{} {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}
