package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

const defaultResumeFormat = "classic-professional"

type Resume struct {
	ID             int             `json:"id"`
	UserID         int             `json:"user_id"`
	Name           string          `json:"name"`
	Email          string          `json:"email,omitempty"`
	Phone          string          `json:"phone,omitempty"`
	Summary        json.RawMessage `json:"summary"`
	Skills         json.RawMessage `json:"skills"`
	SkillsCategorized string       `json:"skills_categorized,omitempty"`
	Experience     string          `json:"experience,omitempty"`
	Education      string          `json:"education,omitempty"`
	JobDescription string          `json:"job_description,omitempty"`
	Location       string          `json:"location,omitempty"`
	SelectedFormat string          `json:"selected_format"`
	LastResumeHTML string          `json:"last_resume_html,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// ResumeWithUser carries a resume along with its owning user's contact info.
type ResumeWithUser struct {
	Resume            Resume
	UserEmail         string
	UserName          string
	EmailUnsubscribed bool
	MarketingOptIn    bool
}

// ResumeProfileSaveInput captures the complete resume payload we store per user.
type ResumeProfileSaveInput struct {
	Name              string
	Email             string
	Phone             string
	Summary           string
	Skills            []string
	SkillsCategorized string
	Experience        string
	Experiences       []ExperienceInput
	Education         string
	JobDescription    string
	Location          string
	SelectedFormat    string
	ProfileJSON       json.RawMessage
}

// ExperienceInput captures a single experience entry to persist in the experiences table.
type ExperienceInput struct {
	JobTitle         string
	Company          string
	City             string
	State            string
	StartDate        string
	EndDate          string
	CurrentlyWorking bool
	Description      string
}

// ExperienceRecord represents a stored experience row.
type ExperienceRecord struct {
	ID               int        `json:"id"`
	JobTitle         string     `json:"job_title"`
	Company          string     `json:"company"`
	City             string     `json:"city,omitempty"`
	State            string     `json:"state,omitempty"`
	StartDate        string     `json:"start_date,omitempty"`
	EndDate          string     `json:"end_date,omitempty"`
	CurrentlyWorking bool       `json:"currently_working"`
	Description      string     `json:"description,omitempty"`
}

type ResumeModel struct {
	DB *sql.DB
}

func NewResumeModel(db *sql.DB) *ResumeModel {
	return &ResumeModel{DB: db}
}

func (m *ResumeModel) GetByUserID(userID int) (*Resume, error) {
	resume := &Resume{}
	query := `
		SELECT id, user_id, name, email, phone, summary, skills, selected_format, skills_categorized, experience, education, job_description, location, created_at, updated_at
		FROM resumes WHERE user_id = $1
	`
	if err := scanResumeRowFull(m.DB.QueryRow(query, userID), resume); err == nil {
		return resume, nil
	} else if !isUndefinedColumnError(err) {
		return nil, err
	}

	legacyQuery := `
		SELECT id, user_id, name, email, phone, summary, skills, selected_format, created_at, updated_at
		FROM resumes WHERE user_id = $1
	`
	if err := scanResumeRowLegacy(m.DB.QueryRow(legacyQuery, userID), resume); err != nil {
		return nil, err
	}
	return resume, nil
}

// SaveProfile replaces (or creates) the single resume profile row for a user.
func (m *ResumeModel) SaveProfile(ctx context.Context, userID int, input ResumeProfileSaveInput) (*Resume, error) {
	if userID == 0 {
		return nil, errors.New("userID is required")
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "User Resume"
	}
	email := strings.TrimSpace(input.Email)
	phone := strings.TrimSpace(input.Phone)
	experience := strings.TrimSpace(input.Experience)
	// Prefer structured experiences table; keep blob only when no structured data provided.
	if len(input.Experiences) > 0 {
		experience = ""
	}
	education := strings.TrimSpace(input.Education)
	jobDescription := strings.TrimSpace(input.JobDescription)
	location := strings.TrimSpace(input.Location)
	skillsCategorized := strings.TrimSpace(input.SkillsCategorized)
	selectedFormat := strings.TrimSpace(input.SelectedFormat)
	if selectedFormat == "" {
		selectedFormat = defaultResumeFormat
	}

	skills := normalizeSkills(input.Skills)
	skillsJSON, err := json.Marshal(skills)
	if err != nil {
		return nil, err
	}

	profileJSON := input.ProfileJSON
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		summary = extractSummaryFromProfile(profileJSON)
	}

	resume := &Resume{}
	if err := m.saveProfileWithResumeName(ctx, resume, userID, name, email, phone, summary, skillsJSON, skillsCategorized, experience, education, jobDescription, location, selectedFormat); err == nil {
		if input.Experiences != nil {
			if err := m.replaceExperiences(ctx, resume.ID, input.Experiences); err != nil {
				return nil, err
			}
		}
		return resume, nil
	} else if !isUndefinedColumnError(err) {
		return nil, err
	}

	if err := m.saveProfileLegacy(ctx, resume, userID, name, email, phone, summary, skillsJSON, selectedFormat); err != nil {
		return nil, err
	}

	if input.Experiences != nil {
		if err := m.replaceExperiences(ctx, resume.ID, input.Experiences); err != nil {
			return nil, err
		}
	}

	return resume, nil
}

// SaveLastHTML stores the user's last rendered resume HTML for tailor reuse.
func (m *ResumeModel) SaveLastHTML(userID int, htmlContent string) error {
	_, err := m.DB.Exec(`UPDATE resumes SET last_resume_html = $1, updated_at = NOW() WHERE user_id = $2`, htmlContent, userID)
	return err
}

// GetLastHTML returns the user's last rendered resume HTML.
func (m *ResumeModel) GetLastHTML(userID int) (string, error) {
	var html string
	err := m.DB.QueryRow(`SELECT COALESCE(last_resume_html, '') FROM resumes WHERE user_id = $1`, userID).Scan(&html)
	return html, err
}

// Save is kept for backward compatibility; it delegates to SaveProfile.
func (m *ResumeModel) Save(userID int, name string, summary, skills json.RawMessage) error {
	input := ResumeProfileSaveInput{
		Name:        name,
		Summary:     stringFromJSON(summary),
		Skills:      skillsFromJSON(skills),
		ProfileJSON: buildProfileJSONFromPieces(name, summary, skills),
	}
	_, err := m.SaveProfile(context.Background(), userID, input)
	return err
}

func (m *ResumeModel) GetSummary(userID int) (json.RawMessage, error) {
	var summaryVal interface{}
	query := `SELECT summary FROM resumes WHERE user_id = $1`
	err := m.DB.QueryRow(query, userID).Scan(&summaryVal)
	return toRawMessage(summaryVal), err
}

// ListWithUsers returns all resumes with their owning user's contact info.
func (m *ResumeModel) ListWithUsers() ([]*ResumeWithUser, error) {
	rows, err := m.DB.Query(`
		SELECT r.id, r.user_id, r.name, r.email, r.phone, r.summary, r.skills, r.selected_format,
		       r.skills_categorized, r.experience, r.education, r.job_description, r.location,
		       r.created_at, r.updated_at,
		       u.email AS user_email, u.name AS user_name,
		       COALESCE(u.email_unsubscribed, false) AS email_unsubscribed,
		       COALESCE(u.marketing_opt_in, false) AS marketing_opt_in
		FROM resumes r
		JOIN users u ON u.id = r.user_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*ResumeWithUser
	for rows.Next() {
		var resume Resume
		var summaryVal, skillsVal, skillsCatVal, experienceVal, educationVal, jobDescVal, locationVal interface{}
		var userEmail, userName string
		var emailUnsubscribed, marketingOptIn bool

		if err := rows.Scan(
			&resume.ID,
			&resume.UserID,
			&resume.Name,
			&resume.Email,
			&resume.Phone,
			&summaryVal,
			&skillsVal,
			&resume.SelectedFormat,
			&skillsCatVal,
			&experienceVal,
			&educationVal,
			&jobDescVal,
			&locationVal,
			&resume.CreatedAt,
			&resume.UpdatedAt,
			&userEmail,
			&userName,
			&emailUnsubscribed,
			&marketingOptIn,
		); err != nil {
			return nil, err
		}

		resume.Summary = toRawMessage(summaryVal)
		resume.Skills = toRawMessage(skillsVal)
		resume.SkillsCategorized = stringFromAny(skillsCatVal)
		resume.Experience = stringFromAny(experienceVal)
		resume.Education = stringFromAny(educationVal)
		resume.JobDescription = stringFromAny(jobDescVal)
		resume.Location = stringFromAny(locationVal)

		results = append(results, &ResumeWithUser{
			Resume:            resume,
			UserEmail:         userEmail,
			UserName:          userName,
			EmailUnsubscribed: emailUnsubscribed,
			MarketingOptIn:    marketingOptIn,
		})
	}

	return results, rows.Err()
}

