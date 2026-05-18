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

// TemplatePreferenceAgent uses an LLM to map free-form text to a template + font size choice.
type TemplatePreferenceAgent struct {
	llm llms.Model
}

type TemplatePreferenceResult struct {
	TemplateID string `json:"template_id"`
	FontSizeID string `json:"font_size_id"`
	Reason     string `json:"reason,omitempty"`
}

// NewTemplatePreferenceAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewTemplatePreferenceAgent() (*TemplatePreferenceAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(), googleai.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client: %w", err)
	}

	return &TemplatePreferenceAgent{llm: llm}, nil
}

// InferPreference converts a natural language description into a concrete template + font size.
func (a *TemplatePreferenceAgent) InferPreference(ctx context.Context, input string) (TemplatePreferenceResult, error) {
	if a == nil || a.llm == nil {
		return TemplatePreferenceResult{}, fmt.Errorf("template preference agent is not initialized")
	}

	prompt := fmt.Sprintf(`You are an assistant helping users choose a resume template and font size based on free-form text.

You MUST pick from these allowed values:
- template_id (exact slug, lowercase):
  - "classic-professional"  (also called "classic", "traditional", "corporate")
  - "modern-clean"          (also called "modern", "clean", "tech", "startup")
  - "executive-serif"       (also called "executive", "serif", "leadership", "manager")
  - "attorney-template"     (also called "attorney", "legal", "law", "lawyer")
  - "harvard-ats"           (also called "Harvard", "ATS", "finance", "consulting", "early career")
  - "tech-minimal"          (also called "tech", "engineering", "developer", "software")
  - "creative-portfolio"    (also called "creative", "marketing", "design", "portfolio")
  - "academic-cv"           (also called "academic", "research", "professor", "PhD", "education")

- font_size_id (exact slug, lowercase):
  - "small"         (also: "compact", "tight", "smaller text")
  - "medium"        (also: "normal", "standard", "default")
  - "large"         (also: "big", "larger", "more readable")
  - "extra-large"   (also: "very big", "huge", "xl", "extra big")

If the user names a specific template like "Modern Clean", map it to the correct slug "modern-clean".
If they describe style (e.g., "tech, clean, modern"), choose "modern-clean" or "tech-minimal" depending on whether they emphasize engineering/projects.
If they do not clearly specify, default to:
- template_id: "classic-professional"
- font_size_id: "medium"

Return ONLY a single JSON object with this exact shape and no extra keys:
{
  "template_id": "<one allowed template_id slug>",
  "font_size_id": "<one allowed font_size_id slug>",
  "reason": "<very short explanation of why you chose this combination>"
}

User description:
%s`, input)

	raw, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return TemplatePreferenceResult{}, err
	}

	clean := sanitizeLLMJSON(raw)

	var result TemplatePreferenceResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return TemplatePreferenceResult{}, fmt.Errorf("failed to parse template preference JSON: %w", err)
	}

	result.TemplateID = normalizeTemplateSlug(strings.TrimSpace(strings.ToLower(result.TemplateID)))
	result.FontSizeID = normalizeFontSizeSlug(strings.TrimSpace(strings.ToLower(result.FontSizeID)))

	if result.TemplateID == "" {
		result.TemplateID = "classic-professional"
	}
	if result.FontSizeID == "" {
		result.FontSizeID = "medium"
	}

	return result, nil
}

func normalizeTemplateSlug(value string) string {
	if value == "" {
		return ""
	}
	switch value {
	case "classic-professional", "classic", "traditional":
		return "classic-professional"
	case "modern-clean", "modern", "clean", "startup":
		return "modern-clean"
	case "executive-serif", "executive", "serif", "leadership", "manager":
		return "executive-serif"
	case "attorney-template", "attorney", "legal", "law", "lawyer":
		return "attorney-template"
	case "harvard-ats", "harvard", "ats", "finance", "consulting", "early-career", "early career":
		return "harvard-ats"
	case "tech-minimal", "tech", "engineering", "developer", "software":
		return "tech-minimal"
	case "creative-portfolio", "creative", "marketing", "design", "portfolio":
		return "creative-portfolio"
	case "academic-cv", "academic", "research", "professor", "phd", "education":
		return "academic-cv"
	default:
		return value
	}
}

func normalizeFontSizeSlug(value string) string {
	if value == "" {
		return ""
	}
	switch value {
	case "small", "compact", "tight", "smaller":
		return "small"
	case "medium", "normal", "standard", "default":
		return "medium"
	case "large", "big", "larger":
		return "large"
	case "extra-large", "xl", "extra large", "very big", "huge":
		return "extra-large"
	default:
		return value
	}
}

