package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
				"error":    "File too large. Please ensure your resume file is under 32MB.",
				"max_size": "32MB",
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

	// Ensure script exists
	if _, statErr := os.Stat("parse_resume.py"); statErr != nil {
		fmt.Printf("[parse] missing script parse_resume.py: %v\n", statErr)
		c.JSON(500, gin.H{"error": "Python script failed", "details": "parse_resume.py not found"})
		return
	}

	// Determine python executable (python3 preferred, fallback to python)
	pythonExec := "python3"
	if _, lookErr := exec.LookPath(pythonExec); lookErr != nil {
		pythonExec = "python"
	}

	// Deterministic extraction via python parse_resume.py
	cmd := exec.Command(pythonExec, "parse_resume.py", tempFile)
	cmd.Dir = "." // script resides in backend root
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("[parse] python exec error: %v\n", err)
		fmt.Printf("[parse] output: %s\n", string(output))
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "Python script failed", "details": err.Error(), "stdout": string(output)})
		return
	}

	var extracted map[string]interface{}
	if err := json.Unmarshal(output, &extracted); err != nil {
		fmt.Printf("[parse] invalid extractor JSON: %v\nraw: %s\n", err, string(output))
		c.JSON(500, gin.H{"error": "Failed to parse extractor output"})
		return
	}

	rawText, _ := extracted["raw_text"].(string)
	email, _ := extracted["email"].(string)
	phone, _ := extracted["phone"].(string)

	// Build strict schema prompt
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
	})
}
