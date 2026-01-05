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

// EducationAgent wraps an LLM client to extract structured education information from natural language.
type EducationAgent struct {
	llm llms.Model
}

// EducationRecord represents a structured education entry.
type EducationRecord struct {
	Degree         string `json:"degree"`
	Field          string `json:"field"`
	School         string `json:"school"`
	City           string `json:"city"`
	State          string `json:"state"`
	GraduationYear string `json:"graduationYear"`
	GPA            string `json:"gpa"`
	Honors         string `json:"honors"`
}

// EducationParseResult is the top-level payload returned by the agent.
type EducationParseResult struct {
	Education []EducationRecord `json:"education"`
}

// NewEducationAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewEducationAgent() (*EducationAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(), googleai.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client for education agent: %w", err)
	}

	return &EducationAgent{llm: llm}, nil
}

// ExtractEducation converts free-form text into structured education information.
func (a *EducationAgent) ExtractEducation(ctx context.Context, input string) (EducationParseResult, error) {
	return a.ExtractEducationWithContext(ctx, input, EducationParseResult{})
}

// ExtractEducationWithContext is like ExtractEducation but accepts existing data
// and intelligently merges, adds, or updates education based on user intent.
func (a *EducationAgent) ExtractEducationWithContext(ctx context.Context, input string, existing EducationParseResult) (EducationParseResult, error) {
	if a == nil || a.llm == nil {
		return EducationParseResult{}, fmt.Errorf("education agent is not initialized")
	}

	existingContext := ""
	if len(existing.Education) > 0 {
		existingContext = "\nExisting education (DO NOT modify unless the user explicitly mentions updating them):\n"
		for i, edu := range existing.Education {
			existingContext += fmt.Sprintf("%d. %s in %s from %s (%s)\n", i+1, edu.Degree, edu.Field, edu.School, edu.GraduationYear)
		}
		existingContext += "\n"
	}

	prompt := fmt.Sprintf(`Extract education information from the user's text and return as JSON.
%s

RULES:
1. Extract ONLY the fields that the user mentions. Set other fields to empty string.
2. For degree: extract the degree type (Bachelor's, Master's, PhD, etc.)
3. For field: extract the field of study or major
4. For school: extract the university or institution name
5. For city/state: extract location if mentioned
6. For graduationYear: extract the graduation year
7. For GPA: extract GPA if mentioned
8. For honors: extract honors, awards, distinctions if mentioned

EXAMPLES:
- "I have a Bachelor's in Computer Science from MIT" → {"education": [{"degree": "Bachelor's", "field": "Computer Science", "school": "MIT", "city": "", "state": "", "graduationYear": "", "gpa": "", "honors": ""}]}
- "graduated from Stanford in 2020 with a Master's in AI" → {"education": [{"degree": "Master's", "field": "AI", "school": "Stanford", "city": "", "state": "", "graduationYear": "2020", "gpa": "", "honors": ""}]}
- "change my school to Harvard" → update only school field of existing education
- "update my graduation year to 2023" → update only graduationYear field
- "remove my GPA" → set gpa to "" for the education entry
- "delete my education at MIT" → remove MIT from the education array
- "clear my honors" → set honors to "" for the education entry

FIELD UPDATE RULES:
- When modifying existing education, identify it by school name, degree, or context
- Only update fields explicitly mentioned in the user's message
- Preserve all other fields from the existing education
- For REMOVAL of a field (remove/delete/clear), set it to empty string ""
- For REMOVAL of an entire education entry, omit it from the returned array

Return ONLY valid JSON:
{"education": [{"degree": <string>, "field": <string>, "school": <string>, "city": <string>, "state": <string>, "graduationYear": <string>, "gpa": <string>, "honors": <string>}]}

User text: %s`, existingContext, input)

	response, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return EducationParseResult{}, err
	}

	clean := sanitizeLLMJSON(response)

	var result EducationParseResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return EducationParseResult{}, fmt.Errorf("failed to parse education JSON: %w", err)
	}

	// Basic normalization
	for i := range result.Education {
		edu := &result.Education[i]
		edu.Degree = strings.TrimSpace(edu.Degree)
		edu.Field = strings.TrimSpace(edu.Field)
		edu.School = strings.TrimSpace(edu.School)
		edu.City = strings.TrimSpace(edu.City)
		edu.State = strings.TrimSpace(edu.State)
		edu.GraduationYear = strings.TrimSpace(edu.GraduationYear)
		edu.GPA = strings.TrimSpace(edu.GPA)
		edu.Honors = strings.TrimSpace(edu.Honors)
	}

	return result, nil
}
