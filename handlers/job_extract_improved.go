package handlers

import (
    "compress/gzip"
    "compress/zlib"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ImprovedExtractJobDescription handles job extraction with better parsing
func ImprovedExtractJobDescription(c *gin.Context) {
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

    // Check if it's a Greenhouse-hosted job (via company domain)
    if desc, title, company, _ := extractGreenhouseJobData(req.URL); desc != "" {
        c.JSON(http.StatusOK, JobExtractResponse{
            Description: desc,
            Title:       title,
            Company:     company,
        })
        return
    }

	// Check if it's a LinkedIn URL that requires login
	if strings.Contains(req.URL, "linkedin.com") && !strings.Contains(req.URL, "/public/") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "LinkedIn jobs require authentication. Please copy and paste the job description directly instead of using the URL.",
			"suggestion": "Copy the job description from LinkedIn and paste it in the job description field.",
		})
		return
	}

	// Check for other sites that require authentication
	protectedSites := []string{"glassdoor.com", "angel.co", "wellfound.com"}
	for _, site := range protectedSites {
		if strings.Contains(req.URL, site) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("This website requires authentication. Please copy and paste the job description directly."),
				"suggestion": "Copy the job description from the website and paste it in the job description field.",
			})
			return
		}
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow up to 5 redirects
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			// Check if redirect is to a login page
			if strings.Contains(req.URL.Path, "/login") || strings.Contains(req.URL.Path, "/signin") {
				return fmt.Errorf("redirect to login page detected")
			}
			return nil
		},
	}

	// Create request
	httpReq, err := http.NewRequest("GET", req.URL, nil)
	if err != nil {
		fmt.Printf("[ImprovedExtractJobDescription] Error creating request: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create request: %v", err)})
		return
	}

	// Set headers to appear more like a real browser
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
    httpReq.Header.Set("Accept-Language", "en-US,en;q=0.5")
    // Do NOT set Accept-Encoding manually. Let net/http handle gzip automatically.
	httpReq.Header.Set("DNT", "1")
	httpReq.Header.Set("Connection", "keep-alive")
	httpReq.Header.Set("Upgrade-Insecure-Requests", "1")

	// Fetch the URL
	resp, err := client.Do(httpReq)
	if err != nil {
		if strings.Contains(err.Error(), "login page") {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "This job posting requires authentication. Please copy and paste the job description directly.",
				"suggestion": "Copy the job description from the website and paste it in the job description field.",
			})
			return
		}
		fmt.Printf("[ImprovedExtractJobDescription] Error fetching URL %s: %v\n", req.URL, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch URL: %v", err)})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "This job posting requires authentication. Please copy and paste the job description directly.",
			"suggestion": "Copy the job description from the website and paste it in the job description field.",
		})
		return
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[ImprovedExtractJobDescription] Bad status code %d for URL %s\n", resp.StatusCode, req.URL)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch URL: status code %d", resp.StatusCode)})
		return
	}

    // Read the response body (limit to 2MB for job descriptions), handling compression
    var reader io.Reader = resp.Body
    // If server still sends compressed content, decompress here
    switch strings.ToLower(resp.Header.Get("Content-Encoding")) {
    case "gzip":
        if gr, gzErr := gzip.NewReader(resp.Body); gzErr == nil {
            defer gr.Close()
            reader = gr
        }
    case "deflate":
        if zr, zErr := zlib.NewReader(resp.Body); zErr == nil {
            defer zr.Close()
            reader = zr
        }
    }

    limitedReader := io.LimitReader(reader, 2*1024*1024)
    bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		fmt.Printf("[ImprovedExtractJobDescription] Error reading response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read response: %v", err)})
		return
	}

	htmlContent := string(bodyBytes)
	
	// Check if we got a login page
	if detectLoginPage(htmlContent) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "This job posting requires authentication. Please copy and paste the job description directly.",
			"suggestion": "Copy the job description from the website and paste it in the job description field.",
		})
		return
	}

	// Extract job description using improved patterns
	description := extractJobDescriptionImproved(htmlContent, req.URL)
	
	if description == "" || len(description) < 100 {
		// If we couldn't extract a meaningful job description
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Could not extract job description from this URL. The page might require login or use dynamic content loading.",
			"suggestion": "Please copy and paste the job description directly from the website.",
		})
		return
	}

	fmt.Printf("[ImprovedExtractJobDescription] Successfully extracted %d characters from %s\n", len(description), req.URL)
	
	response := JobExtractResponse{
		Description: description,
	}

	c.JSON(http.StatusOK, response)
}

