package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"resumeai/models"
	"resumeai/services"
)

// TailorResume handles POST /api/resume/tailor
// It loads the user's resume, polishes all sections based on the provided job description,
// generates a tailored PDF, uploads it to S3, and returns a presigned download URL.
func TailorResume(resumeModel *models.ResumeModel, resumeHistoryModel *models.ResumeHistoryModel) gin.HandlerFunc {
	return func(c *gin.Context) {
		if polishAgent == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Polish agent is not available"})
			return
		}

		var req struct {
			JobDescription string `json:"job_description" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "job_description is required"})
			return
		}

		jobDesc := strings.TrimSpace(req.JobDescription)
		if jobDesc == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "job_description cannot be empty"})
			return
		}

		// Get authenticated user (reuses extractUserID from resume.go)
		userIDInt, ok := extractUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		// Load resume from DB
		resume, err := resumeModel.GetByUserID(userIDInt)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "No resume found. Please create a resume first."})
			return
		}

		// Load structured experiences
		experiences, err := resumeModel.GetExperiencesByResumeID(resume.ID)
		if err != nil {
			fmt.Printf("[Tailor] Warning: failed to load experiences: %v\n", err)
			experiences = nil
		}

		// Build resumeData map for the polish agent
		resumeData := buildTailorResumeData(resume, experiences)

		// Polish all sections using the existing polishAllSections (from polish.go)
		ctx := c.Request.Context()
		updatedData := copyResumeData(resumeData)
		msg, err := polishAllSections(ctx, resumeData, jobDesc, updatedData)
		if err != nil {
			fmt.Printf("[Tailor] Polish failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to polish resume"})
			return
		}
		fmt.Printf("[Tailor] Polish result: %s\n", msg)

		// Build userData for the Python HTML template (reuses generateHTMLResumeWithPython from resume.go)
		templateSlug := normalizeTemplateFormat(resume.SelectedFormat)
		userData := buildTailorTemplateData(updatedData, resume)

		// Ensure output directory
		saveDir := "./static"
		if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create output directory"})
			return
		}

		timestamp := time.Now().UnixNano()

		// Step 1: Render HTML directly from polished data (no frontend needed)
		htmlFilename := fmt.Sprintf("tailored_%d.html", timestamp)
		htmlPath := filepath.Join(saveDir, htmlFilename)
		if err := renderTailorHTML(userData, htmlPath); err != nil {
			fmt.Printf("[Tailor] HTML generation failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate resume HTML"})
			return
		}

		// Step 2: Convert HTML to PDF (reuses generatePDFResumeWithPython)
		pdfFilename := fmt.Sprintf("tailored_%d.pdf", timestamp)
		pdfPath := filepath.Join(saveDir, pdfFilename)
		pdfData := map[string]interface{}{
			"htmlContent": "",
			"htmlPath":    htmlPath,
		}
		if err := generatePDFResumeWithPython(templateSlug, pdfData, pdfPath); err != nil {
			fmt.Printf("[Tailor] PDF generation failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate resume PDF"})
			return
		}

		// Step 3: Upload to S3 (reuses same pattern as resume_pdf_handler.go)
		s3svc, s3err := services.NewS3Service()
		if s3err != nil {
			fmt.Printf("[Tailor] S3 unavailable: %v\n", s3err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage service unavailable"})
			return
		}

		key := "resumes/" + pdfFilename
		if _, uploadErr := s3svc.UploadFile(pdfPath, key); uploadErr != nil {
			fmt.Printf("[Tailor] S3 upload failed: %v\n", uploadErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload resume"})
			return
		}

		// Step 4: Save to resume history (reuses same pattern as resume_pdf_handler.go)
		if resumeHistoryModel != nil {
			resumeName := fmt.Sprintf("Tailored Resume %s", time.Now().Format("2006-01-02 15:04"))
			if _, err := resumeHistoryModel.Create(userIDInt, resumeName, key); err != nil {
				fmt.Printf("[Tailor] Failed to save history: %v\n", err)
			}
			if err := resumeHistoryModel.CleanupOldResumes(userIDInt, 10); err != nil {
				fmt.Printf("[Tailor] Failed to cleanup old resumes: %v\n", err)
			}
		}

		// Step 5: Generate presigned download URL
		presignedURL, err := s3svc.GeneratePresignedURL(key)
		if err != nil {
			fmt.Printf("[Tailor] Presigned URL failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate download URL"})
			return
		}

		// Clean up temp files
		os.Remove(htmlPath)

		c.JSON(http.StatusOK, gin.H{
			"success":     true,
			"downloadUrl": presignedURL,
			"filename":    pdfFilename,
		})
	}
}

// buildTailorResumeData converts the DB resume + experiences into the map format
// expected by polishAllSections.
func buildTailorResumeData(resume *models.Resume, experiences []models.ExperienceRecord) map[string]interface{} {
	data := make(map[string]interface{})

	// Summary: json.RawMessage → string
	if len(resume.Summary) > 0 {
		var s string
		if err := json.Unmarshal(resume.Summary, &s); err != nil {
			s = strings.Trim(string(resume.Summary), "\"")
		}
		data["summary"] = s
	}

	// Skills: json.RawMessage → comma-separated string
	if len(resume.Skills) > 0 {
		var arr []string
		if err := json.Unmarshal(resume.Skills, &arr); err == nil {
			data["skills"] = strings.Join(arr, ", ")
		} else {
			var s string
			if err := json.Unmarshal(resume.Skills, &s); err == nil {
				data["skills"] = s
			} else {
				data["skills"] = strings.Trim(string(resume.Skills), "\"")
			}
		}
	}

	// Structured experiences → []interface{} with camelCase keys (polish agent format)
	if len(experiences) > 0 {
		expList := make([]interface{}, 0, len(experiences))
		for _, exp := range experiences {
			expList = append(expList, map[string]interface{}{
				"jobTitle":         exp.JobTitle,
				"company":          exp.Company,
				"city":             exp.City,
				"state":            exp.State,
				"startDate":        exp.StartDate,
				"endDate":          exp.EndDate,
				"currentlyWorking": exp.CurrentlyWorking,
				"description":      exp.Description,
			})
		}
		data["experiences"] = expList
	}

	if resume.Experience != "" {
		data["experience"] = resume.Experience
	}
	if resume.Education != "" {
		data["education"] = resume.Education
	}

	return data
}

// buildTailorTemplateData converts polished data into the format expected by
// generateHTMLResumeWithPython (same fields as ResumeRequest in resume.go).
func buildTailorTemplateData(polished map[string]interface{}, resume *models.Resume) map[string]interface{} {
	ud := map[string]interface{}{
		"name":  resume.Name,
		"email": resume.Email,
		"phone": resume.Phone,
	}

	// Summary
	if s, ok := polished["summary"].(string); ok && s != "" {
		ud["summary"] = s
	}

	// Skills → []string for template
	if s, ok := polished["skills"].(string); ok && s != "" {
		var cleaned []string
		for _, sk := range strings.Split(s, ",") {
			sk = strings.TrimSpace(sk)
			if sk != "" {
				cleaned = append(cleaned, sk)
			}
		}
		ud["skills"] = cleaned
	}

	// Experiences → formatted text string for template
	if exps, ok := polished["experiences"].([]interface{}); ok && len(exps) > 0 {
		ud["experience"] = tailorFormatExperiences(exps)
	} else if e, ok := polished["experience"].(string); ok && e != "" {
		ud["experience"] = e
	} else if resume.Experience != "" {
		ud["experience"] = resume.Experience
	}

	// Education
	if e, ok := polished["education"].(string); ok && e != "" {
		ud["education"] = e
	} else if resume.Education != "" {
		ud["education"] = resume.Education
	}

	if resume.Location != "" {
		ud["location"] = resume.Location
	}

	return ud
}

// tailorFormatExperiences converts polished experience maps into text for the HTML template.
func tailorFormatExperiences(experiences []interface{}) string {
	var parts []string
	for _, exp := range experiences {
		m, ok := exp.(map[string]interface{})
		if !ok {
			continue
		}
		title, _ := m["jobTitle"].(string)
		company, _ := m["company"].(string)
		start, _ := m["startDate"].(string)
		end, _ := m["endDate"].(string)
		desc, _ := m["description"].(string)

		header := title
		if company != "" {
			header += " at " + company
		}
		if start != "" {
			header += " | " + start
			if end != "" {
				header += " - " + end
			} else {
				header += " - Present"
			}
		}
		entry := header
		if desc != "" {
			entry += "\n" + desc
		}
		parts = append(parts, entry)
	}
	return strings.Join(parts, "\n\n")
}

// renderTailorHTML builds a complete HTML resume from the polished data and writes it to outputPath.
// This replaces the need for pre-rendered htmlContent from the frontend.
func renderTailorHTML(data map[string]interface{}, outputPath string) error {
	name, _ := data["name"].(string)
	email, _ := data["email"].(string)
	phone, _ := data["phone"].(string)
	location, _ := data["location"].(string)
	summary, _ := data["summary"].(string)
	experience, _ := data["experience"].(string)
	education, _ := data["education"].(string)

	// Build contact line
	var contactParts []string
	if email != "" {
		contactParts = append(contactParts, html.EscapeString(email))
	}
	if phone != "" {
		contactParts = append(contactParts, html.EscapeString(phone))
	}
	if location != "" {
		contactParts = append(contactParts, html.EscapeString(location))
	}
	contactLine := strings.Join(contactParts, " &bull; ")

	// Build skills HTML
	var skillsSection string
	if skills, ok := data["skills"].([]string); ok && len(skills) > 0 {
		var escaped []string
		for _, sk := range skills {
			escaped = append(escaped, html.EscapeString(sk))
		}
		skillsSection = fmt.Sprintf(`
    <div class="section">
        <div class="section-title">Skills</div>
        <div class="skills">%s</div>
    </div>`, strings.Join(escaped, " &bull; "))
	}

	// Build summary section
	var summarySection string
	if summary != "" {
		summarySection = fmt.Sprintf(`
    <div class="section">
        <div class="section-title">Summary</div>
        <p class="summary-text">%s</p>
    </div>`, html.EscapeString(summary))
	}

	// Build experience section
	var experienceSection string
	if experience != "" {
		entries := strings.Split(experience, "\n\n")
		var items string
		for _, entry := range entries {
			lines := strings.Split(strings.TrimSpace(entry), "\n")
			if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
				continue
			}
			items += fmt.Sprintf(`        <div class="experience-item">
            <div class="job-title">%s</div>`, html.EscapeString(lines[0]))
			for _, line := range lines[1:] {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				// Remove leading bullet characters
				line = strings.TrimLeft(line, "•-*· ")
				if line != "" {
					items += fmt.Sprintf(`
            <div class="achievement">&bull; %s</div>`, html.EscapeString(line))
				}
			}
			items += `
        </div>`
		}
		experienceSection = fmt.Sprintf(`
    <div class="section">
        <div class="section-title">Experience</div>
%s
    </div>`, items)
	}

	// Build education section
	var educationSection string
	if education != "" {
		educationSection = fmt.Sprintf(`
    <div class="section">
        <div class="section-title">Education</div>
        <div class="education-text">%s</div>
    </div>`, html.EscapeString(education))
	}

	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s - Resume</title>
    <style>
        @page { size: Letter; margin: 0; }
        body {
            font-family: Arial, Helvetica, sans-serif;
            line-height: 1.5;
            margin: 0;
            padding: 36px 48px;
            background-color: white;
            font-size: 11pt;
            color: #333;
        }
        .header {
            text-align: center;
            border-bottom: 2px solid #2c3e50;
            padding-bottom: 12px;
            margin-bottom: 20px;
        }
        .name {
            font-size: 22pt;
            font-weight: bold;
            color: #2c3e50;
            margin-bottom: 4px;
        }
        .contact {
            color: #555;
            font-size: 10pt;
        }
        .section {
            margin-bottom: 16px;
        }
        .section-title {
            font-size: 12pt;
            font-weight: bold;
            color: #2c3e50;
            border-bottom: 1px solid #2c3e50;
            padding-bottom: 3px;
            margin-bottom: 8px;
            text-transform: uppercase;
        }
        .summary-text {
            margin: 0;
            font-size: 10pt;
            line-height: 1.5;
        }
        .skills {
            font-size: 10pt;
            line-height: 1.6;
        }
        .experience-item {
            margin-bottom: 12px;
        }
        .job-title {
            font-size: 11pt;
            font-weight: bold;
            color: #333;
            margin-bottom: 4px;
        }
        .achievement {
            margin-left: 16px;
            margin-bottom: 2px;
            font-size: 10pt;
            line-height: 1.5;
        }
        .education-text {
            font-size: 10pt;
            line-height: 1.5;
            white-space: pre-line;
        }
    </style>
</head>
<body>
    <div class="header">
        <div class="name">%s</div>
        <div class="contact">%s</div>
    </div>
%s%s%s%s
</body>
</html>`,
		html.EscapeString(name),
		html.EscapeString(name),
		contactLine,
		summarySection,
		skillsSection,
		experienceSection,
		educationSection,
	)

	return os.WriteFile(outputPath, []byte(htmlContent), 0644)
}
