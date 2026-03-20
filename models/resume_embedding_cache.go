package models

import (
	"database/sql"
	"errors"

	"github.com/pgvector/pgvector-go"
)

// ResumeEmbeddingCacheModel persists resume embeddings keyed by resume hash.
type ResumeEmbeddingCacheModel struct {
	db *sql.DB
}

// NewResumeEmbeddingCacheModel creates a new ResumeEmbeddingCacheModel.
func NewResumeEmbeddingCacheModel(db *sql.DB) *ResumeEmbeddingCacheModel {
	return &ResumeEmbeddingCacheModel{db: db}
}

// UpsertEmbedding stores or updates a resume embedding.
func (m *ResumeEmbeddingCacheModel) UpsertEmbedding(resumeHash string, embedding []float32) error {
	if m == nil || m.db == nil {
		return errors.New("ResumeEmbeddingCacheModel is not initialised")
	}
	_, err := m.db.Exec(`
		INSERT INTO resume_embedding_cache (resume_hash, embedding)
		VALUES ($1, $2::halfvec)
		ON CONFLICT (resume_hash)
		DO UPDATE SET embedding = EXCLUDED.embedding, created_at = CURRENT_TIMESTAMP
	`, resumeHash, pgvector.NewVector(embedding))
	return err
}

// GetEmbedding retrieves a cached resume embedding. Returns nil if not found.
func (m *ResumeEmbeddingCacheModel) GetEmbedding(resumeHash string) ([]float32, error) {
	if m == nil || m.db == nil {
		return nil, errors.New("ResumeEmbeddingCacheModel is not initialised")
	}
	var vec pgvector.Vector
	err := m.db.QueryRow(`
		SELECT embedding::vector FROM resume_embedding_cache WHERE resume_hash = $1
	`, resumeHash).Scan(&vec)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return vec.Slice(), nil
}