// detectLoginPage checks if the HTML content is a login page
func detectLoginPage(html string) bool {
	loginIndicators := []string{
		"sign in",
		"log in",
		"signin",
		"login",
		"password",
		"email address",
		"create account",
		"forgot password",
		"remember me",
	}
	
	htmlLower := strings.ToLower(html)
	matchCount := 0
	
	for _, indicator := range loginIndicators {
		if strings.Contains(htmlLower, indicator) {
			matchCount++
		}
	}
	
	// If we find 3 or more login-related terms, it's likely a login page
	return matchCount >= 3
}

// extractJobDescriptionImproved uses better patterns to extract job descriptions
func extractJobDescriptionImproved(html string, url string) string {
	// Remove script and style tags first
	re := regexp.MustCompile(`(?i)<script[^>]*>[\s\S]*?</script>|<style[^>]*>[\s\S]*?</style>|<noscript[^>]*>[\s\S]*?</noscript>`)
	html = re.ReplaceAllString(html, " ")
	
	// Site-specific extraction patterns
	if strings.Contains(url, "indeed.com") {
		return extractIndeedJob(html)
	} else if strings.Contains(url, "greenhouse.io") {
		return extractGreenhouseHTML(html)
	} else if strings.Contains(url, "lever.co") {
		return extractLeverJob(html)
	} else if strings.Contains(url, "workday.com") || strings.Contains(url, "myworkdayjobs.com") {
		return extractWorkdayJob(html)
	}
	
	// Generic extraction for unknown sites
	return extractGenericJob(html)
}

// extractIndeedJob extracts job description from Indeed
func extractIndeedJob(html string) string {
	patterns := []string{
		`(?i)<div[^>]*class="[^"]*jobsearch-jobDescriptionText[^"]*"[^>]*>([\s\S]*?)</div>`,
		`(?i)<div[^>]*id="jobDescriptionText"[^>]*>([\s\S]*?)</div>`,
	}
	return tryPatterns(html, patterns)
}

// extractGreenhouseHTML extracts job description from Greenhouse HTML
func extractGreenhouseHTML(html string) string {
	patterns := []string{
		`(?i)<div[^>]*class="[^"]*content[^"]*"[^>]*>([\s\S]*?)</div>`,
		`(?i)<div[^>]*id="content"[^>]*>([\s\S]*?)</div>`,
	}
	return tryPatterns(html, patterns)
}

// extractLeverJob extracts job description from Lever
func extractLeverJob(html string) string {
	patterns := []string{
		`(?i)<div[^>]*class="[^"]*section[^"]*"[^>]*>([\s\S]*?)</div>`,
		`(?i)<div[^>]*class="[^"]*content[^"]*"[^>]*>([\s\S]*?)</div>`,
	}
	return tryPatterns(html, patterns)
}

// extractWorkdayJob extracts job description from Workday
func extractWorkdayJob(html string) string {
	patterns := []string{
		`(?i)<div[^>]*data-automation-id="jobPostingDescription"[^>]*>([\s\S]*?)</div>`,
		`(?i)<div[^>]*class="[^"]*job-description[^"]*"[^>]*>([\s\S]*?)</div>`,
	}
	return tryPatterns(html, patterns)
}

