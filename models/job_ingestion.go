package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
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
	var lastSynced sql.NullTime

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
	if lastSynced.Valid {
		ts := lastSynced.Time
		c.LastSyncedAt = &ts
	}

	return &c, nil
}

// ListActive returns all active companies ordered by sync cadence
func (m *JobCompanyModel) ListActive() ([]*JobCompany, error) {
	const query = `
        SELECT id, name, website_url, careers_url, ats_provider,
               external_identifier, is_active, sync_interval_minutes,
               last_synced_at, last_sync_status, created_at, updated_at
        FROM job_companies
        WHERE is_active = TRUE
        ORDER BY sync_interval_minutes, id
    `

	return m.fetchCompanies(query)
}

// ListAll returns all companies regardless of active flag
func (m *JobCompanyModel) ListAll() ([]*JobCompany, error) {
	const query = `
        SELECT id, name, website_url, careers_url, ats_provider,
               external_identifier, is_active, sync_interval_minutes,
               last_synced_at, last_sync_status, created_at, updated_at
        FROM job_companies
        ORDER BY id
    `

	return m.fetchCompanies(query)
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
               last_synced_at, last_sync_status, created_at, updated_at
        FROM job_companies
        WHERE id = $1
    `

	company, err := scanJobCompanyRow(m.db.QueryRow(query, id))
	if err != nil {
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
		tx.Rollback()
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
			tx.Rollback()
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
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `
	_, err := m.db.Exec(query, companyID, syncedAt, status)
	return err
}

// JobPostingModel manages job_postings records
type JobPostingModel struct {
	db *sql.DB
}

func NewJobPostingModel(db *sql.DB) *JobPostingModel {
	return &JobPostingModel{db: db}
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
            closed_at = NULL
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
