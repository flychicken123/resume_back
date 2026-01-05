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

// ProjectsAgent wraps an LLM client to extract structured project information from natural language.
type ProjectsAgent struct {
	llm llms.Model
}

// ProjectRecord represents a structured project entry.
type ProjectRecord struct {
	ProjectName  string `json:"projectName"`
	Description  string `json:"description"`
	Technologies string `json:"technologies"`
	ProjectURL   string `json:"projectUrl"`
	StartDate    string `json:"startDate"`
	EndDate      string `json:"endDate"`
}

// ProjectsParseResult is the top-level payload returned by the agent.
type ProjectsParseResult struct {
	Projects []ProjectRecord `json:"projects"`
}

// NewProjectsAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewProjectsAgent() (*ProjectsAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(), googleai.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client for projects agent: %w", err)
	}

	return &ProjectsAgent{llm: llm}, nil
}

// ExtractProjects converts free-form text into structured project information.
func (a *ProjectsAgent) ExtractProjects(ctx context.Context, input string) (ProjectsParseResult, error) {
	return a.ExtractProjectsWithContext(ctx, input, ProjectsParseResult{})
}

// ExtractProjectsWithContext is like ExtractProjects but accepts existing data
// and intelligently merges, adds, or updates projects based on user intent.
func (a *ProjectsAgent) ExtractProjectsWithContext(ctx context.Context, input string, existing ProjectsParseResult) (ProjectsParseResult, error) {
	if a == nil || a.llm == nil {
		return ProjectsParseResult{}, fmt.Errorf("projects agent is not initialized")
	}

	existingContext := ""
	if len(existing.Projects) > 0 {
		existingContext = "\nExisting projects (DO NOT modify unless the user explicitly mentions updating them):\n"
		for i, proj := range existing.Projects {
			existingContext += fmt.Sprintf("%d. %s - %s\n", i+1, proj.ProjectName, proj.Description)
		}
		existingContext += "\n"
	}

	prompt := fmt.Sprintf(`Extract project information from the user's text and return as JSON.
%s

RULES:
1. Extract ONLY the fields that the user mentions. Set other fields to empty string.
2. For project name: extract the name of the project
3. For description: extract what the project does or accomplishes
4. For technologies: extract tech stack, languages, frameworks mentioned
5. For URL: extract any project link or URL mentioned
6. For dates: extract start/end dates if mentioned

EXAMPLES:
- "add project called TaskManager using React and Node.js" → {"projects": [{"projectName": "TaskManager", "description": "", "technologies": "React, Node.js", "projectUrl": "", "startDate": "", "endDate": ""}]}
- "my project is an e-commerce platform built with Python" → {"projects": [{"projectName": "e-commerce platform", "description": "", "technologies": "Python", "projectUrl": "", "startDate": "", "endDate": ""}]}
- "change project name to NewApp" → update only projectName field of existing project
- "update project description to: A mobile app for tracking fitness" → update only description field
- "remove the technologies from TaskManager" → set technologies to "" for TaskManager project
- "delete project TaskManager" → remove TaskManager from the projects array
- "clear project description" → set description to "" for the project

FIELD UPDATE RULES:
- When modifying an existing project, identify it by name or context
- Only update fields explicitly mentioned in the user's message
- Preserve all other fields from the existing project
- For REMOVAL of a field (remove/delete/clear), set it to empty string ""
- For REMOVAL of an entire project, omit it from the returned array

Return ONLY valid JSON:
{"projects": [{"projectName": <string>, "description": <string>, "technologies": <string>, "projectUrl": <string>, "startDate": <string>, "endDate": <string>}]}

User text: %s`, existingContext, input)

	response, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return ProjectsParseResult{}, err
	}

	clean := sanitizeLLMJSON(response)

	var result ProjectsParseResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return ProjectsParseResult{}, fmt.Errorf("failed to parse projects JSON: %w", err)
	}

	// Basic normalization
	for i := range result.Projects {
		proj := &result.Projects[i]
		proj.ProjectName = strings.TrimSpace(proj.ProjectName)
		proj.Description = strings.TrimSpace(proj.Description)
		proj.Technologies = strings.TrimSpace(proj.Technologies)
		proj.ProjectURL = strings.TrimSpace(proj.ProjectURL)
		proj.StartDate = strings.TrimSpace(proj.StartDate)
		proj.EndDate = strings.TrimSpace(proj.EndDate)
	}

	return result, nil
}
