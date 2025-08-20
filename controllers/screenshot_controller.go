package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"resumeai/services"
)

type ScreenshotController struct {
	s3Service *services.S3Service
}

func NewScreenshotController() *ScreenshotController {
	s3Service, err := services.NewS3Service()
	if err != nil {
		// S3 not configured, screenshots won't be available
		return &ScreenshotController{
			s3Service: nil,
		}
	}
	
	return &ScreenshotController{
		s3Service: s3Service,
	}
}

// GetScreenshot generates a pre-signed URL for accessing a screenshot
func (c *ScreenshotController) GetScreenshot(ctx *gin.Context) {
	// Get the screenshot key from the URL path
	// Expected format: /api/screenshots/screenshots/filename.png
	screenshotKey := ctx.Param("key")
	
	// Clean up the key if it has extra slashes
	screenshotKey = strings.TrimPrefix(screenshotKey, "/")
	
	// If the key doesn't start with "screenshots/", add it
	if !strings.HasPrefix(screenshotKey, "screenshots/") {
		screenshotKey = "screenshots/" + screenshotKey
	}
	
	if c.s3Service == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Screenshot service not available",
		})
		return
	}
	
	// Generate pre-signed URL that expires in 1 hour
	presignedURL, err := c.s3Service.GeneratePresignedURL(screenshotKey)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate screenshot URL",
			"details": err.Error(),
		})
		return
	}
	
	// Redirect to the pre-signed URL
	ctx.Redirect(http.StatusTemporaryRedirect, presignedURL)
}

// GetScreenshotURL returns a pre-signed URL as JSON instead of redirecting
func (c *ScreenshotController) GetScreenshotURL(ctx *gin.Context) {
	screenshotKey := ctx.Query("key")
	
	if screenshotKey == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Screenshot key is required",
		})
		return
	}
	
	// Clean up the key
	screenshotKey = strings.TrimPrefix(screenshotKey, "/")
	if !strings.HasPrefix(screenshotKey, "screenshots/") && !strings.Contains(screenshotKey, "/") {
		screenshotKey = "screenshots/" + screenshotKey
	}
	
	if c.s3Service == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Screenshot service not available",
		})
		return
	}
	
	// Generate pre-signed URL
	presignedURL, err := c.s3Service.GeneratePresignedURL(screenshotKey)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate screenshot URL",
			"details": err.Error(),
		})
		return
	}
	
	ctx.JSON(http.StatusOK, gin.H{
		"url": presignedURL,
		"expires_in": "1 hour",
		"key": screenshotKey,
	})
}