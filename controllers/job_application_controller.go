package controllers

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"resumeai/models"
	"resumeai/services"
)

type JobApplicationController struct {
	JobApplicationModel     *models.JobApplicationModel
	ResumeModel            *models.ResumeModel
	ResumeHistoryModel     *models.ResumeHistoryModel
	JobAutomationService   *services.JobAutomationService
	UserProfileExtractor   *services.UserProfileExtractor
}

func NewJobApplicationController(db *sql.DB) *JobApplicationController {
	userModel := models.NewUserModel(db)
	resumeModel := models.NewResumeModel(db)
	resumeHistoryModel := models.NewResumeHistoryModel(db)
	experienceModel := models.NewExperienceModel(db)
	educationModel := models.NewEducationModel(db)
	
	resumeDataExtractor := services.NewResumeDataExtractor(userModel, resumeModel, experienceModel, educationModel)
	automationService := services.NewJobAutomationService(resumeDataExtractor)
	userProfileExtractor := services.NewUserProfileExtractor(db)
	
	return &JobApplicationController{
		JobApplicationModel:  models.NewJobApplicationModel(db),
		ResumeModel:         resumeModel,
		ResumeHistoryModel:  resumeHistoryModel,
		JobAutomationService: automationService,
		UserProfileExtractor: userProfileExtractor,
	}
}

type CreateJobApplicationRequest struct {
	ResumeID      int    `json:"resume_id"`
	JobURL        string `json:"job_url"`
	// Remove the following fields - will be extracted automatically
	// CompanyName   string `json:"company_name"`
	// PositionTitle string `json:"position_title"`
	// Notes         string `json:"notes"`
}

type JobProfileRequest struct {
	FullName             string `json:"full_name"`
	Email                string `json:"email"`
	PhoneNumber          string `json:"phone_number"`
	Country              string `json:"country"`
	City                 string `json:"city"`
	State                string `json:"state"`
	ZipCode              string `json:"zip_code"`
	Address              string `json:"address"`
	LinkedInURL          string `json:"linkedin_url"`
	PortfolioURL         string `json:"portfolio_url"`
	WorkAuthorization    string `json:"work_authorization"`    // "yes", "no", "requires_sponsorship"
	RequiresSponsorship  bool   `json:"requires_sponsorship"`
	WillingToRelocate    bool   `json:"willing_to_relocate"`
	SalaryExpectationMin int    `json:"salary_expectation_min"`
	SalaryExpectationMax int    `json:"salary_expectation_max"`
	PreferredLocations   string `json:"preferred_locations"`
	AvailableStartDate   string `json:"available_start_date"`  // "immediately", "2_weeks", "1_month"
	YearsOfExperience    int    `json:"years_of_experience"`
	Gender               string `json:"gender"`
	Ethnicity            string `json:"ethnicity"`
	VeteranStatus        string `json:"veteran_status"`
	DisabilityStatus     string `json:"disability_status"`
	SexualOrientation    string `json:"sexual_orientation"`
	TransgenderStatus    string `json:"transgender_status"`
	MostRecentDegree     string `json:"most_recent_degree"`
	GraduationYear       int    `json:"graduation_year"`
	University           string `json:"university"`
	Major                string `json:"major"`
}

