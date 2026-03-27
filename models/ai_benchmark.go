package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

// AiBenchmarkResult represents a single evaluation result from a benchmark run.
type AiBenchmarkResult struct {
	ID            int64      `json:"id"`
	RunID         string     `json:"run_id"`
	BenchmarkType string     `json:"benchmark_type"`
	EntityID      *int64     `json:"entity_id,omitempty"`
	FieldName     string     `json:"field_name"`
	AiValue       string     `json:"ai_value,omitempty"`
	ExpectedValue string     `json:"expected_value,omitempty"`
	IsCorrect     *bool      `json:"is_correct,omitempty"`
	Score         *float64   `json:"score,omitempty"`
	Reasoning     string     `json:"reasoning,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// BenchmarkRunSummary holds aggregated results for a single benchmark run.
type BenchmarkRunSummary struct {
	RunID         string             `json:"run_id"`
	BenchmarkType string             `json:"benchmark_type"`
	SampleSize    int                `json:"sample_size"`
	Accuracy      map[string]float64 `json:"accuracy,omitempty"`
	AverageScores map[string]float64 `json:"average_scores,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
}

// BenchmarkSystemSummary holds the latest score for one benchmark type.
type BenchmarkSystemSummary struct {
	BenchmarkType string   `json:"benchmark_type"`
	LastRunID     string   `json:"last_run_id"`
	LastRunAt     time.Time `json:"last_run_at"`
	SampleSize    int      `json:"sample_size"`
	OverallScore  float64  `json:"overall_score"`
}

// AiBenchmarkModel handles persistence for benchmark results.
type AiBenchmarkModel struct {
	db *sql.DB
}

// NewAiBenchmarkModel creates a new AiBenchmarkModel.
func NewAiBenchmarkModel(db *sql.DB) *AiBenchmarkModel {
	return &AiBenchmarkModel{db: db}
}

