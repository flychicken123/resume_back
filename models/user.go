package models

import (
	"database/sql"
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
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
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
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	user.MarketingOptedAt = nullTimeToPtr(marketingOptedAt)
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
	query := `
		SELECT id, email, name, is_admin,
		       marketing_opt_in,
		       marketing_opted_at,
		       marketing_opt_in_source,
		       signup_plan_preference,
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
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	user.MarketingOptedAt = nullTimeToPtr(marketingOptedAt)
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

func nullTimeToPtr(nt sql.NullTime) *time.Time {
	if nt.Valid {
		t := nt.Time
		return &t
	}
	return nil
}
