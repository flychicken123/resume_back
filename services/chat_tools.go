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
				"job_title":    {Type: "string", Description: "Job title to identify specific application (needed when multiple apps at same company)"},
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
		{
			Name:        "clear_my_profile",
			Description: "Clear all remembered preferences and facts about the user. Use ONLY when the user explicitly asks to forget their preferences, reset their profile, or start completely fresh.",
			Parameters:  map[string]ToolParam{},
			Handler:     handleClearProfile,
		},
		{
			Name:        "query_user_data",
			Description: "Query the user's data from the database using natural language. Can look up: job applications (status, companies, dates), job matches (scores, details), job postings (titles, locations, companies), and user profile. Use for ANY question about the user's personal data or the job database.",
			Parameters: map[string]ToolParam{
				"question": {Type: "string", Description: "What data to look up, in natural language", Required: true},
			},
			Handler: handleQueryUserData,
		},
		{
			Name:        "update_resume_field",
			Description: "Update a field in the user's resume. Use when the user asks to change their name, email, phone, summary, or add/remove skills via chat.",
			Parameters: map[string]ToolParam{
				"field":  {Type: "string", Description: "Field to update", Required: true, Enum: []string{"name", "email", "phone", "summary", "skills"}},
				"action": {Type: "string", Description: "Action to take", Required: true, Enum: []string{"set", "add", "remove", "clear"}},
				"value":  {Type: "string", Description: "New value, or skill to add/remove", Required: true},
			},
			Handler: handleUpdateResumeField,
		},
		// --- Resume feature tools (migrated from intent router) ---
		{
			Name:        "analyze_resume",
			Description: "Analyze the user's resume and provide expert feedback on strengths, weaknesses, and improvement areas. Use when user asks for resume review, analysis, or advice.",
			Parameters:  map[string]ToolParam{},
			Handler:     handleAnalyzeResume,
		},
		{
			Name:        "generate_cover_letter",
			Description: "Generate a personalized cover letter tailored to a job. Use when user asks for a cover letter.",
			Parameters: map[string]ToolParam{
				"company_name": {Type: "string", Description: "Target company name"},
			},
			Handler: handleGenerateCoverLetter,
		},
		{
			Name:        "generate_recommendation_letter",
			Description: "Generate a recommendation or reference letter. Use when user asks for a recommendation letter or third-party letter.",
			Parameters: map[string]ToolParam{
				"company_name": {Type: "string", Description: "Target company"},
				"position":     {Type: "string", Description: "Target position"},
			},
			Handler: handleGenerateRecommendationLetter,
		},
		{
			Name:        "generate_summary",
			Description: "Generate or rewrite the user's professional resume summary. Use when user asks to create, improve, or rewrite their summary.",
			Parameters:  map[string]ToolParam{},
			Handler:     handleGenerateSummary,
		},
		{
			Name:        "optimize_experience",
			Description: "Optimize and tailor work experience bullets for a job description. Use when user asks to optimize, tailor, or improve their experience section.",
			Parameters:  map[string]ToolParam{},
			Handler:     handleOptimizeExperience,
		},
		{
			Name:        "optimize_project",
			Description: "Optimize project descriptions for a job. Use when user asks to improve their projects section.",
			Parameters:  map[string]ToolParam{},
			Handler:     handleOptimizeProject,
		},
		{
			Name:        "improve_grammar",
			Description: "Fix grammar, spelling, and writing quality in resume sections. Use when user asks to fix grammar or improve writing.",
			Parameters: map[string]ToolParam{
				"section": {Type: "string", Description: "Section to fix", Enum: []string{"summary", "experience", "projects"}},
			},
			Handler: handleImproveGrammar,
		},
		{
			Name:        "generate_skills",
			Description: "Auto-generate relevant skills from the user's resume content. Use when user asks to suggest or add skills.",
			Parameters:  map[string]ToolParam{},
			Handler:     handleGenerateSkills,
		},
		{
			Name:        "categorize_skills",
			Description: "Organize the user's skills into categories (e.g., Languages, Frameworks, Cloud). Use when user asks to categorize or group their skills.",
			Parameters:  map[string]ToolParam{},
			Handler:     handleCategorizeSkills,
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
