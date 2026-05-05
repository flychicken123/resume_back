package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"resumeai/models"
)

// ToolRegistry holds model references needed by tool handlers.
type ToolRegistry struct {
	JobPostingModel  *models.JobPostingModel
	JobAppModel      *models.JobApplicationModel
	JobMatchModel    *models.ResumeJobMatchModel
	ChatProfileModel *models.UserChatProfileModel
	DB               *sql.DB
}

var toolRegistry *ToolRegistry

type chatToolRequestContextKey struct{}

// ChatToolRequestContext carries per-request data needed by tool handlers.
type ChatToolRequestContext struct {
	ResumeData          map[string]any
	JobDescription      string
	ConversationHistory string
}

func WithChatToolRequestContext(ctx context.Context, value ChatToolRequestContext) context.Context {
	return context.WithValue(ctx, chatToolRequestContextKey{}, value)
}

func chatToolRequestContext(ctx context.Context) ChatToolRequestContext {
	if ctx == nil {
		return ChatToolRequestContext{}
	}
	value, _ := ctx.Value(chatToolRequestContextKey{}).(ChatToolRequestContext)
	return value
}

// SetToolRegistry injects model dependencies for tool handlers.
func SetToolRegistry(r *ToolRegistry) {
	toolRegistry = r
}

// SetRequestContext is kept for older callers. New chat requests pass this data
// through context with WithChatToolRequestContext to avoid cross-request races.
func SetRequestContext(resumeData map[string]any, jobDesc, conversationHistory string) {
}

func getStringArg(args map[string]any, key, fallback string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return fallback
}

func getIntArg(args map[string]any, key string, fallback int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case json.Number:
			if i, err := n.Int64(); err == nil {
				return int(i)
			}
		}
	}
	return fallback
}

func getBoolArg(args map[string]any, key string) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func handleSearchJobs(ctx context.Context, userID int, args map[string]any) (any, error) {
	if toolRegistry == nil || toolRegistry.JobPostingModel == nil {
		return map[string]any{"error": "job search not available"}, nil
	}
	query := getStringArg(args, "query", "")
	location := getStringArg(args, "location", "")
	remoteOnly := getBoolArg(args, "remote_only")
	limit := getIntArg(args, "limit", 5)
	if limit > 10 {
		limit = 10
	}

	// Build SQL search with ILIKE
	sqlQuery := `
		SELECT jp.id, jp.title, COALESCE(jc.name, '') AS company,
		       COALESCE(jp.location, '') AS location, COALESCE(jp.remote_type, '') AS remote_type,
		       COALESCE(jp.job_url, '') AS job_url
		FROM job_postings jp
		LEFT JOIN job_companies jc ON jc.id = jp.company_id
		WHERE jp.is_active = TRUE`
	var queryArgs []any
	argIdx := 1

	if query != "" {
		sqlQuery += fmt.Sprintf(` AND (jp.title ILIKE $%d OR jp.description ILIKE $%d)`, argIdx, argIdx)
		queryArgs = append(queryArgs, "%"+query+"%")
		argIdx++
	}
	if location != "" {
		sqlQuery += fmt.Sprintf(` AND jp.location ILIKE $%d`, argIdx)
		queryArgs = append(queryArgs, "%"+location+"%")
		argIdx++
	}
	if remoteOnly {
		sqlQuery += ` AND LOWER(jp.remote_type) LIKE '%remote%'`
	}
	sqlQuery += fmt.Sprintf(` ORDER BY jp.posted_at DESC NULLS LAST LIMIT $%d`, argIdx)
	queryArgs = append(queryArgs, limit)

	rows, err := toolRegistry.JobPostingModel.DB().QueryContext(ctx, sqlQuery, queryArgs...)
	if err != nil {
		return map[string]any{"error": "search failed"}, nil
	}
	defer rows.Close()

	var results []map[string]string
	for rows.Next() {
		var id int64
		var title, company, loc, remote, url string
		if err := rows.Scan(&id, &title, &company, &loc, &remote, &url); err != nil {
			continue
		}
		results = append(results, map[string]string{
			"title": title, "company": company, "location": loc,
			"remote_type": remote, "job_url": url,
		})
	}
	if len(results) == 0 {
		return map[string]any{"message": "No jobs found matching your criteria", "count": 0}, nil
	}
	return map[string]any{"jobs": results, "count": len(results)}, nil
}

