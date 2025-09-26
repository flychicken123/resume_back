package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"resumeai/models"
)

type submitFeedbackRequest struct {
	Scenario          string                 `json:"scenario" binding:"required"`
	Rating            *int                   `json:"rating"`
	Comment           string                 `json:"comment"`
	Metadata          map[string]interface{} `json:"metadata"`
	UserEmail         string                 `json:"user_email"`
	PagePath          string                 `json:"page_path"`
	PageTitle         string                 `json:"page_title"`
	Step              string                 `json:"step"`
	Reason            string                 `json:"reason"`
	PreviousPagePath  string                 `json:"previous_page_path"`
	SessionDurationMs *int64                 `json:"session_duration_ms"`
	PageDurationMs    *int64                 `json:"page_duration_ms"`
	LastStepDeltaMs   *int64                 `json:"last_step_delta_ms"`
	Referrer          string                 `json:"referrer"`
}

type scheduleFollowUpRequest struct {
	Trigger    string                 `json:"trigger" binding:"required"`
	UserEmail  string                 `json:"user_email" binding:"required"`
	DelayHours int                    `json:"delay_hours"`
	Metadata   map[string]interface{} `json:"metadata"`
}

func SubmitFeedback(model *models.FeedbackModel) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req submitFeedbackRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid feedback payload"})
			return
		}

		payload := map[string]interface{}{
			"scenario":            req.Scenario,
			"rating":              req.Rating,
			"comment":             req.Comment,
			"metadata":            req.Metadata,
			"user_email":          req.UserEmail,
			"page_path":           req.PagePath,
			"page_title":          req.PageTitle,
			"step":                req.Step,
			"reason":              req.Reason,
			"previous_page_path":  req.PreviousPagePath,
			"session_duration_ms": req.SessionDurationMs,
			"page_duration_ms":    req.PageDurationMs,
			"last_step_delta_ms":  req.LastStepDeltaMs,
			"referrer":            req.Referrer,
		}

		if err := model.CreateFeedback(c.Request.Context(), payload); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record feedback"})
			return
		}

		c.Status(http.StatusNoContent)
	}
}

func ScheduleFeedbackFollowUp(model *models.FeedbackModel) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req scheduleFollowUpRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid follow-up payload"})
			return
		}

		delay := time.Duration(req.DelayHours) * time.Hour
		if delay <= 0 {
			delay = 24 * time.Hour
		}

		fu := models.FeedbackFollowUp{
			UserEmail:   req.UserEmail,
			TriggerKey:  req.Trigger,
			ScheduledAt: time.Now().Add(delay),
			Metadata:    req.Metadata,
		}

		if err := model.ScheduleFollowUp(c.Request.Context(), fu); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to schedule follow-up"})
			return
		}

		c.Status(http.StatusNoContent)
	}
}
