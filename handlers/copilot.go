package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"resumeai/services"
	"resumeai/utils"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
)

var copilotAgent *services.CopilotAgent

// SetCopilotAgent injects the LangChain-based copilot agent for reuse inside handlers.
func SetCopilotAgent(agent *services.CopilotAgent) {
	copilotAgent = agent
}

// CopilotExperienceInput captures raw experience text coming from the UI.
type CopilotExperienceInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// CopilotProjectInput captures project snippets to refine.
type CopilotProjectInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CopilotRequest is the payload accepted by the copilot endpoint.
type CopilotRequest struct {
	JobDescription string                   `json:"jobDescription"`
	Summary        string                   `json:"summary"`
	Education      string                   `json:"education"`
	Skills         []string                 `json:"skills"`
	Experience     []CopilotExperienceInput `json:"experience"`
	Projects       []CopilotProjectInput    `json:"projects"`
}

type CopilotExperienceSuggestion struct {
	Title     string `json:"title"`
	Original  string `json:"original"`
	Optimized string `json:"optimized"`
}

type CopilotProjectSuggestion struct {
	Name      string `json:"name"`
	Original  string `json:"original"`
	Optimized string `json:"optimized"`
}

type CopilotPlanResponse struct {
	SummarySuggestion   string                        `json:"summary,omitempty"`
	EducationSuggestion string                        `json:"education,omitempty"`
	Experience          []CopilotExperienceSuggestion `json:"experience,omitempty"`
	Projects            []CopilotProjectSuggestion    `json:"projects,omitempty"`
	AdvisorNotes        string                        `json:"advisorNotes,omitempty"`
	ActionPlan          []string                      `json:"actionPlan,omitempty"`
}

// JobFitExplanationRequest is used to ask the copilot for "Why it's a fit" reasons
// for a single job match and the current resume data.
type JobFitExplanationRequest struct {
	ResumeData map[string]interface{} `json:"resumeData" binding:"required"`
	Match      map[string]interface{} `json:"match" binding:"required"`
}

type JobFitExplanationResponse struct {
	Reasons []string `json:"reasons"`
	Message string   `json:"message"`
}

// ResumeCopilot orchestrates multiple AI passes to deliver a guided plan for the resume builder.
func ResumeCopilot(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		utils.UnauthorizedError(c, "User not authenticated")
		return
	}

	var req CopilotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	if isCopilotRequestEmpty(&req) {
		utils.BadRequestError(c, "Provide at least one resume section for the copilot to optimize", nil)
		return
	}

	plan, err := buildCopilotPlan(c.Request.Context(), &req)
	if err != nil {
		utils.InternalServerError(c, "Failed to generate copilot recommendations", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "AI copilot plan generated", plan)
}

// ExplainJobFit uses the LangChain copilot agent to generate a short list of
// natural-language reasons why a particular job match is a good fit for the
// current resume data.
func ExplainJobFit(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		utils.UnauthorizedError(c, "User not authenticated")
		return
	}

	var req JobFitExplanationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	prompt := services.BuildJobFitExplanationPrompt(req.ResumeData, req.Match)
	raw, err := runCopilotPrompt(c.Request.Context(), prompt)
	if err != nil {
		// Fall back to a simple generic reason if AI is unavailable.
		fallback := []string{"This role aligns with the experience and skills saved in your resume."}
		utils.SuccessResponse(c, http.StatusOK, "Job fit explanation fallback", JobFitExplanationResponse{
			Reasons: fallback,
			Message: "AI explanation unavailable; using fallback.",
		})
		return
	}

	reasons := parseJobFitReasons(raw)
	if len(reasons) == 0 {
		reasons = []string{"This role aligns with the experience and skills saved in your resume."}
	}

	utils.SuccessResponse(c, http.StatusOK, "Job fit explanation generated", JobFitExplanationResponse{
		Reasons: reasons,
		Message: "Job fit explanation generated.",
	})
}

