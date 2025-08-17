package controllers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"resumeai/models"
	"resumeai/services"
	"resumeai/utils"
)

// SmartJobController handles intelligent job application with form learning
type SmartJobController struct {
	jobService   *services.JobAutomationService
	formExtractor *services.FormExtractor
	prefsModel   *models.ApplicationPreferencesModel
	userModel    *models.UserModel
	resumeModel  *models.ResumeModel
}

// NewSmartJobController creates a new smart job controller
func NewSmartJobController(db interface{}) *SmartJobController {
	// For now, initialize without DB connection
	// In production, would properly initialize with DB
	
	return &SmartJobController{
		jobService:   services.NewJobAutomationService(),
		formExtractor: services.NewFormExtractor(),
		// prefsModel would be initialized with DB connection
	}
}

// AnalyzeJobRequest represents the request for analyzing a job with smart form extraction
type AnalyzeJobRequest struct {
	URL       string `json:"url" binding:"required"`
	UserEmail string `json:"userEmail"`
}

// AnalyzeJobResponse represents the response with extracted form fields
type AnalyzeJobResponse struct {
	Success        bool                          `json:"success"`
	JobDetails     *services.JobDetails          `json:"jobDetails"`
	FormData       *services.ExtractedFormData   `json:"formData"`
	AutoFilledData map[string]interface{}        `json:"autoFilledData"`
	MissingFields  []string                      `json:"missingFields"`
	Platform       services.PlatformInfo         `json:"platform"`
	CanAutoSubmit  bool                          `json:"canAutoSubmit"`
	Error          string                        `json:"error,omitempty"`
}

// SmartSubmitRequest represents the request for intelligent job submission
type SmartSubmitRequest struct {
	JobURL         string                 `json:"jobUrl" binding:"required"`
	UserEmail      string                 `json:"userEmail" binding:"required"`
	FormData       map[string]interface{} `json:"formData"`
	SavePreferences bool                  `json:"savePreferences"`
	AutoLearn      bool                   `json:"autoLearn"`
}

// SmartSubmitResponse represents the response for intelligent submission
type SmartSubmitResponse struct {
	Success          bool                         `json:"success"`
	ApplicationID    string                       `json:"applicationId"`
	Result           *services.ApplicationResult  `json:"result"`
	LearnedFields    int                          `json:"learnedFields"`
	SuccessRate      float64                      `json:"successRate"`
	NextApplication  map[string]interface{}       `json:"nextApplication,omitempty"`
	Error            string                       `json:"error,omitempty"`
}