// extractGenericJob attempts generic extraction
func extractGenericJob(html string) string {
	// Look for common job description patterns
	patterns := []string{
		// Look for job description sections
		`(?i)<div[^>]*class="[^"]*job[-_]?description[^"]*"[^>]*>([\s\S]*?)</div>`,
		`(?i)<div[^>]*class="[^"]*description[^"]*"[^>]*>([\s\S]*?)</div>`,
		`(?i)<section[^>]*class="[^"]*description[^"]*"[^>]*>([\s\S]*?)</section>`,
		`(?i)<article[^>]*>([\s\S]*?)</article>`,
		
		// Look for main content areas
		`(?i)<div[^>]*class="[^"]*content[^"]*"[^>]*>([\s\S]*?)</div>`,
		`(?i)<main[^>]*>([\s\S]*?)</main>`,
		
		// Look for ID-based patterns
		`(?i)<div[^>]*id="[^"]*job[-_]?description[^"]*"[^>]*>([\s\S]*?)</div>`,
		`(?i)<div[^>]*id="[^"]*description[^"]*"[^>]*>([\s\S]*?)</div>`,
	}
	
	result := tryPatterns(html, patterns)
	
	// If we still don't have content, try to extract the body content
	if result == "" || len(result) < 100 {
		bodyPattern := `(?i)<body[^>]*>([\s\S]*?)</body>`
		re := regexp.MustCompile(bodyPattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			result = cleanHTMLToText(matches[1])
			// Limit to reasonable size for a job description
			if len(result) > 10000 {
				result = result[:10000] + "..."
			}
		}
	}
	
	return result
}

