package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"resumeai/models"
	"resumeai/services"
)

// TailorResume handles POST /api/resume/tailor
// It loads the user's last rendered resume HTML, tailors experience descriptions
// based on the provided job description, and generates a PDF that looks identical
// to the user's frontend template.
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

		// Get authenticated user
		userIDInt, ok := extractUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		// Load resume from DB
		resume, err := resumeModel.GetByUserID(userIDInt)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "no rows") {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":    "Please build your resume on HiHired first, then come back to tailor it.",
					"redirect": "https://hihired.org/build",
				})
			} else {
				fmt.Printf("[Tailor] Error loading resume for user %d: %v\n", userIDInt, err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": fmt.Sprintf("Failed to load resume (user %d): %v", userIDInt, err),
				})
			}
			return
		}

		// Load the user's last rendered HTML (saved when they generated a PDF from the frontend)
		savedHTML, _ := resumeModel.GetLastHTML(userIDInt)
		hasSavedHTML := strings.TrimSpace(savedHTML) != ""

		// Load structured experiences
		experiences, err := resumeModel.GetExperiencesByResumeID(resume.ID)
		if err != nil {
			fmt.Printf("[Tailor] Warning: failed to load experiences: %v\n", err)
			experiences = nil
		}

		if len(experiences) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "No experiences found. Please add work experience on HiHired first.",
				"redirect": "https://hihired.org/build",
			})
			return
		}

		// Build experience maps for the polish agent
		expMaps := make([]map[string]interface{}, 0, len(experiences))
		for _, exp := range experiences {
			expMaps = append(expMaps, map[string]interface{}{
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

		// Polish each experience with AI
		ctx := c.Request.Context()
		polishedMaps := make([]map[string]interface{}, 0, len(expMaps))
		for _, expMap := range expMaps {
			polished, err := polishAgent.PolishExperience(ctx, expMap, jobDesc, "")
			if err != nil {
				fmt.Printf("[Tailor] Failed to polish experience: %v\n", err)
				polishedMaps = append(polishedMaps, expMap)
			} else {
				polishedMaps = append(polishedMaps, polished)
			}
		}
		fmt.Printf("[Tailor] Tailored %d experience(s) for job description\n", len(polishedMaps))

		projects, projectErr := loadProjectsForTailor(projectModel, resume.ID)
		if projectErr != nil {
			fmt.Printf("[Tailor] Warning: failed to load projects: %v\n", projectErr)
		}

		tailoredSummary := tailorSummaryForJob(jobDesc, resume, polishedMaps)
		tailoredProjects := tailorProjectsForJob(jobDesc, projects)

		// Generate tailored HTML
		var tailoredHTML string
		if hasSavedHTML {
			// Saved HTML path still preserves the user's exact frontend template,
			// but now we also inject a JD-specific summary so the result is not
			// just lightly rewritten bullets.
			savedHTML = stripUIChrome(savedHTML)
			tailoredHTML = replaceExperienceBulletsInHTML(savedHTML, experiences, polishedMaps)
			tailoredHTML = replaceSummaryInHTML(tailoredHTML, tailoredSummary)
			fmt.Println("[Tailor] Using saved frontend HTML template")
		} else {
			// Fallback: user has resume data but hasn't generated a PDF since
			// we started saving HTML — use a template matching their selected format
			fmt.Println("[Tailor] No saved HTML, using fallback template")
			tailoredHTML = renderFallbackTailorHTML(resume, experiences, polishedMaps, tailoredSummary, tailoredProjects)
		}

		// Ensure output directory
		saveDir := "./static"
		if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create output directory"})
			return
		}

		timestamp := time.Now().UnixNano()

		// Write tailored HTML to disk
		htmlFilename := fmt.Sprintf("tailored_%d.html", timestamp)
		htmlPath := filepath.Join(saveDir, htmlFilename)
		if err := os.WriteFile(htmlPath, []byte(tailoredHTML), 0644); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write tailored HTML"})
			return
		}

		// Convert HTML to PDF
		templateSlug := normalizeTemplateFormat(resume.SelectedFormat)
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

		// Upload to S3
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

		// Save to resume history
		if resumeHistoryModel != nil {
			resumeName := fmt.Sprintf("Tailored Resume %s", time.Now().Format("2006-01-02 15:04"))
			if _, err := resumeHistoryModel.Create(userIDInt, resumeName, key); err != nil {
				fmt.Printf("[Tailor] Failed to save history: %v\n", err)
			}
			if err := resumeHistoryModel.CleanupOldResumes(userIDInt, 10); err != nil {
				fmt.Printf("[Tailor] Failed to cleanup old resumes: %v\n", err)
			}
		}

		// Generate presigned download URL
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

// replaceExperienceBulletsInHTML finds each experience-entry in the saved HTML
// by matching the company name, then replaces its bullet-point divs with the
// polished description from the AI agent.
//
// The frontend HTML structure for each experience:
//
//	<div class="experience-entry ...">
//	  <div class="experience-header">...company name spans...</div>
//	  <div class="experience-field experience-field-block ...">
//	    <div style="...">• bullet text</div>
//	    <div style="...">• bullet text</div>
//	  </div>
//	</div>
func replaceExperienceBulletsInHTML(htmlContent string, original []models.ExperienceRecord, polished []map[string]interface{}) string {
	if len(original) != len(polished) || len(original) == 0 {
		return htmlContent
	}

	result := htmlContent

	for i, exp := range original {
		polishedMap := polished[i]
		polishedDesc, _ := polishedMap["description"].(string)
		if strings.TrimSpace(polishedDesc) == "" {
			continue
		}

		if strings.TrimSpace(exp.Description) == "" {
			continue
		}

		// Strategy: find the experience-field-block div that belongs to this
		// experience (identified by company name appearing in the preceding
		// experience-header), then replace the bullet divs inside it.
		result = replaceBlockForCompany(result, exp.Company, polishedDesc)
	}

	return result
}

// replaceBlockForCompany finds the experience-field-block associated with a
// given company in the HTML and replaces its bullet content with the polished
// description.
func replaceBlockForCompany(htmlContent, company, polishedDesc string) string {
	if company == "" {
		return htmlContent
	}

	// Find the company name in the HTML (HTML-escaped)
	escapedCompany := html.EscapeString(company)

	// Locate the position of this company in the HTML
	companyIdx := strings.Index(htmlContent, escapedCompany)
	if companyIdx < 0 {
		// Try case-insensitive
		companyIdx = strings.Index(strings.ToLower(htmlContent), strings.ToLower(escapedCompany))
	}
	if companyIdx < 0 {
		fmt.Printf("[Tailor] Could not find company '%s' in HTML\n", company)
		return htmlContent
	}

	// Find the experience-field-block div after the company name
	blockMarker := `experience-field-block`
	blockStart := strings.Index(htmlContent[companyIdx:], blockMarker)
	if blockStart < 0 {
		fmt.Printf("[Tailor] Could not find experience-field-block after company '%s'\n", company)
		return htmlContent
	}
	blockStart += companyIdx

	// Find the opening tag of the block div
	// Walk backwards from blockMarker to find the '<div' that contains it
	divStart := strings.LastIndex(htmlContent[:blockStart], "<div")
	if divStart < 0 {
		return htmlContent
	}

	// Find the closing '>' of this opening div tag
	tagEnd := strings.Index(htmlContent[divStart:], ">")
	if tagEnd < 0 {
		return htmlContent
	}
	contentStart := divStart + tagEnd + 1

	// Find the matching closing </div> for this block
	// We need to handle nested divs
	contentEnd := findMatchingCloseDiv(htmlContent, contentStart)
	if contentEnd < 0 {
		return htmlContent
	}

	// Extract the opening tag (to preserve its style attributes)
	openingTag := htmlContent[divStart : contentStart]

	// Build the new bullet content from the polished description
	// Extract the style from the first existing bullet div to reuse it
	existingContent := htmlContent[contentStart:contentEnd]
	bulletStyle := extractBulletDivStyle(existingContent)

	newBullets := buildBulletDivs(polishedDesc, bulletStyle)

	// Replace the block content
	result := htmlContent[:contentStart] + newBullets + htmlContent[contentEnd:]

	_ = openingTag // used implicitly (we keep the original opening tag)
	return result
}

// findMatchingCloseDiv finds the position of the </div> that closes the div
// starting at contentStart. Returns the index of the start of </div>.
func findMatchingCloseDiv(s string, contentStart int) int {
	depth := 1
	pos := contentStart
	for depth > 0 && pos < len(s) {
		nextOpen := strings.Index(s[pos:], "<div")
		nextClose := strings.Index(s[pos:], "</div>")

		if nextClose < 0 {
			return -1
		}

		if nextOpen >= 0 && nextOpen < nextClose {
			depth++
			pos += nextOpen + 4
		} else {
			depth--
			if depth == 0 {
				return pos + nextClose
			}
			pos += nextClose + 6
		}
	}
	return -1
}

// extractBulletDivStyle extracts the style attribute from the first bullet div
// in the existing content, so we can reuse the same styling for polished bullets.
func extractBulletDivStyle(existingContent string) string {
	// Match: <div style="color: rgb(55, 65, 81); font-size: 14.4px; margin-left: 8.4px; margin-bottom: 3.12px; ...">
	re := regexp.MustCompile(`<div\s+style="([^"]*)"[^>]*>\s*(?:•|&#x2022;|&bull;)`)
	match := re.FindStringSubmatch(existingContent)
	if len(match) > 1 {
		return match[1]
	}
	// Fallback default style matching the frontend's classic template
	return "color: rgb(55, 65, 81); font-size: 14.4px; margin-left: 8.4px; margin-bottom: 3.12px;"
}

// buildBulletDivs converts a polished description (newline-separated bullets)
// into HTML divs matching the frontend's bullet point structure.
func buildBulletDivs(description, style string) string {
	lines := strings.Split(description, "\n")
	var sb strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip leading bullet characters
		line = strings.TrimLeft(line, "•-*· ")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Remove height normalization styles that might be in the original
		// (those are added by the frontend's cloneNode logic)
		cleanStyle := style
		fmt.Fprintf(&sb, `<div style="%s">• %s</div>`, cleanStyle, html.EscapeString(line))
	}
	return sb.String()
}

