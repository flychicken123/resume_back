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
- Carefully identify which field the user wants to update by looking for keywords
- NEVER update multiple fields when only one field is mentioned

FIELD DETECTION PRIORITY (check in this order):
1. Look for explicit field keywords in the user's message
2. Identify the field type based on the keyword that appears BEFORE the value
3. Return null for ALL other fields

FIELD DETECTION RULES:

EMAIL field (HIGHEST PRIORITY when email keywords present):
- Keywords: "email", "e-mail", "email address", "mail", "e mail"
- If ANY of these keywords appear, extract ONLY the email field, set all others to null
- Examples:
  * "change my email to wux1992@gmail.com" → {"name": null, "email": "wux1992@gmail.com", "phone": null, "summary": null}
  * "my email is john@example.com" → {"name": null, "email": "john@example.com", "phone": null, "summary": null}
  * "change my email address to jane@test.com" → {"name": null, "email": "jane@test.com", "phone": null, "summary": null}
  * "update my e-mail to support@company.com" → {"name": null, "email": "support@company.com", "phone": null, "summary": null}
- IMPORTANT: The value after email keywords is ALWAYS the email address, NEVER the name

PHONE field:
- Keywords: "phone", "telephone", "mobile", "cell", "number", "contact number", "phone number"
- If ANY of these keywords appear (WITHOUT email keywords), extract ONLY the phone field
- Examples:
  * "my phone is 555-1234" → {"name": null, "email": null, "phone": "555-1234", "summary": null}
  * "change my phone number to 555-5678" → {"name": null, "email": null, "phone": "555-5678", "summary": null}
  * "call me at 555-9999" → {"name": null, "email": null, "phone": "555-9999", "summary": null}

NAME field:
- Keywords: "my name", "I am", "I'm called", "call me" (followed by a person name), "name is", "full name"
- Only extract if these specific name keywords appear
- Examples:
  * "my name is John Doe" → {"name": "John Doe", "email": null, "phone": null, "summary": null}
  * "call me Jane Smith" → {"name": "Jane Smith", "email": null, "phone": null, "summary": null}
  * "I am Michael Johnson" → {"name": "Michael Johnson", "email": null, "phone": null, "summary": null}

SUMMARY field:
- Keywords: "summary", "background", "professional background", "about me", "bio"
- Examples:
  * "I'm a software engineer with 5 years of experience" → {"name": null, "email": null, "phone": null, "summary": "Software engineer with 5 years of experience"}
  * "my summary is: passionate product manager" → {"name": null, "email": null, "phone": null, "summary": "Passionate product manager"}

CRITICAL: Common mistake to AVOID:
- WRONG: "change my email to wux1992@gmail.com" → {"name": "wux1992@gmail.com", "email": "wux1992@gmail.com", ...}
- CORRECT: "change my email to wux1992@gmail.com" → {"name": null, "email": "wux1992@gmail.com", "phone": null, "summary": null}

Other rules:
- If the user explicitly states they do not have a field (e.g., "I don't have email"), output an empty string "" (not null) for that field.
- Never invent data that wasn't in the input.
- Preserve proper case for names (e.g., "XUan wu" -> "Xuan Wu").
- Strip filler phrases like "my number is" or "email me at" from stored values—return only the raw value.
- For email addresses, return ONLY the email address itself (e.g., "user@example.com"), not any surrounding text.
- For phone numbers, return ONLY the digits and optional + prefix.
- For the summary, combine any sentences that describe the user's professional background into a concise, polished summary.

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