func (c *JobApplicationController) CreateApplication(ctx *gin.Context) {
	userIDInterface, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID := userIDInterface.(int)
	
	var req CreateJobApplicationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.JobURL == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Job URL is required"})
		return
	}

	if req.ResumeID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Resume ID is required"})
		return
	}

	// Since we're now using resume history IDs, we need to get the resume history entry
	resumeHistories, err := c.ResumeHistoryModel.GetByUserID(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch resume history"})
		return
	}

	// Find the selected resume history entry
	var selectedResumeHistory *models.ResumeHistory
	for _, rh := range resumeHistories {
		if rh.ID == req.ResumeID {
			selectedResumeHistory = rh
			break
		}
	}

	if selectedResumeHistory == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Resume not found"})
		return
	}

	// Extract company and position from job URL automatically
	companyName, positionTitle := c.extractJobDetailsFromURL(req.JobURL)

	// Create job application with extracted data
	// Note: Using resume_history.id directly since foreign key constraint was removed
	notes := "Applied via one-click automation with resume: " + selectedResumeHistory.ResumeName
	
	createdApplication, err := c.JobApplicationModel.Create(
		userID,
		selectedResumeHistory.ID, // This is from resume_history table
		req.JobURL,
		companyName,
		positionTitle,
		notes,
	)
	if err != nil {
		// Log detailed error information for debugging
		fmt.Printf("ERROR creating job application: %v\n", err)
		fmt.Printf("DEBUG INFO - UserID: %d, ResumeHistoryID: %d, JobURL: %s, Company: %s, Position: %s\n", 
			userID, selectedResumeHistory.ID, req.JobURL, companyName, positionTitle)
		
		// If there's still a constraint issue, return a detailed error for debugging
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create job application",
			"details": err.Error(),
			"debug_info": gin.H{
				"user_id": userID,
				"resume_history_id": selectedResumeHistory.ID,
				"company": companyName,
				"position": positionTitle,
			},
		})
		return
	}

	// Check if user has complete job profile before automation
	// Pass the resume ID so we can extract data from the actual resume being used
	userProfile, err := c.UserProfileExtractor.ExtractUserProfileWithResume(userID, selectedResumeHistory.ID)
	if err != nil {
		fmt.Printf("Failed to extract user profile: %v\n", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to extract user profile"})
		return
	}

	// Validate required job application information
	missingFields := validateJobProfile(userProfile)
	if len(missingFields) > 0 {
		ctx.JSON(http.StatusPreconditionFailed, gin.H{
			"error": "missing_job_profile_info",
			"message": "Please complete your job application profile before applying",
			"missing_fields": missingFields,
			"application": createdApplication,
			"setup_required": true,
		})
		return
	}

	// Start browser automation
	
	// Initialize browser automation service (using V2 - refactored version)
	automationService, err := services.NewBrowserAutomationServiceV2()
	if err != nil {
		fmt.Printf("Failed to initialize browser automation: %v\n", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "automation_init_failed",
			"message": "Failed to initialize browser automation",
			"application": createdApplication,
		})
		return
	}
	defer automationService.Close()
	// Download resume from S3 to temporary file
	resumeFilePath := ""
	if selectedResumeHistory != nil && selectedResumeHistory.S3Path != "" {
		tempResumePath, err := c.downloadResumeFromS3(selectedResumeHistory.S3Path)
		if err != nil {
			fmt.Printf("Failed to download resume from S3: %v\n", err)
		} else {
			resumeFilePath = tempResumePath
			defer os.Remove(tempResumePath) // Clean up temp file
		}
	}
	
	// Perform browser automation SYNCHRONOUSLY
	automationResult, err := automationService.SubmitJobApplication(req.JobURL, userProfile, resumeFilePath)
	if err != nil {
		fmt.Printf("Browser automation failed: %v\n", err)
		
		// Check if this is a MissingFieldsError
		if missingErr, ok := err.(*services.MissingFieldsError); ok {
			// Debug: Log what fields are being sent to frontend
			log.Printf("=== SENDING MISSING FIELDS TO FRONTEND ===")
			log.Printf("Total missing fields: %d", len(missingErr.Fields))
			for i, field := range missingErr.Fields {
				log.Printf("  Field %d: '%s' (type: %s, options: %d)", 
					i+1, field.FieldName, field.FieldType, len(field.Options))
				if strings.Contains(strings.ToLower(field.FieldName), "racial") || 
				   strings.Contains(strings.ToLower(field.FieldName), "ethnic") {
					log.Printf("    WARNING: Racial/ethnic field should not be here!")
				}
			}
			
			// Return the missing fields to frontend so it can show a popup
			c.JobApplicationModel.UpdateStatus(createdApplication.ID, "missing_fields")
			
			ctx.JSON(http.StatusOK, gin.H{
				"success": false,
				"status": "missing_fields",
				"message": "Additional information required",
				"missing_fields": missingErr.Fields,
				"application": createdApplication,
			})
			return
		}
		
		// Other error - update status to failed
		c.JobApplicationModel.UpdateStatus(createdApplication.ID, "automation_failed")
		
		ctx.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "❌ Job application automation failed",
			"application": createdApplication,
			"error_details": err.Error(),
			"status": "automation_failed",
		})
		return
	}
	
	// Update application status based on automation result
	var newStatus string
	var message string
	var success bool
	
	if automationResult.Success {
		newStatus = "submitted"
		message = "✅ Job application submitted successfully!"
		success = true
	} else {
		success = false
		switch automationResult.Status {
		case "login_required":
			newStatus = "login_required"
			message = "🔐 Please log in to the job site first, then try again"
		case "external_application":
			newStatus = "external_application"
			message = "🔗 This job requires manual application on the company website"
		case "missing_fields":
			newStatus = "incomplete"
			message = "📝 Some required fields could not be filled automatically"
		default:
			newStatus = "automation_failed"
			message = "❌ Automation failed - please apply manually"
		}
	}
	
	// Update the application status in database
	err = c.JobApplicationModel.UpdateStatus(createdApplication.ID, newStatus)
	if err != nil {
		fmt.Printf("Failed to update application status: %v\n", err)
	}
	
	// Save screenshot keys if available
	if automationResult.ApplicationScreenshotKey != "" {
		err = c.JobApplicationModel.UpdateScreenshots(createdApplication.ID, "", automationResult.ApplicationScreenshotKey)
		if err != nil {
			fmt.Printf("Failed to update application screenshots: %v\n", err)
		}
	}
	
	// Browser automation completed
	
	// Generate S3 pre-signed URLs directly for screenshots
	screenshotURLs := gin.H{}
	s3Service, _ := services.NewS3Service()
	if s3Service != nil {
		if automationResult.JobPageScreenshotKey != "" {
			if url, err := s3Service.GeneratePresignedURL(automationResult.JobPageScreenshotKey); err == nil {
				screenshotURLs["job_page"] = url
				fmt.Printf("Generated pre-signed URL for job page screenshot: %s\n", url)
			}
		}
		if automationResult.ApplicationScreenshotKey != "" {
			if url, err := s3Service.GeneratePresignedURL(automationResult.ApplicationScreenshotKey); err == nil {
				screenshotURLs["application"] = url
				fmt.Printf("Generated pre-signed URL for application screenshot: %s\n", url)
			}
		}
		
		// Also update the Screenshots array with proper S3 URLs
		properScreenshots := []string{}
		for _, key := range automationResult.Screenshots {
			// If it's an S3 key, generate presigned URL
			if strings.HasPrefix(key, "screenshots/") || !strings.HasPrefix(key, "http") {
				if url, err := s3Service.GeneratePresignedURL(key); err == nil {
					properScreenshots = append(properScreenshots, url)
				}
			} else {
				properScreenshots = append(properScreenshots, key)
			}
		}
		automationResult.Screenshots = properScreenshots
	}
	
	// Return immediate result
	ctx.JSON(http.StatusOK, gin.H{
		"success": success,
		"message": message,
		"application": createdApplication,
		"automation_result": automationResult,
		"status": newStatus,
		"screenshot_urls": screenshotURLs,
		"details": gin.H{
			"selected_resume": selectedResumeHistory.ResumeName,
			"job_url":         req.JobURL,
			"extracted_company": companyName,
			"extracted_position": positionTitle,
		},
	})
}

