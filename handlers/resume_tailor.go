package handlers

import (
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
			c.JSON(http.StatusNotFound, gin.H{"error": "No resume found. Please create a resume first."})
			return
		}

		// Load the user's last rendered HTML (saved when they generated a PDF from the frontend)
		savedHTML, _ := resumeModel.GetLastHTML(userIDInt)
		if strings.TrimSpace(savedHTML) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "Please build your resume on HiHired first, then come back to tailor it.",
				"redirect": "https://hihired.org/build",
			})
			return
		}

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

		// Replace experience bullets in the user's saved frontend HTML
		tailoredHTML := replaceExperienceBulletsInHTML(savedHTML, experiences, polishedMaps)

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