// GetWithUserByUserID returns a single resume with its owning user's contact info.
func (m *ResumeModel) GetWithUserByUserID(userID int) (*ResumeWithUser, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	row := m.DB.QueryRow(`
		SELECT r.id, r.user_id, r.name, r.email, r.phone, r.summary, r.skills, r.selected_format,
		       r.skills_categorized, r.experience, r.education, r.job_description, r.location,
		       r.created_at, r.updated_at,
		       u.email AS user_email, u.name AS user_name,
		       COALESCE(u.email_unsubscribed, false) AS email_unsubscribed,
		       COALESCE(u.marketing_opt_in, false) AS marketing_opt_in
		FROM resumes r
		JOIN users u ON u.id = r.user_id
		WHERE r.user_id = $1
	`, userID)

	var resume Resume
	var summaryVal, skillsVal, skillsCatVal, experienceVal, educationVal, jobDescVal, locationVal interface{}
	var userEmail, userName string
	var emailUnsubscribed, marketingOptIn bool
	if err := row.Scan(
		&resume.ID,
		&resume.UserID,
		&resume.Name,
		&resume.Email,
		&resume.Phone,
		&summaryVal,
		&skillsVal,
		&resume.SelectedFormat,
		&skillsCatVal,
		&experienceVal,
		&educationVal,
		&jobDescVal,
		&locationVal,
		&resume.CreatedAt,
		&resume.UpdatedAt,
		&userEmail,
		&userName,
		&emailUnsubscribed,
		&marketingOptIn,
	); err != nil {
		return nil, err
	}

	resume.Summary = toRawMessage(summaryVal)
	resume.Skills = toRawMessage(skillsVal)
	resume.SkillsCategorized = stringFromAny(skillsCatVal)
	resume.Experience = stringFromAny(experienceVal)
	resume.Education = stringFromAny(educationVal)
	resume.JobDescription = stringFromAny(jobDescVal)
	resume.Location = stringFromAny(locationVal)

	return &ResumeWithUser{
		Resume:            resume,
		UserEmail:         userEmail,
		UserName:          userName,
		EmailUnsubscribed: emailUnsubscribed,
		MarketingOptIn:    marketingOptIn,
	}, nil
}

