package models

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
)

// JobApplication represents a tracked job application.
type JobApplication struct {
	ID               int64      `json:"id"`
	UserID           int        `json:"user_id"`
	JobPostingID     *int64     `json:"job_posting_id"`
	ResumeJobMatchID *int64     `json:"resume_job_match_id"`
	JobTitle         string     `json:"job_title"`
	CompanyName      string     `json:"company_name"`
	JobURL           string     `json:"job_url"`
	JobLocation      string     `json:"job_location"`
	Status           string     `json:"status"`
	AppliedAt        time.Time  `json:"applied_at"`
	ResumeHash       string     `json:"resume_hash"`
	CoverLetterUsed  bool       `json:"cover_letter_used"`
	Notes            string     `json:"notes"`
	SalaryOffered    *float64   `json:"salary_offered"`
	SalaryCurrency   string     `json:"salary_currency"`
	StatusUpdatedAt  time.Time  `json:"status_updated_at"`
	RemindersEnabled bool       `json:"reminders_enabled"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// JobApplicationStatusHistory records a status transition.
type JobApplicationStatusHistory struct {
	ID            int64     `json:"id"`
	ApplicationID int64     `json:"application_id"`
	FromStatus    *string   `json:"from_status"`
	ToStatus      string    `json:"to_status"`
	ChangedAt     time.Time `json:"changed_at"`
	Note          string    `json:"note"`
}

// JobApplicationModel handles persistence for job_applications.
type JobApplicationModel struct {
	db *sql.DB
}

// NewJobApplicationModel constructs a JobApplicationModel.
func NewJobApplicationModel(db *sql.DB) *JobApplicationModel {
	return &JobApplicationModel{db: db}
}

// ---------------------------------------------------------------------------
// Status lifecycle
// ---------------------------------------------------------------------------

var validTransitions = map[string][]string{
	"applied":      {"screening", "interviewing", "offered", "rejected", "withdrawn"},
	"screening":    {"applied", "interviewing", "offered", "rejected", "withdrawn"},
	"interviewing": {"applied", "screening", "offered", "rejected", "withdrawn"},
	"offered":      {"applied", "screening", "interviewing", "accepted", "rejected", "withdrawn"},
	"accepted":     {"applied", "screening", "interviewing", "offered", "withdrawn"},
	// rejected and withdrawn are terminal
}

// IsValidStatusTransition checks whether moving from → to is allowed.
func IsValidStatusTransition(from, to string) bool {
	if from == "" || to == "" || from == to {
		return false
	}
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	return slices.Contains(allowed, to)
}

var statusOrder = map[string]int{
	"applied":      0,
	"screening":    1,
	"interviewing": 2,
	"offered":      3,
	"accepted":     4,
	"rejected":     5,
	"withdrawn":    6,
}

// isBackwardTransition returns true if moving to an earlier stage.
func isBackwardTransition(from, to string) bool {
	fromIdx, ok1 := statusOrder[from]
	toIdx, ok2 := statusOrder[to]
	return ok1 && ok2 && toIdx < fromIdx
}

// GetAllowedTransitions returns the list of statuses reachable from the given status.
func GetAllowedTransitions(from string) []string {
	if t, ok := validTransitions[from]; ok {
		return t
	}
	return []string{}
}

// ---------------------------------------------------------------------------
// Scanner helper
// ---------------------------------------------------------------------------

func scanJobApplication(scanner rowScanner) (*JobApplication, error) {
	var a JobApplication
	var jobPostingID sql.NullInt64
	var matchID sql.NullInt64
	var resumeHash sql.NullString
	var notes sql.NullString
	var salaryOffered sql.NullFloat64
	var salaryCurrency sql.NullString

	if err := scanner.Scan(
		&a.ID,
		&a.UserID,
		&jobPostingID,
		&matchID,
		&a.JobTitle,
		&a.CompanyName,
		&a.JobURL,
		&a.JobLocation,
		&a.Status,
		&a.AppliedAt,
		&resumeHash,
		&a.CoverLetterUsed,
		&notes,
		&salaryOffered,
		&salaryCurrency,
		&a.StatusUpdatedAt,
		&a.RemindersEnabled,
		&a.CreatedAt,
		&a.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if jobPostingID.Valid {
		v := jobPostingID.Int64
		a.JobPostingID = &v
	}
	if matchID.Valid {
		v := matchID.Int64
		a.ResumeJobMatchID = &v
	}
	a.ResumeHash = resumeHash.String
	a.Notes = notes.String
	if salaryOffered.Valid {
		v := salaryOffered.Float64
		a.SalaryOffered = &v
	}
	a.SalaryCurrency = salaryCurrency.String

	return &a, nil
}

const jobAppColumns = `id, user_id, job_posting_id, resume_job_match_id,
	job_title, company_name, job_url, job_location,
	status, applied_at, resume_hash, cover_letter_used,
	notes, salary_offered, salary_currency,
	status_updated_at, COALESCE(reminders_enabled, true), created_at, updated_at`

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// Create inserts a new job application and records the initial status history.
func (m *JobApplicationModel) Create(app *JobApplication) error {
	if m == nil || m.db == nil {
		return errors.New("JobApplicationModel is not initialised")
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	err = tx.QueryRow(`
		INSERT INTO job_applications (
			user_id, job_posting_id, resume_job_match_id,
			job_title, company_name, job_url, job_location,
			status, resume_hash, cover_letter_used, notes
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, applied_at, status_updated_at, COALESCE(reminders_enabled, true), created_at, updated_at`,
		app.UserID, app.JobPostingID, app.ResumeJobMatchID,
		app.JobTitle, app.CompanyName, app.JobURL, app.JobLocation,
		app.Status, app.ResumeHash, app.CoverLetterUsed, app.Notes,
	).Scan(&app.ID, &app.AppliedAt, &app.StatusUpdatedAt, &app.RemindersEnabled, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert application: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO job_application_status_history (application_id, from_status, to_status)
		VALUES ($1, NULL, $2)`,
		app.ID, app.Status,
	)
	if err != nil {
		return fmt.Errorf("insert initial status history: %w", err)
	}

	return tx.Commit()
}

