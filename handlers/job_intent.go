package handlers

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"resumeai/services"
	"resumeai/utils"
)

var jobIntentAgent *services.JobIntentAgent

// SetJobIntentAgent injects the LangChain-backed job intent agent.
func SetJobIntentAgent(agent *services.JobIntentAgent) {
	jobIntentAgent = agent
}

type jobIntentRequest struct {
	Text string `json:"text" binding:"required"`
}

// ParseJobIntent analyzes natural language to understand job URL + description intent.
func ParseJobIntent(c *gin.Context) {
	if jobIntentAgent == nil {
		utils.InternalServerError(c, "Job intent agent is not configured", nil)
		return
	}

	var req jobIntentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		utils.BadRequestError(c, "text is required", nil)
		return
	}

	result, err := jobIntentAgent.ParseJobIntent(c.Request.Context(), text)
	if err != nil {
		log.Printf("[job-intent] failed to parse text=%q error=%v", sanitizeForLog(text), err)
		utils.InternalServerError(c, "Failed to understand job description details", err)
		return
	}

	log.Printf(
		"[job-intent] text=%q -> url=%q job_description_from_text.len=%d",
		sanitizeForLog(text),
		result.URL,
		len(result.JobDescriptionFromText),
	)

	c.JSON(http.StatusOK, gin.H{
		"url":                       result.URL,
		"job_description_from_text": result.JobDescriptionFromText,
	})
}

