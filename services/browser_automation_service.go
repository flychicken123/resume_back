package services

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

type BrowserAutomationService struct {
	browser playwright.Browser
	context context.Context
	s3Service *S3Service
	lastDropdownError string // Store error message from dropdown handler
}

type AutomationResult struct {
	Success                  bool                   `json:"success"`
	Status                   string                 `json:"status"`
	Message                  string                 `json:"message"`
	Screenshots              []string               `json:"screenshots,omitempty"`
	JobPageScreenshotKey     string                 `json:"job_page_screenshot_key,omitempty"`     // S3 key, not URL
	ApplicationScreenshotKey string                 `json:"application_screenshot_key,omitempty"` // S3 key, not URL
	ConfirmationScreenshotKey string                `json:"confirmation_screenshot_key,omitempty"` // S3 key, not URL
	FilledFields             map[string]string      `json:"filled_fields,omitempty"`
	ErrorDetails             string                 `json:"error_details,omitempty"`
	SubmissionResult         *SubmissionResult      `json:"submission_result,omitempty"`
}

type SubmissionResult struct {
	ApplicationID    string `json:"application_id,omitempty"`
	ConfirmationURL  string `json:"confirmation_url,omitempty"`
	SuccessMessage   string `json:"success_message,omitempty"`
	NextSteps        string `json:"next_steps,omitempty"`
}

type UserProfileData struct {
	FullName              string `json:"full_name"`
	FirstName             string `json:"first_name"`
	LastName              string `json:"last_name"`
	Email                 string `json:"email"`
	Phone                 string `json:"phone"`
	Address               string `json:"address"`
	City                  string `json:"city"`
	State                 string `json:"state"`
	ZipCode               string `json:"zip_code"`
	Country               string `json:"country"`
	LinkedIn              string `json:"linkedin"`
	Portfolio             string `json:"portfolio"`
	Summary               string `json:"summary"`
	Skills                []string `json:"skills"`
	Experience            []ExperienceData `json:"experience"`
	Education             []EducationData `json:"education"`
	
	// Job Application Specific Fields
	WorkAuthorization     string `json:"work_authorization"`     // "yes", "no", "requires_sponsorship"
	RequiresSponsorship   bool   `json:"requires_sponsorship"`
	WillingToRelocate     bool   `json:"willing_to_relocate"`
	SalaryExpectationMin  int    `json:"salary_expectation_min"`
	SalaryExpectationMax  int    `json:"salary_expectation_max"`
	PreferredLocations    string `json:"preferred_locations"`
	AvailableStartDate    string `json:"available_start_date"`   // "immediately", "2_weeks", "1_month"
	YearsOfExperience     int    `json:"years_of_experience"`
	
	// Demographic Fields (for diversity questions)
	Gender                string `json:"gender"`                 // male, female, other, prefer_not_to_say
	Ethnicity             string `json:"ethnicity"`
	VeteranStatus         string `json:"veteran_status"`         // yes, no, prefer_not_to_say
	DisabilityStatus      string `json:"disability_status"`      // yes, no, prefer_not_to_say
	
	// Additional fields for enhanced form filling
	CurrentCompany        string `json:"current_company"`
	CurrentTitle          string `json:"current_title"`
	RecentSchool          string `json:"recent_school"`
	RecentDegree          string `json:"recent_degree"`
	RemoteWorkPreference  string `json:"remote_work_preference"`  // yes, no, hybrid
	
	// Additional common application fields
	SexualOrientation     string `json:"sexual_orientation"`     // Heterosexual, Gay/Lesbian, Bisexual, Prefer not to answer
	TransgenderStatus     string `json:"transgender_status"`     // Yes, No, Prefer not to answer
	MostRecentDegree      string `json:"most_recent_degree"`      // Bachelor's, Master's, PhD, etc.
	GraduationYear        int    `json:"graduation_year"`
	University            string `json:"university"`
	Major                 string `json:"major"`
	
	// Extra Q&A for unknown fields
	ExtraQA               map[string]string `json:"extra_qa"`  // Stores question-answer pairs for fields not in schema
}