// GetByID returns a single application scoped to the given user.
func (m *JobApplicationModel) GetByID(userID int, appID int64) (*JobApplication, error) {
	if m == nil || m.db == nil {
		return nil, errors.New("JobApplicationModel is not initialised")
	}

	row := m.db.QueryRow(
		fmt.Sprintf("SELECT %s FROM job_applications WHERE id = $1 AND user_id = $2", jobAppColumns),
		appID, userID,
	)
	return scanJobApplication(row)
}

// ListByUser returns paginated applications with an optional status filter.
func (m *JobApplicationModel) ListByUser(userID, limit, offset int, statusFilter string) ([]*JobApplication, int, error) {
	if m == nil || m.db == nil {
		return nil, 0, errors.New("JobApplicationModel is not initialised")
	}
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	var countQuery string
	var listQuery string
	args := []any{userID}

	if statusFilter != "" {
		countQuery = "SELECT COUNT(*) FROM job_applications WHERE user_id = $1 AND status = $2"
		listQuery = fmt.Sprintf(
			"SELECT %s FROM job_applications WHERE user_id = $1 AND status = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4",
			jobAppColumns,
		)
		args = append(args, statusFilter)
	} else {
		countQuery = "SELECT COUNT(*) FROM job_applications WHERE user_id = $1"
		listQuery = fmt.Sprintf(
			"SELECT %s FROM job_applications WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
			jobAppColumns,
		)
	}

	if err := m.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count applications: %w", err)
	}

	args = append(args, limit, offset)
	rows, err := m.db.Query(listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()

	var apps []*JobApplication
	for rows.Next() {
		app, err := scanJobApplication(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan application: %w", err)
		}
		apps = append(apps, app)
	}
	return apps, total, rows.Err()
}

