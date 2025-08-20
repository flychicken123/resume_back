package services

import (
	"log"
	"strings"
	
	"github.com/playwright-community/playwright-go"
)

// SubmissionCheckerService checks for successful form submission
type SubmissionCheckerService struct{}

// CheckForSuccess checks if the application was successfully submitted
func (s *SubmissionCheckerService) CheckForSuccess(page playwright.Page) bool {
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
	if s.checkURLForSuccess(pageURL) {
		log.Printf("Found success keyword in URL: %s", pageURL)
		return true
	}
	
	// Check page title
	if s.checkTitleForSuccess(pageTitle) {
		log.Printf("Found success keyword in title: %s", pageTitle)
		return true
	}
	
	// Check for success elements on page
	for _, indicator := range successIndicators {
		element := page.Locator(indicator).First()
		if visible, _ := element.IsVisible(); visible {
			text, _ := element.TextContent()
			log.Printf("Found success indicator: %s (text: %s)", indicator, text)
			return true
		}
	}
	
	// Check for redirect to success page
	if strings.Contains(pageURL, "/success") || 
	   strings.Contains(pageURL, "/confirmation") ||
	   strings.Contains(pageURL, "/thank-you") ||
	   strings.Contains(pageURL, "/complete") {
		log.Printf("Redirected to success page: %s", pageURL)
		return true
	}
	
	log.Printf("No success indicators found")
	return false
}

func (s *SubmissionCheckerService) checkURLForSuccess(url string) bool {
	urlLower := strings.ToLower(url)
	successKeywords := []string{
		"success",
		"confirmation",
		"thank",
		"complete",
		"submitted",
		"received",
	}
	
	for _, keyword := range successKeywords {
		if strings.Contains(urlLower, keyword) {
			return true
		}
	}
	
	return false
}

func (s *SubmissionCheckerService) checkTitleForSuccess(title string) bool {
	titleLower := strings.ToLower(title)
	successKeywords := []string{
		"thank you",
		"success",
		"submitted",
		"complete",
		"received",
		"confirmation",
	}
	
	for _, keyword := range successKeywords {
		if strings.Contains(titleLower, keyword) {
			return true
		}
	}
	
	return false
}

// FindAndClickSubmitButton finds and clicks the submit button
func (s *SubmissionCheckerService) FindAndClickSubmitButton(page playwright.Page) bool {
	submitSelectors := []string{
		"button[type='submit']:visible",
		"input[type='submit']:visible",
		"button:has-text('Submit Application'):visible",
		"button:has-text('Submit'):visible",
		"button:has-text('Apply'):visible",
		"button:has-text('Send Application'):visible",
		"button:has-text('Continue'):visible",
		"button:has-text('Next'):visible",
		"button[class*='submit']:visible",
		"button[id*='submit']:visible",
		"a:has-text('Submit Application'):visible",
	}
	
	for _, selector := range submitSelectors {
		button := page.Locator(selector).First()
		if visible, _ := button.IsVisible(); visible {
			text, _ := button.TextContent()
			log.Printf("Found submit button: %s (text: %s)", selector, text)
			
			// Check if button is enabled
			if disabled, _ := button.IsDisabled(); !disabled {
				if err := button.Click(); err == nil {
					log.Printf("✓ Clicked submit button")
					return true
				} else {
					log.Printf("Failed to click submit button: %v", err)
				}
			} else {
				log.Printf("Submit button is disabled")
			}
		}
	}
	
	log.Printf("No submit button found")
	return false
}

// UploadResume handles resume file upload
func (s *SubmissionCheckerService) UploadResume(page playwright.Page, resumeFilePath string) bool {
	if resumeFilePath == "" {
		return false
	}
	
	// Find file input
	fileInputSelectors := []string{
		"input[type='file']",
		"input[name='resume']",
		"input[name='cv']",
		"input[accept*='pdf']",
		"input[accept*='doc']",
	}
	
	for _, selector := range fileInputSelectors {
		fileInput := page.Locator(selector).First()
		if exists, _ := fileInput.Count(); exists > 0 {
			log.Printf("Found file upload field with selector: %s", selector)
			
			// Set the file
			if err := fileInput.SetInputFiles(resumeFilePath); err == nil {
				log.Printf("✓ Successfully uploaded resume file: %s", resumeFilePath)
				return true
			} else {
				log.Printf("Failed to upload resume: %v", err)
			}
		}
	}
	
	return false
}