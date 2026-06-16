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

type ExperienceOptimizationBatchItem struct {
	Position int
	Index    int
	Input    ExperienceOptimizationInput
}

type ExperienceOptimizationBatchOutcome struct {
	Position            int
	Index               int
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

func OptimizeExperiencesBatchFast(ctx context.Context, items []ExperienceOptimizationBatchItem) ([]ExperienceOptimizationBatchOutcome, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if len(items) == 1 {
		return optimizeSingleExperienceTextFast(ctx, items[0])
	}

	prompt := BuildExperienceBatchOptimizationPrompt(items)
	raw, err := CallGeminiWithTemperatureMaxTokensContext(ctx, prompt, 0.2, experienceBatchMaxTokens(len(items)))
	if err != nil {
		return nil, err
	}

	parsed, err := ParseExperienceBatchOptimizationResults(raw)
	if err != nil {
		return nil, err
	}

	itemByPosition := make(map[int]ExperienceOptimizationBatchItem, len(items))
	itemByIndex := make(map[int]ExperienceOptimizationBatchItem, len(items))
	duplicateIndexes := make(map[int]bool)
	for _, item := range items {
		itemByPosition[item.Position] = item
		if _, exists := itemByIndex[item.Index]; exists {
			duplicateIndexes[item.Index] = true
			continue
		}
		itemByIndex[item.Index] = item
	}

	outcomes := make([]ExperienceOptimizationBatchOutcome, 0, len(parsed))
	for _, result := range parsed {
		item, ok := itemByPosition[result.position]
		if !ok && result.hasIndex && !duplicateIndexes[result.index] {
			item, ok = itemByIndex[result.index]
		}
		if !ok {
			continue
		}

		optimized := strings.TrimSpace(firstNonEmptyExperienceContext(
			result.optimizedExperience,
			result.optimizedExperienceSnake,
			result.description,
		))
		if optimized == "" {
			continue
		}

		reviewStatus := strings.TrimSpace(result.reviewStatus)
		if reviewStatus == "" {
			reviewStatus = "fast_batch"
		}

		outcomes = append(outcomes, ExperienceOptimizationBatchOutcome{
			Position:            item.Position,
			Index:               item.Index,
			OptimizedExperience: ValidateAndCleanOutput(item.Input.UserExperience, optimized),
			ReviewStatus:        reviewStatus,
			ReviewReason:        strings.TrimSpace(result.reviewReason),
		})
	}

	return outcomes, nil
}

func optimizeSingleExperienceTextFast(ctx context.Context, item ExperienceOptimizationBatchItem) ([]ExperienceOptimizationBatchOutcome, error) {
	prompt := BuildExperienceSingleOptimizationPrompt(item)
	raw, err := CallGeminiTextWithTemperatureMaxTokensContext(ctx, prompt, 0.2, 2048)
	if err != nil {
		return nil, err
	}

	optimized := cleanPlainExperienceResponse(raw)
	if optimized == "" {
		return nil, fmt.Errorf("empty experience optimization response")
	}
	optimized, ok := repairIncompleteExperienceOutput(item.Input.UserExperience, optimized)
	if !ok {
		return nil, fmt.Errorf("AI returned incomplete experience text; please retry")
	}
	optimized = preserveExperienceLineCoverage(item.Input.UserExperience, optimized)

	return []ExperienceOptimizationBatchOutcome{
		{
			Position:            item.Position,
			Index:               item.Index,
			OptimizedExperience: ValidateAndCleanOutput(item.Input.UserExperience, optimized),
			ReviewStatus:        "fast_batch",
			ReviewReason:        "checked",
		},
	}, nil
}

func BuildExperienceSingleOptimizationPrompt(item ExperienceOptimizationBatchItem) string {
	input := item.Input
	targetJob := strings.TrimSpace(input.JobDescription)
	if targetJob == "" {
		targetJob = "(none provided; improve against original experience and resume context only)"
	} else {
		targetJob = truncateExperiencePromptText(targetJob, 4000)
	}

	var contextBuilder strings.Builder
	if contextBlock := buildExperienceContextBlock(input.Context); contextBlock != "" {
		contextBuilder.WriteString(contextBlock)
	}
	if skillContext := buildExperienceSkillContext(input.MatchedSkills, input.MissingSkills, input.Context.ResumeSkills); skillContext != "" {
		contextBuilder.WriteString(skillContext)
	}
	originalLineCount := len(splitExperienceTextLines(input.UserExperience))
	if originalLineCount == 0 {
		originalLineCount = 1
	}

	return fmt.Sprintf(`You are an expert resume writer and strict factual editor.

Task:
Improve this single work experience for clarity, action orientation, impact, and target-job alignment when supported by the original facts.

Target Job Description:
%s
%s
Original Experience Description:
%s

Rules:
1. Never invent accomplishments, metrics, technologies, responsibilities, employers, titles, dates, tools, scale, outcomes, percentages, or dollar amounts.
2. Use target-job language only where it naturally fits the user's actual background.
3. If the original lacks metrics, keep statements qualitative.
4. Start each achievement with a strong action verb.
5. Put each achievement on its own line.
6. Return exactly %d achievement lines: one improved line for each original achievement line.
7. Do not merge, delete, summarize, collapse, or reorder original achievement lines.
8. Every returned line must be a complete sentence ending with terminal punctuation.
9. Do not shorten by cutting text mid-sentence; rewrite concisely instead.
10. Do not include bullet symbols, markdown, headers, company names, job titles, dates, JSON, or explanations.

Return ONLY the improved experience text.`, targetJob, contextBuilder.String(), truncateExperiencePromptText(input.UserExperience, 6000), originalLineCount)
}

func cleanPlainExperienceResponse(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "```text")
	value = strings.TrimPrefix(value, "```")
	value = strings.TrimSuffix(value, "```")
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		var decoded string
		if err := json.Unmarshal([]byte(value), &decoded); err == nil {
			return strings.TrimSpace(decoded)
		}
	}
	return value
}

