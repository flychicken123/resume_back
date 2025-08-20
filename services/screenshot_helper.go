package services

import (
	"fmt"
	"strings"
)

// ScreenshotHelper provides utility functions for screenshot URLs
type ScreenshotHelper struct {
	BaseURL string // Base URL for the API (e.g., http://localhost:8081/api)
}

func NewScreenshotHelper(baseURL string) *ScreenshotHelper {
	return &ScreenshotHelper{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
	}
}

// GetScreenshotURL converts an S3 key to an API URL
func (h *ScreenshotHelper) GetScreenshotURL(s3Key string) string {
	if s3Key == "" {
		return ""
	}
	
	// Remove "screenshots/" prefix if present for the URL
	key := strings.TrimPrefix(s3Key, "screenshots/")
	
	// Return the API endpoint that will generate a pre-signed URL
	return fmt.Sprintf("%s/screenshots/%s", h.BaseURL, key)
}

// GetDirectS3URL generates a direct S3 URL (for debugging)
func GetDirectS3URL(s3Key string) string {
	if s3Key == "" {
		return ""
	}
	
	// This is just for reference - actual URLs should use pre-signed URLs
	return fmt.Sprintf("https://airesumestorage.s3.us-east-2.amazonaws.com/%s", s3Key)
}