package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ExperienceOptimizationContext carries non-description resume metadata that
// helps the model rewrite accurately without inventing facts.
type ExperienceOptimizationContext struct {
	JobTitle         string   `json:"jobTitle,omitempty"`
	Company          string   `json:"company,omitempty"`
	City             string   `json:"city,omitempty"`
	State            string   `json:"state,omitempty"`
	Location         string   `json:"location,omitempty"`
	Remote           bool     `json:"remote,omitempty"`
	StartDate        string   `json:"startDate,omitempty"`
	EndDate          string   `json:"endDate,omitempty"`
	CurrentlyWorking bool     `json:"currentlyWorking,omitempty"`
	Seniority        string   `json:"seniority,omitempty"`
	ResumeSkills     []string `json:"resumeSkills,omitempty"`
	TargetRole       string   `json:"targetRole,omitempty"`
	TargetCompany    string   `json:"targetCompany,omitempty"`
}

type ExperienceOptimizationInput struct {
	JobDescription string
	UserExperience string
	MatchedSkills  []string
	MissingSkills  []string
	Context        ExperienceOptimizationContext
}

type ExperienceOptimizationOutcome struct {
	OptimizedExperience string
	ReviewStatus        string
	ReviewReason        string
}

type ExperienceOptimizationReview struct {
	Approved          bool   `json:"approved"`
	RevisedExperience string `json:"revisedExperience"`
	Reason            string `json:"reason"`
}

func BuildContextualExperienceOptimizationPrompt(input ExperienceOptimizationInput) string {
	skillContext := buildExperienceSkillContext(input.MatchedSkills, input.MissingSkills, input.Context.ResumeSkills)
	contextBlock := buildExperienceContextBlock(input.Context)

	return fmt.Sprintf(`You are an expert resume writer. Enhance this work experience description to better align with the job requirements.

CRITICAL INTEGRITY RULES (NEVER VIOLATE):
1. NEVER invent accomplishments, metrics, technologies, responsibilities, employers, titles, dates, or seniority not supported by the provided resume context.
2. NEVER fabricate numbers, percentages, dollar amounts, team size, scale, tools, or outcomes.
3. NEVER add missing skills as if the candidate has them.
4. If the original lacks metrics, keep statements qualitative; do NOT make up numbers.
5. Use job title, company, dates, location, seniority, and resume skills only as context for tone, tense, relevance, and factual boundaries.
6. Preserve the candidate's authentic experience. Only rephrase, reorganize, and highlight.

Target Job Description:
%s
%s%s
User's Original Experience Description:
%s

ENHANCEMENT GUIDELINES:
1. Start each statement with a strong action verb.
2. Emphasize matched skills where they genuinely appear in the original experience or resume context.
3. Use target-job language only where it naturally fits the user's actual background.
4. Improve clarity, specificity, and business impact while keeping the same meaning.
5. Remove weak phrases like "Responsible for", "Helped with", and "Worked on".

IMPORTANT: Return ONLY the enhanced description text. Each achievement on a new line. No bullet points, headers, explanations, JSON keys, or markdown.`, input.JobDescription, contextBlock, skillContext, input.UserExperience)
}

func OptimizeExperienceWithReview(ctx context.Context, input ExperienceOptimizationInput) (ExperienceOptimizationOutcome, error) {
	prompt := BuildContextualExperienceOptimizationPrompt(input)
	candidate, err := CallGeminiWithTemperatureContext(ctx, prompt, 0.3)
	if err != nil {
		return ExperienceOptimizationOutcome{}, err
	}
	if strings.TrimSpace(candidate) == "" {
		return ExperienceOptimizationOutcome{}, fmt.Errorf("empty experience optimization response")
	}

	review, err := ReviewExperienceOptimization(ctx, input, candidate)
	if err != nil {
		return ExperienceOptimizationOutcome{}, err
	}

	final, status, err := applyExperienceOptimizationReview(candidate, review)
	if err != nil {
		return ExperienceOptimizationOutcome{}, err
	}

	return ExperienceOptimizationOutcome{
		OptimizedExperience: ValidateAndCleanOutput(input.UserExperience, final),
		ReviewStatus:        status,
		ReviewReason:        strings.TrimSpace(review.Reason),
	}, nil
}