// UpdateStatus transitions an application to a new status within a transaction.
func (m *JobApplicationModel) UpdateStatus(userID int, appID int64, newStatus, note string) error {
	if m == nil || m.db == nil {
		return errors.New("JobApplicationModel is not initialised")
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentStatus string
	err = tx.QueryRow(
		"SELECT status FROM job_applications WHERE id = $1 AND user_id = $2",
		appID, userID,
	).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("fetch current status: %w", err)
	}

	if !IsValidStatusTransition(currentStatus, newStatus) {
		return &InvalidTransitionError{
			CurrentStatus:  currentStatus,
			RequestedStatus: newStatus,
			Allowed:        GetAllowedTransitions(currentStatus),
		}
	}

	if isBackwardTransition(currentStatus, newStatus) {
		// Undo: find when the target status was last set, restore that timestamp,
		// and delete all history entries that came after it.
		var restoredAt time.Time
		err = tx.QueryRow(`
			SELECT changed_at FROM job_application_status_history
			WHERE application_id = $1 AND to_status = $2
			ORDER BY changed_at DESC LIMIT 1`,
			appID, newStatus,
		).Scan(&restoredAt)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Target status was the initial state; use applied_at
				err = tx.QueryRow(
					"SELECT applied_at FROM job_applications WHERE id = $1",
					appID,
				).Scan(&restoredAt)
				if err != nil {
					return fmt.Errorf("fetch applied_at: %w", err)
				}
			} else {
				return fmt.Errorf("fetch target history entry: %w", err)
			}
		}

		// Delete history entries after the restored point
		_, err = tx.Exec(`
			DELETE FROM job_application_status_history
			WHERE application_id = $1 AND changed_at > $2`,
			appID, restoredAt,
		)
		if err != nil {
			return fmt.Errorf("delete forward history: %w", err)
		}

		// Restore the status and its original timestamp
		_, err = tx.Exec(`
			UPDATE job_applications
			SET status = $1, status_updated_at = $2, updated_at = CURRENT_TIMESTAMP
			WHERE id = $3 AND user_id = $4`,
			newStatus, restoredAt, appID, userID,
		)
		if err != nil {
			return fmt.Errorf("restore status: %w", err)
		}
	} else {
		// Forward transition: record new history entry
		_, err = tx.Exec(`
			UPDATE job_applications
			SET status = $1, status_updated_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE id = $2 AND user_id = $3`,
			newStatus, appID, userID,
		)
		if err != nil {
			return fmt.Errorf("update status: %w", err)
		}

		_, err = tx.Exec(`
			INSERT INTO job_application_status_history (application_id, from_status, to_status, note)
			VALUES ($1, $2, $3, $4)`,
			appID, currentStatus, newStatus, note,
		)
		if err != nil {
			return fmt.Errorf("insert status history: %w", err)
		}
	}

	return tx.Commit()
}

// InvalidTransitionError is returned when a status transition is not allowed.
type InvalidTransitionError struct {
	CurrentStatus   string
	RequestedStatus string
	Allowed         []string
}

func (e *InvalidTransitionError) Error() string {
	return fmt.Sprintf("invalid transition from %q to %q", e.CurrentStatus, e.RequestedStatus)
}

