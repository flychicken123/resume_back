package services

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	
	"github.com/playwright-community/playwright-go"
)

// BrowserAutomationServiceV2 is the refactored, cleaner version
type BrowserAutomationServiceV2 struct {
	playwright         *playwright.Playwright
	browser            playwright.Browser
	formFiller         *FormFillerService
	screenshotService  *ScreenshotService
	submissionChecker  *SubmissionCheckerService
}

// NewBrowserAutomationServiceV2 creates a new instance of the refactored service
func NewBrowserAutomationServiceV2() (*BrowserAutomationServiceV2, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("could not start playwright: %v", err)
	}
	
	// Check if we should run in headless mode (default: true)
	headless := true
	if os.Getenv("HEADLESS") == "false" {
		headless = false
		log.Println("Running browser in visible mode (HEADLESS=false)")
	}
	
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
	})
	if err != nil {
		return nil, fmt.Errorf("could not launch browser: %v", err)
	}
	
	return &BrowserAutomationServiceV2{
		playwright:        pw,
		browser:           browser,
		formFiller:        &FormFillerService{},
		screenshotService: NewScreenshotService(),
		submissionChecker: &SubmissionCheckerService{},
	}, nil
}

// Close cleanly shuts down the browser and playwright
func (s *BrowserAutomationServiceV2) Close() error {
	if s.browser != nil {
		if err := s.browser.Close(); err != nil {
			log.Printf("Error closing browser: %v", err)
		}
	}
	if s.playwright != nil {
		if err := s.playwright.Stop(); err != nil {
			log.Printf("Error stopping playwright: %v", err)
		}
	}
	return nil
}

// SubmitJobApplication is the main entry point for job applications
func (s *BrowserAutomationServiceV2) SubmitJobApplication(jobURL string, userData *UserProfileData, resumeFilePath string) (*AutomationResult, error) {
	// Start automation
	
	result := &AutomationResult{
		Success:      false,
		FilledFields: make(map[string]string),
	}
	
	// Create browser context with viewport
	context, err := s.browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{
			Width:  1920,
			Height: 1080,
		},
	})
	if err != nil {
		return result, fmt.Errorf("could not create context: %v", err)
	}
	defer context.Close()
	
	// Create page
	page, err := context.NewPage()
	if err != nil {
		return result, fmt.Errorf("could not create page: %v", err)
	}
	
	// Navigate to job URL
	// Navigate to job URL
	if _, err := page.Goto(jobURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return result, fmt.Errorf("could not navigate to URL: %v", err)
	}
	
	// Determine application type and route to appropriate handler
	currentURL := page.URL()
	log.Printf("Determining handler for URL: %s", currentURL)
	
	// Use generic handler for all job applications
	// We don't need platform-specific logic
	return s.handleGeneric(page, userData, resumeFilePath, result)
}

// handleGeneric handles generic job application forms for all companies
func (s *BrowserAutomationServiceV2) handleGeneric(page playwright.Page, userData *UserProfileData, resumeFilePath string, result *AutomationResult) (*AutomationResult, error) {
	log.Println("=== Handling job application form ===")
	
	currentURL := page.URL()
	log.Printf("Current URL: %s", currentURL)
	
	// First check if there's an Apply button visible (indicates we're on listing page)
	applyBtn := page.Locator("button:has-text('Apply'):visible, a:has-text('Apply'):visible").First()
	hasApplyButton, _ := applyBtn.Count()
	
	// Also check if we have form inputs visible
	formInputs, _ := page.Locator("input[type='text']:visible, input[type='email']:visible").Count()
	
	// If we have Apply button but no form inputs, we're likely on listing page
	if hasApplyButton > 0 && formInputs == 0 {
		log.Println("Found Apply button but no form inputs - clicking Apply to navigate to form...")
		
		if s.findAndClickApplyButton(page) {
			log.Println("Clicked Apply button, navigating to application form...")
			
			// Wait for navigation to complete
			page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
				State: playwright.LoadStateNetworkidle,
			})
			
			// Check new URL
			newURL := page.URL()
			log.Printf("Navigated to: %s", newURL)
		}
	}
	
	// Now check for the actual form
	// Check for iframe (common in embedded forms)
	iframeCount, _ := page.Locator("iframe").Count()
	if iframeCount > 0 {
		log.Printf("Found %d iframes - form may be embedded", iframeCount)
		return s.handleIframeForm(page, userData, resumeFilePath, result)
	}
	
	// No iframe - form is directly on page
	log.Println("No iframe found - checking for form directly on page")
	
	// Fill regular form fields
	s.formFiller.FillBasicFields(page, userData)
	
	// Upload resume if available
	if s.submissionChecker.UploadResume(page, resumeFilePath) {
		result.FilledFields["resume"] = "uploaded"
	}
	
	// Take screenshot before submission
	screenshotURL, _ := s.screenshotService.SaveScreenshotToResult(page, "before_submit", result)
	if screenshotURL != "" {
		result.ApplicationScreenshotKey = screenshotURL
	}
	
	// Check for required fields
	missingFields, descriptions := s.formFiller.CheckRequiredFields(page)
	if len(missingFields) > 0 {
		log.Printf("Found %d missing required fields", len(missingFields))
		result.Status = "missing_required_fields"
		result.Message = fmt.Sprintf("Missing fields: %v", descriptions)
		return result, nil
	}
	
	// Submit the form
	if s.submissionChecker.FindAndClickSubmitButton(page) {
		// Continue immediately
		
		// Check for success
		if s.submissionChecker.CheckForSuccess(page) {
			result.Success = true
			result.Status = "submitted"
			result.Message = "Application submitted successfully"
			
			// Take confirmation screenshot
			confirmURL, _ := s.screenshotService.SaveScreenshotToResult(page, "confirmation", result)
			if confirmURL != "" {
				result.ConfirmationScreenshotKey = confirmURL
			}
		}
	}
	
	return result, nil
}

