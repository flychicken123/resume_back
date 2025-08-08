package handlers

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

type ParsedResume struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	Education  string `json:"education"`
	Experience string `json:"experience"`
	Skills     string `json:"skills"`
}

func ParseResume(c *gin.Context) {
	file, header, err := c.Request.FormFile("resume")
	if err != nil {
		c.JSON(400, gin.H{"error": "Could not get file"})
		return
	}
	defer file.Close()

	// Save file to temp location
	tempDir := os.TempDir()
	tempFile := filepath.Join(tempDir, header.Filename)
	out, err := os.Create(tempFile)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to save file"})
		return
	}
	defer out.Close()
	_, err = io.Copy(out, file)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to save file"})
		return
	}

	// Call Python script
	cmd := exec.Command("python", "parse_resume.py", tempFile)
	cmd.Dir = "../back" // Set working directory to where the script is
	output, err := cmd.Output()
	if err != nil {
		c.JSON(500, gin.H{"error": "Python script failed", "details": err.Error()})
		return
	}

	// Parse JSON output
	var parsed map[string]interface{}
	if err := json.Unmarshal(output, &parsed); err != nil {
		c.JSON(500, gin.H{"error": "Failed to parse script output"})
		return
	}

	// Clean up temp file
	os.Remove(tempFile)

	c.JSON(200, parsed)
}
