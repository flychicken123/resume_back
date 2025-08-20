package services

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"
	
	"github.com/playwright-community/playwright-go"
)

// ScreenshotService handles screenshot capture and upload
type ScreenshotService struct {
	S3Service *S3Service
}

// NewScreenshotService creates a new screenshot service
func NewScreenshotService() *ScreenshotService {
	s3Service, err := NewS3Service()
	if err != nil {
		log.Printf("Warning: S3 service not initialized: %v", err)
		// Return service without S3 (screenshots won't be uploaded)
		return &ScreenshotService{
			S3Service: nil,
		}
	}
	return &ScreenshotService{
		S3Service: s3Service,
	}
}

// CaptureAndUpload takes a screenshot and uploads it to S3
func (s *ScreenshotService) CaptureAndUpload(page playwright.Page, screenshotType string) (string, error) {
	log.Printf("Taking screenshot: %s", screenshotType)
	
	// Check if S3 service is available
	if s.S3Service == nil {
		log.Printf("S3 service not available - screenshot will not be uploaded")
		return "", nil
	}
	
	// Ensure page is fully expanded before screenshot
	page.Evaluate(`
		// Expand all containers to show full content
		document.querySelectorAll('*').forEach(el => {
			if (el.style) {
				const computed = window.getComputedStyle(el);
				if (computed.overflow === 'hidden' || computed.overflow === 'auto') {
					el.style.overflow = 'visible';
				}
				if (computed.maxHeight && computed.maxHeight !== 'none') {
					el.style.maxHeight = 'none';
				}
			}
		});
		
		// Scroll to load any lazy content
		window.scrollTo(0, document.body.scrollHeight);
		window.scrollTo(0, 0);
	`)
	
	// Take screenshot
	screenshotBytes, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("failed to take screenshot: %v", err)
	}
	
	// Generate unique filename
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("screenshots/%s_%d.png", screenshotType, timestamp)
	
	// Save to temp file first (S3Service expects file path)
	tempFile, err := ioutil.TempFile("", "screenshot_*.png")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	
	if _, err := tempFile.Write(screenshotBytes); err != nil {
		return "", fmt.Errorf("failed to write screenshot to temp file: %v", err)
	}
	tempFile.Close()
	
	// Upload to S3
	log.Printf("Uploading screenshot to S3 with key: %s", filename)
	url, err := s.S3Service.UploadFile(tempFile.Name(), filename)
	if err != nil {
		return "", fmt.Errorf("failed to upload screenshot: %v", err)
	}
	
	log.Printf("Screenshot uploaded to S3 with key: %s", filename)
	return url, nil
}

// CaptureElement takes a screenshot of a specific element
func (s *ScreenshotService) CaptureElement(element playwright.Locator, screenshotType string) (string, error) {
	log.Printf("Taking element screenshot: %s", screenshotType)
	
	// Check if S3 service is available
	if s.S3Service == nil {
		log.Printf("S3 service not available - element screenshot will not be uploaded")
		return "", nil
	}
	
	// Take screenshot of element
	screenshotBytes, err := element.Screenshot()
	if err != nil {
		return "", fmt.Errorf("failed to take element screenshot: %v", err)
	}
	
	// Generate unique filename
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("screenshots/element_%s_%d.png", screenshotType, timestamp)
	
	// Save to temp file first
	tempFile, err := ioutil.TempFile("", "element_screenshot_*.png")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	
	if _, err := tempFile.Write(screenshotBytes); err != nil {
		return "", fmt.Errorf("failed to write screenshot to temp file: %v", err)
	}
	tempFile.Close()
	
	// Upload to S3
	url, err := s.S3Service.UploadFile(tempFile.Name(), filename)
	if err != nil {
		return "", fmt.Errorf("failed to upload element screenshot: %v", err)
	}
	
	log.Printf("Element screenshot uploaded to S3 with key: %s", filename)
	return url, nil
}

// SaveScreenshotToResult saves screenshot URL to result object
func (s *ScreenshotService) SaveScreenshotToResult(page playwright.Page, screenshotType string, result *AutomationResult) (string, error) {
	url, err := s.CaptureAndUpload(page, screenshotType)
	if err != nil {
		log.Printf("Failed to save screenshot: %v", err)
		return "", err
	}
	
	// Store in result based on type
	switch screenshotType {
	case "before_submit_verification":
		result.ApplicationScreenshotKey = url
	case "after_submit_confirmation":
		result.ConfirmationScreenshotKey = url
	case "missing_fields":
		result.ApplicationScreenshotKey = url
	default:
		// Store in generic screenshot field
		if result.ApplicationScreenshotKey == "" {
			result.ApplicationScreenshotKey = url
		}
	}
	
	log.Printf("✓ %s screenshot saved: %s", screenshotType, url)
	return url, nil
}