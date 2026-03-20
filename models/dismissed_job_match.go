package models

import (
	"database/sql"
	"errors"
)

// ValidDismissReasons lists the accepted values for dismiss_reason.
var ValidDismissReasons = map[string]bool{
	"":                       true,
	"wrong_role":             true,
	"too_senior_junior":      true,
	"bad_location":           true,
	"low_pay":                true,
	"not_interested_company": true,
}

// IsValidDismissReason checks whether a reason string is allowed.
func IsValidDismissReason(reason string) bool {
	return ValidDismissReasons[reason]
}

// DismissedJobMatchModel handles persistence of user job dismissals.
type DismissedJobMatchModel struct {
	db *sql.DB
}

// NewDismissedJobMatchModel creates a new DismissedJobMatchModel.
func NewDismissedJobMatchModel(db *sql.DB) *DismissedJobMatchModel {
	return &DismissedJobMatchModel{db: db}
}

// Dismiss records (or updates) a user's dismissal of a job posting.
func (m *DismissedJobMatchModel) Dismiss(userID int, jobPostingID int64, reason string) error {
	if m == nil || m.db == nil {
		return errors.New("DismissedJobMatchModel is not initialised")
	}
	_, err := m.db.Exec(`
		INSERT INTO dismissed_job_matches (user_id, job_posting_id, dismiss_reason)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, job_posting_id)
		DO UPDATE SET dismiss_reason = EXCLUDED.dismiss_reason,
		              dismissed_at = CURRENT_TIMESTAMP
	`, userID, jobPostingID, reason)
	return err
}

// Undismiss removes a user's dismissal of a job posting.
func (m *DismissedJobMatchModel) Undismiss(userID int, jobPostingID int64) error {
	if m == nil || m.db == nil {
		return errors.New("DismissedJobMatchModel is not initialised")
	}
	_, err := m.db.Exec(`
		DELETE FROM dismissed_job_matches
		WHERE user_id = $1 AND job_posting_id = $2
	`, userID, jobPostingID)
	return err
}

// ListDismissedIDs returns all dismissed job posting IDs for a user.
func (m *DismissedJobMatchModel) ListDismissedIDs(userID int) ([]int64, error) {
	if m == nil || m.db == nil {
		return nil, errors.New("DismissedJobMatchModel is not initialised")
	}
	rows, err := m.db.Query(`
		SELECT job_posting_id FROM dismissed_job_matches
		WHERE user_id = $1
		ORDER BY dismissed_at DESC
	`, userID)
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
