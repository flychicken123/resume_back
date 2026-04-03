package services

import (
	"context"

	"github.com/tmc/langchaingo/llms"
)

// ChatTool defines a tool the chatbot can invoke via Gemini function calling.
type ChatTool struct {
	Name        string
	Description string
	Parameters  map[string]ToolParam
	Handler     func(ctx context.Context, userID int, args map[string]any) (any, error)
}

// ToolParam describes a parameter for a tool.
type ToolParam struct {
	Type        string   // "string", "integer", "boolean"
	Description string
	Required    bool
	Enum        []string
}

// ChatTools returns all available tools for the chatbot.
func ChatTools() []ChatTool {
	return []ChatTool{
		{
			Name:        "search_jobs",
			Description: "Search the job database for positions matching criteria. Use when the user asks about available jobs, job listings, or wants to find specific roles.",
			Parameters: map[string]ToolParam{
				"query":       {Type: "string", Description: "Job title or keywords to search for", Required: true},
				"location":    {Type: "string", Description: "Preferred location (city, state, or 'remote')"},
				"remote_only": {Type: "boolean", Description: "Only show remote positions"},
				"limit":       {Type: "integer", Description: "Max results to return (default 5)"},
			},
			Handler: handleSearchJobs,
		},
		{
			Name:        "get_job_matches",
			Description: "Get personalized job matches based on the user's resume. Use when the user asks about jobs that fit their profile or wants recommendations.",
			Parameters: map[string]ToolParam{
				"limit": {Type: "integer", Description: "Max results (default 5)"},
			},
			Handler: handleGetJobMatches,
		},
		{
			Name:        "track_application",
			Description: "Add a job to the user's application tracker. Use when the user says they applied somewhere or wants to track a job.",
			Parameters: map[string]ToolParam{
				"job_title":    {Type: "string", Description: "Job title", Required: true},
				"company_name": {Type: "string", Description: "Company name", Required: true},
				"job_url":      {Type: "string", Description: "Job listing URL"},
				"status":       {Type: "string", Description: "Application status", Enum: []string{"applied", "screening", "interviewing", "offered"}},
			},
			Handler: handleTrackApplication,
		},
		{
			Name:        "update_application_status",
			Description: "Update the status of a tracked job application. Use when user reports progress like getting an interview, offer, or rejection.",
			Parameters: map[string]ToolParam{
				"company_name": {Type: "string", Description: "Company to update", Required: true},
				"new_status":   {Type: "string", Description: "New status", Required: true, Enum: []string{"screening", "interviewing", "offered", "accepted", "rejected", "withdrawn"}},
			},
			Handler: handleUpdateApplicationStatus,
		},
		{
			Name:        "get_my_applications",
			Description: "List the user's tracked job applications. Use when user asks about their application pipeline or status.",
			Parameters: map[string]ToolParam{
				"status_filter": {Type: "string", Description: "Filter by status", Enum: []string{"applied", "screening", "interviewing", "offered", "accepted", "rejected"}},
			},
			Handler: handleGetMyApplications,
		},
		{
			Name:        "get_job_count",
			Description: "Get the count of jobs matching criteria in the platform database. Use when user asks how many jobs are available.",
			Parameters: map[string]ToolParam{
				"role":     {Type: "string", Description: "Job role/title to count"},
				"location": {Type: "string", Description: "Location filter"},
			},
			Handler: handleGetJobCount,
		},
	}
}

// ConvertToLLMTools converts ChatTool definitions to LangChain tool format.
func ConvertToLLMTools(tools []ChatTool) []llms.Tool {
	var llmTools []llms.Tool
	for _, t := range tools {
		properties := map[string]any{}
		var required []string
		for name, param := range t.Parameters {
			prop := map[string]any{
				"type":        param.Type,
				"description": param.Description,
			}
			if len(param.Enum) > 0 {
				prop["enum"] = param.Enum
			}
			properties[name] = prop
			if param.Required {
				required = append(required, name)
			}
		}

		llmTools = append(llmTools, llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters: map[string]any{
					"type":       "object",
					"properties": properties,
					"required":   required,
				},
			},
		})
	}
	return llmTools
}

// FindToolHandler looks up a tool by name and returns its handler.
func FindToolHandler(name string, tools []ChatTool) func(ctx context.Context, userID int, args map[string]any) (any, error) {
	for _, t := range tools {
		if t.Name == name {
			return t.Handler
		}
	}
	return nil
}
