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

	// Convert HTML to PDF using wkhtmltopdf configured to match preview layout
	// Use US Letter (8.5in x 11in) and zero page margins to rely on the
	// preview's own padding (box-sizing: border-box ensures width includes padding)
	cmd := exec.Command(
		"wkhtmltopdf",
		"--page-size", "Letter",
		"--margin-top", "0",
		"--margin-right", "0",
		"--margin-bottom", "0",
		"--margin-left", "0",
		// Slightly shrink to avoid spurious extra blank page due to rounding
		"--zoom", "0.98",
		// Normalize DPI for consistent sizing
		"--dpi", "96",
		"--disable-smart-shrinking",
		htmlPath,
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("wkhtmltopdf error: %v\n", err)
		// If wkhtmltopdf fails, copy HTML content as fallback
		err = os.WriteFile(outputPath, []byte(htmlContent), 0644)
		if err != nil {
			return fmt.Errorf("failed to write fallback HTML: %v", err)
		}
		fmt.Printf("Using HTML fallback due to wkhtmltopdf error: %v\n", err)
		return nil
	}

	// Clean up temporary HTML file
	os.Remove(htmlPath)

	fmt.Printf("PDF generation output: %s\n", string(output))
	return nil
}
