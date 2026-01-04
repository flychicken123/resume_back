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

// PersonalInfoAgent wraps an LLM client to extract personal details from natural language.
type PersonalInfoAgent struct {
	llm llms.Model
}

// PersonalInfoResult captures the structured fields returned by the agent.
type PersonalInfoResult struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Summary string `json:"summary"`
}

// NewPersonalInfoAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewPersonalInfoAgent() (*PersonalInfoAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(), googleai.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client: %w", err)
	}

	return &PersonalInfoAgent{llm: llm}, nil
}

// ExtractPersonalInfo converts free-form text into structured personal information using the LLM.
func (a *PersonalInfoAgent) ExtractPersonalInfo(ctx context.Context, input string) (PersonalInfoResult, error) {
	return a.ExtractPersonalInfoWithContext(ctx, input, PersonalInfoResult{})
}

// ExtractPersonalInfoWithContext is like ExtractPersonalInfo but accepts existing data
// and only updates fields that are explicitly mentioned in the input.
func (a *PersonalInfoAgent) ExtractPersonalInfoWithContext(ctx context.Context, input string, existing PersonalInfoResult) (PersonalInfoResult, error) {
	if a == nil || a.llm == nil {
		return PersonalInfoResult{}, fmt.Errorf("personal info agent is not initialized")
	}

	existingContext := ""
	if existing.Name != "" || existing.Email != "" || existing.Phone != "" || existing.Summary != "" {
		existingContext = fmt.Sprintf(`
Existing data (DO NOT change unless the user explicitly mentions updating these fields):
- Name: %s
- Email: %s
- Phone: %s
- Summary: %s
`, existing.Name, existing.Email, existing.Phone, existing.Summary)
	}

	prompt := fmt.Sprintf(`You are an assistant that extracts resume personal information from any text.
%s
Return ONLY valid JSON using this exact schema:
{
  "name": "<full name OR null if not mentioned>",
  "email": "<email address OR null if not mentioned>",
  "phone": "<phone number digits with optional +country code OR null if not mentioned>",
  "summary": "<1-2 sentence professional summary OR null if not mentioned>"
}

CRITICAL RULES FOR PARTIAL UPDATES:
- ONLY return a value (not null) for fields that the user EXPLICITLY mentions in their message
- If a field is not mentioned, return null (not empty string) to preserve existing data
- Examples:
  * User says "my name is John" → return {"name": "John", "email": null, "phone": null, "summary": null}
  * User says "my email is john@example.com" → return {"name": null, "email": "john@example.com", "phone": null, "summary": null}
  * User says "I'm a software engineer" → return {"name": null, "email": null, "phone": null, "summary": "Software engineer"}

Other rules:
- If the user explicitly states they do not have a field (e.g., "I don't have email"), output an empty string "" (not null) for that field.
- Never invent data that wasn't in the input.
- Preserve proper case for names (e.g., "XUan wu" -> "Xuan Wu").
- Strip filler phrases like "my number is" or "email me at" from stored values—return only the raw value.
- For the summary, combine any sentences that describe the user's professional background (e.g., years of experience, companies, roles) into a concise, polished summary even if they didn't label it as such.

Text:
%s`, existingContext, input)

	response, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return PersonalInfoResult{}, err
	}

	response = sanitizeLLMJSON(response)

	// Parse into a map to detect null values vs empty strings
	var rawMap map[string]interface{}
	if err := json.Unmarshal([]byte(response), &rawMap); err != nil {
		return PersonalInfoResult{}, fmt.Errorf("failed to parse personal info JSON: %w", err)
	}

	// Start with existing data
	result := existing

	// Only update fields that were explicitly returned (not null)
	if val, ok := rawMap["name"]; ok && val != nil {
		if strVal, isString := val.(string); isString {
			result.Name = strings.TrimSpace(strVal)
		}
	}

	if val, ok := rawMap["email"]; ok && val != nil {
		if strVal, isString := val.(string); isString {
			result.Email = strings.TrimSpace(strVal)
		}
	}

	if val, ok := rawMap["phone"]; ok && val != nil {
		if strVal, isString := val.(string); isString {
			result.Phone = strings.TrimSpace(strVal)
		}
	}

	if val, ok := rawMap["summary"]; ok && val != nil {
		if strVal, isString := val.(string); isString {
			result.Summary = strings.TrimSpace(strVal)
		}
	}

	return result, nil
}

func sanitizeLLMJSON(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}
