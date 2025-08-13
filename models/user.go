package models

import (
	"database/sql"
	"time"
)

type User struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Password  string    `json:"-"` // Don't include password in JSON
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserModel struct {
	DB *sql.DB
}

func NewUserModel(db *sql.DB) *UserModel {
	return &UserModel{DB: db}
}

func (m *UserModel) Create(email, name, password string) (*User, error) {
	user := &User{}
	query := `
		INSERT INTO users (email, name, password, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		RETURNING id, email, name, created_at, updated_at
	`
	err := m.DB.QueryRow(query, email, name, password, time.Now()).Scan(
		&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (m *UserModel) GetByEmail(email string) (*User, error) {
	user := &User{}
	query := `
		SELECT id, email, name, password, created_at, updated_at
		FROM users WHERE email = $1
	`
	err := m.DB.QueryRow(query, email).Scan(
		&user.ID, &user.Email, &user.Name, &user.Password, &user.CreatedAt, &user.UpdatedAt,
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
