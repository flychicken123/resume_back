package handlers

import (
	"net/http"
	"resumeai/services"

	"github.com/gin-gonic/gin"
)

type ExperienceOptimizationRequest struct {
	JobDescription string `json:"jobDescription" binding:"required"`
	UserExperience string `json:"userExperience" binding:"required"`
}

type ExperienceOptimizationResponse struct {
	OptimizedExperience string `json:"optimizedExperience"`
	Message             string `json:"message"`
}

func OptimizeExperience(c *gin.Context) {
	var req ExperienceOptimizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build prompt for experience optimization
	prompt := services.BuildExperienceOptimizationPrompt(req.JobDescription, req.UserExperience)

	// Call AI service to generate optimized experience
	optimizedExperience, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := ExperienceOptimizationResponse{
		OptimizedExperience: optimizedExperience,
		Message:             "Experience optimized successfully based on job description.",
	}

	c.JSON(http.StatusOK, response)
}
