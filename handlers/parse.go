package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"resumeai/parsers"
	"resumeai/services"

	"github.com/gin-gonic/gin"
)

func sanitizeAIJSON(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}

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
		if strings.TrimSpace(rawText) == "" {
			c.JSON(422, gin.H{
				"error":   "Could not extract text from this resume file. It may be image-based, scanned, encrypted, or use an unsupported PDF encoding.",
				"go_error": fmt.Sprint(extractErr),
			})
			return
		}
	} else {
		fmt.Printf("[parse] Go extraction successful, extracted %d characters\n", len(rawText))
		// Create extracted structure for compatibility
		extracted = map[string]interface{}{
			"raw_text": rawText,
			"method":   "go_parser",
		}
	}
	// Skip the complex Go parser and go straight to AI for better accuracy
	fmt.Printf("[parse] Using AI for resume extraction...\n")

	// Extract basic contact info if available
	email := ""
	phone := ""

	// Try to extract email and phone with simple regex
	emailRegex := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	if match := emailRegex.FindString(rawText); match != "" {
		email = match
	}

	phoneRegex := regexp.MustCompile(`\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}`)
	if match := phoneRegex.FindString(rawText); match != "" {
		phone = match
	}
	prompt := fmt.Sprintf(`Read this resume and return ONE valid JSON object only.
Do not use markdown.
Do not wrap in code fences.
Do not add commentary.
Do not leave trailing commas.
If unsure, use null.

Return exactly this JSON schema:
{
  "name": "",
  "email": "",
  "phone": "",
  "summary": null,
  "experience": [
    {
      "company": "",
      "role": "",
      "location": "",
      "startDate": "",
      "endDate": "",
      "bullets": ["", ""]
    }
  ],
  "education": [
    {
      "school": "",
      "degree": "",
      "field": "",
      "startDate": "",
      "endDate": ""
    }
  ],
  "projects": [],
  "skills": [""],
  "conversions": []
}

Hard rules:
- Keep at most 4 experience entries, most recent first.
- Keep at most 2 bullets per experience.
- Each bullet must be concise.
- Keep at most 6 skills.
- If there is no explicit summary, use null.
- If there is no explicit projects section, use [].
- Use these exact keys only.
- Output must be complete valid JSON.

Resume text:
%s

Known contact info:
Email: %s
Phone: %s`, rawText, email, phone)

	fmt.Printf("[parse] BUILD=2026-02-14-v2 Calling Gemini AI with temperature=0...\n")
	aiResp, err := services.CallGeminiWithTemperature(prompt, 0.0)
	if err != nil {
		fmt.Printf("[parse] AI extraction failed: %v\n", err)
		fmt.Printf("[parse] Using simple fallback parser instead...\n")

		// Simple fallback parser - better than nothing
		structured := parseResumeSimple(rawText, email, phone)

		// Clean up temp file
		_ = os.Remove(tempFile)

		c.JSON(200, gin.H{
			"structured": structured,
			"extracted":  extracted,
			"method":     "simple_fallback",
			"aiError":    err.Error(),
		})
		return
	}

	fmt.Printf("[parse] AI extraction successful, response length: %d\n", len(aiResp))

	cleaned := sanitizeAIJSON(aiResp)

	var structured map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &structured); err != nil {
		fmt.Printf("[parse] Failed to parse AI JSON response: %v\n", err)
		fmt.Printf("[parse] Raw AI response: %s\n", aiResp)
		fmt.Printf("[parse] Falling back to simple parser after invalid/truncated AI JSON\n")

		// Clean up temp file
		_ = os.Remove(tempFile)

		c.JSON(500, gin.H{"error": "AI output was not valid JSON", "raw": aiResp})
		return
	}

	fmt.Printf("[parse] Successfully parsed AI response into structured data\n")
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