// stripUIChrome removes non-resume UI elements and preview-only styles from
// saved HTML that the frontend may have captured inside .live-preview-container.
func stripUIChrome(h string) string {
	// Remove anchor tags (e.g. "Open resume manually")
	h = regexp.MustCompile(`<a\b[^>]*>.*?</a>`).ReplaceAllString(h, "")
	// Remove button tags
	h = regexp.MustCompile(`<button\b[^>]*>.*?</button>`).ReplaceAllString(h, "")
	// Remove the download-notice wrapper div (contains "pop-ups" or "new tab" text)
	h = regexp.MustCompile(`(?s)<div[^>]*>\s*(?:We opened your resume|If it did not appear)[^<]*(?:<[^>]*>[^<]*)*?</div>\s*</div>`).ReplaceAllString(h, "")

	// Inject a style block before </head> (or prepend) to clean up preview artifacts
	cleanup := `<style id="tailor-cleanup">
.live-preview-container { background: #fff !important; padding: 0 !important; border: none !important; box-shadow: none !important; }
.single-page-container, .page-wrapper { border: none !important; box-shadow: none !important; outline: none !important; overflow: visible !important; }
*:focus-within { outline: none !important; }
</style>`
	if idx := strings.Index(h, "</head>"); idx >= 0 {
		h = h[:idx] + cleanup + h[idx:]
	} else {
		h = cleanup + h
	}
	return h
}

