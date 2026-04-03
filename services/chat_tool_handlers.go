package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

// SetToolRegistry injects model dependencies for tool handlers.
func SetToolRegistry(r *ToolRegistry) {
	toolRegistry = r
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
	if company == "" || newStatus == "" {
		return map[string]any{"error": "please provide company name and new status"}, nil
	}

	// Find app by company name (and optionally job title) for this user
	apps, _, err := toolRegistry.JobAppModel.ListByUser(userID, 100, 0, "")
	if err != nil {
		return map[string]any{"error": "failed to look up applications"}, nil
	}

	for _, app := range apps {
		if !strings.EqualFold(app.CompanyName, company) {
			continue
		}
		if jobTitle != "" && !strings.Contains(strings.ToLower(app.JobTitle), strings.ToLower(jobTitle)) {
			continue
		}
		if err := toolRegistry.JobAppModel.UpdateStatus(userID, app.ID, newStatus, ""); err != nil {
			return map[string]any{"error": "failed to update status"}, nil
		}
		return map[string]any{"success": true, "message": fmt.Sprintf("Updated '%s' at %s to '%s'", app.JobTitle, company, newStatus)}, nil
	}
	return map[string]any{"error": fmt.Sprintf("No application found for '%s'. Track it first.", company)}, nil
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
