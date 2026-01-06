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

// SkillsAgent wraps an LLM client to extract and generate skills.
type SkillsAgent struct {
	llm llms.Model
}

// SkillsParseResult represents the result of parsing skills from user input.
type SkillsParseResult struct {
	Action  string   `json:"action"`  // "add", "remove", "replace", "generate", or "none"
	Skills  []string `json:"skills"`  // List of skills
	Message string   `json:"message"` // Confirmation message for the user
}

// SkillsGenerateRequest contains context for generating skills.
type SkillsGenerateRequest struct {
	Experience []ExperienceRecord `json:"experience"`
	Projects   []ProjectRecord    `json:"projects"`
	Education  []EducationRecord  `json:"education"`
}

// NewSkillsAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewSkillsAgent() (*SkillsAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(), googleai.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client for skills agent: %w", err)
	}

	return &SkillsAgent{llm: llm}, nil
}

// ParseSkills analyzes user input and determines how to handle skills.
func (a *SkillsAgent) ParseSkills(ctx context.Context, input string, existing []string) (SkillsParseResult, error) {
	if a == nil || a.llm == nil {
		return SkillsParseResult{}, fmt.Errorf("skills agent is not initialized")
	}

	existingContext := ""
	if len(existing) > 0 {
		existingContext = fmt.Sprintf("\nExisting skills: %s\n", strings.Join(existing, ", "))
	}

	prompt := fmt.Sprintf(`You are analyzing user input to manage skills for a resume builder.
%s

The user may want to:
1. ADD new skills to the existing list
2. REMOVE specific skills from the list
3. REPLACE all skills with new ones
4. GENERATE skills from their resume data (will be handled separately)
5. Just provide information (no clear intent)

RULES:
1. If user says "I have skill1, skill2..." or "my skills are..." → action is "add" or "replace" depending on context
2. If user says "add skill1" or "also know skill2" → action is "add"
3. If user says "remove skill1" or "delete skill2" → action is "remove"
4. If user says "change my skills to..." or "my skills are..." (replacing all) → action is "replace"
5. If user asks to "generate skills" or "suggest skills from my experience" → action is "generate"
6. If no clear skill content, action is "none"

EXAMPLES:
- "I have Python, JavaScript, React" → action: "replace", skills: ["Python", "JavaScript", "React"]
- "my skills are Java, Spring Boot, AWS" → action: "replace", skills: ["Java", "Spring Boot", "AWS"]
- "add TypeScript to my skills" → action: "add", skills: ["TypeScript"]
- "I also know Docker and Kubernetes" → action: "add", skills: ["Docker", "Kubernetes"]
- "remove Python from my skills" → action: "remove", skills: ["Python"]
- "delete JavaScript" → action: "remove", skills: ["JavaScript"]
- "generate skills from my experience" → action: "generate", skills: []
- "suggest skills based on my projects" → action: "generate", skills: []

Return ONLY valid JSON:
{
  "action": "<add|remove|replace|generate|none>",
  "skills": ["skill1", "skill2", ...],
  "message": "<brief confirmation message for the user>"
}

User input:
%s`, existingContext, input)

	raw, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return SkillsParseResult{}, err
	}

	clean := sanitizeLLMJSON(raw)

	var result SkillsParseResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return SkillsParseResult{}, fmt.Errorf("failed to parse skills JSON: %w", err)
	}

	// Normalize
	result.Action = strings.TrimSpace(strings.ToLower(result.Action))
	result.Message = strings.TrimSpace(result.Message)

	// Normalize skills - trim whitespace
	normalizedSkills := make([]string, 0, len(result.Skills))
	for _, skill := range result.Skills {
		trimmed := strings.TrimSpace(skill)
		if trimmed != "" {
			normalizedSkills = append(normalizedSkills, trimmed)
		}
	}
	result.Skills = normalizedSkills

	// Apply action to compute final skills list
	result.Skills = a.applyAction(result.Action, result.Skills, existing)

	return result, nil
}

