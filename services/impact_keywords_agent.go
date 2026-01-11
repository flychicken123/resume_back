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

	prompt := fmt.Sprintf(`You are an expert resume analyzer. Your task is to identify keywords and phrases in resume text that demonstrate IMPACT, achievements, and accomplishments. Be thorough and generous in identifying impactful content.

Analyze the following resume data and extract keywords/phrases that show impact:

INPUT DATA:
%s

WHAT TO IDENTIFY AS IMPACT KEYWORDS:
1. Quantifiable metrics and numbers (percentages, dollar amounts, time savings, team sizes)
   - Examples: "increased performance by 40%%", "$2M revenue", "team of 10 engineers", "reduced latency by 50ms", "3x improvement"
2. Strong action verbs that show leadership and achievement
   - Examples: "led", "spearheaded", "architected", "delivered", "launched", "pioneered", "drove", "owned"
3. Business outcome phrases
   - Examples: "increased revenue", "reduced costs", "improved efficiency", "accelerated delivery", "boosted engagement"
4. Scale indicators
   - Examples: "enterprise-wide", "1M+ users", "global deployment", "cross-functional", "company-wide"
5. Technical achievement verbs
   - Examples: "built", "developed", "implemented", "created", "designed", "engineered", "automated", "integrated"
6. Improvement and optimization words
   - Examples: "optimized", "streamlined", "enhanced", "upgraded", "modernized", "refactored", "revamped"
7. Collaboration and scope words
   - Examples: "collaborated", "partnered", "mentored", "trained", "coordinated", "managed", "supervised"
8. Success and completion indicators
   - Examples: "achieved", "accomplished", "completed", "delivered", "shipped", "released", "launched", "deployed"

RULES:
- Only extract actual phrases from the text - do not invent or modify them
- Include meaningful context around metrics (e.g., "increased performance by 40%%" not just "40%%")
- For numbers, always include surrounding context (e.g., "team of 10 engineers", "serving 1M users")
- Extract strong action verbs even when standalone (e.g., "Developed", "Built", "Led")
- Be generous - if a word or phrase suggests achievement or impact, include it
- Try to find impact keywords in every description - look for any indicator of accomplishment
- Match the exact text including any special characters

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
