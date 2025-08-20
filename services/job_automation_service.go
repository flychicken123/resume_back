package services

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"resumeai/models"
)

type JobAutomationService struct {
	resumeDataExtractor     *ResumeDataExtractor
}

type AutomationStep struct {
	Name        string      `json:"name"`
	Status      string      `json:"status"`
	Data        interface{} `json:"data"`
	ErrorMsg    string      `json:"error_message,omitempty"`
	Timestamp   time.Time   `json:"timestamp"`
}

type FormField struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Required     bool     `json:"required"`
	Options      []string `json:"options,omitempty"`
	CurrentValue string   `json:"current_value,omitempty"`
	Selector     string   `json:"selector,omitempty"`
}

type OldAutomationResult struct {
	ApplicationID    int                    `json:"application_id"`
	Status          string                 `json:"status"` // 'completed', 'requires_input', 'failed'
	Steps           []AutomationStep       `json:"steps"`
	RequiredInputs  []FormField            `json:"required_inputs,omitempty"`
	SiteData        map[string]interface{} `json:"site_data,omitempty"`
}

func NewJobAutomationService(
	resumeDataExtractor *ResumeDataExtractor,
) *JobAutomationService {
	return &JobAutomationService{
		resumeDataExtractor:     resumeDataExtractor,
	}
}

func (s *JobAutomationService) StartAutomation(userID int, application *models.JobApplication) (*OldAutomationResult, error) {
	result := &OldAutomationResult{
		ApplicationID: application.ID,
		Status:       "pending",
		Steps:        []AutomationStep{},
	}

	// Step 1: Parse job URL and extract domain
	step1 := s.parseJobURL(application.JobURL)
	result.Steps = append(result.Steps, step1)
	// Logging removed - status stored in job_applications table

	if step1.Status == "failed" {
		result.Status = "failed"
		return result, nil
	}

	domain := step1.Data.(map[string]interface{})["domain"].(string)

	// Step 2: Extract user data from resume
	step2 := s.extractResumeData(userID, application.ResumeID)
	result.Steps = append(result.Steps, step2)
	// Logging removed - status stored in job_applications table

	if step2.Status == "failed" {
		result.Status = "failed"
		return result, nil
	}

	// Step 3: Prepare comprehensive automation data (no preferences - all data from resume)
	step3 := s.prepareComprehensiveAutomationData(userID, domain, application, step2.Data)
	result.Steps = append(result.Steps, step3)
	// Logging removed - status stored in job_applications table

	if step3.Status == "completed" {
		result.Status = "ready_for_automation"
		result.SiteData = step3.Data.(map[string]interface{})
	} else {
		result.Status = "failed"
	}

	return result, nil
}

func (s *JobAutomationService) parseJobURL(jobURL string) AutomationStep {
	step := AutomationStep{
		Name:      "parse_job_url",
		Timestamp: time.Now(),
	}

	parsedURL, err := url.Parse(jobURL)
	if err != nil {
		step.Status = "failed"
		step.ErrorMsg = fmt.Sprintf("Invalid URL: %v", err)
		return step
	}

	domain := parsedURL.Hostname()
	if domain == "" {
		step.Status = "failed"
		step.ErrorMsg = "Could not extract domain from URL"
		return step
	}

	step.Status = "completed"
	step.Data = map[string]interface{}{
		"domain":   domain,
		"path":     parsedURL.Path,
		"query":    parsedURL.RawQuery,
		"scheme":   parsedURL.Scheme,
		"full_url": jobURL,
	}

	return step
}

func (s *JobAutomationService) extractResumeData(userID, resumeID int) AutomationStep {
	step := AutomationStep{
		Name:      "extract_resume_data",
		Timestamp: time.Now(),
	}

	extractedData, err := s.resumeDataExtractor.ExtractUserData(userID, resumeID)
	if err != nil {
		step.Status = "failed"
		step.ErrorMsg = fmt.Sprintf("Failed to extract resume data: %v", err)
		return step
	}

	step.Status = "completed"
	step.Data = map[string]interface{}{
		"extracted_data": extractedData,
		"form_data":      s.resumeDataExtractor.ToFormDataMap(extractedData),
		"data_points":    len(s.resumeDataExtractor.ToFormDataMap(extractedData)),
	}

	return step
}


func (s *JobAutomationService) prepareComprehensiveAutomationData(userID int, domain string, application *models.JobApplication, resumeData interface{}) AutomationStep {
	step := AutomationStep{
		Name:      "prepare_comprehensive_automation_data",
		Timestamp: time.Now(),
	}

	// Get extracted resume data
	resumeDataMap := resumeData.(map[string]interface{})
	formData := resumeDataMap["form_data"].(map[string]interface{})

	// Add job-specific data
	formData["job_url"] = application.JobURL
	formData["company_name"] = application.CompanyName
	formData["position_title"] = application.PositionTitle
	formData["notes"] = application.Notes

	// Add intelligent automation instructions
	automationInstructions := map[string]interface{}{
		"steps": []string{
			"navigate_to_job_url",
			"detect_application_system",
			"click_apply_button",
			"fill_personal_information",
			"fill_professional_information",
			"fill_education_information", 
			"upload_resume_if_required",
			"fill_additional_questions",
			"review_application",
			"submit_application",
		},
		"form_data": formData,
		"extracted_data": resumeDataMap["extracted_data"],
		"domain": domain,
		"timestamp": time.Now().Unix(),
		"application_id": application.ID,
		"automation_ready": true,
		"data_source": "resume_extraction",
		"total_data_points": len(formData),
	}

	step.Status = "completed"
	step.Data = automationInstructions

	return step
}

// SaveUserPreferences removed - user data comes from resume only

// UpdateFormRequirements removed - no longer storing form requirements in database
// Form data comes from user resume input only

// logStep function removed - logging handled in job_applications table

// Helper function to extract domain from job site for common patterns
func (s *JobAutomationService) NormalizeDomain(domain string) string {
	domain = strings.ToLower(domain)
	
	// Remove www prefix
	if strings.HasPrefix(domain, "www.") {
		domain = domain[4:]
	}
	
	// Handle common job site patterns
	if strings.Contains(domain, "greenhouse") {
		return "greenhouse.io"
	}
	if strings.Contains(domain, "lever") {
		return "lever.co"
	}
	if strings.Contains(domain, "workday") {
		return "workday.com"
	}
	if strings.Contains(domain, "bamboohr") {
		return "bamboohr.com"
	}
	
	return domain
}