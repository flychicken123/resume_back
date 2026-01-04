package handlers

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"resumeai/services"
	"resumeai/utils"
)

var personalInfoAgent *services.PersonalInfoAgent

// SetPersonalInfoAgent injects the LangChain-powered personal info parser.
func SetPersonalInfoAgent(agent *services.PersonalInfoAgent) {
	personalInfoAgent = agent
}

type personalInfoRequest struct {
	Text     string                         `json:"text" binding:"required"`
	Existing *services.PersonalInfoResult   `json:"existing,omitempty"`
}

// ParsePersonalInfo analyzes free-form text using LangChain to extract personal details.
func ParsePersonalInfo(c *gin.Context) {
	if personalInfoAgent == nil {
		utils.InternalServerError(c, "Personal info agent is not configured", nil)
		return
	}

	var req personalInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	// Use existing data if provided, otherwise start fresh
	existing := services.PersonalInfoResult{}
	if req.Existing != nil {
		existing = *req.Existing
	}

	result, err := personalInfoAgent.ExtractPersonalInfoWithContext(c.Request.Context(), req.Text, existing)
	if err != nil {
		log.Printf("[personal-info] failed to parse text=%q error=%v", sanitizeForLog(req.Text), err)
		utils.InternalServerError(c, "Failed to extract personal details", err)
		return
	}

	log.Printf(
		"[personal-info] text=%q -> name=%q email=%q phone=%q summary=%q",
		sanitizeForLog(req.Text),
		result.Name,
		result.Email,
		result.Phone,
		result.Summary,
	)

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}

func sanitizeForLog(input string) string {
	const max = 500
	trimmed := strings.TrimSpace(input)
	if len(trimmed) > max {
		return trimmed[:max] + "…"
	}
	return trimmed
}