func handleGetJobMatches(ctx context.Context, userID int, args map[string]any) (any, error) {
	if toolRegistry == nil || toolRegistry.JobMatchModel == nil {
		return map[string]any{"error": "job matching not available"}, nil
	}
	if userID == 0 {
		return map[string]any{"error": "please log in to see your job matches"}, nil
	}
	limit := getIntArg(args, "limit", 5)
	if limit > 10 {
		limit = 10
	}

	matches, _, err := toolRegistry.JobMatchModel.ListMostRecentByUser(userID, limit)
	if err != nil || len(matches) == 0 {
		return map[string]any{"message": "No job matches found. Try refreshing your matches from the Job Matches page.", "count": 0}, nil
	}

	var results []map[string]any
	for _, m := range matches {
		company := ""
		if m.CompanyName != nil {
			company = *m.CompanyName
		}
		results = append(results, map[string]any{
			"title":       m.JobTitle,
			"company":     company,
			"location":    m.JobLocation,
			"match_score": m.MatchScore,
			"job_url":     m.JobURL,
		})
	}
	return map[string]any{"matches": results, "count": len(results)}, nil
}

func handleTrackApplication(ctx context.Context, userID int, args map[string]any) (any, error) {
	if toolRegistry == nil || toolRegistry.JobAppModel == nil {
		return map[string]any{"error": "application tracking not available"}, nil
	}
	if userID == 0 {
		return map[string]any{"error": "please log in to track applications"}, nil
	}

	title := getStringArg(args, "job_title", "")
	company := getStringArg(args, "company_name", "")
	url := getStringArg(args, "job_url", "")
	status := getStringArg(args, "status", "applied")

	if title == "" || company == "" {
		return map[string]any{"error": "please provide both job title and company name"}, nil
	}

	app := &models.JobApplication{
		UserID:      userID,
		JobTitle:    title,
		CompanyName: company,
		JobURL:      url,
		Status:      status,
	}
	if err := toolRegistry.JobAppModel.Create(app); err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return map[string]any{"message": fmt.Sprintf("You're already tracking '%s' at %s", title, company)}, nil
		}
		return map[string]any{"error": "failed to track application"}, nil
	}
	return map[string]any{"success": true, "message": fmt.Sprintf("Added '%s' at %s to your tracker (status: %s)", title, company, status)}, nil
}

