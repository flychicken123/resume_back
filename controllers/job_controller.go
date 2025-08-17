package controllers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"resumeai/services"
	"resumeai/utils"
)

// JobController handles job-related API endpoints
type JobController struct {
	jobService *services.JobAutomationService
}

// NewJobController creates a new job controller
func NewJobController() *JobController {
	return &JobController{
		jobService: services.NewJobAutomationService(),
	}
}

// ParseJobRequest represents the request structure for job parsing
type ParseJobRequest struct {
	URL string `json:"url" binding:"required"`
}

// ParseJobResponse represents the response structure for job parsing
type ParseJobResponse struct {
	Success    bool                     `json:"success"`
	JobDetails *services.JobDetails     `json:"jobDetails,omitempty"`
	Platform   services.PlatformInfo    `json:"platform"`
	Error      string                   `json:"error,omitempty"`
}

// SubmitApplicationRequest represents the request structure for job application
type SubmitApplicationRequest struct {
	services.ApplicationData
}

// SubmitApplicationResponse represents the response structure for job application
type SubmitApplicationResponse struct {
	Success bool                         `json:"success"`
	Result  *services.ApplicationResult  `json:"result,omitempty"`
	Error   string                       `json:"error,omitempty"`
}

// ParseJob handles job URL parsing and details extraction
// @Summary Parse job details from URL
// @Description Extract job information from a job posting URL
// @Tags jobs
// @Accept json
// @Produce json
// @Param request body ParseJobRequest true "Job URL to parse"
// @Success 200 {object} ParseJobResponse
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /api/jobs/parse [post]
func (jc *JobController) ParseJob(c *gin.Context) {
	var req ParseJobRequest
	
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestError(c, "Invalid request format", err)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Detect platform first
	platform, err := jc.jobService.DetectPlatform(req.URL)
	if err != nil {
		c.JSON(http.StatusOK, ParseJobResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Parse job details
	jobDetails, err := jc.jobService.ParseJobDetails(ctx, req.URL)
	if err != nil {
		c.JSON(http.StatusOK, ParseJobResponse{
			Success:  false,
			Platform: platform,
			Error:    err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ParseJobResponse{
		Success:    true,
		JobDetails: jobDetails,
		Platform:   platform,
	})
}

// SubmitApplication handles automated job application submission
// @Summary Submit job application automatically
// @Description Automate job application submission using provided data
// @Tags jobs
// @Accept json
// @Produce json
// @Param request body SubmitApplicationRequest true "Application data"
// @Success 200 {object} SubmitApplicationResponse
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /api/jobs/submit [post]
func (jc *JobController) SubmitApplication(c *gin.Context) {
	var req SubmitApplicationRequest
	
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestError(c, "Invalid request format", err)
		return
	}

	// Create context with longer timeout for application submission
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Submit application
	result, err := jc.jobService.SubmitApplication(ctx, req.ApplicationData)
	if err != nil {
		utils.InternalServerError(c, "Application submission failed", err)
		return
	}

	c.JSON(http.StatusOK, SubmitApplicationResponse{
		Success: result.Success,
		Result:  result,
	})
}

// GetSupportedPlatforms returns list of supported job platforms
// @Summary Get supported job platforms
// @Description Retrieve list of platforms that support automated applications
// @Tags jobs
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/jobs/platforms [get]
func (jc *JobController) GetSupportedPlatforms(c *gin.Context) {
	platforms := []map[string]interface{}{
		{
			"name":            "LinkedIn",
			"domain":          "linkedin.com",
			"icon":            "💼",
			"automationLevel": "partial",
			"supportsAuto":    true,
			"requiresManual":  false,
		},
		{
			"name":            "Indeed",
			"domain":          "indeed.com",
			"icon":            "🔍",
			"automationLevel": "full",
			"supportsAuto":    true,
			"requiresManual":  false,
		},
		{
			"name":            "Glassdoor",
			"domain":          "glassdoor.com",
			"icon":            "🏢",
			"automationLevel": "partial",
			"supportsAuto":    true,
			"requiresManual":  false,
		},
		{
			"name":            "AngelList",
			"domain":          "angel.co",
			"icon":            "🚀",
			"automationLevel": "partial",
			"supportsAuto":    true,
			"requiresManual":  false,
		},
		{
			"name":            "Greenhouse ATS",
			"domain":          "Company career pages",
			"icon":            "🌱",
			"automationLevel": "limited",
			"supportsAuto":    true,
			"requiresManual":  true,
		},
		{
			"name":            "Workday ATS",
			"domain":          "*.myworkdayjobs.com",
			"icon":            "📊",
			"automationLevel": "limited",
			"supportsAuto":    true,
			"requiresManual":  true,
		},
		{
			"name":            "Lever ATS",
			"domain":          "jobs.lever.co",
			"icon":            "⚡",
			"automationLevel": "limited",
			"supportsAuto":    true,
			"requiresManual":  true,
		},
		{
			"name":            "Company Careers",
			"domain":          "Various domains",
			"icon":            "🏢",
			"automationLevel": "basic",
			"supportsAuto":    false,
			"requiresManual":  true,
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"platforms": platforms,
		"total":     len(platforms),
	})
}

// CheckApplicationStatus checks the status of a submitted application
// @Summary Check application status
// @Description Check the current status of a submitted job application
// @Tags jobs
// @Produce json
// @Param applicationId path string true "Application ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} utils.ErrorResponse
// @Router /api/jobs/applications/{applicationId}/status [get]
func (jc *JobController) CheckApplicationStatus(c *gin.Context) {
	applicationID := c.Param("applicationId")
	
	if applicationID == "" {
		utils.BadRequestError(c, "Application ID is required", nil)
		return
	}

	// Mock status check - in production this would query a database
	// or check with the external platform
	status := map[string]interface{}{
		"applicationId": applicationID,
		"status":        "submitted",
		"submittedAt":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"lastUpdated":   time.Now().Format(time.RFC3339),
		"platform":      "Greenhouse ATS",
		"updates": []map[string]string{
			{
				"timestamp": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				"status":    "submitted",
				"message":   "Application successfully submitted",
			},
		},
		"nextSteps": []string{
			"Check your email for confirmation",
			"Monitor application status on company portal",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
	})
}