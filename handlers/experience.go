package handlers

import (
	"net/http"
	"resumeai/services"
	"strings"

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

	// Clean up the AI response to remove asterisks and format properly
	cleanedExperience := cleanupAIResponse(optimizedExperience)

	response := ExperienceOptimizationResponse{
		OptimizedExperience: cleanedExperience,
		Message:             "Experience optimized successfully based on job description.",
	}

	c.JSON(http.StatusOK, response)
}

// cleanupAIResponse removes asterisks and cleans up the AI response
func cleanupAIResponse(text string) string {
	// Split into lines
	lines := strings.Split(text, "\n")
	var cleanedLines []string

	for _, line := range lines {
		// Trim whitespace
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Remove leading asterisk and any whitespace after it
		if strings.HasPrefix(line, "*") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		}

		// Remove bullet points if they exist
		if strings.HasPrefix(line, "•") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "•"))
		}
		if strings.HasPrefix(line, "-") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		}

		// Add the cleaned line
		if line != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}

	// Join lines back together with double newlines for proper spacing
	return strings.Join(cleanedLines, "\n\n")
}

type ExperienceGrammarRequest struct {
	UserExperience string `json:"userExperience" binding:"required"`
}

type ExperienceGrammarResponse struct {
	ImprovedExperience string `json:"improvedExperience"`
	Message            string `json:"message"`
}

func ImproveExperienceGrammar(c *gin.Context) {
	var req ExperienceGrammarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build prompt for grammar improvement
	prompt := services.BuildExperienceGrammarPrompt(req.UserExperience)

	// Call AI service to improve grammar
	improvedExperience, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Clean up the AI response
	cleanedExperience := cleanupAIResponse(improvedExperience)

	response := ExperienceGrammarResponse{
		ImprovedExperience: cleanedExperience,
		Message:            "Experience grammar and style improved successfully.",
	}

	c.JSON(http.StatusOK, response)
}
