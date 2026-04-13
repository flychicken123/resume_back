package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"resumeai/models"
	"resumeai/services"
)

// GeneratePDFResumeHandler handles PDF generation with database integration
func GeneratePDFResumeHandler(db *sql.DB, resumeHistoryModel *models.ResumeHistoryModel, userModel *models.UserModel, resumeModel *models.ResumeModel) gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Println("GeneratePDFResumeHandler called")

		// Get authenticated user from context (set by AuthMiddleware)
		userID, exists := c.Get("user_id")
		userEmail, emailExists := c.Get("user_email")

		var userIDInt int
		var userEmailStr string

		if exists {
			userIDInt = userID.(int)
		}
		if emailExists {
			userEmailStr = userEmail.(string)
		}

		fmt.Printf("DEBUG: Authenticated user - ID: %d, Email: %s\n", userIDInt, userEmailStr)

		rawResumeData := strings.TrimSpace(c.PostForm("resumeData"))
		var resumeData json.RawMessage
		if rawResumeData != "" && json.Valid([]byte(rawResumeData)) {
			resumeData = json.RawMessage(rawResumeData)
		} else if rawResumeData != "" {
			fmt.Printf("resumeData payload is not valid JSON (len=%d)\n", len(rawResumeData))
		}

		// Expect a multipart form file field named 'html'
		file, err := c.FormFile("html")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'html' file field"})
			return
		}

		// Ensure output dirs exist
		saveDir := "./static"
		if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
			return
		}

		// Save uploaded HTML to disk
		htmlFilename := fmt.Sprintf("resume_%d.html", time.Now().UnixNano())
		htmlPath := filepath.Join(saveDir, htmlFilename)
		if err := c.SaveUploadedFile(file, htmlPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded HTML"})
			return
		}

		// Save the HTML to DB so the tailor endpoint can reuse the user's template
		if userIDInt > 0 && resumeModel != nil {
			if htmlBytes, readErr := os.ReadFile(htmlPath); readErr == nil {
				if saveErr := resumeModel.SaveLastHTML(userIDInt, string(htmlBytes)); saveErr != nil {
					fmt.Printf("Warning: failed to save last resume HTML for user %d: %v\n", userIDInt, saveErr)
				}
			}
		}

		// Prepare the target PDF path
		pdfFilename := strings.TrimSuffix(htmlFilename, filepath.Ext(htmlFilename)) + ".pdf"
		pdfPath := filepath.Join(saveDir, pdfFilename)

		// Ask Python to render this HTML file into the PDF
		userData := map[string]interface{}{
			"htmlContent": "",
			"htmlPath":    htmlPath,
		}
		if engine := strings.TrimSpace(c.PostForm("engine")); engine != "" {
			userData["engine"] = engine
		}
		if err := generatePDFResumeWithPython(defaultTemplateSlug, userData, pdfPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		saveInput := models.ResumeProfileSaveInput{
			Name:           c.PostForm("name"),
			Email:          c.PostForm("email"),
			Phone:          c.PostForm("phone"),
			SelectedFormat: c.PostForm("format"),
		}
		if err := persistResumeProfileFromJSON(c, resumeData, saveInput); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save resume data"})
			return
		}

		// Upload to S3
		s3svc, s3err := services.NewS3Service()
		if s3err != nil {
			fmt.Printf("S3 not configured or invalid: %v\n", s3err)
			// Continue without S3 - file is still available locally
			downloadURL := fmt.Sprintf("/static/%s", pdfFilename)

			// Save to resume history if we have user ID
			if userIDInt > 0 && resumeHistoryModel != nil {
				resumeName := fmt.Sprintf("Resume %s", time.Now().Format("2006-01-02 15:04"))
				s3Path := downloadURL // Use local path if S3 unavailable

				history, err := resumeHistoryModel.Create(userIDInt, resumeName, s3Path)
				if err != nil {
					fmt.Printf("Failed to save resume history: %v\n", err)
				} else {
					fmt.Printf("Saved resume history ID %d for user %d\n", history.ID, userIDInt)
				}
			}

			c.JSON(http.StatusOK, gin.H{
				"message":     "PDF generated (local storage)",
				"downloadURL": downloadURL,
				"filename":    pdfFilename,
				"userID":      userIDInt,
			})
			return
		}

		// Upload to S3
		key := "resumes/" + pdfFilename
		url, uploadErr := s3svc.UploadFile(pdfPath, key)
		if uploadErr != nil {
			localURL := fmt.Sprintf("/static/%s", pdfFilename)
			fmt.Printf("S3 upload failed: %v. Serving local file %s\n", uploadErr, localURL)
			c.JSON(http.StatusOK, gin.H{
				"message":       "PDF generated locally; cloud storage unavailable",
				"downloadURL":   localURL,
				"filename":      pdfFilename,
				"storage":       "local",
				"s3UploadError": uploadErr.Error(),
				"userID":        userIDInt,
			})
			return
		}

		// Save to resume history if we have user ID
		if userIDInt > 0 && resumeHistoryModel != nil {
			resumeName := fmt.Sprintf("Resume %s", time.Now().Format("2006-01-02 15:04"))

			history, err := resumeHistoryModel.Create(userIDInt, resumeName, key)
			if err != nil {
				fmt.Printf("Failed to save resume history: %v\n", err)
			} else {
				fmt.Printf("Successfully saved resume history ID %d for user %d\n", history.ID, userIDInt)

				// Clean up old resumes (keep only last 10)
				if cleanupErr := resumeHistoryModel.CleanupOldResumes(userIDInt, 10); cleanupErr != nil {
					fmt.Printf("Failed to cleanup old resumes: %v\n", cleanupErr)
				}
			}
		} else {
			fmt.Printf("Cannot save to resume history: userID=%d, hasModel=%v\n", userIDInt, resumeHistoryModel != nil)
		}

		// Generate presigned URL for response
		if presigned, preErr := s3svc.GeneratePresignedURL(key); preErr == nil {
			c.JSON(http.StatusOK, gin.H{
				"message":        "PDF generated and saved to history",
				"downloadURL":    presigned,
				"filename":       pdfFilename,
				"s3Path":         key,
				"userID":         userIDInt,
				"savedToHistory": userIDInt > 0,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":        "PDF generated and saved to history",
			"downloadURL":    url,
			"filename":       pdfFilename,
			"s3Path":         key,
			"userID":         userIDInt,
			"savedToHistory": userIDInt > 0,
		})
	}
}

// SaveHTMLHandler handles POST /api/resume/save-html
// Saves the user's current resume HTML (sent as a multipart 'html' file) so the
// Chrome extension can later use it as the tailoring template.
func SaveHTMLHandler(resumeModel *models.ResumeModel) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
			return
		}
		userIDInt := userID.(int)

		file, err := c.FormFile("html")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'html' file field"})
			return
		}

		f, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read HTML"})
			return
		}
		defer f.Close()

		buf := make([]byte, file.Size)
		if _, err := f.Read(buf); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read HTML content"})
			return
		}

		if err := resumeModel.SaveLastHTML(userIDInt, string(buf)); err != nil {
			fmt.Printf("SaveHTMLHandler: failed to save HTML for user %d: %v\n", userIDInt, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save resume HTML"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Resume template saved for plugin"})
	}
}
