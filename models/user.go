package models

import (
	"database/sql"
	"time"
)

type User struct {
	ID             int       `json:"id"`
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	Password       string    `json:"-"` // Don't include password in JSON
	AuthProvider   string    `json:"auth_provider"`
	GoogleID       string    `json:"google_id,omitempty"`
	ProfilePicture string    `json:"profile_picture,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type UserModel struct {
	DB *sql.DB
}

func NewUserModel(db *sql.DB) *UserModel {
	return &UserModel{DB: db}
}

func (m *UserModel) Create(email, name, password string) (*User, error) {
	return m.CreateWithProvider(email, name, password, "email", "", "")
}

func (m *UserModel) CreateWithProvider(email, name, password, authProvider, googleID, profilePicture string) (*User, error) {
	user := &User{}
	query := `
		INSERT INTO users (email, name, password, auth_provider, google_id, profile_picture, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		RETURNING id, email, name, auth_provider, google_id, profile_picture, created_at, updated_at
	`
	err := m.DB.QueryRow(query, email, name, password, authProvider, googleID, profilePicture, time.Now()).Scan(
		&user.ID, &user.Email, &user.Name, &user.AuthProvider, &user.GoogleID, &user.ProfilePicture, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (m *UserModel) GetByEmail(email string) (*User, error) {
	user := &User{}
	query := `
		SELECT id, email, name, password, auth_provider, google_id, profile_picture, created_at, updated_at
		FROM users WHERE email = $1
	`
	err := m.DB.QueryRow(query, email).Scan(
		&user.ID, &user.Email, &user.Name, &user.Password, &user.AuthProvider, &user.GoogleID, &user.ProfilePicture, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (m *UserModel) GetByID(id int) (*User, error) {
	user := &User{}
	query := `
		SELECT id, email, name, created_at, updated_at
		FROM users WHERE id = $1
	`
	err := m.DB.QueryRow(query, id).Scan(
		&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
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
