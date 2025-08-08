package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"resumeai/services"
	"time"

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

	// Generate filename with timestamp
	filename := "resume_" + time.Now().Format("20060102150405") + ".html"
	filepath := saveDir + "/" + filename

	// Use default template if none selected
	templateFormat := req.Format
	if templateFormat == "" {
		templateFormat = "temp1"
	}

	// Generate HTML resume using Python
	err := generateHTMLResumeWithPython(templateFormat, userData, filepath)
	if err != nil {
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

	// Run Python script with correct working directory
	cmd := exec.Command("python", "generate_resume.py", templateName, string(userDataJSON), outputPath)
	cmd.Dir = "." // Set working directory to current directory where templates are located

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("python script failed: %v, output: %s", err, string(output))
	}

	fmt.Printf("Python script output: %s\n", string(output))
	return nil
}

func GeneratePDFResume(c *gin.Context) {
	fmt.Println("GeneratePDFResume handler called")

	var req ResumeRequest
	var err error
	if err = c.ShouldBindJSON(&req); err != nil {
		fmt.Printf("Error binding JSON: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fmt.Printf("Received PDF generation request: %+v\n", req)

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
	err = os.MkdirAll(saveDir, os.ModePerm)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
		return
	}

	// Generate filename with timestamp
	filename := "resume_" + time.Now().Format("20060102150405") + ".pdf"
	filepath := saveDir + "/" + filename

	// Use default template if none selected
	templateFormat := req.Format
	if templateFormat == "" {
		templateFormat = "temp1"
	}

	// Generate PDF resume using Python
	err = generatePDFResumeWithPython(templateFormat, userData, filepath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Initialize S3 service
	s3Service, err := services.NewS3Service()
	if err != nil {
		log.Printf("Failed to initialize S3 service: %v", err)
		// Fallback to local file
		c.JSON(http.StatusOK, gin.H{
			"message":  "PDF resume generated successfully.",
			"filePath": "/static/" + filename,
		})
		return
	}

	// Upload to S3
	downloadURL, err := s3Service.UploadFile(filepath, filename)
	if err != nil {
		log.Printf("Failed to upload to S3: %v", err)
		// Fallback to local file
		c.JSON(http.StatusOK, gin.H{
			"message":  "PDF resume generated successfully.",
			"filePath": "/static/" + filename,
		})
		return
	}

	// Generate presigned URL for secure download (avoids CORS issues)
	presignedURL, err := s3Service.GeneratePresignedURL(filename)
	if err != nil {
		log.Printf("Failed to generate presigned URL: %v", err)
		// Use direct S3 URL as fallback
		c.JSON(http.StatusOK, gin.H{
			"message":     "PDF resume generated successfully.",
			"downloadURL": downloadURL,
			"filePath":    "/static/" + filename,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "PDF resume generated successfully.",
		"downloadURL": presignedURL,          // Use presigned URL to avoid CORS
		"filePath":    "/static/" + filename, // Keep for backward compatibility
	})
}

func generatePDFResumeWithPython(templateName string, userData map[string]interface{}, outputPath string) error {
	// Get HTML content from the request
	htmlContent, ok := userData["htmlContent"].(string)
	if !ok || htmlContent == "" {
		return fmt.Errorf("HTML content is required for PDF generation")
	}

	// Create temporary HTML file
	htmlPath := outputPath[:len(outputPath)-4] + ".html"
	err := os.WriteFile(htmlPath, []byte(htmlContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write HTML file: %v", err)
	}

	// Convert HTML to PDF using wkhtmltopdf with minimal margins to reduce whitespace
	fmt.Printf("Generating PDF with wkhtmltopdf...\n")
	cmd := exec.Command(
		"wkhtmltopdf",
		"--page-size", "A4",
		"--margin-top", "0.1in",
		"--margin-right", "0.1in",
		"--margin-bottom", "0.1in",
		"--margin-left", "0.1in",
		"--encoding", "UTF-8",
		"--print-media-type",
		"--no-stop-slow-scripts",
		"--load-error-handling", "ignore",
		"--quiet",
		"--disable-smart-shrinking",
		"--zoom", "1.0",
		htmlPath,
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("wkhtmltopdf error: %v\n", err)
		return fmt.Errorf("wkhtmltopdf failed: %v, output: %s", err, string(output))
	}

	fmt.Printf("wkhtmltopdf PDF generation output: %s\n", string(output))

	// Clean up temporary HTML file
	os.Remove(htmlPath)

	fmt.Printf("PDF generation output: %s\n", string(output))

	// Check if PDF file was created and has content
	if fileInfo, err := os.Stat(outputPath); err != nil {
		return fmt.Errorf("PDF file not found after generation: %v", err)
	} else if fileInfo.Size() == 0 {
		return fmt.Errorf("PDF file is empty after generation")
	} else {
		fmt.Printf("PDF file created successfully: %s, size: %d bytes\n", outputPath, fileInfo.Size())
	}

	return nil
}
