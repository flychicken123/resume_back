package services

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormField represents a form field detected on a job application
type FormField struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Label       string      `json:"label"`
	Type        string      `json:"type"`
	Required    bool        `json:"required"`
	Options     []string    `json:"options,omitempty"`
	Value       interface{} `json:"value,omitempty"`
	Placeholder string      `json:"placeholder,omitempty"`
	Pattern     string      `json:"pattern,omitempty"`
	MinLength   int         `json:"minLength,omitempty"`
	MaxLength   int         `json:"maxLength,omitempty"`
}

// ExtractedFormData represents all form fields extracted from a job application
type ExtractedFormData struct {
	Platform       string                 `json:"platform"`
	Company        string                 `json:"company"`
	JobTitle       string                 `json:"jobTitle"`
	FormFields     []FormField            `json:"formFields"`
	RequiredFields []string               `json:"requiredFields"`
	OptionalFields []string               `json:"optionalFields"`
	CustomFields   map[string]interface{} `json:"customFields"`
}

// FormExtractor handles extraction of form fields from job applications
type FormExtractor struct {
	prefsModel interface{} // Will be ApplicationPreferencesModel
}

// NewFormExtractor creates a new form extractor
func NewFormExtractor() *FormExtractor {
	return &FormExtractor{}
}

// ExtractFormFields simulates extracting form fields from a job application page
// In production, this would use headless browser to actually extract fields
func (fe *FormExtractor) ExtractFormFields(jobURL string, platform PlatformInfo) (*ExtractedFormData, error) {
	// Simulate different form fields based on platform
	switch platform.Type {
	case "company_ats":
		return fe.extractATSFields(jobURL, platform)
	case "job_board":
		return fe.extractJobBoardFields(jobURL, platform)
	case "social":
		return fe.extractSocialPlatformFields(jobURL, platform)
	default:
		return fe.extractGenericFields(jobURL, platform)
	}
}

// extractATSFields extracts fields typical of ATS platforms (Greenhouse, Workday, Lever)
func (fe *FormExtractor) extractATSFields(jobURL string, platform PlatformInfo) (*ExtractedFormData, error) {
	formData := &ExtractedFormData{
		Platform: platform.Name,
		Company:  extractCompanyFromURL(jobURL),
		JobTitle: "Cloud Infrastructure Engineer", // Would be extracted from page
		FormFields: []FormField{
			// Personal Information
			{ID: "first_name", Name: "first_name", Label: "First Name", Type: "text", Required: true},
			{ID: "last_name", Name: "last_name", Label: "Last Name", Type: "text", Required: true},
			{ID: "email", Name: "email", Label: "Email", Type: "email", Required: true},
			{ID: "phone", Name: "phone", Label: "Phone", Type: "tel", Required: true},
			{ID: "location", Name: "location", Label: "Current Location", Type: "text", Required: true},
			
			// Resume and Documents
			{ID: "resume", Name: "resume", Label: "Resume", Type: "file", Required: true},
			{ID: "cover_letter", Name: "cover_letter", Label: "Cover Letter", Type: "file", Required: false},
			
			// Professional Information
			{ID: "linkedin_url", Name: "linkedin_url", Label: "LinkedIn Profile", Type: "url", Required: false},
			{ID: "portfolio_url", Name: "portfolio_url", Label: "Portfolio/Website", Type: "url", Required: false},
			{ID: "github_url", Name: "github_url", Label: "GitHub Profile", Type: "url", Required: false},
			
			// Experience Questions
			{ID: "years_experience", Name: "years_experience", Label: "Years of Experience", Type: "number", Required: true},
			{ID: "current_company", Name: "current_company", Label: "Current Company", Type: "text", Required: false},
			{ID: "current_title", Name: "current_title", Label: "Current Job Title", Type: "text", Required: false},
			
			// Salary and Availability
			{ID: "expected_salary", Name: "expected_salary", Label: "Expected Salary", Type: "text", Required: false},
			{ID: "availability", Name: "availability", Label: "When can you start?", Type: "select", Required: true,
				Options: []string{"Immediately", "2 weeks", "1 month", "2 months", "Other"}},
			
			// Work Authorization
			{ID: "work_authorization", Name: "work_authorization", Label: "Are you authorized to work in the US?", Type: "select", Required: true,
				Options: []string{"Yes", "No", "Need Sponsorship"}},
			{ID: "require_sponsorship", Name: "require_sponsorship", Label: "Will you require sponsorship?", Type: "select", Required: true,
				Options: []string{"Yes", "No"}},
			
			// Additional Questions
			{ID: "referral_source", Name: "referral_source", Label: "How did you hear about us?", Type: "select", Required: false,
				Options: []string{"LinkedIn", "Company Website", "Indeed", "Referral", "Other"}},
			{ID: "referred_by", Name: "referred_by", Label: "Referred by (Employee Name)", Type: "text", Required: false},
			
			// Custom Questions (platform specific)
			{ID: "why_interested", Name: "why_interested", Label: "Why are you interested in this role?", Type: "textarea", Required: false, MaxLength: 500},
			{ID: "relevant_experience", Name: "relevant_experience", Label: "Describe your relevant experience", Type: "textarea", Required: false, MaxLength: 1000},
			
			// Diversity Questions (optional)
			{ID: "gender", Name: "gender", Label: "Gender (Optional)", Type: "select", Required: false,
				Options: []string{"Male", "Female", "Non-binary", "Prefer not to say"}},
			{ID: "ethnicity", Name: "ethnicity", Label: "Ethnicity (Optional)", Type: "select", Required: false,
				Options: []string{"Asian", "Black", "Hispanic", "White", "Other", "Prefer not to say"}},
			{ID: "veteran_status", Name: "veteran_status", Label: "Veteran Status (Optional)", Type: "select", Required: false,
				Options: []string{"Yes", "No", "Prefer not to say"}},
		},
	}

	// Separate required and optional fields
	for _, field := range formData.FormFields {
		if field.Required {
			formData.RequiredFields = append(formData.RequiredFields, field.Name)
		} else {
			formData.OptionalFields = append(formData.OptionalFields, field.Name)
		}
	}

	return formData, nil
}