// tryPatterns attempts multiple regex patterns and returns the first successful match
func tryPatterns(html string, patterns []string) string {
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

// extractCoreJobDescription extracts only the relevant job description sections
func extractCoreJobDescription(htmlContent string) string {
	// First decode HTML entities
	htmlContent = strings.ReplaceAll(htmlContent, "&lt;", "<")
	htmlContent = strings.ReplaceAll(htmlContent, "&gt;", ">")
	htmlContent = strings.ReplaceAll(htmlContent, "&quot;", "\"")
	htmlContent = strings.ReplaceAll(htmlContent, "&#39;", "'")
	
	// Split content into sections by headers
	sections := regexp.MustCompile(`(?i)<h[1-6][^>]*>(.*?)</h[1-6]>`).Split(htmlContent, -1)
	headers := regexp.MustCompile(`(?i)<h[1-6][^>]*>(.*?)</h[1-6]>`).FindAllStringSubmatch(htmlContent, -1)
	
	var result strings.Builder
	relevantHeaders := regexp.MustCompile(`(?i)(about|role|position|responsibilities|what you.*do|what you.*bring|requirements|qualifications|who you are|skills|experience|duties)`)
	stopHeaders := regexp.MustCompile(`(?i)(benefits|offer|perks|compensation|salary|workplace|equal|diversity|compliance|export|privacy|disclaimer)`)
	
	for i, header := range headers {
		if len(header) > 1 {
			headerText := cleanHTMLToText(header[1])
			
			// Check if this is a section we want to stop at
			if stopHeaders.MatchString(headerText) {
				break
			}
			
			// Check if this is a relevant section
			if relevantHeaders.MatchString(headerText) {
				if result.Len() > 0 {
					result.WriteString("\n\n")
				}
				result.WriteString(headerText)
				result.WriteString("\n")
				
				// Add the content after this header
				if i+1 < len(sections) {
					content := cleanHTMLToText(sections[i+1])
					if content != "" {
						result.WriteString(content)
					}
				}
			}
		}
	}
	
	// If we didn't find structured sections, fall back to extracting the first part
	if result.Len() == 0 {
		fullText := cleanHTMLToText(htmlContent)
		// Find where benefits/compensation section starts and cut there
		stopWords := []string{
			"What We Offer",
			"Benefits",
			"Compensation",
			"The base salary",
			"Our Workplace",
			"Equal Opportunity",
			"Export Control",
		}
		
		minIndex := len(fullText)
		for _, stopWord := range stopWords {
			if idx := strings.Index(fullText, stopWord); idx > 0 && idx < minIndex {
				minIndex = idx
			}
		}
		
		if minIndex < len(fullText) {
			fullText = fullText[:minIndex]
		}
		
		// Limit to reasonable length for a job description
		if len(fullText) > 4000 {
			fullText = fullText[:4000] + "..."
		}
		
		return strings.TrimSpace(fullText)
	}
	
	return strings.TrimSpace(result.String())
}

// extractGreenhouseJob checks if URL is a Greenhouse-hosted job and fetches via API
func extractGreenhouseJob(jobURL string) string {
	// Parse the URL
	parsedURL, err := url.Parse(jobURL)
	if err != nil {
		return ""
	}
	
	// Extract query parameters
	params := parsedURL.Query()
	
	// Look for Greenhouse job ID in various formats
	var jobID string
	var boardName string
	
	// Check for gh_jid parameter (Greenhouse job ID)
	if ghJID := params.Get("gh_jid"); ghJID != "" {
		jobID = ghJID
	}
	
	// Check for board parameter
	if board := params.Get("board"); board != "" {
		boardName = board
	}
	
	// Also check if the first parameter is a job ID (like CoreWeave's format)
	for key := range params {
		if regexp.MustCompile(`^\d+$`).MatchString(key) {
			jobID = key
			break
		}
	}
	
	// If we don't have both job ID and board name, return empty
	if jobID == "" || boardName == "" {
		return ""
	}
	
	// Try to fetch from Greenhouse API
	apiURL := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs/%s", boardName, jobID)
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	
	// Parse the JSON response
	var jobData struct {
		Title       string `json:"title"`
		Company     string `json:"company_name"`
		Location    struct {
			Name string `json:"name"`
		} `json:"location"`
		Content string `json:"content"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&jobData); err != nil {
		return ""
	}
	
	// Clean the HTML content and extract just the job description
	cleanedContent := extractCoreJobDescription(jobData.Content)
	
	// Format the response
	result := fmt.Sprintf("Job Title: %s\nCompany: %s\nLocation: %s\n\n%s",
		jobData.Title,
		jobData.Company,
		jobData.Location.Name,
		cleanedContent)
	
	return result
}

// extractGreenhouseJobData fetches via Greenhouse API and returns structured fields
func extractGreenhouseJobData(jobURL string) (description, title, company, location string) {
    // Parse the URL
    parsedURL, err := url.Parse(jobURL)
    if err != nil {
        return "", "", "", ""
    }

    // Extract query parameters
    params := parsedURL.Query()

    // Look for Greenhouse job ID in various formats
    var jobID string
    var boardName string

    // Check for gh_jid parameter (Greenhouse job ID)
    if ghJID := params.Get("gh_jid"); ghJID != "" {
        jobID = ghJID
    }

    // Check for board parameter
    if board := params.Get("board"); board != "" {
        boardName = board
    }

    // Also check if the first parameter is a job ID (like CoreWeave's format)
    for key := range params {
        if regexp.MustCompile(`^\d+$`).MatchString(key) {
            jobID = key
            break
        }
    }

    // If we don't have both job ID and board name, return empty
    if jobID == "" || boardName == "" {
        return "", "", "", ""
    }

    // Try to fetch from Greenhouse API
    apiURL := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs/%s", boardName, jobID)

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Get(apiURL)
    if err != nil {
        return "", "", "", ""
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return "", "", "", ""
    }

    // Parse the JSON response
    var jobData struct {
        Title       string `json:"title"`
        Company     string `json:"company_name"`
        Location    struct {
            Name string `json:"name"`
        } `json:"location"`
        Content string `json:"content"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&jobData); err != nil {
        return "", "", "", ""
    }

    cleanedContent := extractCoreJobDescription(jobData.Content)
    return strings.TrimSpace(cleanedContent), jobData.Title, jobData.Company, jobData.Location.Name
}
