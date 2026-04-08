package handlers

import (
	"fmt"
	"net/http"
	"resumeai/services"
	"resumeai/utils"
	"strings"

	"github.com/gin-gonic/gin"
)

type EducationOptimizationRequest struct {
	Education         string `json:"education" binding:"required"`
	ExistingEducation string `json:"existingEducation,omitempty"` // For preserving/updating existing education
}

type EducationOptimizationResponse struct {
	OptimizedEducation string `json:"education"`
	Message            string `json:"message"`
}

type SummaryOptimizationRequest struct {
	Experience      string   `json:"experience"`
	Education       string   `json:"education"`
	Skills          []string `json:"skills"`
	ExistingSummary string   `json:"existingSummary,omitempty"` // For preserving/updating existing summary
	JobDescription  string   `json:"jobDescription,omitempty"`
	MatchedSkills   []string `json:"matchedSkills,omitempty"`
	MissingSkills   []string `json:"missingSkills,omitempty"`
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

	// Build prompt for education optimization (with existing education for preservation)
	prompt := services.BuildEducationOptimizationPrompt(req.Education, req.ExistingEducation)

	// Call AI service to generate optimized education
	optimizedEducation, err := services.CallGeminiWithTemperature(prompt, 0.3)
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

	// Build prompt for summary optimization with skill context
	prompt := services.BuildSummaryOptimizationPromptWithSkills(
		req.Experience, 
		req.Education, 
		req.Skills, 
		req.ExistingSummary,
		req.JobDescription,
		req.MatchedSkills,
		req.MissingSkills,
	)

	// Call AI service to generate optimized summary
	optimizedSummary, err := services.CallGeminiWithTemperature(prompt, 0.3) // Low temp for consistency
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Validate output to catch potential hallucinations
	originalContext := req.Experience + " " + req.Education + " " + strings.Join(req.Skills, " ")
	validatedSummary := services.ValidateAndCleanOutput(originalContext, optimizedSummary)
	
	// Clean up the AI response
	cleanedSummary := cleanupAIResponse(validatedSummary)

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
	improvedSummary, err := services.CallGeminiWithTemperature(prompt, 0.3)
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
	Position       string                 `json:"position"`
	LetterType     string                 `json:"letterType"`
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

	// Decide which letter style to generate based on requested type.
	letterType := strings.ToLower(strings.TrimSpace(req.LetterType))
	var prompt string
	switch letterType {
	case "third_party", "third-party", "recommendation", "3rd_party", "3rd":
		prompt = services.BuildRecommendationLetterPrompt(req.ResumeData, req.JobDescription, req.CompanyName, req.Position)
	default:
		// Default to first-party cover letter if unspecified or unknown.
		prompt = services.BuildCoverLetterPrompt(req.ResumeData, req.JobDescription, req.CompanyName)
	}

	// Call LangChain-based copilot agent to generate the letter
	coverLetter, err := runCopilotPrompt(c.Request.Context(), prompt)
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

// ProgressHint uses Gemini Flash to generate encouragement and a hint based on
// which resume sections are complete/missing.
func ProgressHint(c *gin.Context) {
	var req struct {
		Missing  []string `json:"missing"`
		Complete []string `json:"complete"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"hint":          "Your resume is looking great!",
			"encouragement": "You're doing amazing!",
		})
		return
	}

	completed := len(req.Complete)
	total := completed + len(req.Missing)

	prompt := "You are the user's supportive best friend helping them build their resume. " +
		"They have completed " + fmt.Sprintf("%d out of %d", completed, total) + " sections. " +
		"Completed: " + joinOrNone(req.Complete) + ". " +
		"Still missing: " + joinOrNone(req.Missing) + ". " +
		"Respond with EXACTLY two lines (no labels, no markdown, no numbering):\n" +
		"Line 1: A short warm, friendly encouragement about their progress (like a friend cheering them on, under 12 words). " +
		"Be genuine and specific to their progress level — celebrate if they're far along, be gentle and motivating if just starting.\n" +
		"Line 2: A brief suggestion for which section to work on next and why (under 20 words)."

	result, err := services.CallGeminiFlashWithTemperature(prompt, 0.7)
	if err != nil {
		fallbackEncouragement := "You're making great progress!"
		if completed == 0 {
			fallbackEncouragement = "Let's get started — you've got this!"
		} else if completed >= total {
			fallbackEncouragement = "Your resume is ready to shine!"
		}
		c.JSON(http.StatusOK, gin.H{
			"hint":          "Consider adding your " + req.Missing[0] + " next.",
			"encouragement": fallbackEncouragement,
		})
		return
	}

	// Split into two lines
	lines := splitLines(result)
	encouragement := lines[0]
	hint := ""
	if len(lines) > 1 {
		hint = lines[1]
	}
	if len(req.Missing) == 0 {
		hint = "Your resume is complete! Preview and download it."
	}

	c.JSON(http.StatusOK, gin.H{"hint": hint, "encouragement": encouragement})
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	if len(lines) == 0 {
		return []string{strings.TrimSpace(s)}
	}
	return lines
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	result := ""
	for i, item := range items {
		if i > 0 {
			result += ", "
		}
		result += item
	}
	return result
}
