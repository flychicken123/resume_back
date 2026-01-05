package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"resumeai/services"
	"resumeai/utils"
)

var educationAgent *services.EducationAgent

// SetEducationAgent injects the LangChain-powered education parser.
func SetEducationAgent(agent *services.EducationAgent) {
	educationAgent = agent
}

type educationParseRequest struct {
	Text     string                         `json:"text" binding:"required"`
	Existing *services.EducationParseResult `json:"existing,omitempty"`
}

// ParseEducation analyzes free-form text using LangChain to extract education details.
func ParseEducation(c *gin.Context) {
	if educationAgent == nil {
		utils.InternalServerError(c, "Education agent is not configured", nil)
		return
	}

	var req educationParseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	// Use existing data if provided, otherwise start fresh
	existing := services.EducationParseResult{}
	if req.Existing != nil {
		existing = *req.Existing
	}

	result, err := educationAgent.ExtractEducationWithContext(c.Request.Context(), req.Text, existing)
	if err != nil {
		utils.InternalServerError(c, "Failed to extract education details", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}
