package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"resumeai/models"
)

type DataAnalysisController struct {
	jobPostingModel *models.JobPostingModel
}

func NewDataAnalysisController(jobPostingModel *models.JobPostingModel) *DataAnalysisController {
	return &DataAnalysisController{
		jobPostingModel: jobPostingModel,
	}
}

func (c *DataAnalysisController) GetJobCount(ctx *gin.Context) {
	keyword := strings.TrimSpace(ctx.Query("keyword"))
	if keyword == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "keyword query parameter is required"})
		return
	}

	count, err := c.jobPostingModel.CountActiveByKeyword(keyword)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve job count"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"keyword": keyword,
		"count":   count,
	})
}
