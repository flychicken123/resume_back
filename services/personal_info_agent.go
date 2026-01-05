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

	// Check for modification/removal keywords that indicate a single-field update
	hasModifyKeyword := strings.Contains(lower, "change my") || strings.Contains(lower, "update my") ||
		strings.Contains(lower, "set my") || strings.Contains(lower, "modify my") ||
		strings.Contains(lower, "remove my") || strings.Contains(lower, "delete my") ||
		strings.Contains(lower, "clear my")

	// Check for "my X is Y" pattern (e.g., "my email is X", "my phone is Y")
	// Note: "my name is" is excluded because it's a common introduction phrase, not a field update
	hasMyXIsPattern := strings.Contains(lower, "my email is") || strings.Contains(lower, "my phone is") ||
		strings.Contains(lower, "my number is")

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
		strings.Contains(lower, "cell") || strings.Contains(lower, "telephone") ||
		strings.Contains(lower, "my number is")
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

	// Handle "my X is Y" patterns - single field updates
	if hasMyXIsPattern && fieldCount == 1 {
		if strings.Contains(lower, "my email is") {
			return "email"
		}
		if strings.Contains(lower, "my phone is") || strings.Contains(lower, "my number is") {
			return "phone"
		}
	}

	// If modification/removal keyword is present with a single field, return that field
	// This handles cases like "change my email to X" or "remove my email"
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

	// Detect email address pattern
	emailPattern := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	hasEmailAddress := emailPattern.MatchString(input)
	hasAnyModifyWord := strings.Contains(lower, "change") || strings.Contains(lower, "update") ||
		strings.Contains(lower, "set") || strings.Contains(lower, "modify")

	// Email-only update if:
	// - Input contains @ symbol pattern (email address)
	// - AND contains "change", "update", "set" (not necessarily "change my")
	// - AND only one field keyword is present (email)
	// - AND NO name keywords are present
	if hasEmailAddress && hasAnyModifyWord && hasEmailKeyword && !hasNameKeyword && !hasPhoneKeyword {
		return "email"
	}

	// If input contains ONLY an email address (and no other field keywords),
	// assume user wants to update email field
	if hasEmailAddress && !hasNameKeyword && !hasPhoneKeyword && !hasSummaryKeyword {
		return "email"
	}

	// Detect phone number pattern (digits with optional formatting)
	phonePattern := regexp.MustCompile(`[\d\s()+-]{6,}`)
	hasPhoneNumber := phonePattern.MatchString(input)

	// Phone-only update if input contains phone keyword and phone number pattern
	if hasPhoneNumber && hasPhoneKeyword && !hasNameKeyword && !hasEmailKeyword && !hasSummaryKeyword {
		return "phone"
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

	prompt := fmt.Sprintf(`Extract personal information from the user's text and return as JSON.
%s

RULES:
1. Extract ONLY the field that the user is updating. Set other fields to null.
2. For email: extract the email address (contains @)
3. For phone: extract the phone number (digits only)
4. For name: extract the person's name
5. For summary: extract the professional description
6. For REMOVAL requests (remove/delete/clear): return null for that field to indicate removal

EXAMPLES:
- "change my email to test@gmail.com" → {"name": null, "email": "test@gmail.com", "phone": null, "summary": null}
- "my email is john@example.com" → {"name": null, "email": "john@example.com", "phone": null, "summary": null}
- "my name is John Doe" → {"name": "John Doe", "email": null, "phone": null, "summary": null}
- "my phone is 1234567890" → {"name": null, "email": null, "phone": "1234567890", "summary": null}
- "change my phone number to 9876543210" → {"name": null, "email": null, "phone": "9876543210", "summary": null}
- "remove my email" → {"email": null} (return ONLY the field being removed)
- "delete my phone number" → {"phone": null}
- "clear my summary" → {"summary": null}
- "remove my name" → {"name": null}

Return ONLY valid JSON:
{"name": <string or null>, "email": <string or null>, "phone": <string or null>, "summary": <string or null>}

User text: %s`, existingContext, input)

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

	// Helper to extract string value from LLM response (handles both string and null)
	getStringValue := func(val interface{}) (string, bool) {
		if val == nil {
			return "", true // null means removal/clear
		}
		if strVal, isString := val.(string); isString {
			return strings.TrimSpace(strVal), true
		}
		return "", false
	}

	// Only update the intended field (if detected) or all non-null fields (if not detected)
	switch intendedField {
	case "email":
		// Update email field - null or empty string means removal
		if val, ok := rawMap["email"]; ok {
			if strVal, valid := getStringValue(val); valid {
				result.Email = strVal
			}
		}
	case "phone":
		// Update phone field - null or empty string means removal
		if val, ok := rawMap["phone"]; ok {
			if strVal, valid := getStringValue(val); valid {
				result.Phone = strVal
			}
		}
	case "name":
		// Update name field - null or empty string means removal
		if val, ok := rawMap["name"]; ok {
			if strVal, valid := getStringValue(val); valid {
				result.Name = strVal
			}
		}
	case "summary":
		// Update summary field - null or empty string means removal
		if val, ok := rawMap["summary"]; ok {
			if strVal, valid := getStringValue(val); valid {
				result.Summary = strVal
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
