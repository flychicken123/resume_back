package handlers

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"resumeai/services"
	"resumeai/utils"
)

var experienceAgent *services.ExperienceAgent

// SetExperienceAgent injects the LangChain-powered experience parser.
func SetExperienceAgent(agent *services.ExperienceAgent) {
	experienceAgent = agent
}

type experienceParseRequest struct {
	Text string `json:"text" binding:"required"`
}

// ParseExperience analyzes free-form text using LangChain to extract structured
// experience entries (including per-role projects) that can be bound to the
// resume builder UI.
func ParseExperience(c *gin.Context) {
	if experienceAgent == nil {
		utils.InternalServerError(c, "Experience agent is not configured", nil)
		return
	}

	var req experienceParseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		utils.BadRequestError(c, "text is required", nil)
		return
	}

	result, err := experienceAgent.ExtractExperiences(c.Request.Context(), text)
	if err != nil {
		log.Printf("[experience-agent] failed to parse text=%q error=%v", sanitizeForLog(text), err)
		utils.InternalServerError(c, "Failed to extract experience details", err)
		return
	}

	log.Printf(
		"[experience-agent] text=%q -> experiences=%d",
		sanitizeForLog(text),
		len(result.Experiences),
	)

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}