func BuildContextualExperienceGrammarPrompt(input ExperienceOptimizationInput) string {
	contextBlock := buildExperienceContextBlock(input.Context)
	skillContext := buildExperienceSkillContext(nil, nil, input.Context.ResumeSkills)

	return fmt.Sprintf(`You are an expert resume writer and editor.

Your task is to improve the grammar, clarity, professional tone, action orientation, and impact of this work experience description.

CRITICAL INTEGRITY RULES:
1. Preserve the factual content of the original description.
2. Do NOT invent new duties, technologies, metrics, companies, titles, dates, seniority, or outcomes.
3. Use the provided role, employer, dates, location, seniority, and skills only as context for accurate wording and tense.
4. Only add metrics when they are explicitly present or clearly implied by the original text.

%s%sOriginal Experience Description:
%s

Improve this text by:
1. Correcting grammar and sentence structure.
2. Using stronger action verbs.
3. Removing redundancy and vague phrasing.
4. Making impact clearer without fabricating claims.
5. Keeping the same meaning and authentic scope.

IMPORTANT: Return ONLY the improved description text. No job title, company name, dates, headers, explanations, JSON keys, markdown, or bullet symbols.`, contextBlock, skillContext, input.UserExperience)
}

func ImproveExperienceGrammarWithReview(ctx context.Context, input ExperienceOptimizationInput) (ExperienceOptimizationOutcome, error) {
	prompt := BuildContextualExperienceGrammarPrompt(input)
	candidate, err := CallGeminiWithTemperatureContext(ctx, prompt, 0.3)
	if err != nil {
		return ExperienceOptimizationOutcome{}, err
	}
	if strings.TrimSpace(candidate) == "" {
		return ExperienceOptimizationOutcome{}, fmt.Errorf("empty experience grammar response")
	}

	review, err := ReviewExperienceOptimization(ctx, input, candidate)
	if err != nil {
		return ExperienceOptimizationOutcome{}, err
	}

	final, status, err := applyExperienceOptimizationReview(candidate, review)
	if err != nil {
		return ExperienceOptimizationOutcome{}, err
	}

	return ExperienceOptimizationOutcome{
		OptimizedExperience: ValidateAndCleanOutput(input.UserExperience, final),
		ReviewStatus:        status,
		ReviewReason:        strings.TrimSpace(review.Reason),
	}, nil
}

func ReviewExperienceOptimization(ctx context.Context, input ExperienceOptimizationInput, candidate string) (ExperienceOptimizationReview, error) {
	prompt := BuildExperienceOptimizationReviewPrompt(input, candidate)
	raw, err := CallGeminiWithTemperatureContext(ctx, prompt, 0.0)
	if err != nil {
		return ExperienceOptimizationReview{}, err
	}
	return ParseExperienceOptimizationReview(raw)
}

func BuildExperienceOptimizationReviewPrompt(input ExperienceOptimizationInput, candidate string) string {
	contextBlock := buildExperienceContextBlock(input.Context)
	skillContext := buildExperienceSkillContext(input.MatchedSkills, input.MissingSkills, input.Context.ResumeSkills)
	targetJob := strings.TrimSpace(input.JobDescription)
	if targetJob == "" {
		targetJob = "(none provided; review against the original experience and resume context only)"
	}

	return fmt.Sprintf(`You are a strict resume quality reviewer. Review the rewritten experience description against the original facts and context.

Reject or revise the candidate if it:
1. Adds unsupported accomplishments, metrics, technologies, duties, companies, titles, dates, seniority, or outcomes.
2. Claims a missing skill as existing experience.
3. Is less clear, less specific, or less resume-ready than the original.
4. Overuses generic buzzwords without concrete action or impact.
5. Fails to align with the target job when truthful alignment is possible.

If the candidate is good as-is, return approved=true and revisedExperience="".
If it needs fixes and can be repaired, return approved=false and put the complete corrected description in revisedExperience.
If it cannot be repaired without inventing facts, return approved=false and revisedExperience="".

Target Job Description:
%s
%s%s
Original Experience Description:
%s

Candidate Rewritten Description:
%s

Return JSON only with this shape:
{"approved": true, "revisedExperience": "", "reason": "brief reason"}`, targetJob, contextBlock, skillContext, input.UserExperience, candidate)
}