func (m *ResumeModel) saveProfileWithResumeName(ctx context.Context, resume *Resume, userID int, name, email, phone, summary string, skillsJSON json.RawMessage, skillsCategorized, experience, education, jobDescription, location, selectedFormat string) error {
	query := `
		INSERT INTO resumes (
			user_id, resume_name, name, email, phone, summary, skills, selected_format, skills_categorized, experience, education, job_description, location, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			resume_name = EXCLUDED.resume_name,
			name = EXCLUDED.name,
			email = EXCLUDED.email,
			phone = EXCLUDED.phone,
			summary = EXCLUDED.summary,
			skills = EXCLUDED.skills,
			selected_format = EXCLUDED.selected_format,
			skills_categorized = EXCLUDED.skills_categorized,
			experience = EXCLUDED.experience,
			education = EXCLUDED.education,
			job_description = EXCLUDED.job_description,
			location = EXCLUDED.location,
			updated_at = NOW()
		RETURNING id, user_id, name, email, phone, summary, skills, selected_format, skills_categorized, experience, education, job_description, location, created_at, updated_at
	`

	return scanResumeRowFull(
		m.DB.QueryRowContext(ctx, query, userID, name, name, email, phone, summary, skillsJSON, selectedFormat, skillsCategorized, experience, education, jobDescription, location),
		resume,
	)
}