func (c *JobApplicationController) GetUserApplications(ctx *gin.Context) {
	userIDInterface, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID := userIDInterface.(int)
	
	// Parse pagination parameters
	limitStr := ctx.Query("limit")
	offsetStr := ctx.Query("offset")
	
	limit := 20 // default
	offset := 0 // default
	
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	
	if offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	applications, err := c.JobApplicationModel.GetByUserID(userID, limit, offset)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch job applications"})
		return
	}

	// Convert S3 keys to API URLs for screenshots
	for i := range applications {
		applications[i].JobPageScreenshotURL = c.convertS3KeyToAPIURL(ctx, applications[i].JobPageScreenshotURL)
		applications[i].ApplicationScreenshotURL = c.convertS3KeyToAPIURL(ctx, applications[i].ApplicationScreenshotURL)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"applications": applications,
		"limit":        limit,
		"offset":       offset,
	})
}

func (c *JobApplicationController) GetApplication(ctx *gin.Context) {
	userIDInterface, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID := userIDInterface.(int)
	
	applicationID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid application ID"})
		return
	}

	application, err := c.JobApplicationModel.GetByID(applicationID)
	if err != nil {
		if err == sql.ErrNoRows {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch application"})
		return
	}

	// Ensure the application belongs to the user
	if application.UserID != userID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Application does not belong to user"})
		return
	}

	// Convert S3 keys to API URLs for screenshots
// 	application.JobPageScreenshotURL = c.convertS3KeyToAPIURL(ctx, application.JobPageScreenshotURL)
	application.ApplicationScreenshotURL = c.convertS3KeyToAPIURL(ctx, application.ApplicationScreenshotURL)

	ctx.JSON(http.StatusOK, gin.H{
		"application": application,
	})
}

type UpdateApplicationStatusRequest struct {
	Status string `json:"status"`
}

func (c *JobApplicationController) UpdateApplicationStatus(ctx *gin.Context) {
	userIDInterface, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID := userIDInterface.(int)
	
	applicationID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid application ID"})
		return
	}

	var req UpdateApplicationStatusRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Verify the application belongs to the user
	application, err := c.JobApplicationModel.GetByID(applicationID)
	if err != nil {
		if err == sql.ErrNoRows {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch application"})
		return
	}

	if application.UserID != userID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Application does not belong to user"})
		return
	}

	err = c.JobApplicationModel.UpdateStatus(applicationID, req.Status)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update application status"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Application status updated successfully",
	})
}

func (c *JobApplicationController) DeleteApplication(ctx *gin.Context) {
	userIDInterface, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID := userIDInterface.(int)
	
	applicationID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid application ID"})
		return
	}

	// Verify the application belongs to the user
	application, err := c.JobApplicationModel.GetByID(applicationID)
	if err != nil {
		if err == sql.ErrNoRows {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch application"})
		return
	}

	if application.UserID != userID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Application does not belong to user"})
		return
	}

	err = c.JobApplicationModel.Delete(applicationID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete application"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Application deleted successfully",
	})
}

