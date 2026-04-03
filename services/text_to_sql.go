package services

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const userDataSchema = `Available tables:

job_applications: user_id, job_title, company_name, job_url, job_location, status,
    applied_at, notes, salary_offered, cover_letter_used, created_at
    Status values: applied, screening, interviewing, offered, accepted, rejected, withdrawn

resume_job_matches: user_id, resume_hash, job_posting_id, match_score, matched_at, fit_reasons

job_postings: id, title, location, remote_type, department, employment_type,
    job_url, description, salary_min, salary_max, career_field, seniority,
    extracted_skills, posted_at, is_active
    (join with resume_job_matches ON job_postings.id = resume_job_matches.job_posting_id)

job_companies: id, name, careers_url
    (join with job_postings ON job_companies.id = job_postings.company_id)

user_chat_profiles: user_id, profile (JSONB), summary, updated_at`

var allowedTables = map[string]bool{
	"job_applications":    true,
	"resume_job_matches":  true,
	"job_postings":        true,
	"job_companies":       true,
	"user_chat_profiles":  true,
}

var userScopedTables = map[string]bool{
	"job_applications":    true,
	"resume_job_matches":  true,
	"user_chat_profiles":  true,
}

func generateUserDataSQL(question string, userID int) (string, error) {
	prompt := fmt.Sprintf(`Generate a PostgreSQL SELECT query for this question:
"%s"

%s

Rules:
- For tables with user_id column (job_applications, resume_job_matches, user_chat_profiles), MUST use WHERE user_id = $1
- For public tables (job_postings, job_companies), user_id filter is not needed
- SELECT only
- LIMIT 20 max
- Use JOINs when needed to get related data
- Return ONLY the raw SQL, no markdown, no explanation`, question, userDataSchema)

	raw, err := CallGeminiFlashWithTemperature(prompt, 0.0)
	if err != nil {
		return "", err
	}

	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```sql")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned, nil
}

func validateSQL(query string) (string, error) {
	upper := strings.ToUpper(strings.TrimSpace(query))

	if !strings.HasPrefix(upper, "SELECT") {
		return "", fmt.Errorf("only SELECT queries allowed")
	}

	dangerous := []string{"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "TRUNCATE",
		"CREATE", "GRANT", "REVOKE", "EXEC", "EXECUTE"}
	for _, kw := range dangerous {
		if strings.Contains(upper, kw) {
			return "", fmt.Errorf("query contains forbidden keyword: %s", kw)
		}
	}

	tables := extractTableNames(query)
	for _, t := range tables {
		if !allowedTables[strings.ToLower(t)] {
			return "", fmt.Errorf("table not allowed: %s", t)
		}
	}

	needsUserID := false
	for _, t := range tables {
		if userScopedTables[strings.ToLower(t)] {
			needsUserID = true
			break
		}
	}
	if needsUserID && !strings.Contains(query, "$1") {
		return "", fmt.Errorf("query on user data must filter by user_id ($1)")
	}

	if !strings.Contains(upper, "LIMIT") {
		query += " LIMIT 20"
	}

	return query, nil
}

var tableNameRe = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+(\w+)`)

func extractTableNames(query string) []string {
	matches := tableNameRe.FindAllStringSubmatch(query, -1)
	var tables []string
	for _, m := range matches {
		if len(m) > 1 {
			tables = append(tables, m[1])
		}
	}
	return tables
}

func fixUserDataSQL(failedSQL, errMsg, question string, userID int) (string, error) {
	prompt := fmt.Sprintf(`The following SQL query failed. Fix it.

Original question: "%s"
Failed SQL: %s
Error: %s

%s

Rules:
- For tables with user_id, use WHERE user_id = $1
- SELECT only, LIMIT 20
- Return ONLY the fixed SQL, no explanation`, question, failedSQL, errMsg, userDataSchema)

	raw, err := CallGeminiFlashWithTemperature(prompt, 0.0)
	if err != nil {
		return "", err
	}

	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```sql")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	return strings.TrimSpace(cleaned), nil
}

func rowsToMaps(rows *sql.Rows, maxRows int) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]any
	for rows.Next() && len(results) < maxRows {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		row := map[string]any{}
		for i, col := range cols {
			row[col] = values[i]
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// ExecuteUserDataQuery generates, validates, and executes a text-to-SQL query with self-correction.
func ExecuteUserDataQuery(ctx context.Context, db *sql.DB, question string, userID int) ([]map[string]any, error) {
	sqlQuery, err := generateUserDataSQL(question, userID)
	if err != nil {
		return nil, fmt.Errorf("SQL generation failed: %w", err)
	}

	sqlQuery, err = validateSQL(sqlQuery)
	if err != nil {
		return nil, fmt.Errorf("SQL validation failed: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	rows, err := db.QueryContext(queryCtx, sqlQuery, userID)
	if err != nil {
		// Self-correction: retry once
		fixedSQL, fixErr := fixUserDataSQL(sqlQuery, err.Error(), question, userID)
		if fixErr != nil {
			return nil, fmt.Errorf("query failed: %w", err)
		}
		fixedSQL, fixErr = validateSQL(fixedSQL)
		if fixErr != nil {
			return nil, fmt.Errorf("fixed query validation failed: %w", fixErr)
		}
		rows, err = db.QueryContext(queryCtx, fixedSQL, userID)
		if err != nil {
			return nil, fmt.Errorf("query failed after retry: %w", err)
		}
	}
	defer rows.Close()

	return rowsToMaps(rows, 20)
}
