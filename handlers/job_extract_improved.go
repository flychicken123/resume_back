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

	// Handle Ashby job postings via embedded JSON
	if strings.Contains(req.URL, "jobs.ashbyhq.com") {
		if desc, title, company, err := extractAshbyJob(req.URL); err == nil && strings.TrimSpace(desc) != "" {
			c.JSON(http.StatusOK, JobExtractResponse{
				Description: desc,
				Title:       title,
				Company:     company,
			})
			return
		}
	}

	// Check if it's a LinkedIn URL that requires login
	if strings.Contains(req.URL, "linkedin.com") && !strings.Contains(req.URL, "/public/") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "LinkedIn jobs require authentication. Please copy and paste the job description directly instead of using the URL.",
			"suggestion": "Copy the job description from LinkedIn and paste it in the job description field.",
		})
		return
	}

	// Check for other sites that require authentication
	protectedSites := []string{"glassdoor.com", "angel.co", "wellfound.com"}
	for _, site := range protectedSites {
		if strings.Contains(req.URL, site) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      "This website requires authentication. Please copy and paste the job description directly.",
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
				"error":      "This job posting requires authentication. Please copy and paste the job description directly.",
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
			"error":      "This job posting requires authentication. Please copy and paste the job description directly.",
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
			"error":      "This job posting requires authentication. Please copy and paste the job description directly.",
			"suggestion": "Copy the job description from the website and paste it in the job description field.",
		})
		return
	}

	// Extract job description using improved patterns
	description := extractJobDescriptionImproved(htmlContent, req.URL)

	if description == "" || len(description) < 100 {
		// If we couldn't extract a meaningful job description
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Could not extract job description from this URL. The page might require login or use dynamic content loading.",
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

	// Require at least one strong password-related hint before we treat a page
	// as a login wall. This avoids misclassifying public pages that simply have
	// a "Sign in" link in the header (like many career sites).
	passwordHints := []string{
		"password",
		"forgot password",
		"remember me",
	}
	hasPasswordHint := false
	for _, hint := range passwordHints {
		if strings.Contains(htmlLower, hint) {
			hasPasswordHint = true
			break
		}
	}
	if !hasPasswordHint {
		return false
	}

	matchCount := 0
	for _, indicator := range loginIndicators {
		if strings.Contains(htmlLower, indicator) {
			matchCount++
		}
	}

	// If we find 3 or more login-related terms (including a password hint),
	// it's likely a real login page rather than a public job posting.
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

	// Clean the full HTML to text first
	fullText := cleanHTMLToText(htmlContent)

	// Find where the job description actually starts (skip company intro)
	startPhrases := []string{
		"About this Role:",
		"About the Role:",
		"Role Overview:",
		"Position Overview:",
		"Job Description:",
		"The Role:",
		"We are seeking",
		"We are looking for",
		"We're looking for",
		"We are hiring",
	}

	startIndex := 0
	for _, phrase := range startPhrases {
		if idx := strings.Index(fullText, phrase); idx >= 0 && (startIndex == 0 || idx < startIndex) {
			startIndex = idx
		}
	}

	// If we found a start point, use it
	if startIndex > 0 {
		fullText = fullText[startIndex:]
	}

	// Find where the salary/compensation/benefits section starts and cut there
	stopPhrases := []string{
		"The base salary range",
		"base salary range for this role",
		"salary range",
		"What We Offer",
		"Benefits",
		"Compensation",
		"Our Workplace",
		"Equal Opportunity",
		"Export Control",
		"is an equal opportunity",
		"As part of this commitment",
	}

	minIndex := len(fullText)
	for _, phrase := range stopPhrases {
		if idx := strings.Index(fullText, phrase); idx > 0 && idx < minIndex {
			minIndex = idx
		}
	}

	// Cut the text at the stop point
	if minIndex < len(fullText) {
		fullText = fullText[:minIndex]
	}

	// Also remove any trailing incomplete sentences
	fullText = strings.TrimSpace(fullText)

	// Try to extract structured sections
	var result strings.Builder
	lines := strings.Split(fullText, "\n")
	inRelevantSection := false
	relevantHeaders := regexp.MustCompile(`(?i)^(about this role|about the role|role overview|position overview|what you.*do|responsibilities|requirements|qualifications|who you are|skills|experience)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if this is a relevant header
		if relevantHeaders.MatchString(line) {
			inRelevantSection = true
			if result.Len() > 0 {
				result.WriteString("\n\n")
			}
			result.WriteString(line)
			if !strings.HasSuffix(line, ":") {
				result.WriteString(":")
			}
			result.WriteString("\n")
		} else if inRelevantSection || result.Len() == 0 {
			// Include content if we're in a relevant section or haven't found sections yet
			if result.Len() > 0 && !strings.HasSuffix(result.String(), "\n") {
				result.WriteString(" ")
			}
			result.WriteString(line)
		}
	}

	finalResult := result.String()
	if finalResult == "" {
		finalResult = fullText
	}

	return strings.TrimSpace(finalResult)
}

func extractAshbyJob(jobURL string) (description, title, company string, err error) {
	req, err := http.NewRequest(http.MethodGet, jobURL, nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; HiHiredBot/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", "", "", fmt.Errorf("ashby page returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("read ashby page: %w", err)
	}

	re := regexp.MustCompile(`(?s)window\.__appData\s*=\s*({.*?});`)
	matches := re.FindSubmatch(body)
	if len(matches) < 2 {
		return "", "", "", fmt.Errorf("ashby metadata not found")
	}

	var payload struct {
		Organization *struct {
			Name string `json:"name"`
		} `json:"organization"`
		Posting *struct {
			Title           string `json:"title"`
			DescriptionHTML string `json:"descriptionHtml"`
			Sections        []struct {
				Title    string `json:"title"`
				BodyHTML string `json:"bodyHtml"`
				BodyText string `json:"bodyText"`
			} `json:"sections"`
			HiringTeam *struct {
				OrganizationName string `json:"organizationName"`
			} `json:"hiringTeam"`
		} `json:"posting"`
	}

	if err := json.Unmarshal(matches[1], &payload); err != nil {
		return "", "", "", fmt.Errorf("parse ashby metadata: %w", err)
	}
	if payload.Posting == nil {
		return "", "", "", fmt.Errorf("ashby posting data missing")
	}

	desc := cleanHTMLToText(payload.Posting.DescriptionHTML)
	if strings.TrimSpace(desc) == "" {
		desc = buildAshbyDescription(payload.Posting.Sections)
	}
	if strings.TrimSpace(desc) == "" {
		desc = payload.Posting.Title
	}

	company = ""
	if payload.Organization != nil && strings.TrimSpace(payload.Organization.Name) != "" {
		company = strings.TrimSpace(payload.Organization.Name)
	} else if payload.Posting.HiringTeam != nil && strings.TrimSpace(payload.Posting.HiringTeam.OrganizationName) != "" {
		company = strings.TrimSpace(payload.Posting.HiringTeam.OrganizationName)
	} else {
		company = humanizeSlug(ashbySlugFromURL(jobURL))
	}

	return strings.TrimSpace(desc), strings.TrimSpace(payload.Posting.Title), company, nil
}

func buildAshbyDescription(sections []struct {
	Title    string `json:"title"`
	BodyHTML string `json:"bodyHtml"`
	BodyText string `json:"bodyText"`
}) string {
	var builder strings.Builder
	for _, section := range sections {
		content := cleanHTMLToText(section.BodyHTML)
		if strings.TrimSpace(content) == "" && strings.TrimSpace(section.BodyText) != "" {
			content = section.BodyText
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		if strings.TrimSpace(section.Title) != "" {
			builder.WriteString(strings.TrimSpace(section.Title))
			builder.WriteString("\n")
		}
		builder.WriteString(strings.TrimSpace(content))
	}
	return builder.String()
}

func ashbySlugFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	segments := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })
	if len(segments) == 0 {
		return ""
	}
	return segments[0]
}

func humanizeSlug(slug string) string {
	if slug == "" {
		return ""
	}
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_' || r == ' ' || r == '.'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		parts[i] = strings.ToUpper(lower[:1]) + lower[1:]
	}
	return strings.Join(parts, " ")
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

	// If we don't yet have both job ID and board name, try to infer them from common Greenhouse path formats
	if jobID == "" || boardName == "" {
		segments := strings.FieldsFunc(parsedURL.Path, func(r rune) bool { return r == '/' })
		// Handle job-boards.greenhouse.io/<board>/jobs/<id>
		if len(segments) >= 3 && segments[1] == "jobs" {
			if boardName == "" {
				boardName = segments[0]
			}
			if jobID == "" {
				jobID = segments[2]
			}
		} else if len(segments) >= 4 && segments[0] == "boards" && segments[2] == "jobs" {
			// Handle boards.greenhouse.io/boards/<board>/jobs/<id> style URLs
			if boardName == "" {
				boardName = segments[1]
			}
			if jobID == "" {
				jobID = segments[3]
			}
		}
	}

	// If we still don't have both job ID and board name, return empty
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
		Title    string `json:"title"`
		Company  string `json:"company_name"`
		Location struct {
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
