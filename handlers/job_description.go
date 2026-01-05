package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"resumeai/services"
	"resumeai/utils"
)

var jobDescriptionAgent *services.JobDescriptionAgent

// SetJobDescriptionAgent injects the LangChain-powered job description manager.
func SetJobDescriptionAgent(agent *services.JobDescriptionAgent) {
	jobDescriptionAgent = agent
}

type jobDescriptionParseRequest struct {
	Text     string                          `json:"text" binding:"required"`
	Existing []services.JobDescriptionEntry  `json:"existing,omitempty"`
}

// ParseJobDescription analyzes user input to add, modify, or remove job descriptions.
func ParseJobDescription(c *gin.Context) {
	if jobDescriptionAgent == nil {
		utils.InternalServerError(c, "Job description agent is not configured", nil)
		return
	}

	var req jobDescriptionParseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	// Use existing data if provided, otherwise start fresh
	existing := []services.JobDescriptionEntry{}
	if req.Existing != nil {
		existing = req.Existing
	}

	result, err := jobDescriptionAgent.ParseJobDescription(c.Request.Context(), req.Text, existing)
	if err != nil {
		utils.InternalServerError(c, "Failed to parse job description", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}
