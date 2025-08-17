package handlers

import (
	"net/http"
	"resumeai/services"
	"resumeai/utils"

	"github.com/gin-gonic/gin"
)

type EducationOptimizationRequest struct {
	Education string `json:"education" binding:"required"`
}

type EducationOptimizationResponse struct {
	OptimizedEducation string `json:"education"`
	Message            string `json:"message"`
}

type SummaryOptimizationRequest struct {
	Experience string   `json:"experience"`
	Education  string   `json:"education"`
	Skills     []string `json:"skills"`
}

type SummaryOptimizationResponse struct {
	Summary string `json:"summary"`
	Message string `json:"message"`
}

func OptimizeEducation(c *gin.Context) {
	var req EducationOptimizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	// Build prompt for education optimization
	prompt := services.BuildEducationOptimizationPrompt(req.Education)

	// Call AI service to generate optimized education
	optimizedEducation, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		utils.InternalServerError(c, "Failed to optimize education", err)
		return
	}

	// Clean up the AI response
	cleanedEducation := cleanupAIResponse(optimizedEducation)

	response := EducationOptimizationResponse{
		OptimizedEducation: cleanedEducation,
		Message:            "Education optimized successfully.",
	}

	utils.SuccessResponse(c, http.StatusOK, "Education optimized successfully", response)
}

func OptimizeSummary(c *gin.Context) {
	var req SummaryOptimizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build prompt for summary optimization
	prompt := services.BuildSummaryOptimizationPrompt(req.Experience, req.Education, req.Skills)

	// Call AI service to generate optimized summary
	optimizedSummary, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Clean up the AI response
	cleanedSummary := cleanupAIResponse(optimizedSummary)

	response := SummaryOptimizationResponse{
		Summary: cleanedSummary,
		Message: "Summary optimized successfully.",
	}

	c.JSON(http.StatusOK, response)
}

type SummaryGrammarRequest struct {
	Summary string `json:"summary" binding:"required"`
}

type SummaryGrammarResponse struct {
	ImprovedSummary string `json:"improvedSummary"`
	Message         string `json:"message"`
}

func ImproveSummaryGrammar(c *gin.Context) {
	var req SummaryGrammarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build prompt for grammar improvement
	prompt := services.BuildSummaryGrammarPrompt(req.Summary)

	// Call AI service to improve grammar
	improvedSummary, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Clean up the AI response
	cleanedSummary := cleanupAIResponse(improvedSummary)

	response := SummaryGrammarResponse{
		ImprovedSummary: cleanedSummary,
		Message:         "Summary grammar and style improved successfully.",
	}

	c.JSON(http.StatusOK, response)
}

type ResumeAdviceRequest struct {
	ResumeData     map[string]interface{} `json:"resumeData" binding:"required"`
	JobDescription string                 `json:"jobDescription"`
}

type ResumeAdviceResponse struct {
	Advice  string `json:"advice"`
	Message string `json:"message"`
}

func AnalyzeResumeAdvice(c *gin.Context) {
	var req ResumeAdviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build prompt for resume analysis
	prompt := services.BuildResumeAdvicePrompt(req.ResumeData, req.JobDescription)

	// Call AI service to analyze resume
	advice, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := ResumeAdviceResponse{
		Advice:  advice,
		Message: "Resume analysis completed successfully.",
	}

	c.JSON(http.StatusOK, response)
}

type CoverLetterRequest struct {
	ResumeData     map[string]interface{} `json:"resumeData" binding:"required"`
	JobDescription string                 `json:"jobDescription"`
	CompanyName    string                 `json:"companyName"`
}

type CoverLetterResponse struct {
	CoverLetter string `json:"coverLetter"`
	Message     string `json:"message"`
}

func GenerateCoverLetter(c *gin.Context) {
	var req CoverLetterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build prompt for cover letter generation
	prompt := services.BuildCoverLetterPrompt(req.ResumeData, req.JobDescription, req.CompanyName)

	// Call AI service to generate cover letter
	coverLetter, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := CoverLetterResponse{
		CoverLetter: coverLetter,
		Message:     "Cover letter generated successfully.",
	}

	c.JSON(http.StatusOK, response)
}