// handleIframeForm handles forms inside iframes
func (s *BrowserAutomationServiceV2) handleIframeForm(page playwright.Page, userData *UserProfileData, resumeFilePath string, result *AutomationResult) (*AutomationResult, error) {
	log.Println("=== Handling iframe form ===")
	
	// Check if iframe exists
	iframeCount, _ := page.Locator("iframe").Count()
	log.Printf("Found %d iframes on page", iframeCount)
	
	if iframeCount == 0 {
		log.Println("ERROR: No iframe found on page!")
		result.Status = "no_iframe"
		result.Message = "No iframe found on page"
		return result, nil
	}
	
	// Try each iframe to find one with form content
	var formIframe playwright.FrameLocator
	foundFormIframe := false
	
	for i := 0; i < iframeCount; i++ {
		iframe := page.FrameLocator("iframe").Nth(i)
		
		// Check if this iframe has form content
		inputCount, _ := iframe.Locator("input").Count()
		selectCount, _ := iframe.Locator("select").Count()
		buttonCount, _ := iframe.Locator("button").Count()
		
		log.Printf("Iframe %d: inputs=%d, selects=%d, buttons=%d", i, inputCount, selectCount, buttonCount)
		
		if inputCount > 0 || selectCount > 0 {
			log.Printf("Found form content in iframe %d - using this iframe", i)
			formIframe = iframe
			foundFormIframe = true
			break
		}
	}
	
	if !foundFormIframe {
		log.Println("No iframe contains form content - using first iframe")
		formIframe = page.FrameLocator("iframe").First()
	}
	
	iframe := formIframe
	log.Println("Using selected iframe for form filling")
	
	// Debug what's in the iframe
	DebugIframeContent(iframe, page)
	
	// Fill iframe fields using existing handlers
	filledCount, err := s.fillIframeFieldsSimple(iframe, userData, page, resumeFilePath)
	if err != nil {
		// Check if it's a MissingFieldsError - this should be returned to frontend for popup
		if missingErr, ok := err.(*MissingFieldsError); ok {
			log.Printf("Missing fields detected: %d fields need user input", len(missingErr.Fields))
			
			// Take screenshot of partially filled form to show progress
			screenshotURL, _ := s.screenshotService.SaveScreenshotToResult(page, "missing_fields", result)
			if screenshotURL != "" {
				result.ApplicationScreenshotKey = screenshotURL
				log.Printf("Screenshot taken with missing fields: %s", screenshotURL)
			}
			
			// Return the error so controller can handle it
			return result, missingErr
		}
		
		// Check if it's validation or other missing field error
		if strings.Contains(err.Error(), "Unable to fill") || 
		   strings.Contains(err.Error(), "required fields") || 
		   strings.Contains(err.Error(), "validation errors") {
			log.Printf("ERROR: Form incomplete - %v", err)
			result.Status = "incomplete_form"
			result.Message = fmt.Sprintf("Could not complete form: %v", err)
			
			// NO SCREENSHOT for incomplete forms either
			return result, nil
		}
		return result, err
	}
	
	// Fields filled
	result.FilledFields["iframe_fields"] = fmt.Sprintf("%d", filledCount)
	
	// Take a screenshot BEFORE submitting to show the filled form
	log.Println("Taking pre-submit screenshot of filled form...")
	preSubmitURL, _ := s.screenshotService.SaveScreenshotToResult(page, "pre_submit", result)
	if preSubmitURL != "" {
		result.ApplicationScreenshotKey = preSubmitURL
		log.Printf("Pre-submit screenshot saved: %s", preSubmitURL)
	}
	
	// Expand iframe to show all content before screenshot
	page.Evaluate(`
		const iframe = document.querySelector('iframe');
		if (iframe) {
			// Remove any height restrictions
			iframe.style.height = 'auto';
			iframe.style.minHeight = '3000px';
			iframe.style.maxHeight = 'none';
			iframe.style.overflow = 'visible';
			
			// Also expand parent containers
			let parent = iframe.parentElement;
			while (parent) {
				parent.style.height = 'auto';
				parent.style.minHeight = '3000px';
				parent.style.maxHeight = 'none';
				parent.style.overflow = 'visible';
				parent = parent.parentElement;
			}
			
			// Scroll to top to ensure everything is visible
			window.scrollTo(0, 0);
		}
	`)
	// Content should adjust immediately
	
	// Take screenshot
	screenshotURL, _ := s.screenshotService.SaveScreenshotToResult(page, "after_fill", result)
	if screenshotURL != "" {
		result.ApplicationScreenshotKey = screenshotURL
	}
	
	// Validations should complete immediately
	
	// Take screenshot BEFORE clicking submit to see the filled form
	beforeSubmitURL, _ := s.screenshotService.SaveScreenshotToResult(page, "before_submit", result)
	if beforeSubmitURL != "" {
		log.Printf("Screenshot taken before submit: %s", beforeSubmitURL)
		result.ApplicationScreenshotKey = beforeSubmitURL
	}
	
	// Submit iframe form - the submit button is INSIDE the Greenhouse iframe
	log.Println("Looking for submit button INSIDE iframe...")
	
	// Try inside iframe
	if s.submitIframeFormSimple(iframe, page) {
		log.Println("Submit button clicked successfully!")
		result.Success = true
		result.Status = "submitted"
		result.Message = "Application submitted via iframe"
		
		// Take confirmation screenshot after submission
		confirmURL, _ := s.screenshotService.SaveScreenshotToResult(page, "confirmation", result)
		if confirmURL != "" {
			result.ConfirmationScreenshotKey = confirmURL
		}
	} else {
		log.Printf("WARNING: Could not find submit button")
		result.Status = "submit_button_not_found"
		result.Message = "Application filled successfully but submit button could not be found. Please review and submit manually."
		result.Success = false
		
		// Still take a screenshot to show the filled form
		finalScreenshot, _ := s.screenshotService.SaveScreenshotToResult(page, "filled_form", result)
		if finalScreenshot != "" {
			result.ApplicationScreenshotKey = finalScreenshot
		}
	}
	
	return result, nil
}

