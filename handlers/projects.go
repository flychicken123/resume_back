package handlers

import (
	"net/http"
	"resumeai/services"
	"resumeai/utils"
	"strings"

	"github.com/gin-gonic/gin"
)

type ProjectOptimizationRequest struct {
	JobDescription  string `json:"jobDescription"`
	ProjectData     string `json:"projectData" binding:"required"`
	ExistingProject string `json:"existingProject,omitempty"` // For preserving/updating existing project
}

type ProjectOptimizationResponse struct {
	OptimizedProject string `json:"optimizedProject"`
	Message          string `json:"message"`
}

type ProjectGrammarRequest struct {
	ProjectData string `json:"projectData" binding:"required"`
}

type ProjectGrammarResponse struct {
	ImprovedProject string `json:"improvedProject"`
	Message         string `json:"message"`
}

// OptimizeProject optimizes project description based on job description
func OptimizeProject(c *gin.Context) {
	var req ProjectOptimizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	// If job description is provided, optimize for it
	// Otherwise, just improve grammar and professionalism
	var prompt string
	if req.JobDescription != "" {
		prompt = services.BuildProjectOptimizationPrompt(req.JobDescription, req.ProjectData, req.ExistingProject)
	} else {
		// Fall back to grammar improvement if no job description
		prompt = services.BuildProjectGrammarPrompt(req.ProjectData, req.ExistingProject)
	}

	// Call AI service to generate optimized project
	optimizedProject, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		utils.InternalServerError(c, "Failed to optimize project", err)
		return
	}

	// Clean up the AI response
	cleanedProject := cleanupProjectResponse(optimizedProject)

	message := "Project optimized successfully"
	if req.JobDescription != "" {
		message += " based on job description."
	} else {
		message += " for grammar and professionalism."
	}

	response := ProjectOptimizationResponse{
		OptimizedProject: cleanedProject,
		Message:          message,
	}

	utils.SuccessResponse(c, http.StatusOK, message, response)
}

// ImproveProjectGrammar improves grammar and professionalism of project description
func ImproveProjectGrammar(c *gin.Context) {
	var req ProjectGrammarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build prompt for grammar improvement
	prompt := services.BuildProjectGrammarPrompt(req.ProjectData)

	// Call AI service to improve grammar
	improvedProject, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Clean up the AI response
	cleanedProject := cleanupProjectResponse(improvedProject)

	response := ProjectGrammarResponse{
		ImprovedProject: cleanedProject,
		Message:         "Project description grammar and style improved successfully.",
	}

	c.JSON(http.StatusOK, response)
}

// cleanupProjectResponse removes asterisks and cleans up the AI response
func cleanupProjectResponse(text string) string {
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
