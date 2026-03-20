package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
)

type rowScanner interface {
	Scan(dest ...interface{}) error
}

// JobCompany stores metadata about a company whose ATS we ingest
type JobCompany struct {
	ID                  int        `json:"id"`
	Name                string     `json:"name"`
	WebsiteURL          string     `json:"website_url"`
	CareersURL          string     `json:"careers_url"`
	ATSProvider         string     `json:"ats_provider"`
	ExternalIdentifier  *string    `json:"external_identifier"`
	IsActive            bool       `json:"is_active"`
	SyncIntervalMinutes int        `json:"sync_interval_minutes"`
	LastSyncedAt        *time.Time `json:"last_synced_at"`
	LastSyncStatus      *string    `json:"last_sync_status"`
	LastSyncError       *string    `json:"last_sync_error"`
	SyncFailureCount    int        `json:"sync_failure_count"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// JobPosting represents a single job fetched from an ATS
type JobPosting struct {
	ID             int64           `json:"id"`
	CompanyID      int             `json:"company_id"`
	ExternalJobID  string          `json:"external_job_id"`
	Title          string          `json:"title"`
	Location       string          `json:"location"`
	RemoteType     string          `json:"remote_type"`
	Department     string          `json:"department"`
	EmploymentType string          `json:"employment_type"`
	JobURL         string          `json:"job_url"`
	ApplicationURL string          `json:"application_url"`
	Description    string          `json:"description"`
	SalaryMin      *float64        `json:"salary_min"`
	SalaryMax      *float64        `json:"salary_max"`
	SalaryCurrency string          `json:"salary_currency"`
	PostedAt       *time.Time      `json:"posted_at"`
	FirstSeenAt    time.Time       `json:"first_seen_at"`
	LastSeenAt     time.Time       `json:"last_seen_at"`
	ClosedAt       *time.Time      `json:"closed_at"`
	IsActive       bool            `json:"is_active"`
	RawPayload     json.RawMessage `json:"raw_payload"`
}

// JobSyncRun captures a historical ingestion run per company
type JobSyncRun struct {
	ID          int64      `json:"id"`
	CompanyID   int        `json:"company_id"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
	Status      string     `json:"status"`
	Message     *string    `json:"message"`
	JobsFound   int        `json:"jobs_found"`
	JobsCreated int        `json:"jobs_created"`
	JobsUpdated int        `json:"jobs_updated"`
	JobsClosed  int        `json:"jobs_closed"`
}

// JobCompanyModel handles persistence for job_companies
type JobCompanyModel struct {
	db *sql.DB
}

func NewJobCompanyModel(db *sql.DB) *JobCompanyModel {
	return &JobCompanyModel{db: db}
}

func scanJobCompanyRow(scanner rowScanner) (*JobCompany, error) {
	var c JobCompany
	var website sql.NullString
	var externalID sql.NullString
	var lastStatus sql.NullString
	var lastError sql.NullString
	var lastSynced sql.NullTime
	var failureCount sql.NullInt64

	if err := scanner.Scan(
		&c.ID,
		&c.Name,
		&website,
		&c.CareersURL,
		&c.ATSProvider,
		&externalID,
		&c.IsActive,
		&c.SyncIntervalMinutes,
		&lastSynced,
		&lastStatus,
		&lastError,
		&failureCount,
		&c.CreatedAt,
		&c.UpdatedAt,
	); err != nil {
		return nil, err
	}

	c.WebsiteURL = website.String
	if externalID.Valid {
		c.ExternalIdentifier = &externalID.String
	}
	if lastStatus.Valid {
		c.LastSyncStatus = &lastStatus.String
	}
	if lastError.Valid {
		c.LastSyncError = &lastError.String
	}
	if lastSynced.Valid {
		ts := lastSynced.Time
		c.LastSyncedAt = &ts
	}
	if failureCount.Valid {
		c.SyncFailureCount = int(failureCount.Int64)
	}

	return &c, nil
}

// ListActive returns all active companies ordered by sync cadence
func (m *JobCompanyModel) ListActive() ([]*JobCompany, error) {
	const query = `
         SELECT id, name, website_url, careers_url, ats_provider,
                external_identifier, is_active, sync_interval_minutes,
                last_synced_at, last_sync_status, last_sync_error, sync_failure_count,
                created_at, updated_at
         FROM job_companies
         WHERE is_active = TRUE
         ORDER BY last_synced_at NULLS FIRST, sync_interval_minutes, id
      `
	companies, err := m.fetchCompanies(query)
	if err != nil && isUndefinedColumnError(err) {
		const legacyQuery = `
            SELECT id, name, website_url, careers_url, ats_provider,
                   external_identifier, is_active, sync_interval_minutes,
                   last_synced_at, last_sync_status,
                   NULL AS last_sync_error, 0 AS sync_failure_count,
                   created_at, updated_at
            FROM job_companies
            WHERE is_active = TRUE
            ORDER BY last_synced_at NULLS FIRST, sync_interval_minutes, id
        `
		return m.fetchCompanies(legacyQuery)
	}
	return companies, err
}

// ListAll returns all companies regardless of active flag
func (m *JobCompanyModel) ListAll() ([]*JobCompany, error) {
	const query = `
        SELECT id, name, website_url, careers_url, ats_provider,
               external_identifier, is_active, sync_interval_minutes,
               last_synced_at, last_sync_status, last_sync_error, sync_failure_count,
               created_at, updated_at
        FROM job_companies
        ORDER BY id
     `
	companies, err := m.fetchCompanies(query)
	if err != nil && isUndefinedColumnError(err) {
		const legacyQuery = `
            SELECT id, name, website_url, careers_url, ats_provider,
                   external_identifier, is_active, sync_interval_minutes,
                   last_synced_at, last_sync_status,
                   NULL AS last_sync_error, 0 AS sync_failure_count,
                   created_at, updated_at
            FROM job_companies
            ORDER BY id
        `
		return m.fetchCompanies(legacyQuery)
	}
	return companies, err
}

