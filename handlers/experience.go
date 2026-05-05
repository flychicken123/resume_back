package handlers

import (
	"encoding/json"
	"net/http"
	"resumeai/services"
	"resumeai/utils"
	"strings"

	"github.com/gin-gonic/gin"
)

type ExperienceOptimizationRequest struct {
	JobDescription string   `json:"jobDescription" binding:"required"`
	UserExperience string   `json:"userExperience" binding:"required"`
	MatchedSkills  []string `json:"matchedSkills,omitempty"`
	MissingSkills  []string `json:"missingSkills,omitempty"`
}

type ExperienceOptimizationResponse struct {
	OptimizedExperience string `json:"optimizedExperience"`
	Message             string `json:"message"`
}

func OptimizeExperience(c *gin.Context) {
	var req ExperienceOptimizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	// Build prompt for experience optimization with skill context
	prompt := services.BuildExperienceOptimizationPromptWithSkills(
		req.JobDescription,
		req.UserExperience,
		req.MatchedSkills,
		req.MissingSkills,
	)

	// Call AI service to generate optimized experience
	optimizedExperience, err := services.CallGeminiWithTemperature(prompt, 0.3) // Low temp for consistency
	if err != nil {
		utils.InternalServerError(c, "Failed to optimize experience", err)
		return
	}

	// Validate output to catch potential hallucinations
	validatedExperience := services.ValidateAndCleanOutput(req.UserExperience, optimizedExperience)

	// Clean up the AI response to remove asterisks and format properly
	cleanedExperience := cleanupAIResponse(validatedExperience)

	response := ExperienceOptimizationResponse{
		OptimizedExperience: cleanedExperience,
		Message:             "Experience optimized successfully based on job description.",
	}

	utils.SuccessResponse(c, http.StatusOK, "Experience optimized successfully", response)
}

// cleanupAIResponse removes asterisks and cleans up the AI response
func cleanupAIResponse(text string) string {
	if jsonText := cleanupJSONTextResponse(text); jsonText != "" {
		text = jsonText
	}

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

func cleanupJSONTextResponse(text string) string {
	cleaned := strings.TrimSpace(text)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	if strings.HasPrefix(cleaned, `"`) && strings.HasSuffix(cleaned, `"`) {
		var value string
		if err := json.Unmarshal([]byte(cleaned), &value); err != nil {
			return ""
		}
		return strings.TrimSpace(value)
	}

	if !strings.HasPrefix(cleaned, "[") || !strings.HasSuffix(cleaned, "]") {
		return ""
	}

	var bullets []string
	if err := json.Unmarshal([]byte(cleaned), &bullets); err != nil {
		return ""
	}

	out := make([]string, 0, len(bullets))
	for _, bullet := range bullets {
		bullet = strings.TrimSpace(bullet)
		if bullet != "" {
			out = append(out, bullet)
		}
	}
	return strings.Join(out, "\n")
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
	improvedExperience, err := services.CallGeminiWithTemperature(prompt, 0.3)
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