func (m *ResumeModel) saveProfileLegacy(ctx context.Context, resume *Resume, userID int, name, email, phone, summary string, skillsJSON json.RawMessage, selectedFormat string) error {
	query := `
		INSERT INTO resumes (
			user_id, name, email, phone, summary, skills, selected_format, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			name = EXCLUDED.name,
			email = EXCLUDED.email,
			phone = EXCLUDED.phone,
			summary = EXCLUDED.summary,
			skills = EXCLUDED.skills,
			selected_format = EXCLUDED.selected_format,
			updated_at = NOW()
		RETURNING id, user_id, name, email, phone, summary, skills, selected_format, created_at, updated_at
	`

	return scanResumeRowLegacy(
		m.DB.QueryRowContext(ctx, query, userID, name, email, phone, summary, skillsJSON, selectedFormat),
		resume,
	)
}

func isUndefinedColumnError(err error) bool {
	if err == nil {
		return false
	}
	if pgErr, ok := err.(*pq.Error); ok {
		return pgErr.Code == "42703"
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "column") && strings.Contains(msg, "does not exist")
}

func scanResumeRowFull(scanner rowScanner, resume *Resume) error {
	var summaryVal, skillsVal, skillsCatVal, experienceVal, educationVal, jobDescVal, locationVal interface{}

	if err := scanner.Scan(
		&resume.ID,
		&resume.UserID,
		&resume.Name,
		&resume.Email,
		&resume.Phone,
		&summaryVal,
		&skillsVal,
		&resume.SelectedFormat,
		&skillsCatVal,
		&experienceVal,
		&educationVal,
		&jobDescVal,
		&locationVal,
		&resume.CreatedAt,
		&resume.UpdatedAt,
	); err != nil {
		return err
	}

	resume.Summary = toRawMessage(summaryVal)
	resume.Skills = toRawMessage(skillsVal)
	resume.SkillsCategorized = stringFromAny(skillsCatVal)
	resume.Experience = stringFromAny(experienceVal)
	resume.Education = stringFromAny(educationVal)
	resume.JobDescription = stringFromAny(jobDescVal)
	resume.Location = stringFromAny(locationVal)
	return nil
}

func scanResumeRowLegacy(scanner rowScanner, resume *Resume) error {
	var summaryVal, skillsVal interface{}

	if err := scanner.Scan(
		&resume.ID,
		&resume.UserID,
		&resume.Name,
		&resume.Email,
		&resume.Phone,
		&summaryVal,
		&skillsVal,
		&resume.SelectedFormat,
		&resume.CreatedAt,
		&resume.UpdatedAt,
	); err != nil {
		return err
	}

	resume.Summary = toRawMessage(summaryVal)
	resume.Skills = toRawMessage(skillsVal)
	return nil
}

func toRawMessage(value interface{}) json.RawMessage {
	switch v := value.(type) {
	case nil:
		return nil
	case json.RawMessage:
		return append(json.RawMessage{}, v...)
	case []byte:
		return append(json.RawMessage{}, v...)
	case string:
		return json.RawMessage(v)
	default:
		return nil
	}
}

func normalizeSkills(skills []string) []string {
	var normalized []string
	seen := make(map[string]struct{})
	for _, skill := range skills {
		s := strings.TrimSpace(skill)
		if s == "" {
			continue
		}
		lower := strings.ToLower(s)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		normalized = append(normalized, s)
	}
	return normalized
}

func extractSummaryFromProfile(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}
	return stringFromAnyOrJSON(data["summary"])
}

func stringFromJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return stringFromAny(value)
}

func skillsFromJSON(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var slice []string
	if err := json.Unmarshal(raw, &slice); err == nil {
		return slice
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return normalizeSkills(strings.Split(str, ","))
	}
	return nil
}