func isCopilotRequestEmpty(req *CopilotRequest) bool {
	if req == nil {
		return true
	}
	if strings.TrimSpace(req.JobDescription) != "" {
		return false
	}
	if strings.TrimSpace(req.Summary) != "" {
		return false
	}
	if strings.TrimSpace(req.Education) != "" {
		return false
	}
	if len(req.Skills) > 0 {
		for _, skill := range req.Skills {
			if strings.TrimSpace(skill) != "" {
				return false
			}
		}
	}
	for _, exp := range req.Experience {
		if strings.TrimSpace(exp.Description) != "" {
			return false
		}
	}
	for _, proj := range req.Projects {
		if strings.TrimSpace(proj.Description) != "" {
			return false
		}
	}
	return true
}

func buildCopilotPlan(ctx context.Context, req *CopilotRequest) (*CopilotPlanResponse, error) {
	response := &CopilotPlanResponse{}

	jobDescription := strings.TrimSpace(req.JobDescription)

	var experienceForSummary []string
	var rawExperiences []map[string]interface{}

	for idx, exp := range req.Experience {
		original := strings.TrimSpace(exp.Description)
		if original == "" {
			continue
		}

		prompt := services.BuildExperienceOptimizationPrompt(jobDescription, original)
		optimized, err := runCopilotPrompt(ctx, prompt)
		if err != nil {
			return nil, err
		}

		cleaned := cleanupAIResponse(optimized)
		experienceForSummary = append(experienceForSummary, cleaned)

		title := strings.TrimSpace(exp.Title)
		if title == "" {
			title = generateFallbackTitle(idx)
		}

		response.Experience = append(response.Experience, CopilotExperienceSuggestion{
			Title:     title,
			Original:  original,
			Optimized: cleaned,
		})

		rawExperiences = append(rawExperiences, map[string]interface{}{
			"jobTitle":    title,
			"company":     "",
			"description": original,
		})
	}

	educationText := strings.TrimSpace(req.Education)
	if educationText != "" {
		prompt := services.BuildEducationOptimizationPrompt(educationText)
		result, err := runCopilotPrompt(ctx, prompt)
		if err != nil {
			return nil, err
		}
		response.EducationSuggestion = cleanupAIResponse(result)
	}

	var projectEntries []map[string]interface{}
	for idx, proj := range req.Projects {
		original := strings.TrimSpace(proj.Description)
		if original == "" {
			continue
		}

		prompt := services.BuildProjectOptimizationPrompt(jobDescription, original)
		optimized, err := runCopilotPrompt(ctx, prompt)
		if err != nil {
			return nil, err
		}

		cleaned := cleanupAIResponse(optimized)
		name := strings.TrimSpace(proj.Name)
		if name == "" {
			name = generateFallbackProjectName(idx)
		}
		response.Projects = append(response.Projects, CopilotProjectSuggestion{
			Name:      name,
			Original:  original,
			Optimized: cleaned,
		})

		projectEntries = append(projectEntries, map[string]interface{}{
			"name":        name,
			"description": original,
		})
	}

	summaryPrompt := ""
	trimmedSummary := strings.TrimSpace(req.Summary)
	switch {
	case trimmedSummary != "":
		summaryPrompt = services.BuildSummaryGrammarPrompt(trimmedSummary)
	case len(experienceForSummary) > 0 || educationText != "" || len(req.Skills) > 0:
		joinedExperience := strings.Join(experienceForSummary, "\n")
		if joinedExperience == "" {
			joinedExperience = gatherOriginalExperiences(req.Experience)
		}
		summaryPrompt = services.BuildSummaryOptimizationPrompt(joinedExperience, educationText, req.Skills)
	}

	if summaryPrompt != "" {
		result, err := runCopilotPrompt(ctx, summaryPrompt)
		if err != nil {
			return nil, err
		}
		response.SummarySuggestion = cleanupAIResponse(result)
	}

	resumeData := map[string]interface{}{}
	if response.SummarySuggestion != "" {
		resumeData["summary"] = response.SummarySuggestion
	} else if trimmedSummary != "" {
		resumeData["summary"] = trimmedSummary
	}

	if len(rawExperiences) > 0 {
		interfaceSlice := make([]interface{}, len(rawExperiences))
		for i, exp := range rawExperiences {
			interfaceSlice[i] = exp
		}
		resumeData["experiences"] = interfaceSlice
	}

	if educationText != "" {
		resumeData["education"] = []interface{}{
			map[string]interface{}{
				"degree": educationText,
				"school": "",
			},
		}
	}

	if len(req.Skills) > 0 {
		resumeData["skills"] = strings.Join(req.Skills, ", ")
	}

	if len(projectEntries) > 0 {
		resumeData["projects"] = projectEntries
	}

	if len(resumeData) > 0 {
		prompt := services.BuildResumeAdvicePrompt(resumeData, jobDescription)
		advice, err := runCopilotPrompt(ctx, prompt)
		if err != nil {
			return nil, err
		}
		response.AdvisorNotes = strings.TrimSpace(advice)
		response.ActionPlan = extractActionItems(response.AdvisorNotes)
	}

	return response, nil
}

