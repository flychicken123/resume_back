package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
)

// ImpactKeywordsAgent wraps an LLM client to extract impact keywords from
// resume experience and project descriptions.
type ImpactKeywordsAgent struct {
	llm llms.Model
}

// ExperienceImpactInput represents the input for a single experience entry.
type ExperienceImpactInput struct {
	ID          string               `json:"id"`
	Description string               `json:"description"`
	Projects    []ProjectImpactInput `json:"projects,omitempty"`
}

// ProjectImpactInput represents the input for a single project.
type ProjectImpactInput struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Technologies string `json:"technologies"`
}

// ImpactKeywordsRequest is the top-level request payload.
type ImpactKeywordsRequest struct {
	Experiences []ExperienceImpactInput `json:"experiences"`
}

// ProjectImpactKeywords holds the keywords for a single project.
type ProjectImpactKeywords struct {
	Name         []string `json:"name"`
	Description  []string `json:"description"`
	Technologies []string `json:"technologies"`
}

// ExperienceImpactKeywords holds the keywords for a single experience.
type ExperienceImpactKeywords struct {
	Description []string                         `json:"description"`
	Projects    map[string]ProjectImpactKeywords `json:"projects,omitempty"`
}

// ImpactKeywordsResult is the top-level response payload.
type ImpactKeywordsResult struct {
	Experiences map[string]ExperienceImpactKeywords `json:"experiences"`
}

const (
	impactKeywordsMaxOutputTokens = 8192
	impactKeywordsMaxPerField     = 6
	impactKeywordsMaxWords        = 6
)

type impactKeywordCandidate struct {
	Text   string  `json:"text"`
	Type   string  `json:"type,omitempty"`
	Score  float64 `json:"score,omitempty"`
	Reason string  `json:"reason,omitempty"`
}

func (c *impactKeywordCandidate) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		c.Text = text
		return nil
	}
	type candidate impactKeywordCandidate
	var parsed candidate
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*c = impactKeywordCandidate(parsed)
	return nil
}

type impactCandidateProject struct {
	Name         []impactKeywordCandidate `json:"name,omitempty"`
	Description  []impactKeywordCandidate `json:"description,omitempty"`
	Technologies []impactKeywordCandidate `json:"technologies,omitempty"`
}

type impactCandidateExperience struct {
	Description []impactKeywordCandidate          `json:"description,omitempty"`
	Projects    map[string]impactCandidateProject `json:"projects,omitempty"`
}

type impactCandidateResult struct {
	Experiences map[string]impactCandidateExperience `json:"experiences"`
}

// NewImpactKeywordsAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewImpactKeywordsAgent() (*ImpactKeywordsAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(),
		googleai.WithAPIKey(apiKey),
		googleai.WithDefaultModel(DefaultGeminiGenerationModel),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client for impact keywords agent: %w", err)
	}

	return &ImpactKeywordsAgent{llm: llm}, nil
}

// ExtractImpactKeywords analyzes experience and project descriptions to identify
// impact-showing keywords that should be highlighted.
func (a *ImpactKeywordsAgent) ExtractImpactKeywords(ctx context.Context, input ImpactKeywordsRequest) (ImpactKeywordsResult, error) {
	if a == nil || a.llm == nil {
		return ImpactKeywordsResult{}, fmt.Errorf("impact keywords agent is not initialized")
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return ImpactKeywordsResult{}, fmt.Errorf("failed to marshal input: %w", err)
	}

	candidates, err := a.extractImpactKeywordCandidates(ctx, string(inputJSON))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ImpactKeywordsResult{}, ctxErr
		}
		log.Printf("[impact-keywords-agent] candidate extraction failed, returning empty highlights: %v", err)
		return emptyImpactKeywords(input), nil
	}

	reviewed, err := a.reviewImpactKeywordCandidates(ctx, string(inputJSON), candidates)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ImpactKeywordsResult{}, ctxErr
		}
		log.Printf("[impact-keywords-agent] candidate review failed, using extractor candidates: %v", err)
		reviewed = candidates
	}

	return validateImpactKeywordCandidates(input, reviewed), nil
}

func (a *ImpactKeywordsAgent) extractImpactKeywordCandidates(ctx context.Context, inputJSON string) (impactCandidateResult, error) {
	raw, err := a.generateImpactKeywordJSON(ctx, buildImpactKeywordExtractorPrompt(inputJSON))
	if err != nil {
		return impactCandidateResult{}, err
	}
	candidates, err := parseImpactCandidateResult(raw)
	if err == nil {
		return candidates, nil
	}

	retryRaw, retryErr := a.generateImpactKeywordJSON(ctx, buildImpactKeywordRetryPrompt(inputJSON))
	if retryErr != nil {
		return impactCandidateResult{}, fmt.Errorf("parse extractor JSON: %w; retry call: %v", err, retryErr)
	}
	retryCandidates, retryParseErr := parseImpactCandidateResult(retryRaw)
	if retryParseErr != nil {
		return impactCandidateResult{}, fmt.Errorf("parse extractor JSON: %w; parse retry JSON: %v", err, retryParseErr)
	}
	return retryCandidates, nil
}