// Resume-specific endpoints

func (c *JobApplicationController) GetJobProfile(ctx *gin.Context) {
	userIDInterface, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID := userIDInterface.(int)
	
	// Try to get existing job profile
	userJobProfileModel := models.NewUserJobProfileModel(c.JobApplicationModel.DB)
	profile, err := userJobProfileModel.GetByUserID(userID)
	if err != nil {
		// Return empty profile if none exists
		ctx.JSON(http.StatusOK, gin.H{
			"profile": nil,
			"exists": false,
			"message": "No job profile found. Please create one to enable job automation.",
		})
		return
	}
	
	ctx.JSON(http.StatusOK, gin.H{
		"profile": profile,
		"exists": true,
	})
}

func (c *JobApplicationController) SaveJobProfile(ctx *gin.Context) {
	userIDInterface, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID := userIDInterface.(int)
	
	var req JobProfileRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	
	// Validate required fields
	if req.WorkAuthorization == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Work authorization is required"})
		return
	}
	
	if req.LinkedInURL == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "LinkedIn URL is required"})
		return
	}
	
	if req.Country == "" || req.City == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Country and City are required"})
		return
	}
	
	// Create profile object
	profile := &models.UserJobProfile{
		FullName:             req.FullName,
		Email:                req.Email,
		PhoneNumber:          req.PhoneNumber,
		Country:              req.Country,
		City:                 req.City,
		State:                req.State,
		ZipCode:              req.ZipCode,
		Address:              req.Address,
		LinkedInURL:          req.LinkedInURL,
		PortfolioURL:         req.PortfolioURL,
		WorkAuthorization:    req.WorkAuthorization,
		RequiresSponsorship:  req.RequiresSponsorship,
		WillingToRelocate:    req.WillingToRelocate,
		SalaryExpectationMin: req.SalaryExpectationMin,
		SalaryExpectationMax: req.SalaryExpectationMax,
		PreferredLocations:   req.PreferredLocations,
		AvailableStartDate:   req.AvailableStartDate,
		YearsOfExperience:    req.YearsOfExperience,
		Gender:               req.Gender,
		Ethnicity:            req.Ethnicity,
		VeteranStatus:        req.VeteranStatus,
		DisabilityStatus:     req.DisabilityStatus,
		SexualOrientation:    req.SexualOrientation,
		TransgenderStatus:    req.TransgenderStatus,
		MostRecentDegree:     req.MostRecentDegree,
		GraduationYear:       req.GraduationYear,
		University:           req.University,
		Major:                req.Major,
	}
	
	// Save to database
	userJobProfileModel := models.NewUserJobProfileModel(c.JobApplicationModel.DB)
	err := userJobProfileModel.CreateOrUpdate(userID, profile)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save job profile"})
		return
	}
	
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Job profile saved successfully",
		"profile": profile,
	})
}

func (c *JobApplicationController) GetUserRecentResumes(ctx *gin.Context) {
	userIDInterface, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID := userIDInterface.(int)
	
	// Get last 3 resume history entries for the user (these are the actual generated resumes)
	resumeHistories, err := c.ResumeHistoryModel.GetByUserID(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch resume history"})
		return
	}

	// Limit to 3 most recent
	if len(resumeHistories) > 3 {
		resumeHistories = resumeHistories[:3]
	}

	// Simplify response - only send necessary fields
	type SimpleResume struct {
		ID        int       `json:"id"`
		Name      string    `json:"name"`
		UpdatedAt string    `json:"updated_at"`
	}

	simpleResumes := make([]SimpleResume, len(resumeHistories))
	for i, resume := range resumeHistories {
		simpleResumes[i] = SimpleResume{
			ID:        resume.ID,
			Name:      resume.ResumeName,
			UpdatedAt: resume.GeneratedAt.Format("2006-01-02 15:04"),
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"resumes": simpleResumes,
	})
}

// Automation-specific endpoints

type ContinueApplicationRequest struct {
	ExtraQA   map[string]string                  `json:"extra_qa"`
	FieldInfo map[string]models.FieldAnswer     `json:"field_info,omitempty"` // New format with types
}

