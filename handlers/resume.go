package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"resumeai/services"

	"github.com/gin-gonic/gin"
)

type ResumeRequest struct {
	Position    string   `json:"position"`
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	Phone       string   `json:"phone"`
	Summary     string   `json:"summary"`
	Experience  string   `json:"experience"`
	Education   string   `json:"education"`
	Skills      []string `json:"skills"`
	Format      string   `json:"format"`
	HtmlContent string   `json:"htmlContent"` // HTML content from live preview
}

func GenerateResume(c *gin.Context) {
	var req ResumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Prepare user data for template
	userData := map[string]interface{}{
		"name":        req.Name,
		"email":       req.Email,
		"phone":       req.Phone,
		"summary":     req.Summary,
		"experience":  req.Experience,
		"education":   req.Education,
		"skills":      req.Skills,
		"position":    req.Position,
		"htmlContent": req.HtmlContent,
	}

	// Create static directory if it doesn't exist
	saveDir := "./static"
	if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
		return
	}

	// Generate unique filename (nanosecond precision) to avoid caching/stale files
	filename := fmt.Sprintf("resume_%d.html", time.Now().UnixNano())
	filepath := saveDir + "/" + filename

	// Use default template if none selected
	templateFormat := req.Format
	if templateFormat == "" {
		templateFormat = "temp1"
	}

	// Generate HTML resume using Python
	if err := generateHTMLResumeWithPython(templateFormat, userData, filepath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Resume generated successfully.",
		"filePath": "/static/" + filename,
	})
}

func generateHTMLResumeWithPython(templateName string, userData map[string]interface{}, outputPath string) error {
	// Convert userData to JSON
	userDataJSON, err := json.Marshal(userData)
	if err != nil {
		return fmt.Errorf("failed to marshal user data: %v", err)
	}

	// Pass JSON via stdin to avoid command line length limits (Windows)
	cmd := exec.Command("python3", "generate_resume.py", templateName, "-", outputPath)
	cmd.Dir = "."
	cmd.Stdin = bytes.NewReader(userDataJSON)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("python script failed: %v, output: %s", err, string(output))
	}
	return nil
}

func GeneratePDFResume(c *gin.Context) {
	fmt.Println("GeneratePDFResume handler called")

	var req ResumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fmt.Printf("Error binding JSON: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	htmlContent := req.HtmlContent
	if htmlContent == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "HTML content is required"})
		return
	}

	// Log HTML content details (debug-only)
	fmt.Printf("HTML Content Length: %d characters\n", len(htmlContent))

	previewLength := 1000
	if len(htmlContent) < previewLength {
		previewLength = len(htmlContent)
	}
	fmt.Printf("HTML Content Preview (first %d chars): %s\n", previewLength, htmlContent[:previewLength])

	// Check for specific CSS properties in the HTML
	if strings.Contains(htmlContent, "@page") {
		fmt.Println("Found @page CSS rule in HTML content")
	}
	if strings.Contains(htmlContent, ".preview") {
		fmt.Println("Found .preview CSS class in HTML content")
	}
	if strings.Contains(htmlContent, "width:") {
		fmt.Println("Found width CSS property in HTML content")
	}

	// Check for our specific CSS overrides
	if strings.Contains(htmlContent, "font-size: 18pt") {
		fmt.Println("Found font-size: 18pt override in HTML content")
	}
	if strings.Contains(htmlContent, "font-size: 14pt") {
		fmt.Println("Found font-size: 14pt override in HTML content")
	}
	if strings.Contains(htmlContent, "!important") {
		fmt.Println("Found !important declarations in HTML content")
	}
	if strings.Contains(htmlContent, "ULTRA-AGGRESSIVE") {
		fmt.Println("Found ULTRA-AGGRESSIVE overrides in HTML content")
	}
	if strings.Contains(htmlContent, "color: #000000") {
		fmt.Println("Found #000000 color overrides in HTML content")
	}
	if strings.Contains(htmlContent, ".preview.modern .name") {
		fmt.Println("Found .preview.modern .name overrides in HTML content")
	}

	// Ensure output dir exists
	saveDir := "./static"
	if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
		return
	}

	// Unique filename for PDF to avoid collisions and stale caching
	filename := fmt.Sprintf("resume_%d.pdf", time.Now().UnixNano())
	pdfPath := filepath.Join(saveDir, filename)

	// Prepare user data for PDF generation
	userData := map[string]interface{}{
		"htmlContent": htmlContent,
	}

	// Generate PDF via Python + wkhtmltopdf
	if err := generatePDFResumeWithPython("temp1", userData, pdfPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Upload to S3 (required)
	s3svc, s3err := services.NewS3Service()
	if s3err != nil {
		fmt.Printf("S3 not configured or invalid: %v\n", s3err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage service unavailable"})
		return
	}

	key := "resumes/" + filename
	url, uploadErr := s3svc.UploadFile(pdfPath, key)
	if uploadErr != nil {
		fmt.Printf("S3 upload failed: %v\n", uploadErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload PDF to storage"})
		return
	}

	if presigned, preErr := s3svc.GeneratePresignedURL(key); preErr == nil {
		c.JSON(http.StatusOK, gin.H{
			"message":     "PDF resume generated and uploaded to S3.",
			"downloadURL": presigned,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "PDF resume generated and uploaded to S3.",
		"downloadURL": url,
	})
}

func generatePDFResumeWithPython(templateName string, userData map[string]interface{}, outputPath string) error {
	userDataJSON, err := json.Marshal(userData)
	if err != nil {
		return fmt.Errorf("failed to marshal user data: %v", err)
	}

	fmt.Printf("Calling Python script with template: %s, outputPath: %s (passing %d bytes via stdin)\n", templateName, outputPath, len(userDataJSON))
	// Pass JSON via stdin to avoid command line length limits (Windows) and quoting issues
	cmd := exec.Command("python3", "generate_resume.py", templateName, "-", outputPath)
	cmd.Dir = "."
	cmd.Stdin = bytes.NewReader(userDataJSON)
	output, err := cmd.CombinedOutput()

	// Always log the output for debugging
	fmt.Printf("Python script output:\n%s\n", string(output))

	if err != nil {
		return fmt.Errorf("python script failed: %v, output: %s", err, string(output))
	}
	return nil
}
