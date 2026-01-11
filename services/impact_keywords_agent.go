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

	prompt := fmt.Sprintf(`You are an expert resume analyzer. Your task is to identify keywords and phrases that demonstrate MEASURABLE IMPACT, concrete achievements, and quantifiable accomplishments.

Analyze the following resume data and extract keywords/phrases that show REAL impact:

INPUT DATA:
%s

WHAT TO IDENTIFY AS IMPACT KEYWORDS:
1. Quantifiable metrics and numbers (HIGHEST PRIORITY)
   - Percentages: "40%%", "increased by 25%%", "reduced by 60%%"
   - Dollar amounts: "$2M revenue", "saved $500K", "$10M budget"
   - Time savings: "reduced latency by 50ms", "cut deployment time from 2 hours to 10 minutes"
   - Scale numbers: "team of 10 engineers", "1M+ users", "500K daily requests", "100+ microservices"
   - Multipliers: "3x improvement", "doubled throughput", "tripled efficiency"
2. Business outcome phrases with results
   - "increased revenue by", "reduced costs by", "improved efficiency by", "accelerated delivery"
   - "boosted engagement", "grew user base", "expanded market share"
3. Scale and scope indicators
   - "enterprise-wide", "company-wide", "global deployment", "cross-functional"
   - "production environment", "high-availability", "99.9%% uptime"
4. Leadership with scope (only when followed by team size or scope)
   - "led a team of 10", "mentored 5 junior developers", "managed 3 direct reports"
   - NOT standalone verbs like "Led", "Managed", "Mentored"
5. Concrete outcomes and deliverables
   - "resulting in", "which led to", "achieving", "enabling"
   - "launched to 50K users", "adopted by 200+ teams"

DO NOT HIGHLIGHT (these are generic and don't show impact):
- Standalone action verbs at the start of bullet points: "Developed", "Built", "Designed", "Architected", "Implemented", "Created", "Deployed", "Engineered", "Integrated", "Configured"
- Generic technical work: "wrote code", "fixed bugs", "attended meetings"
- Vague phrases: "worked on", "helped with", "responsible for", "participated in"
- Standalone tools/technologies without context

RULES:
- Focus on RESULTS and OUTCOMES, not activities
- Only extract actual phrases from the text - do not invent or modify them
- Always include the full context around metrics (e.g., "increased performance by 40%%" not just "40%%")
- For numbers, include surrounding context (e.g., "team of 10 engineers", "serving 1M users")
- DO NOT extract standalone action verbs like "Developed", "Built", "Designed" - these describe tasks, not impact
- Match the exact text including any special characters
- If a description has no measurable impact, return an empty array - don't force-find keywords

OUTPUT FORMAT:
Return ONLY a valid JSON object with this exact structure (no explanations):
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

Analyze the input and return ONLY the JSON output:`, string(inputJSON))

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
