package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"resumeai/services"
	"resumeai/utils"
)

var resumeModifyAgent *services.ResumeModifyAgent

// SetResumeModifyAgent injects the LangChain-powered resume modification agent.
func SetResumeModifyAgent(agent *services.ResumeModifyAgent) {
	resumeModifyAgent = agent
}

type resumeModifyRequest struct {
	Text         string                 `json:"text" binding:"required"`
	ExistingData map[string]interface{} `json:"existingData,omitempty"`
}

// AnalyzeResumeModification analyzes user input to detect resume modification intent.
func AnalyzeResumeModification(c *gin.Context) {
	if resumeModifyAgent == nil {
		utils.InternalServerError(c, "Resume modification agent is not configured", nil)
		return
	}

	var req resumeModifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	existingData := req.ExistingData
	if existingData == nil {
		existingData = make(map[string]interface{})
	}

	result, err := resumeModifyAgent.AnalyzeModification(c.Request.Context(), req.Text, existingData)
	if err != nil {
		utils.InternalServerError(c, "Failed to analyze modification request", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}