func repairIncompleteExperienceOutput(original, optimized string) (string, bool) {
	optimizedLines := splitExperienceTextLines(optimized)
	if len(optimizedLines) == 0 {
		return "", false
	}

	originalLines := splitExperienceTextLines(original)
	repaired := false
	for i, line := range optimizedLines {
		if !isLikelyIncompleteExperienceLine(line) {
			continue
		}
		if i >= len(originalLines) || strings.TrimSpace(originalLines[i]) == "" {
			return "", false
		}
		optimizedLines[i] = originalLines[i]
		repaired = true
	}

	if !repaired {
		return strings.TrimSpace(optimized), true
	}
	return strings.Join(optimizedLines, "\n"), true
}

func preserveExperienceLineCoverage(original, optimized string) string {
	originalLines := splitExperienceTextLines(original)
	optimizedLines := splitExperienceTextLines(optimized)
	if len(originalLines) <= 1 || len(optimizedLines) == 0 || len(optimizedLines) >= len(originalLines) {
		return strings.TrimSpace(optimized)
	}

	merged := make([]string, 0, len(originalLines))
	merged = append(merged, optimizedLines...)
	merged = append(merged, originalLines[len(optimizedLines):]...)
	return strings.Join(merged, "\n")
}

func splitExperienceTextLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		line = strings.TrimSpace(line)
		line = trimLeadingExperienceBullet(line)
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func trimLeadingExperienceBullet(line string) string {
	return strings.TrimLeftFunc(line, func(r rune) bool {
		return r == '-' || r == '*' || r == '•' || r == ' ' || r == '\t'
	})
}

func isLikelyIncompleteExperienceLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	last := line[len(line)-1]
	return last != '.' && last != '!' && last != '?' && last != ')'
}

func BuildExperienceBatchOptimizationPrompt(items []ExperienceOptimizationBatchItem) string {
	var targetJob string
	for _, item := range items {
		if value := strings.TrimSpace(item.Input.JobDescription); value != "" {
			targetJob = truncateExperiencePromptText(value, 4000)
			break
		}
	}

	mode := "Improve grammar, clarity, professional tone, action orientation, and impact for each work experience description."
	if targetJob != "" {
		mode = "Enhance each work experience description to better align with the target job while preserving the candidate's original facts."
	}

	var inputBuilder strings.Builder
	for _, item := range items {
		input := item.Input
		fmt.Fprintf(&inputBuilder, "=== EXPERIENCE POSITION %d / ORIGINAL INDEX %d ===\n", item.Position, item.Index)
		if contextBlock := buildExperienceContextBlock(input.Context); contextBlock != "" {
			inputBuilder.WriteString(contextBlock)
		}
		if skillContext := buildExperienceSkillContext(input.MatchedSkills, input.MissingSkills, input.Context.ResumeSkills); skillContext != "" {
			inputBuilder.WriteString(skillContext)
		}
		inputBuilder.WriteString("Original Experience Description:\n")
		inputBuilder.WriteString(truncateExperiencePromptText(input.UserExperience, 6000))
		inputBuilder.WriteString("\n\n")
	}

	if strings.TrimSpace(targetJob) == "" {
		targetJob = "(none provided; improve against original experience and resume context only)"
	}

	return fmt.Sprintf(`You are an expert resume writer and strict factual editor.

Task:
%s

Target Job Description:
%s

Experiences:
%s

CRITICAL INTEGRITY RULES:
1. NEVER invent accomplishments, metrics, technologies, responsibilities, employers, titles, dates, seniority, tools, scale, outcomes, percentages, or dollar amounts.
2. NEVER claim a missing skill as existing experience unless it is explicitly supported by the original description or resume context for that same experience.
3. Preserve the user's authentic scope. Only rephrase, reorganize, clarify, and highlight.
4. If the original lacks metrics, keep statements qualitative.
5. Return one result for every supplied experience position.

WRITING RULES:
1. Start each achievement with a strong action verb.
2. Keep each achievement on its own line inside optimizedExperience.
3. Do not include bullet symbols, markdown, headers, company names, job titles, dates, or explanations in optimizedExperience.
4. Return one improved line for each original achievement line.
5. Do not merge, delete, summarize, collapse, or reorder original achievement lines.
6. Every achievement line must be a complete sentence ending with terminal punctuation.
7. Do not shorten by cutting text mid-sentence; rewrite concisely instead.
8. Keep reviewReason to "checked" unless you corrected a factual issue; maximum 12 words.

Before returning, self-check each optimizedExperience against its original description and context. Remove any unsupported claim.

Return ONLY valid JSON with this shape:
{
  "results": [
    {
      "position": 0,
      "index": 0,
      "optimizedExperience": "rewritten achievement one\nrewritten achievement two",
      "reviewStatus": "fast_batch",
      "reviewReason": "checked"
    }
  ]
}`, mode, targetJob, inputBuilder.String())
}

