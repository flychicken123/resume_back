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
	
	log.Printf("Expanding page for full screenshot...")
	
	// First, get the actual content height of the iframe
	_, err := page.Evaluate(`
		() => {
			const iframe = document.querySelector('#grnhse_iframe') || document.querySelector('iframe');
			if (!iframe) {
				return { found: false, height: 0 };
			}
			
			// Try to get the actual content height from the iframe
			let contentHeight = 3000; // Default height
			try {
				if (iframe.contentDocument && iframe.contentDocument.body) {
					// We can access the iframe content (same origin)
					const doc = iframe.contentDocument;
					const body = doc.body;
					const html = doc.documentElement;
					
					// Get the maximum height from various measurements
					contentHeight = Math.max(
						body.scrollHeight,
						body.offsetHeight,
						html.clientHeight,
						html.scrollHeight,
						html.offsetHeight
					);
					
					// Add some padding for safety
					contentHeight += 500;
					console.log('Detected iframe content height:', contentHeight);
				}
			} catch (e) {
				// Cross-origin iframe, use a large default
				contentHeight = 5000;
				console.log('Cross-origin iframe, using default height:', contentHeight);
			}
			
			return { found: true, height: contentHeight };
		}
	`)
	
	if err != nil {
		log.Printf("Warning: Could not detect iframe height: %v", err)
	}
	
	// Now expand the iframe to the detected height
	_, err = page.Evaluate(`
		(height) => {
			// Find and expand iframe if present
			const iframe = document.querySelector('#grnhse_iframe') || document.querySelector('iframe');
			if (iframe) {
				// Set the calculated height for the iframe
				const finalHeight = height + 'px';
				iframe.style.height = finalHeight;
				iframe.style.minHeight = finalHeight;
				iframe.style.maxHeight = 'none';
				iframe.style.width = '100%';
				iframe.style.border = 'none';
				
				// Make sure iframe container can expand
				if (iframe.parentElement) {
					iframe.parentElement.style.height = 'auto';
					iframe.parentElement.style.minHeight = finalHeight;
					iframe.parentElement.style.maxHeight = 'none';
					iframe.parentElement.style.overflow = 'visible';
				}
			}
			
			// Expand body and html to accommodate the iframe
			const expandedHeight = (height + 1000) + 'px';
			document.body.style.minHeight = expandedHeight;
			document.body.style.height = 'auto';
			document.documentElement.style.minHeight = expandedHeight;
			document.documentElement.style.height = 'auto';
			
			// Remove any overflow hidden that might clip the content
			document.body.style.overflow = 'visible';
			document.documentElement.style.overflow = 'visible';
			
			// Remove any max-height restrictions
			document.body.style.maxHeight = 'none';
			document.documentElement.style.maxHeight = 'none';
			
			// Scroll to top before screenshot
			window.scrollTo(0, 0);
			
			return true;
		}
	`, 5000)
	
	if err != nil {
		log.Printf("Warning: Could not expand page: %v", err)
	}
	
	// Wait a bit longer for content to render after expansion
	time.Sleep(1000 * time.Millisecond)
	
	log.Printf("Taking full page screenshot...")
	
	// Take screenshot of the current page
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
	
	log.Printf("Screenshot uploaded successfully: %s", filename)
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