func handleUpdateApplicationStatus(ctx context.Context, userID int, args map[string]any) (any, error) {
	if toolRegistry == nil || toolRegistry.JobAppModel == nil {
		return map[string]any{"error": "application tracking not available"}, nil
	}
	if userID == 0 {
		return map[string]any{"error": "please log in to update applications"}, nil
	}

	company := getStringArg(args, "company_name", "")
	jobTitle := getStringArg(args, "job_title", "")
	newStatus := getStringArg(args, "new_status", "")
	log.Printf("[TOOL:update_status] userID=%d company=%q jobTitle=%q newStatus=%q", userID, company, jobTitle, newStatus)
	if company == "" && jobTitle == "" {
		log.Printf("[TOOL:update_status] FAIL: no company or job title provided")
		return map[string]any{"error": "please provide company name or job title"}, nil
	}
	if newStatus == "" {
		log.Printf("[TOOL:update_status] FAIL: no new_status provided")
		return map[string]any{"error": "please provide the new status"}, nil
	}

	apps, _, err := toolRegistry.JobAppModel.ListByUser(userID, 100, 0, "")
	if err != nil {
		log.Printf("[TOOL:update_status] FAIL: ListByUser error: %v", err)
		return map[string]any{"error": "failed to look up applications"}, nil
	}
	log.Printf("[TOOL:update_status] found %d total applications", len(apps))

	// Score each application for best fuzzy match
	type scored struct {
		app   *models.JobApplication
		score int
	}
	companyLower := strings.ToLower(company)
	titleLower := strings.ToLower(jobTitle)
	var matches []scored
	for _, app := range apps {
		s := 0
		appCompany := strings.ToLower(app.CompanyName)
		appTitle := strings.ToLower(app.JobTitle)

		if company != "" {
			if strings.EqualFold(app.CompanyName, company) {
				s += 10 // exact company match
			} else if strings.Contains(appCompany, companyLower) || strings.Contains(companyLower, appCompany) {
				s += 5 // partial company match
			}
		}
		if jobTitle != "" {
			if strings.EqualFold(app.JobTitle, jobTitle) {
				s += 10 // exact title match
			} else if strings.Contains(appTitle, titleLower) || strings.Contains(titleLower, appTitle) {
				s += 5 // partial title match
			}
		}
		if s > 0 {
			matches = append(matches, scored{app: app, score: s})
		}
	}

	if len(matches) == 0 {
		hint := company
		if hint == "" {
			hint = jobTitle
		}
		available := make([]string, 0, len(apps))
		for _, app := range apps {
			available = append(available, fmt.Sprintf("'%s' at %s (status: %s)", app.JobTitle, app.CompanyName, app.Status))
		}
		log.Printf("[TOOL:update_status] FAIL: no match for company=%q title=%q. Available: %v", company, jobTitle, available)
		return map[string]any{
			"error":          fmt.Sprintf("No application found matching '%s'. Track it first.", hint),
			"available_apps": available,
		}, nil
	}

	// Pick the best match
	best := matches[0]
	for _, m := range matches[1:] {
		if m.score > best.score {
			best = m
		}
	}
	app := best.app
	log.Printf("[TOOL:update_status] best match: id=%d title=%q company=%q currentStatus=%q score=%d", app.ID, app.JobTitle, app.CompanyName, app.Status, best.score)

	if err := toolRegistry.JobAppModel.UpdateStatus(userID, app.ID, newStatus, ""); err != nil {
		log.Printf("[TOOL:update_status] FAIL: UpdateStatus error: %v", err)
		var ite *models.InvalidTransitionError
		if errors.As(err, &ite) {
			if ite.CurrentStatus == newStatus {
				return map[string]any{"already_done": true, "message": fmt.Sprintf("'%s' at %s is already in '%s' status", app.JobTitle, app.CompanyName, newStatus)}, nil
			}
			return map[string]any{"error": fmt.Sprintf("Cannot move '%s' at %s from '%s' to '%s'. Allowed: %v", app.JobTitle, app.CompanyName, ite.CurrentStatus, ite.RequestedStatus, ite.Allowed)}, nil
		}
		return map[string]any{"error": fmt.Sprintf("failed to update status: %s", err.Error())}, nil
	}
	return map[string]any{"success": true, "message": fmt.Sprintf("Updated '%s' at %s to '%s'", app.JobTitle, app.CompanyName, newStatus)}, nil
}

func handleGetMyApplications(ctx context.Context, userID int, args map[string]any) (any, error) {
	if toolRegistry == nil || toolRegistry.JobAppModel == nil {
		return map[string]any{"error": "application tracking not available"}, nil
	}
	if userID == 0 {
		return map[string]any{"error": "please log in to see your applications"}, nil
	}

	apps, _, err := toolRegistry.JobAppModel.ListByUser(userID, 100, 0, "")
	if err != nil || len(apps) == 0 {
		return map[string]any{"message": "You don't have any tracked applications yet.", "count": 0}, nil
	}

	statusFilter := getStringArg(args, "status_filter", "")
	var results []map[string]any
	for _, app := range apps {
		if statusFilter != "" && !strings.EqualFold(app.Status, statusFilter) {
			continue
		}
		results = append(results, map[string]any{
			"job_title":  app.JobTitle,
			"company":    app.CompanyName,
			"status":     app.Status,
			"applied_at": app.AppliedAt.Format("Jan 2, 2006"),
		})
	}
	if len(results) == 0 {
		return map[string]any{"message": fmt.Sprintf("No applications with status '%s'", statusFilter), "count": 0}, nil
	}
	return map[string]any{"applications": results, "count": len(results)}, nil
}