func (a *ImpactKeywordsAgent) reviewImpactKeywordCandidates(ctx context.Context, inputJSON string, candidates impactCandidateResult) (impactCandidateResult, error) {
	candidatesJSON, err := json.Marshal(candidates)
	if err != nil {
		return impactCandidateResult{}, fmt.Errorf("marshal candidates: %w", err)
	}
	raw, err := a.generateImpactKeywordJSON(ctx, buildImpactKeywordReviewerPrompt(inputJSON, string(candidatesJSON)))
	if err != nil {
		return impactCandidateResult{}, err
	}
	return parseImpactCandidateResult(raw)
}

func (a *ImpactKeywordsAgent) generateImpactKeywordJSON(ctx context.Context, prompt string) (string, error) {
	return llms.GenerateFromSinglePrompt(
		ctx,
		a.llm,
		prompt,
		llms.WithTemperature(0.0),
		llms.WithMaxTokens(impactKeywordsMaxOutputTokens),
		llms.WithJSONMode(),
	)
}

func parseImpactCandidateResult(raw string) (impactCandidateResult, error) {
	clean := sanitizeLLMJSON(raw)
	var result impactCandidateResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return impactCandidateResult{}, fmt.Errorf("failed to parse impact keyword JSON: %w (raw_len=%d)", err, len(clean))
	}
	if result.Experiences == nil {
		result.Experiences = make(map[string]impactCandidateExperience)
	}
	return result, nil
}

func buildImpactKeywordExtractorPrompt(inputJSON string) string {
	return fmt.Sprintf(`You are an expert resume reviewer. Extract candidate phrases that may deserve visual emphasis in a resume preview.

INPUT DATA:
%s

QUALITY RUBRIC:
- Extract phrases a recruiter would notice as evidence of measurable impact, business outcome, scale, ownership, performance, automation, reliability, leadership, or unusually relevant technical impact.
- Ordinary technology names, job duties, and generic responsibility phrases should only be included when the surrounding wording makes them clearly impact-bearing.
- Each text value must be an exact substring from the provided input. Do not rewrite, summarize, normalize, or invent phrases.
- Each text value must be 1-%d words and must not be a full sentence or bullet.
- Return no more than 10 candidates per array. It is acceptable to return fewer.

Return ONLY valid JSON in this shape:
{
  "experiences": {
    "<experience_id>": {
      "description": [
        {"text": "<exact substring>", "type": "metric|outcome|scale|ownership|technology|leadership", "score": 0.0, "reason": "<brief reason>"}
      ],
      "projects": {
        "<project_id>": {
          "name": [],
          "description": [],
          "technologies": []
        }
      }
    }
  }
}`, inputJSON, impactKeywordsMaxWords)
}

func buildImpactKeywordRetryPrompt(inputJSON string) string {
	return fmt.Sprintf(`Your previous impact-keyword JSON was invalid or truncated. Re-extract compact, valid JSON.

INPUT DATA:
%s

Rules:
- Return ONLY JSON.
- Use exact substrings from the input.
- Each text value is 1-%d words.
- Keep at most %d strongest items per array.
- Do not include explanations outside JSON.

JSON shape:
{"experiences":{"<experience_id>":{"description":[{"text":"<exact substring>","type":"metric|outcome|scale|ownership|technology|leadership","score":0.0,"reason":"<brief reason>"}],"projects":{"<project_id>":{"name":[],"description":[],"technologies":[]}}}}}`, inputJSON, impactKeywordsMaxWords, impactKeywordsMaxPerField)
}

func buildImpactKeywordReviewerPrompt(inputJSON, candidatesJSON string) string {
	return fmt.Sprintf(`You are a strict resume highlight reviewer. Select only the strongest highlight phrases from the candidates.

ORIGINAL INPUT:
%s

CANDIDATES:
%s

REVIEW RULES:
- Keep only phrases that would make a resume bullet stronger at a glance.
- Prefer quantified achievements, concrete outcomes, scale, ownership, leadership, automation, performance, reliability, and business value.
- Drop ordinary task descriptions, weak buzzwords, generic verbs, duplicate ideas, and technology-only phrases unless they are clearly tied to impact in the original wording.
- Keep at most %d phrases per array; 3-5 is usually best.
- Preserve each selected candidate's text exactly. Do not invent or rewrite.
- Return the same JSON shape as CANDIDATES, with only selected candidates.

Return ONLY valid JSON.`, inputJSON, candidatesJSON, impactKeywordsMaxPerField)
}

