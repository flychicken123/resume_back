package models

import (
	"database/sql"
	"errors"

	"github.com/lib/pq"
)

// ResumeSkillCacheModel persists LLM-inferred skills keyed by resume hash.
type ResumeSkillCacheModel struct {
	db *sql.DB
}

// NewResumeSkillCacheModel creates a new ResumeSkillCacheModel.
func NewResumeSkillCacheModel(db *sql.DB) *ResumeSkillCacheModel {
	return &ResumeSkillCacheModel{db: db}
}

// UpsertSkills stores or updates inferred skills for a resume hash.
func (m *ResumeSkillCacheModel) UpsertSkills(resumeHash string, skills []string) error {
	if m == nil || m.db == nil {
		return errors.New("ResumeSkillCacheModel is not initialised")
	}
	_, err := m.db.Exec(`
		INSERT INTO resume_skill_cache (resume_hash, skills)
		VALUES ($1, $2)
		ON CONFLICT (resume_hash)
		DO UPDATE SET skills = EXCLUDED.skills, updated_at = CURRENT_TIMESTAMP
	`, resumeHash, pq.StringArray(skills))
	return err
}

// GetSkills retrieves cached skills for a resume hash. Returns nil if not found.
func (m *ResumeSkillCacheModel) GetSkills(resumeHash string) ([]string, error) {
	if m == nil || m.db == nil {
		return nil, errors.New("ResumeSkillCacheModel is not initialised")
	}
	var skills pq.StringArray
	err := m.db.QueryRow(`
		SELECT skills FROM resume_skill_cache WHERE resume_hash = $1
	`, resumeHash).Scan(&skills)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return []string(skills), nil
}
