package models

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type User struct {
	ID                   int        `json:"id"`
	Email                string     `json:"email"`
	Name                 string     `json:"name"`
	Password             string     `json:"-"` // Don't include password in JSON
	AuthProvider         string     `json:"auth_provider"`
	GoogleID             string     `json:"google_id,omitempty"`
	ProfilePicture       string     `json:"profile_picture,omitempty"`
	IsAdmin              bool       `json:"is_admin"`
	MarketingOptIn       bool       `json:"marketing_opt_in"`
	MarketingOptedAt     *time.Time `json:"marketing_opted_at,omitempty"`
	MarketingOptInSource string     `json:"marketing_opt_in_source,omitempty"`
	SignupPlanPreference string     `json:"signup_plan_preference,omitempty"`
	EmailUnsubscribed        bool            `json:"email_unsubscribed"`
	EmailUnsubscribedAt      *time.Time      `json:"email_unsubscribed_at,omitempty"`
	JobPreferences           json.RawMessage `json:"job_preferences,omitempty"`
	FollowupRemindersEnabled bool            `json:"followup_reminders_enabled"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
}

type UserModel struct {
	DB *sql.DB
}

func NewUserModel(db *sql.DB) *UserModel {
	return &UserModel{DB: db}
}

func (m *UserModel) Create(email, name, password string, marketingOptIn bool, planPreference string) (*User, error) {
	return m.createInternal(email, name, password, "email", "", "", marketingOptIn, "email_signup", planPreference)
}

func (m *UserModel) CreateWithProvider(email, name, password, authProvider, googleID, profilePicture string, marketingOptIn bool, planPreference string) (*User, error) {
	source := authProvider
	if source == "" {
		source = "external_signup"
	}
	return m.createInternal(email, name, password, authProvider, googleID, profilePicture, marketingOptIn, source, planPreference)
}

func (m *UserModel) createInternal(email, name, password, authProvider, googleID, profilePicture string, marketingOptIn bool, marketingSource string, planPreference string) (*User, error) {
	now := time.Now()
	var optedAt sql.NullTime
	if marketingOptIn {
		optedAt = sql.NullTime{Time: now, Valid: true}
	}

	marketingSource = strings.TrimSpace(marketingSource)
	if marketingSource == "" {
		marketingSource = "unspecified"
	}

	planPreference = strings.ToLower(strings.TrimSpace(planPreference))
	if planPreference == "" {
		planPreference = "free"
	}

	user := &User{}
	var marketingOptedAt sql.NullTime
	var marketingSourceNS sql.NullString
	var planPreferenceNS sql.NullString

	query := `
		INSERT INTO users (
			email, name, password, auth_provider, google_id, profile_picture,
			marketing_opt_in, marketing_opted_at, marketing_opt_in_source, signup_plan_preference,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''), $11, $11)
		RETURNING id, email, name, auth_provider, google_id, profile_picture, is_admin,
		          marketing_opt_in, marketing_opted_at, marketing_opt_in_source, signup_plan_preference,
		          COALESCE(followup_reminders_enabled, true),
		          created_at, updated_at
	`

	err := m.DB.QueryRow(query,
		email,
		name,
		password,
		authProvider,
		googleID,
		profilePicture,
		marketingOptIn,
		optedAt,
		marketingSource,
		planPreference,
		now,
	).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.AuthProvider,
		&user.GoogleID,
		&user.ProfilePicture,
		&user.IsAdmin,
		&user.MarketingOptIn,
		&marketingOptedAt,
		&marketingSourceNS,
		&planPreferenceNS,
		&user.FollowupRemindersEnabled,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	user.MarketingOptedAt = nullTimeToPtr(marketingOptedAt)
	if marketingSourceNS.Valid {
		user.MarketingOptInSource = marketingSourceNS.String
	}
	if planPreferenceNS.Valid {
		user.SignupPlanPreference = planPreferenceNS.String
	}
	return user, nil
}

func (m *UserModel) GetByEmail(email string) (*User, error) {
	user := &User{}
	var marketingOptedAt sql.NullTime
	var marketingSource sql.NullString
	var planPreference sql.NullString
	var emailUnsubscribedAt sql.NullTime
	query := `
		SELECT id, email, name, password,
		       COALESCE(auth_provider, 'email') as auth_provider,
		       google_id,
		       profile_picture,
		       is_admin,
		       marketing_opt_in,
		       marketing_opted_at,
		       marketing_opt_in_source,
		       signup_plan_preference,
		       COALESCE(email_unsubscribed, false),
		       email_unsubscribed_at,
		       COALESCE(followup_reminders_enabled, true),
		       created_at, updated_at
		FROM users WHERE email = $1
	`
	err := m.DB.QueryRow(query, email).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.Password,
		&user.AuthProvider,
		&user.GoogleID,
		&user.ProfilePicture,
		&user.IsAdmin,
		&user.MarketingOptIn,
		&marketingOptedAt,
		&marketingSource,
		&planPreference,
		&user.EmailUnsubscribed,
		&emailUnsubscribedAt,
		&user.FollowupRemindersEnabled,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	user.MarketingOptedAt = nullTimeToPtr(marketingOptedAt)
	user.EmailUnsubscribedAt = nullTimeToPtr(emailUnsubscribedAt)
	if marketingSource.Valid {
		user.MarketingOptInSource = marketingSource.String
	}
	if planPreference.Valid {
		user.SignupPlanPreference = planPreference.String
	}
	return user, nil
}

func (m *UserModel) GetByID(id int) (*User, error) {
	user := &User{}
	var marketingOptedAt sql.NullTime
	var marketingSource sql.NullString
	var planPreference sql.NullString
	var emailUnsubscribedAt sql.NullTime
	query := `
		SELECT id, email, name, is_admin,
		       marketing_opt_in,
		       marketing_opted_at,
		       marketing_opt_in_source,
		       signup_plan_preference,
		       COALESCE(email_unsubscribed, false),
		       email_unsubscribed_at,
		       COALESCE(followup_reminders_enabled, true),
		       created_at, updated_at
		FROM users WHERE id = $1
	`
	err := m.DB.QueryRow(query, id).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.IsAdmin,
		&user.MarketingOptIn,
		&marketingOptedAt,
		&marketingSource,
		&planPreference,
		&user.EmailUnsubscribed,
		&emailUnsubscribedAt,
		&user.FollowupRemindersEnabled,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	user.MarketingOptedAt = nullTimeToPtr(marketingOptedAt)
	user.EmailUnsubscribedAt = nullTimeToPtr(emailUnsubscribedAt)
	if marketingSource.Valid {
		user.MarketingOptInSource = marketingSource.String
	}
	if planPreference.Valid {
		user.SignupPlanPreference = planPreference.String
	}
	return user, nil
}

func (m *UserModel) UpdateProfile(id int, name string) error {
	query := `UPDATE users SET name = $1, updated_at = $2 WHERE id = $3`
	_, err := m.DB.Exec(query, name, time.Now(), id)
	return err
}

func (m *UserModel) UpdatePassword(id int, password string) error {
	query := `UPDATE users SET password = $1, updated_at = $2 WHERE id = $3`
	_, err := m.DB.Exec(query, password, time.Now(), id)
	return err
}

// SetEmailUnsubscribed updates the email_unsubscribed status for a user by email
func (m *UserModel) SetEmailUnsubscribed(email string, unsubscribed bool) error {
	now := time.Now()
	var unsubscribedAt sql.NullTime
	if unsubscribed {
		unsubscribedAt = sql.NullTime{Time: now, Valid: true}
	}
	query := `UPDATE users SET email_unsubscribed = $1, email_unsubscribed_at = $2, updated_at = $3 WHERE email = $4`
	_, err := m.DB.Exec(query, unsubscribed, unsubscribedAt, now, email)
	return err
}

// IsEmailUnsubscribed checks if a user has unsubscribed from emails
func (m *UserModel) IsEmailUnsubscribed(email string) (bool, error) {
	var unsubscribed bool
	query := `SELECT COALESCE(email_unsubscribed, false) FROM users WHERE email = $1`
	err := m.DB.QueryRow(query, email).Scan(&unsubscribed)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return unsubscribed, err
}

// SetFollowupReminders updates the followup_reminders_enabled flag for a user
func (m *UserModel) SetFollowupReminders(userID int, enabled bool) error {
	query := `UPDATE users SET followup_reminders_enabled = $1, updated_at = $2 WHERE id = $3`
	result, err := m.DB.Exec(query, enabled, time.Now(), userID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetJobPreferences returns the job_preferences JSON for a user
func (m *UserModel) GetJobPreferences(userID int) (json.RawMessage, error) {
	var prefs sql.NullString
	err := m.DB.QueryRow(`SELECT COALESCE(job_preferences::text, '{}') FROM users WHERE id = $1`, userID).Scan(&prefs)
	if err != nil {
		return nil, err
	}
	if !prefs.Valid || prefs.String == "" {
		return json.RawMessage(`{}`), nil
	}
	return json.RawMessage(prefs.String), nil
}

// SetJobPreferences updates the job_preferences JSON for a user
func (m *UserModel) SetJobPreferences(userID int, prefs json.RawMessage) error {
	_, err := m.DB.Exec(
		`UPDATE users SET job_preferences = $1, updated_at = $2 WHERE id = $3`,
		string(prefs), time.Now(), userID,
	)
	return err
}

func nullTimeToPtr(nt sql.NullTime) *time.Time {
	if nt.Valid {
		t := nt.Time
		return &t
	}
	return nil
}
