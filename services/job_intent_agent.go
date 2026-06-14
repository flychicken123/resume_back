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

// JobIntentAgent uses an LLM to understand mixed job URL + description input.
type JobIntentAgent struct {
	llm llms.Model
}

// JobIntentResult captures the structured interpretation of the user's message.
//
// URL is the first job posting URL mentioned in the text (if any).
// JobDescriptionFromText is any job description content the user pasted or described directly.
type JobIntentResult struct {
	URL                    string `json:"url"`
	JobDescriptionFromText string `json:"job_description_from_text"`
}

// NewJobIntentAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewJobIntentAgent() (*JobIntentAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(),
		googleai.WithAPIKey(apiKey),
		googleai.WithDefaultModel(DefaultGeminiGenerationModel),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client: %w", err)
	}

	return &JobIntentAgent{llm: llm}, nil
}

// ParseJobIntent uses the LLM to extract a job posting URL (if present)
// and any job description text the user supplied directly in the message.
func (a *JobIntentAgent) ParseJobIntent(ctx context.Context, input string) (JobIntentResult, error) {
	if a == nil || a.llm == nil {
		return JobIntentResult{}, fmt.Errorf("job intent agent is not initialized")
	}

	prompt := fmt.Sprintf(`You will receive a message from a user who may:
- Paste a job posting URL (e.g., from Greenhouse, Ashby, Lever, Workday, etc.)
- Paste the full job description text
- Do both in the same message

Your job is to extract:
- The first job posting URL, if one is present
- Any job description text the user provided directly (not by visiting the URL)

CRITICAL RULES:
- NEVER invent a URL. Only return a URL if it is explicitly present in the text.
- If multiple URLs are present, pick the one that most clearly looks like a job posting.
- The job_description_from_text field should contain ONLY job description content that is written in the message itself
  (e.g., responsibilities, requirements, company blurb). Do NOT try to fetch or hallucinate content from any URL.
- If the user only says something like "use this job" and only gives a URL, leave job_description_from_text as an empty string.
- If the user only pastes job description text and no URL, set url to an empty string.
- If both URL and text are present, fill in BOTH fields.
- Do not include explanations, comments, or any extra keys.

Return ONLY a single JSON object with this exact shape and key names:
{
  "url": "<job posting URL or empty string>",
  "job_description_from_text": "<job description text from the message itself or empty string>"
}

User message:
%s`, input)

	raw, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return JobIntentResult{}, err
	}

	clean := sanitizeLLMJSON(raw)

	var result JobIntentResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return JobIntentResult{}, fmt.Errorf("failed to parse job intent JSON: %w", err)
	}

	result.URL = strings.TrimSpace(result.URL)
	result.JobDescriptionFromText = strings.TrimSpace(result.JobDescriptionFromText)

	return result, nil
}