// fillIframeFieldsSimple is a simplified version of iframe field filling
func (s *BrowserAutomationServiceV2) fillIframeFieldsSimple(iframe playwright.FrameLocator, userData *UserProfileData, page playwright.Page, resumeFilePath string) (int, error) {
	filledCount := 0
	unknownRequiredFields := []string{}
	
	log.Println("=== Starting to fill iframe fields ===")
	
	// Fill text inputs
	inputs, _ := iframe.Locator("input[type='text']:visible, input[type='email']:visible, input[type='tel']:visible").All()
	log.Printf("Found %d text/email/tel inputs", len(inputs))
	for _, input := range inputs {
		name, _ := input.GetAttribute("name")
		placeholder, _ := input.GetAttribute("placeholder")
		ariaLabel, _ := input.GetAttribute("aria-label")
		
		// Get the actual label text for this input
		labelText, _ := input.Evaluate(`el => {
			// Try to find associated label
			if (el.id) {
				const label = document.querySelector('label[for="' + el.id + '"]');
				if (label) return label.textContent.trim();
			}
			// Try parent label
			const parentLabel = el.closest('label');
			if (parentLabel) return parentLabel.textContent.trim();
			// Try previous sibling
			const prev = el.previousElementSibling;
			if (prev && prev.tagName === 'LABEL') return prev.textContent.trim();
			// Try to find text before this input
			const parent = el.parentElement;
			if (parent) {
				const textNodes = Array.from(parent.childNodes).filter(n => n.nodeType === 3);
				if (textNodes.length > 0) return textNodes[0].textContent.trim();
			}
			return "";
		}`, nil)
		
		labelStr := ""
		if labelText != nil {
			labelStr = labelText.(string)
		}
		
		fieldInfo := strings.ToLower(labelStr + " " + name + " " + placeholder + " " + ariaLabel)
		
		var value string
		fieldInfoLower := strings.ToLower(fieldInfo)
		
		// Basic personal info
		if strings.Contains(fieldInfoLower, "first") && strings.Contains(fieldInfoLower, "name") {
			value = userData.FirstName
		} else if strings.Contains(fieldInfoLower, "last") && strings.Contains(fieldInfoLower, "name") {
			value = userData.LastName
		} else if strings.Contains(fieldInfoLower, "email") {
			value = userData.Email
		} else if strings.Contains(fieldInfoLower, "phone") {
			value = userData.Phone
		} else if strings.Contains(fieldInfoLower, "linkedin") {
			value = userData.LinkedIn
		} else if strings.Contains(fieldInfoLower, "portfolio") || strings.Contains(fieldInfoLower, "website") {
			value = userData.Portfolio
		} else if strings.Contains(fieldInfoLower, "how would you describe") || 
		          strings.Contains(fieldInfoLower, "gender identity") || 
		          strings.Contains(fieldInfoLower, "racial") || 
		          strings.Contains(fieldInfoLower, "ethnic") || 
		          strings.Contains(fieldInfoLower, "sexual orientation") ||
		          strings.Contains(fieldInfoLower, "mark all that apply") {
			// Skip demographic fields - they should be handled as dropdowns/comboboxes
			log.Printf("  Skipping demographic field in text input phase: %s", fieldInfo)
			continue
		} else if strings.Contains(fieldInfoLower, "employer") || strings.Contains(fieldInfoLower, "company") || 
		          strings.Contains(fieldInfoLower, "current or previous employer") {
			// Check saved preferences first
			if userData.ExtraQA != nil {
				for question, answer := range userData.ExtraQA {
					questionLower := strings.ToLower(question)
					if strings.Contains(questionLower, "employer") || strings.Contains(questionLower, "company") {
						value = answer
						log.Printf("  Using employer from saved preferences: %s", value)
						break
					}
				}
			}
			// Fall back to experience data
			if value == "" && len(userData.Experience) > 0 {
				value = userData.Experience[0].Company
				log.Printf("  Using employer from resume: %s", value)
			}
			// If still empty, use a default
			if value == "" {
				value = "Self-employed"
				log.Printf("  Using default employer: %s", value)
			}
		} else if (strings.Contains(fieldInfoLower, "job title") || strings.Contains(fieldInfoLower, "current position") || 
		          strings.Contains(fieldInfoLower, "previous position") || strings.Contains(fieldInfoLower, "current or previous job") ||
		          (strings.Contains(fieldInfoLower, "title") && !strings.Contains(fieldInfoLower, "remote"))) && 
		          !strings.Contains(fieldInfoLower, "role offers") {
			// Check saved preferences first
			if userData.ExtraQA != nil {
				for question, answer := range userData.ExtraQA {
					questionLower := strings.ToLower(question)
					if strings.Contains(questionLower, "job title") || strings.Contains(questionLower, "position") ||
					   strings.Contains(questionLower, "job") && strings.Contains(questionLower, "title") {
						value = answer
						log.Printf("  Using job title from saved preferences: %s", value)
						break
					}
				}
			}
			// Fall back to experience data
			if value == "" && len(userData.Experience) > 0 {
				value = userData.Experience[0].Title
				log.Printf("  Using job title from resume: %s", value)
			}
			// If still empty, use a default
			if value == "" {
				value = "Software Engineer"
				log.Printf("  Using default job title: %s", value)
			}
		} else if (strings.Contains(fieldInfoLower, "school") || strings.Contains(fieldInfoLower, "university") || strings.Contains(fieldInfoLower, "college") || strings.Contains(fieldInfoLower, "institution")) && strings.Contains(fieldInfoLower, "attended") {
			// Check saved preferences first
			if userData.ExtraQA != nil {
				for question, answer := range userData.ExtraQA {
					if strings.Contains(strings.ToLower(question), "school") || strings.Contains(strings.ToLower(question), "attended") {
						value = answer
						log.Printf("  Using school from saved preferences: %s", value)
						break
					}
				}
			}
			// Fall back to education data
			if value == "" && len(userData.Education) > 0 {
				value = userData.Education[0].Institution
				log.Printf("  Using school from resume: %s", value)
			}
		} else if strings.Contains(fieldInfoLower, "degree") || strings.Contains(fieldInfoLower, "qualification") {
			// Check saved preferences first - look for exact match to avoid confusion
			if userData.ExtraQA != nil {
				// Try exact match first
				if answer, exists := userData.ExtraQA[fieldInfo]; exists && answer != "" {
					// Don't use if it looks like a school name
					answerLower := strings.ToLower(answer)
					if !strings.Contains(answerLower, "university") && !strings.Contains(answerLower, "college") && 
					   !strings.Contains(answerLower, "tech") && !strings.Contains(answerLower, "institute") {
						value = answer
						log.Printf("  Using degree from saved preferences (exact match): %s", value)
					}
				}
				// If no exact match or invalid, look for degree-specific saved answers
				if value == "" {
					for question, answer := range userData.ExtraQA {
						questionLower := strings.ToLower(question)
						// Only match if it's clearly asking for a degree
						if strings.Contains(questionLower, "degree") && strings.Contains(questionLower, "obtained") {
							// Make sure answer looks like a degree
							answerLower := strings.ToLower(answer)
							if strings.Contains(answerLower, "bachelor") || strings.Contains(answerLower, "master") || 
							   strings.Contains(answerLower, "phd") || strings.Contains(answerLower, "associate") ||
							   strings.Contains(answerLower, "diploma") {
								value = answer
								log.Printf("  Using degree from saved preferences: %s", value)
								break
							}
						}
					}
				}
			}
			// Fall back to education data
			if value == "" && len(userData.Education) > 0 {
				value = userData.Education[0].Degree
				log.Printf("  Using degree from resume: %s", value)
			}
			// Default to Bachelor's if still empty and it's a degree field
			if value == "" && strings.Contains(fieldInfoLower, "degree") {
				value = "Bachelor's"
				log.Printf("  Using default degree: Bachelor's")
			}
		} else if strings.Contains(fieldInfoLower, "field of study") || strings.Contains(fieldInfoLower, "major") {
			// Get field of study from Education
			if len(userData.Education) > 0 {
				value = userData.Education[0].Field
				log.Printf("  Using field of study from resume: %s", value)
			}
		}
		
		if value != "" {
			log.Printf("  Filling input '%s' with value: %s", fieldInfo, value)
			if err := input.Fill(value); err == nil {
				filledCount++
				log.Printf("  ✓ Filled successfully")
			} else {
				log.Printf("  ✗ Failed to fill: %v", err)
			}
		} else {
			// Before giving up, check ExtraQA for any matching saved answer
			if userData.ExtraQA != nil && fieldInfo != "" {
				fieldLower := strings.ToLower(fieldInfo)
				
				// Special handling for degree field to avoid confusion with school
				if strings.Contains(fieldLower, "degree") && strings.Contains(fieldLower, "obtained") {
					// Look specifically for degree answers
					for question, answer := range userData.ExtraQA {
						questionLower := strings.ToLower(question)
						if strings.Contains(questionLower, "degree") && strings.Contains(questionLower, "obtained") {
							// Validate it's a degree answer, not a school
							answerLower := strings.ToLower(answer)
							if !strings.Contains(answerLower, "university") && !strings.Contains(answerLower, "college") && 
							   !strings.Contains(answerLower, "tech") && !strings.Contains(answerLower, "institute") {
								value = answer
								log.Printf("  Found matching degree answer in ExtraQA for '%s': %s", fieldInfo, value)
								if err := input.Fill(value); err == nil {
									filledCount++
									log.Printf("  ✓ Filled from saved Q&A successfully")
								} else {
									log.Printf("  ✗ Failed to fill from saved Q&A: %v", err)
								}
								break
							}
						}
					}
				} else {
					// General matching for other fields
					for question, answer := range userData.ExtraQA {
						questionLower := strings.ToLower(question)
						// Check for various matching patterns
						if strings.Contains(questionLower, fieldLower) || strings.Contains(fieldLower, questionLower) ||
						   (len(fieldInfo) > 10 && strings.HasPrefix(questionLower, fieldLower[:10])) {
							value = answer
							log.Printf("  Found matching answer in ExtraQA for '%s': %s", fieldInfo, value)
							if err := input.Fill(value); err == nil {
								filledCount++
								log.Printf("  ✓ Filled from saved Q&A successfully")
							} else {
								log.Printf("  ✗ Failed to fill from saved Q&A: %v", err)
							}
							break
						}
					}
				}
			}
			
			// If still no value found
			if value == "" {
				// Check if this is a required field
				required, _ := input.GetAttribute("required")
				ariaRequired, _ := input.GetAttribute("aria-required")
				if required == "true" || required == "" && input != nil || ariaRequired == "true" {
					// This is a required field we can't fill
					if fieldInfo != "" {
						// IMPORTANT: Skip fields that will be handled by dropdown/combobox handler
						fieldLower := strings.ToLower(fieldInfo)
						// Skip demographic fields
						if strings.Contains(fieldLower, "racial") || strings.Contains(fieldLower, "ethnic") ||
						   strings.Contains(fieldLower, "gender identity") || strings.Contains(fieldLower, "sexual orientation") ||
						   strings.Contains(fieldLower, "mark all that apply") {
							log.Printf("  ⚠ Skipping demographic field (will be handled as combobox): %s", fieldInfo)
						// Skip fields that are typically dropdowns/comboboxes
						} else if strings.Contains(fieldLower, "country") || strings.Contains(fieldLower, "authorized") ||
						   strings.Contains(fieldLower, "sponsor") || strings.Contains(fieldLower, "remote") ||
						   strings.Contains(fieldLower, "previously employed") || strings.Contains(fieldLower, "ever been employed") ||
						   strings.Contains(fieldLower, "whatsapp") || strings.Contains(fieldLower, "transgender") ||
						   strings.Contains(fieldLower, "disability") || strings.Contains(fieldLower, "veteran") ||
						   strings.Contains(fieldLower, "armed forces") {
							log.Printf("  ⚠ Skipping dropdown field (will be handled as combobox): %s", fieldInfo)
						} else {
							unknownRequiredFields = append(unknownRequiredFields, fieldInfo)
							log.Printf("  ⚠ REQUIRED field skipped - no value for: %s", fieldInfo)
						}
					}
				} else {
					log.Printf("  Skipping input - no value determined for: %s", fieldInfo)
				}
			}
		}
	}
	
	// Handle resume upload
	if resumeFilePath != "" {
		// Check for resume upload
		
		// First try to find and click the "Attach" button for resume
		attachButtons, _ := iframe.Locator("button").All()
		for _, btn := range attachButtons {
			btnText, _ := btn.TextContent()
			// Check if this button is in the resume section
			parentText, _ := btn.Evaluate(`el => {
				let parent = el.closest('div');
				let depth = 0;
				while (parent && depth < 5) {
					if (parent.textContent.toLowerCase().includes('resume') || 
					    parent.textContent.toLowerCase().includes('cv')) {
						return parent.textContent;
					}
					parent = parent.parentElement;
					depth++;
				}
				return '';
			}`, nil)
			
			if strings.Contains(strings.ToLower(btnText), "attach") && 
			   parentText != nil && strings.Contains(strings.ToLower(parentText.(string)), "resume") {
				// Click attach button
				btn.Click()
				// Continue immediately
				break
			}
		}
		
		// Now look for file input fields (may be hidden but still usable)
		fileInputs, _ := iframe.Locator("input[type='file']").All()
		for _, fileInput := range fileInputs {
			// Check if this is the resume field
			accept, _ := fileInput.GetAttribute("accept")
			name, _ := fileInput.GetAttribute("name")
			
			// Check parent context for resume/cv keywords
			parentText, _ := fileInput.Evaluate(`el => {
				let parent = el.closest('div');
				while (parent && parent.parentElement) {
					if (parent.textContent.toLowerCase().includes('resume') || 
					    parent.textContent.toLowerCase().includes('cv')) {
						return parent.textContent;
					}
					parent = parent.parentElement;
				}
				return '';
			}`, nil)
			
			// If it accepts PDFs/docs and is in resume context, upload
			if (strings.Contains(accept, "pdf") || strings.Contains(accept, "doc") || 
			    strings.Contains(strings.ToLower(name), "resume") ||
			    (parentText != nil && strings.Contains(strings.ToLower(parentText.(string)), "resume"))) {
				
				if err := fileInput.SetInputFiles(resumeFilePath); err == nil {
					filledCount++
				}
				break
			}
		}
	}
	
	// Handle ALL dropdowns comprehensively
	// First try the comprehensive handler that catches everything
	log.Println("Starting dropdown filling...")
	
	// FIRST: Handle Stripe-specific form fields that might not be standard dropdowns
	s.handleStripeSpecificFields(iframe, userData)
	
	dropdownErr := HandleAllDropdownsComprehensive(iframe, userData)
	if dropdownErr != nil {
		log.Printf("Some dropdowns could not be filled: %v", dropdownErr)
		
		// Check if this is a special MISSING_FIELDS error
		if strings.HasPrefix(dropdownErr.Error(), "MISSING_FIELDS:") {
			// Parse the missing fields
			missingFieldsStr := strings.TrimPrefix(dropdownErr.Error(), "MISSING_FIELDS:")
			missingQuestions := strings.Split(missingFieldsStr, " | ")
			
			log.Printf("=== MISSING FIELDS DETECTED: %d fields need user input ===", len(missingQuestions))
			
			// Create MissingFieldInfo for each question
			var missingFields []MissingFieldInfo
			for _, question := range missingQuestions {
				question = strings.TrimSpace(question)
				if question != "" {
					missingFields = append(missingFields, createMissingFieldInfo(question))
				}
			}
			
			// Return error to frontend so it can collect the missing information
			return filledCount, &MissingFieldsError{
				Fields: missingFields,
				Message: "Additional information required to complete the application",
			}
		} else if missingErr, ok := dropdownErr.(*MissingFieldsError); ok {
			// Already structured error, return it to frontend
			log.Printf("=== MISSING FIELDS DETECTED: %d fields need user input ===", len(missingErr.Fields))
			// Return the error so frontend can show popup to collect missing data
			return filledCount, missingErr
		} else if strings.Contains(dropdownErr.Error(), "Unable to fill") {
			// Old format error, extract fields for backward compatibility
			// Parse out the questions from format: "Unable to fill N fields: field1 | field2 | ..."
			parts := strings.SplitN(dropdownErr.Error(), ": ", 2)
			if len(parts) > 1 {
				// Split by | to get individual questions
				questionList := strings.Split(parts[1], " | ")
				for _, q := range questionList {
					q = strings.TrimSpace(q)
					if q != "" {
						unknownRequiredFields = append(unknownRequiredFields, q)
					}
				}
			}
		}
	}
	log.Println("Finished dropdown filling")
	
	// FIRST check if we have unknown required fields that we couldn't fill
	if len(unknownRequiredFields) > 0 {
		log.Printf("Need user input for %d unknown required fields", len(unknownRequiredFields))
		
		// Create proper MissingFieldsError so frontend shows popup
		var missingFields []MissingFieldInfo
		for _, fieldName := range unknownRequiredFields {
			fieldName = strings.TrimSpace(fieldName)
			if fieldName != "" {
				// Double-check: Skip demographic fields that should have been handled
				fieldLower := strings.ToLower(fieldName)
				if strings.Contains(fieldLower, "racial") || strings.Contains(fieldLower, "ethnic") ||
				   strings.Contains(fieldLower, "gender identity") || strings.Contains(fieldLower, "sexual orientation") ||
				   strings.Contains(fieldLower, "mark all that apply") {
					log.Printf("WARNING: Demographic field in unknownRequiredFields, skipping: %s", fieldName)
					continue
				}
				missingFields = append(missingFields, createMissingFieldInfo(fieldName))
			}
		}
		
		return filledCount, &MissingFieldsError{
			Fields: missingFields,
			Message: "Additional information required to complete the application",
		}
	}
	
	// Then check if there are any remaining required fields
	requiredFields, _ := iframe.Locator("input[required]:visible, select[required]:visible, [aria-required='true']:visible").All()
	unfilledRequired := 0
	var emptyRequiredFields []string
	for _, field := range requiredFields {
		// Skip file upload fields
		fieldType, _ := field.GetAttribute("type")
		if fieldType == "file" {
			continue
		}
		
		// Skip hidden or disabled fields
		isDisabled, _ := field.IsDisabled()
		if isDisabled {
			continue
		}
		
		value, _ := field.InputValue()
		if value == "" {
			tagName, _ := field.Evaluate(`el => el.tagName`, nil)
			name, _ := field.GetAttribute("name")
			placeholder, _ := field.GetAttribute("placeholder")
			ariaLabel, _ := field.GetAttribute("aria-label")
			
			// Skip if this is a FIELDSET (radio/checkbox group already handled)
			if tagName == "FIELDSET" {
				continue
			}
			
			// Try to get a meaningful field name
			fieldDesc := ""
			if placeholder != "" {
				fieldDesc = placeholder
			} else if ariaLabel != "" {
				fieldDesc = ariaLabel
			} else if name != "" {
				fieldDesc = name
			} else {
				// Skip generic unnamed fields
				continue
			}
			
			unfilledRequired++
			log.Printf("WARNING: Required field still empty: %v[name='%s'] - %s", tagName, name, fieldDesc)
			emptyRequiredFields = append(emptyRequiredFields, fieldDesc)
		}
	}
	
	if unfilledRequired > 0 {
		log.Printf("WARNING: %d required fields are still empty", unfilledRequired)
		
		// Return as MissingFieldsError so frontend shows popup
		var missingFields []MissingFieldInfo
		for _, fieldName := range emptyRequiredFields {
			fieldName = strings.TrimSpace(fieldName)
			if fieldName != "" {
				missingFields = append(missingFields, createMissingFieldInfo(fieldName))
			}
		}
		
		return filledCount, &MissingFieldsError{
			Fields: missingFields,
			Message: fmt.Sprintf("%d required fields need to be filled", unfilledRequired),
		}
	}
	
	// Check for actual validation errors more carefully
	// Look for required but empty fields
	emptyRequiredSelects, _ := iframe.Locator("select[required]:visible").All()
	emptySelectCount := 0
	for _, sel := range emptyRequiredSelects {
		value, _ := sel.InputValue()
		if value == "" || value == "Select..." {
			emptySelectCount++
			// Log which select is empty
			name, _ := sel.GetAttribute("name")
			ariaLabel, _ := sel.GetAttribute("aria-label")
			log.Printf("WARNING: Empty required select - name: %s, aria-label: %s", name, ariaLabel)
		}
	}
	
	// Check for file upload requirements
	fileInputs, _ := iframe.Locator("input[type='file'][required]:visible").All()
	emptyFileCount := 0
	for _, fileInput := range fileInputs {
		files, _ := fileInput.Evaluate(`el => el.files.length`, nil)
		if files == 0 || files == nil {
			emptyFileCount++
			name, _ := fileInput.GetAttribute("name")
			log.Printf("WARNING: Required file upload is empty - name: %s", name)
		}
	}
	
	// Check for validation error messages
	errorMessages, _ := iframe.Locator(".error-message:visible, .field-error:visible, [class*='error']:visible").All()
	if len(errorMessages) > 0 {
		log.Printf("WARNING: %d error messages visible on form", len(errorMessages))
		for i, errMsg := range errorMessages {
			if i < 5 { // Log first 5 errors
				text, _ := errMsg.TextContent()
				log.Printf("  Error %d: %s", i+1, strings.TrimSpace(text))
			}
		}
	}
	
	totalValidationIssues := emptySelectCount + emptyFileCount + len(errorMessages)
	if totalValidationIssues > 0 {
		log.Printf("WARNING: Total validation issues: %d (empty selects: %d, empty files: %d, error messages: %d)", 
			totalValidationIssues, emptySelectCount, emptyFileCount, len(errorMessages))
		// Still don't return error - let's see what's actually wrong
	}
	
	log.Println("All fields appear to be filled correctly")
	return filledCount, nil
}

