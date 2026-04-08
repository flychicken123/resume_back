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
// It loads the user's resume, tailors experiences based on the provided job description,
// generates a PDF using the user's template, uploads to S3, and returns a presigned download URL.
func TailorResume(resumeModel *models.ResumeModel, resumeHistoryModel *models.ResumeHistoryModel, projectModel *models.ProjectModel) gin.HandlerFunc {
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

		// Load projects
		var projects []*models.Project
		if projectModel != nil {
			projects, err = projectModel.GetByResumeID(resume.ID)
			if err != nil {
				fmt.Printf("[Tailor] Warning: failed to load projects: %v\n", err)
				projects = nil
			}
		}

		// Build resumeData map for the polish agent
		resumeData := buildTailorResumeData(resume, experiences)

		// Only tailor experiences based on job description — keep everything else as-is
		ctx := c.Request.Context()
		updatedData := copyResumeData(resumeData)

		if exps, ok := resumeData["experiences"].([]interface{}); ok && len(exps) > 0 {
			polishedExps := make([]interface{}, 0, len(exps))
			for _, exp := range exps {
				if expMap, ok := exp.(map[string]interface{}); ok {
					polished, err := polishAgent.PolishExperience(ctx, expMap, jobDesc, "")
					if err != nil {
						fmt.Printf("[Tailor] Failed to polish experience: %v\n", err)
						polishedExps = append(polishedExps, exp)
					} else {
						polishedExps = append(polishedExps, polished)
					}
				} else {
					polishedExps = append(polishedExps, exp)
				}
			}
			updatedData["experiences"] = polishedExps
			fmt.Printf("[Tailor] Tailored %d experience(s) for job description\n", len(polishedExps))
		} else {
			fmt.Println("[Tailor] No structured experiences found to tailor")
		}

		// Build userData for the Python HTML template
		templateSlug := normalizeTemplateFormat(resume.SelectedFormat)
		userData := buildTailorTemplateData(updatedData, resume, projects)

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
// renderTailorHTML. Keeps original resume data and only uses tailored experiences.
func buildTailorTemplateData(polished map[string]interface{}, resume *models.Resume, projects []*models.Project) map[string]interface{} {
	ud := map[string]interface{}{
		"name":  resume.Name,
		"email": resume.Email,
		"phone": resume.Phone,
	}

	// Summary — use original from resume (not polished)
	if len(resume.Summary) > 0 {
		var s string
		if err := json.Unmarshal(resume.Summary, &s); err != nil {
			s = strings.Trim(string(resume.Summary), "\"")
		}
		if s != "" {
			ud["summary"] = s
		}
	}

	// Skills — use original from resume (not polished)
	if len(resume.Skills) > 0 {
		var arr []string
		if err := json.Unmarshal(resume.Skills, &arr); err == nil {
			ud["skills"] = arr
		} else {
			var s string
			if err := json.Unmarshal(resume.Skills, &s); err == nil && s != "" {
				var cleaned []string
				for _, sk := range strings.Split(s, ",") {
					sk = strings.TrimSpace(sk)
					if sk != "" {
						cleaned = append(cleaned, sk)
					}
				}
				ud["skills"] = cleaned
			}
		}
	}

	// Experiences — use TAILORED version
	if exps, ok := polished["experiences"].([]interface{}); ok && len(exps) > 0 {
		ud["experience"] = tailorFormatExperiences(exps)
	} else if e, ok := polished["experience"].(string); ok && e != "" {
		ud["experience"] = e
	} else if resume.Experience != "" {
		ud["experience"] = resume.Experience
	}

	// Education — use original from resume
	if resume.Education != "" {
		ud["education"] = resume.Education
	}

	// Projects — use original from DB
	if len(projects) > 0 {
		ud["projects"] = tailorFormatProjects(projects)
	}

	if resume.Location != "" {
		ud["location"] = resume.Location
	}

	return ud
}

// tailorFormatProjects converts project records into formatted text for the HTML template.
func tailorFormatProjects(projects []*models.Project) string {
	var parts []string
	for _, p := range projects {
		header := p.ProjectName
		if p.Technologies != "" {
			header += " | " + p.Technologies
		}
		entry := header
		if p.Description != "" {
			entry += "\n" + p.Description
		}
		parts = append(parts, entry)
	}
	return strings.Join(parts, "\n\n")
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

// renderTailorHTML builds a professional HTML resume and writes it to outputPath.
func renderTailorHTML(data map[string]interface{}, outputPath string) error {
	name, _ := data["name"].(string)
	email, _ := data["email"].(string)
	phone, _ := data["phone"].(string)
	location, _ := data["location"].(string)
	summary, _ := data["summary"].(string)
	experience, _ := data["experience"].(string)
	education, _ := data["education"].(string)
	projectsText, _ := data["projects"].(string)

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
	contactLine := strings.Join(contactParts, " | ")

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
            <div class="job-header">%s</div>`, html.EscapeString(lines[0]))
			for _, line := range lines[1:] {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				line = strings.TrimLeft(line, "•-*· ")
				if line != "" {
					items += fmt.Sprintf(`
            <div class="bullet">&bull; %s</div>`, html.EscapeString(line))
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

	// Build projects section
	var projectsSection string
	if projectsText != "" {
		entries := strings.Split(projectsText, "\n\n")
		var items string
		for _, entry := range entries {
			lines := strings.Split(strings.TrimSpace(entry), "\n")
			if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
				continue
			}
			items += fmt.Sprintf(`        <div class="experience-item">
            <div class="job-header">%s</div>`, html.EscapeString(lines[0]))
			for _, line := range lines[1:] {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				line = strings.TrimLeft(line, "•-*· ")
				if line != "" {
					items += fmt.Sprintf(`
            <div class="bullet">&bull; %s</div>`, html.EscapeString(line))
				}
			}
			items += `
        </div>`
		}
		projectsSection = fmt.Sprintf(`
    <div class="section">
        <div class="section-title">Projects</div>
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
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Calibri', 'Segoe UI', Arial, sans-serif;
            line-height: 1.4;
            padding: 0.5in 0.6in;
            background: white;
            font-size: 10.5pt;
            color: #1a1a1a;
        }
        .header {
            text-align: center;
            padding-bottom: 8px;
            margin-bottom: 10px;
            border-bottom: 1.5px solid #1a1a1a;
        }
        .name {
            font-size: 20pt;
            font-weight: 700;
            color: #1a1a1a;
            letter-spacing: 0.5px;
            margin-bottom: 3px;
        }
        .contact {
            color: #444;
            font-size: 9.5pt;
        }
        .section {
            margin-bottom: 10px;
        }
        .section-title {
            font-size: 11pt;
            font-weight: 700;
            color: #1a1a1a;
            border-bottom: 1px solid #999;
            padding-bottom: 2px;
            margin-bottom: 6px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .summary-text {
            font-size: 10pt;
            line-height: 1.45;
            color: #333;
        }
        .skills {
            font-size: 10pt;
            line-height: 1.5;
            color: #333;
        }
        .experience-item {
            margin-bottom: 8px;
        }
        .job-header {
            font-size: 10.5pt;
            font-weight: 600;
            color: #1a1a1a;
            margin-bottom: 3px;
        }
        .bullet {
            margin-left: 14px;
            margin-bottom: 1px;
            font-size: 10pt;
            line-height: 1.4;
            color: #333;
        }
        .education-text {
            font-size: 10pt;
            line-height: 1.45;
            white-space: pre-line;
            color: #333;
        }
    </style>
</head>
<body>
    <div class="header">
        <div class="name">%s</div>
        <div class="contact">%s</div>
    </div>
%s%s%s%s%s
</body>
</html>`,
		html.EscapeString(name),
		html.EscapeString(name),
		contactLine,
		summarySection,
		skillsSection,
		experienceSection,
		projectsSection,
		educationSection,
	)

	return os.WriteFile(outputPath, []byte(htmlContent), 0644)
}
