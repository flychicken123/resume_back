package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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
	cmd := exec.Command("python3", "generate_resume.py", templateName, string(userDataJSON), outputPath)
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

	fmt.Printf("Received resume generation request: %+v\n", req)

	// Get HTML content from the request
	htmlContent := req.HtmlContent
	if htmlContent == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "HTML content is required"})
		return
	}

	// Create static directory if it doesn't exist
	saveDir := "./static"
	if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
		return
	}

	// Generate filename with timestamp
	filename := "resume_" + time.Now().Format("20060102150405") + ".pdf"
	filepath := saveDir + "/" + filename

	// Prepare user data for PDF generation
	userData := map[string]interface{}{
		"htmlContent": htmlContent,
	}

	// Generate PDF resume using Python
	err = generatePDFResumeWithPython("temp1", userData, filepath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// S3-only: attempt upload and return URL; otherwise return error
	s3svc, s3err := services.NewS3Service()
	if s3err != nil {
		fmt.Printf("S3 not configured or invalid: %v\n", s3err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage service unavailable"})
		return
	}

	key := "resumes/" + filename
	url, uploadErr := s3svc.UploadFile(filepath, key)
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
	return
}

func generatePDFResumeWithPython(templateName string, userData map[string]interface{}, outputPath string) error {
	// Convert userData to JSON
	userDataJSON, err := json.Marshal(userData)
	if err != nil {
		return fmt.Errorf("failed to marshal user data: %v", err)
	}

	// Run Python script with correct working directory
	cmd := exec.Command("python3", "generate_resume.py", templateName, string(userDataJSON), outputPath)
	cmd.Dir = "." // Set working directory to current directory where templates are located

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("python script failed: %v, output: %s", err, string(output))
	}

	fmt.Printf("Python script output: %s\n", string(output))
	return nil
}