// Delete removes a job application.
func (m *JobApplicationModel) Delete(userID int, appID int64) error {
	if m == nil || m.db == nil {
		return errors.New("JobApplicationModel is not initialised")
	}
	result, err := m.db.Exec(
		"DELETE FROM job_applications WHERE id = $1 AND user_id = $2",
		appID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete application: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetStatusHistory returns the status transition timeline for an application.
func (m *JobApplicationModel) GetStatusHistory(appID int64) ([]*JobApplicationStatusHistory, error) {
	if m == nil || m.db == nil {
		return nil, errors.New("JobApplicationModel is not initialised")
	}

	rows, err := m.db.Query(`
		SELECT id, application_id, from_status, to_status, changed_at, note
		FROM job_application_status_history
		WHERE application_id = $1
		ORDER BY changed_at ASC`,
		appID,
	)
	if err != nil {
		return nil, fmt.Errorf("query status history: %w", err)
	}
	defer rows.Close()

	var history []*JobApplicationStatusHistory
	for rows.Next() {
		var h JobApplicationStatusHistory
		var fromStatus sql.NullString
		var note sql.NullString
		if err := rows.Scan(&h.ID, &h.ApplicationID, &fromStatus, &h.ToStatus, &h.ChangedAt, &note); err != nil {
			return nil, fmt.Errorf("scan status history: %w", err)
		}
		if fromStatus.Valid {
			h.FromStatus = &fromStatus.String
		}
		h.Note = note.String
		history = append(history, &h)
	}
	return history, rows.Err()
}

// GetStatsByUser returns application counts grouped by status.
func (m *JobApplicationModel) GetStatsByUser(userID int) (map[string]int, int, int, int, error) {
	if m == nil || m.db == nil {
		return nil, 0, 0, 0, errors.New("JobApplicationModel is not initialised")
	}

	// Counts by status
	rows, err := m.db.Query(
		"SELECT status, COUNT(*) FROM job_applications WHERE user_id = $1 GROUP BY status",
		userID,
	)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("query stats: %w", err)
	}
	defer rows.Close()

	byStatus := make(map[string]int)
	total := 0
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, 0, 0, 0, fmt.Errorf("scan stats: %w", err)
		}
		byStatus[status] = count
		total += count
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, 0, err
	}

	// This week count
	var thisWeek int
	_ = m.db.QueryRow(
		"SELECT COUNT(*) FROM job_applications WHERE user_id = $1 AND created_at >= date_trunc('week', CURRENT_DATE)",
		userID,
	).Scan(&thisWeek)

	// This month count
	var thisMonth int
	_ = m.db.QueryRow(
		"SELECT COUNT(*) FROM job_applications WHERE user_id = $1 AND created_at >= date_trunc('month', CURRENT_DATE)",
		userID,
	).Scan(&thisMonth)

	return byStatus, total, thisWeek, thisMonth, nil
}

// SetRemindersEnabled updates the reminders_enabled flag for a specific application.
func (m *JobApplicationModel) SetRemindersEnabled(userID int, appID int64, enabled bool) error {
	if m == nil || m.db == nil {
		return errors.New("JobApplicationModel is not initialised")
	}
	result, err := m.db.Exec(
		`UPDATE job_applications SET reminders_enabled = $1, updated_at = CURRENT_TIMESTAMP
		 WHERE id = $2 AND user_id = $3`,
		enabled, appID, userID,
	)
	if err != nil {
		return fmt.Errorf("update reminders_enabled: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// FindStaleApplications returns applications that haven't been updated in staleDays days,
// excluding terminal statuses and apps with reminders disabled.
func (m *JobApplicationModel) FindStaleApplications(userID int, staleDays int) ([]*JobApplication, error) {
	if m == nil || m.db == nil {
		return nil, errors.New("JobApplicationModel is not initialised")
	}

	query := fmt.Sprintf(
		`SELECT %s FROM job_applications
		 WHERE user_id = $1
		   AND status NOT IN ('rejected', 'withdrawn', 'accepted')
		   AND COALESCE(reminders_enabled, true) = true
		   AND status_updated_at < NOW() - ($2 * interval '1 day')
		 ORDER BY status_updated_at ASC`, jobAppColumns)

	rows, err := m.db.Query(query, userID, staleDays)
	if err != nil {
		return nil, fmt.Errorf("find stale applications: %w", err)
	}
	defer rows.Close()

	var apps []*JobApplication
	for rows.Next() {
		app, err := scanJobApplication(rows)
		if err != nil {
			return nil, fmt.Errorf("scan stale application: %w", err)
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}