// renderFallbackTailorHTML generates HTML using the user's selected template
// style when no saved frontend HTML exists (user hasn't downloaded a PDF yet).
func renderFallbackTailorHTML(resume *models.Resume, experiences []models.ExperienceRecord, polished []map[string]interface{}, tailoredSummary string, tailoredProjects []tailoredProject) string {
	name := resume.Name
	email := resume.Email
	phone := resume.Phone
	location := resume.Location

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

	var summarySection string
	if strings.TrimSpace(tailoredSummary) != "" {
		summarySection = fmt.Sprintf(`
    <div class="section">
        <div class="section-title">Summary</div>
        <p class="summary-text">%s</p>
    </div>`, html.EscapeString(strings.TrimSpace(tailoredSummary)))
	} else if len(resume.Summary) > 0 {
		var s string
		if err := json.Unmarshal(resume.Summary, &s); err != nil {
			s = strings.Trim(string(resume.Summary), "\"")
		}
		if s != "" {
			summarySection = fmt.Sprintf(`
    <div class="section">
        <div class="section-title">Summary</div>
        <p class="summary-text">%s</p>
    </div>`, html.EscapeString(s))
		}
	}

	var skillsSection string
	if len(resume.Skills) > 0 {
		var arr []string
		if err := json.Unmarshal(resume.Skills, &arr); err == nil && len(arr) > 0 {
			var escaped []string
			for _, sk := range arr {
				escaped = append(escaped, html.EscapeString(sk))
			}
			skillsSection = fmt.Sprintf(`
    <div class="section">
        <div class="section-title">Skills</div>
        <div class="skills">%s</div>
    </div>`, strings.Join(escaped, " &bull; "))
		}
	}

	var experienceSection string
	if len(polished) > 0 {
		var items strings.Builder
		for idx, p := range polished {
			title, _ := p["jobTitle"].(string)
			company, _ := p["company"].(string)
			desc, _ := p["description"].(string)

			var header string
			if idx < len(experiences) {
				exp := experiences[idx]
				header = html.EscapeString(title)
				if company != "" {
					header += " &bull; " + html.EscapeString(company)
				}
				loc := strings.TrimSpace(exp.City + ", " + exp.State)
				loc = strings.Trim(loc, ", ")
				if loc != "" {
					header += " &bull; " + html.EscapeString(loc)
				}
				dates := formatExpDates(exp.StartDate, exp.EndDate, exp.CurrentlyWorking)
				if dates != "" {
					header += " &bull; " + html.EscapeString(dates)
				}
			} else {
				header = html.EscapeString(title)
				if company != "" {
					header += " &bull; " + html.EscapeString(company)
				}
			}

			fmt.Fprintf(&items, `        <div class="experience-item">
            <div class="job-header">%s</div>`, header)
			for _, line := range strings.Split(desc, "\n") {
				line = strings.TrimSpace(line)
				line = strings.TrimLeft(line, "•-*· ")
				line = strings.TrimSpace(line)
				if line != "" {
					fmt.Fprintf(&items, `
            <div class="bullet">&bull; %s</div>`, html.EscapeString(line))
				}
			}
			items.WriteString(`
        </div>`)
		}
		experienceSection = fmt.Sprintf(`
    <div class="section">
        <div class="section-title">Experience</div>
%s
    </div>`, items.String())
	}

	var projectsSection string
	if len(tailoredProjects) > 0 {
		var items strings.Builder
		for _, p := range tailoredProjects {
			header := html.EscapeString(p.Name)
			if p.Technologies != "" {
				header += " | " + html.EscapeString(p.Technologies)
			}
			fmt.Fprintf(&items, `        <div class="experience-item">
            <div class="job-header">%s</div>`, header)
			for _, line := range strings.Split(p.Description, "\n") {
				line = strings.TrimSpace(line)
				line = strings.TrimLeft(line, "•-*· ")
				line = strings.TrimSpace(line)
				if line != "" {
					fmt.Fprintf(&items, `
            <div class="bullet">&bull; %s</div>`, html.EscapeString(line))
				}
			}
			items.WriteString(`
        </div>`)
		}
		projectsSection = fmt.Sprintf(`
    <div class="section">
        <div class="section-title">Projects</div>
%s
    </div>`, items.String())
	}

	var educationSection string
	if resume.Education != "" {
		educationSection = fmt.Sprintf(`
    <div class="section">
        <div class="section-title">Education</div>
        <div class="education-text">%s</div>
    </div>`, html.EscapeString(resume.Education))
	}

	templateCSS := getFallbackTemplateCSS(resume.SelectedFormat)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>%s - Resume</title>
    <style>%s</style>
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
		templateCSS,
		html.EscapeString(name),
		contactLine,
		summarySection,
		skillsSection,
		experienceSection,
		projectsSection,
		educationSection,
	)
}