func (m *JobCompanyModel) ListWithPagination(status string, limit, offset int) ([]*JobCompany, int, error) {
	if limit <= 0 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}

	whereClause := ""
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		whereClause = "WHERE is_active = TRUE"
	case "inactive":
		whereClause = "WHERE is_active = FALSE"
	}

	countQuery := "SELECT COUNT(*) FROM job_companies " + whereClause
	var total int
	if err := m.db.QueryRow(countQuery).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQuery := fmt.Sprintf(`
         SELECT id, name, COALESCE(website_url, ''), careers_url, ats_provider,
                external_identifier, is_active, sync_interval_minutes,
                last_synced_at, last_sync_status, last_sync_error, sync_failure_count,
                created_at, updated_at
         FROM job_companies
         %s
         ORDER BY id DESC
         LIMIT $1 OFFSET $2
     `, whereClause)
	companies, err := m.fetchCompanies(dataQuery, limit, offset)
	if err != nil {
		if isUndefinedColumnError(err) {
			legacyQuery := fmt.Sprintf(`
                SELECT id, name, COALESCE(website_url, ''), careers_url, ats_provider,
                       external_identifier, is_active, sync_interval_minutes,
                       last_synced_at, last_sync_status,
                       NULL AS last_sync_error, 0 AS sync_failure_count,
                       created_at, updated_at
                FROM job_companies
                %s
                ORDER BY id DESC
                LIMIT $1 OFFSET $2
            `, whereClause)
			companies, err = m.fetchCompanies(legacyQuery, limit, offset)
			if err != nil {
				return nil, 0, err
			}
			return companies, total, nil
		}
		return nil, 0, err
	}

	return companies, total, nil
}

func (m *JobCompanyModel) fetchCompanies(query string, args ...interface{}) ([]*JobCompany, error) {
	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var companies []*JobCompany
	for rows.Next() {
		company, err := scanJobCompanyRow(rows)
		if err != nil {
			return nil, err
		}
		companies = append(companies, company)
	}

	return companies, rows.Err()
}

// GetByID fetches a single company by identifier
func (m *JobCompanyModel) GetByID(id int) (*JobCompany, error) {
	const query = `
         SELECT id, name, COALESCE(website_url, ''), careers_url, ats_provider,
                external_identifier, is_active, sync_interval_minutes,
                last_synced_at, last_sync_status, last_sync_error, sync_failure_count,
                created_at, updated_at
         FROM job_companies
         WHERE id = $1
     `
	company, err := scanJobCompanyRow(m.db.QueryRow(query, id))
	if err != nil {
		if isUndefinedColumnError(err) {
			const legacyQuery = `
                SELECT id, name, COALESCE(website_url, ''), careers_url, ats_provider,
                       external_identifier, is_active, sync_interval_minutes,
                       last_synced_at, last_sync_status,
                       NULL AS last_sync_error, 0 AS sync_failure_count,
                       created_at, updated_at
                FROM job_companies
                WHERE id = $1
            `
			company, err = scanJobCompanyRow(m.db.QueryRow(legacyQuery, id))
			if err != nil {
				return nil, err
			}
			return company, nil
		}
		return nil, err
	}

	return company, nil
}

// Create inserts a new company entry
func (m *JobCompanyModel) Create(company *JobCompany) error {
	const query = `
        INSERT INTO job_companies (
            name, website_url, careers_url, ats_provider, external_identifier,
            is_active, sync_interval_minutes
        ) VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id, created_at, updated_at
    `

	return m.db.QueryRow(
		query,
		company.Name,
		nullString(company.WebsiteURL),
		company.CareersURL,
		company.ATSProvider,
		nullStringPtr(company.ExternalIdentifier),
		company.IsActive,
		company.SyncIntervalMinutes,
	).Scan(&company.ID, &company.CreatedAt, &company.UpdatedAt)
}

