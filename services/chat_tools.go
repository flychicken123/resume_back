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

// ChatTools returns the 6 merged tools for the chatbot.
// Fewer tools = more reliable LLM tool selection (recommended: 5-10 tools max).
func ChatTools() []ChatTool {
	return []ChatTool{
		{
			Name:        "job_search",
			Description: "Search jobs, count jobs, or get personalized matches. Use for ANY question about available jobs or the user's matches.",
			Parameters: map[string]ToolParam{
				"operation":   {Type: "string", Required: true, Enum: []string{"search", "count", "matches"}, Description: "search=find jobs by criteria, count=count matching jobs, matches=personalized matches"},
				"query":       {Type: "string", Description: "Keywords or job title to search for"},
				"location":    {Type: "string", Description: "Location filter (city, state, or 'remote')"},
				"remote_only": {Type: "boolean", Description: "Only show remote positions"},
				"limit":       {Type: "integer", Description: "Max results to return (default 5)"},
				"role":        {Type: "string", Description: "Job role/title to count (for count operation)"},
			},
			Handler: handleJobSearchDispatch,
		},
		{
			Name:        "application_manager",
			Description: "Track, update, or list job applications. Use for ANY request about the user's job applications — viewing, adding, or changing status.",
			Parameters: map[string]ToolParam{
				"operation":     {Type: "string", Required: true, Enum: []string{"track", "update_status", "list"}, Description: "track=add new application, update_status=change status, list=show all"},
				"company_name":  {Type: "string", Description: "Company name"},
				"job_title":     {Type: "string", Description: "Job title"},
				"job_url":       {Type: "string", Description: "Job listing URL (for track)"},
				"new_status":    {Type: "string", Description: "New status (for update_status)", Enum: []string{"applied", "screening", "interviewing", "offered", "accepted", "rejected", "withdrawn"}},
				"status_filter": {Type: "string", Description: "Filter by status (for list)"},
			},
			Handler: handleApplicationManagerDispatch,
		},
		{
			Name:        "resume_editor",
			Description: "Update resume fields, generate summary, fix grammar, generate or categorize skills. Use for ANY request to modify or generate resume content.",
			Parameters: map[string]ToolParam{
				"operation": {Type: "string", Required: true, Enum: []string{"update_field", "generate_summary", "improve_grammar", "generate_skills", "categorize_skills"}, Description: "What to do with the resume"},
				"field":     {Type: "string", Description: "Field to update (for update_field)", Enum: []string{"name", "email", "phone", "summary", "skills"}},
				"action":    {Type: "string", Description: "set/add/remove/clear (for update_field)"},
				"value":     {Type: "string", Description: "New value (for update_field)"},
				"section":   {Type: "string", Description: "Section for grammar fix", Enum: []string{"summary", "experience", "projects"}},
			},
			Handler: handleResumeEditorDispatch,
		},
		{
			Name:        "resume_optimizer",
			Description: "Optimize experience/projects for a job, or analyze resume quality. Use when user wants to IMPROVE, TAILOR, or get FEEDBACK on their resume.",
			Parameters: map[string]ToolParam{
				"operation": {Type: "string", Required: true, Enum: []string{"optimize_experience", "optimize_project", "analyze"}, Description: "What to optimize or analyze"},
			},
			Handler: handleResumeOptimizerDispatch,
		},
		{
			Name:        "letter_generator",
			Description: "Generate cover letters or recommendation letters. Use when user asks for any kind of letter.",
			Parameters: map[string]ToolParam{
				"operation":    {Type: "string", Required: true, Enum: []string{"cover_letter", "recommendation"}, Description: "Type of letter"},
				"company_name": {Type: "string", Description: "Target company"},
				"position":     {Type: "string", Description: "Target position"},
			},
			Handler: handleLetterGeneratorDispatch,
		},
		{
			Name:        "query_data",
			Description: "Query user data from the database using natural language, or clear user profile. Use for data lookups that other tools don't cover.",
			Parameters: map[string]ToolParam{
				"operation": {Type: "string", Required: true, Enum: []string{"query", "clear_profile"}, Description: "query=look up data, clear_profile=reset remembered preferences"},
				"question":  {Type: "string", Description: "What to look up (for query operation)"},
			},
			Handler: handleQueryDataDispatch,
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