// submitIframeFormSimple finds and clicks submit button in iframe
func (s *BrowserAutomationServiceV2) submitIframeFormSimple(iframe playwright.FrameLocator, page playwright.Page) bool {
	log.Println("=== Looking for submit button in iframe ===")
	
	// Add a maximum time limit to prevent hanging forever
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Printf("Submit button search attempt %d/%d", attempt, maxAttempts)
		
		// Scroll to the bottom of the iframe first to make sure submit button is in view
		scrollResult, _ := page.Evaluate(`
			() => {
				const iframe = document.querySelector('#grnhse_iframe') || document.querySelector('iframe');
				if (iframe && iframe.contentWindow) {
					try {
						const doc = iframe.contentWindow.document;
						const scrollHeight = doc.body.scrollHeight;
						iframe.contentWindow.scrollTo(0, scrollHeight);
						
						// Also try to find and scroll to submit button specifically
						const submitBtn = doc.querySelector('button[type="submit"], .application--submit button');
						if (submitBtn) {
							submitBtn.scrollIntoView({ behavior: 'instant', block: 'center' });
							return { scrolled: true, foundSubmit: true, submitText: submitBtn.textContent };
						}
						return { scrolled: true, foundSubmit: false, scrollHeight: scrollHeight };
					} catch (e) {
						return { error: 'Cannot access iframe: ' + e.message };
					}
				}
				return { error: 'No iframe found' };
			}
		`)
		log.Printf("Scroll result: %+v", scrollResult)
		
		// Check if button exists and try to click it
		if s.tryClickSubmitButton(iframe, page) {
			return true
		}
		
		if attempt < maxAttempts {
			log.Println("Submit button not found, retrying...")
		}
	}
	
	log.Println("ERROR: Could not find submit button after all attempts")
	return false
}

