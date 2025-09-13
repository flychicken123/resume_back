package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run fetch_job_html.go <job_url>")
		fmt.Println("Example: go run fetch_job_html.go https://example.com/job-posting")
		return
	}

	jobURL := os.Args[1]
	
	fmt.Println("Fetching job page from:", jobURL)
	fmt.Println("=====================================")
	
	// Validate URL
	if !strings.HasPrefix(jobURL, "http://") && !strings.HasPrefix(jobURL, "https://") {
		fmt.Println("Error: URL must start with http:// or https://")
		return
	}

	// Fetch the page
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", jobURL, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	// Set user agent to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	fmt.Println("Sending request...")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error fetching URL: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Response Status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
	
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error: Server returned status code %d\n", resp.StatusCode)
		return
	}

	// Read the response (limit to 2MB for job pages)
	limitedReader := io.LimitReader(resp.Body, 2*1024*1024)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	htmlContent := string(bodyBytes)
	fmt.Printf("Fetched %d bytes of HTML\n", len(htmlContent))
	
	// Save raw HTML
	htmlFile := "job_page.html"
	if err := os.WriteFile(htmlFile, bodyBytes, 0644); err != nil {
		fmt.Printf("Error saving HTML: %v\n", err)
	} else {
		fmt.Printf("✅ HTML saved to: %s\n", htmlFile)
	}
	
	// Try to extract text content (remove HTML tags)
	// This is a very basic extraction
	textContent := extractText(htmlContent)
	
	// Save text version
	textFile := "job_page_text.txt"
	if err := os.WriteFile(textFile, []byte(textContent), 0644); err != nil {
		fmt.Printf("Error saving text: %v\n", err)
	} else {
		fmt.Printf("✅ Text content saved to: %s\n", textFile)
	}
	
	// Show first 1000 characters of text
	fmt.Println("\n📄 PREVIEW OF EXTRACTED TEXT:")
	fmt.Println("=====================================")
	if len(textContent) > 1000 {
		fmt.Println(textContent[:1000] + "...")
	} else {
		fmt.Println(textContent)
	}
	fmt.Println("=====================================")
	
	fmt.Println("\n✅ Complete! Check the saved files for full content.")
	fmt.Println("- job_page.html: Raw HTML content")
	fmt.Println("- job_page_text.txt: Extracted text content")
}

// Basic text extraction (removes HTML tags)
func extractText(html string) string {
	// Remove script and style content
	for {
		start := strings.Index(html, "<script")
		if start == -1 {
			break
		}
		end := strings.Index(html[start:], "</script>")
		if end == -1 {
			break
		}
		html = html[:start] + html[start+end+9:]
	}
	
	for {
		start := strings.Index(html, "<style")
		if start == -1 {
			break
		}
		end := strings.Index(html[start:], "</style>")
		if end == -1 {
			break
		}
		html = html[:start] + html[start+end+8:]
	}
	
	// Replace common tags with newlines
	replacements := map[string]string{
		"</p>": "\n\n",
		"</div>": "\n",
		"</li>": "\n",
		"<br>": "\n",
		"<br/>": "\n",
		"<br />": "\n",
		"</h1>": "\n\n",
		"</h2>": "\n\n",
		"</h3>": "\n\n",
		"</h4>": "\n",
		"</h5>": "\n",
		"</h6>": "\n",
	}
	
	for tag, replacement := range replacements {
		html = strings.ReplaceAll(html, tag, replacement)
	}
	
	// Remove all remaining HTML tags
	result := ""
	inTag := false
	for _, ch := range html {
		if ch == '<' {
			inTag = true
		} else if ch == '>' {
			inTag = false
		} else if !inTag {
			result += string(ch)
		}
	}
	
	// Clean up whitespace
	lines := strings.Split(result, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}
	
	// Decode common HTML entities
	text := strings.Join(cleanLines, "\n")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	
	return text
}