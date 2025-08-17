package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"resumeai/parsers"
	"resumeai/services"

	"github.com/gin-gonic/gin"
)

type ParsedResume struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	Education  string `json:"education"`
	Experience string `json:"experience"`
	Skills     string `json:"skills"`
}

func ParseResume(c *gin.Context) {
	// Log request details for debugging
	fmt.Printf("[ParseResume] Request from: %s\n", c.Request.RemoteAddr)
	fmt.Printf("[ParseResume] Origin: %s\n", c.GetHeader("Origin"))
	fmt.Printf("[ParseResume] Content-Type: %s\n", c.GetHeader("Content-Type"))
	fmt.Printf("[ParseResume] Content-Length: %s\n", c.GetHeader("Content-Length"))
	fmt.Printf("[ParseResume] Method: %s\n", c.Request.Method)

	// Check if this is a preflight request
	if c.Request.Method == "OPTIONS" {
		fmt.Printf("[ParseResume] Handling OPTIONS preflight request\n")
		c.Status(200)
		return
	}

	// Check content type
	contentType := c.GetHeader("Content-Type")
	if !strings.Contains(contentType, "multipart/form-data") {
		c.JSON(400, gin.H{"error": "Expected multipart/form-data content type"})
		return
	}

	file, header, err := c.Request.FormFile("resume")
	if err != nil {
		// Check if it's a size-related error
		if strings.Contains(err.Error(), "too large") || strings.Contains(err.Error(), "413") {
			fmt.Printf("[ParseResume] File too large error: %v\n", err)
			c.JSON(413, gin.H{
				"error":    "File too large. Please ensure your resume file is under 8MB.",
				"max_size": "8MB",
				"details":  err.Error(),
			})
			return
		}
		fmt.Printf("[ParseResume] Error getting file: %v\n", err)
		c.JSON(400, gin.H{"error": "Could not get file", "details": err.Error()})
		return
	}
	defer file.Close()

	// Log file details
	fmt.Printf("[ParseResume] File received: %s, size: %d bytes\n", header.Filename, header.Size)

	// Save file to a safe temp location with preserved extension
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".bin"
	}
	tmp, err := os.CreateTemp("", "resume-*")
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to create temp file"})
		return
	}
	tempPathNoExt := tmp.Name()
	tmp.Close()
	tempFile := tempPathNoExt + ext
	if err := os.Rename(tempPathNoExt, tempFile); err != nil {
		c.JSON(500, gin.H{"error": "Failed to prepare temp file"})
		return
	}
	out, err := os.Create(tempFile)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to save file"})
		return
	}
	defer out.Close()
	if _, err = io.Copy(out, file); err != nil {
		c.JSON(500, gin.H{"error": "Failed to save file"})
		return
	}

	// Method 1: Use our Go-based parser (primary method)
	fmt.Printf("[parse] Attempting Go-based extraction...\n")
	
	extractor := parsers.NewPDFExtractor()
	rawText, extractErr := extractor.ExtractFromFile(tempFile)
	
	var extracted map[string]interface{}
	
	if extractErr != nil || strings.TrimSpace(rawText) == "" {
		fmt.Printf("[parse] Go extraction failed: %v, falling back to Python...\n", extractErr)
		
		// Method 2: Fallback to Python script
		extracted = fallbackToPython(tempFile)
		if extracted == nil {
			c.JSON(500, gin.H{"error": "All extraction methods failed", "go_error": extractErr})
			return
		}
		rawText, _ = extracted["raw_text"].(string)
	} else {
		fmt.Printf("[parse] Go extraction successful, extracted %d characters\n", len(rawText))
		// Create extracted structure for compatibility
		extracted = map[string]interface{}{
			"raw_text": rawText,
			"method":   "go_parser",
		}
	}
	// Method 3: Use our Go parser for structured extraction (primary method)
	fmt.Printf("[parse] Attempting Go-based structured parsing...\n")
	
	goParser := parsers.NewResumeParser()
	structuredData, parseErr := goParser.Parse(rawText)
	
	if parseErr == nil && structuredData != nil {
		fmt.Printf("[parse] Go parsing successful!\n")
		// Clean up temp file
		_ = os.Remove(tempFile)
		
		c.JSON(200, gin.H{
			"structured": structuredData,
			"extracted":  extracted,
			"method":     "go_primary",
		})
		return
	}
	
	fmt.Printf("[parse] Go parsing failed: %v, falling back to AI...\n", parseErr)
	
	// Extract basic contact info for AI fallback
	email, _ := extracted["email"].(string)
	phone, _ := extracted["phone"].(string)

	// Method 4: Fallback to AI parsing
	fmt.Printf("[parse] Using AI as final fallback...\n")
	schema := `{
      "name": string | null,
      "email": string | null,
      "phone": string | null,
      "summary": string | null,
      "experience": [
        {"company": string | null, "role": string | null, "location": string | null, "startDate": string | null, "endDate": string | null, "bullets": string[] | null}
      ],
      "education": [
        {"school": string | null, "degree": string | null, "field": string | null, "startDate": string | null, "endDate": string | null}
      ],
      "skills": string[]
    }`

	prompt := fmt.Sprintf(`Extract resume information from the following text and return ONLY strict JSON matching this schema (no markdown, no extra text). Do not invent data. Unknown fields must be null.

Schema:
%s

Text:
%s

Hints:
- If a value is already known, keep it. Known email: %s; known phone: %s.
- Dates can be in formats like "Nov 2022 - Present".`, schema, rawText, email, phone)

	aiResp, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		// Graceful fallback: build a minimal structured object from deterministic extraction
		sections, _ := extracted["sections"].(map[string]interface{})
		getSection := func(key string) string {
			if sections == nil {
				return ""
			}
			lowerKey := strings.ToLower(key)
			for k, v := range sections {
				if strings.Contains(strings.ToLower(k), lowerKey) {
					if s, ok := v.(string); ok {
						return s
					}
				}
			}
			return ""
		}

		summaryText := getSection("summary")
		experienceText := getSection("experience")
		educationText := getSection("education")
		skillsText := getSection("skill")

		// Build skills array by splitting commas/newlines/semicolons
		var skillsArr []string
		if skillsText != "" {
			fields := strings.FieldsFunc(skillsText, func(r rune) bool { return r == ',' || r == '\n' || r == ';' })
			for _, f := range fields {
				trimmed := strings.TrimSpace(f)
				if trimmed != "" {
					skillsArr = append(skillsArr, trimmed)
				}
			}
		}

		// Build a single experience entry with lines as bullets
		var expArr []map[string]interface{}
		if experienceText != "" {
			var bullets []string
			for _, line := range strings.Split(experienceText, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					bullets = append(bullets, line)
				}
			}
			expArr = append(expArr, map[string]interface{}{
				"company":   nil,
				"role":      nil,
				"location":  nil,
				"startDate": nil,
				"endDate":   nil,
				"bullets":   bullets,
			})
		}

		// Build minimal education array
		var eduArr []map[string]interface{}
		if educationText != "" {
			eduArr = append(eduArr, map[string]interface{}{
				"school":    nil,
				"degree":    nil,
				"field":     nil,
				"startDate": nil,
				"endDate":   nil,
			})
		}

		structured := map[string]interface{}{
			"name":       nil,
			"email":      email,
			"phone":      phone,
			"summary":    summaryText,
			"experience": expArr,
			"education":  eduArr,
			"skills":     skillsArr,
		}

		c.JSON(200, gin.H{
			"structured": structured,
			"extracted":  extracted,
			"aiError":    err.Error(),
		})
		return
	}

	cleaned := strings.TrimSpace(aiResp)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var structured map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &structured); err != nil {
		c.JSON(500, gin.H{"error": "AI output was not valid JSON", "raw": aiResp})
		return
	}

	// Clean up temp file
	_ = os.Remove(tempFile)

	c.JSON(200, gin.H{
		"structured": structured,
		"extracted":  extracted,
		"method":     "ai_fallback",
	})
}

// fallbackToPython attempts to extract text using the Python script
func fallbackToPython(tempFile string) map[string]interface{} {
	// Check if Python script exists
	if _, statErr := os.Stat("parse_resume.py"); statErr != nil {
		fmt.Printf("[parse] Python script not found: %v\n", statErr)
		return nil
	}

	// Determine python executable
	pythonExec := "python3"
	if _, lookErr := exec.LookPath(pythonExec); lookErr != nil {
		pythonExec = "python"
		if _, lookErr := exec.LookPath(pythonExec); lookErr != nil {
			fmt.Printf("[parse] Python not available\n")
			return nil
		}
	}

	// Run Python script
	cmd := exec.Command(pythonExec, "parse_resume.py", tempFile)
	cmd.Dir = "."

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("[parse] Python execution failed: %v\n", err)
		fmt.Printf("[parse] stderr: %s\n", stderr.String())
		return nil
	}

	// Parse JSON output
	var extracted map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &extracted); err != nil {
		fmt.Printf("[parse] Failed to parse Python output: %v\n", err)
		return nil
	}

	fmt.Printf("[parse] Python extraction successful\n")
	return extracted
}