func ParseExperienceOptimizationReview(raw string) (ExperienceOptimizationReview, error) {
	var parsed struct {
		Approved               bool   `json:"approved"`
		RevisedExperience      string `json:"revisedExperience"`
		RevisedExperienceSnake string `json:"revised_experience"`
		Reason                 string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &parsed); err != nil {
		return ExperienceOptimizationReview{}, fmt.Errorf("parse experience optimization review JSON: %w", err)
	}

	revised := strings.TrimSpace(parsed.RevisedExperience)
	if revised == "" {
		revised = strings.TrimSpace(parsed.RevisedExperienceSnake)
	}

	return ExperienceOptimizationReview{
		Approved:          parsed.Approved,
		RevisedExperience: revised,
		Reason:            strings.TrimSpace(parsed.Reason),
	}, nil
}

func applyExperienceOptimizationReview(candidate string, review ExperienceOptimizationReview) (string, string, error) {
	revised := strings.TrimSpace(review.RevisedExperience)
	if revised != "" {
		return revised, "revised", nil
	}

	cleanCandidate := strings.TrimSpace(candidate)
	if review.Approved && cleanCandidate != "" {
		return cleanCandidate, "approved", nil
	}

	reason := strings.TrimSpace(review.Reason)
	if reason == "" {
		reason = "reviewer rejected the rewritten experience"
	}
	return "", "rejected", errors.New(reason)
}

func buildExperienceSkillContext(matchedSkills, missingSkills, resumeSkills []string) string {
	var sb strings.Builder
	hasAny := len(matchedSkills) > 0 || len(missingSkills) > 0 || len(resumeSkills) > 0
	if !hasAny {
		return ""
	}

	sb.WriteString("\nSkill Context:\n")
	if joined := joinLimitedStrings(resumeSkills, 25); joined != "" {
		sb.WriteString("- Candidate's existing resume skills: ")
		sb.WriteString(joined)
		sb.WriteString("\n")
	}
	if joined := joinLimitedStrings(matchedSkills, 20); joined != "" {
		sb.WriteString("- Skills that match the target job: ")
		sb.WriteString(joined)
		sb.WriteString("\n")
	}
	if joined := joinLimitedStrings(missingSkills, 20); joined != "" {
		sb.WriteString("- Skills the target job wants but are not confirmed in the resume: ")
		sb.WriteString(joined)
		sb.WriteString("\n")
	}
	return sb.String()
}

func buildExperienceContextBlock(ctx ExperienceOptimizationContext) string {
	var lines []string
	add := func(label, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			lines = append(lines, fmt.Sprintf("- %s: %s", label, value))
		}
	}

	add("Current role title", ctx.JobTitle)
	add("Current employer", ctx.Company)
	add("Seniority or role level", ctx.Seniority)
	add("Target role", ctx.TargetRole)
	add("Target company", ctx.TargetCompany)
	add("Location", firstNonEmptyExperienceContext(ctx.Location, joinLocation(ctx.City, ctx.State)))
	add("Start date", ctx.StartDate)
	if ctx.CurrentlyWorking {
		add("End date", "Present")
	} else {
		add("End date", ctx.EndDate)
	}
	if ctx.Remote {
		add("Work setting", "Remote")
	}

	if len(lines) == 0 {
		return ""
	}
	return "\nCurrent Experience Context:\n" + strings.Join(lines, "\n") + "\n"
}

func joinLimitedStrings(values []string, limit int) string {
	if limit <= 0 {
		return ""
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, minInt(len(values), limit))
	for _, value := range values {
		cleaned := strings.TrimSpace(value)
		key := strings.ToLower(cleaned)
		if cleaned == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, cleaned)
		if len(out) >= limit {
			break
		}
	}
	return strings.Join(out, ", ")
}

func joinLocation(city, state string) string {
	parts := make([]string, 0, 2)
	if city = strings.TrimSpace(city); city != "" {
		parts = append(parts, city)
	}
	if state = strings.TrimSpace(state); state != "" {
		parts = append(parts, state)
	}
	return strings.Join(parts, ", ")
}

func firstNonEmptyExperienceContext(values ...string) string {
	for _, value := range values {
		if cleaned := strings.TrimSpace(value); cleaned != "" {
			return cleaned
		}
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