func validateImpactKeywordCandidates(input ImpactKeywordsRequest, candidates impactCandidateResult) ImpactKeywordsResult {
	result := emptyImpactKeywords(input)
	for i, exp := range input.Experiences {
		expID := normalizedExperienceImpactID(exp.ID, i)
		candidateExp, ok := candidates.Experiences[expID]
		if !ok {
			continue
		}

		expOut := result.Experiences[expID]
		expOut.Description = validateImpactKeywordList(exp.Description, candidateExp.Description)

		for j, project := range exp.Projects {
			projectID := normalizedProjectImpactID(project.ID, j)
			candidateProject, ok := candidateExp.Projects[projectID]
			if !ok {
				continue
			}
			projectOut := expOut.Projects[projectID]
			projectOut.Name = validateImpactKeywordList(project.Name, candidateProject.Name)
			projectOut.Description = validateImpactKeywordList(project.Description, candidateProject.Description)
			projectOut.Technologies = validateImpactKeywordList(project.Technologies, candidateProject.Technologies)
			expOut.Projects[projectID] = projectOut
		}

		result.Experiences[expID] = expOut
	}
	return result
}

func emptyImpactKeywords(input ImpactKeywordsRequest) ImpactKeywordsResult {
	result := ImpactKeywordsResult{
		Experiences: make(map[string]ExperienceImpactKeywords, len(input.Experiences)),
	}
	for i, exp := range input.Experiences {
		expID := normalizedExperienceImpactID(exp.ID, i)
		out := ExperienceImpactKeywords{
			Description: []string{},
		}
		if len(exp.Projects) > 0 {
			out.Projects = make(map[string]ProjectImpactKeywords, len(exp.Projects))
			for j, project := range exp.Projects {
				projectID := normalizedProjectImpactID(project.ID, j)
				out.Projects[projectID] = ProjectImpactKeywords{
					Name:         []string{},
					Description:  []string{},
					Technologies: []string{},
				}
			}
		}
		result.Experiences[expID] = out
	}
	return result
}

func validateImpactKeywordList(source string, candidates []impactKeywordCandidate) []string {
	source = strings.TrimSpace(source)
	if source == "" || len(candidates) == 0 {
		return []string{}
	}

	seen := map[string]struct{}{}
	ranges := make([][2]int, 0, impactKeywordsMaxPerField)
	keywords := make([]string, 0, impactKeywordsMaxPerField)
	for _, candidate := range candidates {
		text := normalizeImpactCandidateText(candidate.Text)
		if text == "" || wordCount(text) > impactKeywordsMaxWords {
			continue
		}
		start, end, originalText, ok := findOriginalImpactPhrase(source, text)
		if !ok {
			continue
		}
		key := strings.ToLower(originalText)
		if _, exists := seen[key]; exists {
			continue
		}
		if overlapsAny(start, end, ranges) {
			continue
		}
		seen[key] = struct{}{}
		ranges = append(ranges, [2]int{start, end})
		keywords = append(keywords, originalText)
		if len(keywords) >= impactKeywordsMaxPerField {
			break
		}
	}

	return keywords
}

func normalizeImpactCandidateText(text string) string {
	text = strings.TrimSpace(strings.Trim(text, ".,;:()[]{}\"'`"))
	return strings.Join(strings.Fields(text), " ")
}

func findOriginalImpactPhrase(source, phrase string) (int, int, string, bool) {
	sourceLower := strings.ToLower(source)
	phraseLower := strings.ToLower(phrase)
	start := strings.Index(sourceLower, phraseLower)
	if start < 0 {
		return 0, 0, "", false
	}
	end := start + len(phrase)
	if end > len(source) {
		return 0, 0, "", false
	}
	return start, end, source[start:end], true
}

func overlapsAny(start, end int, ranges [][2]int) bool {
	for _, existing := range ranges {
		if start < existing[1] && end > existing[0] {
			return true
		}
	}
	return false
}

func wordCount(text string) int {
	return len(strings.Fields(text))
}

func normalizedExperienceImpactID(id string, index int) string {
	id = strings.TrimSpace(id)
	if id != "" {
		return id
	}
	return fmt.Sprintf("experience_%d", index)
}

func normalizedProjectImpactID(id string, index int) string {
	id = strings.TrimSpace(id)
	if id != "" {
		return id
	}
	return fmt.Sprintf("project_%d", index)
}
