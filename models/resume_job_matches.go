package models

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"resumeai/utils"
)

// ResumeJobMatchRecord represents a stored match enriched with job metadata.
type ResumeJobMatchRecord struct {
	ID             int64     `json:"id"`
	UserID         int       `json:"user_id"`
	ResumeHash     string    `json:"resume_hash"`
	JobPostingID   int64     `json:"job_posting_id"`
	MatchScore     float64   `json:"match_score"`
	MatchedAt      time.Time `json:"matched_at"`
	JobTitle       string    `json:"job_title"`
	JobLocation    string    `json:"job_location"`
	JobRemoteType  string    `json:"job_remote_type"`
	JobDepartment  string    `json:"job_department"`
	JobEmployment  string    `json:"job_employment_type"`
	JobURL         string    `json:"job_url"`
	JobDescription string    `json:"job_description"`
	CompanyID      *int      `json:"company_id,omitempty"`
	CompanyName    *string   `json:"company_name,omitempty"`

	// Skill gap analysis (computed at runtime, not stored in DB)
	RequiredSkills []string `json:"required_skills,omitempty"`
	MatchedSkills  []string `json:"matched_skills,omitempty"`
	MissingSkills  []string `json:"missing_skills,omitempty"`
}

// JobMatchCreate holds the data required to persist a match.
type JobMatchCreate struct {
	JobPostingID int64
	MatchScore   float64
}

// ResumeJobMatchModel handles persistence for resume job matches.
type ResumeJobMatchModel struct {
	db *sql.DB
}

// NewResumeJobMatchModel constructs a ResumeJobMatchModel.
func NewResumeJobMatchModel(db *sql.DB) *ResumeJobMatchModel {
	return &ResumeJobMatchModel{db: db}
}