// ContinueApplication handles continuing a paused application with user-provided answers
func (c *JobApplicationController) ContinueApplication(ctx *gin.Context) {
	applicationCode := ctx.Param("code")
	if applicationCode == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid application code"})
		return
	}

	userIDInterface, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID := userIDInterface.(int)

	// Get the application by code to verify ownership
	application, err := c.JobApplicationModel.GetByApplicationCode(applicationCode)
	if err != nil {
		if err == sql.ErrNoRows {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch application"})
		return
	}

	// Verify the application belongs to the user
	if application.UserID != userID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	var req ContinueApplicationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Save the extra Q&A to the job profile for future use
	userJobProfileModel := models.NewUserJobProfileModel(c.JobApplicationModel.DB)
	profile, err := userJobProfileModel.GetByUserID(userID)
	
	// Track which questions are handled by specific profile fields
	handledByProfileFields := make(map[string]bool)
	qaToSaveWithTypes := make(map[string]models.FieldAnswer)
	
	// Check if we have the new format with field types
	if req.FieldInfo != nil && len(req.FieldInfo) > 0 {
		// Use new format with field types
		log.Printf("Using new format with field types for %d fields", len(req.FieldInfo))
		
		if err == nil && profile != nil {
			// Update profile with commonly asked questions if they match known fields
			updated := false
			for question, fieldAnswer := range req.FieldInfo {
				questionLower := strings.ToLower(question)
				fieldHandled := false
				
				// Check if this is a sexual orientation question
				if strings.Contains(questionLower, "sexual orientation") && profile.SexualOrientation == "" {
					profile.SexualOrientation = fieldAnswer.Answer
					updated = true
					fieldHandled = true
				}
				// Check if this is a transgender question
				if strings.Contains(questionLower, "transgender") && profile.TransgenderStatus == "" {
					profile.TransgenderStatus = fieldAnswer.Answer
					updated = true
					fieldHandled = true
				}
				// Check if this is a degree question
				if strings.Contains(questionLower, "degree") && profile.MostRecentDegree == "" {
					profile.MostRecentDegree = fieldAnswer.Answer
					updated = true
					fieldHandled = true
				}
				// Check if this is a university question
				if strings.Contains(questionLower, "university") || strings.Contains(questionLower, "college") {
					if profile.University == "" {
						profile.University = fieldAnswer.Answer
						updated = true
						fieldHandled = true
					}
				}
				// Check if this is a major question
				if strings.Contains(questionLower, "major") || strings.Contains(questionLower, "field of study") {
					if profile.Major == "" {
						profile.Major = fieldAnswer.Answer
						updated = true
						fieldHandled = true
					}
				}
				
				// Only save to ExtraQA if not handled by a specific profile field
				if !fieldHandled {
					qaToSaveWithTypes[question] = fieldAnswer
					handledByProfileFields[question] = false
				} else {
					handledByProfileFields[question] = true
				}
			}
			
			if updated {
				// Save the updated profile
				userJobProfileModel.CreateOrUpdate(userID, profile)
			}
		} else {
			// No profile exists or error getting it, save all Q&A
			qaToSaveWithTypes = req.FieldInfo
		}
		
		// Save unhandled questions to ExtraQA with types
		if len(qaToSaveWithTypes) > 0 {
			log.Printf("Saving %d unresolved Q&A pairs with types to user profile ExtraQA", len(qaToSaveWithTypes))
			for q, fa := range qaToSaveWithTypes {
				log.Printf("  Q: %s => A: %s (type: %s)", q, fa.Answer, fa.FieldType)
			}
			err = userJobProfileModel.UpdateExtraQAWithTypes(userID, qaToSaveWithTypes)
			if err != nil {
				log.Printf("Failed to update ExtraQA with types: %v", err)
			}
		}
	} else {
		// Fall back to old format without types
		qaToSave := make(map[string]string)
		
		if err == nil && profile != nil {
			// Update profile with commonly asked questions if they match known fields
			updated := false
			for question, answer := range req.ExtraQA {
				questionLower := strings.ToLower(question)
				fieldHandled := false
				
				// Check if this is a sexual orientation question
				if strings.Contains(questionLower, "sexual orientation") && profile.SexualOrientation == "" {
					profile.SexualOrientation = answer
					updated = true
					fieldHandled = true
				}
				// Check if this is a transgender question
				if strings.Contains(questionLower, "transgender") && profile.TransgenderStatus == "" {
					profile.TransgenderStatus = answer
					updated = true
					fieldHandled = true
				}
				// Check if this is a degree question
				if strings.Contains(questionLower, "degree") && profile.MostRecentDegree == "" {
					profile.MostRecentDegree = answer
					updated = true
					fieldHandled = true
				}
				// Check if this is a university question
				if strings.Contains(questionLower, "university") || strings.Contains(questionLower, "college") {
					if profile.University == "" {
						profile.University = answer
						updated = true
						fieldHandled = true
					}
				}
				// Check if this is a major question
				if strings.Contains(questionLower, "major") || strings.Contains(questionLower, "field of study") {
					if profile.Major == "" {
						profile.Major = answer
						updated = true
						fieldHandled = true
					}
				}
				
				// Only save to ExtraQA if not handled by a specific profile field
				if !fieldHandled {
					qaToSave[question] = answer
					handledByProfileFields[question] = false
				} else {
					handledByProfileFields[question] = true
				}
			}
			
			if updated {
				// Save the updated profile
				userJobProfileModel.CreateOrUpdate(userID, profile)
			}
		} else {
			// No profile exists or error getting it, save all Q&A
			qaToSave = req.ExtraQA
		}
		
		// Save unhandled questions to ExtraQA for future reuse
		if len(qaToSave) > 0 {
			log.Printf("Saving %d unresolved Q&A pairs to user profile ExtraQA", len(qaToSave))
			for q, a := range qaToSave {
				log.Printf("  Q: %s => A: %s", q, a)
			}
			err = userJobProfileModel.UpdateExtraQA(userID, qaToSave)
			if err != nil {
				log.Printf("Failed to update ExtraQA: %v", err)
			}
		}
	}

	// Get user profile data with the new answers
	userProfile, err := c.UserProfileExtractor.ExtractUserProfileWithResume(userID, application.ResumeID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to extract user profile"})
		return
	}
	
	// Add the extra Q&A to the user profile
	if userProfile.ExtraQA == nil {
		userProfile.ExtraQA = make(map[string]string)
	}
	for k, v := range req.ExtraQA {
		userProfile.ExtraQA[k] = v
	}

	// Get the resume file path if available
	resumeFilePath := ""
	resumeHistory, err := c.ResumeHistoryModel.GetByID(application.ResumeID)
	if err == nil && resumeHistory.S3Path != "" {
		tempResumePath, err := c.downloadResumeFromS3(resumeHistory.S3Path)
		if err == nil {
			resumeFilePath = tempResumePath
			defer os.Remove(tempResumePath)
		}
	}

	// Resume automation with the updated profile
	// Using V2 service
	automationService, err := services.NewBrowserAutomationServiceV2()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize automation service"})
		return
	}
	defer automationService.Close()

	// Continue the application
	automationResult, err := automationService.SubmitJobApplication(application.JobURL, userProfile, resumeFilePath)
	if err != nil {
		// Check if this is another MissingFieldsError
		if missingErr, ok := err.(*services.MissingFieldsError); ok {
			// More fields still needed
			ctx.JSON(http.StatusOK, gin.H{
				"success": false,
				"status": "missing_fields",
				"message": "Additional information required",
				"missing_fields": missingErr.Fields,
				"application": application,
				"automation_result": automationResult,
			})
			return
		}
		
		// Update screenshots if available even on failure
		if automationResult.ApplicationScreenshotKey != "" {
			err := c.JobApplicationModel.UpdateScreenshots(application.ID, "", automationResult.ApplicationScreenshotKey)
			if err != nil {
				fmt.Printf("Failed to update application screenshots: %v\n", err)
			}
		}
		
		c.JobApplicationModel.UpdateStatus(application.ID, "automation_failed")
		ctx.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Automation failed",
			"error_details": err.Error(),
			"automation_result": automationResult,
		})
		return
	}

	// Update screenshots if available
	if automationResult.ApplicationScreenshotKey != "" {
		err = c.JobApplicationModel.UpdateScreenshots(application.ID, "", automationResult.ApplicationScreenshotKey)
		if err != nil {
			fmt.Printf("Failed to update application screenshots: %v\n", err)
		}
	}
	
	// Update application status based on result
	if automationResult.Success {
		c.JobApplicationModel.UpdateStatus(application.ID, "submitted")
		ctx.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Application completed successfully!",
			"automation_result": automationResult,
		})
	} else if automationResult.Status == "user_input_required" {
		// Still more fields needed
		ctx.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": automationResult.Message,
			"automation_result": automationResult,
		})
	} else {
		c.JobApplicationModel.UpdateStatus(application.ID, "automation_failed")
		ctx.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Automation failed",
			"automation_result": automationResult,
		})
	}
}

