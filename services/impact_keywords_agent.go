package services

import (
	"context"
	"encoding/json"
	"fmt"
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
	ID          string                  `json:"id"`
	Description string                  `json:"description"`
	Projects    []ProjectImpactInput    `json:"projects,omitempty"`
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

// NewImpactKeywordsAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewImpactKeywordsAgent() (*ImpactKeywordsAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(), googleai.WithAPIKey(apiKey))
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

	prompt := fmt.Sprintf(`You are an expert resume analyzer. Extract ONLY the most impactful SHORT keywords (1-4 words) that show measurable results.

=== ABSOLUTE RULE: MAXIMUM 4 WORDS PER KEYWORD ===
Count the words before including any keyword. If it has more than 4 words, DO NOT include it.

INPUT DATA:
%s

=== WHAT TO EXTRACT (1-4 words ONLY) ===

PRIORITY 1 - Numbers and Metrics:
- "40%%", "80%% reduction", "$2M", "saved $500K"
- "3x faster", "10x improvement", "doubled throughput"
- "1M+ users", "500K requests/day", "99.9%% uptime"

PRIORITY 2 - Short Impact Phrases (max 3-4 words):
- "increased revenue", "reduced costs", "improved efficiency"
- "team of 10", "12 engineers", "5 direct reports"

PRIORITY 3 - Scale Words (1-2 words):
- "enterprise-wide", "company-wide", "global"
- "production", "high-availability"

=== BANNED - NEVER EXTRACT ===
❌ Full sentences or clauses
❌ Anything with 5+ words
❌ Action verbs: "Developed", "Built", "Designed", "Implemented", "Led", "Created", "Managed"
❌ Phrases starting with verbs: "Reduced latency by...", "Increased sales through..."
❌ Generic phrases: "worked on", "responsible for", "helped with"
❌ Technology names without nearby metrics

=== CORRECT vs WRONG EXAMPLES ===

WRONG: "reduced API latency by 80%% using Redis caching" (9 words)
CORRECT: ["80%%", "Redis"] (extract only the metric and tech)

WRONG: "led a cross-functional team of 12 engineers" (8 words)
CORRECT: ["12 engineers"] (extract only the number phrase)

WRONG: "increased monthly revenue by $2M through optimization" (7 words)
CORRECT: ["$2M"] (extract only the metric)

WRONG: "Implemented microservices architecture improving scalability" (5 words)
CORRECT: [] (no measurable metric, skip entirely)

WRONG: "Built real-time data pipeline processing 500K events" (7 words)
CORRECT: ["500K events"] (extract only the metric)

=== VALIDATION BEFORE OUTPUT ===
For each keyword you want to include:
1. Count the words - if more than 4, DO NOT include it
2. Check if it starts with an action verb - if yes, DO NOT include it
3. Check if it contains a number/metric - if no, reconsider if it's truly impactful

=== OUTPUT FORMAT ===
Return ONLY valid JSON (no markdown, no explanations):
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

	raw, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return ImpactKeywordsResult{}, err
	}

	clean := sanitizeLLMJSON(raw)

	var result ImpactKeywordsResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return ImpactKeywordsResult{}, fmt.Errorf("failed to parse impact keywords JSON: %w (raw: %s)", err, clean)
	}

	// Ensure non-nil maps
	if result.Experiences == nil {
		result.Experiences = make(map[string]ExperienceImpactKeywords)
	}

	return result, nil
}