// MergeProfileFieldsFromJSON enriches the provided input with fields found in the JSON payload.
func MergeProfileFieldsFromJSON(raw json.RawMessage, input *ResumeProfileSaveInput) {
	if len(raw) == 0 || input == nil {
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}

	if input.Name == "" {
		input.Name = stringFromAny(data["name"])
	}
	if input.Email == "" {
		input.Email = stringFromAny(data["email"])
	}
	if input.Phone == "" {
		input.Phone = stringFromAny(data["phone"])
	}
	if input.Summary == "" {
		input.Summary = stringFromAnyOrJSON(data["summary"])
	}
	if input.Experience == "" {
		if v, ok := data["experience"]; ok {
			input.Experience = stringFromAnyOrJSON(v)
		} else if v, ok := data["experiences"]; ok {
			input.Experience = stringFromAnyOrJSON(v)
		}
	}
	if input.Education == "" {
		input.Education = stringFromAnyOrJSON(data["education"])
	}
	if input.JobDescription == "" {
		input.JobDescription = stringFromAnyOrJSON(data["jobDescription"])
	}
	if input.Location == "" {
		input.Location = stringFromAnyOrJSON(data["location"])
	}
	if input.SelectedFormat == "" {
		input.SelectedFormat = stringFromAny(data["selectedFormat"])
	}
	if input.SkillsCategorized == "" {
		input.SkillsCategorized = stringFromAny(data["skillsCategorized"])
	}
	if len(input.Skills) == 0 {
		input.Skills = skillsFromInterface(data["skills"])
	}
	if len(input.Experiences) == 0 {
		input.Experiences = experiencesFromInterface(data["experiences"])
	}
}

func stringFromAny(v interface{}) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case []byte:
		return strings.TrimSpace(string(val))
	case json.Number:
		return strings.TrimSpace(val.String())
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(val, 'f', -1, 64))
	case fmt.Stringer:
		return strings.TrimSpace(val.String())
	default:
		return ""
	}
}

// stringFromAnyOrJSON falls back to JSON marshaling for non-string values so we can persist structured sections.
func stringFromAnyOrJSON(v interface{}) string {
	if s := stringFromAny(v); s != "" {
		return s
	}
	if v == nil {
		return ""
	}
	if raw, err := json.Marshal(v); err == nil {
		return strings.TrimSpace(string(raw))
	}
	return ""
}

func buildProfileJSONFromPieces(name string, summary, skills json.RawMessage) json.RawMessage {
	summaryStr := stringFromJSON(summary)
	skillsSlice := skillsFromJSON(skills)
	payload := map[string]interface{}{
		"name":    strings.TrimSpace(name),
		"summary": summaryStr,
		"skills":  skillsSlice,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return raw
}

func skillsFromInterface(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return normalizeSkills(v)
	case []interface{}:
		var skills []string
		for _, item := range v {
			if s := stringFromAny(item); s != "" {
				skills = append(skills, s)
			}
		}
		return normalizeSkills(skills)
	case string:
		return normalizeSkills(strings.Split(v, ","))
	default:
		return nil
	}
}

