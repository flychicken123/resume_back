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
	if a == nil || a.llm == nil {
		return PersonalInfoResult{}, fmt.Errorf("personal info agent is not initialized")
	}

	prompt := fmt.Sprintf(`You are an assistant that extracts resume personal information from any text.
Return ONLY valid JSON using this exact schema:
{
  "name": "<full name or empty string if unknown>",
  "email": "<email address or empty string if not provided>",
  "phone": "<phone number digits with optional +country code or empty string if not provided>",
  "summary": "<1-2 sentence professional summary capturing experience; empty string only if nothing relevant is provided>"
}

Rules:
- If the user explicitly states they do not have a field (e.g., "I don't have email"), output an empty string for that field.
- Never invent data that wasn't in the input.
- Preserve proper case for names (e.g., "XUan wu" -> "Xuan Wu").
- Strip filler phrases like "my number is" or "email me at" from stored values—return only the raw value.
- For the summary, combine any sentences that describe the user's professional background (e.g., years of experience, companies, roles) into a concise, polished summary even if they didn't label it as such.
- If no career context is present, return an empty string for summary.

Text:
%s`, input)

	response, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return PersonalInfoResult{}, err
	}

	response = sanitizeLLMJSON(response)

	var result PersonalInfoResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return PersonalInfoResult{}, fmt.Errorf("failed to parse personal info JSON: %w", err)
	}

	result.Name = strings.TrimSpace(result.Name)
	result.Email = strings.TrimSpace(result.Email)
	result.Phone = strings.TrimSpace(result.Phone)

	return result, nil
}

func sanitizeLLMJSON(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}