type ExperienceData struct {
	Title       string `json:"title"`       // Changed from JobTitle for consistency
	Company     string `json:"company"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Description string `json:"description"`
	IsCurrent   bool   `json:"is_current"`
}

type EducationData struct {
	Degree       string `json:"degree"`
	Institution  string `json:"institution"`  // Changed from School for consistency
	Field        string `json:"field"`        // Added field of study
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	GPA          string `json:"gpa,omitempty"`
	Description  string `json:"description,omitempty"`
}

func NewBrowserAutomationService(ctx context.Context) (*BrowserAutomationService, error) {
	// Initialize Playwright
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to start playwright: %w", err)
	}

	// Launch browser in headless mode
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true), // Set to false for debugging
		Args: []string{
			"--no-sandbox",
			"--disable-blink-features=AutomationControlled",
			"--disable-extensions",
			"--disable-plugins-discovery",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	// Initialize S3 service for screenshot uploads
	s3Service, err := NewS3Service()
	if err != nil {
		log.Printf("Warning: S3 service not available for screenshots: %v", err)
		// Continue without S3 - screenshots will be saved locally
	}

	return &BrowserAutomationService{
		browser:   browser,
		context:   ctx,
		s3Service: s3Service,
	}, nil
}

func (s *BrowserAutomationService) Close() error {
	if s.browser != nil {
		return s.browser.Close()
	}
	return nil
}

// saveScreenshot takes a screenshot and saves it to S3, returns the S3 key (not URL)
func (s *BrowserAutomationService) saveScreenshot(page playwright.Page, screenshotType string, result *AutomationResult) (string, error) {
	log.Printf("Taking screenshot: %s", screenshotType)
	// Create temporary file path
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("%s_%d.png", screenshotType, timestamp)
	tempPath := fmt.Sprintf("./temp_%s", filename)
	
	// Take FULL PAGE screenshot
	_, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(tempPath),
		FullPage: playwright.Bool(true), // Capture entire page
	})
	if err != nil {
		return "", fmt.Errorf("failed to take screenshot: %w", err)
	}
	
	// Upload to S3 if available
	if s.s3Service != nil {
		s3Key := fmt.Sprintf("screenshots/%s", filename)
		log.Printf("Uploading screenshot to S3 with key: %s", s3Key)
		_, err := s.s3Service.UploadFile(tempPath, s3Key)
		if err != nil {
			log.Printf("Failed to upload screenshot to S3: %v", err)
			// Fall back to local storage
			localPath := fmt.Sprintf("./static/%s", filename)
			os.Rename(tempPath, localPath)
			localURL := fmt.Sprintf("/static/%s", filename)
			result.Screenshots = append(result.Screenshots, localURL)
			log.Printf("Screenshot saved locally: %s", localURL)
			return localURL, nil
		}
		
		// Clean up temp file
		os.Remove(tempPath)
		result.Screenshots = append(result.Screenshots, s3Key)
		log.Printf("Screenshot uploaded to S3 with key: %s", s3Key)
		return s3Key, nil
	} else {
		log.Printf("S3 not available, saving screenshot locally: %s", filename)
		// Save locally if S3 not available
		localPath := fmt.Sprintf("./static/%s", filename)
		err = os.Rename(tempPath, localPath)
		if err != nil {
			return "", fmt.Errorf("failed to save screenshot locally: %w", err)
		}
		localURL := fmt.Sprintf("/static/%s", filename)
		result.Screenshots = append(result.Screenshots, localURL)
		log.Printf("Screenshot saved locally: %s", localURL)
		return localURL, nil
	}
}

func (s *BrowserAutomationService) SubmitJobApplication(jobURL string, userData *UserProfileData, resumeFilePath string) (*AutomationResult, error) {
	log.Printf("Starting job application automation for URL: %s", jobURL)
	
	result := &AutomationResult{
		FilledFields: make(map[string]string),
	}

	// Parse job URL to determine the platform
	parsedURL, err := url.Parse(jobURL)
	if err != nil {
		result.Success = false
				return result, nil
		result.Status = "error"
		result.Message = "Invalid job URL"
		result.ErrorDetails = err.Error()
		return result, err
	}

	domain := parsedURL.Hostname()
	log.Printf("Detected domain: %s", domain)

	// Route to specific platform handler
	switch {
	case strings.Contains(domain, "linkedin.com"):
		return s.handleLinkedInApplication(jobURL, userData, resumeFilePath)
	case strings.Contains(domain, "indeed.com"):
		return s.handleIndeedApplication(jobURL, userData, resumeFilePath)
	case strings.Contains(domain, "glassdoor.com"):
		return s.handleGlassdoorApplication(jobURL, userData, resumeFilePath)
	case strings.Contains(domain, "careers") || strings.Contains(domain, "jobs"):
		return s.handleCareerPageApplication(jobURL, userData, resumeFilePath)
	default:
		return s.handleGenericApplication(jobURL, userData, resumeFilePath)
	}
}

func (s *BrowserAutomationService) handleLinkedInApplication(jobURL string, userData *UserProfileData, resumeFilePath string) (*AutomationResult, error) {
	log.Printf("Handling LinkedIn application")
	
	result := &AutomationResult{
		FilledFields: make(map[string]string),
		Status:       "processing",
	}

	// Create a new page
	page, err := s.browser.NewPage()
	if err != nil {
		result.Success = false
				return result, nil
		result.Status = "error"
		result.Message = "Failed to create browser page"
		result.ErrorDetails = err.Error()
		return result, err
	}
	defer page.Close()

	// Set user agent to avoid detection
	page.SetExtraHTTPHeaders(map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
	})

	// Navigate to job page
	log.Printf("Navigating to LinkedIn job page...")
	_, err = page.Goto(jobURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	})
	if err != nil {
		result.Success = false
				return result, nil
		result.Status = "error"
		result.Message = "Failed to load job page"
		result.ErrorDetails = err.Error()
		return result, err
	}

	// Take screenshot for job page
// 	jobPageScreenshotURL, err := s.saveScreenshot(page, "linkedin_job_page", result)
// 	if err != nil {
// 		log.Printf("Failed to save job page screenshot: %v", err)
// 	} else {
// 		result.JobPageScreenshotKey = jobPageScreenshotURL
// 	}

	// Check if user is logged in to LinkedIn
	isLoggedIn := false
	if signInButton, err := page.Locator("a[href*='login']").First().IsVisible(); err == nil && signInButton {
		log.Printf("User not logged in to LinkedIn")
		result.Success = false
		result.Status = "login_required"
		result.Message = "Please log in to LinkedIn first and try again"
		result.Success = false
				return result, nil
		isLoggedIn = true
		log.Printf("User appears to be logged in to LinkedIn")
	}

	if !isLoggedIn {
		result.Success = false
				return result, nil
		result.Status = "login_required"
		result.Message = "LinkedIn login required. Please log in manually first."
		result.Success = false
				return result, nil
				return result, nil
	}

	// Look for "Easy Apply" button
	easyApplyButton := page.Locator("button[aria-label*='Easy Apply'], button:has-text('Easy Apply')")
	if exists, err := easyApplyButton.First().IsVisible(); err == nil && exists {
		log.Printf("Found Easy Apply button, clicking...")
		
		err = easyApplyButton.First().Click()
		if err != nil {
			result.Success = false
				return result, nil
			result.Status = "error"
			result.Message = "Failed to click Easy Apply button"
			result.ErrorDetails = err.Error()
			return result, err
		}

		// Wait for application form to load
		page.WaitForTimeout(2000)
		
		// Fill out the application form
		return s.fillLinkedInApplicationForm(page, userData, resumeFilePath, result)
	} else {
		log.Printf("Easy Apply not available, looking for external application")
		
		// Look for external apply button
		applyButton := page.Locator("button:has-text('Apply'), a:has-text('Apply')")
		if exists, err := applyButton.First().IsVisible(); err == nil && exists {
			result.Success = false
				return result, nil
			result.Status = "external_application"
			result.Message = "This job requires external application. Please apply directly on the company website."
			result.Success = false
				return result, nil
				return result, nil
		}
	}

	result.Success = false
				return result, nil
	result.Status = "no_apply_button"
	result.Message = "Could not find application button on the page"
	result.Success = false
				return result, nil
				return result, nil
}

func (s *BrowserAutomationService) fillLinkedInApplicationForm(page playwright.Page, userData *UserProfileData, resumeFilePath string, result *AutomationResult) (*AutomationResult, error) {
	log.Printf("Filling LinkedIn application form...")
	
	// Take screenshot of application form
	applicationScreenshotURL, err := s.saveScreenshot(page, "linkedin_application_form", result)
	if err != nil {
		log.Printf("Failed to save application form screenshot: %v", err)
	} else {
		result.ApplicationScreenshotKey = applicationScreenshotURL
	}

	// Common LinkedIn form fields
	formFields := map[string]string{
		"input[name*='phoneNumber'], input[id*='phone']":     userData.Phone,
		"input[name*='email'], input[id*='email']":           userData.Email,
		"input[name*='firstName'], input[id*='firstName']":   strings.Split(userData.FullName, " ")[0],
		"input[name*='lastName'], input[id*='lastName']":     func() string {
			parts := strings.Split(userData.FullName, " ")
			if len(parts) > 1 {
				return strings.Join(parts[1:], " ")
			}
			return ""
		}(),
		"textarea[name*='coverLetter'], textarea[id*='coverLetter']": userData.Summary,
	}

	// Fill form fields
	for selector, value := range formFields {
		if value == "" {
			continue
		}
		
		field := page.Locator(selector)
		if exists, err := field.First().IsVisible(); err == nil && exists {
			log.Printf("Filling field %s with value: %s", selector, value)
			err = field.First().Fill(value)
			if err != nil {
				log.Printf("Failed to fill field %s: %v", selector, err)
			} else {
				result.FilledFields[selector] = value
			}
		}
	}

	// Upload resume if file path provided
	if resumeFilePath != "" {
		// Try multiple selectors for file upload
		fileUploadSelectors := []string{
			"input[type='file']",
			"input[type=file]",
			"input[name*='resume']",
			"input[name*='cv']",
			"input[name*='document']",
			"input[accept*='pdf']",
		}
		
		var uploadSuccess bool
		for _, selector := range fileUploadSelectors {
			resumeUpload := page.Locator(selector)
			if count, _ := resumeUpload.Count(); count > 0 {
				log.Printf("Found %d file input(s) with selector: %s", count, selector)
				for i := 0; i < count && !uploadSuccess; i++ {
					fileInput := resumeUpload.Nth(i)
					if visible, _ := fileInput.IsVisible(); visible {
						log.Printf("Uploading resume file to input %d: %s", i, resumeFilePath)
						err := fileInput.SetInputFiles(resumeFilePath)
						if err != nil {
							log.Printf("Failed to upload resume to input %d: %v", i, err)
						} else {
							log.Printf("✓ Successfully uploaded resume file")
							result.FilledFields["resume_upload"] = resumeFilePath
							uploadSuccess = true
							break
						}
					}
				}
			}
		}
		
		if !uploadSuccess {
			log.Printf("⚠ No visible file upload input found for resume")
		}
	}

	// Handle multi-step forms
	for step := 1; step <= 5; step++ {
		log.Printf("Processing form step %d", step)
		
		// Look for "Next" button
		nextButton := page.Locator("button:has-text('Next'), button[aria-label*='Continue'], button[aria-label*='Next']")
		if exists, err := nextButton.First().IsVisible(); err == nil && exists {
			log.Printf("Found Next button, clicking...")
			
			// Wait a moment for any validation
			page.WaitForTimeout(1000)
			
			err = nextButton.First().Click()
			if err != nil {
				log.Printf("Failed to click Next button: %v", err)
				break
			}
			
			// Wait for next step to load
			page.WaitForTimeout(2000)
			
			// Take screenshot of next step
			stepScreenshotURL, err := s.saveScreenshot(page, fmt.Sprintf("linkedin_step_%d", step), result)
			if err != nil {
				log.Printf("Failed to save step screenshot: %v", err)
			} else {
				// Update application screenshot with the latest step
				result.ApplicationScreenshotKey = stepScreenshotURL
			}
		} else {
			log.Printf("No Next button found, checking for Submit button")
			break
		}
	}

	// Look for submit button
	submitButton := page.Locator("button:has-text('Submit'), button[aria-label*='Submit'], button:has-text('Submit application')")
	if exists, err := submitButton.First().IsVisible(); err == nil && exists {
		log.Printf("Found Submit button, submitting application...")
		
		// Take final screenshot before submission
		finalScreenshotURL, err := s.saveScreenshot(page, "linkedin_before_submit", result)
		if err != nil {
			log.Printf("Failed to save final screenshot: %v", err)
		} else {
			result.ApplicationScreenshotKey = finalScreenshotURL
		}
		
		err = submitButton.First().Click()
		if err != nil {
			result.Success = false
				return result, nil
			result.Status = "error"
			result.Message = "Failed to submit application"
			result.ErrorDetails = err.Error()
			return result, err
		}
		
		// Wait for submission confirmation
		page.WaitForTimeout(1500)
		
		// Take screenshot of confirmation
		confirmationScreenshotURL, err := s.saveScreenshot(page, "linkedin_confirmation", result)
		if err != nil {
			log.Printf("Failed to save confirmation screenshot: %v", err)
		} else {
			// Use confirmation as the final application screenshot
			result.ApplicationScreenshotKey = confirmationScreenshotURL
		}
		
		// Check for success indicators
		successIndicators := []string{
			"text=Application sent",
			"text=Your application has been sent",
			"text=Application submitted",
			"text=Thank you for applying",
		}
		
		for _, indicator := range successIndicators {
			if exists, err := page.Locator(indicator).First().IsVisible(); err == nil && exists {
				result.Success = true
				result.Status = "submitted"
				result.Message = "Application successfully submitted to LinkedIn"
				result.SubmissionResult = &SubmissionResult{
					SuccessMessage: "Your application has been submitted successfully",
					NextSteps:      "The hiring team will review your application and contact you if interested",
				}
				result.Success = false
				return result, nil
				return result, nil
			}
		}
		
		result.Success = true
		result.Status = "submitted"
		result.Message = "Application appears to have been submitted successfully"
		result.Success = false
				return result, nil
				return result, nil
	}

	result.Success = false
				return result, nil
	result.Status = "error"
	result.Message = "Could not find submit button to complete application"
	result.Success = false
				return result, nil
				return result, nil
}

func (s *BrowserAutomationService) handleIndeedApplication(jobURL string, userData *UserProfileData, resumeFilePath string) (*AutomationResult, error) {
	log.Printf("Handling Indeed application")
	
	result := &AutomationResult{
		FilledFields: make(map[string]string),
		Status:       "processing",
	}

	// Create a new page
	page, err := s.browser.NewPage()
	if err != nil {
		result.Success = false
				return result, nil
		result.Status = "error"
		result.Message = "Failed to create browser page"
		result.ErrorDetails = err.Error()
		return result, err
	}
	defer page.Close()

	// Navigate to job page
	_, err = page.Goto(jobURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	})
	if err != nil {
		result.Success = false
				return result, nil
		result.Status = "error"
		result.Message = "Failed to load Indeed job page"
		result.ErrorDetails = err.Error()
		return result, err
	}

	// Implementation for Indeed application process
	// This would be similar to LinkedIn but with Indeed-specific selectors
	result.Success = false
				return result, nil
	result.Status = "not_implemented"
	result.Message = "Indeed automation is not yet implemented. Please apply manually."
	result.Success = false
				return result, nil
				return result, nil
}

func (s *BrowserAutomationService) handleGlassdoorApplication(jobURL string, userData *UserProfileData, resumeFilePath string) (*AutomationResult, error) {
	result := &AutomationResult{
		FilledFields: make(map[string]string),
		Status:       "not_implemented",
		Success:      false,
		Message:      "Glassdoor automation is not yet implemented. Please apply manually.",
	}
	result.Success = false
				return result, nil
				return result, nil
}

func (s *BrowserAutomationService) handleCareerPageApplication(jobURL string, userData *UserProfileData, resumeFilePath string) (*AutomationResult, error) {
	log.Printf("Handling career page application")
	
	result := &AutomationResult{
		FilledFields: make(map[string]string),
		Status:       "processing",
	}

	// This would implement generic form filling for company career pages
	result.Success = false
				return result, nil
	result.Status = "not_implemented"
	result.Message = "Generic career page automation is not yet implemented. Please apply manually."
	result.Success = false
				return result, nil
				return result, nil
}

func (s *BrowserAutomationService) handleGreenhouseApplication(page playwright.Page, userData *UserProfileData, resumeFilePath string, result *AutomationResult) (*AutomationResult, error) {
	log.Printf("Handling Greenhouse embedded application")
	
	// Wait for iframe to load
	page.WaitForTimeout(2000)
	
	// Get the Greenhouse iframe
	iframe := page.FrameLocator("iframe[src*='greenhouse.io'], iframe[src*='boards.greenhouse.io']").First()
	
	// Check if we can find the application form inside the iframe
	formExists, _ := iframe.Locator("form#application_form, div.application-form").Count()
	if formExists == 0 {
		log.Printf("No application form found in Greenhouse iframe")
		result.Success = false
				return result, nil
		result.Status = "external_application"
		result.Message = "This job requires applying on the Greenhouse platform. Click the apply button to proceed."
		result.Success = false
				return result, nil
				return result, nil
	}
	
	log.Printf("Found Greenhouse application form in iframe")
	
	// Take screenshot before filling
	_, err := s.saveScreenshot(page, "before_fill_greenhouse", result)
	if err != nil {
		log.Printf("Failed to save before-fill screenshot: %v", err)
	}
	
	// Fill the Greenhouse form
	// TODO: Implement fillGreenhouseForm
	result.Success = false
				return result, nil
}

func (s *BrowserAutomationService) handleGenericApplication(jobURL string, userData *UserProfileData, resumeFilePath string) (*AutomationResult, error) {
	log.Printf("Handling generic application for URL: %s", jobURL)
	
	result := &AutomationResult{
		FilledFields: make(map[string]string),
		Status:       "processing",
	}

	// Create a new page
	page, err := s.browser.NewPage()
	if err != nil {
		result.Success = false
				return result, nil
		result.Status = "error"
		result.Message = "Failed to create browser page"
		result.ErrorDetails = err.Error()
		return result, err
	}
	defer page.Close()

	// Set user agent
	page.SetExtraHTTPHeaders(map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
	})

	// Navigate to job page
	log.Printf("Navigating to job page...")
	_, err = page.Goto(jobURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateLoad,
		Timeout:   playwright.Float(30000),
	})
	if err != nil {
		result.Success = false
				return result, nil
		result.Status = "error"
		result.Message = "Failed to load job page"
		result.ErrorDetails = err.Error()
		return result, err
	}
	
	// Wait for content to load
	log.Printf("Waiting for page content to load...")
	page.WaitForTimeout(1500)

	// Take screenshot for debugging
// 	jobPageScreenshotKey, err := s.saveScreenshot(page, "job_page", result)
// 	if err != nil {
// 		log.Printf("Failed to save job page screenshot: %v", err)
// 	} else {
// 		result.JobPageScreenshotKey = jobPageScreenshotKey
// 	}

	// Check for Greenhouse iframe first (CoreWeave and many others use this)
	iframeCount, _ := page.Locator("iframe").Count()
	log.Printf("Found %d iframes on the page", iframeCount)
	
	if iframeCount > 0 {
		// Check if it's a Greenhouse iframe
		for i := 0; i < iframeCount; i++ {
			iframeSrc, _ := page.Locator("iframe").Nth(i).GetAttribute("src")
			if strings.Contains(iframeSrc, "greenhouse.io") || strings.Contains(iframeSrc, "boards.greenhouse.io") {
				log.Printf("Detected Greenhouse iframe application at: %s", iframeSrc)
				// Handle as Greenhouse application
				return s.handleGreenhouseApplication(page, userData, resumeFilePath, result)
			}
		}
	}

	// Look for specific job application buttons with priority order
	applySelectors := []string{
		// High priority - specific job application text
		"a:has-text('Apply for this role')",
		"button:has-text('Apply for this role')",
		"a:has-text('Apply for this position')",
		"button:has-text('Apply for this position')",
		"a:has-text('Apply Now')",
		"button:has-text('Apply Now')",
		"a:has-text('Apply for this job')",
		"button:has-text('Apply for this job')",
		"a:has-text('Submit Application')",
		"button:has-text('Submit Application')",
		
		// Medium priority - href-based application links
		"a[href*='/apply']",
		"a[href*='application']",
		"a[href*='job'][href*='apply']",
		
		// Specific company patterns
		"a[href*='greenhouse.io']",
		"a[href*='lever.co']",
		"a[href*='workday.com']",
		"a[href*='bamboohr.com']",
		"a[href*='apply'][class*='button']",
		"a[href*='apply'][class*='btn']",
		
		// Lower priority - generic apply buttons
		"button:has-text('Apply')",
		"a:has-text('Apply')",
		"input[value*='Apply']",
		"input[type='submit'][value*='Apply']",
		
		// Data attributes for modern frameworks
		"[data-testid*='apply']",
		"[data-cy*='apply']",
		"[data-automation*='apply']",
		"[aria-label*='Apply']",
		"[title*='Apply']",
	}

	var applyButtonFound bool
	for _, selector := range applySelectors {
		if exists, err := page.Locator(selector).First().IsVisible(); err == nil && exists {
			log.Printf("Found apply button with selector: %s", selector)
			
			// Get the text content to verify it's the right button
			buttonText, _ := page.Locator(selector).First().TextContent()
			log.Printf("Button text: '%s'", buttonText)
			
			err = page.Locator(selector).First().Click()
			if err != nil {
				log.Printf("Failed to click apply button: %v", err)
				continue
			}
			
			applyButtonFound = true
			log.Printf("Successfully clicked apply button: %s (text: '%s')", selector, buttonText)
			break
		}
	}
	
	if !applyButtonFound {
		log.Printf("No apply button found. Checking all buttons and links on page...")
		// Debug: Get all buttons and links for troubleshooting
		allButtons, _ := page.Locator("button").All()
		allLinks, _ := page.Locator("a").All()
		
		log.Printf("Found %d buttons on the page", len(allButtons))
		for i, button := range allButtons {
			if i >= 5 { break } // Limit logging
			text, _ := button.TextContent()
			log.Printf("Button %d: '%s'", i+1, text)
		}
		
		log.Printf("Found %d links on the page", len(allLinks))
		for i, link := range allLinks {
			if i >= 10 { break } // Limit logging
			text, _ := link.TextContent()
			href, _ := link.GetAttribute("href")
			if text != "" && (strings.Contains(strings.ToLower(text), "apply") || strings.Contains(strings.ToLower(href), "apply")) {
				log.Printf("Apply-related link %d: text='%s', href='%s'", i+1, text, href)
			}
		}
	}

	if !applyButtonFound {
		// Debug: Let's see what buttons are actually available on the page
		log.Printf("No apply button found, debugging available buttons...")
		
		// Get all buttons and links on the page for debugging
		allButtons, _ := page.Locator("button").All()
		allLinks, _ := page.Locator("a").All()
		
		log.Printf("Found %d buttons on the page", len(allButtons))
		for i, button := range allButtons {
			if i >= 10 { break } // Limit to first 10 for debugging
			text, _ := button.TextContent()
			innerHTML, _ := button.InnerHTML()
			classes, _ := button.GetAttribute("class")
			id, _ := button.GetAttribute("id")
			log.Printf("Button %d: text='%s', class='%s', id='%s', html='%s'", i+1, text, classes, id, innerHTML)
		}
		
		log.Printf("Found %d links on the page", len(allLinks))
		for i, link := range allLinks {
			if i >= 20 { break } // Check more links for debugging
			text, _ := link.TextContent()
			href, _ := link.GetAttribute("href")
			classes, _ := link.GetAttribute("class")
			id, _ := link.GetAttribute("id")
			
			// Log all links that might be apply-related (broader search)
			if strings.Contains(strings.ToLower(text), "apply") || 
			   strings.Contains(strings.ToLower(href), "apply") ||
			   strings.Contains(strings.ToLower(text), "submit") ||
			   strings.Contains(strings.ToLower(text), "join") ||
			   strings.Contains(strings.ToLower(href), "job") ||
			   strings.Contains(strings.ToLower(classes), "apply") ||
			   strings.Contains(strings.ToLower(classes), "button") ||
			   strings.Contains(strings.ToLower(classes), "btn") ||
			   strings.Contains(strings.ToLower(classes), "primary") ||
			   strings.Contains(strings.ToLower(classes), "cta") {
				log.Printf("Potential apply link %d: text='%s', href='%s', class='%s', id='%s'", i+1, text, href, classes, id)
			}
		}
		
		result.Success = false
				return result, nil
		result.Status = "external_application"
		result.Message = "Could not find apply button. Please apply manually on the company website."
		result.Success = false
				return result, nil
				return result, nil
	}

	// Wait for form elements to appear
	log.Printf("Waiting for application form to load...")
	// Use smart waiting - wait for form elements or page load
	page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(3000), // Max 3 seconds
	})
	
	// First check for iframes (Greenhouse often uses iframes)
	iframeCount, _ = page.Locator("iframe").Count()
	log.Printf("Found %d iframes on the page", iframeCount)
	
	// If there's an iframe, it might contain the form
	if iframeCount > 0 {
		log.Printf("Detected iframe - application form might be inside")
		// Try to get the iframe
		iframe := page.FrameLocator("iframe").First()
		
		// Check if form elements exist in the iframe
		iframeInputCount, _ := iframe.Locator("input[type='text'], input[type='email']").Count()
		log.Printf("Found %d input fields inside iframe", iframeInputCount)
		
		if iframeInputCount > 0 {
			log.Printf("Form detected inside iframe - switching context")
			// Note: We'll need to handle iframe differently
			// For now, just wait for it to load
			page.WaitForTimeout(2000)
		}
	}
	
	// Try to wait for common form elements to be visible
	formSelectors := []string{
		"input[type='text']",
		"input[type='email']",
		"input[name*='name']",
		"input[placeholder*='Name']",
		"form",
		"fieldset",
	}
	
	formFound := false
	for _, selector := range formSelectors {
		err := page.Locator(selector).First().WaitFor(playwright.LocatorWaitForOptions{
			State: playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(3000), // Increased to 5 second timeout
		})
		if err == nil {
			formFound = true
			log.Printf("Form element found: %s", selector)
			
			// Log how many of this type are visible
			count, _ := page.Locator(selector).Count()
			log.Printf("Total %s elements on page: %d", selector, count)
			break
		}
	}
	
	if !formFound {
		log.Printf("No form elements found after waiting - page might be loading slowly")
		// Give it more time
		page.WaitForTimeout(1500)
		
		// Try one more time to count all inputs
		inputCount, _ := page.Locator("input").Count()
		visibleInputCount, _ := page.Locator("input:visible").Count() 
		log.Printf("After additional wait - Total inputs: %d, Visible inputs: %d", inputCount, visibleInputCount)
	}

	// Check if we were redirected to an external application site or if this is a contact form
	currentURL := page.URL()
	log.Printf("Current URL after apply click: %s", currentURL)
	
	// Debug: Log page title and check if page loaded properly
	pageTitle, err := page.Title()
	if err != nil {
		log.Printf("Failed to get page title: %v", err)
	} else {
		log.Printf("Page title after apply click: '%s'", pageTitle)
	}
	
	// IMMEDIATELY try to fill any visible form fields
	log.Printf("=== ATTEMPTING TO FILL FORM FIELDS IMMEDIATELY ===")
	
	// Validate user data before attempting to fill
	if userData.FullName == "" || userData.FullName == "John Doe" {
		log.Printf("WARNING: User data appears to be default/empty - Name: %s", userData.FullName)
		result.Success = false
				return result, nil
		result.Status = "missing_user_data"
		result.Message = "User profile data is missing. Please complete your profile before applying."
		result.Success = false
				return result, nil
				return result, nil
	}
	
	log.Printf("User Profile Data: Name=%s, Email=%s, Phone=%s, LinkedIn=%s", 
		userData.FullName, userData.Email, userData.Phone, userData.LinkedIn)
	
	// Check if form is in an iframe (common for Greenhouse)
	iframeCount2, _ := page.Locator("iframe").Count()
	if iframeCount2 > 0 {
		log.Printf("Detected %d iframe(s) - checking for form inside iframe", iframeCount2)
		
		// Try to fill form in iframe
		iframe := page.FrameLocator("iframe").First()
		iframeFilledCount, err := s.fillIframeFields(iframe, userData, page)
		
		// Check if we have an error (meaning unknown fields were found)
		if err != nil {
			log.Printf("⚠️ STOPPING: Need user input for unknown dropdown fields: %v", err)
			result.Status = "user_input_required"
			// Use the detailed error message from the dropdown handler
			result.Message = err.Error()
			result.Success = false
			return result, nil
		}
		
		log.Printf("Filled %d fields in iframe (first pass)", iframeFilledCount)
		
		// Always do a second pass to catch any missed dropdowns
		// Greenhouse forms often have dropdowns that only become interactive after initial fill
		page.WaitForTimeout(500)
		additionalFields := s.fillRemainingDropdowns(iframe, userData, page)
		if additionalFields > 0 {
			log.Printf("Filled %d additional dropdown fields in second pass", additionalFields)
			iframeFilledCount += additionalFields
		}
		
		if iframeFilledCount > 0 {
			log.Printf("Successfully filled iframe form with total of %d fields", iframeFilledCount)
			result.FilledFields["iframe_fields"] = fmt.Sprintf("%d fields", iframeFilledCount)
			
			// Take screenshot after filling
			afterFillScreenshot, _ := s.saveScreenshot(page, "after_iframe_fill", result)
			if afterFillScreenshot != "" {
				result.ApplicationScreenshotKey = afterFillScreenshot
			}
			
			// Look for submit button in iframe
			s.submitIframeForm(iframe, page, result)
			
			// If submit was successful, result.Success will be true
			if result.Success {
				return result, nil
			}
			
			// If no submit button found or submission failed, continue with page
			log.Printf("No submit button found in iframe or submission failed, checking main page")
		}
	}
	
	// If no iframe or iframe filling failed, try regular form filling
	filledCount := s.fillCurrentStepFields(page, userData, resumeFilePath)
	log.Printf("Initial form fill attempt: Filled %d fields", filledCount)
	
	// Take screenshot AFTER filling the form
	log.Printf("Taking screenshot after form filling attempt...")
	afterClickScreenshotURL, err := s.saveScreenshot(page, "after_apply_click", result)
	if err != nil {
		log.Printf("Failed to save after-click screenshot: %v", err)
	} else {
		result.ApplicationScreenshotKey = afterClickScreenshotURL
	}
	
	// Check for ATS systems (including Greenhouse elements on the page)
	if strings.Contains(currentURL, "greenhouse") || strings.Contains(currentURL, "lever") || strings.Contains(currentURL, "workday") || strings.Contains(currentURL, "bamboohr") {
		log.Printf("Detected external ATS system in URL: %s", currentURL)
		// Already filled the form above, but continue with ATS flow for multi-step handling
		return s.handleATSApplication(page, userData, resumeFilePath, result)
	}
	
	// First check if URL indicates we're on an application page
	if strings.Contains(currentURL, "/apply") {
		log.Printf("URL contains '/apply' - checking for form elements...")
		
		// Look for ANY form fields that suggest this is an application
		formSelectors := []string{
			"input[type='text']",
			"input[type='email']", 
			"input[type='tel']",
			"textarea",
			"select",
			"input[type='file']",
			"form",
		}
		
		for _, selector := range formSelectors {
			if count, err := page.Locator(selector).Count(); err == nil && count > 0 {
				log.Printf("Found %d elements matching '%s' - treating as application form", count, selector)
				return s.handleATSApplication(page, userData, resumeFilePath, result)
			}
		}
	}
	
	// Enhanced Greenhouse detection - check for various Greenhouse indicators
	greenhouseSelectors := []string{
		"button:has-text('Autofill with Greenhouse')",
		"[class*='greenhouse']",
		"[data-greenhouse]",
		"form[action*='greenhouse']",
		"[data-testid*='greenhouse']",
		".application-form",
		"input[name='first_name']",
		"input[name='last_name']",
		"input[placeholder*='First Name']",
		"input[placeholder*='Last Name']",
		"fieldset legend:has-text('Apply for this job')",
		"h1:has-text('Apply for this position')",
		"h2:has-text('Apply for this position')",
		".job-application-form",
		// Additional Greenhouse patterns commonly found on company sites
		"input[name*='applicant']",
		"form[class*='application']",
		"div[class*='application-form']",
		"div[data-qa*='application']",
		"input[name='job_application[first_name]']",
		"input[name='job_application[last_name]']",
		"input[name='job_application[email]']",
		"input[name='job_application[phone]']",
		"select[name*='job_application']",
		"textarea[name*='job_application']",
		"input[data-qa*='input']",
		"form[data-qa*='form']",
		// Stripe-specific or common modern patterns
		"input[data-testid*='field']",
		"div[data-testid*='application']",
		"fieldset[data-qa]",
		"form div[class*='field']",
	}
	
	log.Printf("Checking %d Greenhouse selectors for ATS detection...", len(greenhouseSelectors))
	for i, selector := range greenhouseSelectors {
		if exists, err := page.Locator(selector).First().IsVisible(); err == nil && exists {
			log.Printf("Detected Greenhouse ATS elements with selector '%s' on page: %s", selector, currentURL)
			return s.handleATSApplication(page, userData, resumeFilePath, result)
		} else if i < 5 {
			// Log first few failed attempts for debugging
			log.Printf("Greenhouse selector %d '%s' not found", i+1, selector)
		}
	}
	log.Printf("No Greenhouse selectors matched - proceeding with generic form detection")
	
	// Check if this looks like a contact form instead of a job application
	pageTitle, _ = page.Title()
	pageContent, _ := page.Locator("body").TextContent()
	
	// Look for indicators that this is a contact form, not a job application
	contactIndicators := []string{
		"contact us", "get in touch", "reach out", "contact form", 
		"general inquiry", "sales", "support", "demo request",
	}
	
	isContactForm := false
	for _, indicator := range contactIndicators {
		if strings.Contains(strings.ToLower(pageTitle), indicator) || 
		   strings.Contains(strings.ToLower(pageContent), indicator) {
			isContactForm = true
			break
		}
	}
	
	if isContactForm {
		log.Printf("Detected contact form instead of job application: %s", currentURL)
		result.Success = false
				return result, nil
		result.Status = "contact_form_detected"
		result.Message = "This appears to be a contact form, not a job application. Please find the specific job posting and try again."
		result.Success = false
				return result, nil
				return result, nil
	}
	
	// Look for job application specific indicators
	jobAppIndicators := []string{
		"job application", "apply for", "career", "position", "role", 
		"resume upload", "cover letter", "employment", "work experience",
		"application form", "join our team", "vacancy",
	}
	
	isJobApplication := false
	for _, indicator := range jobAppIndicators {
		if strings.Contains(strings.ToLower(pageTitle), indicator) || 
		   strings.Contains(strings.ToLower(pageContent), indicator) {
			isJobApplication = true
			break
		}
	}
	
	if !isJobApplication {
		log.Printf("Cannot confirm this is a job application page: %s", currentURL)
		result.Success = false
				return result, nil
		result.Status = "uncertain_application_page"
		result.Message = "Cannot confirm this is a job application page. Please verify the URL leads to a specific job posting."
		result.Success = false
				return result, nil
				return result, nil
	}

	// Look for form fields to fill
	log.Printf("All checks passed - proceeding to fill generic form")
	return s.fillGenericForm(page, userData, resumeFilePath, result)
}

func (s *BrowserAutomationService) handleATSApplication(page playwright.Page, userData *UserProfileData, resumeFilePath string, result *AutomationResult) (*AutomationResult, error) {
	log.Printf("=== HANDLING ATS APPLICATION FORM ===")
	log.Printf("User data - Name: %s, Email: %s, Phone: %s", userData.FullName, userData.Email, userData.Phone)
	
	// Take initial screenshot of ATS form
	atsScreenshotURL, err := s.saveScreenshot(page, "ats_application_form", result)
	if err != nil {
		log.Printf("Failed to save ATS form screenshot: %v", err)
	} else {
		result.ApplicationScreenshotKey = atsScreenshotURL
		log.Printf("ATS form screenshot saved: %s", atsScreenshotURL)
	}
	
	// Wait for the form to fully load
	log.Printf("Waiting for form to fully load...")
	page.WaitForTimeout(1500)
	
	// Handle multi-step application process
	maxSteps := 5 // Maximum number of steps to process
	for step := 1; step <= maxSteps; step++ {
		log.Printf("=== PROCESSING STEP %d ===", step)
		
		// Fill current step's form fields
		filledFields := s.fillCurrentStepFields(page, userData, resumeFilePath)
		log.Printf("Step %d: Filled %d fields", step, filledFields)
		
		// If we filled some fields, track them
		if filledFields > 0 {
			result.FilledFields[fmt.Sprintf("step_%d_fields", step)] = fmt.Sprintf("%d fields filled", filledFields)
		}
		
		// Upload resume if file input is available and we have a resume file
	if resumeFilePath != "" {
		// Try multiple selectors for file upload
		fileUploadSelectors := []string{
			"input[type='file']",
			"input[type=file]",
			"input[name*='resume']",
			"input[name*='cv']",
			"input[name*='document']",
			"input[accept*='pdf']",
		}
		
		var uploadSuccess bool
		for _, selector := range fileUploadSelectors {
			resumeUpload := page.Locator(selector)
			if count, _ := resumeUpload.Count(); count > 0 {
				log.Printf("Found %d file input(s) with selector: %s", count, selector)
				for i := 0; i < count && !uploadSuccess; i++ {
					fileInput := resumeUpload.Nth(i)
					if visible, _ := fileInput.IsVisible(); visible {
						log.Printf("Uploading resume file to input %d: %s", i, resumeFilePath)
						err := fileInput.SetInputFiles(resumeFilePath)
						if err != nil {
							log.Printf("Failed to upload resume to input %d: %v", i, err)
						} else {
							log.Printf("✓ Successfully uploaded resume file")
							result.FilledFields["resume_upload"] = resumeFilePath
							uploadSuccess = true
							break
						}
					}
				}
			}
		}
		
		if !uploadSuccess {
			log.Printf("⚠ No visible file upload input found for resume")
		}
	}
		
		// Look for Next/Continue/Submit buttons
		nextButtonSelectors := []string{
			"button:has-text('Next')",
			"button:has-text('Continue')", 
			"button:has-text('Submit Application')",
			"button:has-text('Submit')",
			"input[type='submit']",
			"button[type='submit']",
			"a:has-text('Next')",
			"a:has-text('Continue')",
		}
		
		buttonClicked := false
		var buttonText string
		for _, selector := range nextButtonSelectors {
			if exists, err := page.Locator(selector).First().IsVisible(); err == nil && exists {
				buttonText, _ = page.Locator(selector).First().TextContent()
				log.Printf("Found button: '%s' with selector: %s", buttonText, selector)
				
				// Take screenshot before clicking button
				beforeClickURL, err := s.saveScreenshot(page, fmt.Sprintf("before_step_%d_submit", step), result)
				if err != nil {
					log.Printf("Failed to save before-click screenshot: %v", err)
				} else {
					result.ApplicationScreenshotKey = beforeClickURL
				}
				
				err = page.Locator(selector).First().Click()
				if err != nil {
					log.Printf("Failed to click button '%s': %v", buttonText, err)
					continue
				}
				
				log.Printf("Successfully clicked button: '%s'", buttonText)
				buttonClicked = true
				break
			}
		}
		
		if !buttonClicked {
			log.Printf("No next/submit button found on step %d", step)
			break
		}
		
		// Wait for page to load after button click
		page.WaitForTimeout(1500)
		
		// Take screenshot after button click
		afterClickURL, err := s.saveScreenshot(page, fmt.Sprintf("after_step_%d_click", step), result)
		if err != nil {
			log.Printf("Failed to save after-click screenshot: %v", err)
		} else {
			result.ApplicationScreenshotKey = afterClickURL
		}
		
		// Check if this was the final submit button and we got a success page
		if strings.Contains(strings.ToLower(buttonText), "submit") {
			log.Printf("Clicked submit button, checking for success confirmation...")
			
			// Wait for success page to load
			page.WaitForTimeout(2000)
			
			// Take final screenshot
			finalScreenshotURL, err := s.saveScreenshot(page, "final_submission_result", result)
			if err != nil {
				log.Printf("Failed to save final screenshot: %v", err)
			} else {
				result.ApplicationScreenshotKey = finalScreenshotURL
				log.Printf("Final submission screenshot saved: %s", finalScreenshotURL)
			}
			
			// Check for success indicators on the page
			if s.checkForSubmissionSuccess(page) {
				result.Success = true
				result.Status = "submitted"
				result.Message = fmt.Sprintf("ATS application submitted successfully after %d steps!", step)
				result.Success = false
				return result, nil
				return result, nil
			} else {
				result.Success = false
				return result, nil
				result.Status = "submission_uncertain"
				result.Message = fmt.Sprintf("Application submitted but success confirmation unclear after %d steps", step)
				result.Success = false
				return result, nil
				return result, nil
			}
		}
		
		// Continue to next step
		log.Printf("Proceeding to step %d", step+1)
	}
	
	result.Success = false
				return result, nil
	result.Status = "incomplete_application"
	result.Message = fmt.Sprintf("Application process incomplete after %d steps", maxSteps)
	result.Success = false
				return result, nil
				return result, nil
}

func (s *BrowserAutomationService) fillCurrentStepFields(page playwright.Page, userData *UserProfileData, resumeFilePath string) int {
	
	firstName := userData.FirstName
	if firstName == "" {
		firstName = strings.Split(userData.FullName, " ")[0]
	}
	lastName := userData.LastName
	if parts := strings.Split(userData.FullName, " "); len(parts) > 1 {
		lastName = strings.Join(parts[1:], " ")
	}
	
	// Debug: Log what data we're trying to fill
	log.Printf("Attempting to fill form with: Name=%s (First: %s, Last: %s), Email=%s, Phone=%s", 
		userData.FullName, firstName, lastName, userData.Email, userData.Phone)
	
	// Wait for form fields to be ready - longer wait for Greenhouse forms
	page.WaitForTimeout(1500)
	
	// Check if this is a Greenhouse form and wait for it to fully load
	if greenhouseForm, _ := page.Locator("form[id*='application'], div.application-form, input[name*='job_application']").First().IsVisible(); greenhouseForm {
		log.Printf("Detected Greenhouse form, waiting for fields to be ready...")
		page.WaitForSelector("input[type='text'], input[type='email']", playwright.PageWaitForSelectorOptions{
			State: playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(3000),
		})
	}
	
	// Try to fill fields by common patterns
	// First try the most specific, then fall back to more generic
	fieldMappings := []struct {
		selectors []string
		value     string
		fieldName string
	}{
		// First Name - expanded selectors for better matching
		{
			selectors: []string{
				// Greenhouse specific selectors first
				"input[name='job_application[first_name]']",
				"input[name='candidate[first_name]']",
				"#first_name",
				"input[data-qa='input-firstname']",
				"input[autocomplete='given-name']",
				// Standard selectors
				"input[name='first_name']",
				"input[name='firstName']", 
				"input[id='first_name']",
				"input[id='firstName']",
				"input[placeholder*='First']",
				"input[aria-label*='First']",
				"input[name*='first']:not([name*='last'])",
				// Label-based selectors - improved for Greenhouse
				"label:has-text('First Name') + input",
				"label:has-text('First Name') ~ input",
				"label:has-text('First Name') input",
				"div:has(label:has-text('First Name')) input",
				"div.field:has(label:has-text('First')) input",
				// Position-based fallback
				"form input[type='text']:nth-of-type(1)",
			},
			value: firstName,
			fieldName: "First Name",
		},
		// Last Name - expanded selectors for better matching
		{
			selectors: []string{
				// Greenhouse specific selectors first
				"input[name='job_application[last_name]']",
				"input[name='candidate[last_name]']",
				"#last_name",
				"input[data-qa='input-lastname']",
				"input[autocomplete='family-name']",
				// Standard selectors
				"input[name='last_name']",
				"input[name='lastName']",
				"input[id='last_name']", 
				"input[id='lastName']",
				"input[placeholder*='Last']",
				"input[aria-label*='Last']",
				"input[name*='last']:not([name*='first'])",
				// Label-based selectors - improved for Greenhouse
				"label:has-text('Last Name') + input",
				"label:has-text('Last Name') ~ input",
				"label:has-text('Last Name') input",
				"div:has(label:has-text('Last Name')) input",
				"div.field:has(label:has-text('Last')) input",
				// Position-based fallback
				"form input[type='text']:nth-of-type(2)",
			},
			value: lastName,
			fieldName: "Last Name",
		},
		// Email - expanded selectors
		{
			selectors: []string{
				"input[name='email']",
				"input[id='email']",
				"input[type='email']",
				"input[placeholder*='Email']",
				"input[aria-label*='Email']",
				"input[name*='email']",
				// Additional selectors
				"label:has-text('Email') + input",
				"label:has-text('Email') input",
				"div:has-text('Email') input",
				"fieldset:has-text('Email') input",
			},
			value: userData.Email,
			fieldName: "Email",
		},
		// Phone - expanded selectors
		{
			selectors: []string{
				"input[name='phone']",
				"input[name='phone_number']",
				"input[id='phone']",
				"input[type='tel']",
				"input[placeholder*='Phone']",
				"input[aria-label*='Phone']",
				"input[name*='phone']",
				// Additional selectors
				"label:has-text('Phone') + input",
				"label:has-text('Phone') input",
				"div:has-text('Phone') input",
				"fieldset:has-text('Phone') input",
			},
			value: userData.Phone,
			fieldName: "Phone",
		},
		// LinkedIn
		{
			selectors: []string{
				"input[name='linkedin']",
				"input[name*='linkedin']",
				"input[placeholder*='LinkedIn']",
				"input[aria-label*='LinkedIn']",
				"label:has-text('LinkedIn') + input",
				"label:has-text('LinkedIn') input",
				"div:has-text('LinkedIn') input",
			},
			value: userData.LinkedIn,
			fieldName: "LinkedIn",
		},
	}
	
	filledCount := 0
	for _, mapping := range fieldMappings {
		if mapping.value == "" {
			log.Printf("Skipping %s field - no value provided", mapping.fieldName)
			continue
		}
		
		filled := false
		for _, selector := range mapping.selectors {
			if filled {
				break
			}
			
			// Try to find all matching elements
			elements, err := page.Locator(selector).All()
			if err != nil || len(elements) == 0 {
				continue
			}
			
			// Try to fill the first visible one
			for _, element := range elements {
				if visible, _ := element.IsVisible(); visible {
					log.Printf("Attempting to fill %s field with selector '%s' with value: %s", mapping.fieldName, selector, mapping.value)
					
					// Clear the field first, then fill
					err = element.Clear()
					if err != nil {
						log.Printf("Warning: Could not clear field: %v", err)
					}
					
					err = element.Fill(mapping.value)
					if err == nil {
						log.Printf("✓ Successfully filled %s field", mapping.fieldName)
						filledCount++
						filled = true
						break
					} else {
						log.Printf("Failed to fill %s field: %v", mapping.fieldName, err)
					}
				}
			}
		}
		
		if !filled {
			log.Printf("⚠ Could not fill %s field - no matching selector found", mapping.fieldName)
		}
	}
	
	// New approach: Find all input fields and their associated labels
	// This is more robust for forms that use label text instead of names/IDs
	log.Printf("Attempting label-based field detection...")
	
	// Get all visible input fields
	allInputs, _ := page.Locator("input:visible").All()
	log.Printf("Found %d visible input fields on the page", len(allInputs))
	
	for _, input := range allInputs {
		// Check if already has value
		if value, _ := input.InputValue(); value != "" {
			continue
		}
		
		// Try to find the label for this input
		// Method 1: Check for label with 'for' attribute
		inputId, _ := input.GetAttribute("id")
		inputName, _ := input.GetAttribute("name")
		placeholder, _ := input.GetAttribute("placeholder")
		ariaLabel, _ := input.GetAttribute("aria-label")
		inputType, _ := input.GetAttribute("type")
		
		// Skip file and hidden inputs
		if inputType == "file" || inputType == "hidden" || inputType == "submit" || inputType == "button" {
			continue
		}
		
		var labelText string
		
		// Try to find label text
		if inputId != "" {
			// Look for label with matching 'for' attribute
			labelLocator := page.Locator(fmt.Sprintf("label[for='%s']", inputId))
			if exists, _ := labelLocator.First().IsVisible(); exists {
				labelText, _ = labelLocator.First().TextContent()
			}
		}
		
		// If no label found, try to find label that contains this input
		if labelText == "" {
			// Check if input is inside a label
			parentLabel := input.Locator("xpath=ancestor::label")
			if exists, _ := parentLabel.First().Count(); exists > 0 {
				labelText, _ = parentLabel.First().TextContent()
			}
		}
		
		// If still no label, check for nearby text elements
		if labelText == "" {
			// Look for preceding sibling text
			precedingText := input.Locator("xpath=preceding-sibling::*[1]")
			if exists, _ := precedingText.First().Count(); exists > 0 {
				labelText, _ = precedingText.First().TextContent()
			}
		}
		
		// Combine all field information for matching
		fieldInfo := strings.ToLower(labelText + " " + placeholder + " " + inputName + " " + inputId + " " + ariaLabel)
		fieldInfo = strings.TrimSpace(fieldInfo)
		
		log.Printf("Field analysis - Label: '%s', Name: '%s', ID: '%s', Placeholder: '%s', Type: '%s'", 
			labelText, inputName, inputId, placeholder, inputType)
		
		var valueToFill string
		var fieldType string
		
		// Determine what to fill based on field information
		if strings.Contains(fieldInfo, "first") && strings.Contains(fieldInfo, "name") && !strings.Contains(fieldInfo, "last") {
			valueToFill = firstName
			fieldType = "First Name"
		} else if strings.Contains(fieldInfo, "last") && strings.Contains(fieldInfo, "name") && !strings.Contains(fieldInfo, "first") {
			valueToFill = lastName
			fieldType = "Last Name"
		} else if strings.Contains(fieldInfo, "email") || inputType == "email" {
			valueToFill = userData.Email
			fieldType = "Email"
		} else if strings.Contains(fieldInfo, "phone") || strings.Contains(fieldInfo, "tel") || inputType == "tel" {
			valueToFill = userData.Phone
			fieldType = "Phone"
		} else if strings.Contains(fieldInfo, "linkedin") {
			valueToFill = userData.LinkedIn
			fieldType = "LinkedIn"
		} else if strings.Contains(fieldInfo, "portfolio") || strings.Contains(fieldInfo, "website") {
			valueToFill = userData.Portfolio
			fieldType = "Portfolio/Website"
		}
		
		if valueToFill != "" {
			log.Printf("Filling %s field (detected from: %s) with: %s", fieldType, fieldInfo, valueToFill)
			
			// Clear and fill the field
			input.Clear()
			if err := input.Fill(valueToFill); err == nil {
				log.Printf("✓ Successfully filled %s field", fieldType)
				filledCount++
			} else {
				log.Printf("Failed to fill %s field: %v", fieldType, err)
			}
		}
	}
	
	// Handle select/dropdown fields for non-iframe forms (like direct Greenhouse forms)
	allSelects, _ := page.Locator("select").All()
	log.Printf("=== DROPDOWN DETECTION: Found %d select elements on page ===", len(allSelects))
	
	for i, selectElem := range allSelects {
		// Check if visible
		isVisible, _ := selectElem.IsVisible()
		if !isVisible {
			continue
		}
		
		// Get select attributes
		selectName, _ := selectElem.GetAttribute("name")
		selectId, _ := selectElem.GetAttribute("id")
		ariaLabel, _ := selectElem.GetAttribute("aria-label")
		
		// Get label text - CRITICAL for identifying what the dropdown is for
		var labelText string
		if selectId != "" {
			labelElem := page.Locator(fmt.Sprintf("label[for='%s']", selectId))
			if exists, _ := labelElem.Count(); exists > 0 {
				labelText, _ = labelElem.First().TextContent()
			}
		}
		
		// Try parent label if not found
		if labelText == "" {
			parentLabel := selectElem.Locator("xpath=ancestor::label")
			if exists, _ := parentLabel.Count(); exists > 0 {
				labelText, _ = parentLabel.First().TextContent()
			}
		}
		
		// Try preceding sibling
		if labelText == "" {
			precedingLabel := selectElem.Locator("xpath=preceding-sibling::*[1]")
			if exists, _ := precedingLabel.Count(); exists > 0 {
				labelText, _ = precedingLabel.First().TextContent()
			}
		}
		
		// Check current value
		currentValue, _ := selectElem.InputValue()
		currentText, _ := selectElem.Locator("option:checked").TextContent()
		
		// Skip if already has a valid value (not "Select..." or similar)
		if currentValue != "" && currentValue != "0" && currentValue != "-1" && 
		   !strings.Contains(strings.ToLower(currentText), "select") && 
		   !strings.Contains(strings.ToLower(currentText), "choose") &&
		   !strings.Contains(strings.ToLower(currentText), "please") {
			log.Printf("Dropdown %d already has value: %s", i, currentText)
			continue
		}
		
		// Combine all field info for matching
		fieldInfo := strings.ToLower(labelText + " " + selectName + " " + selectId + " " + ariaLabel)
		log.Printf("Dropdown %d - Label: '%s', Name: '%s', Current: '%s'", i, labelText, selectName, currentText)
		
		// Determine what to select based on the label/field info
		var valueToSelect string
		
		if strings.Contains(fieldInfo, "gender") && strings.Contains(fieldInfo, "identity") {
			// Gender identity dropdown
			valueToSelect = "Prefer not to answer"
			if userData.Gender != "" {
				if strings.ToLower(userData.Gender) == "male" {
					valueToSelect = "Man"
				} else if strings.ToLower(userData.Gender) == "female" {
					valueToSelect = "Woman"
				}
			}
		} else if strings.Contains(fieldInfo, "racial") || strings.Contains(fieldInfo, "ethnic") {
			// Race/ethnicity dropdown
			valueToSelect = "Prefer not to answer"
			if userData.Ethnicity != "" {
				valueToSelect = userData.Ethnicity
			}
		} else if strings.Contains(fieldInfo, "sexual orientation") {
			// Sexual orientation dropdown
			valueToSelect = "Prefer not to answer"
		} else if strings.Contains(fieldInfo, "transgender") {
			// Transgender dropdown
			valueToSelect = "No"
		} else if strings.Contains(fieldInfo, "disability") || strings.Contains(fieldInfo, "chronic condition") {
			// Disability dropdown
			valueToSelect = "No"
			if userData.DisabilityStatus == "yes" {
				valueToSelect = "Yes"
			}
		} else if strings.Contains(fieldInfo, "veteran") || strings.Contains(fieldInfo, "armed forces") {
			// Veteran status dropdown
			valueToSelect = "I am not a protected veteran"
			if userData.VeteranStatus == "yes" {
				valueToSelect = "Yes"
			}
		} else if strings.Contains(fieldInfo, "country") {
			valueToSelect = "United States"
		} else if strings.Contains(fieldInfo, "state") {
			valueToSelect = userData.State
			if valueToSelect == "" {
				valueToSelect = "California"
			}
		}
		
		// Try to select the value
		if valueToSelect != "" {
			options, _ := selectElem.Locator("option").All()
			selected := false
			
			// First try exact match
			for _, option := range options {
				optionText, _ := option.TextContent()
				optionValue, _ := option.GetAttribute("value")
				
				if strings.TrimSpace(optionText) == valueToSelect {
					_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{optionValue}})
					if err == nil {
						log.Printf("✓ Selected '%s' in dropdown: %s", optionText, labelText)
						filledCount++
						selected = true
						break
					}
				}
			}
			
			// If not found, try partial match
			if !selected {
				for _, option := range options {
					optionText, _ := option.TextContent()
					optionValue, _ := option.GetAttribute("value")
					optionTextLower := strings.ToLower(strings.TrimSpace(optionText))
					
					if strings.Contains(optionTextLower, "prefer not") || 
					   strings.Contains(optionTextLower, "decline") ||
					   (strings.Contains(fieldInfo, "veteran") && strings.Contains(optionTextLower, "not a protected")) {
						_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{optionValue}})
						if err == nil {
							log.Printf("✓ Selected privacy option '%s' in dropdown: %s", optionText, labelText)
							filledCount++
							selected = true
							break
						}
					}
				}
			}
			
			if !selected {
				log.Printf("⚠ Could not find matching option for '%s' in dropdown: %s", valueToSelect, labelText)
			}
		}
	}
	
	// Handle checkboxes for acknowledgments
	checkboxes, _ := page.Locator("input[type='checkbox']:visible").All()
	if len(checkboxes) > 0 {
		log.Printf("Found %d visible checkboxes", len(checkboxes))
		for i, checkbox := range checkboxes {
			checkboxName, _ := checkbox.GetAttribute("name")
			fieldInfo := strings.ToLower(checkboxName)
			
			// Check common acknowledgment checkboxes
			if strings.Contains(fieldInfo, "acknowledge") || 
			   strings.Contains(fieldInfo, "agree") ||
			   strings.Contains(fieldInfo, "consent") {
				if checked, _ := checkbox.IsChecked(); !checked {
					checkbox.Check()
					log.Printf("✓ Checked acknowledgment checkbox %d", i)
					filledCount++
				}
			}
		}
	}
	
	log.Printf("Total fields filled in this step: %d", filledCount)
	
	// Use working dropdown handler for React Select components
	if err := WorkingDropdownHandler(page, userData); err != nil {
		log.Printf("Working dropdown handler error: %v", err)
	}
	
	// Ensure resume is uploaded if path is provided
	if resumeFilePath != "" {
		if err := EnsureResumeUpload(page, resumeFilePath); err != nil {
			log.Printf("Resume upload verification error: %v", err)
		}
	}
	
	return filledCount
}

func (s *BrowserAutomationService) fillIframeFields(iframe playwright.FrameLocator, userData *UserProfileData, page playwright.Page) (int, error) {
	log.Printf("Attempting to fill fields inside iframe")
	
	firstName := userData.FirstName
	if firstName == "" {
		firstName = strings.Split(userData.FullName, " ")[0]
	}
	lastName := userData.LastName
	if parts := strings.Split(userData.FullName, " "); len(parts) > 1 {
		lastName = strings.Join(parts[1:], " ")
	}
	
	filledCount := 0
	
	// First, try to get all visible inputs in the iframe and analyze them
	allInputs, err := iframe.Locator("input:visible").All()
	if err != nil {
		log.Printf("Error getting inputs from iframe: %v", err)
		return 0, nil
	}
	
	log.Printf("Found %d visible inputs in iframe - analyzing each one", len(allInputs))
	
	// Analyze each input field
	for i, input := range allInputs {
		// Get attributes to understand what this field is for
		inputType, _ := input.GetAttribute("type")
		inputName, _ := input.GetAttribute("name")
		inputId, _ := input.GetAttribute("id")
		placeholder, _ := input.GetAttribute("placeholder")
		ariaLabel, _ := input.GetAttribute("aria-label")
		
		// Skip certain input types
		if inputType == "hidden" || inputType == "submit" || inputType == "button" || inputType == "checkbox" || inputType == "radio" {
			continue
		}
		
		// Check if field already has value
		currentValue, _ := input.InputValue()
		if currentValue != "" {
			log.Printf("Input %d already has value, skipping", i)
			continue
		}
		
		// Log field details for debugging
		log.Printf("Input %d: type='%s', name='%s', id='%s', placeholder='%s', aria-label='%s'", 
			i, inputType, inputName, inputId, placeholder, ariaLabel)
		
		// Combine all attributes to determine field purpose
		fieldInfo := strings.ToLower(inputName + " " + inputId + " " + placeholder + " " + ariaLabel)
		
		var valueToFill string
		var fieldDescription string
		
		// Determine what to fill based on field information
		if (strings.Contains(fieldInfo, "first") && strings.Contains(fieldInfo, "name")) || 
		   strings.Contains(fieldInfo, "firstname") || 
		   strings.Contains(fieldInfo, "first_name") {
			valueToFill = firstName
			fieldDescription = "First Name"
		} else if (strings.Contains(fieldInfo, "last") && strings.Contains(fieldInfo, "name")) || 
		          strings.Contains(fieldInfo, "lastname") || 
		          strings.Contains(fieldInfo, "last_name") {
			valueToFill = lastName
			fieldDescription = "Last Name"
		} else if strings.Contains(fieldInfo, "email") || inputType == "email" {
			valueToFill = userData.Email
			fieldDescription = "Email"
		} else if strings.Contains(fieldInfo, "phone") || strings.Contains(fieldInfo, "tel") || inputType == "tel" {
			valueToFill = userData.Phone
			fieldDescription = "Phone"
		} else if strings.Contains(fieldInfo, "linkedin") {
			valueToFill = userData.LinkedIn
			fieldDescription = "LinkedIn"
		} else if strings.Contains(fieldInfo, "website") || strings.Contains(fieldInfo, "portfolio") {
			valueToFill = userData.Portfolio
			fieldDescription = "Portfolio/Website"
		} else if inputType == "text" && i == 0 {
			// First text field is often first name
			valueToFill = firstName
			fieldDescription = "First Name (assumed - first text field)"
		} else if inputType == "text" && i == 1 {
			// Second text field is often last name
			valueToFill = lastName
			fieldDescription = "Last Name (assumed - second text field)"
		} else if strings.Contains(strings.ToLower(ariaLabel), "employer") || strings.Contains(strings.ToLower(ariaLabel), "company") {
			// Current/Previous employer question
			if len(userData.Experience) > 0 {
				valueToFill = userData.Experience[0].Company
			} else {
				valueToFill = "Self-employed"
			}
			fieldDescription = "Current/Previous Employer"
		} else if strings.Contains(strings.ToLower(ariaLabel), "job title") || strings.Contains(strings.ToLower(ariaLabel), "position") {
			// Current/Previous job title question
			if len(userData.Experience) > 0 {
				valueToFill = userData.Experience[0].Title
			} else {
				valueToFill = "Software Engineer"
			}
			fieldDescription = "Job Title"
		} else if strings.Contains(strings.ToLower(ariaLabel), "school") || strings.Contains(strings.ToLower(ariaLabel), "university") {
			// School/University question
			if len(userData.Education) > 0 {
				valueToFill = userData.Education[0].Institution
			} else {
				valueToFill = "University"
			}
			fieldDescription = "School/University"
		} else if strings.Contains(strings.ToLower(ariaLabel), "degree") || strings.Contains(strings.ToLower(ariaLabel), "qualification") {
			// Degree question
			if len(userData.Education) > 0 {
				valueToFill = userData.Education[0].Degree
			} else {
				valueToFill = "Bachelor's Degree"
			}
			fieldDescription = "Degree"
		} else if strings.Contains(inputId, "question_") && inputType == "text" {
			// Generic question field - try to be helpful
			if strings.Contains(strings.ToLower(ariaLabel), "linkedin") {
				valueToFill = userData.LinkedIn
				fieldDescription = "LinkedIn URL"
			} else if strings.Contains(strings.ToLower(ariaLabel), "website") || strings.Contains(strings.ToLower(ariaLabel), "portfolio") {
				valueToFill = userData.Portfolio
				fieldDescription = "Portfolio/Website"
			} else if strings.Contains(strings.ToLower(ariaLabel), "github") {
				valueToFill = userData.Portfolio
				fieldDescription = "GitHub/Portfolio"
			} else if ariaLabel != "" {
				// Log that we found a question but couldn't fill it
				log.Printf("Found custom question but no data to fill: '%s'", ariaLabel)
			}
		}
		
		if valueToFill != "" {
			log.Printf("Attempting to fill %s (input %d) with: %s", fieldDescription, i, valueToFill)
			if err := input.Fill(valueToFill); err == nil {
				log.Printf("✓ Successfully filled %s in iframe", fieldDescription)
				filledCount++
			} else {
				log.Printf("Failed to fill %s: %v", fieldDescription, err)
			}
		} else {
			log.Printf("Could not determine purpose of input %d - skipping", i)
		}
	}
	
	// Now handle select/dropdown fields
	// Try multiple approaches to find dropdowns (native selects and custom dropdowns)
	allSelects, _ := iframe.Locator("select").All()
	log.Printf("Found %d select elements in iframe", len(allSelects))
	
	// IMPORTANT: Also look for Greenhouse-specific dropdown patterns
	// Greenhouse often uses input elements with readonly attribute for dropdowns
	greenhouseDropdowns, _ := iframe.Locator("input[readonly]:has-text('Select'), input[placeholder*='Select']").All()
	log.Printf("Found %d Greenhouse-style dropdown inputs", len(greenhouseDropdowns))
	
	// Handle Greenhouse dropdown inputs first
	for i, dropdownInput := range greenhouseDropdowns {
		isVisible, _ := dropdownInput.IsVisible()
		if !isVisible {
			continue
		}
		
		// Get the current value
		currentValue, _ := dropdownInput.InputValue()
		placeholder, _ := dropdownInput.GetAttribute("placeholder")
		
		// Check if it's already filled
		if currentValue != "" && !strings.Contains(strings.ToLower(currentValue), "select") {
			log.Printf("Dropdown input %d already has value: %s", i, currentValue)
			continue
		}
		
		// Get parent or associated label to understand what this dropdown is for
		var fieldLabel string
		parentDiv := dropdownInput.Locator("xpath=ancestor::div[contains(@class,'field') or contains(@class,'form-group')][1]")
		if parentDiv != nil {
			labelElem := parentDiv.Locator("label").First()
			if labelElem != nil {
				fieldLabel, _ = labelElem.TextContent()
			}
		}
		
		fieldInfo := strings.ToLower(fieldLabel + " " + placeholder)
		log.Printf("Dropdown input %d: label='%s', placeholder='%s'", i, fieldLabel, placeholder)
		
		// Click the input to open dropdown
		if err := dropdownInput.Click(); err == nil {
			log.Printf("Clicked dropdown input to open menu")
			page.WaitForTimeout(500)
			
			// Determine what to select based on field
			var searchText string
			if strings.Contains(fieldInfo, "country") || strings.Contains(fieldInfo, "location") {
				searchText = "United States"
			} else if strings.Contains(fieldInfo, "gender") {
				searchText = userData.Gender
				if searchText == "" {
					searchText = "Prefer not to"
				}
			} else if strings.Contains(fieldInfo, "race") || strings.Contains(fieldInfo, "ethnicity") {
				searchText = userData.Ethnicity
				if searchText == "" {
					searchText = "Prefer not to"
				}
			} else if strings.Contains(fieldInfo, "veteran") {
				searchText = "not a veteran"
			} else if strings.Contains(fieldInfo, "disability") {
				searchText = "not wish to"
			} else {
				// For unknown fields, try to select first non-empty option
				firstOption := iframe.Locator("div[role='option'], li[role='option'], div[class*='option']").First()
				if firstOption != nil {
					if visible, _ := firstOption.IsVisible(); visible {
						firstOption.Click()
						log.Printf("Selected first available option for unknown dropdown")
						filledCount++
						page.WaitForTimeout(300)
						continue
					}
				}
			}
			
			// Look for the option to select
			if searchText != "" {
				optionSelectors := []string{
					fmt.Sprintf("*[role='option']:has-text('%s')", searchText),
					fmt.Sprintf("li:has-text('%s')", searchText),
					fmt.Sprintf("div:has-text('%s'):not(input)", searchText),
				}
				
				for _, selector := range optionSelectors {
					option := iframe.Locator(selector).First()
					if option != nil {
						if visible, _ := option.IsVisible(); visible {
							if err := option.Click(); err == nil {
								log.Printf("✓ Selected '%s' in dropdown", searchText)
								filledCount++
								page.WaitForTimeout(300)
								break
							}
						}
					}
				}
			}
		}
	}
	
	for i, selectElem := range allSelects {
		// Check if select is visible
		isVisible, _ := selectElem.IsVisible()
		if !isVisible {
			log.Printf("Select %d is not visible, skipping", i)
			continue
		}
		
		// Get attributes to understand what this select is for
		selectName, _ := selectElem.GetAttribute("name")
		selectId, _ := selectElem.GetAttribute("id")
		ariaLabel, _ := selectElem.GetAttribute("aria-label")
		
		// Try to find the label text for this select
		labelText := ""
		// Try to find associated label
		if selectId != "" {
			if label, err := iframe.Locator(fmt.Sprintf("label[for='%s']", selectId)).TextContent(); err == nil {
				labelText = label
			}
		}
		// Try to find parent label
		if labelText == "" {
			if parent, err := selectElem.Locator("xpath=ancestor::label").TextContent(); err == nil {
				labelText = parent
			}
		}
		// Try to find preceding label
		if labelText == "" {
			if preceding, err := selectElem.Locator("xpath=preceding-sibling::label[1]").TextContent(); err == nil {
				labelText = preceding
			}
		}
		// Try to find parent div with label
		if labelText == "" {
			if parentDiv, err := selectElem.Locator("xpath=ancestor::div[contains(@class,'field') or contains(@class,'form-group')][1]//label").TextContent(); err == nil {
				labelText = parentDiv
			}
		}
		
		// Check if already has a value selected
		currentValue, _ := selectElem.InputValue()
		if currentValue != "" && currentValue != "0" && currentValue != "-1" && !strings.Contains(strings.ToLower(currentValue), "select") && !strings.Contains(strings.ToLower(currentValue), "please") && !strings.Contains(strings.ToLower(currentValue), "choose") {
			log.Printf("Select %d already has value '%s', skipping", i, currentValue)
			continue
		}
		
		fieldInfo := strings.ToLower(selectName + " " + selectId + " " + ariaLabel + " " + labelText)
		log.Printf("Select %d: name='%s', id='%s', aria-label='%s', label='%s'", i, selectName, selectId, ariaLabel, labelText)
		
		var valueToSelect string
		var fieldDescription string
		
		// Determine what to select based on field information and label
		if strings.Contains(fieldInfo, "country") || strings.Contains(fieldInfo, "nation") || strings.Contains(fieldInfo, "location") {
			valueToSelect = userData.Country
			if valueToSelect == "" {
				valueToSelect = "United States"
			}
			fieldDescription = "Country/Location"
			
			// Special handling for country - try to find US/United States option
			options, _ := selectElem.Locator("option").All()
			foundCountry := false
			
			// Country variants to try
			countryVariants := []string{"United States", "USA", "US", "United States of America", "U.S.A.", "U.S.", "America"}
			
			for _, option := range options {
				optionText, _ := option.TextContent()
				optionValue, _ := option.GetAttribute("value")
				optionTextClean := strings.TrimSpace(optionText)
				optionValueClean := strings.TrimSpace(optionValue)
				
				// Check each variant
				for _, variant := range countryVariants {
					if strings.EqualFold(optionTextClean, variant) || strings.EqualFold(optionValueClean, variant) ||
					   (strings.Contains(strings.ToLower(optionTextClean), "united") && strings.Contains(strings.ToLower(optionTextClean), "states")) {
						_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{optionValue}})
						if err == nil {
							log.Printf("✓ Successfully selected country: %s (matched: %s)", optionTextClean, variant)
							filledCount++
							foundCountry = true
							break
						}
					}
				}
				if foundCountry {
					break
				}
			}
			
			if !foundCountry {
				log.Printf("⚠ Could not find US/United States option in country dropdown")
				// Log first few options for debugging
				for j, option := range options {
					if j > 5 { break }
					optionText, _ := option.TextContent()
					optionValue, _ := option.GetAttribute("value")
					log.Printf("  Available option %d: text='%s', value='%s'", j, strings.TrimSpace(optionText), optionValue)
				}
			}
			continue // Skip the generic selection logic below
		} else if strings.Contains(fieldInfo, "state") || strings.Contains(fieldInfo, "province") {
			valueToSelect = userData.State
			if valueToSelect == "" {
				valueToSelect = "California" // Default state
			}
			fieldDescription = "State/Province"
		} else if strings.Contains(fieldInfo, "gender") {
			valueToSelect = userData.Gender
			if valueToSelect == "" {
				valueToSelect = "prefer_not_to_say"
			}
			fieldDescription = "Gender"
		} else if strings.Contains(fieldInfo, "veteran") {
			valueToSelect = userData.VeteranStatus
			if valueToSelect == "" {
				valueToSelect = "no"
			}
			fieldDescription = "Veteran Status"
		} else if strings.Contains(fieldInfo, "disability") {
			valueToSelect = userData.DisabilityStatus
			if valueToSelect == "" {
				valueToSelect = "no"
			}
			fieldDescription = "Disability Status"
		} else if strings.Contains(fieldInfo, "ethnicity") || strings.Contains(fieldInfo, "race") {
			valueToSelect = userData.Ethnicity
			if valueToSelect == "" {
				valueToSelect = "prefer_not_to_say"
			}
			fieldDescription = "Ethnicity/Race"
		} else if strings.Contains(fieldInfo, "authorization") || strings.Contains(fieldInfo, "work") {
			if userData.WorkAuthorization == "yes" {
				valueToSelect = "authorized"
			} else if userData.RequiresSponsorship {
				valueToSelect = "sponsorship"
			}
			fieldDescription = "Work Authorization"
		}
		
		if valueToSelect != "" {
			log.Printf("Attempting to select %s in dropdown %d with value: %s", fieldDescription, i, valueToSelect)
			
			// Try to select by visible text first
			options, _ := selectElem.Locator("option").All()
			selected := false
			
			// First pass: exact match
			for _, option := range options {
				optionText, _ := option.TextContent()
				optionValue, _ := option.GetAttribute("value")
				optionTextLower := strings.ToLower(strings.TrimSpace(optionText))
				optionValueLower := strings.ToLower(strings.TrimSpace(optionValue))
				valueToSelectLower := strings.ToLower(strings.TrimSpace(valueToSelect))
				
				// Exact match
				if optionTextLower == valueToSelectLower || optionValueLower == valueToSelectLower {
					_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{optionValue}})
					if err == nil {
						log.Printf("✓ Successfully selected %s in dropdown (exact match)", fieldDescription)
						filledCount++
						selected = true
						break
					}
				}
			}
			
			// Second pass: flexible matching for countries
		// Flexible matching for gender
			if !selected && fieldDescription == "Gender" {
				genderVariants := map[string][]string{
					"male": {"m", "man", "male", "Male", "M"},
					"female": {"f", "woman", "female", "Female", "F"},
					"other": {"other", "non-binary", "Other"},
					"prefer_not_to_say": {"prefer not", "decline", "not specified", "Prefer not to answer"},
				}
				
				for _, option := range options {
					optionText, _ := option.TextContent()
					optionValue, _ := option.GetAttribute("value")
					optionTextLower := strings.ToLower(strings.TrimSpace(optionText))
					
					for baseGender, variants := range genderVariants {
						if strings.Contains(strings.ToLower(valueToSelect), baseGender) {
							for _, variant := range variants {
								if strings.Contains(optionTextLower, strings.ToLower(variant)) {
									_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{optionValue}})
									if err == nil {
										log.Printf("✓ Successfully selected gender: %s", optionText)
										filledCount++
										selected = true
										break
									}
								}
							}
						}
						if selected { break }
					}
					if selected { break }
				}
			}
			if !selected && fieldDescription == "Country" {
				countryVariants := map[string][]string{
					"united states": {"usa", "us", "united states of america", "u.s.a.", "u.s.", "america"},
					"united kingdom": {"uk", "gb", "great britain", "england"},
					"canada": {"ca", "can"},
				}
				
				valueToSelectLower := strings.ToLower(strings.TrimSpace(valueToSelect))
				variants := []string{valueToSelectLower}
				if alts, ok := countryVariants[strings.ToLower(valueToSelect)]; ok {
					variants = append(variants, alts...)
				}
				
				for _, option := range options {
					optionText, _ := option.TextContent()
					optionValue, _ := option.GetAttribute("value")
					optionTextLower := strings.ToLower(strings.TrimSpace(optionText))
					optionValueLower := strings.ToLower(strings.TrimSpace(optionValue))
					
					for _, variant := range variants {
						if optionTextLower == variant || optionValueLower == variant ||
						   strings.Contains(optionTextLower, variant) || strings.Contains(optionValueLower, variant) {
							_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{optionValue}})
							if err == nil {
								log.Printf("✓ Successfully selected %s in dropdown (variant match: %s)", fieldDescription, variant)
								filledCount++
								selected = true
								break
							}
						}
					}
					if selected {
						break
					}
				}
			}
			
			// Third pass: partial match for other fields
			if !selected {
				for _, option := range options {
					optionText, _ := option.TextContent()
					optionValue, _ := option.GetAttribute("value")
					
					// Try to match by text or value (partial match)
					if strings.Contains(strings.ToLower(optionText), strings.ToLower(valueToSelect)) ||
					   strings.Contains(strings.ToLower(optionValue), strings.ToLower(valueToSelect)) {
						_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{optionValue}})
						if err == nil {
							log.Printf("✓ Successfully selected %s in dropdown (partial match)", fieldDescription)
							filledCount++
							selected = true
							break
						}
					}
				}
			}
			
			if !selected {
				log.Printf("⚠ Could not find matching option for %s with value '%s'", fieldDescription, valueToSelect)
				// Log available options for debugging
				if len(options) > 0 && len(options) < 20 {
					log.Printf("Available options:")
					for j, option := range options {
						if j > 10 { break } // Limit logging
						optionText, _ := option.TextContent()
						optionValue, _ := option.GetAttribute("value")
						log.Printf("  - text: '%s', value: '%s'", strings.TrimSpace(optionText), optionValue)
					}
				}
			}
		}
	}
	
	// Call the V6 iframe dropdown handler - ultra-fast without timeouts
	log.Printf("Calling HandleIframeDropdownsV6 for dropdown components...")
	if err := HandleIframeDropdownsV6(iframe, userData); err != nil {
		log.Printf("⚠️ STOPPING: HandleIframeDropdownsV6 found unknown fields: %v", err)
		// Return the error directly so it can be used in the calling function
		return 0, err
	} else {
		log.Printf("HandleIframeDropdownsV6 completed successfully")
		// Assume it filled some fields - we can't easily count them
		filledCount += 1
	}
	
	// Call Stripe-specific dropdown handler for remaining unfilled dropdowns
	log.Printf("Calling HandleStripeSpecificDropdowns for remaining dropdowns...")
	if err := HandleStripeSpecificDropdowns(iframe, userData); err != nil {
		log.Printf("Warning: HandleStripeSpecificDropdowns had issues: %v", err)
	} else {
		log.Printf("HandleStripeSpecificDropdowns completed successfully")
	}
	log.Printf("Continuing after Stripe dropdown handler...")
	
	// Skip custom Greenhouse dropdowns for now - they might be causing hangs
	log.Printf("Skipping custom Greenhouse dropdown check - moving to end of fillIframeFields")
	return filledCount, nil
	
	// Handle custom Greenhouse dropdowns (divs that act like dropdowns)
	// These are often used for country selection in Greenhouse forms
	log.Printf("Looking for custom Greenhouse dropdown elements...")
	
	// Look for specific dropdown patterns only
	customDropdownSelectors := []string{
		"div[aria-haspopup='listbox']",
		"div[role='combobox']",
		"button[aria-haspopup='listbox']",
	}
	
	for _, selector := range customDropdownSelectors {
		customDropdowns, _ := iframe.Locator(selector).All()
		if len(customDropdowns) > 0 {
			log.Printf("Found %d custom dropdowns with selector %s", len(customDropdowns), selector)
			
			for i, dropdown := range customDropdowns {
				// Get text to identify what this dropdown is for
				dropdownText, _ := dropdown.TextContent()
				ariaLabel, _ := dropdown.GetAttribute("aria-label")
				
				// Check if this looks like a country dropdown or any empty dropdown
				fieldInfo := strings.ToLower(dropdownText + " " + ariaLabel)
				isCountryDropdown := strings.Contains(fieldInfo, "country") || strings.Contains(fieldInfo, "location")
				isEmptyDropdown := strings.Contains(fieldInfo, "select") || strings.Contains(fieldInfo, "choose") || strings.Contains(fieldInfo, "please")
				
				if isCountryDropdown || isEmptyDropdown {
					if isCountryDropdown {
						log.Printf("Custom dropdown %d appears to be for country selection", i)
					} else {
						log.Printf("Custom dropdown %d appears to be empty/unselected", i)
					}
					
					// Click the dropdown to open it
					if err := dropdown.Click(); err == nil {
						log.Printf("Clicked custom dropdown to open it")
						page.WaitForTimeout(1000) // Wait longer for dropdown to open
						
						// Look for United States option in the dropdown menu
						// Try multiple patterns as Greenhouse may use different structures
						usOptionSelectors := []string{
							"div:text-is('United States')",
							"div:has-text('United States'):not([aria-haspopup])",
							"li:has-text('United States')",
							"span:text-is('United States')",
							"*[role='option']:has-text('United States')",
							"div[id*='option']:has-text('United States')",
							"div[class*='option']:has-text('United States')",
						}
						
						foundOption := false
						for _, optionSelector := range usOptionSelectors {
							options, err := iframe.Locator(optionSelector).All()
							if err == nil && len(options) > 0 {
								log.Printf("Found %d options matching selector %s", len(options), optionSelector)
								for _, opt := range options {
									if visible, _ := opt.IsVisible(); visible {
										if err := opt.Click(); err == nil {
											log.Printf("✓ Successfully selected United States from custom dropdown")
											filledCount++
											foundOption = true
											page.WaitForTimeout(500) // Wait for selection to register
											break
										}
									}
								}
								if foundOption {
									break
								}
							}
						}
						
						if !foundOption {
							// If we didn't find United States, log what options we can see
							allOptions, _ := iframe.Locator("*[role='option'], div[id*='option'], li").All()
							log.Printf("Could not find United States option. Found %d total options in dropdown", len(allOptions))
							if len(allOptions) > 0 && len(allOptions) < 10 {
								for j, opt := range allOptions {
									optText, _ := opt.TextContent()
									log.Printf("  Option %d: %s", j, strings.TrimSpace(optText))
								}
							}
						}
					} else {
						log.Printf("Failed to click dropdown: %v", err)
					}
				}
			}
		}
	}
	
	// Handle checkboxes in iframe
	checkboxes, _ := iframe.Locator("input[type='checkbox']:visible").All()
	log.Printf("Found %d visible checkboxes in iframe", len(checkboxes))
	
	for i, checkbox := range checkboxes {
		checkboxName, _ := checkbox.GetAttribute("name")
		checkboxId, _ := checkbox.GetAttribute("id")
		checkboxValue, _ := checkbox.GetAttribute("value")
		
		// Try to find the label text for this checkbox
		labelText := ""
		if checkboxId != "" {
			if label, err := iframe.Locator(fmt.Sprintf("label[for='%s']", checkboxId)).TextContent(); err == nil {
				labelText = label
			}
		}
		// Try parent label
		if labelText == "" {
			if parent, err := checkbox.Locator("xpath=ancestor::label").TextContent(); err == nil {
				labelText = parent
			}
		}
		// Try following label
		if labelText == "" {
			if following, err := checkbox.Locator("xpath=following-sibling::label[1]").TextContent(); err == nil {
				labelText = following
			}
		}
		
		fieldInfo := strings.ToLower(checkboxName + " " + checkboxId + " " + checkboxValue + " " + labelText)
		log.Printf("Checkbox %d: name='%s', id='%s', value='%s', label='%s'", i, checkboxName, checkboxId, checkboxValue, labelText)
		
		// Check if already checked
		if checked, _ := checkbox.IsChecked(); checked {
			log.Printf("Checkbox %d already checked, skipping", i)
			continue
		}
		
		// Determine if we should check this box
		shouldCheck := false
		checkReason := ""
		
		// Check for US/United States selection - be more flexible
		labelLower := strings.ToLower(labelText)
		if labelLower == "us" || labelLower == "usa" || labelLower == "united states" ||
		   labelLower == "u.s." || labelLower == "u.s.a." || labelLower == "america" ||
		   strings.Contains(fieldInfo, "united states") || strings.Contains(fieldInfo, "usa") || 
		   strings.Contains(fieldInfo, "u.s.") || (strings.Contains(fieldInfo, "us") && strings.Contains(fieldInfo, "citizen")) {
			shouldCheck = true
			checkReason = "US/United States selection"
		} else if strings.Contains(fieldInfo, "authorize") || strings.Contains(fieldInfo, "authorized") ||
		          strings.Contains(fieldInfo, "legally") || strings.Contains(fieldInfo, "eligible") {
			// Work authorization checkboxes
			if userData.WorkAuthorization == "yes" {
				shouldCheck = true
				checkReason = "Work authorization"
			}
		} else if strings.Contains(fieldInfo, "acknowledge") || strings.Contains(fieldInfo, "agree") ||
		          strings.Contains(fieldInfo, "consent") || strings.Contains(fieldInfo, "confirm") ||
		          strings.Contains(fieldInfo, "accept") {
			// General acknowledgment checkboxes
			shouldCheck = true
			checkReason = "Acknowledgment/Agreement"
		} else if strings.Contains(fieldInfo, "privacy") || strings.Contains(fieldInfo, "terms") {
			// Privacy and terms checkboxes
			shouldCheck = true
			checkReason = "Privacy/Terms acceptance"
		} else if strings.Contains(checkboxName, "question") && (labelLower == "us" || labelLower == "usa" || labelLower == "united states") {
			// This is likely a country selection checkbox list
			shouldCheck = true
			checkReason = "Country selection (US)"
		}
		
		if shouldCheck {
			err := checkbox.Check()
			if err == nil {
				log.Printf("✓ Checked checkbox %d for: %s", i, checkReason)
				filledCount++
			} else {
				log.Printf("Failed to check checkbox %d: %v", i, err)
			}
		}
	}
	
	// Handle radio buttons for US selection
	radioButtons, _ := iframe.Locator("input[type='radio']:visible").All()
	if len(radioButtons) > 0 {
		log.Printf("Found %d visible radio buttons in iframe", len(radioButtons))
		
		for _, radio := range radioButtons {
			radioId, _ := radio.GetAttribute("id")
			radioValue, _ := radio.GetAttribute("value")
			
			// Try to find the label
			labelText := ""
			if radioId != "" {
				if label, err := iframe.Locator(fmt.Sprintf("label[for='%s']", radioId)).TextContent(); err == nil {
					labelText = label
				}
			}
			
			fieldInfo := strings.ToLower(radioValue + " " + labelText)
			
			// Select US/United States radio button
			if strings.Contains(fieldInfo, "united states") || strings.Contains(fieldInfo, "usa") ||
			   strings.Contains(fieldInfo, "u.s.") || (strings.Contains(fieldInfo, "us") && !strings.Contains(fieldInfo, "non-us")) {
				if checked, _ := radio.IsChecked(); !checked {
					err := radio.Check()
					if err == nil {
						log.Printf("✓ Selected US radio button option: %s", labelText)
						filledCount++
					}
				}
			}
		}
	}
	
	log.Printf("Completed iframe field filling - filled %d fields (inputs + selects + checkboxes)", filledCount)
	return filledCount, nil
}

func (s *BrowserAutomationService) fillRemainingDropdowns(iframe playwright.FrameLocator, userData *UserProfileData, page playwright.Page) int {
	log.Printf("Second pass: Looking specifically for unfilled dropdowns")
	filledCount := 0
	
	// Look for ALL potential unfilled dropdowns
	dropdownSelectors := []string{
		"input[value*='Select']",
		"input[placeholder*='Select']",
		"input[readonly]",
		"div:has-text('Select'):visible",
		"button:has-text('Select')",
	}
	
	maxAttempts := 10 // Allow more attempts since we're missing dropdowns
	attemptCount := 0
	
	for _, selector := range dropdownSelectors {
		if attemptCount >= maxAttempts {
			break
		}
		
		elements, _ := iframe.Locator(selector).All()
		if len(elements) > 2 {
			elements = elements[:2] // Only check first 2 of each type
		}
		
		for _, elem := range elements {
			if attemptCount >= maxAttempts {
				break
			}
			
			// Check if element is visible
			visible, _ := elem.IsVisible()
			if !visible {
				continue
			}
			
			text, _ := elem.TextContent()
			placeholder, _ := elem.GetAttribute("placeholder")
			
			// Only proceed if it really looks like an unfilled dropdown
			combinedText := strings.ToLower(text + " " + placeholder)
			if !strings.Contains(combinedText, "select") && !strings.Contains(combinedText, "choose") {
				continue
			}
			
			// Check if it might already have a value
			value, _ := elem.InputValue()
			if value != "" && !strings.Contains(strings.ToLower(value), "select") {
				continue
			}
			
			log.Printf("Attempting to click dropdown: %s", strings.TrimSpace(text))
			if err := elem.Click(); err == nil {
				attemptCount++
				page.WaitForTimeout(200) // Shorter wait
				
				// Quick check for United States option
				usOption := iframe.Locator("*[role='option']:has-text('United States')").First()
				if usOption != nil {
					if visible, _ := usOption.IsVisible(); visible {
						if err := usOption.Click(); err == nil {
							log.Printf("✓ Selected United States in second pass")
							filledCount++
							page.WaitForTimeout(200)
						}
					}
				}
			}
		}
	}
	
	log.Printf("Second pass completed - filled %d additional dropdowns", filledCount)
	return filledCount
}

func (s *BrowserAutomationService) submitIframeForm(iframe playwright.FrameLocator, page playwright.Page, result *AutomationResult) {
	log.Printf("Looking for submit button in iframe")
	
	submitSelectors := []string{
		"button[type='submit']",
		"input[type='submit']",
		"button:has-text('Submit')",
		"button:has-text('Apply')",
		"button:has-text('Continue')",
		"button:has-text('Next')",
	}
	
	for _, selector := range submitSelectors {
		btn := iframe.Locator(selector).First()
		if visible, _ := btn.IsVisible(); visible {
			text, _ := btn.TextContent()
			log.Printf("Found submit button in iframe: %s (text: %s)", selector, text)
			
		// IMPORTANT: Check for missing required fields before submitting
			log.Printf("Checking for missing required fields before submission...")
			missingFields, fieldDescriptions := s.checkRequiredFields(page)
			
			if len(missingFields) > 0 {
				log.Printf("⚠️ STOPPING: Found %d missing required fields", len(missingFields))
				for _, field := range missingFields {
					log.Printf("  - Missing: %s", fieldDescriptions[field])
				}
				
				// Take screenshot to show current state
				screenshotURL, _ := s.saveScreenshot(page, "missing_fields", result)
				if screenshotURL != "" {
					result.ApplicationScreenshotKey = screenshotURL
				}
				
				// Return with request for user input
				result.Status = "missing_required_fields"
				result.Message = fmt.Sprintf("Please provide the following information: %v", missingFields)
				result.Success = false
				return
			}
			
			// All fields filled - take screenshot BEFORE submitting so user can verify
			log.Printf("All required fields filled! Taking screenshot for verification...")
			beforeSubmitURL, _ := s.saveScreenshot(page, "before_submit_verification", result)
			if beforeSubmitURL != "" {
				result.ApplicationScreenshotKey = beforeSubmitURL
				log.Printf("✓ Verification screenshot saved: %s", beforeSubmitURL)
			}
			
			// Now click submit
			if err := btn.Click(); err == nil {
				log.Printf("✓ Clicked submit button in iframe")
				page.WaitForTimeout(1500)
				
				// Take a second screenshot after submit to confirm submission
				afterSubmitURL, _ := s.saveScreenshot(page, "after_submit_confirmation", result)
				if afterSubmitURL != "" {
					// Store this as confirmation but keep the pre-submit as the main screenshot
					result.ConfirmationScreenshotKey = afterSubmitURL
					log.Printf("✓ Confirmation screenshot saved: %s", afterSubmitURL)
				}
				
				result.Success = true
				result.Status = "submitted_via_iframe"
				result.Message = "Application submitted through iframe form"
				return
			} else {
				log.Printf("Failed to click submit button: %v", err)
			}
		}
	}
	
	log.Printf("No submit button found in iframe")
}

func (s *BrowserAutomationService) uploadResumeIfAvailable(page playwright.Page, resumeFilePath string, result *AutomationResult) {
	resumeUpload := page.Locator("input[type='file']")
	if exists, err := resumeUpload.First().IsVisible(); err == nil && exists {
		log.Printf("Found file upload field, uploading resume: %s", resumeFilePath)
		err = resumeUpload.First().SetInputFiles(resumeFilePath)
		if err != nil {
			log.Printf("Failed to upload resume: %v", err)
		} else {
			log.Printf("Successfully uploaded resume file")
			result.FilledFields["resume_upload"] = resumeFilePath
		}
	}
}

func (s *BrowserAutomationService) checkForSubmissionSuccess(page playwright.Page) bool {
	// Common success indicators
	successIndicators := []string{
		"text=Thank you for your application",
		"text=Application submitted successfully", 
		"text=Your application has been submitted",
		"text=Application received",
		"text=Thank you for applying",
		"text=Application complete",
		"text=Submission successful",
		"text=We have received your application",
		"text=Your application is now complete",
		"text=Successfully submitted",
		"text=We'll be in touch",
		"text=Good luck",
		"text=Next steps",
		"text=What happens next",
		"text=You're all set",
		"text=All done",
		"[class*='success']",
		"[class*='confirmation']",
		"[class*='submitted']",
		"[class*='complete']",
		"[data-testid*='success']",
		"[data-testid*='confirmation']",
		"h1:has-text('Thank you')",
		"h2:has-text('Thank you')",
		"h1:has-text('Submitted')",
		"h2:has-text('Submitted')",
		"h1:has-text('Complete')",
		"h2:has-text('Complete')",
	}
	
	pageTitle, _ := page.Title()
	pageURL := page.URL()
	
	log.Printf("Checking for success indicators. Page title: '%s', URL: '%s'", pageTitle, pageURL)
	
	// Check URL for success indicators
	if strings.Contains(strings.ToLower(pageURL), "success") ||
	   strings.Contains(strings.ToLower(pageURL), "confirmation") ||
	   strings.Contains(strings.ToLower(pageURL), "thank") ||
	   strings.Contains(strings.ToLower(pageURL), "complete") {
		log.Printf("Found success keyword in URL: %s", pageURL)
		return true
	}
	
	// Check page title for success keywords
	successKeywords := []string{
		"thank you", "submitted", "received", "complete", "success", "confirmation",
		"application sent", "all set", "done", "finished",
	}
	
	titleLower := strings.ToLower(pageTitle)
	for _, keyword := range successKeywords {
		if strings.Contains(titleLower, keyword) {
			log.Printf("Found success keyword '%s' in page title", keyword)
			return true
		}
	}
	
	// Check for specific success elements
	for _, indicator := range successIndicators {
		if exists, err := page.Locator(indicator).First().IsVisible(); err == nil && exists {
			text, _ := page.Locator(indicator).First().TextContent()
			log.Printf("Found success indicator: %s with text: '%s'", indicator, text)
			return true
		}
	}
	
	// Check main heading for success messages
	headings := []string{"h1", "h2", "h3"}
	for _, heading := range headings {
		if count, _ := page.Locator(heading).Count(); count > 0 {
			for i := 0; i < count && i < 5; i++ { // Check first 5 headings
				text, _ := page.Locator(heading).Nth(i).TextContent()
				textLower := strings.ToLower(text)
				for _, keyword := range successKeywords {
					if strings.Contains(textLower, keyword) {
						log.Printf("Found success keyword '%s' in heading: %s", keyword, text)
						return true
					}
				}
			}
		}
	}
	
	// Check for absence of form fields (indicating we've moved past the form)
	formFieldsCount := 0
	formSelectors := []string{
		"input[type='text']:visible",
		"input[type='email']:visible",
		"textarea:visible",
		"select:visible",
	}
	
	for _, selector := range formSelectors {
		if count, err := page.Locator(selector).Count(); err == nil {
			formFieldsCount += count
		}
	}
	
	// If we had form fields before and now have very few or none, might indicate success
	if formFieldsCount < 3 {
		log.Printf("Very few form fields remaining (%d), might indicate successful submission", formFieldsCount)
		// Look for any positive messaging
		positiveWords := []string{"thank", "success", "complete", "received", "submitted", "confirm"}
		bodyText, _ := page.Locator("body").TextContent()
		bodyTextLower := strings.ToLower(bodyText)
		for _, word := range positiveWords {
			if strings.Contains(bodyTextLower, word) {
				log.Printf("Found positive word '%s' with minimal form fields - likely success", word)
				return true
			}
		}
	}
	
	log.Printf("No clear success indicators found (checked %d form fields remaining)", formFieldsCount)
	return false
}


func (s *BrowserAutomationService) fillGenericForm(page playwright.Page, userData *UserProfileData, resumeFilePath string, result *AutomationResult) (*AutomationResult, error) {
	log.Printf("Filling generic application form")
	
	// Take screenshot first to see what page we're actually on
	pageScreenshotURL, err := s.saveScreenshot(page, "application_page", result)
	if err != nil {
		log.Printf("Failed to save application page screenshot: %v", err)
	} else {
		result.ApplicationScreenshotKey = pageScreenshotURL
		log.Printf("Application page screenshot saved: %s", pageScreenshotURL)
	}
	
	// Check if there are actually application-specific form fields
	applicationFields := []string{
		"input[name*='resume'], input[id*='resume']",
		"input[type='file']",
		"input[name*='cv'], input[id*='cv']",
		"textarea[name*='cover'], textarea[id*='cover']",
		"select[name*='position'], select[id*='position']",
		"select[name*='experience'], select[id*='experience']",
		"input[name*='salary'], input[id*='salary']",
		"select[name*='authorization'], select[id*='authorization']",
		"input[name*='first'], input[id*='first']",
		"input[name*='last'], input[id*='last']",
		"input[name*='name'], input[id*='name']",
		"input[name*='email'], input[id*='email']",
		"input[name*='phone'], input[id*='phone']",
	}
	
	hasApplicationFields := false
	foundFields := []string{}
	for _, selector := range applicationFields {
		if exists, err := page.Locator(selector).First().IsVisible(); err == nil && exists {
			hasApplicationFields = true
			foundFields = append(foundFields, selector)
		}
	}
	
	log.Printf("Application fields check: hasApplicationFields=%v, foundFields=%v", hasApplicationFields, foundFields)
	
	if !hasApplicationFields {
		log.Printf("No job application specific fields found. This may not be a job application form.")
		// Still take a screenshot to see what we got
		result.Success = false
				return result, nil
		result.Status = "no_application_fields"
		result.Message = "This page doesn't appear to have job application specific fields. Please verify this is a job application form."
		result.Success = false
				return result, nil
				return result, nil
	}
	
	// Common form field patterns
	formFields := map[string]string{
		"input[name*='first'], input[id*='first']":           strings.Split(userData.FullName, " ")[0],
		"input[name*='last'], input[id*='last']":             func() string {
			parts := strings.Split(userData.FullName, " ")
			if len(parts) > 1 { return strings.Join(parts[1:], " ") }
			return ""
		}(),
		"input[name*='name'], input[id*='name']":             userData.FullName,
		"input[name*='email'], input[id*='email']":           userData.Email,
		"input[name*='phone'], input[id*='phone']":           userData.Phone,
		"textarea[name*='cover'], textarea[id*='cover']":     userData.Summary,
		"input[name*='linkedin'], input[id*='linkedin']":     userData.LinkedIn,
		"input[name*='portfolio'], input[id*='portfolio']":   userData.Portfolio,
	}

	// Fill form fields
	filledCount := 0
	for selector, value := range formFields {
		if value == "" {
			continue
		}
		
		field := page.Locator(selector)
		if exists, err := field.First().IsVisible(); err == nil && exists {
			log.Printf("Filling field %s with value: %s", selector, value)
			err = field.First().Fill(value)
			if err != nil {
				log.Printf("Failed to fill field %s: %v", selector, err)
			} else {
				result.FilledFields[selector] = value
				filledCount++
			}
		}
	}
	
	// Upload resume if available
	if resumeFilePath != "" {
		// Try multiple selectors for file upload
		fileUploadSelectors := []string{
			"input[type='file']",
			"input[type=file]",
			"input[name*='resume']",
			"input[name*='cv']",
			"input[name*='document']",
			"input[accept*='pdf']",
		}
		
		var uploadSuccess bool
		for _, selector := range fileUploadSelectors {
			resumeUpload := page.Locator(selector)
			if count, _ := resumeUpload.Count(); count > 0 {
				log.Printf("Found %d file input(s) with selector: %s", count, selector)
				for i := 0; i < count && !uploadSuccess; i++ {
					fileInput := resumeUpload.Nth(i)
					if visible, _ := fileInput.IsVisible(); visible {
						log.Printf("Uploading resume file to input %d: %s", i, resumeFilePath)
						err := fileInput.SetInputFiles(resumeFilePath)
						if err != nil {
							log.Printf("Failed to upload resume to input %d: %v", i, err)
						} else {
							log.Printf("✓ Successfully uploaded resume file")
							result.FilledFields["resume_upload"] = resumeFilePath
							uploadSuccess = true
							break
						}
					}
				}
			}
		}
		
		if !uploadSuccess {
			log.Printf("⚠ No visible file upload input found for resume")
		}
	}
	
	// Look for submit button and try to submit
	submitSelectors := []string{
		"button:has-text('Submit Application')",
		"button:has-text('Apply Now')",
		"button:has-text('Submit')",
		"input[type='submit'][value*='Apply']",
		"input[type='submit'][value*='Submit']",
		"button[type='submit']",
	}
	
	submitted := false
	for _, selector := range submitSelectors {
		if exists, err := page.Locator(selector).First().IsVisible(); err == nil && exists {
			log.Printf("Found submit button with selector: %s", selector)
			
		// Check for missing required fields before submission
			log.Printf("Checking for missing required fields before submission...")
			missingFields, fieldDescriptions := s.checkRequiredFields(page)
			
			if len(missingFields) > 0 {
				log.Printf("⚠️ STOPPING: Found %d missing required fields", len(missingFields))
				for _, field := range missingFields {
					log.Printf("  - Missing: %s", fieldDescriptions[field])
				}
				
				// Take screenshot to show current state
				screenshotURL, _ := s.saveScreenshot(page, "missing_fields", result)
				if screenshotURL != "" {
					result.ApplicationScreenshotKey = screenshotURL
				}
				
				// Return with request for user input
				result.Success = false
				return result, nil
				result.Status = "missing_required_fields"
				result.Message = fmt.Sprintf("Please provide the following information: %v", missingFields)
				result.Success = false
				return result, nil
				return result, nil
			}
			
			// All fields filled - take screenshot before submission
			beforeSubmitURL, err := s.saveScreenshot(page, "before_submit", result)
			if err != nil {
				log.Printf("Failed to save before-submit screenshot: %v", err)
			} else {
				result.ApplicationScreenshotKey = beforeSubmitURL
			}
			
			err = page.Locator(selector).First().Click()
			if err != nil {
				log.Printf("Failed to click submit button: %v", err)
				continue
			}
			
			// Wait for submission to process
			page.WaitForTimeout(1500)
			
			// Take screenshot after submission
			afterSubmitURL, err := s.saveScreenshot(page, "after_submit", result)
			if err != nil {
				log.Printf("Failed to save after-submit screenshot: %v", err)
			} else {
				result.ApplicationScreenshotKey = afterSubmitURL
			}
			
			// Check for success indicators
			successIndicators := []string{
				"text=application submitted",
				"text=thank you for applying",
				"text=application received",
				"text=successfully submitted",
				"text=confirmation",
				"[class*='success']",
				"[class*='confirmation']",
			}
			
			for _, indicator := range successIndicators {
				if exists, err := page.Locator(indicator).First().IsVisible(); err == nil && exists {
					log.Printf("Found success indicator: %s", indicator)
					submitted = true
					break
				}
			}
			
			if submitted {
				break
			}
		}
	}
	
	if !submitted {
		result.Success = false
				return result, nil
		result.Status = "submission_uncertain"
		result.Message = fmt.Sprintf("Form filled (%d fields) but could not confirm successful submission. Manual verification required.", filledCount)
		result.Success = false
				return result, nil
				return result, nil
	}

	result.Success = true
	result.Status = "submitted"
	result.Message = fmt.Sprintf("Job application submitted successfully! Filled %d fields and confirmed submission.", filledCount)
	result.Success = false
				return result, nil
				return result, nil
}
// checkRequiredFields checks if all required fields are filled and returns missing fields
func (s *BrowserAutomationService) checkRequiredFields(page playwright.Page) ([]string, map[string]string) {
	missingFields := []string{}
	fieldDescriptions := make(map[string]string)
	
	// Check all visible input fields
	inputs, _ := page.Locator("input:visible, select:visible, textarea:visible").All()
	
	for _, input := range inputs {
		// Check if field is required
		required, _ := input.GetAttribute("required")
		ariaRequired, _ := input.GetAttribute("aria-required")
		
		if required == "true" || required == "required" || ariaRequired == "true" {
			// Get field value
			inputType, _ := input.GetAttribute("type")
			var value string
			
			if inputType == "checkbox" || inputType == "radio" {
				checked, _ := input.IsChecked()
				if !checked {
					value = ""
				} else {
					value = "checked"
				}
			} else {
				value, _ = input.InputValue()
			}
			
			// If field is empty, add to missing fields
			if value == "" || value == "0" || strings.Contains(strings.ToLower(value), "select") || strings.Contains(strings.ToLower(value), "choose") {
				// Get field label or name for description
				name, _ := input.GetAttribute("name")
				id, _ := input.GetAttribute("id")
				placeholder, _ := input.GetAttribute("placeholder")
				
				// Try to find label
				var label string
				if id != "" {
					labelElem := page.Locator(fmt.Sprintf("label[for='%s']", id))
					if labelElem != nil {
						label, _ = labelElem.TextContent()
					}
				}
				
				fieldKey := name
				if fieldKey == "" {
					fieldKey = id
				}
				if fieldKey == "" {
					fieldKey = fmt.Sprintf("field_%d", len(missingFields))
				}
				
				fieldDesc := label
				if fieldDesc == "" {
					fieldDesc = placeholder
				}
				if fieldDesc == "" {
					fieldDesc = name
				}
				
				missingFields = append(missingFields, fieldKey)
				fieldDescriptions[fieldKey] = fieldDesc
			}
		}
	}
	
	return missingFields, fieldDescriptions
}
