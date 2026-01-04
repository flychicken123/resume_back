package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
)

// detectIntendedField determines which field the user wants to update based on keywords in the input.
// Returns "email", "phone", "name", "summary", or "" if no specific field detected or multiple fields detected.
func detectIntendedField(input string) string {
	lower := strings.ToLower(input)

	// Check for modification keywords that indicate a single-field update
	hasModifyKeyword := strings.Contains(lower, "change my") || strings.Contains(lower, "update my") ||
		strings.Contains(lower, "set my") || strings.Contains(lower, "modify my")

	// Count how many different field types are mentioned
	fieldCount := 0

	// Check for email keywords
	hasEmailKeyword := strings.Contains(lower, "email") || strings.Contains(lower, "e-mail") ||
		strings.Contains(lower, "e mail")
	if hasEmailKeyword {
		fieldCount++
	}

	// Check for phone keywords
	hasPhoneKeyword := strings.Contains(lower, "phone") || strings.Contains(lower, "mobile") ||
		strings.Contains(lower, "cell") || strings.Contains(lower, "telephone")
	if hasPhoneKeyword {
		fieldCount++
	}

	// Check for name keywords
	hasNameKeyword := strings.Contains(lower, "my name") || strings.Contains(lower, "name is") ||
		strings.Contains(lower, "i am ") || strings.Contains(lower, "i'm ")
	if hasNameKeyword {
		fieldCount++
	}

	// Check for summary keywords
	hasSummaryKeyword := strings.Contains(lower, "summary") || strings.Contains(lower, "background") ||
		strings.Contains(lower, "about me") || strings.Contains(lower, "bio")
	if hasSummaryKeyword {
		fieldCount++
	}

	// If multiple fields are mentioned, this is a full update - allow all fields
	if fieldCount > 1 {
		return ""
	}

	// If modification keyword is present with a single field, return that field
	// This handles cases like "change my email to X" - only update email
	if hasModifyKeyword {
		if hasEmailKeyword {
			return "email"
		}
		if hasPhoneKeyword {
			return "phone"
		}
		if hasNameKeyword {
			return "name"
		}
		if hasSummaryKeyword {
			return "summary"
		}
	}

	// Also detect email-only update if:
	// - Input contains @ symbol pattern (email address)
	// - AND contains "change", "update", "set" (not necessarily "change my")
	// - AND only one field keyword is present (email)
	// - AND NO name keywords are present
	emailPattern := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	hasEmailAddress := emailPattern.MatchString(input)
	hasAnyModifyWord := strings.Contains(lower, "change") || strings.Contains(lower, "update") ||
		strings.Contains(lower, "set") || strings.Contains(lower, "modify")

	if hasEmailAddress && hasAnyModifyWord && hasEmailKeyword && !hasNameKeyword && !hasPhoneKeyword {
		return "email"
	}

	// No specific single-field update detected, allow all fields to be updated
	return ""
}

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

MANDATORY STEP-BY-STEP PROCESS (follow these steps in order):

STEP 1: Identify which keywords are present in the user's text
- Does the text contain "email" or "email address" or "e-mail" or "mail"? → EMAIL field
- Does the text contain "phone" or "mobile" or "telephone" or "cell"? → PHONE field
- Does the text contain "my name" or "I am" or "name is"? → NAME field
- Does the text contain "summary" or "professional background" or "about me"? → SUMMARY field

STEP 2: Determine which ONE field to extract
- If STEP 1 found email keywords → extract ONLY email, set all others to null
- If STEP 1 found phone keywords (but NOT email) → extract ONLY phone, set all others to null
- If STEP 1 found name keywords (but NOT email or phone) → extract ONLY name, set all others to null
- If STEP 1 found summary keywords → extract ONLY summary, set all others to null

STEP 3: Extract the value for that field
- For email: extract the email address (contains @)
- For phone: extract the phone number (digits with optional + prefix)
- For name: extract the person's name
- For summary: extract the professional description

STEP 4: Return JSON with ONLY that field populated, all others set to null

CRITICAL EXAMPLES (study these carefully):

Example 1:
Input: "change my email address to wux1992@gmail.com"
Step 1: Contains "email address" → EMAIL field
Step 2: Extract ONLY email field
Step 3: Email value = "wux1992@gmail.com"
Output: {"name": null, "email": "wux1992@gmail.com", "phone": null, "summary": null}

Example 2:
Input: "change my email to wux1992@gmail.com"
Step 1: Contains "email" → EMAIL field
Step 2: Extract ONLY email field
Step 3: Email value = "wux1992@gmail.com"
Output: {"name": null, "email": "wux1992@gmail.com", "phone": null, "summary": null}

Example 3:
Input: "my name is John Doe"
Step 1: Contains "my name" → NAME field
Step 2: Extract ONLY name field
Step 3: Name value = "John Doe"
Output: {"name": "John Doe", "email": null, "phone": null, "summary": null}

Example 4:
Input: "my phone is 555-1234"
Step 1: Contains "phone" → PHONE field
Step 2: Extract ONLY phone field
Step 3: Phone value = "555-1234"
Output: {"name": null, "email": null, "phone": "555-1234", "summary": null}

COMMON MISTAKES TO AVOID:
❌ WRONG: "change my email address to wux1992@gmail.com" → {"name": "address to wux1992@gmail", "email": "wux1992@gmail.com", ...}
✅ CORRECT: "change my email address to wux1992@gmail.com" → {"name": null, "email": "wux1992@gmail.com", "phone": null, "summary": null}

❌ WRONG: Extracting multiple fields when only one is mentioned
✅ CORRECT: Extract ONLY the field whose keyword appears in the text

Return ONLY valid JSON using this exact schema:
{
  "name": "<full name OR null if not mentioned>",
  "email": "<email address OR null if not mentioned>",
  "phone": "<phone number digits with optional +country code OR null if not mentioned>",
  "summary": "<1-2 sentence professional summary OR null if not mentioned>"
}

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

	// Detect which field the user intended to update using programmatic keyword detection
	// This overrides any incorrect LLM behavior
	intendedField := detectIntendedField(input)

	// Start with existing data
	result := existing

	// Only update the intended field (if detected) or all non-null fields (if not detected)
	switch intendedField {
	case "email":
		// Only update email field, ignore any other fields the LLM might have returned
		if val, ok := rawMap["email"]; ok && val != nil {
			if strVal, isString := val.(string); isString {
				result.Email = strings.TrimSpace(strVal)
			}
		}
	case "phone":
		// Only update phone field
		if val, ok := rawMap["phone"]; ok && val != nil {
			if strVal, isString := val.(string); isString {
				result.Phone = strings.TrimSpace(strVal)
			}
		}
	case "name":
		// Only update name field
		if val, ok := rawMap["name"]; ok && val != nil {
			if strVal, isString := val.(string); isString {
				result.Name = strings.TrimSpace(strVal)
			}
		}
	case "summary":
		// Only update summary field
		if val, ok := rawMap["summary"]; ok && val != nil {
			if strVal, isString := val.(string); isString {
				result.Summary = strings.TrimSpace(strVal)
			}
		}
	default:
		// No specific field detected, update all non-null fields (original behavior)
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
