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

// ResumeModifyAgent detects resume modification intents and routes to appropriate handlers.
type ResumeModifyAgent struct {
	llm llms.Model
}

// ResumeModifyResult represents the result of analyzing a modification request.
type ResumeModifyResult struct {
	IsModification    bool                   `json:"isModification"`    // Whether this is a modification request
	Action            string                 `json:"action"`            // "create", "update", "delete", or "none"
	Section           string                 `json:"section"`           // "personal", "experience", "education", "projects", "skills", "summary"
	Field             string                 `json:"field"`             // Specific field if applicable (e.g., "name", "email", "phone")
	Value             interface{}            `json:"value"`             // The new value to set
	NeedsClarification bool                  `json:"needsClarification"` // Whether we need to ask user for clarification
	ClarificationQuestion string             `json:"clarificationQuestion"` // Question to ask user if clarification needed
	Message           string                 `json:"message"`           // Confirmation message for the user
	ParsedData        map[string]interface{} `json:"parsedData"`        // Structured data for the section
}

// NewResumeModifyAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewResumeModifyAgent() (*ResumeModifyAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(), googleai.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client for resume modify agent: %w", err)
	}

	return &ResumeModifyAgent{llm: llm}, nil
}

// AnalyzeModification analyzes user input to detect resume modification intent.
func (a *ResumeModifyAgent) AnalyzeModification(ctx context.Context, input string, existingData map[string]interface{}) (ResumeModifyResult, error) {
	if a == nil || a.llm == nil {
		return ResumeModifyResult{}, fmt.Errorf("resume modify agent is not initialized")
	}

	// Build context from existing data
	existingContext := buildExistingDataContext(existingData)

	prompt := fmt.Sprintf(`You are analyzing user input to detect if they want to modify their resume data.

%s

The user may want to:
1. CREATE/ADD new data to their resume
2. UPDATE/CHANGE existing data
3. DELETE/REMOVE data from their resume
4. Just ask a question (not a modification)

SECTIONS:
- personal: name, email, phone
- summary: professional summary text
- skills: list of skills
- experience: work experience entries (jobTitle, company, description, etc.)
- projects: project entries (projectName, description, technologies, etc.)
- education: education entries (degree, school, field, graduationYear, etc.)

RULES:
1. Detect if the input is asking to modify resume data
2. If it's a modification, identify the action (create/update/delete)
3. Identify which section and field to modify
4. Extract the new value if provided
5. If ambiguous (e.g., "change my title" could be job title in experience), set needsClarification=true
6. For experience/projects/education with multiple entries, if user doesn't specify which one, ask for clarification

EXAMPLES:
- "update my name to Lily" → isModification=true, action="update", section="personal", field="name", value="Lily"
- "change my email to test@gmail.com" → isModification=true, action="update", section="personal", field="email", value="test@gmail.com"
- "set my phone to 1234567890" → isModification=true, action="update", section="personal", field="phone", value="1234567890"
- "my name is John" → isModification=true, action="update", section="personal", field="name", value="John"
- "add Python to my skills" → isModification=true, action="update", section="skills", value="add Python"
- "remove JavaScript from skills" → isModification=true, action="update", section="skills", value="remove JavaScript"
- "change my summary to: Experienced developer..." → isModification=true, action="update", section="summary", value="Experienced developer..."
- "delete my summary" → isModification=true, action="delete", section="summary"
- "update my job title" → needsClarification=true (which experience entry?)
- "what is my name?" → isModification=false (just a question)
- "how do I export my resume?" → isModification=false (just a question)

Return ONLY valid JSON:
{
  "isModification": <boolean>,
  "action": "<create|update|delete|none>",
  "section": "<personal|summary|skills|experience|projects|education|none>",
  "field": "<specific field or empty string>",
  "value": <the new value or null>,
  "needsClarification": <boolean>,
  "clarificationQuestion": "<question to ask user if needsClarification is true>",
  "message": "<confirmation message for the user>"
}

User input: %s`, existingContext, input)

	raw, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return ResumeModifyResult{}, err
	}

	clean := sanitizeLLMJSON(raw)

	var result ResumeModifyResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return ResumeModifyResult{}, fmt.Errorf("failed to parse modification result JSON: %w", err)
	}

	// Normalize
	result.Action = strings.TrimSpace(strings.ToLower(result.Action))
	result.Section = strings.TrimSpace(strings.ToLower(result.Section))
	result.Field = strings.TrimSpace(strings.ToLower(result.Field))
	result.Message = strings.TrimSpace(result.Message)
	result.ClarificationQuestion = strings.TrimSpace(result.ClarificationQuestion)

	// Build parsedData for direct application
	if result.IsModification && !result.NeedsClarification && result.Action != "none" {
		result.ParsedData = buildParsedData(result)
	}

	return result, nil
}

func buildExistingDataContext(data map[string]interface{}) string {
	if len(data) == 0 {
		return "Current resume data: (empty)"
	}

	var b strings.Builder
	b.WriteString("Current resume data:\n")

	if name, ok := data["name"].(string); ok && name != "" {
		b.WriteString(fmt.Sprintf("- Name: %s\n", name))
	}
	if email, ok := data["email"].(string); ok && email != "" {
		b.WriteString(fmt.Sprintf("- Email: %s\n", email))
	}
	if phone, ok := data["phone"].(string); ok && phone != "" {
		b.WriteString(fmt.Sprintf("- Phone: %s\n", phone))
	}
	if summary, ok := data["summary"].(string); ok && summary != "" {
		b.WriteString(fmt.Sprintf("- Summary: %s\n", truncateText(summary, 100)))
	}
	if skills, ok := data["skills"].(string); ok && skills != "" {
		b.WriteString(fmt.Sprintf("- Skills: %s\n", skills))
	}

	if experiences, ok := data["experiences"].([]interface{}); ok && len(experiences) > 0 {
		b.WriteString("- Experience entries:\n")
		for i, exp := range experiences {
			if expMap, ok := exp.(map[string]interface{}); ok {
				title, _ := expMap["jobTitle"].(string)
				company, _ := expMap["company"].(string)
				b.WriteString(fmt.Sprintf("  %d. %s at %s\n", i+1, title, company))
			}
		}
	}

	if projects, ok := data["projects"].([]interface{}); ok && len(projects) > 0 {
		b.WriteString("- Project entries:\n")
		for i, proj := range projects {
			if projMap, ok := proj.(map[string]interface{}); ok {
				name, _ := projMap["projectName"].(string)
				b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, name))
			}
		}
	}

	if education, ok := data["education"].([]interface{}); ok && len(education) > 0 {
		b.WriteString("- Education entries:\n")
		for i, edu := range education {
			if eduMap, ok := edu.(map[string]interface{}); ok {
				degree, _ := eduMap["degree"].(string)
				school, _ := eduMap["school"].(string)
				b.WriteString(fmt.Sprintf("  %d. %s from %s\n", i+1, degree, school))
			}
		}
	}

	return b.String()
}

func buildParsedData(result ResumeModifyResult) map[string]interface{} {
	data := make(map[string]interface{})

	switch result.Section {
	case "personal":
		if result.Field != "" && result.Value != nil {
			data[result.Field] = result.Value
		}
	case "summary":
		if result.Action == "delete" {
			data["summary"] = ""
		} else if result.Value != nil {
			data["summary"] = result.Value
		}
	case "skills":
		if result.Value != nil {
			data["skills"] = result.Value
		}
	default:
		if result.Value != nil {
			data["value"] = result.Value
		}
	}

	return data
}
