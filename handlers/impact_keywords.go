package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"resumeai/services"
	"resumeai/utils"
)

var impactKeywordsAgent *services.ImpactKeywordsAgent

// SetImpactKeywordsAgent injects the LangChain-powered impact keywords extractor.
func SetImpactKeywordsAgent(agent *services.ImpactKeywordsAgent) {
	impactKeywordsAgent = agent
}

// ExtractImpactKeywords analyzes experience and project descriptions to identify
// impact-showing keywords that should be highlighted in the resume preview and PDF.
func ExtractImpactKeywords(c *gin.Context) {
	if impactKeywordsAgent == nil {
		utils.InternalServerError(c, "Impact keywords agent is not configured", nil)
		return
	}

	var req services.ImpactKeywordsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	if len(req.Experiences) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"data": services.ImpactKeywordsResult{
				Experiences: make(map[string]services.ExperienceImpactKeywords),
			},
		})
		return
	}

	result, err := impactKeywordsAgent.ExtractImpactKeywords(c.Request.Context(), req)
	if err != nil {
		log.Printf("[impact-keywords-agent] failed to extract keywords: %v", err)
		utils.InternalServerError(c, "Failed to extract impact keywords", err)
		return
	}

	log.Printf(
		"[impact-keywords-agent] processed %d experiences",
		len(req.Experiences),
	)

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}
