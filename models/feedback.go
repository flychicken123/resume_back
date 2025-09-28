package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/lib/pq"
)

type Feedback struct {
	ID              int64
	UserEmail       string
	Scenario        string
	Rating          sql.NullInt16
	Comment         sql.NullString
	PagePath        sql.NullString
	Step            sql.NullString
	Reason          sql.NullString
	SessionDuration sql.NullInt64
	PageDuration    sql.NullInt64
	LastStepDelta   sql.NullInt64
	Referrer        sql.NullString
	Metadata        map[string]interface{}
	CreatedAt       time.Time
}

type FeedbackFollowUp struct {
	ID          int64
	UserEmail   string
	TriggerKey  string
	ScheduledAt time.Time
	SentAt      sql.NullTime
	Status      string
	Metadata    map[string]interface{}
	LastError   sql.NullString
	CreatedAt   time.Time
}

type FeedbackModel struct {
	db *sql.DB
}

func NewFeedbackModel(db *sql.DB) *FeedbackModel {
	return &FeedbackModel{db: db}
}

func EnsureFeedbackSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS feedback (
            id SERIAL PRIMARY KEY,
            user_email VARCHAR(255),
            scenario VARCHAR(100) NOT NULL,
            rating SMALLINT,
            comment TEXT,
            page_path TEXT,
            page_title TEXT,
            step TEXT,
            reason TEXT,
            previous_page_path TEXT,
            session_duration_ms BIGINT,
            page_duration_ms BIGINT,
            last_step_delta_ms BIGINT,
            referrer TEXT,
            metadata JSONB,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE TABLE IF NOT EXISTS feedback_followups (
            id SERIAL PRIMARY KEY,
            user_email VARCHAR(255) NOT NULL,
            trigger_key VARCHAR(100) NOT NULL,
            scheduled_at TIMESTAMP NOT NULL,
            sent_at TIMESTAMP,
            status VARCHAR(50) DEFAULT 'pending',
            metadata JSONB,
            last_error TEXT,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_feedback_created_at ON feedback(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_feedback_followups_status ON feedback_followups(status, scheduled_at)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_feedback_followups_unique ON feedback_followups(user_email, trigger_key, scheduled_at)`,
		`ALTER TABLE feedback ADD COLUMN IF NOT EXISTS page_title TEXT`,
		`ALTER TABLE feedback ADD COLUMN IF NOT EXISTS previous_page_path TEXT`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}
func (m *FeedbackModel) CreateFeedback(ctx context.Context, data map[string]interface{}) error {
	metaBytes, _ := json.Marshal(data["metadata"])

	ratingValue := intOrNil(data["rating"])

	_, err := m.db.ExecContext(
		ctx,
		`INSERT INTO feedback (
            user_email, scenario, rating, comment, page_path, page_title, step, reason,
            previous_page_path, session_duration_ms, page_duration_ms, last_step_delta_ms, referrer, metadata
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		strOrNil(data["user_email"]),
		strOrNil(data["scenario"]),
		ratingValue,
		strOrNil(data["comment"]),
		strOrNil(data["page_path"]),
		strOrNil(data["page_title"]),
		strOrNil(data["step"]),
		strOrNil(data["reason"]),
		strOrNil(data["previous_page_path"]),
		intOrNil(data["session_duration_ms"]),
		intOrNil(data["page_duration_ms"]),
		intOrNil(data["last_step_delta_ms"]),
		strOrNil(data["referrer"]),
		metaBytes,
	)
	return err
}

func (m *FeedbackModel) ScheduleFollowUp(ctx context.Context, fu FeedbackFollowUp) error {
	metaBytes, _ := json.Marshal(fu.Metadata)
	_, err := m.db.ExecContext(
		ctx,
		`INSERT INTO feedback_followups (
            user_email, trigger_key, scheduled_at, metadata
        ) VALUES ($1,$2,$3,$4)
        ON CONFLICT DO NOTHING`,
		fu.UserEmail,
		fu.TriggerKey,
		fu.ScheduledAt,
		metaBytes,
	)
	return err
}

func (m *FeedbackModel) FetchDueFollowUps(ctx context.Context, limit int) ([]FeedbackFollowUp, error) {
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(ctx, `
        SELECT id, user_email, trigger_key, scheduled_at, metadata
        FROM feedback_followups
        WHERE status = 'pending' AND scheduled_at <= NOW()
        ORDER BY scheduled_at ASC
        LIMIT $1
        FOR UPDATE SKIP LOCKED
    `, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []FeedbackFollowUp
	for rows.Next() {
		var item FeedbackFollowUp
		var metaBytes []byte
		if err = rows.Scan(&item.ID, &item.UserEmail, &item.TriggerKey, &item.ScheduledAt, &metaBytes); err != nil {
			return nil, err
		}
		if len(metaBytes) > 0 {
			_ = json.Unmarshal(metaBytes, &item.Metadata)
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	if len(items) > 0 {
		ids := make([]int64, len(items))
		for i, item := range items {
			ids[i] = item.ID
		}
		query := `UPDATE feedback_followups SET status = 'processing' WHERE id = ANY($1)`
		if _, err = tx.ExecContext(ctx, query, pq.Array(ids)); err != nil {
			return nil, err
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (m *FeedbackModel) MarkFollowUpSent(ctx context.Context, id int64) error {
	_, err := m.db.ExecContext(ctx, `
        UPDATE feedback_followups
        SET status = 'sent', sent_at = NOW(), last_error = NULL
        WHERE id = $1
    `, id)
	return err
}

func (m *FeedbackModel) MarkFollowUpFailed(ctx context.Context, id int64, errMsg string) error {
	_, err := m.db.ExecContext(ctx, `
        UPDATE feedback_followups
        SET status = 'failed', last_error = $2
        WHERE id = $1
    `, id, errMsg)
	return err
}

func strOrNil(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		if strings.TrimSpace(val) == "" {
			return nil
		}
		return strings.TrimSpace(val)
	default:
		return val
	}
}

func intOrNil(v interface{}) interface{} {
	switch val := v.(type) {
	case nil:
		return nil
	case int:
		if val <= 0 {
			return nil
		}
		return val
	case int64:
		if val <= 0 {
			return nil
		}
		return val
	case float64:
		if val <= 0 {
			return nil
		}
		return int64(val)
	case *int:
		if val == nil || *val <= 0 {
			return nil
		}
		return *val
	case *int64:
		if val == nil || *val <= 0 {
			return nil
		}
		return *val
	default:
		return nil
	}
}
