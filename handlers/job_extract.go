package handlers

import (
	"fmt"
	"net/http"
	"resumeai/services"
	"strings"

	"github.com/gin-gonic/gin"
)

type JobExtractRequest struct {
	URL string `json:"url" binding:"required"`
}

type JobExtractResponse struct {
	Description string `json:"description"`
	Title       string `json:"title,omitempty"`
	Company     string `json:"company,omitempty"`
}

// ExtractJobDescription extracts job description from a URL
func ExtractJobDescription(c *gin.Context) {
	var req JobExtractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request. Please provide a valid URL."})
		return
	}

	// Validate URL
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide a valid URL starting with http:// or https://"})
		return
	}

	// Fetch and extract job description using WebFetch service
	prompt := `Extract the job description from this page. Focus on:
1. Job title
2. Company name
3. Full job description including responsibilities, requirements, and qualifications
4. Any technical skills or technologies mentioned

Return the extracted information as plain text in this format:
Job Title: [title if found]
Company: [company if found]

[Full job description text]

If this is not a job posting page, return "NOT_A_JOB_POSTING"`

	result, err := services.FetchAndProcessURL(req.URL, prompt)
	if err != nil {
		fmt.Printf("[ExtractJobDescription] Error fetching URL %s: %v\n", req.URL, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch job description from URL"})
		return
	}

	// Check if it's a job posting
	if strings.Contains(result, "NOT_A_JOB_POSTING") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "The provided URL does not appear to be a job posting"})
		return
	}

	// Parse the result to extract title and company if present
	lines := strings.Split(result, "\n")
	var title, company, description string
	var descriptionStart int

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Job Title:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "Job Title:"))
		} else if strings.HasPrefix(line, "Company:") {
			company = strings.TrimSpace(strings.TrimPrefix(line, "Company:"))
		} else if line != "" && i > 1 {
			descriptionStart = i
			break
		}
	}

	// Get the full description
	if descriptionStart > 0 {
		descriptionLines := lines[descriptionStart:]
		description = strings.TrimSpace(strings.Join(descriptionLines, "\n"))
	} else {
		description = strings.TrimSpace(result)
	}

	if description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not extract job description from the provided URL"})
		return
	}

	response := JobExtractResponse{
		Description: description,
		Title:       title,
		Company:     company,
	}

	c.JSON(http.StatusOK, response)
}
