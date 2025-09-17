package controllers

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"resumeai/models"
	"resumeai/services"
)

type ResumeController struct {
	resumeHistoryModel *models.ResumeHistoryModel
	resumeService      *services.ResumeService
}

func NewResumeController(resumeHistoryModel *models.ResumeHistoryModel, resumeService *services.ResumeService) *ResumeController {
	return &ResumeController{
		resumeHistoryModel: resumeHistoryModel,
		resumeService:      resumeService,
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

	// Convert historyID to int (you might want to add proper validation)
	var id int
	if _, err := fmt.Sscanf(historyID, "%d", &id); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid history ID"})
		return
	}

	err := c.resumeHistoryModel.DeleteByID(id, userID.(int))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete resume from history"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Resume deleted from history",
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
		"success": true,
		"downloadUrl": presignedURL,
		"filename": filename,
	})
}