func formatExpDates(start, end string, currentlyWorking bool) string {
	if start == "" && end == "" {
		return ""
	}
	if currentlyWorking {
		if start != "" {
			return start + " - Present"
		}
		return "Present"
	}
	if start != "" && end != "" {
		return start + " - " + end
	}
	if start != "" {
		return start
	}
	return end
}

type tailoredProject struct {
	Name         string
	Technologies string
	Description  string
}

func loadProjectsForTailor(projectModel *models.ProjectModel, resumeID int) ([]*models.Project, error) {
	if projectModel == nil {
		return nil, nil
	}
	return projectModel.GetByResumeID(resumeID)
}

func tailorSummaryForJob(jobDesc string, resume *models.Resume, polished []map[string]interface{}) string {
	var existingSummary string
	if len(resume.Summary) > 0 {
		if err := json.Unmarshal(resume.Summary, &existingSummary); err != nil {
			existingSummary = strings.Trim(string(resume.Summary), "\"")
		}
	}

	var expLines []string
	for _, p := range polished {
		if desc, _ := p["description"].(string); strings.TrimSpace(desc) != "" {
			expLines = append(expLines, desc)
		}
	}

	var skills []string
	if len(resume.Skills) > 0 {
		_ = json.Unmarshal(resume.Skills, &skills)
	}

	prompt := services.BuildSummaryOptimizationPromptWithSkills(
		strings.Join(expLines, "\n"),
		resume.Education,
		skills,
		existingSummary,
		jobDesc,
		nil,
		nil,
	)
	result, err := services.CallGeminiStrict(prompt)
	if err != nil {
		fmt.Printf("[Tailor] Failed to tailor summary: %v\n", err)
		return existingSummary
	}
	result = strings.TrimSpace(result)
	if result == "" {
		return existingSummary
	}
	return result
}