// BulkUpsert inserts or updates a batch of job companies using careers_url as the unique key.
func (m *JobCompanyModel) BulkUpsert(companies []*JobCompany) (int, int, error) {
	if len(companies) == 0 {
		return 0, 0, nil
	}

	tx, err := m.db.Begin()
	if err != nil {
		return 0, 0, err
	}

	const query = `
		INSERT INTO job_companies (
			name, website_url, careers_url, ats_provider, external_identifier, is_active, sync_interval_minutes
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (careers_url) DO UPDATE SET
			name = EXCLUDED.name,
			website_url = EXCLUDED.website_url,
			careers_url = EXCLUDED.careers_url,
			ats_provider = EXCLUDED.ats_provider,
			external_identifier = EXCLUDED.external_identifier,
			is_active = EXCLUDED.is_active,
			sync_interval_minutes = EXCLUDED.sync_interval_minutes,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id, created_at, updated_at, (xmax = 0) AS inserted
	`

	stmt, err := tx.Prepare(query)
	if err != nil {
		_ = tx.Rollback()
		return 0, 0, err
	}
	defer stmt.Close()

	var inserted, updated int
	for _, company := range companies {
		if company == nil {
			continue
		}

		name := strings.TrimSpace(company.Name)
		careersURL := strings.TrimSpace(company.CareersURL)
		atsProvider := strings.TrimSpace(company.ATSProvider)
		if name == "" || careersURL == "" || atsProvider == "" {
			continue
		}

		if company.SyncIntervalMinutes <= 0 {
			company.SyncIntervalMinutes = 1440
		}

		var createdAt, updatedAt time.Time
		var rowInserted bool
		err = stmt.QueryRow(
			name,
			nullString(company.WebsiteURL),
			careersURL,
			atsProvider,
			nullStringPtr(company.ExternalIdentifier),
			company.IsActive,
			company.SyncIntervalMinutes,
		).Scan(&company.ID, &createdAt, &updatedAt, &rowInserted)
		if err != nil {
			stmt.Close()
			_ = tx.Rollback()
			return 0, 0, err
		}
		company.CreatedAt = createdAt
		company.UpdatedAt = updatedAt

		if rowInserted {
			inserted++
		} else {
			updated++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}

	return inserted, updated, nil
}

// UpdateSyncMetadata stores the results of the latest sync attempt
func (m *JobCompanyModel) UpdateSyncMetadata(companyID int, syncedAt time.Time, status string) error {
	const query = `
        UPDATE job_companies
        SET last_synced_at = $2,
            last_sync_status = $3,
            last_sync_error = NULL,
            sync_failure_count = 0,
            is_active = TRUE,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `
	if _, err := m.db.Exec(query, companyID, syncedAt, status); err == nil {
		return nil
	} else if !isUndefinedColumnError(err) {
		return err
	}

	const legacyErrorQuery = `
        UPDATE job_companies
        SET last_synced_at = $2,
            last_sync_status = $3,
            last_sync_error = NULL,
            is_active = TRUE,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `
	if _, err := m.db.Exec(legacyErrorQuery, companyID, syncedAt, status); err == nil {
		return nil
	} else if !isUndefinedColumnError(err) {
		return err
	}

	const legacyMinimalQuery = `
        UPDATE job_companies
        SET last_synced_at = $2,
            last_sync_status = $3,
            is_active = TRUE,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `
	_, err := m.db.Exec(legacyMinimalQuery, companyID, syncedAt, status)
	return err
}

func (m *JobCompanyModel) RecordSyncFailure(companyID int, syncedAt time.Time, status, errorMessage string) error {
	trimmed := strings.TrimSpace(errorMessage)
	const query = `
        UPDATE job_companies
        SET last_synced_at = $2,
            last_sync_status = $3,
            last_sync_error = NULLIF($4, ''),
            sync_failure_count = sync_failure_count + 1,
            is_active = FALSE,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `
	if _, err := m.db.Exec(query, companyID, syncedAt, status, trimmed); err == nil {
		return nil
	} else if !isUndefinedColumnError(err) {
		return err
	}

	const legacyErrorQuery = `
        UPDATE job_companies
        SET last_synced_at = $2,
            last_sync_status = $3,
            last_sync_error = NULLIF($4, ''),
            is_active = FALSE,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `
	if _, err := m.db.Exec(legacyErrorQuery, companyID, syncedAt, status, trimmed); err == nil {
		return nil
	} else if !isUndefinedColumnError(err) {
		return err
	}

	const legacyMinimalQuery = `
        UPDATE job_companies
        SET last_synced_at = $2,
            last_sync_status = $3,
            is_active = FALSE,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `
	_, err := m.db.Exec(legacyMinimalQuery, companyID, syncedAt, status)
	return err
}

func (m *JobCompanyModel) SetCompanyActive(companyID int, active bool) error {
	if active {
		const query = `
            UPDATE job_companies
            SET is_active = TRUE,
                sync_failure_count = 0,
                last_sync_error = NULL,
                updated_at = CURRENT_TIMESTAMP
            WHERE id = $1
        `
		if _, err := m.db.Exec(query, companyID); err == nil {
			return nil
		} else if !isUndefinedColumnError(err) {
			return err
		}

		const legacyQuery = `
            UPDATE job_companies
            SET is_active = TRUE,
                updated_at = CURRENT_TIMESTAMP
            WHERE id = $1
        `
		_, err := m.db.Exec(legacyQuery, companyID)
		return err
	}
	const query = `
        UPDATE job_companies
        SET is_active = FALSE,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `
	_, err := m.db.Exec(query, companyID)
	return err
}

// JobPostingModel manages job_postings records
type JobPostingModel struct {
	db *sql.DB
}

func NewJobPostingModel(db *sql.DB) *JobPostingModel {
	return &JobPostingModel{db: db}
}

// GetByID fetches a single job posting with its company name.
func (m *JobPostingModel) GetByID(id int64) (*JobPosting, *string, error) {
	if id <= 0 {
		return nil, nil, fmt.Errorf("invalid job id")
	}

	query := `
        SELECT p.id, p.company_id, p.external_job_id, p.title,
               COALESCE(p.location, ''), COALESCE(p.remote_type, ''),
               COALESCE(p.department, ''), COALESCE(p.employment_type, ''),
               p.job_url, COALESCE(p.application_url, ''),
               COALESCE(p.description, ''), p.salary_min, p.salary_max, COALESCE(p.salary_currency, ''),
               p.posted_at, p.first_seen_at, p.last_seen_at, p.closed_at, p.is_active,
               c.name as company_name
        FROM job_postings p
        LEFT JOIN job_companies c ON c.id = p.company_id
        WHERE p.id = $1
    `

	row := m.db.QueryRow(query, id)
	var posting JobPosting
	var postedAt sql.NullTime
	var closedAt sql.NullTime
	var companyName sql.NullString
	if err := row.Scan(
		&posting.ID,
		&posting.CompanyID,
		&posting.ExternalJobID,
		&posting.Title,
		&posting.Location,
		&posting.RemoteType,
		&posting.Department,
		&posting.EmploymentType,
		&posting.JobURL,
		&posting.ApplicationURL,
		&posting.Description,
		&posting.SalaryMin,
		&posting.SalaryMax,
		&posting.SalaryCurrency,
		&postedAt,
		&posting.FirstSeenAt,
		&posting.LastSeenAt,
		&closedAt,
		&posting.IsActive,
		&companyName,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, err
		}
		return nil, nil, err
	}
	if postedAt.Valid {
		posting.PostedAt = &postedAt.Time
	}
	if closedAt.Valid {
		posting.ClosedAt = &closedAt.Time
	}
	var namePtr *string
	if companyName.Valid {
		name := companyName.String
		namePtr = &name
	}
	return &posting, namePtr, nil
}

// Upsert creates or updates a job posting and refreshes last_seen_at
func (m *JobPostingModel) Upsert(posting *JobPosting) (bool, error) {
	rawPayload := []byte(nil)
	if len(posting.RawPayload) > 0 {
		rawPayload = posting.RawPayload
	}

	const query = `
        INSERT INTO job_postings (
            company_id, external_job_id, title, location, remote_type,
            department, employment_type, job_url, application_url, description,
            salary_min, salary_max, salary_currency, posted_at, raw_payload
        ) VALUES (
            $1, $2, $3, NULLIF($4, ''), NULLIF($5, ''),
            NULLIF($6, ''), NULLIF($7, ''), $8, NULLIF($9, ''), NULLIF($10, ''),
            $11, $12, NULLIF($13, ''), $14, $15
        )
        ON CONFLICT (company_id, external_job_id)
        DO UPDATE SET
            title = EXCLUDED.title,
            location = EXCLUDED.location,
            remote_type = EXCLUDED.remote_type,
            department = EXCLUDED.department,
            employment_type = EXCLUDED.employment_type,
            job_url = EXCLUDED.job_url,
            application_url = EXCLUDED.application_url,
            description = EXCLUDED.description,
            salary_min = EXCLUDED.salary_min,
            salary_max = EXCLUDED.salary_max,
            salary_currency = EXCLUDED.salary_currency,
            posted_at = EXCLUDED.posted_at,
            raw_payload = EXCLUDED.raw_payload,
            is_active = TRUE,
            last_seen_at = CURRENT_TIMESTAMP,
            closed_at = NULL,
            embedding = CASE
                WHEN EXCLUDED.description IS DISTINCT FROM job_postings.description
                  OR EXCLUDED.title IS DISTINCT FROM job_postings.title
                  OR EXCLUDED.department IS DISTINCT FROM job_postings.department
                THEN NULL
                ELSE job_postings.embedding
            END
        RETURNING id, first_seen_at, last_seen_at, is_active, (xmax = 0) AS inserted
    `

	var firstSeen, lastSeen time.Time
	var isActive bool
	var inserted bool

	err := m.db.QueryRow(
		query,
		posting.CompanyID,
		posting.ExternalJobID,
		posting.Title,
		posting.Location,
		posting.RemoteType,
		posting.Department,
		posting.EmploymentType,
		posting.JobURL,
		posting.ApplicationURL,
		posting.Description,
		posting.SalaryMin,
		posting.SalaryMax,
		posting.SalaryCurrency,
		posting.PostedAt,
		rawPayload,
	).Scan(&posting.ID, &firstSeen, &lastSeen, &isActive, &inserted)
	if err != nil {
		return false, err
	}

	posting.FirstSeenAt = firstSeen
	posting.LastSeenAt = lastSeen
	posting.IsActive = isActive
	return inserted, nil
}