func handleGetJobCount(ctx context.Context, userID int, args map[string]any) (any, error) {
	if toolRegistry == nil || toolRegistry.JobPostingModel == nil {
		return map[string]any{"error": "job data not available"}, nil
	}

	role := getStringArg(args, "role", "")
	location := getStringArg(args, "location", "")

	if role == "" && location == "" {
		// Total active count
		stats, err := toolRegistry.JobPostingModel.GetStatistics()
		if err != nil {
			return map[string]any{"error": "failed to get job stats"}, nil
		}
		return map[string]any{"total_active": stats.ActivePostings, "message": fmt.Sprintf("HiHired has %d active job postings", stats.ActivePostings)}, nil
	}

	// Filtered count
	sqlQuery := `SELECT COUNT(*) FROM job_postings WHERE is_active = TRUE`
	var queryArgs []any
	argIdx := 1
	if role != "" {
		sqlQuery += fmt.Sprintf(` AND title ILIKE $%d`, argIdx)
		queryArgs = append(queryArgs, "%"+role+"%")
		argIdx++
	}
	if location != "" {
		sqlQuery += fmt.Sprintf(` AND location ILIKE $%d`, argIdx)
		queryArgs = append(queryArgs, "%"+location+"%")
	}

	var count int
	err := toolRegistry.JobPostingModel.DB().QueryRow(sqlQuery, queryArgs...).Scan(&count)
	if err != nil {
		return map[string]any{"error": "failed to count jobs"}, nil
	}
	msg := fmt.Sprintf("Found %d active %s positions", count, role)
	if location != "" {
		msg += " in " + location
	}
	return map[string]any{"count": count, "role": role, "location": location, "message": msg}, nil
}

func handleClearProfile(ctx context.Context, userID int, args map[string]any) (any, error) {
	if toolRegistry == nil || toolRegistry.ChatProfileModel == nil {
		return map[string]any{"error": "profile service not available"}, nil
	}
	if userID <= 0 {
		return map[string]any{"error": "not authenticated"}, nil
	}
	if err := toolRegistry.ChatProfileModel.Clear(userID); err != nil {
		return map[string]any{"error": "failed to clear profile"}, nil
	}
	return map[string]any{"success": true, "message": "Your preferences and remembered facts have been cleared. I'll start fresh from here."}, nil
}

func handleQueryUserData(ctx context.Context, userID int, args map[string]any) (any, error) {
	question := getStringArg(args, "question", "")
	if question == "" {
		return map[string]any{"error": "question is required"}, nil
	}
	if userID <= 0 {
		return map[string]any{"error": "not authenticated"}, nil
	}
	if toolRegistry == nil || toolRegistry.DB == nil {
		return map[string]any{"error": "database not available"}, nil
	}

	results, err := ExecuteUserDataQuery(ctx, toolRegistry.DB, question, userID)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}

	return map[string]any{
		"results": results,
		"count":   len(results),
	}, nil
}

func handleUpdateResumeField(ctx context.Context, userID int, args map[string]any) (any, error) {
	field := getStringArg(args, "field", "")
	action := getStringArg(args, "action", "set")
	value := getStringArg(args, "value", "")

	if field == "" || value == "" {
		return map[string]any{"error": "field and value are required"}, nil
	}

	return map[string]any{
		"resume_update": true,
		"field":         field,
		"action":        action,
		"value":         value,
		"message":       fmt.Sprintf("Updated %s: %s '%s'", field, action, value),
	}, nil
}

// --- Resume feature tool handlers (migrated from intent router) ---

func getResumeString(ctx context.Context, key string) string {
	requestCtx := chatToolRequestContext(ctx)
	if requestCtx.ResumeData == nil {
		return ""
	}
	v, _ := requestCtx.ResumeData[key].(string)
	return strings.TrimSpace(v)
}

func getResumeSlice(ctx context.Context, key string) []any {
	requestCtx := chatToolRequestContext(ctx)
	if requestCtx.ResumeData == nil {
		return nil
	}
	v, _ := requestCtx.ResumeData[key].([]any)
	return v
}

