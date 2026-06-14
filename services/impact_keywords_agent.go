package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
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
	impactKeywordsMaxPerField     = 8
)

var (
	impactMetricRe  = regexp.MustCompile(`(?i)(?:\$ ?\d[\d,.]*(?:\s?(?:k|m|b|million|billion))?|\d+(?:\.\d+)?\s?(?:%|x|ms|sec|secs|seconds|minutes|hours|days|weeks|k\+?|m\+?|b\+?|\+|users|customers|requests/day|requests|engineers|people|members|revenue|costs?))`)
	impactPhraseRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:reduced|increased|improved|automated|streamlined|optimized|scaled|accelerated|saved|delivered|deployed|migrated|built|designed|implemented)\s+[a-z0-9#+./-]+(?:\s+[a-z0-9#+./-]+){0,4}`),
		regexp.MustCompile(`(?i)\b(?:scalable|distributed|real-time|high-availability|production|enterprise-wide|cross-functional|low-latency|full-stack|backend|frontend)\s+[a-z0-9#+./-]+(?:\s+[a-z0-9#+./-]+){0,4}`),
	}
	impactKnownTerms = []string{
		"scalable loan management system",
		"improved operational efficiency",
		"automated batch processing",
		"streamlining workflows",
		"Java Spring backend",
		"JavaScript frontend",
		"stored procedures",
		"Spring Boot",
		"TypeScript",
		"JavaScript",
		"Kubernetes",
		"microservices",
		"PostgreSQL",
		"distributed system",
		"real-time pipeline",
		"production system",
		"high-availability",
		"Python",
		"React",
		"Node.js",
		"Redis",
		"Docker",
		"Java",
		"AWS",
		"GCP",
	}
	leadingImpactVerbs = map[string]struct{}{
		"reduced": {}, "increased": {}, "improved": {}, "automated": {}, "streamlined": {},
		"optimized": {}, "scaled": {}, "accelerated": {}, "saved": {}, "delivered": {},
		"deployed": {}, "migrated": {}, "built": {}, "designed": {}, "implemented": {},
	}
	genericImpactKeywords = map[string]struct{}{
		"built": {}, "developed": {}, "implemented": {}, "worked": {}, "responsible": {},
	}
)

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

	// Build a simplified input for the LLM
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return ImpactKeywordsResult{}, fmt.Errorf("failed to marshal input: %w", err)
	}

	prompt := fmt.Sprintf(`You are an expert resume analyzer. Extract SHORT keywords and phrases (1-5 words) that demonstrate impact and achievements.

=== RULE: MAXIMUM 5 WORDS PER KEYWORD ===
Each keyword must be 1-5 words. Never extract full sentences.
Return at most 8 keywords per array. Prefer fewer, high-signal keywords.

INPUT DATA:
%s

=== WHAT TO EXTRACT (1-5 words each) ===

1. Numbers and Metrics (highest priority):
   - "40%%", "80%% reduction", "$2M revenue", "saved $500K"
   - "3x faster", "10x improvement", "99.9%% uptime"
   - "1M+ users", "500K requests/day", "50ms latency"

2. Team and Scale:
   - "team of 10", "12 engineers", "cross-functional team"
   - "enterprise-wide", "company-wide", "global deployment"
   - "production environment", "high-availability system"

3. Key Technologies (when significant):
   - "Redis", "Kubernetes", "AWS", "microservices"
   - "real-time pipeline", "distributed system"

4. Outcome Words:
   - "increased revenue", "reduced costs", "improved performance"
   - "automated deployment", "streamlined workflow"
   - "award-winning", "patent pending"

=== DO NOT EXTRACT ===
- Full sentences or bullet points (6+ words)
- Standalone action verbs: "Developed", "Built", "Implemented"
- Generic phrases: "worked on", "responsible for", "helped with"

=== EXAMPLES ===

Input: "Reduced API latency by 80%% using Redis caching for production"
Extract: ["80%%", "Redis", "production"] ✓
NOT: ["Reduced API latency by 80%% using Redis caching for production"] ✗

Input: "Led a cross-functional team of 12 engineers to deliver on time"
Extract: ["cross-functional team", "12 engineers"] ✓
NOT: ["Led a cross-functional team of 12 engineers to deliver on time"] ✗

Input: "Built scalable microservices architecture handling 1M requests"
Extract: ["scalable microservices", "1M requests"] ✓
NOT: ["Built scalable microservices architecture handling 1M requests"] ✗

=== OUTPUT FORMAT ===
Return ONLY valid JSON:
{
  "experiences": {
    "<experience_id>": {
      "description": ["keyword1", "keyword2"],
      "projects": {
        "<project_id>": {
          "name": [],
          "description": ["keyword1"],
          "technologies": []
        }
      }
    }
  }
}

Return ONLY the JSON:`, string(inputJSON))

	raw, err := llms.GenerateFromSinglePrompt(
		ctx,
		a.llm,
		prompt,
		llms.WithTemperature(0.0),
		llms.WithMaxTokens(impactKeywordsMaxOutputTokens),
		llms.WithJSONMode(),
	)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ImpactKeywordsResult{}, ctxErr
		}
		log.Printf("[impact-keywords-agent] LLM call failed, using fallback extractor: %v", err)
		return fallbackImpactKeywords(input), nil
	}

	clean := sanitizeLLMJSON(raw)

	var result ImpactKeywordsResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		log.Printf("[impact-keywords-agent] failed to parse LLM JSON, using fallback extractor: %v raw_len=%d", err, len(clean))
		return fallbackImpactKeywords(input), nil
	}

	// Ensure non-nil maps
	if result.Experiences == nil {
		result.Experiences = make(map[string]ExperienceImpactKeywords)
	}

	return result, nil
}

