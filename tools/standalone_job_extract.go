// +build ignore

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"resumeai/services"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run standalone_job_extract.go <job_url>")
		fmt.Println("Example: go run standalone_job_extract.go https://example.com/job-posting")
		return
	}

	jobURL := os.Args[1]
	
	fmt.Println("Extracting job description from:", jobURL)
	fmt.Println("=====================================")
	
	// Fetch and process the URL directly
	description, title, company, err := extractJobFromURL(jobURL)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	// Display results
	if title != "" {
		fmt.Printf("\n📌 JOB TITLE: %s\n", title)
	}
	
	if company != "" {
		fmt.Printf("🏢 COMPANY: %s\n", company)
	}
	
	fmt.Println("\n📄 JOB DESCRIPTION:")
	fmt.Println("=====================================")
	fmt.Println(description)
	fmt.Println("=====================================")
	
	// Save to file
	fileName := "extracted_job.txt"
	content := ""
	if title != "" {
		content += fmt.Sprintf("Job Title: %s\n", title)
	}
	if company != "" {
		content += fmt.Sprintf("Company: %s\n\n", company)
	}
	content += description
	
	if err := os.WriteFile(fileName, []byte(content), 0644); err != nil {
		fmt.Printf("\nError saving to file: %v\n", err)
	} else {
		fmt.Printf("\n✅ Saved to: %s\n", fileName)
	}
}

func extractJobFromURL(url string) (description, title, company string, err error) {
	// Validate URL
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", "", "", fmt.Errorf("invalid URL: must start with http:// or https://")
	}

	// Fetch the page
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create request: %v", err)
	}

	// Set user agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("failed to fetch URL: status code %d", resp.StatusCode)
	}

	// Read the response (limit to 1MB)
	limitedReader := io.LimitReader(resp.Body, 1024*1024)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read response: %v", err)
	}

	htmlContent := string(bodyBytes)
	
	// Use AI to extract job information
	prompt := `Extract the job description from this HTML page. Focus on:
1. Job title
2. Company name  
3. Full job description including responsibilities, requirements, and qualifications
4. Any technical skills or technologies mentioned

Return the extracted information as plain text in this format:
Job Title: [title if found]
Company: [company if found]

[Full job description text]

If this is not a job posting page, return "NOT_A_JOB_POSTING"`

	fullPrompt := fmt.Sprintf(`Given this HTML content from a web page, %s

HTML Content:
%s`, prompt, htmlContent)

	// Call Gemini to process
	result, err := services.CallGeminiWithAPIKey(fullPrompt)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to process with AI: %v", err)
	}

	// Check if it's a job posting
	if strings.Contains(result, "NOT_A_JOB_POSTING") {
		return "", "", "", fmt.Errorf("the URL does not appear to be a job posting")
	}

	// Parse the result
	lines := strings.Split(result, "\n")
	var descriptionStart int

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Job Title:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "Job Title:"))
		} else if strings.HasPrefix(line, "Company:") {
			company = strings.TrimSpace(strings.TrimPrefix(line, "Company:"))
		} else if line != "" && i > 1 {
			descriptionStart = i
			break
		}
	}

	// Get the full description
	if descriptionStart > 0 {
		descriptionLines := lines[descriptionStart:]
		description = strings.TrimSpace(strings.Join(descriptionLines, "\n"))
	} else {
		description = strings.TrimSpace(result)
	}

	if description == "" {
		return "", "", "", fmt.Errorf("could not extract job description")
	}

	return description, title, company, nil
}