func experiencesFromInterface(value interface{}) []ExperienceInput {
	rawSlice, ok := value.([]interface{})
	if !ok {
		return nil
	}
	if len(rawSlice) == 0 {
		return []ExperienceInput{}
	}

	var experiences []ExperienceInput
	for _, item := range rawSlice {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		jobTitle := stringFromAny(m["jobTitle"])
		if jobTitle == "" {
			jobTitle = stringFromAny(m["title"])
		}
		company := stringFromAny(m["company"])
		city := stringFromAny(m["city"])
		state := stringFromAny(m["state"])
		location := stringFromAny(m["location"])
		if city == "" && state == "" && location != "" {
			parts := strings.Split(location, ",")
			if len(parts) > 0 {
				city = strings.TrimSpace(parts[0])
			}
			if len(parts) > 1 {
				state = strings.TrimSpace(parts[1])
			}
		}

		startDate := stringFromAny(m["startDate"])
		if startDate == "" {
			startDate = strings.TrimSpace(strings.Join([]string{
				stringFromAny(m["startMonth"]),
				stringFromAny(m["startYear"]),
			}, " "))
		}
		endDate := stringFromAny(m["endDate"])
		if endDate == "" {
			endDate = strings.TrimSpace(strings.Join([]string{
				stringFromAny(m["endMonth"]),
				stringFromAny(m["endYear"]),
			}, " "))
		}

		currentlyWorking := false
		if v, ok := m["currentlyWorking"].(bool); ok {
			currentlyWorking = v
		} else if v, ok := m["current"].(bool); ok {
			currentlyWorking = v
		} else if endDate == "" {
			currentlyWorking = true
		}

		description := stringFromAnyOrJSON(m["description"])
		if description == "" {
			if bullets, ok := m["bullets"].([]interface{}); ok {
				var lines []string
				for _, b := range bullets {
					if s := stringFromAny(b); s != "" {
						lines = append(lines, s)
					}
				}
				description = strings.Join(lines, "\n")
			}
		}

		if jobTitle == "" && company == "" {
			continue
		}

		experiences = append(experiences, ExperienceInput{
			JobTitle:         jobTitle,
			Company:          company,
			City:             city,
			State:            state,
			StartDate:        startDate,
			EndDate:          endDate,
			CurrentlyWorking: currentlyWorking,
			Description:      description,
		})
	}

	return experiences
}

func (m *ResumeModel) replaceExperiences(ctx context.Context, resumeID int, experiences []ExperienceInput) error {
	if resumeID == 0 {
		return errors.New("resumeID is required to save experiences")
	}
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM experiences WHERE resume_id = $1`, resumeID); err != nil {
		return err
	}

	stmt := `
		INSERT INTO experiences (
			resume_id, job_title, company, city, state, start_date, end_date, currently_working, description, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
	`

	for _, exp := range experiences {
		jobTitle := strings.TrimSpace(exp.JobTitle)
		company := strings.TrimSpace(exp.Company)
		if jobTitle == "" && company == "" {
			continue
		}
		city := strings.TrimSpace(exp.City)
		state := strings.TrimSpace(exp.State)
		desc := strings.TrimSpace(exp.Description)
		start := parseDateString(exp.StartDate)
		end := parseDateString(exp.EndDate)

		if _, err := tx.ExecContext(ctx, stmt,
			resumeID,
			jobTitle,
			company,
			city,
			state,
			nullTimeToInterface(start),
			nullTimeToInterface(end),
			exp.CurrentlyWorking,
			desc,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetExperiencesByResumeID fetches structured experiences for a resume.
func (m *ResumeModel) GetExperiencesByResumeID(resumeID int) ([]ExperienceRecord, error) {
	rows, err := m.DB.Query(`
		SELECT id, job_title, company, city, state, start_date, end_date, currently_working, description
		FROM experiences
		WHERE resume_id = $1
		ORDER BY start_date DESC NULLS LAST, id
	`, resumeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ExperienceRecord
	for rows.Next() {
		var rec ExperienceRecord
		var start, end sql.NullTime
		if err := rows.Scan(
			&rec.ID,
			&rec.JobTitle,
			&rec.Company,
			&rec.City,
			&rec.State,
			&start,
			&end,
			&rec.CurrentlyWorking,
			&rec.Description,
		); err != nil {
			return nil, err
		}
		if start.Valid {
			rec.StartDate = start.Time.Format("2006-01-02")
		}
		if end.Valid {
			rec.EndDate = end.Time.Format("2006-01-02")
		}
		results = append(results, rec)
	}

	return results, rows.Err()
}

func parseDateString(raw string) sql.NullTime {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sql.NullTime{}
	}

	layouts := []string{
		"2006-01-02",
		"2006-01",
		"01/2006",
		"2006",
		"Jan 2006",
		"January 2006",
		"Jan 2 2006",
		"January 2 2006",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return sql.NullTime{Time: t, Valid: true}
		}
	}
	return sql.NullTime{}
}

func nullTimeToInterface(nt sql.NullTime) interface{} {
	if nt.Valid {
		return nt
	}
	return nil
}