// InsertResult stores a single benchmark evaluation result.
func (m *AiBenchmarkModel) InsertResult(r AiBenchmarkResult) error {
	if m == nil || m.db == nil {
		return errors.New("AiBenchmarkModel is not initialised")
	}
	_, err := m.db.Exec(`
		INSERT INTO ai_benchmark_results (run_id, benchmark_type, entity_id, field_name, ai_value, expected_value, is_correct, score, reasoning)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, r.RunID, r.BenchmarkType, r.EntityID, r.FieldName, r.AiValue, r.ExpectedValue, r.IsCorrect, r.Score, r.Reasoning)
	return err
}

// InsertResults stores multiple results in a batch.
func (m *AiBenchmarkModel) InsertResults(results []AiBenchmarkResult) error {
	if m == nil || m.db == nil {
		return errors.New("AiBenchmarkModel is not initialised")
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

	stmt, err := tx.Prepare(pq.CopyIn("ai_benchmark_results",
		"run_id", "benchmark_type", "entity_id", "field_name", "ai_value", "expected_value", "is_correct", "score", "reasoning"))
	if err != nil {
		// Fallback to individual inserts if COPY not available
		for _, r := range results {
			if _, err := tx.Exec(`
				INSERT INTO ai_benchmark_results (run_id, benchmark_type, entity_id, field_name, ai_value, expected_value, is_correct, score, reasoning)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			`, r.RunID, r.BenchmarkType, r.EntityID, r.FieldName, r.AiValue, r.ExpectedValue, r.IsCorrect, r.Score, r.Reasoning); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
	defer stmt.Close()

	for _, r := range results {
		if _, err := stmt.Exec(r.RunID, r.BenchmarkType, r.EntityID, r.FieldName, r.AiValue, r.ExpectedValue, r.IsCorrect, r.Score, r.Reasoning); err != nil {
			return err
		}
	}
	if _, err := stmt.Exec(); err != nil {
		return err
	}
	return tx.Commit()
}

// GetResultsByRunID returns all results for a specific run.
func (m *AiBenchmarkModel) GetResultsByRunID(runID string) ([]AiBenchmarkResult, error) {
	rows, err := m.db.Query(`
		SELECT id, run_id, benchmark_type, entity_id, field_name, ai_value, expected_value, is_correct, score, reasoning, created_at
		FROM ai_benchmark_results WHERE run_id = $1 ORDER BY id
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBenchmarkResults(rows)
}

// GetLatestResultsByType returns results from the most recent run of a given type.
func (m *AiBenchmarkModel) GetLatestResultsByType(benchmarkType string) ([]AiBenchmarkResult, error) {
	// Find latest run_id for this type
	var runID string
	err := m.db.QueryRow(`
		SELECT run_id FROM ai_benchmark_results WHERE benchmark_type = $1
		ORDER BY created_at DESC LIMIT 1
	`, benchmarkType).Scan(&runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return m.GetResultsByRunID(runID)
}

// GetHistory returns summary of all benchmark runs.
func (m *AiBenchmarkModel) GetHistory(limit int) ([]BenchmarkRunSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := m.db.Query(`
		SELECT run_id, benchmark_type,
		       COUNT(*) AS sample_size,
		       AVG(CASE WHEN is_correct = TRUE THEN 1.0 WHEN is_correct = FALSE THEN 0.0 END) AS avg_accuracy,
		       AVG(score) AS avg_score,
		       MIN(created_at) AS created_at
		FROM ai_benchmark_results
		GROUP BY run_id, benchmark_type
		ORDER BY MIN(created_at) DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BenchmarkRunSummary
	for rows.Next() {
		var s BenchmarkRunSummary
		var avgAccuracy, avgScore sql.NullFloat64
		if err := rows.Scan(&s.RunID, &s.BenchmarkType, &s.SampleSize, &avgAccuracy, &avgScore, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.Accuracy = map[string]float64{}
		s.AverageScores = map[string]float64{}
		if avgAccuracy.Valid {
			s.Accuracy["overall"] = avgAccuracy.Float64
		}
		if avgScore.Valid {
			s.AverageScores["overall"] = avgScore.Float64
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// GetSummary returns the latest score for each benchmark type.
func (m *AiBenchmarkModel) GetSummary() ([]BenchmarkSystemSummary, error) {
	rows, err := m.db.Query(`
		WITH latest_runs AS (
			SELECT DISTINCT ON (benchmark_type)
			       benchmark_type, run_id, created_at
			FROM ai_benchmark_results
			ORDER BY benchmark_type, created_at DESC
		)
		SELECT lr.benchmark_type, lr.run_id, lr.created_at,
		       COUNT(r.id) AS sample_size,
		       COALESCE(AVG(CASE WHEN r.is_correct = TRUE THEN 1.0 WHEN r.is_correct = FALSE THEN 0.0 END), AVG(r.score) / 5.0) AS overall_score
		FROM latest_runs lr
		JOIN ai_benchmark_results r ON r.run_id = lr.run_id AND r.benchmark_type = lr.benchmark_type
		GROUP BY lr.benchmark_type, lr.run_id, lr.created_at
		ORDER BY lr.benchmark_type
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BenchmarkSystemSummary
	for rows.Next() {
		var s BenchmarkSystemSummary
		if err := rows.Scan(&s.BenchmarkType, &s.LastRunID, &s.LastRunAt, &s.SampleSize, &s.OverallScore); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// HasResultsFromThisWeek returns true if any benchmark ran this week.
func (m *AiBenchmarkModel) HasResultsFromThisWeek() bool {
	var count int
	err := m.db.QueryRow(`
		SELECT COUNT(*) FROM ai_benchmark_results
		WHERE created_at >= DATE_TRUNC('week', CURRENT_TIMESTAMP)
	`).Scan(&count)
	return err == nil && count > 0
}

func scanBenchmarkResults(rows *sql.Rows) ([]AiBenchmarkResult, error) {
	var results []AiBenchmarkResult
	for rows.Next() {
		var r AiBenchmarkResult
		var entityID sql.NullInt64
		var isCorrect sql.NullBool
		var score sql.NullFloat64
		if err := rows.Scan(&r.ID, &r.RunID, &r.BenchmarkType, &entityID, &r.FieldName,
			&r.AiValue, &r.ExpectedValue, &isCorrect, &score, &r.Reasoning, &r.CreatedAt); err != nil {
			return nil, err
		}
		if entityID.Valid {
			r.EntityID = &entityID.Int64
		}
		if isCorrect.Valid {
			r.IsCorrect = &isCorrect.Bool
		}
		if score.Valid {
			r.Score = &score.Float64
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