// tryClickSubmitButton attempts to find and click the submit button
func (s *BrowserAutomationServiceV2) tryClickSubmitButton(iframe playwright.FrameLocator, page playwright.Page) bool {
	// Use JavaScript to analyze the iframe content thoroughly
	log.Println("=== ANALYZING IFRAME CONTENT FOR SUBMIT BUTTON ===")
	
	// Try to get iframe content info via JavaScript
	iframeInfo, err := page.Evaluate(`
		() => {
			const iframe = document.querySelector('#grnhse_iframe') || document.querySelector('iframe');
			if (!iframe) return { error: 'No iframe found' };
			
			try {
				const iframeDoc = iframe.contentDocument || iframe.contentWindow.document;
				
				// Find all buttons
				const buttons = Array.from(iframeDoc.querySelectorAll('button'));
				const submitButtons = Array.from(iframeDoc.querySelectorAll('button[type="submit"], .application--submit button, .btn--pill'));
				
				// Get the submit div
				const submitDiv = iframeDoc.querySelector('.application--submit');
				
				return {
					totalButtons: buttons.length,
					submitButtons: submitButtons.length,
					hasSubmitDiv: submitDiv !== null,
					buttonDetails: buttons.slice(0, 10).map(btn => ({
						text: btn.textContent.trim(),
						type: btn.type,
						className: btn.className,
						disabled: btn.disabled,
						visible: btn.offsetParent !== null,
						parent: btn.parentElement ? btn.parentElement.className : ''
					})),
					submitDivHTML: submitDiv ? submitDiv.outerHTML : null
				};
			} catch (e) {
				return { error: 'Cannot access iframe: ' + e.message };
			}
		}
	`)
	
	if err != nil {
		log.Printf("JavaScript error analyzing iframe: %v", err)
	} else {
		log.Printf("Iframe analysis result: %+v", iframeInfo)
	}
	
	// First log all buttons to see what's available
	allButtons, _ := iframe.Locator("button").All()
	log.Printf("Total buttons in iframe (via Playwright): %d", len(allButtons))
	
	visibleButtons, _ := iframe.Locator("button:visible").All()
	log.Printf("Total VISIBLE buttons in iframe: %d", len(visibleButtons))
	
	// Log ALL buttons, not just visible ones
	for i, btn := range allButtons {
		if i >= 10 { // Log first 10 buttons for debugging
			break
		}
		text, _ := btn.TextContent()
		disabled, _ := btn.IsDisabled()
		classes, _ := btn.GetAttribute("class")
		ariaLabel, _ := btn.GetAttribute("aria-label")
		btnType, _ := btn.GetAttribute("type")
		visible, _ := btn.IsVisible()
		log.Printf("  Button %d: text='%s', type='%s', visible=%v, disabled=%v, class='%s', aria-label='%s'", 
			i, strings.TrimSpace(text), btnType, visible, disabled, classes, ariaLabel)
	}
	
	// FIRST: Try the most obvious selector - button[type='submit']
	submitBtn := iframe.Locator("button[type='submit']").First()
	if count, _ := submitBtn.Count(); count > 0 {
		text, _ := submitBtn.TextContent()
		log.Printf("Found button[type='submit']: '%s'", strings.TrimSpace(text))
		
		// Check if visible and enabled
		visible, _ := submitBtn.IsVisible()
		disabled, _ := submitBtn.IsDisabled()
		ariaDisabled, _ := submitBtn.GetAttribute("aria-disabled")
		log.Printf("  Visible: %v, Disabled: %v, AriaDisabled: %v", visible, disabled, ariaDisabled)
		
		// Try to click regardless of visibility (might be scrolled out of view)
		submitBtn.ScrollIntoViewIfNeeded()
		
		// Force click it
		if err := submitBtn.Click(playwright.LocatorClickOptions{
			Force: playwright.Bool(true), // Force click even if covered or not visible
			Timeout: playwright.Float(5000), // 5 second timeout
		}); err == nil {
			log.Printf("✓ Successfully clicked submit button: '%s'", strings.TrimSpace(text))
			return true
		} else {
			log.Printf("ERROR clicking submit button: %v", err)
			
			// Try JavaScript click as fallback
			log.Println("Trying JavaScript click on submit button...")
			clicked, _ := submitBtn.Evaluate(`el => { el.click(); return true; }`, nil)
			if clicked == true {
				log.Printf("✓ Successfully clicked submit button via JavaScript")
				return true
			}
		}
	} else {
		log.Println("No button[type='submit'] found in iframe")
	}
	
	// Try multiple strategies to find submit button
	submitSelectors := []string{
		"div.application--submit button", // Button inside submit div - SPECIFIC TO GREENHOUSE
		"button.btn.btn--pill", // Specific class from your example
		"button[type='submit']",
		"button:text-is('Submit application')", // Exact text match
		"button:text('Submit application')",
		"button:text('Submit Application')",
		"input[type='submit']",
		"button:has-text('Submit')",
		"button:has-text('Apply')",
		"button:has-text('Send')",
		"button:has-text('Continue')",
		"button:has-text('Next')",
		"button:has-text('Finish')",
		"button[class*='submit']",
		"button[aria-label*='Submit']",
	}
	
	// Try each selector
	for _, selector := range submitSelectors {
		btn := iframe.Locator(selector).First()
		if count, _ := btn.Count(); count > 0 {
			visible, _ := btn.IsVisible()
			if visible {
				// Check if button is enabled
				disabled, _ := btn.IsDisabled()
				ariaDisabled, _ := btn.GetAttribute("aria-disabled")
				
				if !disabled && ariaDisabled != "true" {
					text, _ := btn.TextContent()
					log.Printf("Found potential submit button with selector '%s': '%s'", selector, strings.TrimSpace(text))
					
					// Scroll button into view first
					btn.ScrollIntoViewIfNeeded()
					
					if err := btn.Click(playwright.LocatorClickOptions{
						Force: playwright.Bool(true),
					}); err == nil {
						log.Printf("✓ Successfully clicked submit button: '%s'", strings.TrimSpace(text))
						return true
					} else {
						log.Printf("Failed to click button: %v", err)
					}
				}
			}
		}
	}
	
	// Try to find button using JavaScript as backup
	log.Println("Trying JavaScript method to find submit button...")
	result, _ := iframe.Locator("button, input[type='submit'], input[type='button']").EvaluateAll(`
		buttons => {
			let foundButtons = [];
			for (const btn of buttons) {
				const text = (btn.innerText || btn.value || '').toLowerCase();
				if (text.includes('submit') || text.includes('apply') || text.includes('send') || text.includes('continue')) {
					// Check if button is visible and enabled
					if (btn.offsetParent !== null && !btn.disabled) {
						foundButtons.push(text);
						btn.scrollIntoView();
						btn.click();
						return true;
					}
				}
			}
			console.log('Found these button texts:', foundButtons);
			return false;
		}
	`)
	
	if result == true {
		log.Println("✓ Successfully clicked submit button via JavaScript")
		return true
	}
	
	// Last resort - try clicking any visible button at the bottom of the form
	log.Println("Last resort - looking for any button at bottom of form...")
	bottomButtons, _ := iframe.Locator("button:visible").All()
	if len(bottomButtons) > 0 {
		// Try the last few buttons
		for i := len(bottomButtons) - 1; i >= 0 && i >= len(bottomButtons)-3; i-- {
			btn := bottomButtons[i]
			text, _ := btn.TextContent()
			text = strings.TrimSpace(text)
			
			// Skip navigation or cancel buttons
			textLower := strings.ToLower(text)
			if strings.Contains(textLower, "cancel") || 
			   strings.Contains(textLower, "back") || 
			   strings.Contains(textLower, "previous") ||
			   strings.Contains(textLower, "save") && !strings.Contains(textLower, "submit") {
				continue
			}
			
			// If it looks like it could be a submit button
			if text != "" && len(text) < 50 {
				log.Printf("Trying bottom button: '%s'", text)
				if err := btn.Click(); err == nil {
					log.Printf("✓ Clicked button: '%s'", text)
					return true
				}
			}
		}
	}
	
	// Final attempt - use JavaScript to find ANY submit button
	log.Println("Final attempt - using JavaScript to find and click ANY submit-like button...")
	jsClicked, _ := page.Evaluate(`
		() => {
			// Try to find submit button in iframe
			const iframe = document.querySelector('iframe');
			if (iframe && iframe.contentDocument) {
				const doc = iframe.contentDocument;
				
				// Look for button[type='submit']
				let submitBtn = doc.querySelector('button[type="submit"]');
				if (submitBtn) {
					console.log('Found submit button:', submitBtn.textContent);
					submitBtn.scrollIntoView();
					submitBtn.click();
					return 'Clicked: ' + submitBtn.textContent;
				}
				
				// Look for button with submit text
				const buttons = doc.querySelectorAll('button');
				for (const btn of buttons) {
					const text = btn.textContent.toLowerCase();
					if (text.includes('submit') || text.includes('apply') || text.includes('send')) {
						console.log('Found button with text:', btn.textContent);
						btn.scrollIntoView();
						btn.click();
						return 'Clicked: ' + btn.textContent;
					}
				}
				
				// Look for button with submit class
				submitBtn = doc.querySelector('.application--submit button, .btn--pill, button[class*="submit"]');
				if (submitBtn) {
					console.log('Found button by class:', submitBtn.textContent);
					submitBtn.scrollIntoView();
					submitBtn.click();
					return 'Clicked: ' + submitBtn.textContent;
				}
			}
			return false;
		}
	`)
	
	if jsClicked != false && jsClicked != nil {
		log.Printf("✓ JavaScript successfully clicked button: %v", jsClicked)
		return true
	}
	
	// Submit button not found after all attempts
	log.Println("ERROR: Could not find submit button after trying all methods")
	log.Println("The form has been filled but requires manual submission")
	return false
}

