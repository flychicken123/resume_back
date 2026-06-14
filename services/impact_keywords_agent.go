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