func extractExperience(ctx context.Context) string {
	exps := getResumeSlice(ctx, "experiences")
	if len(exps) == 0 {
		return getResumeString(ctx, "experience")
	}
	var sb strings.Builder
	for _, e := range exps {
		if m, ok := e.(map[string]any); ok {
			title, _ := m["jobTitle"].(string)
			company, _ := m["company"].(string)
			desc, _ := m["description"].(string)
			if title != "" || company != "" {
				fmt.Fprintf(&sb, "%s at %s\n%s\n\n", title, company, desc)
			}
		}
	}
	return sb.String()
}

func extractEducation(ctx context.Context) string {
	edus := getResumeSlice(ctx, "education")
	if len(edus) == 0 {
		return getResumeString(ctx, "education")
	}
	var sb strings.Builder
	for _, e := range edus {
		if m, ok := e.(map[string]any); ok {
			degree, _ := m["degree"].(string)
			school, _ := m["school"].(string)
			field, _ := m["field"].(string)
			fmt.Fprintf(&sb, "%s in %s, %s\n", degree, field, school)
		}
	}
	return sb.String()
}

func extractSkills(ctx context.Context) []string {
	s := getResumeString(ctx, "skills")
	if s == "" {
		return nil
	}
	var skills []string
	for _, part := range strings.Split(s, ",") {
		t := strings.TrimSpace(part)
		if t != "" {
			skills = append(skills, t)
		}
	}
	return skills
}

func extractProjects(ctx context.Context) string {
	projs := getResumeSlice(ctx, "projects")
	if len(projs) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, p := range projs {
		if m, ok := p.(map[string]any); ok {
			name, _ := m["projectName"].(string)
			desc, _ := m["description"].(string)
			fmt.Fprintf(&sb, "%s: %s\n", name, desc)
		}
	}
	return sb.String()
}

func handleAnalyzeResume(ctx context.Context, userID int, args map[string]any) (any, error) {
	requestCtx := chatToolRequestContext(ctx)
	if requestCtx.ResumeData == nil {
		return map[string]any{"error": "no resume data available"}, nil
	}
	prompt := BuildResumeAdvicePrompt(requestCtx.ResumeData, requestCtx.JobDescription)
	result, err := CallGeminiWithTemperatureContext(ctx, prompt, 0.3)
	if err != nil {
		return map[string]any{"error": "analysis failed"}, nil
	}
	return map[string]any{"advice": strings.TrimSpace(result)}, nil
}

func handleGenerateCoverLetter(ctx context.Context, userID int, args map[string]any) (any, error) {
	requestCtx := chatToolRequestContext(ctx)
	if requestCtx.ResumeData == nil {
		return map[string]any{"error": "no resume data available"}, nil
	}
	company := getStringArg(args, "company_name", "")
	prompt := BuildCoverLetterPrompt(requestCtx.ResumeData, requestCtx.JobDescription, company)
	result, err := CallGeminiWithTemperatureContext(ctx, prompt, 0.3)
	if err != nil {
		return map[string]any{"error": "cover letter generation failed"}, nil
	}
	return map[string]any{"cover_letter": strings.TrimSpace(result)}, nil
}

func handleGenerateRecommendationLetter(ctx context.Context, userID int, args map[string]any) (any, error) {
	requestCtx := chatToolRequestContext(ctx)
	if requestCtx.ResumeData == nil {
		return map[string]any{"error": "no resume data available"}, nil
	}
	company := getStringArg(args, "company_name", "")
	position := getStringArg(args, "position", "")
	prompt := BuildRecommendationLetterPrompt(requestCtx.ResumeData, requestCtx.JobDescription, company, position)
	result, err := CallGeminiWithTemperatureContext(ctx, prompt, 0.3)
	if err != nil {
		return map[string]any{"error": "letter generation failed"}, nil
	}
	return map[string]any{"letter": strings.TrimSpace(result)}, nil
}

