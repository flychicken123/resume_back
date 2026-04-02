package models

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
)

type KnowledgeEntry struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Category  string    `json:"category"`
	Content   string    `json:"content"`
	IsActive  bool      `json:"is_active"`
	HasEmbed  bool      `json:"has_embedding"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type KnowledgeBaseModel struct {
	db *sql.DB
}

func NewKnowledgeBaseModel(db *sql.DB) *KnowledgeBaseModel {
	return &KnowledgeBaseModel{db: db}
}

func (m *KnowledgeBaseModel) Insert(title, category, content string) (int, error) {
	var id int
	err := m.db.QueryRow(
		`INSERT INTO knowledge_base (title, category, content) VALUES ($1, $2, $3) RETURNING id`,
		title, category, content,
	).Scan(&id)
	return id, err
}

func (m *KnowledgeBaseModel) Update(id int, title, category, content string) error {
	_, err := m.db.Exec(
		`UPDATE knowledge_base SET title=$1, category=$2, content=$3, embedding=NULL, updated_at=CURRENT_TIMESTAMP WHERE id=$4`,
		title, category, content, id,
	)
	return err
}

func (m *KnowledgeBaseModel) Delete(id int) error {
	_, err := m.db.Exec(`UPDATE knowledge_base SET is_active=FALSE WHERE id=$1`, id)
	return err
}

func (m *KnowledgeBaseModel) UpdateEmbedding(id int, embedding []float32) error {
	_, err := m.db.Exec(
		`UPDATE knowledge_base SET embedding=$1 WHERE id=$2`,
		pgvector.NewVector(embedding), id,
	)
	return err
}

func (m *KnowledgeBaseModel) SearchByVector(embedding []float32, limit int, threshold float32) ([]KnowledgeEntry, error) {
	if limit <= 0 {
		limit = 4
	}
	rows, err := m.db.Query(`
		SELECT id, title, category, content
		FROM knowledge_base
		WHERE is_active = TRUE AND embedding IS NOT NULL
		  AND 1 - (embedding <=> $1::halfvec) >= $3
		ORDER BY embedding <=> $1::halfvec
		LIMIT $2`,
		pgvector.NewVector(embedding), limit, threshold,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []KnowledgeEntry
	for rows.Next() {
		var e KnowledgeEntry
		if err := rows.Scan(&e.ID, &e.Title, &e.Category, &e.Content); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (m *KnowledgeBaseModel) ListAll() ([]KnowledgeEntry, error) {
	rows, err := m.db.Query(`
		SELECT id, title, category, content, is_active, embedding IS NOT NULL, created_at, updated_at
		FROM knowledge_base ORDER BY category, title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []KnowledgeEntry
	for rows.Next() {
		var e KnowledgeEntry
		if err := rows.Scan(&e.ID, &e.Title, &e.Category, &e.Content, &e.IsActive, &e.HasEmbed, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (m *KnowledgeBaseModel) ListWithNullEmbedding() ([]KnowledgeEntry, error) {
	rows, err := m.db.Query(`
		SELECT id, title, category, content
		FROM knowledge_base WHERE is_active = TRUE AND embedding IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []KnowledgeEntry
	for rows.Next() {
		var e KnowledgeEntry
		if err := rows.Scan(&e.ID, &e.Title, &e.Category, &e.Content); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (m *KnowledgeBaseModel) Count() (int, error) {
	var count int
	err := m.db.QueryRow(`SELECT COUNT(*) FROM knowledge_base WHERE is_active = TRUE`).Scan(&count)
	return count, err
}

// Ensure pq is used (for future array operations)
var _ = pq.Array
