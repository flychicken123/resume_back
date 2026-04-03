package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

type UserChatProfile struct {
	UserID    int                    `json:"user_id"`
	Profile   map[string]interface{} `json:"profile"`
	Summary   string                 `json:"summary"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type UserChatProfileModel struct {
	db *sql.DB
}

func NewUserChatProfileModel(db *sql.DB) *UserChatProfileModel {
	return &UserChatProfileModel{db: db}
}

func (m *UserChatProfileModel) Get(userID int) (*UserChatProfile, error) {
	var p UserChatProfile
	var profileJSON []byte
	err := m.db.QueryRow(
		`SELECT user_id, profile, summary, updated_at FROM user_chat_profiles WHERE user_id = $1`,
		userID,
	).Scan(&p.UserID, &profileJSON, &p.Summary, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(profileJSON, &p.Profile); err != nil {
		p.Profile = map[string]interface{}{}
	}
	return &p, nil
}

func (m *UserChatProfileModel) Upsert(userID int, profile map[string]interface{}, summary string) error {
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	_, err = m.db.Exec(`
		INSERT INTO user_chat_profiles (user_id, profile, summary, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id) DO UPDATE SET
			profile = $2,
			summary = $3,
			updated_at = CURRENT_TIMESTAMP`,
		userID, profileJSON, summary,
	)
	return err
}

func (m *UserChatProfileModel) Clear(userID int) error {
	_, err := m.db.Exec(
		`DELETE FROM user_chat_profiles WHERE user_id = $1`, userID,
	)
	return err
}