// DeactivateMissing marks jobs not returned in the latest sync as closed

// ListActive returns recent active postings optionally filtered by company ID
func (m *JobPostingModel) ListActive(companyID *int, limit int) ([]*JobPosting, error) {
	if limit <= 0 {
		limit = 100
	}

	args := make([]interface{}, 0, 2)
	where := []string{"is_active = TRUE"}
	placeholder := 1
	if companyID != nil {
		where = append(where, fmt.Sprintf("company_id = $%d", placeholder))
		args = append(args, *companyID)
		placeholder++
	}

	args = append(args, limit)
	query := fmt.Sprintf(`
        SELECT id, company_id, external_job_id, title,
               COALESCE(location, ''), COALESCE(remote_type, ''),
               COALESCE(department, ''), COALESCE(employment_type, ''),
               job_url, COALESCE(application_url, ''),
               COALESCE(description, ''), salary_min, salary_max, COALESCE(salary_currency, ''),
               posted_at, first_seen_at, last_seen_at, closed_at, is_active
        FROM job_postings
        WHERE %s
        ORDER BY COALESCE(posted_at, first_seen_at) DESC
        LIMIT $%d
    `, strings.Join(where, " AND "), placeholder)

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var postings []*JobPosting
	for rows.Next() {
		var posting JobPosting
		var postedAt sql.NullTime
		var closedAt sql.NullTime
		var location sql.NullString
		var remoteType sql.NullString
		var department sql.NullString
		var employmentType sql.NullString
		var applicationURL sql.NullString
		var description sql.NullString
		var salaryCurrency sql.NullString

		if err := rows.Scan(
			&posting.ID,
			&posting.CompanyID,
			&posting.ExternalJobID,
			&posting.Title,
			&location,
			&remoteType,
			&department,
			&employmentType,
			&posting.JobURL,
			&applicationURL,
			&description,
			&posting.SalaryMin,
			&posting.SalaryMax,
			&salaryCurrency,
			&postedAt,
			&posting.FirstSeenAt,
			&posting.LastSeenAt,
			&closedAt,
			&posting.IsActive,
		); err != nil {
			return nil, err
		}

		posting.Location = location.String
		posting.RemoteType = remoteType.String
		posting.Department = department.String
		posting.EmploymentType = employmentType.String
		posting.ApplicationURL = applicationURL.String
		posting.Description = description.String
		posting.SalaryCurrency = salaryCurrency.String

		if postedAt.Valid {
			ts := postedAt.Time
			posting.PostedAt = &ts
		}
		if closedAt.Valid {
			ts := closedAt.Time
			posting.ClosedAt = &ts
		}

		postings = append(postings, &posting)
	}

	return postings, rows.Err()
}

// ListActiveByRelevance returns active job postings filtered by skills and position keywords.
// This performs a broad SQL-level filter (OR-based) to get a larger pool of relevant jobs
// before detailed scoring is applied. Returns up to `limit` jobs.
func (m *JobPostingModel) ListActiveByRelevance(skills []string, position string, limit int) ([]*JobPosting, error) {
	if limit <= 0 {
		limit = 500
	}

	// Build search terms from skills and position
	searchTerms := make([]string, 0, len(skills)+5)
	for _, skill := range skills {
		trimmed := strings.TrimSpace(strings.ToLower(skill))
		if trimmed != "" && len(trimmed) >= 2 {
			searchTerms = append(searchTerms, trimmed)
		}
	}

	// Add position keywords
	positionLower := strings.ToLower(strings.TrimSpace(position))
	if positionLower != "" {
		// Split position into words and add significant ones
		for _, word := range strings.Fields(positionLower) {
			if len(word) >= 3 && word != "the" && word != "and" && word != "for" {
				searchTerms = append(searchTerms, word)
			}
		}
	}

	// If no search terms, fall back to ListActive
	if len(searchTerms) == 0 {
		return m.ListActive(nil, limit)
	}

	// Build OR conditions for each search term matching title or description
	conditions := make([]string, 0, len(searchTerms))
	args := make([]interface{}, 0, len(searchTerms)+1)
	for i, term := range searchTerms {
		pattern := "%" + term + "%"
		placeholder := i + 1
		conditions = append(conditions, fmt.Sprintf("(LOWER(title) LIKE $%d OR LOWER(COALESCE(description, '')) LIKE $%d)", placeholder, placeholder))
		args = append(args, pattern)
	}

	args = append(args, limit)
	limitPlaceholder := len(searchTerms) + 1

	query := fmt.Sprintf(`
		SELECT id, company_id, external_job_id, title,
		       COALESCE(location, ''), COALESCE(remote_type, ''),
		       COALESCE(department, ''), COALESCE(employment_type, ''),
		       job_url, COALESCE(application_url, ''),
		       COALESCE(description, ''), salary_min, salary_max, COALESCE(salary_currency, ''),
		       posted_at, first_seen_at, last_seen_at, closed_at, is_active
		FROM job_postings
		WHERE is_active = TRUE
		  AND (%s)
		ORDER BY COALESCE(posted_at, first_seen_at) DESC
		LIMIT $%d
	`, strings.Join(conditions, " OR "), limitPlaceholder)

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var postings []*JobPosting
	for rows.Next() {
		var posting JobPosting
		var postedAt sql.NullTime
		var closedAt sql.NullTime
		var location sql.NullString
		var remoteType sql.NullString
		var department sql.NullString
		var employmentType sql.NullString
		var applicationURL sql.NullString
		var description sql.NullString
		var salaryCurrency sql.NullString

		if err := rows.Scan(
			&posting.ID,
			&posting.CompanyID,
			&posting.ExternalJobID,
			&posting.Title,
			&location,
			&remoteType,
			&department,
			&employmentType,
			&posting.JobURL,
			&applicationURL,
			&description,
			&posting.SalaryMin,
			&posting.SalaryMax,
			&salaryCurrency,
			&postedAt,
			&posting.FirstSeenAt,
			&posting.LastSeenAt,
			&closedAt,
			&posting.IsActive,
		); err != nil {
			return nil, err
		}

		posting.Location = location.String
		posting.RemoteType = remoteType.String
		posting.Department = department.String
		posting.EmploymentType = employmentType.String
		posting.ApplicationURL = applicationURL.String
		posting.Description = description.String
		posting.SalaryCurrency = salaryCurrency.String

		if postedAt.Valid {
			ts := postedAt.Time
			posting.PostedAt = &ts
		}
		if closedAt.Valid {
			ts := closedAt.Time
			posting.ClosedAt = &ts
		}

		postings = append(postings, &posting)
	}

	return postings, rows.Err()
}