// extractJobBoardFields extracts fields typical of job boards (Indeed, Glassdoor)
func (fe *FormExtractor) extractJobBoardFields(jobURL string, platform PlatformInfo) (*ExtractedFormData, error) {
	formData := &ExtractedFormData{
		Platform: platform.Name,
		Company:  "StartupXYZ", // Would be extracted from page
		JobTitle: "Frontend Developer",
		FormFields: []FormField{
			// Basic Information
			{ID: "full_name", Name: "full_name", Label: "Full Name", Type: "text", Required: true},
			{ID: "email", Name: "email", Label: "Email", Type: "email", Required: true},
			{ID: "phone", Name: "phone", Label: "Phone", Type: "tel", Required: true},
			
			// Resume
			{ID: "resume", Name: "resume", Label: "Resume", Type: "file", Required: true},
			
			// Quick Apply Questions
			{ID: "years_experience", Name: "years_experience", Label: "Years of Experience", Type: "select", Required: true,
				Options: []string{"0-1", "2-3", "4-5", "6-10", "10+"}},
			{ID: "education_level", Name: "education_level", Label: "Highest Education", Type: "select", Required: true,
				Options: []string{"High School", "Associate", "Bachelor's", "Master's", "PhD"}},
			{ID: "willing_to_relocate", Name: "willing_to_relocate", Label: "Willing to relocate?", Type: "checkbox", Required: false},
		},
	}

	// Separate required and optional fields
	for _, field := range formData.FormFields {
		if field.Required {
			formData.RequiredFields = append(formData.RequiredFields, field.Name)
		} else {
			formData.OptionalFields = append(formData.OptionalFields, field.Name)
		}
	}

	return formData, nil
}

// extractSocialPlatformFields extracts fields typical of LinkedIn
func (fe *FormExtractor) extractSocialPlatformFields(jobURL string, platform PlatformInfo) (*ExtractedFormData, error) {
	formData := &ExtractedFormData{
		Platform: platform.Name,
		Company:  "Tech Innovations Inc.",
		JobTitle: "Senior Software Engineer",
		FormFields: []FormField{
			// LinkedIn Easy Apply typically has minimal fields
			{ID: "email", Name: "email", Label: "Email", Type: "email", Required: true},
			{ID: "phone", Name: "phone", Label: "Phone", Type: "tel", Required: true},
			{ID: "resume", Name: "resume", Label: "Resume", Type: "file", Required: true},
			
			// Additional Questions
			{ID: "years_experience", Name: "years_experience", Label: "How many years of experience do you have?", Type: "number", Required: true},
			{ID: "notice_period", Name: "notice_period", Label: "Notice Period", Type: "select", Required: false,
				Options: []string{"Immediate", "2 weeks", "1 month", "2 months", "3 months"}},
		},
	}

	// Separate required and optional fields
	for _, field := range formData.FormFields {
		if field.Required {
			formData.RequiredFields = append(formData.RequiredFields, field.Name)
		} else {
			formData.OptionalFields = append(formData.OptionalFields, field.Name)
		}
	}

	return formData, nil
}

// extractGenericFields extracts basic fields for unknown platforms
func (fe *FormExtractor) extractGenericFields(jobURL string, platform PlatformInfo) (*ExtractedFormData, error) {
	return &ExtractedFormData{
		Platform: platform.Name,
		Company:  "Company",
		JobTitle: "Position",
		FormFields: []FormField{
			{ID: "name", Name: "name", Label: "Name", Type: "text", Required: true},
			{ID: "email", Name: "email", Label: "Email", Type: "email", Required: true},
			{ID: "phone", Name: "phone", Label: "Phone", Type: "tel", Required: false},
			{ID: "resume", Name: "resume", Label: "Resume", Type: "file", Required: true},
			{ID: "cover_letter", Name: "cover_letter", Label: "Cover Letter", Type: "textarea", Required: false},
		},
		RequiredFields: []string{"name", "email", "resume"},
		OptionalFields: []string{"phone", "cover_letter"},
	}, nil
}