type SavePreferencesRequest struct {
	JobSiteDomain string            `json:"job_site_domain"`
	Preferences   map[string]string `json:"preferences"`
}

// SaveUserPreferences saves extra Q&A responses from the frontend popup
func (c *JobApplicationController) SaveUserPreferences(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req SavePreferencesRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	// Update the user's job profile with the extra Q&A data
	query := `
		UPDATE user_job_profiles 
		SET extra_qa = extra_qa || $1::jsonb,
		    updated_at = NOW()
		WHERE user_id = $2`
	
	// Log what we're saving for debugging
	fmt.Printf("Saving preferences for user %d:\n", userID.(int))
	for key, value := range req.Preferences {
		fmt.Printf("  '%s' => '%s'\n", key, value)
	}
	
	preferencesJSON, err := json.Marshal(req.Preferences)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process preferences"})
		return
	}

	_, err = c.JobApplicationModel.DB.Exec(query, string(preferencesJSON), userID.(int))
	if err != nil {
		fmt.Printf("Failed to save user preferences: %v\n", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save preferences"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Preferences saved successfully",
	})
}

// GetUserPreferences removed - user data comes from resume only
func (c *JobApplicationController) GetUserPreferences(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"preferences": []interface{}{},
		"message": "User preferences are no longer supported. All data comes from resume.",
	})
}

