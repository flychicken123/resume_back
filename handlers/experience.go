package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"resumeai/services"
	"resumeai/utils"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

const experienceBatchConcurrency = 4

var (
	optimizeExperienceForBatch       = services.OptimizeExperienceWithReview
	improveExperienceGrammarForBatch = services.ImproveExperienceGrammarWithReview
	optimizeExperiencesBatchFast     = services.OptimizeExperiencesBatchFast
)

type ExperienceOptimizationRequest struct {
	JobDescription    string                                 `json:"jobDescription" binding:"required"`
	UserExperience    string                                 `json:"userExperience" binding:"required"`
	MatchedSkills     []string                               `json:"matchedSkills,omitempty"`
	MissingSkills     []string                               `json:"missingSkills,omitempty"`
	ExperienceContext services.ExperienceOptimizationContext `json:"experienceContext,omitempty"`
}

type ExperienceOptimizationResponse struct {
	OptimizedExperience string `json:"optimizedExperience"`
	Message             string `json:"message"`
	ReviewStatus        string `json:"reviewStatus,omitempty"`
	ReviewReason        string `json:"reviewReason,omitempty"`
}

type ExperienceBatchOptimizationRequest struct {
	JobDescription string                       `json:"jobDescription,omitempty"`
	Experiences    []ExperienceBatchItemRequest `json:"experiences" binding:"required"`
	MatchedSkills  []string                     `json:"matchedSkills,omitempty"`
	MissingSkills  []string                     `json:"missingSkills,omitempty"`
}

type ExperienceBatchItemRequest struct {
	Index             int                                    `json:"index"`
	UserExperience    string                                 `json:"userExperience"`
	MatchedSkills     []string                               `json:"matchedSkills,omitempty"`
	MissingSkills     []string                               `json:"missingSkills,omitempty"`
	ExperienceContext services.ExperienceOptimizationContext `json:"experienceContext,omitempty"`
}

type ExperienceBatchOptimizationResponse struct {
	Results []ExperienceBatchItemResponse `json:"results"`
	Message string                        `json:"message"`
}

type ExperienceBatchItemResponse struct {
	Index               int    `json:"index"`
	Status              string `json:"status"`
	OptimizedExperience string `json:"optimizedExperience,omitempty"`
	Error               string `json:"error,omitempty"`
	ReviewStatus        string `json:"reviewStatus,omitempty"`
	ReviewReason        string `json:"reviewReason,omitempty"`
}

func OptimizeExperience(c *gin.Context) {
	var req ExperienceOptimizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	outcome, err := services.OptimizeExperienceWithReview(c.Request.Context(), services.ExperienceOptimizationInput{
		JobDescription: req.JobDescription,
		UserExperience: req.UserExperience,
		MatchedSkills:  req.MatchedSkills,
		MissingSkills:  req.MissingSkills,
		Context:        req.ExperienceContext,
	})
	if err != nil {
		utils.InternalServerError(c, "Failed to optimize experience", err)
		return
	}

	// Clean up the AI response to remove asterisks and format properly
	cleanedExperience := cleanupAIResponse(outcome.OptimizedExperience)

	response := ExperienceOptimizationResponse{
		OptimizedExperience: cleanedExperience,
		Message:             "Experience optimized successfully based on job description.",
		ReviewStatus:        outcome.ReviewStatus,
		ReviewReason:        outcome.ReviewReason,
	}

	utils.SuccessResponse(c, http.StatusOK, "Experience optimized successfully", response)
}

func OptimizeExperienceBatch(c *gin.Context) {
	var req ExperienceBatchOptimizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}
	if len(req.Experiences) == 0 {
		utils.ValidationError(c, fmt.Errorf("experiences is required"))
		return
	}

	jobDescription := strings.TrimSpace(req.JobDescription)
	ctx := c.Request.Context()
	results := make([]ExperienceBatchItemResponse, len(req.Experiences))
	batchItems := make([]services.ExperienceOptimizationBatchItem, 0, len(req.Experiences))

	for i, item := range req.Experiences {
		result := ExperienceBatchItemResponse{Index: item.Index}
		userExperience := strings.TrimSpace(item.UserExperience)
		if userExperience == "" {
			result.Status = "skipped"
			result.Error = "missing userExperience"
			results[i] = result
			continue
		}

		matchedSkills := req.MatchedSkills
		if len(item.MatchedSkills) > 0 {
			matchedSkills = item.MatchedSkills
		}
		missingSkills := req.MissingSkills
		if len(item.MissingSkills) > 0 {
			missingSkills = item.MissingSkills
		}

		input := services.ExperienceOptimizationInput{
			JobDescription: jobDescription,
			UserExperience: userExperience,
			MatchedSkills:  matchedSkills,
			MissingSkills:  missingSkills,
			Context:        item.ExperienceContext,
		}

		batchItems = append(batchItems, services.ExperienceOptimizationBatchItem{
			Position: i,
			Index:    item.Index,
			Input:    input,
		})
	}

	if len(batchItems) > 0 {
		if err := processExperienceBatchFast(ctx, jobDescription, batchItems, results); err != nil {
			processExperienceBatchIndividually(ctx, jobDescription, batchItems, results)
		}
	}

	response := ExperienceBatchOptimizationResponse{
		Results: results,
		Message: "Experience batch processed successfully.",
	}
	utils.SuccessResponse(c, http.StatusOK, "Experience batch processed successfully", response)
}

