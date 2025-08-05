package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/gin-gonic/gin"
)

type ResumeRequest struct {
	Position   string   `json:"position"`
	Name       string   `json:"name"`
	Email      string   `json:"email"`
	Phone      string   `json:"phone"`
	Summary    string   `json:"summary"`
	Experience string   `json:"experience"`
	Education  string   `json:"education"`
	Skills     []string `json:"skills"`
	Format     string   `json:"format"`
}

func GenerateResume(c *gin.Context) {
	var req ResumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Prepare user data for template
	userData := map[string]interface{}{
		"name":       req.Name,
		"email":      req.Email,
		"phone":      req.Phone,
		"summary":    req.Summary,
		"experience": req.Experience,
		"education":  req.Education,
		"skills":     req.Skills,
		"position":   req.Position,
	}

	// Create static directory if it doesn't exist
	saveDir := "./static"
	os.MkdirAll(saveDir, os.ModePerm)

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
	if err := c.ShouldBindJSON(&req); err != nil {
		fmt.Printf("Error binding JSON: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fmt.Printf("Received PDF generation request: %+v\n", req)

	// Prepare user data for template
	userData := map[string]interface{}{
		"name":       req.Name,
		"email":      req.Email,
		"phone":      req.Phone,
		"summary":    req.Summary,
		"experience": req.Experience,
		"education":  req.Education,
		"skills":     req.Skills,
		"position":   req.Position,
	}

	// Create static directory if it doesn't exist
	saveDir := "./static"
	os.MkdirAll(saveDir, os.ModePerm)

	// Generate filename with timestamp
	filename := "resume_" + time.Now().Format("20060102150405") + ".pdf"
	filepath := saveDir + "/" + filename

	// Use default template if none selected
	templateFormat := req.Format
	if templateFormat == "" {
		templateFormat = "temp1"
	}

	// Generate PDF resume using Python
	err := generatePDFResumeWithPython(templateFormat, userData, filepath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "PDF resume generated successfully.",
		"filePath": "/static/" + filename,
	})
}

func generatePDFResumeWithPython(templateName string, userData map[string]interface{}, outputPath string) error {
	// Convert userData to JSON
	userDataJSON, err := json.Marshal(userData)
	if err != nil {
		return fmt.Errorf("failed to marshal user data: %v", err)
	}

	// Run Python script - it will automatically detect PDF output from the .pdf extension
	cmd := exec.Command("python", "generate_resume.py", templateName, string(userDataJSON), outputPath)
	cmd.Dir = "." // Set working directory to current directory where templates are located

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("python script failed: %v, output: %s", err, string(output))
	}

	fmt.Printf("Python script output: %s\n", string(output))
	return nil
}