// UpsertMatches replaces the stored matches for a user/resume combination.
func (m *ResumeJobMatchModel) UpsertMatches(userID int, resumeHash string, matches []JobMatchCreate) error {
	if m == nil || m.db == nil {
		return errors.New("ResumeJobMatchModel is not initialised")
	}

	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM resume_job_matches WHERE user_id = $1 AND resume_hash = $2`, userID, resumeHash); err != nil {
		return err
	}

	if len(matches) == 0 {
		err = tx.Commit()
		return err
	}

	stmt, err := tx.Prepare(`
        INSERT INTO resume_job_matches (user_id, resume_hash, job_posting_id, match_score)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (user_id, resume_hash, job_posting_id)
        DO UPDATE SET
            match_score = EXCLUDED.match_score,
            matched_at = CURRENT_TIMESTAMP
    `)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, match := range matches {
		if _, err = stmt.Exec(userID, resumeHash, match.JobPostingID, match.MatchScore); err != nil {
			return err
		}
	}

	err = tx.Commit()
	return err
}

// DeleteOlderThan removes matches with matched_at before the threshold.
func (m *ResumeJobMatchModel) DeleteOlderThan(threshold time.Time) (int64, error) {
	if m == nil || m.db == nil {
		return 0, errors.New("ResumeJobMatchModel is not initialised")
	}
	res, err := m.db.Exec(`DELETE FROM resume_job_matches WHERE matched_at < $1`, threshold)
	if err != nil {
		return 0, err
	}
	rows, _ := res.RowsAffected()
	return rows, nil
}

// ListByUserAndResume returns matches for the provided user and resume hash.
func (m *ResumeJobMatchModel) ListByUserAndResume(userID int, resumeHash string, limit int) ([]*ResumeJobMatchRecord, error) {
	if limit <= 0 {
		limit = 25
	}
	fetchLimit := limit * 3
	if fetchLimit < limit {
		fetchLimit = limit
	}

	query := `
        WITH dismiss_centroid AS (
            SELECT AVG(jp.embedding) AS centroid
            FROM dismissed_job_matches djm
            JOIN job_postings jp ON jp.id = djm.job_posting_id
            WHERE djm.user_id = $1 AND jp.embedding IS NOT NULL
            HAVING COUNT(*) >= 3
        )
        SELECT m.id, m.user_id, m.resume_hash, m.job_posting_id,
               CASE WHEN dc.centroid IS NOT NULL AND p.embedding IS NOT NULL
                    THEN GREATEST(0, m.match_score - GREATEST(0,
                        (1 - (p.embedding <=> dc.centroid) - 0.70) * 15))
                    ELSE m.match_score
               END AS match_score,
               m.matched_at,
               COALESCE(p.title, ''), COALESCE(p.location, ''), COALESCE(p.remote_type, ''),
               COALESCE(p.department, ''), COALESCE(p.employment_type, ''), p.job_url,
               COALESCE(p.description, ''), p.company_id, c.name
        FROM resume_job_matches m
        JOIN job_postings p ON p.id = m.job_posting_id
        LEFT JOIN job_companies c ON c.id = p.company_id
        LEFT JOIN dismiss_centroid dc ON TRUE
        WHERE m.user_id = $1 AND m.resume_hash = $2
          AND m.job_posting_id NOT IN (
              SELECT job_posting_id FROM dismissed_job_matches WHERE user_id = $1)
        ORDER BY match_score DESC, m.matched_at DESC
        LIMIT $3
    `

	rows, err := m.db.Query(query, userID, resumeHash, fetchLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*ResumeJobMatchRecord
	for rows.Next() {
		var record ResumeJobMatchRecord
		var companyID sql.NullInt64
		var companyName sql.NullString

		if err := rows.Scan(
			&record.ID,
			&record.UserID,
			&record.ResumeHash,
			&record.JobPostingID,
			&record.MatchScore,
			&record.MatchedAt,
			&record.JobTitle,
			&record.JobLocation,
			&record.JobRemoteType,
			&record.JobDepartment,
			&record.JobEmployment,
			&record.JobURL,
			&record.JobDescription,
			&companyID,
			&companyName,
		); err != nil {
			return nil, err
		}

		if companyID.Valid {
			cid := int(companyID.Int64)
			record.CompanyID = &cid
		}
		if companyName.Valid {
			name := companyName.String
			record.CompanyName = &name
		}

		if utils.JobLooksLikeInternRole(record.JobTitle, record.JobDescription) {
			continue
		}

		results = append(results, &record)
		if len(results) >= limit {
			break
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	// Limit to at most 3 jobs per company for diverse results.
	results = limitPerCompany(results, 3)

	return results, nil
}

// ListMostRecentByUser fetches matches for the user most recently generated resume.
func (m *ResumeJobMatchModel) ListMostRecentByUser(userID int, limit int) ([]*ResumeJobMatchRecord, string, error) {
	const latestResumeQuery = `
        SELECT resume_hash
        FROM resume_job_matches
        WHERE user_id = $1
        GROUP BY resume_hash
        ORDER BY MAX(matched_at) DESC
        LIMIT 1
    `

	var resumeHash string
	err := m.db.QueryRow(latestResumeQuery, userID).Scan(&resumeHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", nil
		}
		return nil, "", err
	}

	matches, err := m.ListByUserAndResume(userID, resumeHash, limit)
	return matches, resumeHash, err
}

// ListTopMatchesForUser returns the highest scoring matches across resumes.
func (m *ResumeJobMatchModel) ListTopMatchesForUser(userID int, limit int) ([]*ResumeJobMatchRecord, error) {
	if limit <= 0 {
		limit = 10
	}
	fetchLimit := limit * 3
	if fetchLimit < limit {
		fetchLimit = limit
	}

	query := `
        WITH dismiss_centroid AS (
            SELECT AVG(jp.embedding) AS centroid
            FROM dismissed_job_matches djm
            JOIN job_postings jp ON jp.id = djm.job_posting_id
            WHERE djm.user_id = $1 AND jp.embedding IS NOT NULL
            HAVING COUNT(*) >= 3
        )
        SELECT m.id, m.user_id, m.resume_hash, m.job_posting_id,
               CASE WHEN dc.centroid IS NOT NULL AND p.embedding IS NOT NULL
                    THEN GREATEST(0, m.match_score - GREATEST(0,
                        (1 - (p.embedding <=> dc.centroid) - 0.70) * 15))
                    ELSE m.match_score
               END AS match_score,
               m.matched_at,
               COALESCE(p.title, ''), COALESCE(p.location, ''), COALESCE(p.remote_type, ''),
               COALESCE(p.department, ''), COALESCE(p.employment_type, ''), p.job_url,
               COALESCE(p.description, ''), p.company_id, c.name
        FROM resume_job_matches m
        JOIN job_postings p ON p.id = m.job_posting_id
        LEFT JOIN job_companies c ON c.id = p.company_id
        LEFT JOIN dismiss_centroid dc ON TRUE
        WHERE m.user_id = $1
          AND m.job_posting_id NOT IN (
              SELECT job_posting_id FROM dismissed_job_matches WHERE user_id = $1)
        ORDER BY match_score DESC, m.matched_at DESC
        LIMIT $2
    `

	rows, err := m.db.Query(query, userID, fetchLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*ResumeJobMatchRecord
	for rows.Next() {
		var record ResumeJobMatchRecord
		var companyID sql.NullInt64
		var companyName sql.NullString

		if err := rows.Scan(
			&record.ID,
			&record.UserID,
			&record.ResumeHash,
			&record.JobPostingID,
			&record.MatchScore,
			&record.MatchedAt,
			&record.JobTitle,
			&record.JobLocation,
			&record.JobRemoteType,
			&record.JobDepartment,
			&record.JobEmployment,
			&record.JobURL,
			&record.JobDescription,
			&companyID,
			&companyName,
		); err != nil {
			return nil, err
		}

		if companyID.Valid {
			cid := int(companyID.Int64)
			record.CompanyID = &cid
		}
		if companyName.Valid {
			name := companyName.String
			record.CompanyName = &name
		}

		if utils.JobLooksLikeInternRole(record.JobTitle, record.JobDescription) {
			continue
		}

		results = append(results, &record)
		if len(results) >= limit {
			break
		}
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	// Limit to at most 3 jobs per company for diverse results.
	results = limitPerCompany(results, 3)

	return results, nil
}

// CountByUserAndResume returns how many matches are stored for a resume.
func (m *ResumeJobMatchModel) CountByUserAndResume(userID int, resumeHash string) (int, error) {
	var count int
	err := m.db.QueryRow(`
		SELECT COUNT(*) FROM resume_job_matches m
		WHERE m.user_id = $1 AND m.resume_hash = $2
		  AND m.job_posting_id NOT IN (
		      SELECT job_posting_id FROM dismissed_job_matches WHERE user_id = $1)
	`, userID, resumeHash).Scan(&count)
	return count, err
}

// DebugDump prints a quick representation of matches for troubleshooting (non production helper).
func (m *ResumeJobMatchModel) DebugDump(userID int, resumeHash string) error {
	matches, err := m.ListByUserAndResume(userID, resumeHash, 10)
	if err != nil {
		return err
	}
	for _, match := range matches {
		fmt.Printf("match %d user=%d resume=%s job=%d score=%.2f\n", match.ID, match.UserID, match.ResumeHash, match.JobPostingID, match.MatchScore)
	}
	return nil
}

// limitPerCompany keeps at most maxPerCompany jobs from the same company.
// Results are assumed to be sorted by score so the highest-scoring jobs are kept.
func limitPerCompany(records []*ResumeJobMatchRecord, maxPerCompany int) []*ResumeJobMatchRecord {
	companyCount := make(map[int]int)
	filtered := make([]*ResumeJobMatchRecord, 0, len(records))
	for _, r := range records {
		cid := 0
		if r.CompanyID != nil {
			cid = *r.CompanyID
		}
		if companyCount[cid] < maxPerCompany {
			filtered = append(filtered, r)
			companyCount[cid]++
		}
	}
	return filtered
}
