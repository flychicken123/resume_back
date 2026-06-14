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

// ExperienceAgent wraps an LLM client to extract structured work experience
// (including per-role projects) from natural language.
type ExperienceAgent struct {
	llm llms.Model
}

// ExperienceRecord represents a structured work experience entry aligned with
// the resume builder's Experience model.
type ExperienceRecord struct {
	JobTitle         string `json:"jobTitle"`
	Company          string `json:"company"`
	City             string `json:"city"`
	State            string `json:"state"`
	Remote           bool   `json:"remote"`
	StartDate        string `json:"startDate"`
	EndDate          string `json:"endDate"`
	CurrentlyWorking bool   `json:"currentlyWorking"`
	Description      string `json:"description"`
}

// ExperienceParseResult is the top-level payload returned by the agent.
type ExperienceParseResult struct {
	Experiences []ExperienceRecord `json:"experiences"`
}

// NewExperienceAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewExperienceAgent() (*ExperienceAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(),
		googleai.WithAPIKey(apiKey),
		googleai.WithDefaultModel(DefaultGeminiGenerationModel),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client for experience agent: %w", err)
	}

	return &ExperienceAgent{llm: llm}, nil
}

// ExtractExperiences converts free-form text into a list of structured
// experiences, each with optional projects, using the LLM.
func (a *ExperienceAgent) ExtractExperiences(ctx context.Context, input string) (ExperienceParseResult, error) {
	return a.ExtractExperiencesWithContext(ctx, input, ExperienceParseResult{})
}

// ExtractExperiencesWithContext is like ExtractExperiences but accepts existing data
// and intelligently merges, adds, or updates experiences based on user intent.
func (a *ExperienceAgent) ExtractExperiencesWithContext(ctx context.Context, input string, existing ExperienceParseResult) (ExperienceParseResult, error) {
	if a == nil || a.llm == nil {
		return ExperienceParseResult{}, fmt.Errorf("experience agent is not initialized")
	}

	existingContext := ""
	if len(existing.Experiences) > 0 {
		existingContext = "\nExisting experiences (DO NOT modify unless the user explicitly mentions updating them):\n"
		for i, exp := range existing.Experiences {
			existingContext += fmt.Sprintf("%d. %s at %s (%s to %s) - %s (Remote: %t)\n",
				i+1,
				exp.JobTitle,
				exp.Company,
				exp.StartDate,
				exp.EndDate,
				exp.City,
				exp.Remote,
			)
		}
		existingContext += "\n"
	}

	prompt := fmt.Sprintf(`You are an assistant that extracts structured WORK EXPERIENCE from natural language.
%s

Your goal is to turn the user's text into one or more experience entries that match this JSON schema:

{
  "experiences": [
    {
      "jobTitle": "<job title or empty string if unknown>",
      "company": "<company name or empty string>",
      "city": "<city or empty string>",
      "state": "<state/region or empty string>",
      "remote": <true if the role is remote, false if onsite/hybrid or not specified>,
      "startDate": "<start date in YYYY-MM format if possible, or any reasonable date string, or empty string>",
      "endDate": "<end date in YYYY-MM format if possible, or any reasonable date string, or empty string>",
      "currentlyWorking": <true if this is the current role, otherwise false>,
      "description": "<1-3 sentence summary of responsibilities and impact for this role, including any project details>"
    }
  ]
}

CRITICAL RULES FOR PARTIAL UPDATES:
- If existing experiences are provided above, determine the user's intent:
  * ADDING: User says "add experience at Company X" or describes a completely new role → append to experiences array
  * MODIFYING: User says "change my title at Company Y to Z" or "update my role at Company X" → find that specific experience and update only the mentioned field(s), preserve everything else
  * REMOVING FIELD: User says "remove/delete/clear my description at Company X" → set that field to empty string ""
  * REMOVING ENTRY: User says "remove/delete my experience at Company X" → remove that entire experience from the array
  * REPLACING ALL: User provides a complete work history → replace all existing experiences with the new list

FIELD UPDATE RULES:
- When modifying an existing experience, identify it by company name, job title, or context clues
- When modifying, reuse the exact company name from the existing experience if the user references it, even if the user input has typos
- Only update fields explicitly mentioned in the user's message
- For the remote field, set it only when the user explicitly mentions remote/onsite/hybrid/in-person; otherwise preserve the existing value
- Preserve all other fields from the existing experience
- For REMOVAL of a field, set it to empty string ""
- For REMOVAL of an entire experience, omit it from the returned array
- Examples:
  * "change my job title at Microsoft to Senior Engineer" → find Microsoft entry, update only jobTitle field
  * "update my end date at Google to 2024-12" → find Google entry, update only endDate field
  * "add description: led a team of 5 engineers at Amazon" → find Amazon entry, update only description field
  * "remove my description at Microsoft" → find Microsoft entry, set description to ""
  * "delete my experience at Google" → remove Google entry from the array entirely

GENERAL RULES:
- Never invent employers, dates, or projects that are not implied by the text.
- If the text clearly mentions multiple NEW jobs (e.g., Microsoft then Amazon), create multiple entries in "experiences".
- If dates are approximate (e.g., "from 2018 to 2025"), use a best-effort YYYY-MM (e.g., "2018-07") or just the year; if you truly can't infer, use an empty string.
- For location like "Redmond WA", split into city "Redmond" and state "WA" when possible.
- Include all project details as part of the experience description.
- Do NOT use placeholder text in square brackets like "[Project 1 Name]" or "[Briefly describe the project]".
- If you don't know specific project names or details, describe them naturally (for example, "an internal cost analytics platform") instead of leaving blanks or bracketed prompts.

OUTPUT FORMAT:
- Return ONLY a single JSON object matching the schema above with ALL experiences (existing + new/modified)
- If modifying an existing experience, return the full experiences array with the modification applied
- Do not include any explanations, comments, or additional top-level fields.

User text:
%s`, existingContext, input)

	raw, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return ExperienceParseResult{}, err
	}

	clean := sanitizeLLMJSON(raw)

	var result ExperienceParseResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return ExperienceParseResult{}, fmt.Errorf("failed to parse experience JSON: %w", err)
	}

	// Basic normalization: trim spaces and ensure non-nil slices.
	for i := range result.Experiences {
		exp := &result.Experiences[i]
		exp.JobTitle = strings.TrimSpace(exp.JobTitle)
		exp.Company = strings.TrimSpace(exp.Company)
		exp.City = strings.TrimSpace(exp.City)
		exp.State = strings.TrimSpace(exp.State)
		exp.StartDate = strings.TrimSpace(exp.StartDate)
		exp.EndDate = strings.TrimSpace(exp.EndDate)
		exp.Description = strings.TrimSpace(exp.Description)
	}

	return result, nil
}
