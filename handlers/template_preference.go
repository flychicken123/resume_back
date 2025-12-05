package handlers

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"resumeai/services"
	"resumeai/utils"
)

var templatePreferenceAgent *services.TemplatePreferenceAgent

// SetTemplatePreferenceAgent injects the LangChain-backed template preference agent.
func SetTemplatePreferenceAgent(agent *services.TemplatePreferenceAgent) {
	templatePreferenceAgent = agent
}

type templatePreferenceRequest struct {
	Text string `json:"text" binding:"required"`
}

// InferTemplatePreference analyzes natural language to choose a template + font size.
func InferTemplatePreference(c *gin.Context) {
	if templatePreferenceAgent == nil {
		utils.InternalServerError(c, "Template preference agent is not configured", nil)
		return
	}

	var req templatePreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		utils.BadRequestError(c, "text is required", nil)
		return
	}

	result, err := templatePreferenceAgent.InferPreference(c.Request.Context(), text)
	if err != nil {
		log.Printf("[template-preference] failed to infer preference text=%q error=%v", sanitizeForLog(text), err)
		utils.InternalServerError(c, "Failed to infer template preference", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"template_id":  result.TemplateID,
		"font_size_id": result.FontSizeID,
		"reason":       result.Reason,
	})
}

