package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// UserApplicationPreference represents a saved user preference for job applications
type UserApplicationPreference struct {
	ID              int       `json:"id"`
	UserEmail       string    `json:"user_email"`
	FieldKey        string    `json:"field_key"`
	FieldValue      string    `json:"field_value"`
	FieldType       string    `json:"field_type"`
	FieldLabel      string    `json:"field_label"`
	ConfidenceScore float64   `json:"confidence_score"`
	UsageCount      int       `json:"usage_count"`
	LastUsed        time.Time `json:"last_used"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// JobFormField represents a detected form field from job applications
type JobFormField struct {
	ID              int       `json:"id"`
	Platform        string    `json:"platform"`
	Company         string    `json:"company"`
	FieldID         string    `json:"field_id"`
	FieldLabel      string    `json:"field_label"`
	FieldType       string    `json:"field_type"`
	FieldOptions    string    `json:"field_options"` // JSON string
	FieldValidation string    `json:"field_validation"` // JSON string
	IsRequired      bool      `json:"is_required"`
	MappedTo        string    `json:"mapped_to"`
	Frequency       int       `json:"frequency"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ApplicationSubmission represents a job application submission record
type ApplicationSubmission struct {
	ID             int       `json:"id"`
	ApplicationID  string    `json:"application_id"`
	UserEmail      string    `json:"user_email"`
	JobURL         string    `json:"job_url"`
	JobTitle       string    `json:"job_title"`
	Company        string    `json:"company"`
	Platform       string    `json:"platform"`
	FormData       string    `json:"form_data"` // JSON string
	Status         string    `json:"status"`
	MissingFields  string    `json:"missing_fields"` // JSON string
	SubmissionTime time.Time `json:"submission_time"`
	SuccessRate    float64   `json:"success_rate"`
	Notes          string    `json:"notes"`
}

// FieldMapping represents a mapping between external and internal field names
type FieldMapping struct {
	ID            int     `json:"id"`
	ExternalField string  `json:"external_field"`
	StandardField string  `json:"standard_field"`
	Platform      string  `json:"platform"`
	Confidence    float64 `json:"confidence"`
}

// ApplicationPreferencesModel handles database operations for application preferences
type ApplicationPreferencesModel struct {
	db *sql.DB
}

// NewApplicationPreferencesModel creates a new instance of ApplicationPreferencesModel
func NewApplicationPreferencesModel(db *sql.DB) *ApplicationPreferencesModel {
	return &ApplicationPreferencesModel{db: db}
}

// SaveUserPreference saves or updates a user's preference for a specific field
func (m *ApplicationPreferencesModel) SaveUserPreference(pref *UserApplicationPreference) error {
	query := `
		INSERT INTO user_application_preferences 
		(user_email, field_key, field_value, field_type, field_label, confidence_score)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_email, field_key) 
		DO UPDATE SET 
			field_value = EXCLUDED.field_value,
			field_type = EXCLUDED.field_type,
			field_label = EXCLUDED.field_label,
			confidence_score = EXCLUDED.confidence_score,
			usage_count = user_application_preferences.usage_count + 1,
			last_used = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id, usage_count, last_used, created_at, updated_at`

	err := m.db.QueryRow(query,
		pref.UserEmail,
		pref.FieldKey,
		pref.FieldValue,
		pref.FieldType,
		pref.FieldLabel,
		pref.ConfidenceScore,
	).Scan(&pref.ID, &pref.UsageCount, &pref.LastUsed, &pref.CreatedAt, &pref.UpdatedAt)

	return err
}

// GetUserPreferences retrieves all preferences for a user
func (m *ApplicationPreferencesModel) GetUserPreferences(userEmail string) ([]*UserApplicationPreference, error) {
	query := `
		SELECT id, user_email, field_key, field_value, field_type, field_label, 
		       confidence_score, usage_count, last_used, created_at, updated_at
		FROM user_application_preferences
		WHERE user_email = $1
		ORDER BY usage_count DESC, last_used DESC`

	rows, err := m.db.Query(query, userEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var preferences []*UserApplicationPreference
	for rows.Next() {
		pref := &UserApplicationPreference{}
		err := rows.Scan(
			&pref.ID,
			&pref.UserEmail,
			&pref.FieldKey,
			&pref.FieldValue,
			&pref.FieldType,
			&pref.FieldLabel,
			&pref.ConfidenceScore,
			&pref.UsageCount,
			&pref.LastUsed,
			&pref.CreatedAt,
			&pref.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		preferences = append(preferences, pref)
	}

	return preferences, nil
}

// GetUserPreferenceByKey retrieves a specific preference for a user
func (m *ApplicationPreferencesModel) GetUserPreferenceByKey(userEmail, fieldKey string) (*UserApplicationPreference, error) {
	query := `
		SELECT id, user_email, field_key, field_value, field_type, field_label, 
		       confidence_score, usage_count, last_used, created_at, updated_at
		FROM user_application_preferences
		WHERE user_email = $1 AND field_key = $2`

	pref := &UserApplicationPreference{}
	err := m.db.QueryRow(query, userEmail, fieldKey).Scan(
		&pref.ID,
		&pref.UserEmail,
		&pref.FieldKey,
		&pref.FieldValue,
		&pref.FieldType,
		&pref.FieldLabel,
		&pref.ConfidenceScore,
		&pref.UsageCount,
		&pref.LastUsed,
		&pref.CreatedAt,
		&pref.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return pref, err
}

// SaveApplicationSubmission saves a record of a job application submission
func (m *ApplicationPreferencesModel) SaveApplicationSubmission(submission *ApplicationSubmission) error {
	query := `
		INSERT INTO application_submissions 
		(application_id, user_email, job_url, job_title, company, platform, 
		 form_data, status, missing_fields, success_rate, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, submission_time`

	err := m.db.QueryRow(query,
		submission.ApplicationID,
		submission.UserEmail,
		submission.JobURL,
		submission.JobTitle,
		submission.Company,
		submission.Platform,
		submission.FormData,
		submission.Status,
		submission.MissingFields,
		submission.SuccessRate,
		submission.Notes,
	).Scan(&submission.ID, &submission.SubmissionTime)

	return err
}

// GetApplicationHistory retrieves application history for a user
func (m *ApplicationPreferencesModel) GetApplicationHistory(userEmail string, limit int) ([]*ApplicationSubmission, error) {
	query := `
		SELECT id, application_id, user_email, job_url, job_title, company, platform,
		       form_data, status, missing_fields, submission_time, success_rate, notes
		FROM application_submissions
		WHERE user_email = $1
		ORDER BY submission_time DESC
		LIMIT $2`

	rows, err := m.db.Query(query, userEmail, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var submissions []*ApplicationSubmission
	for rows.Next() {
		sub := &ApplicationSubmission{}
		err := rows.Scan(
			&sub.ID,
			&sub.ApplicationID,
			&sub.UserEmail,
			&sub.JobURL,
			&sub.JobTitle,
			&sub.Company,
			&sub.Platform,
			&sub.FormData,
			&sub.Status,
			&sub.MissingFields,
			&sub.SubmissionTime,
			&sub.SuccessRate,
			&sub.Notes,
		)
		if err != nil {
			return nil, err
		}
		submissions = append(submissions, sub)
	}

	return submissions, nil
}

// SaveFormField saves or updates a detected form field
func (m *ApplicationPreferencesModel) SaveFormField(field *JobFormField) error {
	query := `
		INSERT INTO job_form_fields 
		(platform, company, field_id, field_label, field_type, field_options, 
		 field_validation, is_required, mapped_to)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (platform, field_id) 
		DO UPDATE SET 
			frequency = job_form_fields.frequency + 1,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id, frequency, created_at, updated_at`

	err := m.db.QueryRow(query,
		field.Platform,
		field.Company,
		field.FieldID,
		field.FieldLabel,
		field.FieldType,
		field.FieldOptions,
		field.FieldValidation,
		field.IsRequired,
		field.MappedTo,
	).Scan(&field.ID, &field.Frequency, &field.CreatedAt, &field.UpdatedAt)

	return err
}

// GetFieldMapping finds the standard field name for an external field
func (m *ApplicationPreferencesModel) GetFieldMapping(externalField, platform string) (string, error) {
	// First try platform-specific mapping
	query := `
		SELECT standard_field FROM field_mappings 
		WHERE external_field = $1 AND platform = $2
		ORDER BY confidence DESC
		LIMIT 1`

	var standardField string
	err := m.db.QueryRow(query, externalField, platform).Scan(&standardField)
	if err == nil {
		return standardField, nil
	}

	// Try generic mapping if no platform-specific mapping found
	if err == sql.ErrNoRows {
		query = `
			SELECT standard_field FROM field_mappings 
			WHERE external_field = $1 AND platform IS NULL
			ORDER BY confidence DESC
			LIMIT 1`
		
		err = m.db.QueryRow(query, externalField).Scan(&standardField)
		if err == sql.ErrNoRows {
			return "", nil // No mapping found
		}
	}

	return standardField, err
}

// BatchSavePreferences saves multiple preferences at once
func (m *ApplicationPreferencesModel) BatchSavePreferences(userEmail string, preferences map[string]interface{}) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for key, value := range preferences {
		valueStr := fmt.Sprintf("%v", value)
		fieldType := "text"
		
		// Determine field type
		switch value.(type) {
		case int, int64, float64:
			fieldType = "number"
		case bool:
			fieldType = "checkbox"
		}

		_, err = tx.Exec(`
			INSERT INTO user_application_preferences 
			(user_email, field_key, field_value, field_type)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_email, field_key) 
			DO UPDATE SET 
				field_value = EXCLUDED.field_value,
				usage_count = user_application_preferences.usage_count + 1,
				last_used = CURRENT_TIMESTAMP,
				updated_at = CURRENT_TIMESTAMP`,
			userEmail, key, valueStr, fieldType)
		
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetMissingFields returns fields that don't have saved preferences
func (m *ApplicationPreferencesModel) GetMissingFields(userEmail string, requiredFields []string) ([]string, error) {
	// Get all user preferences
	prefs, err := m.GetUserPreferences(userEmail)
	if err != nil {
		return nil, err
	}

	// Create a map of existing preferences
	existingFields := make(map[string]bool)
	for _, pref := range prefs {
		existingFields[pref.FieldKey] = true
	}

	// Find missing fields
	var missingFields []string
	for _, field := range requiredFields {
		if !existingFields[field] {
			missingFields = append(missingFields, field)
		}
	}

	return missingFields, nil
}

// AutoFillFormData attempts to fill form data using saved preferences
func (m *ApplicationPreferencesModel) AutoFillFormData(userEmail string, formFields map[string]interface{}) (map[string]interface{}, []string, error) {
	filledData := make(map[string]interface{})
	var missingFields []string

	// Get user preferences
	prefs, err := m.GetUserPreferences(userEmail)
	if err != nil {
		return nil, nil, err
	}

	// Create preference map for quick lookup
	prefMap := make(map[string]*UserApplicationPreference)
	for _, pref := range prefs {
		prefMap[pref.FieldKey] = pref
	}

	// Attempt to fill each form field
	for fieldName := range formFields {
		// Try to get standard field name
		standardField, _ := m.GetFieldMapping(fieldName, "")
		if standardField == "" {
			standardField = fieldName
		}

		// Check if we have a preference for this field
		if pref, exists := prefMap[standardField]; exists {
			// Convert value based on field type
			switch pref.FieldType {
			case "number":
				var num float64
				json.Unmarshal([]byte(pref.FieldValue), &num)
				filledData[fieldName] = num
			case "checkbox", "boolean":
				filledData[fieldName] = pref.FieldValue == "true"
			default:
				filledData[fieldName] = pref.FieldValue
			}
		} else {
			missingFields = append(missingFields, fieldName)
		}
	}

	successRate := float64(len(filledData)) / float64(len(formFields)) * 100
	
	// Save submission record
	submission := &ApplicationSubmission{
		ApplicationID: fmt.Sprintf("app_%d", time.Now().Unix()),
		UserEmail:     userEmail,
		Status:        "auto_filled",
		SuccessRate:   successRate,
	}
	
	if len(missingFields) > 0 {
		missingJSON, _ := json.Marshal(missingFields)
		submission.MissingFields = string(missingJSON)
		submission.Status = "pending_info"
	}

	m.SaveApplicationSubmission(submission)

	return filledData, missingFields, nil
}