func gatherOriginalExperiences(items []CopilotExperienceInput) string {
	var segments []string
	for _, exp := range items {
		if trimmed := strings.TrimSpace(exp.Description); trimmed != "" {
			segments = append(segments, trimmed)
		}
	}
	return strings.Join(segments, "\n")
}

func generateFallbackTitle(idx int) string {
	return "Experience #" + strconv.Itoa(idx+1)
}

func generateFallbackProjectName(idx int) string {
	return "Project #" + strconv.Itoa(idx+1)
}

func extractActionItems(advice string) []string {
	var items []string
	lines := strings.Split(advice, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isBulletLine(trimmed) {
			trimmed = strings.TrimSpace(trimBulletPrefix(trimmed))
			if trimmed != "" {
				items = append(items, trimmed)
			}
		}
	}

	if len(items) == 0 {
		paragraphs := strings.Split(advice, "\n\n")
		for _, para := range paragraphs {
			trimmed := strings.TrimSpace(para)
			if trimmed != "" {
				items = append(items, trimmed)
			}
			if len(items) >= 5 {
				break
			}
		}
	}

	return items
}

// parseJobFitReasons attempts to interpret the LLM output as either
// { "reasons": ["...", "..."] } JSON or a simple newline/bullet list.
func parseJobFitReasons(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	// Strip common markdown fences if present.
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	// First try JSON with the expected schema.
	var wrapper struct {
		Reasons []string `json:"reasons"`
	}
	if err := json.Unmarshal([]byte(trimmed), &wrapper); err == nil && len(wrapper.Reasons) > 0 {
		out := make([]string, 0, len(wrapper.Reasons))
		for _, r := range wrapper.Reasons {
			r = strings.TrimSpace(r)
			if r != "" {
				out = append(out, r)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	// Fallback: treat the response as plain text lines with optional bullets.
	var reasons []string
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Remove common "Reason X:" prefixes if present.
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "reason") && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			line = strings.TrimSpace(parts[1])
			if line == "" {
				continue
			}
		}
		line = trimBulletPrefix(line)
		line = strings.TrimSpace(line)
		if line != "" {
			reasons = append(reasons, line)
		}
	}

	return reasons
}

func isBulletLine(line string) bool {
	if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "\u2022") {
		return true
	}

	digits := 0
	for _, r := range line {
		if unicode.IsDigit(r) {
			digits++
			continue
		}
		if (r == '.' || r == ')' || r == ':') && digits > 0 {
			return true
		}
		break
	}

	return false
}

func runCopilotPrompt(ctx context.Context, prompt string) (string, error) {
	if copilotAgent == nil {
		return "", fmt.Errorf("copilot agent is not configured")
	}
	return copilotAgent.RunPrompt(ctx, prompt)
}

func trimBulletPrefix(value string) string {
	return strings.TrimLeftFunc(value, func(r rune) bool {
		if r == '-' || r == '*' || r == '\u2022' {
			return true
		}
		return unicode.IsSpace(r)
	})
}
