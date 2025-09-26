package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"resumeai/services"
)

// JobController exposes endpoints for job discovery features.
type JobController struct {
	jobService *services.JobMatchingService
}

// NewJobController creates a controller with injected dependencies.
func NewJobController(jobService *services.JobMatchingService) *JobController {
	return &JobController{jobService: jobService}
}

type jobMatchRequest struct {
	ResumeData     services.ResumeData `json:"resumeData" binding:"required"`
	JobDescription string              `json:"jobDescription"`
	Limit          int                 `json:"limit"`
	ClientCountry  string              `json:"clientCountry"`
}

// MatchJobs aggregates job postings from configured providers and returns ranked matches.
func (c *JobController) MatchJobs(ctx *gin.Context) {
	var req jobMatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if strings.TrimSpace(req.ClientCountry) == "" {
		req.ClientCountry = detectCountryFromRequest(ctx)
	}

	result, err := c.jobService.FindMatches(ctx.Request.Context(), req.ResumeData, req.JobDescription, req.Limit, req.ClientCountry)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch job matches"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func detectCountryFromRequest(ctx *gin.Context) string {
	candidateHeaders := []string{
		"CF-IPCountry",
		"Cloudfront-Viewer-Country",
		"X-Appengine-Country",
		"X-Country",
		"X-Country-Code",
		"X-Geo-Country",
	}
	for _, header := range candidateHeaders {
		if normalized := normalizeCountryHeader(ctx.GetHeader(header)); normalized != "" {
			return normalized
		}
	}
	if normalized := countryFromAcceptLanguage(ctx.GetHeader("Accept-Language")); normalized != "" {
		return normalized
	}
	return ""
}

func normalizeCountryHeader(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	switch lower {
	case "usa", "us", "united states", "united states of america", "en-us":
		return "us"
	case "canada", "ca", "en-ca", "fr-ca":
		return "ca"
	case "united kingdom", "uk", "great britain", "gb", "en-gb":
		return "uk"
	case "australia", "au", "en-au":
		return "au"
	case "germany", "de", "de-de":
		return "de"
	case "france", "fr", "fr-fr":
		return "fr"
	case "india", "in", "en-in":
		return "in"
	}
	if len(lower) == 2 {
		return lower
	}
	if len(lower) == 5 && lower[2] == '-' {
		return lower[3:]
	}
	return ""
}

func countryFromAcceptLanguage(header string) string {
	if strings.TrimSpace(header) == "" {
		return ""
	}
	tokens := strings.Split(header, ",")
	for _, token := range tokens {
		part := strings.TrimSpace(token)
		if part == "" {
			continue
		}
		if idx := strings.Index(part, ";"); idx >= 0 {
			part = part[:idx]
		}
		segments := strings.Split(part, "-")
		if len(segments) == 2 {
			if code := normalizeCountryHeader(segments[1]); code != "" {
				return code
			}
		}
	}
	return ""
}