func handleGenerateSummary(ctx context.Context, userID int, args map[string]any) (any, error) {
	requestCtx := chatToolRequestContext(ctx)
	experience := extractExperience(ctx)
	education := extractEducation(ctx)
	skills := extractSkills(ctx)
	existingSummary := getResumeString(ctx, "summary")

	prompt := BuildSummaryOptimizationPromptWithSkills(experience, education, skills, existingSummary, requestCtx.JobDescription, nil, nil)
	result, err := CallGeminiWithTemperatureContext(ctx, prompt, 0.3)
	if err != nil {
		return map[string]any{"error": "summary generation failed"}, nil
	}
	cleaned := strings.TrimSpace(result)
	return map[string]any{
		"resume_update": true,
		"field":         "summary",
		"action":        "set",
		"value":         cleaned,
		"message":       "Generated professional summary",
	}, nil
}

func handleOptimizeExperience(ctx context.Context, userID int, args map[string]any) (any, error) {
	requestCtx := chatToolRequestContext(ctx)
	experience := extractExperience(ctx)
	if experience == "" {
		return map[string]any{"error": "no experience data to optimize"}, nil
	}
	prompt := BuildExperienceOptimizationPromptWithSkills(requestCtx.JobDescription, experience, nil, nil)
	result, err := CallGeminiWithTemperatureContext(ctx, prompt, 0.3)
	if err != nil {
		return map[string]any{"error": "optimization failed"}, nil
	}
	validated := ValidateAndCleanOutput(experience, result)
	return map[string]any{"optimized": strings.TrimSpace(validated)}, nil
}

func handleOptimizeProject(ctx context.Context, userID int, args map[string]any) (any, error) {
	requestCtx := chatToolRequestContext(ctx)
	projects := extractProjects(ctx)
	if projects == "" {
		return map[string]any{"error": "no project data to optimize"}, nil
	}
	prompt := BuildProjectOptimizationPromptWithSkills(requestCtx.JobDescription, projects, "", nil, nil)
	result, err := CallGeminiWithTemperatureContext(ctx, prompt, 0.3)
	if err != nil {
		return map[string]any{"error": "optimization failed"}, nil
	}
	validated := ValidateAndCleanOutput(projects, result)
	return map[string]any{"optimized": strings.TrimSpace(validated)}, nil
}

func handleImproveGrammar(ctx context.Context, userID int, args map[string]any) (any, error) {
	section := getStringArg(args, "section", "experience")
	var prompt string
	switch section {
	case "summary":
		summary := getResumeString(ctx, "summary")
		if summary == "" {
			return map[string]any{"error": "no summary to improve"}, nil
		}
		prompt = BuildSummaryGrammarPrompt(summary)
	default:
		experience := extractExperience(ctx)
		if experience == "" {
			return map[string]any{"error": "no experience to improve"}, nil
		}
		prompt = BuildExperienceGrammarPrompt(experience)
	}
	result, err := CallGeminiWithTemperatureContext(ctx, prompt, 0.3)
	if err != nil {
		return map[string]any{"error": "grammar improvement failed"}, nil
	}
	cleaned := strings.TrimSpace(result)
	if section == "summary" {
		return map[string]any{
			"resume_update": true,
			"field":         "summary",
			"action":        "set",
			"value":         cleaned,
			"message":       "Improved summary grammar",
		}, nil
	}
	return map[string]any{"improved": cleaned}, nil
}

func handleGenerateSkills(ctx context.Context, userID int, args map[string]any) (any, error) {
	requestCtx := chatToolRequestContext(ctx)
	if requestCtx.ResumeData == nil {
		return map[string]any{"error": "no resume data"}, nil
	}
	experience := extractExperience(ctx)
	education := extractEducation(ctx)
	projects := extractProjects(ctx)
	prompt := fmt.Sprintf(`Extract relevant professional skills from this resume content. Return a JSON array of skill strings.

Experience: %s
Education: %s
Projects: %s

Return JSON array only: ["skill1", "skill2", ...]`, experience, education, projects)

	raw, err := CallWithRateLimitRetry(ctx, DefaultRetryConfig(),
		func(ctx context.Context) (string, error) {
			return CallGeminiFlashWithTemperatureContext(ctx, prompt, 0.0)
		},
		RetryOpts{Endpoint: "tool_auto_generate_skills"},
	)
	if err != nil {
		return map[string]any{"error": "skill generation failed"}, nil
	}
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var skills []string
	if err := json.Unmarshal([]byte(cleaned), &skills); err != nil {
		return map[string]any{"error": "failed to parse skills"}, nil
	}
	skillsText := strings.Join(skills, ", ")
	return map[string]any{
		"resume_update": true,
		"field":         "skills",
		"action":        "set",
		"value":         skillsText,
		"skills":        skills,
		"message":       fmt.Sprintf("Generated %d skills", len(skills)),
	}, nil
}