func (c *JobApplicationController) GetApplicationStatus(ctx *gin.Context) {
	applicationID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid application ID"})
		return
	}

	userIDInterface, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID := userIDInterface.(int)

	// Get the application
	application, err := c.JobApplicationModel.GetByID(applicationID)
	if err != nil {
		if err == sql.ErrNoRows {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch application"})
		return
	}

	// Ensure the application belongs to the user
	if application.UserID != userID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Application does not belong to user"})
		return
	}

	// Return detailed status information
	statusInfo := map[string]string{
		"submitted":             "✅ Application successfully submitted to company",
		"login_required":        "🔐 Please log in to the job site and try again",
		"external_application":  "🔗 This job requires applying on the company website",
		"automation_failed":     "❌ Automation failed - please apply manually",
		"processing":           "⏳ Application is being processed...",
	}

	statusMessage := statusInfo[application.ApplicationStatus]
	if statusMessage == "" {
		statusMessage = "📝 Application saved - status unknown"
	}

	// Convert S3 keys to API URLs for screenshots
// 	application.JobPageScreenshotURL = c.convertS3KeyToAPIURL(ctx, application.JobPageScreenshotURL)
	application.ApplicationScreenshotURL = c.convertS3KeyToAPIURL(ctx, application.ApplicationScreenshotURL)

	ctx.JSON(http.StatusOK, gin.H{
		"application_id": applicationID,
		"status":         application.ApplicationStatus,
		"status_message": statusMessage,
		"company":        application.CompanyName,
		"position":       application.PositionTitle,
		"applied_at":     application.AppliedAt,
		"job_url":        application.JobURL,
	})
}

func (c *JobApplicationController) GetAutomationStatus(ctx *gin.Context) {
	applicationID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid application ID"})
		return
	}

	userIDInterface, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID := userIDInterface.(int)
	
	// Verify the application belongs to the user
	application, err := c.JobApplicationModel.GetByID(applicationID)
	if err != nil {
		if err == sql.ErrNoRows {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch application"})
		return
	}

	if application.UserID != userID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Application does not belong to user"})
		return
	}

	// Automation logs are no longer stored separately
	// Return application status instead
	ctx.JSON(http.StatusOK, gin.H{
		"application_id": applicationID,
		"status": application.ApplicationStatus,
		"message": "Automation logs are stored in job_applications table",
	})
}

type RetryAutomationRequest struct {
	Preferences map[string]string `json:"preferences,omitempty"`
}

func (c *JobApplicationController) RetryAutomation(ctx *gin.Context) {
	applicationID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid application ID"})
		return
	}

	userIDInterface, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID := userIDInterface.(int)
	
	var req RetryAutomationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Verify the application belongs to the user
	application, err := c.JobApplicationModel.GetByID(applicationID)
	if err != nil {
		if err == sql.ErrNoRows {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch application"})
		return
	}

	if application.UserID != userID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Application does not belong to user"})
		return
	}

	// Preferences are no longer saved - all data comes from resume

	// Retry automation
	automationResult, err := c.JobAutomationService.StartAutomation(userID, application)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retry automation"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message":           "Automation retried successfully",
		"automation_result": automationResult,
	})
}

// extractJobDetailsFromURL extracts company name and position title from job posting URLs
func (c *JobApplicationController) extractJobDetailsFromURL(jobURL string) (companyName, positionTitle string) {
	// Parse the URL to get domain and path components
	parsedURL, err := url.Parse(jobURL)
	if err != nil {
		return "Company", "Position" // Default fallback
	}

	domain := parsedURL.Hostname()
	path := parsedURL.Path

	// Extract company name from common job sites
	switch {
	case strings.Contains(domain, "linkedin.com"):
		companyName = c.extractFromLinkedIn(path)
	case strings.Contains(domain, "indeed.com"):
		companyName = c.extractFromIndeed(path)
	case strings.Contains(domain, "glassdoor.com"):
		companyName = c.extractFromGlassdoor(path)
	case strings.Contains(domain, "careers") || strings.Contains(domain, "jobs"):
		// Company career pages like jobs.google.com, careers.microsoft.com
		companyName = c.extractFromCareerPage(domain)
	default:
		// Try to extract from domain name
		companyName = c.extractFromDomain(domain)
	}

	// For position title, use a generic approach since it's harder to extract
	positionTitle = "Software Engineer" // Default fallback - could be improved with web scraping

	// Clean up extracted values
	if companyName == "" {
		companyName = "Company"
	}
	if positionTitle == "" {
		positionTitle = "Position"
	}

	return companyName, positionTitle
}

func (c *JobApplicationController) extractFromLinkedIn(path string) string {
	// LinkedIn URLs typically have company name in path like /company/microsoft/jobs/
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "company" && i+1 < len(parts) {
			return c.cleanCompanyName(parts[i+1])
		}
	}
	return ""
}

