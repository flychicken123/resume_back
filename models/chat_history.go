package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// ChatMessageRecord captures a single chat message exchanged with the assistant.
type ChatMessageRecord struct {
	SessionID string
	Role      string
	Message   string
	UserEmail sql.NullString
	PagePath  sql.NullString
	Metadata  map[string]interface{}
}

// ChatHistoryModel persists chat exchanges for later review.
type ChatHistoryModel struct {
	db *sql.DB
}

// NewChatHistoryModel constructs a ChatHistoryModel.
func NewChatHistoryModel(db *sql.DB) *ChatHistoryModel {
	return &ChatHistoryModel{db: db}
}

// SaveMessages stores one or more chat messages in a single transaction to keep ordering consistent.
func (m *ChatHistoryModel) SaveMessages(ctx context.Context, messages []ChatMessageRecord) error {
	if len(messages) == 0 {
		return nil
	}

	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt := `
        INSERT INTO assistant_chat_messages (
            session_id,
            role,
            message,
            user_email,
            page_path,
            metadata,
            created_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7)
    `

	now := time.Now().UTC()
	for _, msg := range messages {
		metaBytes, _ := json.Marshal(msg.Metadata)
		if _, err = tx.ExecContext(
			ctx,
			stmt,
			msg.SessionID,
			msg.Role,
			msg.Message,
			nullStringValue(msg.UserEmail),
			nullStringValue(msg.PagePath),
			nullJSONValue(metaBytes),
			now,
		); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func nullStringValue(value sql.NullString) interface{} {
	if value.Valid {
		return value.String
	}
	return nil
}

func nullJSONValue(raw []byte) interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}