func tailorProjectsForJob(jobDesc string, projects []*models.Project) []tailoredProject {
	if len(projects) == 0 {
		return nil
	}

	out := make([]tailoredProject, 0, len(projects))
	for _, p := range projects {
		if p == nil {
			continue
		}
		desc := strings.TrimSpace(p.Description)
		if desc == "" {
			out = append(out, tailoredProject{Name: p.ProjectName, Technologies: p.Technologies, Description: p.Description})
			continue
		}
		prompt := services.BuildProjectOptimizationPromptWithSkills(jobDesc, desc, desc, nil, nil)
		result, err := services.CallGeminiStrict(prompt)
		if err != nil || strings.TrimSpace(result) == "" {
			if err != nil {
				fmt.Printf("[Tailor] Failed to tailor project '%s': %v\n", p.ProjectName, err)
			}
			result = desc
		}
		out = append(out, tailoredProject{
			Name:         p.ProjectName,
			Technologies: p.Technologies,
			Description:  strings.TrimSpace(result),
		})
	}
	return out
}

func replaceSummaryInHTML(htmlContent, tailoredSummary string) string {
	if strings.TrimSpace(tailoredSummary) == "" {
		return htmlContent
	}

	re := regexp.MustCompile(`(?is)(<div[^>]*class="[^"]*section-title[^"]*"[^>]*>\s*Summary\s*</div>\s*<p[^>]*class="[^"]*summary-text[^"]*"[^>]*>)(.*?)(</p>)`)
	if re.MatchString(htmlContent) {
		return re.ReplaceAllString(htmlContent, `${1}`+html.EscapeString(strings.TrimSpace(tailoredSummary))+`${3}`)
	}
	return htmlContent
}