func experienceBatchMaxTokens(itemCount int) int {
	if itemCount <= 1 {
		return 1536
	}
	maxTokens := itemCount * 900
	if maxTokens < 1024 {
		return 1024
	}
	if maxTokens > 4096 {
		return 4096
	}
	return maxTokens
}

func truncateExperiencePromptText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	cut := value[:max]
	minBoundary := max * 2 / 3
	if boundary := strings.LastIndexAny(cut, ".!?\n"); boundary >= minBoundary {
		cut = cut[:boundary+1]
	} else if boundary := strings.LastIndexAny(cut, " \t"); boundary >= minBoundary {
		cut = cut[:boundary]
	}
	return strings.TrimSpace(cut) + "\n[truncated]"
}

type experienceBatchOptimizationResult struct {
	position                 int
	index                    int
	hasIndex                 bool
	optimizedExperience      string
	optimizedExperienceSnake string
	description              string
	reviewStatus             string
	reviewReason             string
}

func ParseExperienceBatchOptimizationResults(raw string) ([]experienceBatchOptimizationResult, error) {
	type llmResult struct {
		Position                 *int   `json:"position"`
		Index                    *int   `json:"index"`
		OptimizedExperience      string `json:"optimizedExperience"`
		OptimizedExperienceSnake string `json:"optimized_experience"`
		Description              string `json:"description"`
		ReviewStatus             string `json:"reviewStatus"`
		ReviewReason             string `json:"reviewReason"`
	}

	decode := func(clean string) ([]llmResult, error) {
		var wrapped struct {
			Results []llmResult `json:"results"`
		}
		if err := json.Unmarshal([]byte(clean), &wrapped); err == nil && wrapped.Results != nil {
			return wrapped.Results, nil
		}

		var array []llmResult
		if err := json.Unmarshal([]byte(clean), &array); err != nil {
			return nil, err
		}
		return array, nil
	}

	clean := strings.TrimSpace(stripMarkdownFence(raw))
	decoded, err := decode(clean)
	if err != nil {
		if array := extractJSONArray(clean); array != clean {
			decoded, err = decode(array)
		}
		if err != nil {
			return nil, fmt.Errorf("parse batch experience optimization JSON: %w", err)
		}
	}

	results := make([]experienceBatchOptimizationResult, 0, len(decoded))
	for _, item := range decoded {
		if item.Position == nil {
			continue
		}
		result := experienceBatchOptimizationResult{
			position:                 *item.Position,
			optimizedExperience:      strings.TrimSpace(item.OptimizedExperience),
			optimizedExperienceSnake: strings.TrimSpace(item.OptimizedExperienceSnake),
			description:              strings.TrimSpace(item.Description),
			reviewStatus:             strings.TrimSpace(item.ReviewStatus),
			reviewReason:             strings.TrimSpace(item.ReviewReason),
		}
		if item.Index != nil {
			result.index = *item.Index
			result.hasIndex = true
		}
		results = append(results, result)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("batch experience optimization returned no results")
	}
	return results, nil
}

func extractJSONArray(raw string) string {
	cleaned := strings.TrimSpace(stripMarkdownFence(raw))
	start := strings.Index(cleaned, "[")
	end := strings.LastIndex(cleaned, "]")
	if start >= 0 && end >= start {
		return cleaned[start : end+1]
	}
	return cleaned
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