// parseResumeSimple is a simple fallback parser when AI is not available
func parseResumeSimple(text string, email string, phone string) map[string]interface{} {
	lines := strings.Split(text, "\n")

	// Extract name (usually first non-empty line)
	name := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(line, "@") && !strings.Contains(line, "Resume") {
			// Likely the name
			name = line
			break
		}
	}

	// Initialize result structure
	result := map[string]interface{}{
		"name":       name,
		"email":      email,
		"phone":      phone,
		"summary":    nil,
		"experience": []map[string]interface{}{},
		"education":  []map[string]interface{}{},
		"projects":   []map[string]interface{}{},
		"skills":     []string{},
	}

	// Simple section detection
	currentSection := ""
	var currentExperience map[string]interface{}
	var currentEducation map[string]interface{}
	var currentProject map[string]interface{}
	var bullets []string

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		lineLower := strings.ToLower(line)

		// Detect sections
		if strings.Contains(lineLower, "experience") || strings.Contains(lineLower, "employment") {
			currentSection = "experience"
			continue
		} else if strings.Contains(lineLower, "education") {
			currentSection = "education"
			continue
		} else if strings.Contains(lineLower, "project") && !strings.Contains(lineLower, "projecturl") {
			currentSection = "projects"
			continue
		} else if strings.Contains(lineLower, "skill") {
			currentSection = "skills"
			continue
		} else if strings.Contains(lineLower, "summary") || strings.Contains(lineLower, "objective") {
			currentSection = "summary"
			continue
		}

		// Process content based on current section
		switch currentSection {
		case "experience":
			// Look for company names (often have Inc, LLC, etc)
			if strings.Contains(line, "Inc") || strings.Contains(line, "LLC") ||
				strings.Contains(line, "Corp") || strings.Contains(line, "Company") ||
				(len(line) < 50 && i < len(lines)-1 && !strings.HasPrefix(line, "•")) {
				// Save previous experience if exists
				if currentExperience != nil {
					if len(bullets) > 0 {
						currentExperience["bullets"] = bullets
					}
					result["experience"] = append(result["experience"].([]map[string]interface{}), currentExperience)
					bullets = []string{}
				}

				// Start new experience
				currentExperience = map[string]interface{}{
					"company":   line,
					"role":      nil,
					"location":  nil,
					"startDate": nil,
					"endDate":   nil,
				}
			} else if strings.HasPrefix(line, "•") || strings.HasPrefix(line, "-") {
				// Bullet point
				bullet := strings.TrimPrefix(strings.TrimPrefix(line, "•"), "-")
				bullet = strings.TrimSpace(bullet)
				bullets = append(bullets, bullet)
			} else if currentExperience != nil && currentExperience["role"] == nil {
				// Likely the job title
				currentExperience["role"] = line
			}

		case "education":
			// Look for university/college
			if strings.Contains(lineLower, "university") || strings.Contains(lineLower, "college") ||
				strings.Contains(lineLower, "institute") || strings.Contains(lineLower, "school") {
				// Save previous education if exists
				if currentEducation != nil {
					result["education"] = append(result["education"].([]map[string]interface{}), currentEducation)
				}

				// Start new education
				currentEducation = map[string]interface{}{
					"school":    line,
					"degree":    nil,
					"field":     nil,
					"startDate": nil,
					"endDate":   nil,
				}
			} else if currentEducation != nil && currentEducation["degree"] == nil {
				// Likely the degree
				if strings.Contains(lineLower, "bachelor") || strings.Contains(lineLower, "master") ||
					strings.Contains(lineLower, "b.s") || strings.Contains(lineLower, "m.s") ||
					strings.Contains(lineLower, "ph.d") || strings.Contains(lineLower, "degree") {
					currentEducation["degree"] = line
				}
			}

		case "skills":
			// Add to skills - split by comma if present
			if strings.Contains(line, ",") {
				skills := strings.Split(line, ",")
				for _, skill := range skills {
					skill = strings.TrimSpace(skill)
					if skill != "" && !strings.HasPrefix(skill, "•") {
						result["skills"] = append(result["skills"].([]string), skill)
					}
				}
			} else if !strings.HasPrefix(line, "•") {
				result["skills"] = append(result["skills"].([]string), line)
			}

		case "summary":
			// Add to summary
			if result["summary"] == nil {
				result["summary"] = line
			} else {
				result["summary"] = result["summary"].(string) + " " + line
			}

		case "projects":
			// Look for project names (often capitalized or followed by a colon)
			if !strings.HasPrefix(line, "•") && !strings.HasPrefix(line, "-") &&
				(strings.Contains(line, ":") || (len(line) < 60 && i < len(lines)-1)) {
				// Save previous project if exists
				if currentProject != nil {
					if len(bullets) > 0 {
						currentProject["bullets"] = bullets
					}
					result["projects"] = append(result["projects"].([]map[string]interface{}), currentProject)
					bullets = []string{}
				}

				// Start new project
				projectName := line
				if idx := strings.Index(line, ":"); idx > 0 {
					projectName = strings.TrimSpace(line[:idx])
				}

				currentProject = map[string]interface{}{
					"projectName":  projectName,
					"description":  nil,
					"technologies": nil,
					"projectUrl":   nil,
				}
			} else if strings.HasPrefix(line, "•") || strings.HasPrefix(line, "-") {
				// Bullet point for project
				bullet := strings.TrimPrefix(strings.TrimPrefix(line, "•"), "-")
				bullet = strings.TrimSpace(bullet)
				bullets = append(bullets, bullet)
			} else if currentProject != nil && currentProject["description"] == nil {
				// Likely the project description
				currentProject["description"] = line
			}
		}
	}

	// Save last experience if exists
	if currentExperience != nil {
		if len(bullets) > 0 {
			currentExperience["bullets"] = bullets
		}
		result["experience"] = append(result["experience"].([]map[string]interface{}), currentExperience)
	}

	// Save last education if exists
	if currentEducation != nil {
		result["education"] = append(result["education"].([]map[string]interface{}), currentEducation)
	}

	// Save last project if exists
	if currentProject != nil {
		if len(bullets) > 0 {
			currentProject["bullets"] = bullets
		}
		result["projects"] = append(result["projects"].([]map[string]interface{}), currentProject)
	}

	return result
}
