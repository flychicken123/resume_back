package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// SimpleExtractJobDescription fetches and returns raw HTML from a URL
func SimpleExtractJobDescription(c *gin.Context) {
	var req JobExtractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request. Please provide a valid URL."})
		return
	}

	// Validate URL
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide a valid URL starting with http:// or https://"})
		return
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create request
	httpReq, err := http.NewRequest("GET", req.URL, nil)
	if err != nil {
		fmt.Printf("[SimpleExtractJobDescription] Error creating request: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create request: %v", err)})
		return
	}

	// Set user agent to avoid being blocked
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	// Fetch the URL
	resp, err := client.Do(httpReq)
	if err != nil {
		fmt.Printf("[SimpleExtractJobDescription] Error fetching URL %s: %v\n", req.URL, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch URL: %v", err)})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[SimpleExtractJobDescription] Bad status code %d for URL %s\n", resp.StatusCode, req.URL)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch URL: status code %d", resp.StatusCode)})
		return
	}

	// Read the response body (limit to 100KB for testing)
	limitedReader := io.LimitReader(resp.Body, 100*1024)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		fmt.Printf("[SimpleExtractJobDescription] Error reading response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read response: %v", err)})
		return
	}

	htmlContent := string(bodyBytes)

	// For now, just return the first 1000 characters of HTML to verify fetch works
	preview := htmlContent
	if len(preview) > 1000 {
		preview = preview[:1000] + "..."
	}

	fmt.Printf("[SimpleExtractJobDescription] Successfully fetched %d bytes from %s\n", len(bodyBytes), req.URL)

	c.JSON(http.StatusOK, gin.H{
		"status":        "success",
		"url":           req.URL,
		"contentLength": len(bodyBytes),
		"preview":       preview,
		"message":       "Successfully fetched HTML. Full AI extraction not yet implemented.",
	})
}