// handleStripeSpecificFields handles Stripe-specific form fields that might not be standard dropdowns
func (s *BrowserAutomationServiceV2) handleStripeSpecificFields(iframe playwright.FrameLocator, userData *UserProfileData) {
	log.Println("=== Handling Stripe-specific fields ===")
	
	// Handle all fields in the form
	allFields, _ := iframe.Locator(".field").All()
	log.Printf("Found %d total fields to check", len(allFields))
	
	for i, field := range allFields {
		// Get the question text
		questionText, _ := field.Locator("label, .field__label, div:first-child").First().TextContent()
		questionText = strings.TrimSpace(questionText)
		
		if questionText == "" {
			continue
		}
		
		questionLower := strings.ToLower(questionText)
		log.Printf("Field %d: %s", i+1, questionText)
		
		// Handle country selection (dropdown)
		if strings.Contains(questionLower, "country where you currently reside") {
			// Look for dropdown/select in this field
			selectElem := field.Locator("select").First()
			if count, _ := selectElem.Count(); count > 0 {
				// Try to select United States
				selectElem.SelectOption(playwright.SelectOptionValues{
					Values: &[]string{"United States"},
				})
				log.Printf("  ✓ Selected United States for country of residence")
			} else {
				// Might be a custom dropdown - try clicking and selecting
				dropdown := field.Locator("div[role='combobox'], div[class*='select']").First()
				if count, _ := dropdown.Count(); count > 0 {
					dropdown.Click()
					time.Sleep(500 * time.Millisecond)
					// Try to click United States option
					option := iframe.Locator("div:has-text('United States'):visible").First()
					if count, _ := option.Count(); count > 0 {
						option.Click()
						log.Printf("  ✓ Selected United States via custom dropdown")
					}
				}
			}
		}
		
		// Handle countries you anticipate working (checkboxes)
		if strings.Contains(questionLower, "countries you anticipate working") {
			// Find United States checkbox
			checkbox := field.Locator("input[type='checkbox'][value='United States'], label:has-text('United States') input[type='checkbox']").First()
			if count, _ := checkbox.Count(); count > 0 {
				checkbox.Check()
				log.Printf("  ✓ Checked United States for work location")
			}
		}
		
		// Handle work authorization (Yes/No radio)
		if strings.Contains(questionLower, "authorized to work in the location") {
			// Default to Yes
			yesRadio := field.Locator("input[type='radio'][value='Yes'], label:has-text('Yes') input[type='radio']").First()
			if count, _ := yesRadio.Count(); count > 0 {
				yesRadio.Check()
				log.Printf("  ✓ Selected Yes for work authorization")
			} else {
				// Try clicking the Yes label directly
				yesLabel := field.Locator("label:has-text('Yes')").First()
				if count, _ := yesLabel.Count(); count > 0 {
					yesLabel.Click()
					log.Printf("  ✓ Clicked Yes for work authorization")
				}
			}
		}
		
		// Handle sponsorship requirement (Yes/No radio)
		if strings.Contains(questionLower, "require stripe to sponsor") || strings.Contains(questionLower, "work permit") {
			// Default to No (don't need sponsorship)
			noRadio := field.Locator("input[type='radio'][value='No'], label:has-text('No') input[type='radio']").First()
			if count, _ := noRadio.Count(); count > 0 {
				noRadio.Check()
				log.Printf("  ✓ Selected No for sponsorship requirement")
			} else {
				// Try clicking the No label directly
				noLabel := field.Locator("label:has-text('No')").First()
				if count, _ := noLabel.Count(); count > 0 {
					noLabel.Click()
					log.Printf("  ✓ Clicked No for sponsorship requirement")
				}
			}
		}
		
		// Handle remote work preference (Yes/No radio)
		if strings.Contains(questionLower, "work from a remote location") {
			// Check saved preferences
			answer := "No" // Default
			if userData.ExtraQA != nil {
				for q, a := range userData.ExtraQA {
					if strings.Contains(strings.ToLower(q), "remote") {
						answer = a
						break
					}
				}
			}
			
			radio := field.Locator(fmt.Sprintf("input[type='radio'][value='%s'], label:has-text('%s') input[type='radio']", answer, answer)).First()
			if count, _ := radio.Count(); count > 0 {
				radio.Check()
				log.Printf("  ✓ Selected %s for remote work", answer)
			} else {
				// Try clicking the label directly
				label := field.Locator(fmt.Sprintf("label:has-text('%s')", answer)).First()
				if count, _ := label.Count(); count > 0 {
					label.Click()
					log.Printf("  ✓ Clicked %s for remote work", answer)
				}
			}
		}
		
		// Handle Stripe affiliate question (Yes/No radio)
		if strings.Contains(questionLower, "employed by stripe") || strings.Contains(questionLower, "stripe affiliate") {
			// Default to No
			noRadio := field.Locator("input[type='radio'][value='No'], label:has-text('No') input[type='radio']").First()
			if count, _ := noRadio.Count(); count > 0 {
				noRadio.Check()
				log.Printf("  ✓ Selected No for Stripe employment")
			} else {
				// Try clicking the No label directly
				noLabel := field.Locator("label:has-text('No')").First()
				if count, _ := noLabel.Count(); count > 0 {
					noLabel.Click()
					log.Printf("  ✓ Clicked No for Stripe employment")
				}
			}
		}
		
		// Handle WhatsApp opt-in (Yes/No radio)
		if strings.Contains(questionLower, "whatsapp") {
			// Default to No
			noRadio := field.Locator("input[type='radio'][value='No'], label:has-text('No') input[type='radio']").First()
			if count, _ := noRadio.Count(); count > 0 {
				noRadio.Check()
				log.Printf("  ✓ Selected No for WhatsApp messages")
			} else {
				// Try clicking the No label directly
				noLabel := field.Locator("label:has-text('No')").First()
				if count, _ := noLabel.Count(); count > 0 {
					noLabel.Click()
					log.Printf("  ✓ Clicked No for WhatsApp messages")
				}
			}
		}
		
		// Handle "why interested" text field
		if strings.Contains(questionLower, "why") && strings.Contains(questionLower, "interested") {
			textarea := field.Locator("textarea, input[type='text']").First()
			if count, _ := textarea.Count(); count > 0 {
				// Check saved preferences first
				answer := "I am excited about this opportunity to contribute my skills and experience to your team."
				if userData.ExtraQA != nil {
					for q, a := range userData.ExtraQA {
						if strings.Contains(strings.ToLower(q), "interested") {
							answer = a
							break
						}
					}
				}
				textarea.Fill(answer)
				log.Printf("  ✓ Filled interest reason")
			}
		}
	}
}

