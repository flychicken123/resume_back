package middleware

import (
	"net/http"
	"strings"
	"github.com/gin-gonic/gin"
	"resumeai/utils"
)

// MaxRequestSize limits the request body size
func MaxRequestSize(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)
		c.Next()
	}
}

// ValidateContentType ensures the request has expected content type
func ValidateContentType(expectedTypes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		contentType := c.GetHeader("Content-Type")
		
		// Skip validation for GET and DELETE requests
		if c.Request.Method == "GET" || c.Request.Method == "DELETE" {
			c.Next()
			return
		}
		
		valid := false
		for _, expectedType := range expectedTypes {
			if strings.Contains(contentType, expectedType) {
				valid = true
				break
			}
		}
		
		if !valid {
			utils.BadRequestError(c, "Invalid content type", nil)
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// ValidateJSON middleware for JSON request validation
func ValidateJSON() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip validation for GET and DELETE requests
		if c.Request.Method == "GET" || c.Request.Method == "DELETE" || c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}
		
		contentType := c.GetHeader("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			utils.BadRequestError(c, "Content-Type must be application/json", nil)
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// SanitizeInput removes potentially dangerous characters from input
func SanitizeInput() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get all query parameters and sanitize them
		queryParams := c.Request.URL.Query()
		for key, values := range queryParams {
			for i, value := range values {
				queryParams[key][i] = sanitizeString(value)
			}
		}
		c.Request.URL.RawQuery = queryParams.Encode()
		
		c.Next()
	}
}

// sanitizeString removes potentially dangerous characters
func sanitizeString(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")
	
	// Trim whitespace
	input = strings.TrimSpace(input)
	
	// Limit length to prevent buffer overflow attacks
	if len(input) > 10000 {
		input = input[:10000]
	}
	
	return input
}