func (c *JobApplicationController) extractFromIndeed(path string) string {
	// Indeed URLs often have company info in query params, for now just return generic
	return "Company"
}

func (c *JobApplicationController) extractFromGlassdoor(path string) string {
	// Glassdoor has company info in various formats
	return "Company"
}

func (c *JobApplicationController) extractFromCareerPage(domain string) string {
	// Extract from domain like careers.microsoft.com, jobs.google.com
	parts := strings.Split(domain, ".")
	for _, part := range parts {
		if part != "careers" && part != "jobs" && part != "com" && part != "www" && len(part) > 2 {
			return c.cleanCompanyName(part)
		}
	}
	return ""
}

func (c *JobApplicationController) extractFromDomain(domain string) string {
	// Extract company name from domain like microsoft.com
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		companyPart := parts[len(parts)-2] // Get the part before .com
		if companyPart != "www" && len(companyPart) > 2 {
			return c.cleanCompanyName(companyPart)
		}
	}
	return ""
}

// validateJobProfile checks if user has all required information for job applications
func validateJobProfile(profile *services.UserProfileData) []string {
	missingFields := []string{}
	
	// Essential fields that we absolutely need
	if profile.FullName == "" || profile.FullName == "John Doe" {
		missingFields = append(missingFields, "full_name")
	}
	
	if profile.Email == "" || profile.Email == "test@example.com" {
		missingFields = append(missingFields, "email")
	}
	
	if profile.Phone == "" || profile.Phone == "(555) 123-4567" {
		missingFields = append(missingFields, "phone_number")
	}
	
	// Location information (often required)
	if profile.City == "" {
		missingFields = append(missingFields, "city")
	}
	
	if profile.State == "" && profile.Country == "United States" {
		missingFields = append(missingFields, "state")
	}
	
	if profile.Country == "" {
		missingFields = append(missingFields, "country")
	}
	
	// Professional information
	if profile.LinkedIn == "" {
		missingFields = append(missingFields, "linkedin_url")
	}
	
	if profile.WorkAuthorization == "" {
		missingFields = append(missingFields, "work_authorization")
	}
	
	// Check if years of experience is set (0 might be valid for entry-level)
	if profile.YearsOfExperience < 0 {
		missingFields = append(missingFields, "years_of_experience")
	}
	
	return missingFields
}

func (c *JobApplicationController) cleanCompanyName(name string) string {
	// Clean and format company name
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	
	// Capitalize first letter of each word
	words := strings.Fields(name)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}
// convertS3KeyToAPIURL converts an S3 key to a proper API URL for screenshots
func (c *JobApplicationController) convertS3KeyToAPIURL(ctx *gin.Context, s3Key string) string {
	if s3Key == "" {
		return ""
	}
	
	// If it's already a full URL, return as is (but not localhost:3000)
	if strings.HasPrefix(s3Key, "http://") || strings.HasPrefix(s3Key, "https://") {
		// Check if it's a localhost:3000 URL (incorrect) and fix it
		if strings.Contains(s3Key, "localhost:3000") {
			// Extract the key and rebuild with correct API URL
			parts := strings.Split(s3Key, "/screenshots/")
			if len(parts) > 1 {
				s3Key = "screenshots/" + parts[1]
			}
		} else {
			// It's already a valid URL
			return s3Key
		}
	}
	
	// Build the API URL
	scheme := "http"
	if ctx.Request.TLS != nil || ctx.Request.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	
	host := ctx.Request.Host
	// Handle production domain
	if host == "www.hihired.org" {
		host = "hihired.org"
	}
	
	// Clean up the S3 key
	s3Key = strings.TrimPrefix(s3Key, "/")
	if !strings.HasPrefix(s3Key, "screenshots/") {
		s3Key = "screenshots/" + s3Key
	}
	
	return fmt.Sprintf("%s://%s/api/%s", scheme, host, s3Key)
}

// downloadResumeFromS3 downloads a resume from S3 to a temporary file
func (c *JobApplicationController) downloadResumeFromS3(s3Path string) (string, error) {
	if s3Path == "" {
		return "", fmt.Errorf("S3 path is empty")
	}
	
	// Create S3 service
	s3Service, err := services.NewS3Service()
	if err != nil {
		return "", fmt.Errorf("failed to create S3 service: %v", err)
	}
	
	// Generate presigned URL
	presignedURL, err := s3Service.GeneratePresignedURL(s3Path)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %v", err)
	}
	
	// Download the file
	resp, err := http.Get(presignedURL)
	if err != nil {
		return "", fmt.Errorf("failed to download resume: %v", err)
	}
	defer resp.Body.Close()
	
	// Create temporary file
	tempFile, err := os.CreateTemp("", "resume_*.pdf")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	
	// Copy content to temp file
	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to save resume: %v", err)
	}
	
	tempFile.Close()
	return tempFile.Name(), nil
}
