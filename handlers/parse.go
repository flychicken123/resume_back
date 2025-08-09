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
	file, header, err := c.Request.FormFile("resume")
	if err != nil {
		c.JSON(400, gin.H{"error": "Could not get file"})
		return
	}
	defer file.Close()

	// Save file to temp location
	tempDir := os.TempDir()
	tempFile := filepath.Join(tempDir, header.Filename)
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

	// Deterministic extraction via python3 parse_resume.py
	cmd := exec.Command("python3", "parse_resume.py", tempFile)
	cmd.Dir = "." // script resides in backend root
	output, err := cmd.CombinedOutput()
	if err != nil {
		c.JSON(500, gin.H{"error": "Text extraction failed", "details": err.Error(), "stdout": string(output)})
		return
	}

	var extracted map[string]interface{}
	if err := json.Unmarshal(output, &extracted); err != nil {
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
		c.JSON(500, gin.H{"error": err.Error()})
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