func processExperienceBatchFast(ctx context.Context, jobDescription string, batchItems []services.ExperienceOptimizationBatchItem, results []ExperienceBatchItemResponse) error {
	outcomes, err := optimizeExperiencesBatchFast(ctx, batchItems)
	if err != nil {
		return err
	}

	outcomeByPosition := make(map[int]services.ExperienceOptimizationBatchOutcome, len(outcomes))
	for _, outcome := range outcomes {
		outcomeByPosition[outcome.Position] = outcome
	}

	status := "improved"
	if jobDescription != "" {
		status = "optimized"
	}

	for _, item := range batchItems {
		result := ExperienceBatchItemResponse{Index: item.Index}
		outcome, ok := outcomeByPosition[item.Position]
		if !ok || strings.TrimSpace(outcome.OptimizedExperience) == "" {
			result.Status = "failed"
			result.Error = "missing optimized experience in batch response"
			results[item.Position] = result
			continue
		}

		result.Status = status
		result.OptimizedExperience = cleanupAIResponse(outcome.OptimizedExperience)
		result.ReviewStatus = outcome.ReviewStatus
		result.ReviewReason = outcome.ReviewReason
		results[item.Position] = result
	}

	return nil
}

func processExperienceBatchIndividually(ctx context.Context, jobDescription string, batchItems []services.ExperienceOptimizationBatchItem, results []ExperienceBatchItemResponse) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, experienceBatchConcurrency)

	for _, batchItem := range batchItems {
		batchItem := batchItem
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[batchItem.Position] = ExperienceBatchItemResponse{
					Index:  batchItem.Index,
					Status: "failed",
					Error:  ctx.Err().Error(),
				}
				return
			}

			result := ExperienceBatchItemResponse{Index: batchItem.Index}
			var (
				outcome services.ExperienceOptimizationOutcome
				err     error
			)
			if jobDescription != "" {
				outcome, err = optimizeExperienceForBatch(ctx, batchItem.Input)
				result.Status = "optimized"
			} else {
				outcome, err = improveExperienceGrammarForBatch(ctx, batchItem.Input)
				result.Status = "improved"
			}
			if err != nil {
				result.Status = "failed"
				result.Error = err.Error()
				results[batchItem.Position] = result
				return
			}

			result.OptimizedExperience = cleanupAIResponse(outcome.OptimizedExperience)
			result.ReviewStatus = outcome.ReviewStatus
			result.ReviewReason = outcome.ReviewReason
			results[batchItem.Position] = result
		}()
	}
	wg.Wait()
}

// cleanupAIResponse removes asterisks and cleans up the AI response
func cleanupAIResponse(text string) string {
	if jsonText := cleanupJSONTextResponse(text); jsonText != "" {
		text = jsonText
	}

	// Split into lines
	lines := strings.Split(text, "\n")
	var cleanedLines []string

	for _, line := range lines {
		// Trim whitespace
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Remove leading asterisk and any whitespace after it
		if strings.HasPrefix(line, "*") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		}

		// Remove bullet points if they exist
		if strings.HasPrefix(line, "•") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "•"))
		}
		if strings.HasPrefix(line, "-") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		}

		// Add the cleaned line
		if line != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}

	// Join lines back together with double newlines for proper spacing
	return strings.Join(cleanedLines, "\n\n")
}

func cleanupJSONTextResponse(text string) string {
	cleaned := strings.TrimSpace(text)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	if strings.HasPrefix(cleaned, `"`) && strings.HasSuffix(cleaned, `"`) {
		var value string
		if err := json.Unmarshal([]byte(cleaned), &value); err == nil {
			return strings.TrimSpace(value)
		}
		return decodeLooseQuotedText(cleaned)
	}

	if !strings.HasPrefix(cleaned, "[") || !strings.HasSuffix(cleaned, "]") {
		return ""
	}

	var bullets []string
	if err := json.Unmarshal([]byte(cleaned), &bullets); err != nil {
		return ""
	}

	out := make([]string, 0, len(bullets))
	for _, bullet := range bullets {
		bullet = strings.TrimSpace(bullet)
		if bullet != "" {
			out = append(out, bullet)
		}
	}
	return strings.Join(out, "\n")
}

func decodeLooseQuotedText(text string) string {
	if len(text) < 2 || !strings.HasPrefix(text, `"`) || !strings.HasSuffix(text, `"`) {
		return ""
	}

	inner := strings.TrimSpace(text[1 : len(text)-1])
	inner = strings.NewReplacer(
		`\r\n`, "\n",
		`\n`, "\n",
		`\r`, "\n",
		`\t`, "\t",
		`\"`, `"`,
		`\\`, `\`,
	).Replace(inner)
	return strings.TrimSpace(inner)
}

type ExperienceGrammarRequest struct {
	UserExperience    string                                 `json:"userExperience" binding:"required"`
	ExperienceContext services.ExperienceOptimizationContext `json:"experienceContext,omitempty"`
}

type ExperienceGrammarResponse struct {
	ImprovedExperience string `json:"improvedExperience"`
	Message            string `json:"message"`
	ReviewStatus       string `json:"reviewStatus,omitempty"`
	ReviewReason       string `json:"reviewReason,omitempty"`
}

func ImproveExperienceGrammar(c *gin.Context) {
	var req ExperienceGrammarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	outcome, err := services.ImproveExperienceGrammarWithReview(c.Request.Context(), services.ExperienceOptimizationInput{
		UserExperience: req.UserExperience,
		Context:        req.ExperienceContext,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Clean up the AI response
	cleanedExperience := cleanupAIResponse(outcome.OptimizedExperience)

	response := ExperienceGrammarResponse{
		ImprovedExperience: cleanedExperience,
		Message:            "Experience grammar and style improved successfully.",
		ReviewStatus:       outcome.ReviewStatus,
		ReviewReason:       outcome.ReviewReason,
	}

	c.JSON(http.StatusOK, response)
}
