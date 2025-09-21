package handlers

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ParseExtractJobDescription fetches and extracts job description without AI
func ParseExtractJobDescription(c *gin.Context) {
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
		fmt.Printf("[ParseExtractJobDescription] Error creating request: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create request: %v", err)})
		return
	}

	// Set user agent to avoid being blocked
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	httpReq.Header.Set("Accept-Language", "en-US,en;q=0.5")

	// Fetch the URL
	resp, err := client.Do(httpReq)
	if err != nil {
		fmt.Printf("[ParseExtractJobDescription] Error fetching URL %s: %v\n", req.URL, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch URL: %v", err)})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[ParseExtractJobDescription] Bad status code %d for URL %s\n", resp.StatusCode, req.URL)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch URL: status code %d", resp.StatusCode)})
		return
	}

	// Read the response body (limit to 1MB)
	limitedReader := io.LimitReader(resp.Body, 1024*1024)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		fmt.Printf("[ParseExtractJobDescription] Error reading response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read response: %v", err)})
		return
	}

	htmlContent := string(bodyBytes)

	// Extract text content from HTML
	description := extractJobDescriptionFromHTML(htmlContent)

	if description == "" {
		// If we couldn't extract a job description, return the cleaned text
		description = cleanHTMLToText(htmlContent)
		if len(description) > 5000 {
			description = description[:5000] + "..."
		}
	}

	fmt.Printf("[ParseExtractJobDescription] Successfully extracted %d characters from %s\n", len(description), req.URL)

	response := JobExtractResponse{
		Description: description,
	}

	c.JSON(http.StatusOK, response)
}

// extractJobDescriptionFromHTML attempts to extract job description from common patterns
func extractJobDescriptionFromHTML(html string) string {
	// Remove script and style tags
	re := regexp.MustCompile(`(?i)<script[^>]*>[\s\S]*?</script>|<style[^>]*>[\s\S]*?</style>`)
	html = re.ReplaceAllString(html, "")

	// Look for common job description patterns
	patterns := []string{
		`(?i)<div[^>]*class="[^"]*job-description[^"]*"[^>]*>([\s\S]*?)</div>`,
		`(?i)<div[^>]*class="[^"]*description[^"]*"[^>]*>([\s\S]*?)</div>`,
		`(?i)<div[^>]*id="[^"]*job-description[^"]*"[^>]*>([\s\S]*?)</div>`,
		`(?i)<section[^>]*class="[^"]*description[^"]*"[^>]*>([\s\S]*?)</section>`,
		`(?i)<article[^>]*>([\s\S]*?)</article>`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			content := cleanHTMLToText(matches[1])
			if len(content) > 200 { // Likely a real job description
				return content
			}
		}
	}

	return ""
}

// cleanHTMLToText removes HTML tags and cleans up text
func cleanHTMLToText(html string) string {
	// First decode HTML entities that might contain tags
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")

	// Remove HTML tags
	re := regexp.MustCompile(`<[^>]+>`)
	text := re.ReplaceAllString(html, " ")

	// Decode remaining HTML entities
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&rsquo;", "'")
	text = strings.ReplaceAll(text, "&lsquo;", "'")
	text = strings.ReplaceAll(text, "&rdquo;", "\"")
	text = strings.ReplaceAll(text, "&ldquo;", "\"")
	text = strings.ReplaceAll(text, "&mdash;", "-")
	text = strings.ReplaceAll(text, "&ndash;", "-")
	text = strings.ReplaceAll(text, "&#8217;", "'")
	text = strings.ReplaceAll(text, "&#8220;", "\"")
	text = strings.ReplaceAll(text, "&#8221;", "\"")
	text = strings.ReplaceAll(text, "&#8211;", "-")
	text = strings.ReplaceAll(text, "&#8212;", "-")

	// Clean up whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	return text
}
