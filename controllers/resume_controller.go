package controllers

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"resumeai/models"
	"resumeai/services"
)

type ResumeController struct {
	resumeHistoryModel *models.ResumeHistoryModel
	resumeService      *services.ResumeService
	s3Service          *services.S3Service
}

func NewResumeController(resumeHistoryModel *models.ResumeHistoryModel, resumeService *services.ResumeService, s3Service *services.S3Service) *ResumeController {
	return &ResumeController{
		resumeHistoryModel: resumeHistoryModel,
		resumeService:      resumeService,
		s3Service:          s3Service,
	}
}

func (c *ResumeController) GetHistory(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	histories, err := c.resumeHistoryModel.GetByUserID(userID.(int))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch resume history"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"history": histories,
	})
}

func (c *ResumeController) DeleteHistory(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	historyID := ctx.Param("id")
	if historyID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "History ID is required"})
		return
	}

	// Convert historyID to int
	var id int
	if _, err := fmt.Sscanf(historyID, "%d", &id); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid history ID"})
		return
	}

	// First get the S3 path before deleting from database
	histories, err := c.resumeHistoryModel.GetByUserID(userID.(int))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch resume history"})
		return
	}

	var s3Path string
	for _, history := range histories {
		if history.ID == id {
			s3Path = history.S3Path
			break
		}
	}

	// Delete from database
	err = c.resumeHistoryModel.DeleteByID(id, userID.(int))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete resume from history"})
		return
	}

	// Delete from S3 if s3Service is available and path exists
	if c.s3Service != nil && s3Path != "" {
		// Extract filename from S3 path
		// S3 path format: https://bucket.s3.region.amazonaws.com/filename
		// We need just the filename
		parts := []string{}
		for i := len(s3Path) - 1; i >= 0; i-- {
			if s3Path[i] == '/' {
				parts = append(parts, s3Path[i+1:])
				break
			}
		}

		if len(parts) > 0 {
			fileName := parts[0]
			if err := c.s3Service.DeleteFile(fileName); err != nil {
				// Log error but don't fail the request
				fmt.Printf("Warning: Failed to delete file from S3: %v\n", err)
			}
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Resume deleted from history and storage",
	})
}

func (c *ResumeController) RenameResume(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	historyID := ctx.Param("id")
	if historyID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "History ID is required"})
		return
	}

	// Convert historyID to int
	var id int
	if _, err := fmt.Sscanf(historyID, "%d", &id); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid history ID"})
		return
	}

	// Parse request body
	var req struct {
		ResumeName string `json:"resume_name" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Resume name is required"})
		return
	}

	// Update the resume name
	err := c.resumeHistoryModel.UpdateResumeName(id, userID.(int), req.ResumeName)
	if err != nil {
		if err == sql.ErrNoRows {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Resume not found or access denied"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to rename resume"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Resume renamed successfully",
	})
}

func sanitizeFilename(name string) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "uploaded.pdf"
	}
	base := filepath.Base(clean)
	base = strings.ReplaceAll(base, " ", "_")
	base = strings.ReplaceAll(base, "..", "")
	if !strings.HasSuffix(strings.ToLower(base), ".pdf") {
		base = base + ".pdf"
	}
	return base
}

func (c *ResumeController) UploadHistoryPDF(ctx *gin.Context) {
	userIDVal, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	emailVal, hasEmail := ctx.Get("user_email")
	email := ""
	if hasEmail {
		email = strings.ToLower(fmt.Sprint(emailVal))
	}
	if email != "harwtalk@gmail.com" {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Upload is currently limited to administrators."})
		return
	}
	file, err := ctx.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "PDF file is required"})
		return
	}
	if strings.ToLower(filepath.Ext(file.Filename)) != ".pdf" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Only PDF files are allowed"})
		return
	}
	src, err := file.Open()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Unable to read uploaded file"})
		return
	}
	defer src.Close()

	tempFile, err := os.CreateTemp("", "resume-history-upload-*.pdf")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to prepare file upload"})
		return
	}
	defer func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}()

	if _, err := io.Copy(tempFile, src); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to copy uploaded file"})
		return
	}
	if err := tempFile.Close(); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize uploaded file"})
		return
	}

	if c.resumeService == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Resume service unavailable"})
		return
	}

	filename := c.resumeService.GenerateUniqueFilename(".pdf")
	s3URL, err := c.resumeService.UploadPDF(tempFile.Name(), filename)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store PDF"})
		return
	}

	displayName := strings.TrimSpace(file.Filename)
	if displayName == "" {
		displayName = fmt.Sprintf("Uploaded Resume %s", time.Now().Format("Jan 2 2006 15:04"))
	}
	record, err := c.resumeHistoryModel.Create(userIDVal.(int), displayName, s3URL)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record resume history"})
		return
	}
	_ = c.resumeHistoryModel.CleanupOldResumes(userIDVal.(int), 3)

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"history": record,
	})
}

func (c *ResumeController) DownloadResume(ctx *gin.Context) {
	filename := ctx.Param("filename")
	if filename == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Filename is required"})
		return
	}

	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Generate presigned URL for download
	presignedURL, err := c.resumeService.GeneratePresignedURL(filename)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Resume not found"})
		return
	}

	// Record download in history (optional, don't fail the download if this fails)
	err = c.resumeService.RecordDownload(userID.(int), filename, presignedURL)
	if err != nil {
		// Log the error but don't fail the download
		fmt.Printf("Failed to record download: %v\n", err)
	}

	// Return the presigned URL as JSON instead of redirecting
	// This avoids CORS issues when the frontend tries to follow the redirect
	ctx.JSON(http.StatusOK, gin.H{
		"success":     true,
		"downloadUrl": presignedURL,
		"filename":    filename,
	})
}