// checkIfFormStillPresent checks if the form is still present after submit attempt
func (s *BrowserAutomationServiceV2) checkIfFormStillPresent(page playwright.Page, iframe playwright.FrameLocator) bool {
	// Check if iframe still has form elements
	formPresent, _ := page.Evaluate(`
		() => {
			const iframe = document.querySelector('#grnhse_iframe') || document.querySelector('iframe');
			if (!iframe) return false;
			
			try {
				const iframeDoc = iframe.contentDocument || iframe.contentWindow.document;
				
				// Check for form elements
				const hasForm = iframeDoc.querySelector('form') !== null;
				const hasInputs = iframeDoc.querySelectorAll('input[type="text"], input[type="email"], textarea').length > 0;
				const hasSubmitButton = iframeDoc.querySelector('button[type="submit"], button.btn--pill') !== null;
				
				// Check if we see a confirmation message instead
				const hasConfirmation = iframeDoc.body.textContent.toLowerCase().includes('thank you') ||
				                        iframeDoc.body.textContent.toLowerCase().includes('application received') ||
				                        iframeDoc.body.textContent.toLowerCase().includes('successfully submitted');
				
				return {
					hasForm: hasForm,
					hasInputs: hasInputs,
					hasSubmitButton: hasSubmitButton,
					hasConfirmation: hasConfirmation,
					stillHasForm: (hasForm || hasInputs || hasSubmitButton) && !hasConfirmation
				};
			} catch (e) {
				return { error: e.message };
			}
		}
	`)
	
	if formInfo, ok := formPresent.(map[string]interface{}); ok {
		log.Printf("Form presence check: %+v", formInfo)
		if stillHasForm, exists := formInfo["stillHasForm"]; exists {
			if hasForm, ok := stillHasForm.(bool); ok {
				return hasForm
			}
		}
	}
	
	// Default to assuming form is still there if we can't determine
	return true
}