func fallbackImpactKeywords(input ImpactKeywordsRequest) ImpactKeywordsResult {
	result := ImpactKeywordsResult{
		Experiences: make(map[string]ExperienceImpactKeywords, len(input.Experiences)),
	}
	for i, exp := range input.Experiences {
		expID := strings.TrimSpace(exp.ID)
		if expID == "" {
			expID = fmt.Sprintf("experience_%d", i)
		}

		out := ExperienceImpactKeywords{
			Description: extractImpactKeywordsFromText(exp.Description, impactKeywordsMaxPerField),
		}
		if len(exp.Projects) > 0 {
			out.Projects = make(map[string]ProjectImpactKeywords, len(exp.Projects))
			for j, project := range exp.Projects {
				projectID := strings.TrimSpace(project.ID)
				if projectID == "" {
					projectID = fmt.Sprintf("project_%d", j)
				}
				out.Projects[projectID] = ProjectImpactKeywords{
					Name:         extractImpactKeywordsFromText(project.Name, impactKeywordsMaxPerField),
					Description:  extractImpactKeywordsFromText(project.Description, impactKeywordsMaxPerField),
					Technologies: extractImpactKeywordsFromText(project.Technologies, impactKeywordsMaxPerField),
				}
			}
		}

		result.Experiences[expID] = out
	}
	return result
}

func extractImpactKeywordsFromText(text string, max int) []string {
	text = strings.TrimSpace(text)
	if text == "" || max <= 0 {
		return nil
	}

	seen := make(map[string]struct{})
	keywords := make([]string, 0, max)
	add := func(keyword string) bool {
		keyword = cleanImpactKeyword(keyword)
		if keyword == "" {
			return false
		}
		if _, skip := genericImpactKeywords[strings.ToLower(keyword)]; skip {
			return false
		}
		key := strings.ToLower(keyword)
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
		keywords = append(keywords, keyword)
		return len(keywords) >= max
	}

	for _, match := range impactMetricRe.FindAllString(text, -1) {
		if add(match) {
			return keywords
		}
	}

	lower := strings.ToLower(text)
	for _, term := range impactKnownTerms {
		if strings.Contains(lower, strings.ToLower(term)) {
			if add(term) {
				return keywords
			}
		}
	}

	for _, re := range impactPhraseRes {
		for _, match := range re.FindAllString(text, -1) {
			if add(stripLeadingImpactVerb(match)) {
				return keywords
			}
		}
	}

	if len(keywords) == 0 {
		for _, keyword := range extractKeywords(text, max) {
			if add(keyword) {
				return keywords
			}
		}
	}

	return keywords
}

func stripLeadingImpactVerb(value string) string {
	words := strings.Fields(value)
	if len(words) <= 1 {
		return value
	}
	if _, ok := leadingImpactVerbs[strings.ToLower(words[0])]; ok {
		return strings.Join(words[1:], " ")
	}
	return value
}

func cleanImpactKeyword(value string) string {
	value = strings.TrimSpace(strings.Trim(value, ".,;:()[]{}\"'`"))
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}

	words := strings.Fields(value)
	if len(words) > 5 {
		value = strings.Join(words[:5], " ")
	}
	return value
}
