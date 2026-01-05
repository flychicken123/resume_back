package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
)

// JobDescriptionAgent wraps an LLM client to manage job descriptions intelligently.
type JobDescriptionAgent struct {
	llm llms.Model
}

// JobDescriptionEntry represents a single job description entry.
type JobDescriptionEntry struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
	URL   string `json:"url"`
}

// JobDescriptionParseResult is the result of parsing user input for job descriptions.
type JobDescriptionParseResult struct {
	Action       string                `json:"action"`       // "add", "modify", "remove", or "none"
	TargetID     string                `json:"targetId"`     // ID of entry to modify/remove (if applicable)
	Title        string                `json:"title"`        // Job title extracted
	Text         string                `json:"text"`         // Job description text
	URL          string                `json:"url"`          // Job posting URL if provided
	Entries      []JobDescriptionEntry `json:"entries"`      // Updated list of all entries
	Message      string                `json:"message"`      // Confirmation message for the user
}

// NewJobDescriptionAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewJobDescriptionAgent() (*JobDescriptionAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(), googleai.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client for job description agent: %w", err)
	}

	return &JobDescriptionAgent{llm: llm}, nil
}

// ParseJobDescription analyzes user input and determines how to handle job descriptions.
func (a *JobDescriptionAgent) ParseJobDescription(ctx context.Context, input string, existing []JobDescriptionEntry) (JobDescriptionParseResult, error) {
	if a == nil || a.llm == nil {
		return JobDescriptionParseResult{}, fmt.Errorf("job description agent is not initialized")
	}

	existingContext := ""
	if len(existing) > 0 {
		existingContext = "\nExisting job descriptions:\n"
		for i, entry := range existing {
			existingContext += fmt.Sprintf("%d. ID: %s, Title: %s, URL: %s\n   Text preview: %s\n",
				i+1,
				entry.ID,
				entry.Title,
				entry.URL,
				truncateText(entry.Text, 100),
			)
		}
		existingContext += "\n"
	}

	prompt := fmt.Sprintf(`You are analyzing user input to manage job descriptions for a resume builder.
%s

The user may want to:
1. ADD a new job description (paste new job posting URL or text)
2. MODIFY an existing job description (update/change/edit existing entry)
3. REMOVE a job description (delete/remove/clear an entry)
4. Just provide information (no clear intent)

RULES:
1. If user pastes a job URL or job description text and there are NO existing entries, action is "add"
2. If user pastes a job URL or job description text and there IS an existing entry, action is "modify" the first/most recent entry
3. If user explicitly says "add another job" or "new job description", action is "add" even if entries exist
4. If user says "remove", "delete", "clear" a job description, action is "remove"
5. If user just wants to view or there's no job content, action is "none"
6. Extract the job title from the job description if possible
7. Extract any job posting URL if present

EXAMPLES:
- User pastes job description with no existing entries → action: "add"
- User pastes job description with 1 existing entry → action: "modify", targetId: first entry's ID
- "add another job description: [content]" → action: "add"
- "update the job description to: [content]" → action: "modify"
- "remove the job description" → action: "remove"
- "delete my job description" → action: "remove"
- "clear job description" → action: "remove"

Return ONLY valid JSON:
{
  "action": "<add|modify|remove|none>",
  "targetId": "<ID of entry to modify/remove, or empty string>",
  "title": "<extracted job title or empty string>",
  "text": "<job description text or empty string>",
  "url": "<job posting URL or empty string>",
  "message": "<brief confirmation message for the user>"
}

User input:
%s`, existingContext, input)

	raw, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return JobDescriptionParseResult{}, err
	}

	clean := sanitizeLLMJSON(raw)

	var result JobDescriptionParseResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return JobDescriptionParseResult{}, fmt.Errorf("failed to parse job description JSON: %w", err)
	}

	// Normalize fields
	result.Action = strings.TrimSpace(strings.ToLower(result.Action))
	result.TargetID = strings.TrimSpace(result.TargetID)
	result.Title = strings.TrimSpace(result.Title)
	result.Text = strings.TrimSpace(result.Text)
	result.URL = strings.TrimSpace(result.URL)
	result.Message = strings.TrimSpace(result.Message)

	// Build the updated entries list based on action
	result.Entries = a.applyAction(result, existing)

	return result, nil
}

// applyAction applies the parsed action to the existing entries and returns updated list.
func (a *JobDescriptionAgent) applyAction(result JobDescriptionParseResult, existing []JobDescriptionEntry) []JobDescriptionEntry {
	switch result.Action {
	case "add":
		// Add new entry
		newEntry := JobDescriptionEntry{
			ID:    generateEntryID(),
			Title: result.Title,
			Text:  result.Text,
			URL:   result.URL,
		}
		return append(existing, newEntry)

	case "modify":
		// Modify existing entry
		if len(existing) == 0 {
			// No existing entry, treat as add
			newEntry := JobDescriptionEntry{
				ID:    generateEntryID(),
				Title: result.Title,
				Text:  result.Text,
				URL:   result.URL,
			}
			return []JobDescriptionEntry{newEntry}
		}

		// Find target entry to modify
		targetIdx := 0
		if result.TargetID != "" {
			for i, entry := range existing {
				if entry.ID == result.TargetID {
					targetIdx = i
					break
				}
			}
		}

		// Update the target entry
		updated := make([]JobDescriptionEntry, len(existing))
		copy(updated, existing)
		if result.Title != "" {
			updated[targetIdx].Title = result.Title
		}
		if result.Text != "" {
			updated[targetIdx].Text = result.Text
		}
		if result.URL != "" {
			updated[targetIdx].URL = result.URL
		}
		return updated

	case "remove":
		// Remove entry
		if len(existing) == 0 {
			return existing
		}

		if result.TargetID == "" {
			// Remove the first/only entry
			if len(existing) == 1 {
				return []JobDescriptionEntry{}
			}
			return existing[1:]
		}

		// Remove specific entry by ID
		var updated []JobDescriptionEntry
		for _, entry := range existing {
			if entry.ID != result.TargetID {
				updated = append(updated, entry)
			}
		}
		return updated

	default:
		// No action, return existing unchanged
		return existing
	}
}

// generateEntryID creates a simple unique ID for a new entry.
func generateEntryID() string {
	return fmt.Sprintf("jd_%d", time.Now().UnixNano())
}

// truncateText truncates text to a maximum length with ellipsis.
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}