// getValidationErrors checks for validation errors in the form
func (s *BrowserAutomationServiceV2) getValidationErrors(page playwright.Page, iframe playwright.FrameLocator) []string {
	errors := []string{}
	
	// Check for validation errors via JavaScript
	validationInfo, _ := page.Evaluate(`
		() => {
			const iframe = document.querySelector('#grnhse_iframe') || document.querySelector('iframe');
			if (!iframe) return { errors: [], details: [] };
			
			try {
				const iframeDoc = iframe.contentDocument || iframe.contentWindow.document;
				const errors = [];
				const details = [];
				
				// Look for error messages
				const errorElements = iframeDoc.querySelectorAll('.field-error-msg, .error, .error-message, [class*="error"]:not(.field)');
				errorElements.forEach(el => {
					const text = el.textContent.trim();
					if (text && !errors.includes(text) && text.length > 0 && text.length < 200) {
						errors.push(text);
						// Try to find the associated field
						const field = el.closest('.field');
						if (field) {
							const label = field.querySelector('label')?.textContent?.trim();
							if (label) {
								details.push({ field: label, error: text });
							}
						}
					}
				});
				
				// Check for required fields that are empty
				const allFields = iframeDoc.querySelectorAll('.field');
				allFields.forEach(fieldDiv => {
					const isRequired = fieldDiv.querySelector('.required, [required]') !== null ||
					                  fieldDiv.textContent.includes('*');
					if (isRequired) {
						const input = fieldDiv.querySelector('input, textarea, select');
						if (input) {
							const value = input.value || '';
							const isEmpty = value.trim() === '' || 
							               (input.tagName === 'SELECT' && (value === 'Please select' || value === ''));
							
							if (isEmpty) {
								const label = fieldDiv.querySelector('label')?.textContent?.replace('*', '').trim() || 
								             input.getAttribute('aria-label') || 
								             input.getAttribute('placeholder') || 
								             input.getAttribute('name') || 
								             'Unknown field';
								const errorMsg = 'Required field is empty: ' + label;
								if (!errors.includes(errorMsg)) {
									errors.push(errorMsg);
									details.push({ field: label, error: 'Required field' });
								}
							}
						}
					}
				});
				
				// Check for fields with aria-invalid="true"
				const invalidFields = iframeDoc.querySelectorAll('[aria-invalid="true"]');
				invalidFields.forEach(field => {
					const label = field.closest('.field')?.querySelector('label')?.textContent?.replace('*', '').trim() || 
					             field.getAttribute('aria-label') || 
					             field.getAttribute('placeholder') || 
					             'Unknown field';
					const errorMsg = 'Invalid field: ' + label;
					if (!errors.includes(errorMsg)) {
						errors.push(errorMsg);
						details.push({ field: label, error: 'Invalid value' });
					}
				});
				
				// Check for file upload fields that might be required
				const fileUploads = iframeDoc.querySelectorAll('input[type="file"]');
				fileUploads.forEach(fileInput => {
					const fieldDiv = fileInput.closest('.field');
					if (fieldDiv) {
						const isRequired = fieldDiv.querySelector('.required') !== null || 
						                  fieldDiv.textContent.includes('*');
						if (isRequired && !fileInput.files.length) {
							const label = fieldDiv.querySelector('label')?.textContent?.replace('*', '').trim() || 'File upload';
							const errorMsg = 'Required file not uploaded: ' + label;
							if (!errors.includes(errorMsg)) {
								errors.push(errorMsg);
								details.push({ field: label, error: 'File required' });
							}
						}
					}
				});
				
				console.log('Validation check found:', errors.length, 'errors');
				console.log('Details:', details);
				
				return { errors: errors, details: details };
			} catch (e) {
				return { errors: [], details: [], error: e.message };
			}
		}
	`)
	
	if errorInfo, ok := validationInfo.(map[string]interface{}); ok {
		// Log detailed error information
		if details, exists := errorInfo["details"]; exists {
			if detailList, ok := details.([]interface{}); ok && len(detailList) > 0 {
				log.Println("=== VALIDATION ERROR DETAILS ===")
				for _, detail := range detailList {
					if d, ok := detail.(map[string]interface{}); ok {
						field := d["field"]
						error := d["error"]
						log.Printf("  - Field: %v, Error: %v", field, error)
					}
				}
				log.Println("================================")
			}
		}
		
		if errorList, exists := errorInfo["errors"]; exists {
			if errs, ok := errorList.([]interface{}); ok {
				for _, err := range errs {
					if errStr, ok := err.(string); ok {
						errors = append(errors, errStr)
					}
				}
			}
		}
	}
	
	// Also check using Playwright selectors
	errorMsgs, _ := iframe.Locator(".field-error-msg, .error-message").AllTextContents()
	for _, msg := range errorMsgs {
		msg = strings.TrimSpace(msg)
		if msg != "" && !contains(errors, msg) {
			errors = append(errors, msg)
		}
	}
	
	return errors
}

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// findAndClickApplyButton finds and clicks the apply button on job pages
func (s *BrowserAutomationServiceV2) findAndClickApplyButton(page playwright.Page) bool {
	applySelectors := []string{
		"button:has-text('Apply'):visible",
		"button:has-text('Apply Now'):visible",
		"button:has-text('Apply for this job'):visible",
		"a:has-text('Apply'):visible",
		"a:has-text('Apply Now'):visible",
	}
	
	for _, selector := range applySelectors {
		btn := page.Locator(selector).First()
		if visible, _ := btn.IsVisible(); visible {
			if err := btn.Click(); err == nil {
				// Clicked apply button
				// Continue immediately
				return true
			}
		}
	}
	
	return false
}

// handleSpecificIframeForm handles a specific iframe that was already identified
func (s *BrowserAutomationServiceV2) handleSpecificIframeForm(page playwright.Page, iframe playwright.FrameLocator, userData *UserProfileData, resumeFilePath string, result *AutomationResult) (*AutomationResult, error) {
	log.Println("=== Handling specific iframe form ===")
	
	// Debug what's in the iframe
	DebugIframeContent(iframe, page)
	
	// Fill iframe fields
	filledCount, err := s.fillIframeFieldsSimple(iframe, userData, page, resumeFilePath)
	if err != nil {
		// Only return early for critical errors, not validation warnings
		if strings.Contains(err.Error(), "Unable to fill") || 
		   strings.Contains(err.Error(), "required fields") {
			log.Printf("ERROR: Form incomplete - %v", err)
			result.Status = "incomplete_form"
			result.Message = fmt.Sprintf("Could not complete form: %v", err)
			
			// Take screenshot of incomplete form
			screenshotURL, _ := s.screenshotService.SaveScreenshotToResult(page, "incomplete_form", result)
			if screenshotURL != "" {
				result.ApplicationScreenshotKey = screenshotURL
			}
			return result, nil
		}
		// For validation errors, log but continue to try submitting
		if strings.Contains(err.Error(), "validation errors") {
			log.Printf("WARNING: Validation issues detected but attempting to submit: %v", err)
		} else {
			return result, err
		}
	}
	
	result.FilledFields["iframe_fields"] = fmt.Sprintf("%d", filledCount)
	
	// Take screenshot before submit
	screenshotURL, _ := s.screenshotService.SaveScreenshotToResult(page, "before_submit", result)
	if screenshotURL != "" {
		log.Printf("Screenshot taken before submit: %s", screenshotURL)
		result.ApplicationScreenshotKey = screenshotURL
	}
	
	// Submit form - submit button is INSIDE the iframe
	log.Println("Looking for submit button INSIDE iframe...")
	
	// Try inside iframe
	if s.submitIframeFormSimple(iframe, page) {
		log.Println("Submit button clicked - verifying submission...")
		
		// Wait a moment to see if the form actually submits
		time.Sleep(3 * time.Second)
		
		// Check if we're still on the same page (form didn't submit)
		stillHasForm := s.checkIfFormStillPresent(page, iframe)
		
		if stillHasForm {
			// Form is still there - check for validation errors
			log.Println("Form is still present after clicking submit - checking for validation errors...")
			
			validationErrors := s.getValidationErrors(page, iframe)
			if len(validationErrors) > 0 {
				log.Printf("VALIDATION ERRORS FOUND: %v", validationErrors)
				result.Status = "validation_errors"
				result.Message = fmt.Sprintf("Form has validation errors: %v", validationErrors)
				result.Success = false
				
				// Take screenshot showing validation errors
				errorScreenshot, _ := s.screenshotService.SaveScreenshotToResult(page, "validation_errors", result)
				if errorScreenshot != "" {
					result.ApplicationScreenshotKey = errorScreenshot
				}
			} else {
				// No obvious validation errors but form didn't submit
				log.Println("Form didn't submit but no validation errors detected")
				result.Status = "submit_failed"
				result.Message = "Submit button was clicked but form did not submit. Please review and submit manually."
				result.Success = false
				
				// Take screenshot of the unsubmitted form
				failedScreenshot, _ := s.screenshotService.SaveScreenshotToResult(page, "submit_failed", result)
				if failedScreenshot != "" {
					result.ApplicationScreenshotKey = failedScreenshot
				}
			}
		} else {
			// Form seems to have submitted successfully
			log.Println("Form appears to have submitted successfully!")
			result.Success = true
			result.Status = "submitted"
			result.Message = "Application submitted via iframe"
			
			// Take confirmation screenshot
			confirmURL, _ := s.screenshotService.SaveScreenshotToResult(page, "confirmation", result)
			if confirmURL != "" {
				result.ConfirmationScreenshotKey = confirmURL
			}
		}
	} else {
		log.Printf("WARNING: Could not find submit button")
		result.Status = "submit_button_not_found"
		result.Message = "Application filled successfully but submit button could not be found. Please review and submit manually."
		result.Success = false
		
		// Still take a screenshot to show the filled form
		finalScreenshot, _ := s.screenshotService.SaveScreenshotToResult(page, "filled_form", result)
		if finalScreenshot != "" {
			result.ApplicationScreenshotKey = finalScreenshot
		}
	}
	
	return result, nil
}


