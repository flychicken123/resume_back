package handlers

import (
	"database/sql"
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
func GeneratePDFResumeHandler(db *sql.DB, resumeHistoryModel *models.ResumeHistoryModel, userModel *models.UserModel) gin.HandlerFunc {
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

		// Prepare the target PDF path
		pdfFilename := strings.TrimSuffix(htmlFilename, filepath.Ext(htmlFilename)) + ".pdf"
		pdfPath := filepath.Join(saveDir, pdfFilename)

		// Ask Python to render this HTML file into the PDF
		userData := map[string]interface{}{
			"htmlContent": "",
			"htmlPath":    htmlPath,
		}
		if err := generatePDFResumeWithPython("temp1", userData, pdfPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
				"message": "PDF generated (local storage)", 
				"downloadURL": downloadURL,
				"filename": pdfFilename,
				"userID": userIDInt,
			})
			return
		}
		
		// Upload to S3
		key := "resumes/" + pdfFilename
		url, uploadErr := s3svc.UploadFile(pdfPath, key)
		if uploadErr != nil {
			fmt.Printf("S3 upload failed: %v\n", uploadErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload PDF to storage"})
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
				"message": "PDF generated and saved to history", 
				"downloadURL": presigned,
				"filename": pdfFilename,
				"s3Path": key,
				"userID": userIDInt,
				"savedToHistory": userIDInt > 0,
			})
			return
		}
		
		c.JSON(http.StatusOK, gin.H{
			"message": "PDF generated and saved to history", 
			"downloadURL": url,
			"filename": pdfFilename,
			"s3Path": key,
			"userID": userIDInt,
			"savedToHistory": userIDInt > 0,
		})
	}
}