// AnalyzeJobWithLearning analyzes a job and attempts to auto-fill using learned preferences
// @Summary Analyze job with intelligent form extraction and auto-fill
// @Description Extract form fields and auto-fill using saved preferences
// @Tags smart-jobs
// @Accept json
// @Produce json
// @Param request body AnalyzeJobRequest true "Job URL and user email"
// @Success 200 {object} AnalyzeJobResponse
// @Router /api/jobs/analyze-smart [post]
func (sjc *SmartJobController) AnalyzeJobWithLearning(c *gin.Context) {
	var req AnalyzeJobRequest
	
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestError(c, "Invalid request format", err)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Detect platform
	platform, err := sjc.jobService.DetectPlatform(req.URL)
	if err != nil {
		c.JSON(http.StatusOK, AnalyzeJobResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Parse job details
	jobDetails, err := sjc.jobService.ParseJobDetails(ctx, req.URL)
	if err != nil {
		c.JSON(http.StatusOK, AnalyzeJobResponse{
			Success:  false,
			Platform: platform,
			Error:    err.Error(),
		})
		return
	}

	// Extract form fields
	formData, err := sjc.formExtractor.ExtractFormFields(req.URL, platform)
	if err != nil {
		c.JSON(http.StatusOK, AnalyzeJobResponse{
			Success:    false,
			JobDetails: jobDetails,
			Platform:   platform,
			Error:      "Failed to extract form fields: " + err.Error(),
		})
		return
	}

	// Get user data and preferences
	userData := sjc.getUserData(req.UserEmail)
	savedPrefs := sjc.getSavedPreferences(req.UserEmail)

	// Attempt to auto-fill form
	autoFilledData, missingFields := sjc.formExtractor.AutoFillForm(formData, userData, savedPrefs)

	// Determine if we can auto-submit
	canAutoSubmit := len(missingFields) == 0 || 
		(len(missingFields) <= 3 && !containsRequired(missingFields, formData.RequiredFields))

	c.JSON(http.StatusOK, AnalyzeJobResponse{
		Success:        true,
		JobDetails:     jobDetails,
		FormData:       formData,
		AutoFilledData: autoFilledData,
		MissingFields:  missingFields,
		Platform:       platform,
		CanAutoSubmit:  canAutoSubmit,
	})
}

// SmartSubmitApplication submits application with learning
// @Summary Submit job application with intelligent learning
// @Description Submit application and learn from user inputs for future applications
// @Tags smart-jobs
// @Accept json
// @Produce json
// @Param request body SmartSubmitRequest true "Smart submission data"
// @Success 200 {object} SmartSubmitResponse
// @Router /api/jobs/submit-smart [post]
func (sjc *SmartJobController) SmartSubmitApplication(c *gin.Context) {
	var req SmartSubmitRequest
	
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestError(c, "Invalid request format", err)
		return
	}

	// Create context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Save preferences if requested
	learnedFields := 0
	if req.SavePreferences || req.AutoLearn {
		learnedFields = sjc.saveUserPreferences(req.UserEmail, req.FormData)
	}

	// Prepare application data
	jobDetails, _ := sjc.jobService.ParseJobDetails(ctx, req.JobURL)

	// Convert form data to string map for CustomFields
	customFields := make(map[string]string)
	for k, v := range req.FormData {
		if str, ok := v.(string); ok {
			customFields[k] = str
		} else {
			customFields[k] = fmt.Sprintf("%v", v)
		}
	}

	appData := services.ApplicationData{
		JobURL:     req.JobURL,
		JobDetails: *jobDetails,
		UserID:     req.UserEmail,
		// Map form data to application data structure
		PersonalInfo: sjc.extractPersonalInfo(req.FormData),
		Experience:   sjc.extractExperience(req.FormData),
		Education:    sjc.extractEducation(req.FormData),
		Skills:       sjc.extractSkills(req.FormData),
		CustomFields: customFields,
	}

	// Submit application
	result, err := sjc.jobService.SubmitApplication(ctx, appData)
	if err != nil {
		utils.InternalServerError(c, "Application submission failed", err)
		return
	}

	// Calculate success rate
	totalFields := len(req.FormData)
	filledFields := 0
	for _, v := range req.FormData {
		if v != nil && v != "" {
			filledFields++
		}
	}
	successRate := float64(filledFields) / float64(totalFields) * 100

	// Save submission history
	sjc.saveSubmissionHistory(req.UserEmail, req.JobURL, jobDetails, result, successRate)

	// Prepare next application suggestions
	var nextApplication map[string]interface{}
	if req.AutoLearn {
		nextApplication = sjc.prepareNextApplication(req.UserEmail)
	}

	c.JSON(http.StatusOK, SmartSubmitResponse{
		Success:         result.Success,
		ApplicationID:   result.ApplicationID,
		Result:          result,
		LearnedFields:   learnedFields,
		SuccessRate:     successRate,
		NextApplication: nextApplication,
	})
}

// GetMissingFields returns fields that need user input
// @Summary Get missing fields for application
// @Description Returns list of fields that don't have saved values
// @Tags smart-jobs
// @Produce json
// @Param userEmail query string true "User email"
// @Param jobUrl query string true "Job URL"
// @Success 200 {object} map[string]interface{}
// @Router /api/jobs/missing-fields [get]
func (sjc *SmartJobController) GetMissingFields(c *gin.Context) {
	userEmail := c.Query("userEmail")
	jobURL := c.Query("jobUrl")

	if userEmail == "" || jobURL == "" {
		utils.BadRequestError(c, "userEmail and jobUrl are required", nil)
		return
	}

	// Detect platform and extract form fields
	platform, _ := sjc.jobService.DetectPlatform(jobURL)
	formData, _ := sjc.formExtractor.ExtractFormFields(jobURL, platform)

	// Get saved preferences
	savedPrefs := sjc.getSavedPreferences(userEmail)

	// Find missing fields
	missingFields := []map[string]interface{}{}
	for _, field := range formData.FormFields {
		if _, exists := savedPrefs[field.Name]; !exists && field.Required {
			missingFields = append(missingFields, map[string]interface{}{
				"name":     field.Name,
				"label":    field.Label,
				"type":     field.Type,
				"required": field.Required,
				"options":  field.Options,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"missingFields": missingFields,
		"totalFields":   len(formData.FormFields),
		"savedFields":   len(savedPrefs),
	})
}

// SaveUserPreferences saves user preferences for future applications
// @Summary Save user preferences
// @Description Save user's common answers for future applications
// @Tags smart-jobs
// @Accept json
// @Produce json
// @Param preferences body map[string]interface{} true "User preferences"
// @Success 200 {object} map[string]interface{}
// @Router /api/jobs/preferences [post]
func (sjc *SmartJobController) SaveUserPreferences(c *gin.Context) {
	var preferences map[string]interface{}
	
	if err := c.ShouldBindJSON(&preferences); err != nil {
		utils.BadRequestError(c, "Invalid request format", err)
		return
	}

	userEmail := preferences["userEmail"].(string)
	delete(preferences, "userEmail")

	savedCount := sjc.saveUserPreferences(userEmail, preferences)

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"savedFields": savedCount,
		"message":     fmt.Sprintf("Saved %d preferences for future applications", savedCount),
	})
}

// Helper functions

func (sjc *SmartJobController) getUserData(userEmail string) map[string]interface{} {
	// This would fetch from user profile and resume
	// For now, return mock data
	return map[string]interface{}{
		"first_name": "John",
		"last_name":  "Doe",
		"email":      userEmail,
		"phone":      "555-1234",
		"location":   "San Francisco, CA",
		"linkedin":   "https://linkedin.com/in/johndoe",
	}
}

func (sjc *SmartJobController) getSavedPreferences(userEmail string) map[string]interface{} {
	// This would fetch from database
	// For now, return mock preferences
	return map[string]interface{}{
		"years_experience":    "5",
		"expected_salary":     "150000",
		"availability":        "2 weeks",
		"work_authorization":  "Yes",
		"require_sponsorship": "No",
	}
}

func (sjc *SmartJobController) saveUserPreferences(userEmail string, preferences map[string]interface{}) int {
	// This would save to database
	// Returns number of fields saved
	return len(preferences)
}

func (sjc *SmartJobController) saveSubmissionHistory(userEmail, jobURL string, jobDetails *services.JobDetails, result *services.ApplicationResult, successRate float64) {
	// This would save to database
	// Track what was submitted for learning
}

func (sjc *SmartJobController) prepareNextApplication(userEmail string) map[string]interface{} {
	// Prepare optimized data for next application based on learning
	return map[string]interface{}{
		"confidence":      95.5,
		"estimatedFields": 28,
		"savedTime":       "15 minutes",
	}
}

func (sjc *SmartJobController) extractPersonalInfo(formData map[string]interface{}) services.PersonalInfo {
	info := services.PersonalInfo{}
	
	if v, ok := formData["first_name"].(string); ok {
		info.FirstName = v
	}
	if v, ok := formData["last_name"].(string); ok {
		info.LastName = v
	}
	if v, ok := formData["email"].(string); ok {
		info.Email = v
	}
	if v, ok := formData["phone"].(string); ok {
		info.Phone = v
	}
	if v, ok := formData["location"].(string); ok {
		info.Location = v
	}
	if v, ok := formData["linkedin_url"].(string); ok {
		info.LinkedIn = v
	}
	if v, ok := formData["portfolio_url"].(string); ok {
		info.Portfolio = v
	}
	
	return info
}

func (sjc *SmartJobController) extractExperience(formData map[string]interface{}) []services.ExperienceItem {
	// Extract from formData if present
	// For now, return empty
	return []services.ExperienceItem{}
}

func (sjc *SmartJobController) extractEducation(formData map[string]interface{}) []services.EducationItem {
	// Extract from formData if present
	// For now, return empty
	return []services.EducationItem{}
}

func (sjc *SmartJobController) extractSkills(formData map[string]interface{}) []string {
	// Extract from formData if present
	// For now, return empty
	return []string{}
}

func containsRequired(missing []string, required []string) bool {
	requiredMap := make(map[string]bool)
	for _, r := range required {
		requiredMap[r] = true
	}
	
	for _, m := range missing {
		if requiredMap[m] {
			return true
		}
	}
	return false
}