package handlers

import (
	"net/http"
	"resumeai/services"

	"github.com/gin-gonic/gin"
)

type EducationOptimizationRequest struct {
	Education string `json:"education" binding:"required"`
}

type EducationOptimizationResponse struct {
	OptimizedEducation string `json:"education"`
	Message            string `json:"message"`
}

type SummaryOptimizationRequest struct {
	Experience string   `json:"experience"`
	Education  string   `json:"education"`
	Skills     []string `json:"skills"`
}

type SummaryOptimizationResponse struct {
	Summary string `json:"summary"`
	Message string `json:"message"`
}

func OptimizeEducation(c *gin.Context) {
	var req EducationOptimizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build prompt for education optimization
	prompt := services.BuildEducationOptimizationPrompt(req.Education)

	// Call AI service to generate optimized education
	optimizedEducation, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Clean up the AI response
	cleanedEducation := cleanupAIResponse(optimizedEducation)

	response := EducationOptimizationResponse{
		OptimizedEducation: cleanedEducation,
		Message:            "Education optimized successfully.",
	}

	c.JSON(http.StatusOK, response)
}

func OptimizeSummary(c *gin.Context) {
	var req SummaryOptimizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build prompt for summary optimization
	prompt := services.BuildSummaryOptimizationPrompt(req.Experience, req.Education, req.Skills)

	// Call AI service to generate optimized summary
	optimizedSummary, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Clean up the AI response
	cleanedSummary := cleanupAIResponse(optimizedSummary)

	response := SummaryOptimizationResponse{
		Summary: cleanedSummary,
		Message: "Summary optimized successfully.",
	}

	c.JSON(http.StatusOK, response)
}