// applyAction applies the parsed action to compute the final skills list.
func (a *SkillsAgent) applyAction(action string, parsed []string, existing []string) []string {
	switch action {
	case "add":
		// Add new skills to existing (avoid duplicates)
		skillSet := make(map[string]bool)
		for _, s := range existing {
			skillSet[strings.ToLower(s)] = true
		}
		result := append([]string{}, existing...)
		for _, s := range parsed {
			if !skillSet[strings.ToLower(s)] {
				result = append(result, s)
				skillSet[strings.ToLower(s)] = true
			}
		}
		return result

	case "remove":
		// Remove specified skills from existing
		removeSet := make(map[string]bool)
		for _, s := range parsed {
			removeSet[strings.ToLower(s)] = true
		}
		var result []string
		for _, s := range existing {
			if !removeSet[strings.ToLower(s)] {
				result = append(result, s)
			}
		}
		return result

	case "replace":
		// Replace all skills with new ones
		return parsed

	case "generate":
		// Return existing - generation will be handled separately
		return existing

	default:
		// No action, return existing
		return existing
	}
}

// GenerateSkills uses LLM to generate skills from resume data.
func (a *SkillsAgent) GenerateSkills(ctx context.Context, req SkillsGenerateRequest, existing []string) (SkillsParseResult, error) {
	if a == nil || a.llm == nil {
		return SkillsParseResult{}, fmt.Errorf("skills agent is not initialized")
	}

	// Build context from resume data
	var contextParts []string

	if len(req.Experience) > 0 {
		contextParts = append(contextParts, "Work Experience:")
		for _, exp := range req.Experience {
			entry := fmt.Sprintf("- %s at %s", exp.JobTitle, exp.Company)
			if exp.Description != "" {
				entry += fmt.Sprintf(": %s", truncateText(exp.Description, 200))
			}
			contextParts = append(contextParts, entry)
		}
	}

	if len(req.Projects) > 0 {
		contextParts = append(contextParts, "\nProjects:")
		for _, proj := range req.Projects {
			entry := fmt.Sprintf("- %s", proj.ProjectName)
			if proj.Description != "" {
				entry += fmt.Sprintf(": %s", truncateText(proj.Description, 200))
			}
			if proj.Technologies != "" {
				entry += fmt.Sprintf(" (Tech: %s)", proj.Technologies)
			}
			contextParts = append(contextParts, entry)
		}
	}

	if len(req.Education) > 0 {
		contextParts = append(contextParts, "\nEducation:")
		for _, edu := range req.Education {
			entry := fmt.Sprintf("- %s in %s from %s", edu.Degree, edu.Field, edu.School)
			contextParts = append(contextParts, entry)
		}
	}

	if len(contextParts) == 0 {
		return SkillsParseResult{
			Action:  "none",
			Skills:  existing,
			Message: "No experience, projects, or education found to generate skills from. Please add some information first.",
		}, nil
	}

	resumeContext := strings.Join(contextParts, "\n")

	existingContext := ""
	if len(existing) > 0 {
		existingContext = fmt.Sprintf("\nExisting skills (include these and add more): %s\n", strings.Join(existing, ", "))
	}

	prompt := fmt.Sprintf(`Based on the following resume information, generate a comprehensive list of relevant technical and professional skills.
%s
%s

RULES:
1. Extract skills mentioned in job titles, descriptions, and technologies
2. Infer related skills based on the work experience and projects
3. Include both technical skills (programming languages, frameworks, tools) and soft skills if evident
4. Keep skills concise (1-3 words each)
5. Return 10-20 most relevant skills
6. Avoid duplicates
7. Order by relevance (most important first)

Return ONLY valid JSON:
{
  "action": "replace",
  "skills": ["skill1", "skill2", ...],
  "message": "Generated skills based on your experience and projects."
}

Resume information:
%s`, existingContext, "", resumeContext)

	raw, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return SkillsParseResult{}, err
	}

	clean := sanitizeLLMJSON(raw)

	var result SkillsParseResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return SkillsParseResult{}, fmt.Errorf("failed to parse generated skills JSON: %w", err)
	}

	// Normalize
	result.Action = "replace"
	result.Message = strings.TrimSpace(result.Message)
	if result.Message == "" {
		result.Message = "Generated skills based on your experience and projects."
	}

	// Normalize skills
	normalizedSkills := make([]string, 0, len(result.Skills))
	for _, skill := range result.Skills {
		trimmed := strings.TrimSpace(skill)
		if trimmed != "" {
			normalizedSkills = append(normalizedSkills, trimmed)
		}
	}
	result.Skills = normalizedSkills

	return result, nil
}