func (m *JobPostingModel) CountActiveByKeyword(keyword string) (int, error) {
	if keyword == "" {
		return 0, fmt.Errorf("keyword required")
	}
	pattern := fmt.Sprintf("%%%s%%", strings.ToLower(keyword))
	var count int
	err := m.db.QueryRow(`
        SELECT COUNT(*)
        FROM job_postings
        WHERE is_active = TRUE
          AND LOWER(title) LIKE $1
    `, pattern).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (m *JobPostingModel) DeactivateMissing(companyID int, activeExternalIDs []string, closedAt time.Time) (int64, error) {
	if len(activeExternalIDs) == 0 {
		const query = `
            UPDATE job_postings
            SET is_active = FALSE,
                closed_at = COALESCE(closed_at, $2),
                last_seen_at = CURRENT_TIMESTAMP
            WHERE company_id = $1 AND is_active = TRUE
        `
		res, err := m.db.Exec(query, companyID, closedAt)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}

	const query = `
        UPDATE job_postings
        SET is_active = FALSE,
            closed_at = COALESCE(closed_at, $3),
            last_seen_at = CURRENT_TIMESTAMP
        WHERE company_id = $1
          AND is_active = TRUE
          AND external_job_id != ALL($2)
    `
	res, err := m.db.Exec(query, companyID, pqArray(activeExternalIDs), closedAt)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteInactiveBefore removes inactive job postings that were last seen before the cutoff.
// Returns the count deleted for this batch.
func (m *JobPostingModel) DeleteInactiveBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 5000
	}

	const query = `
		DELETE FROM job_postings
		WHERE id IN (
			SELECT id
			FROM job_postings
			WHERE is_active = FALSE
			  AND COALESCE(closed_at, last_seen_at, first_seen_at) < $1
			ORDER BY id
			LIMIT $2
		)
	`

	res, err := m.db.ExecContext(ctx, query, cutoff, batchSize)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListActiveByVectorSimilarity returns active job postings whose embedding cosine
// similarity to the given vector meets the threshold, ordered by similarity.
func (m *JobPostingModel) ListActiveByVectorSimilarity(ctx context.Context, embedding []float32, limit int, threshold float32) ([]*JobPosting, error) {
	if limit <= 0 {
		limit = 500
	}

	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "SET LOCAL hnsw.ef_search = 100"); err != nil {
		return nil, err
	}

	const query = `
		SELECT id, company_id, external_job_id, title,
		       COALESCE(location, ''), COALESCE(remote_type, ''),
		       COALESCE(department, ''), COALESCE(employment_type, ''),
		       job_url, COALESCE(application_url, ''),
		       COALESCE(description, ''), salary_min, salary_max, COALESCE(salary_currency, ''),
		       posted_at, first_seen_at, last_seen_at, closed_at, is_active
		FROM job_postings
		WHERE is_active = TRUE
		  AND embedding IS NOT NULL
		  AND 1 - (embedding <=> $1::halfvec) >= $2
		ORDER BY embedding <=> $1::halfvec
		LIMIT $3
	`

	rows, err := tx.QueryContext(ctx, query, pgvector.NewVector(embedding), threshold, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var postings []*JobPosting
	for rows.Next() {
		var posting JobPosting
		var postedAt sql.NullTime
		var closedAt sql.NullTime
		var location sql.NullString
		var remoteType sql.NullString
		var department sql.NullString
		var employmentType sql.NullString
		var applicationURL sql.NullString
		var description sql.NullString
		var salaryCurrency sql.NullString

		if err := rows.Scan(
			&posting.ID,
			&posting.CompanyID,
			&posting.ExternalJobID,
			&posting.Title,
			&location,
			&remoteType,
			&department,
			&employmentType,
			&posting.JobURL,
			&applicationURL,
			&description,
			&posting.SalaryMin,
			&posting.SalaryMax,
			&salaryCurrency,
			&postedAt,
			&posting.FirstSeenAt,
			&posting.LastSeenAt,
			&closedAt,
			&posting.IsActive,
		); err != nil {
			return nil, err
		}

		posting.Location = location.String
		posting.RemoteType = remoteType.String
		posting.Department = department.String
		posting.EmploymentType = employmentType.String
		posting.ApplicationURL = applicationURL.String
		posting.Description = description.String
		posting.SalaryCurrency = salaryCurrency.String

		if postedAt.Valid {
			ts := postedAt.Time
			posting.PostedAt = &ts
		}
		if closedAt.Valid {
			ts := closedAt.Time
			posting.ClosedAt = &ts
		}

		postings = append(postings, &posting)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return postings, nil
}

// UpdateEmbedding sets the embedding vector for a job posting by ID.
func (m *JobPostingModel) UpdateEmbedding(ctx context.Context, id int64, embedding []float32) error {
	res, err := m.db.ExecContext(ctx,
		`UPDATE job_postings SET embedding = $1::halfvec WHERE id = $2`,
		pgvector.NewVector(embedding), id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("job posting %d not found", id)
	}
	return nil
}

// ListActiveWithNullEmbedding returns IDs of active job postings that have no embedding yet.
// When sinceDays > 0, only jobs posted within the last sinceDays days are returned.
func (m *JobPostingModel) ListActiveWithNullEmbedding(limit int, sinceDays int) ([]int64, error) {
	if limit <= 0 {
		limit = 500
	}
	var rows *sql.Rows
	var err error
	if sinceDays > 0 {
		// $2::text cast required — PostgreSQL cannot concatenate int directly into interval
		rows, err = m.db.Query(
			`SELECT id FROM job_postings
			 WHERE is_active = TRUE AND embedding IS NULL
			   AND posted_at >= NOW() - ($2::text || ' days')::interval
			 ORDER BY id LIMIT $1`,
			limit, sinceDays,
		)
	} else {
		rows, err = m.db.Query(
			`SELECT id FROM job_postings WHERE is_active = TRUE AND embedding IS NULL ORDER BY id LIMIT $1`,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CountActiveWithNullEmbedding returns the number of active postings without an embedding.
// When sinceDays > 0, only jobs posted within the last sinceDays days are counted.
func (m *JobPostingModel) CountActiveWithNullEmbedding(sinceDays int) (int, error) {
	var count int
	var err error
	if sinceDays > 0 {
		err = m.db.QueryRow(
			`SELECT COUNT(*) FROM job_postings
			 WHERE is_active = TRUE AND embedding IS NULL
			   AND posted_at >= NOW() - ($1::text || ' days')::interval`,
			sinceDays,
		).Scan(&count)
	} else {
		err = m.db.QueryRow(
			`SELECT COUNT(*) FROM job_postings WHERE is_active = TRUE AND embedding IS NULL`,
		).Scan(&count)
	}
	return count, err
}

// GetByIDForEmbedding fetches the fields needed to generate a job embedding.
func (m *JobPostingModel) GetByIDForEmbedding(ctx context.Context, id int64) (*JobPosting, error) {
	var posting JobPosting
	err := m.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(title,''), COALESCE(department,''), COALESCE(description,'')
		 FROM job_postings WHERE id = $1`,
		id,
	).Scan(&posting.ID, &posting.Title, &posting.Department, &posting.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("job posting %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	return &posting, nil
}

// JobSyncRunModel records historical sync attempts
type JobSyncRunModel struct {
	db *sql.DB
}

func NewJobSyncRunModel(db *sql.DB) *JobSyncRunModel {
	return &JobSyncRunModel{db: db}
}

func (m *JobSyncRunModel) Start(companyID int) (*JobSyncRun, error) {
	const query = `
        INSERT INTO job_sync_runs (company_id, status)
        VALUES ($1, 'running')
        RETURNING id, started_at
    `
	var run JobSyncRun
	run.CompanyID = companyID
	run.Status = "running"
	err := m.db.QueryRow(query, companyID).Scan(&run.ID, &run.StartedAt)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (m *JobSyncRunModel) Finish(runID int64, status string, message *string, metrics map[string]int) error {
	const query = `
        UPDATE job_sync_runs
        SET finished_at = CURRENT_TIMESTAMP,
            status = $2,
            message = $3,
            jobs_found = COALESCE($4, jobs_found),
            jobs_created = COALESCE($5, jobs_created),
            jobs_updated = COALESCE($6, jobs_updated),
            jobs_closed = COALESCE($7, jobs_closed)
        WHERE id = $1
    `
	_, err := m.db.Exec(
		query,
		runID,
		status,
		message,
		metrics["found"],
		metrics["created"],
		metrics["updated"],
		metrics["closed"],
	)
	return err
}

// ---------------------------------------------------------------------------
// Admin job posting management types and methods
// ---------------------------------------------------------------------------

// JobPostingFilterParams controls pagination, search and filters for admin listing.
type JobPostingFilterParams struct {
	Page           int
	PageSize       int
	Search         string
	CompanyID      *int
	IsActive       *bool
	RemoteType     string
	EmploymentType string
	SeniorityLevel *int
	DateFrom       *time.Time
	DateTo         *time.Time
	SortBy         string
	SortDir        string
}

// JobPostingWithCompany extends JobPosting with joined company name and seniority.
type JobPostingWithCompany struct {
	JobPosting
	CompanyName    string `json:"company_name"`
	SeniorityLevel int    `json:"seniority_level"`
}

// SyncRunFilterParams controls pagination and filters for sync run listing.
type SyncRunFilterParams struct {
	Page      int
	PageSize  int
	CompanyID *int
	Status    string
	DateFrom  *time.Time
	DateTo    *time.Time
}

// JobSyncRunWithCompany extends JobSyncRun with joined company name.
type JobSyncRunWithCompany struct {
	JobSyncRun
	CompanyName string `json:"company_name"`
}

// JobStatistics holds aggregate counts for the admin dashboard.
type JobStatistics struct {
	TotalPostings    int                    `json:"total_postings"`
	ActivePostings   int                    `json:"active_postings"`
	InactivePostings int                    `json:"inactive_postings"`
	ByCompany        []CompanyJobCount      `json:"by_company"`
	BySeniority      []SeniorityJobCount    `json:"by_seniority"`
	ByRemoteType     []RemoteTypeJobCount   `json:"by_remote_type"`
	ByEmploymentType []EmploymentTypeJobCount `json:"by_employment_type"`
	RecentTrend      []DailyJobCount        `json:"recent_trend"`
}

type CompanyJobCount struct {
	CompanyID   int    `json:"company_id"`
	CompanyName string `json:"company_name"`
	Total       int    `json:"total"`
	Active      int    `json:"active"`
}

type SeniorityJobCount struct {
	SeniorityLevel int `json:"seniority_level"`
	Count          int `json:"count"`
}

type RemoteTypeJobCount struct {
	RemoteType string `json:"remote_type"`
	Count      int    `json:"count"`
}

type EmploymentTypeJobCount struct {
	EmploymentType string `json:"employment_type"`
	Count          int    `json:"count"`
}

type DailyJobCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

var allowedSortColumns = map[string]string{
	"id":            "p.id",
	"title":         "p.title",
	"company_name":  "c.name",
	"location":      "p.location",
	"posted_at":     "p.posted_at",
	"first_seen_at": "p.first_seen_at",
	"last_seen_at":  "p.last_seen_at",
	"seniority_level": "p.seniority_level",
}

var allowedUpdateFields = map[string]string{
	"title":           "title",
	"location":        "location",
	"salary_min":      "salary_min",
	"salary_max":      "salary_max",
	"salary_currency": "salary_currency",
	"seniority_level": "seniority_level",
	"remote_type":     "remote_type",
	"employment_type": "employment_type",
	"is_active":       "is_active",
}

// ListWithFilters returns paginated job postings with dynamic filters for admin use.
func (m *JobPostingModel) ListWithFilters(params JobPostingFilterParams) ([]JobPostingWithCompany, int, error) {
	if params.PageSize <= 0 {
		params.PageSize = 25
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	if params.Page <= 0 {
		params.Page = 1
	}

	where := []string{"1=1"}
	args := []interface{}{}
	placeholder := 1

	if params.Search != "" {
		pattern := "%" + strings.ToLower(strings.TrimSpace(params.Search)) + "%"
		where = append(where, fmt.Sprintf("(LOWER(p.title) LIKE $%d OR LOWER(COALESCE(p.location, '')) LIKE $%d)", placeholder, placeholder))
		args = append(args, pattern)
		placeholder++
	}
	if params.CompanyID != nil {
		where = append(where, fmt.Sprintf("p.company_id = $%d", placeholder))
		args = append(args, *params.CompanyID)
		placeholder++
	}
	if params.IsActive != nil {
		where = append(where, fmt.Sprintf("p.is_active = $%d", placeholder))
		args = append(args, *params.IsActive)
		placeholder++
	}
	if params.RemoteType != "" {
		where = append(where, fmt.Sprintf("p.remote_type = $%d", placeholder))
		args = append(args, params.RemoteType)
		placeholder++
	}
	if params.EmploymentType != "" {
		where = append(where, fmt.Sprintf("p.employment_type = $%d", placeholder))
		args = append(args, params.EmploymentType)
		placeholder++
	}
	if params.SeniorityLevel != nil {
		where = append(where, fmt.Sprintf("p.seniority_level = $%d", placeholder))
		args = append(args, *params.SeniorityLevel)
		placeholder++
	}
	if params.DateFrom != nil {
		where = append(where, fmt.Sprintf("COALESCE(p.posted_at, p.first_seen_at) >= $%d", placeholder))
		args = append(args, *params.DateFrom)
		placeholder++
	}
	if params.DateTo != nil {
		where = append(where, fmt.Sprintf("COALESCE(p.posted_at, p.first_seen_at) <= $%d", placeholder))
		args = append(args, *params.DateTo)
		placeholder++
	}

	whereClause := strings.Join(where, " AND ")

	// Count query
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM job_postings p LEFT JOIN job_companies c ON c.id = p.company_id WHERE %s", whereClause)
	var total int
	if err := m.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Sort
	orderCol := "p.id"
	if col, ok := allowedSortColumns[params.SortBy]; ok {
		orderCol = col
	}
	orderDir := "DESC"
	if strings.ToUpper(params.SortDir) == "ASC" {
		orderDir = "ASC"
	}

	offset := (params.Page - 1) * params.PageSize
	args = append(args, params.PageSize, offset)

	dataQuery := fmt.Sprintf(`
		SELECT p.id, p.company_id, p.external_job_id, p.title,
		       COALESCE(p.location, ''), COALESCE(p.remote_type, ''),
		       COALESCE(p.department, ''), COALESCE(p.employment_type, ''),
		       p.job_url, COALESCE(p.application_url, ''),
		       LEFT(COALESCE(p.description, ''), 200),
		       p.salary_min, p.salary_max, COALESCE(p.salary_currency, ''),
		       p.posted_at, p.first_seen_at, p.last_seen_at, p.closed_at, p.is_active,
		       COALESCE(c.name, '') AS company_name,
		       p.seniority_level
		FROM job_postings p
		LEFT JOIN job_companies c ON c.id = p.company_id
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, orderCol, orderDir, placeholder, placeholder+1)

	rows, err := m.db.Query(dataQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var postings []JobPostingWithCompany
	for rows.Next() {
		var p JobPostingWithCompany
		var postedAt sql.NullTime
		var closedAt sql.NullTime
		if err := rows.Scan(
			&p.ID, &p.CompanyID, &p.ExternalJobID, &p.Title,
			&p.Location, &p.RemoteType,
			&p.Department, &p.EmploymentType,
			&p.JobURL, &p.ApplicationURL,
			&p.Description,
			&p.SalaryMin, &p.SalaryMax, &p.SalaryCurrency,
			&postedAt, &p.FirstSeenAt, &p.LastSeenAt, &closedAt, &p.IsActive,
			&p.CompanyName,
			&p.SeniorityLevel,
		); err != nil {
			return nil, 0, err
		}
		if postedAt.Valid {
			p.PostedAt = &postedAt.Time
		}
		if closedAt.Valid {
			p.ClosedAt = &closedAt.Time
		}
		postings = append(postings, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return postings, total, nil
}

// GetByIDFull returns a single job posting with company name, raw_payload and seniority_level.
func (m *JobPostingModel) GetByIDFull(id int64) (*JobPostingWithCompany, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid job id")
	}

	const query = `
		SELECT p.id, p.company_id, p.external_job_id, p.title,
		       COALESCE(p.location, ''), COALESCE(p.remote_type, ''),
		       COALESCE(p.department, ''), COALESCE(p.employment_type, ''),
		       p.job_url, COALESCE(p.application_url, ''),
		       COALESCE(p.description, ''),
		       p.salary_min, p.salary_max, COALESCE(p.salary_currency, ''),
		       p.posted_at, p.first_seen_at, p.last_seen_at, p.closed_at, p.is_active,
		       p.raw_payload,
		       COALESCE(c.name, '') AS company_name,
		       p.seniority_level
		FROM job_postings p
		LEFT JOIN job_companies c ON c.id = p.company_id
		WHERE p.id = $1
	`

	var p JobPostingWithCompany
	var postedAt sql.NullTime
	var closedAt sql.NullTime
	if err := m.db.QueryRow(query, id).Scan(
		&p.ID, &p.CompanyID, &p.ExternalJobID, &p.Title,
		&p.Location, &p.RemoteType,
		&p.Department, &p.EmploymentType,
		&p.JobURL, &p.ApplicationURL,
		&p.Description,
		&p.SalaryMin, &p.SalaryMax, &p.SalaryCurrency,
		&postedAt, &p.FirstSeenAt, &p.LastSeenAt, &closedAt, &p.IsActive,
		&p.RawPayload,
		&p.CompanyName,
		&p.SeniorityLevel,
	); err != nil {
		return nil, err
	}
	if postedAt.Valid {
		p.PostedAt = &postedAt.Time
	}
	if closedAt.Valid {
		p.ClosedAt = &closedAt.Time
	}
	return &p, nil
}

// UpdatePosting updates whitelisted fields on a job posting.
func (m *JobPostingModel) UpdatePosting(id int64, fields map[string]interface{}) error {
	if id <= 0 {
		return fmt.Errorf("invalid job id")
	}

	setClauses := []string{}
	args := []interface{}{}
	placeholder := 1

	for key, value := range fields {
		col, ok := allowedUpdateFields[key]
		if !ok {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, placeholder))
		args = append(args, value)
		placeholder++
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no valid fields to update")
	}

	setClauses = append(setClauses, "last_seen_at = CURRENT_TIMESTAMP")
	args = append(args, id)

	query := fmt.Sprintf("UPDATE job_postings SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), placeholder)

	res, err := m.db.Exec(query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeletePosting removes a job posting by ID.
func (m *JobPostingModel) DeletePosting(id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid job id")
	}
	res, err := m.db.Exec("DELETE FROM job_postings WHERE id = $1", id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// BulkUpdateActive sets is_active for a list of posting IDs.
func (m *JobPostingModel) BulkUpdateActive(ids []int64, active bool) (int64, error) {
	if len(ids) == 0 {
		return 0, fmt.Errorf("ids required")
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	args[0] = active
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}
	query := fmt.Sprintf(
		"UPDATE job_postings SET is_active = $1, last_seen_at = CURRENT_TIMESTAMP WHERE id IN (%s)",
		strings.Join(placeholders, ", "),
	)
	res, err := m.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetStatistics returns aggregate job posting counts for the admin dashboard.
func (m *JobPostingModel) GetStatistics() (*JobStatistics, error) {
	stats := &JobStatistics{}

	// Total / active / inactive
	err := m.db.QueryRow(`
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE is_active = TRUE),
			COUNT(*) FILTER (WHERE is_active = FALSE)
		FROM job_postings
	`).Scan(&stats.TotalPostings, &stats.ActivePostings, &stats.InactivePostings)
	if err != nil {
		return nil, err
	}

	// By company (top 50)
	rows, err := m.db.Query(`
		SELECT c.id, COALESCE(c.name, 'Unknown'), COUNT(*), COUNT(*) FILTER (WHERE p.is_active = TRUE)
		FROM job_postings p
		LEFT JOIN job_companies c ON c.id = p.company_id
		GROUP BY c.id, c.name
		ORDER BY COUNT(*) DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r CompanyJobCount
		if err := rows.Scan(&r.CompanyID, &r.CompanyName, &r.Total, &r.Active); err != nil {
			return nil, err
		}
		stats.ByCompany = append(stats.ByCompany, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// By seniority
	senRows, err := m.db.Query(`
		SELECT seniority_level, COUNT(*) FROM job_postings GROUP BY seniority_level ORDER BY seniority_level
	`)
	if err != nil {
		return nil, err
	}
	defer senRows.Close()
	for senRows.Next() {
		var r SeniorityJobCount
		if err := senRows.Scan(&r.SeniorityLevel, &r.Count); err != nil {
			return nil, err
		}
		stats.BySeniority = append(stats.BySeniority, r)
	}
	if err := senRows.Err(); err != nil {
		return nil, err
	}

	// By remote type
	remoteRows, err := m.db.Query(`
		SELECT COALESCE(remote_type, 'unspecified'), COUNT(*) FROM job_postings GROUP BY remote_type ORDER BY COUNT(*) DESC
	`)
	if err != nil {
		return nil, err
	}
	defer remoteRows.Close()
	for remoteRows.Next() {
		var r RemoteTypeJobCount
		if err := remoteRows.Scan(&r.RemoteType, &r.Count); err != nil {
			return nil, err
		}
		stats.ByRemoteType = append(stats.ByRemoteType, r)
	}
	if err := remoteRows.Err(); err != nil {
		return nil, err
	}

	// By employment type
	empRows, err := m.db.Query(`
		SELECT COALESCE(employment_type, 'unspecified'), COUNT(*) FROM job_postings GROUP BY employment_type ORDER BY COUNT(*) DESC
	`)
	if err != nil {
		return nil, err
	}
	defer empRows.Close()
	for empRows.Next() {
		var r EmploymentTypeJobCount
		if err := empRows.Scan(&r.EmploymentType, &r.Count); err != nil {
			return nil, err
		}
		stats.ByEmploymentType = append(stats.ByEmploymentType, r)
	}
	if err := empRows.Err(); err != nil {
		return nil, err
	}

	// 30-day trend based on first_seen_at
	trendRows, err := m.db.Query(`
		SELECT first_seen_at::date AS d, COUNT(*)
		FROM job_postings
		WHERE first_seen_at >= CURRENT_DATE - INTERVAL '30 days'
		GROUP BY d
		ORDER BY d DESC
	`)
	if err != nil {
		return nil, err
	}
	defer trendRows.Close()
	for trendRows.Next() {
		var r DailyJobCount
		var d time.Time
		if err := trendRows.Scan(&d, &r.Count); err != nil {
			return nil, err
		}
		r.Date = d.Format("2006-01-02")
		stats.RecentTrend = append(stats.RecentTrend, r)
	}
	if err := trendRows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}

// ListWithFilters returns paginated sync runs with company name for admin use.
func (m *JobSyncRunModel) ListWithFilters(params SyncRunFilterParams) ([]JobSyncRunWithCompany, int, error) {
	if params.PageSize <= 0 {
		params.PageSize = 25
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	if params.Page <= 0 {
		params.Page = 1
	}

	where := []string{"1=1"}
	args := []interface{}{}
	placeholder := 1

	if params.CompanyID != nil {
		where = append(where, fmt.Sprintf("r.company_id = $%d", placeholder))
		args = append(args, *params.CompanyID)
		placeholder++
	}
	if params.Status != "" {
		where = append(where, fmt.Sprintf("r.status = $%d", placeholder))
		args = append(args, params.Status)
		placeholder++
	}
	if params.DateFrom != nil {
		where = append(where, fmt.Sprintf("r.started_at >= $%d", placeholder))
		args = append(args, *params.DateFrom)
		placeholder++
	}
	if params.DateTo != nil {
		where = append(where, fmt.Sprintf("r.started_at <= $%d", placeholder))
		args = append(args, *params.DateTo)
		placeholder++
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM job_sync_runs r WHERE %s", whereClause)
	if err := m.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PageSize
	args = append(args, params.PageSize, offset)

	dataQuery := fmt.Sprintf(`
		SELECT r.id, r.company_id, r.started_at, r.finished_at, r.status, r.message,
		       r.jobs_found, r.jobs_created, r.jobs_updated, r.jobs_closed,
		       COALESCE(c.name, '') AS company_name
		FROM job_sync_runs r
		LEFT JOIN job_companies c ON c.id = r.company_id
		WHERE %s
		ORDER BY r.started_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, placeholder, placeholder+1)

	rows, err := m.db.Query(dataQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var runs []JobSyncRunWithCompany
	for rows.Next() {
		var r JobSyncRunWithCompany
		var finishedAt sql.NullTime
		var message sql.NullString
		if err := rows.Scan(
			&r.ID, &r.CompanyID, &r.StartedAt, &finishedAt, &r.Status, &message,
			&r.JobsFound, &r.JobsCreated, &r.JobsUpdated, &r.JobsClosed,
			&r.CompanyName,
		); err != nil {
			return nil, 0, err
		}
		if finishedAt.Valid {
			r.FinishedAt = &finishedAt.Time
		}
		if message.Valid {
			r.Message = &message.String
		}
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return runs, total, nil
}

// Helper functions ---------------------------------------------------------

func nullString(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return trimmed
}

func nullStringPtr(value *string) interface{} {
	if value == nil {
		return nil
	}
	return nullString(*value)
}

func pqArray(values []string) interface{} {
	if len(values) == 0 {
		return pq.StringArray{}
	}
	return pq.StringArray(values)
}