// AutoFillForm attempts to fill form fields with user data and preferences
func (fe *FormExtractor) AutoFillForm(formData *ExtractedFormData, userData map[string]interface{}, savedPrefs map[string]interface{}) (map[string]interface{}, []string) {
	filledData := make(map[string]interface{})
	var missingFields []string

	// Combine user data and saved preferences
	allData := make(map[string]interface{})
	for k, v := range userData {
		allData[k] = v
	}
	for k, v := range savedPrefs {
		allData[k] = v
	}

	// Try to fill each form field
	for _, field := range formData.FormFields {
		// Try multiple possible keys for each field
		possibleKeys := fe.getPossibleKeys(field.Name)
		filled := false

		for _, key := range possibleKeys {
			if value, exists := allData[key]; exists && value != nil && value != "" {
				filledData[field.Name] = value
				filled = true
				break
			}
		}

		if !filled && field.Required {
			missingFields = append(missingFields, field.Name)
		}
	}

	return filledData, missingFields
}

// getPossibleKeys returns possible data keys for a form field
func (fe *FormExtractor) getPossibleKeys(fieldName string) []string {
	// Map common variations
	variations := map[string][]string{
		"first_name":          {"first_name", "firstName", "fname", "given_name"},
		"last_name":           {"last_name", "lastName", "lname", "surname", "family_name"},
		"full_name":           {"full_name", "fullName", "name"},
		"email":               {"email", "email_address", "emailAddress"},
		"phone":               {"phone", "phone_number", "phoneNumber", "mobile", "telephone"},
		"location":            {"location", "current_location", "city", "address"},
		"linkedin_url":        {"linkedin_url", "linkedin", "linkedIn", "linkedin_profile"},
		"portfolio_url":       {"portfolio_url", "portfolio", "website", "personal_website"},
		"github_url":          {"github_url", "github", "github_profile"},
		"years_experience":    {"years_experience", "total_experience", "experience_years"},
		"expected_salary":     {"expected_salary", "salary_expectation", "desired_salary"},
		"availability":        {"availability", "start_date", "available_date", "when_can_start"},
		"work_authorization":  {"work_authorization", "visa_status", "eligible_to_work"},
		"referral_source":     {"referral_source", "how_did_you_hear", "source"},
	}

	if keys, exists := variations[fieldName]; exists {
		return keys
	}

	// Return the field name itself as the only option
	return []string{fieldName}
}

// Helper function to extract company from URL
func extractCompanyFromURL(jobURL string) string {
	// This is a simplified version - in production would be more sophisticated
	parts := strings.Split(jobURL, "/")
	for _, part := range parts {
		if strings.Contains(part, ".com") {
			domain := strings.Split(part, ".")[0]
			return strings.Title(strings.Replace(domain, "www", "", 1))
		}
	}
	return "Technology Company"
}

// PrepareSubmissionData prepares the final data for submission
func (fe *FormExtractor) PrepareSubmissionData(formData *ExtractedFormData, filledData map[string]interface{}, additionalData map[string]interface{}) map[string]interface{} {
	submissionData := make(map[string]interface{})

	// Copy filled data
	for k, v := range filledData {
		submissionData[k] = v
	}

	// Add any additional data provided by user
	for k, v := range additionalData {
		submissionData[k] = v
	}

	// Ensure all required fields have at least empty values
	for _, field := range formData.FormFields {
		if _, exists := submissionData[field.Name]; !exists {
			// Set appropriate default based on type
			switch field.Type {
			case "checkbox":
				submissionData[field.Name] = false
			case "number":
				submissionData[field.Name] = 0
			case "select":
				if len(field.Options) > 0 {
					submissionData[field.Name] = field.Options[0]
				} else {
					submissionData[field.Name] = ""
				}
			default:
				submissionData[field.Name] = ""
			}
		}
	}

	return submissionData
}

// ValidateFormData validates that all required fields are filled
func (fe *FormExtractor) ValidateFormData(formData *ExtractedFormData, submissionData map[string]interface{}) (bool, []string) {
	var missingRequired []string

	for _, fieldName := range formData.RequiredFields {
		if value, exists := submissionData[fieldName]; !exists || value == nil || value == "" {
			missingRequired = append(missingRequired, fieldName)
		}
	}

	return len(missingRequired) == 0, missingRequired
}

// ConvertToJSON converts form data to JSON for storage
func (fe *FormExtractor) ConvertToJSON(data interface{}) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to convert to JSON: %w", err)
	}
	return string(jsonData), nil
}