func getFallbackTemplateCSS(selectedFormat string) string {
	base := `
        @page { size: Letter; margin: 0; }
        * { margin: 0; padding: 0; box-sizing: border-box; }
        .section { margin-bottom: 10px; }
        .summary-text { line-height: 1.45; }
        .skills { line-height: 1.5; }
        .experience-item { margin-bottom: 8px; }
        .education-text { line-height: 1.45; white-space: pre-line; }
`
	switch normalizeTemplateFormat(selectedFormat) {
	case "modern-clean":
		return base + `
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.4; padding: 0.5in 0.6in; background: white; font-size: 10.5pt; color: #1a1a1a; }
        .header { text-align: center; padding-bottom: 8px; margin-bottom: 10px; border-bottom: 3px solid #3498db; }
        .name { font-size: 20pt; font-weight: 600; color: #2c3e50; margin-bottom: 3px; }
        .contact { color: #7f8c8d; font-size: 9.5pt; }
        .section-title { font-size: 11pt; font-weight: 600; color: #3498db; border-bottom: 1px solid #000; padding-bottom: 2px; margin-bottom: 6px; text-transform: uppercase; letter-spacing: 1px; }
        .job-header { font-size: 10.5pt; font-weight: 600; color: #2c3e50; margin-bottom: 3px; }
        .bullet { margin-left: 14px; margin-bottom: 1px; font-size: 10pt; line-height: 1.4; color: #374151; }
`
	case "executive-serif":
		return base + `
        body { font-family: Georgia, serif; line-height: 1.4; padding: 0.5in 0.6in; background: white; font-size: 10.5pt; color: #1a1a1a; }
        .header { text-align: center; padding-bottom: 8px; margin-bottom: 10px; border-bottom: 1.5px solid #2A7B88; }
        .name { font-size: 20pt; font-weight: 700; color: #2A7B88; margin-bottom: 3px; }
        .contact { color: #7f8c8d; font-size: 9.5pt; }
        .section-title { font-size: 11pt; font-weight: 700; color: #2A7B88; border-bottom: 1px solid #999; padding-bottom: 2px; margin-bottom: 6px; text-transform: uppercase; }
        .job-header { font-size: 10.5pt; font-weight: 600; color: #1a1a1a; margin-bottom: 3px; }
        .bullet { margin-left: 14px; margin-bottom: 1px; font-size: 10pt; line-height: 1.4; color: #333; }
`
	case "attorney-template":
		return base + `
        body { font-family: Georgia, 'Times New Roman', serif; line-height: 1.4; padding: 0.5in 0.6in; background: white; font-size: 10.5pt; color: #443730; }
        .header { text-align: center; padding: 12px 18px; margin-bottom: 10px; background: #DCC3AE; border-bottom: 5px solid #B68A65; }
        .name { font-size: 18pt; font-weight: 700; color: #3C2E27; letter-spacing: 3px; text-transform: uppercase; margin-bottom: 3px; }
        .contact { color: #3C2E27; font-size: 9.5pt; }
        .section-title { font-size: 11pt; font-weight: 700; color: #3C2E27; border-bottom: 1px solid #E5D1C0; padding-bottom: 2px; margin-bottom: 6px; text-transform: uppercase; letter-spacing: 2px; }
        .job-header { font-size: 10.5pt; font-weight: 600; color: #3C2E27; margin-bottom: 3px; }
        .bullet { margin-left: 14px; margin-bottom: 1px; font-size: 10pt; line-height: 1.4; color: #443730; }
`
	default: // classic-professional
		return base + `
        body { font-family: 'Calibri', 'Segoe UI', Arial, sans-serif; line-height: 1.4; padding: 0.5in 0.6in; background: white; font-size: 10.5pt; color: #1a1a1a; }
        .header { text-align: center; padding-bottom: 8px; margin-bottom: 10px; border-bottom: 1.5px solid #1a1a1a; }
        .name { font-size: 20pt; font-weight: 700; color: #1a1a1a; margin-bottom: 3px; }
        .contact { color: #444; font-size: 9.5pt; }
        .section-title { font-size: 11pt; font-weight: 700; color: #1a1a1a; border-bottom: 1px solid #999; padding-bottom: 2px; margin-bottom: 6px; text-transform: uppercase; letter-spacing: 0.5px; }
        .job-header { font-size: 10.5pt; font-weight: 600; color: #1a1a1a; margin-bottom: 3px; }
        .bullet { margin-left: 14px; margin-bottom: 1px; font-size: 10pt; line-height: 1.4; color: #333; }
`
	}
}