func handleCategorizeSkills(ctx context.Context, userID int, args map[string]any) (any, error) {
	skillsText := getResumeString(ctx, "skills")
	if skillsText == "" {
		return map[string]any{"error": "no skills to categorize"}, nil
	}
	prompt := fmt.Sprintf(`Categorize these skills into groups. Return a formatted string like:
"Languages: Python, Go, TypeScript\n\nFrameworks: React, Django\n\nCloud: AWS, GCP"

Skills: %s

Return the categorized text only:`, skillsText)

	raw, err := CallWithRateLimitRetry(ctx, DefaultRetryConfig(),
		func(ctx context.Context) (string, error) {
			return CallGeminiFlashWithTemperatureContext(ctx, prompt, 0.0)
		},
		RetryOpts{Endpoint: "tool_categorize_skills"},
	)
	if err != nil {
		return map[string]any{"error": "categorization failed"}, nil
	}
	categorized := strings.TrimSpace(raw)
	return map[string]any{
		"resume_update": true,
		"field":         "skills",
		"action":        "set",
		"value":         categorized,
		"message":       "Categorized skills",
	}, nil
}

// --- Merged tool dispatchers (6 tools instead of 19) ---

func handleJobSearchDispatch(ctx context.Context, userID int, args map[string]any) (any, error) {
	switch getStringArg(args, "operation", "search") {
	case "count":
		return handleGetJobCount(ctx, userID, args)
	case "matches":
		return handleGetJobMatches(ctx, userID, args)
	default:
		return handleSearchJobs(ctx, userID, args)
	}
}

func handleApplicationManagerDispatch(ctx context.Context, userID int, args map[string]any) (any, error) {
	switch getStringArg(args, "operation", "list") {
	case "track":
		return handleTrackApplication(ctx, userID, args)
	case "update_status":
		return handleUpdateApplicationStatus(ctx, userID, args)
	default:
		return handleGetMyApplications(ctx, userID, args)
	}
}

func handleResumeEditorDispatch(ctx context.Context, userID int, args map[string]any) (any, error) {
	switch getStringArg(args, "operation", "update_field") {
	case "generate_summary":
		return handleGenerateSummary(ctx, userID, args)
	case "improve_grammar":
		return handleImproveGrammar(ctx, userID, args)
	case "generate_skills":
		return handleGenerateSkills(ctx, userID, args)
	case "categorize_skills":
		return handleCategorizeSkills(ctx, userID, args)
	default:
		return handleUpdateResumeField(ctx, userID, args)
	}
}

func handleResumeOptimizerDispatch(ctx context.Context, userID int, args map[string]any) (any, error) {
	switch getStringArg(args, "operation", "analyze") {
	case "optimize_experience":
		return handleOptimizeExperience(ctx, userID, args)
	case "optimize_project":
		return handleOptimizeProject(ctx, userID, args)
	default:
		return handleAnalyzeResume(ctx, userID, args)
	}
}

func handleLetterGeneratorDispatch(ctx context.Context, userID int, args map[string]any) (any, error) {
	switch getStringArg(args, "operation", "cover_letter") {
	case "recommendation":
		return handleGenerateRecommendationLetter(ctx, userID, args)
	default:
		return handleGenerateCoverLetter(ctx, userID, args)
	}
}

func handleQueryDataDispatch(ctx context.Context, userID int, args map[string]any) (any, error) {
	switch getStringArg(args, "operation", "query") {
	case "clear_profile":
		return handleClearProfile(ctx, userID, args)
	default:
		return handleQueryUserData(ctx, userID, args)
	}
}
