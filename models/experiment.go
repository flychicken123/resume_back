package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/lib/pq"
)

var ErrExperimentNotFound = errors.New("experiment not found")

type Experiment struct {
	ID          int        `json:"id"`
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	StartAt     *time.Time `json:"start_at,omitempty"`
	EndAt       *time.Time `json:"end_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type ExperimentVariant struct {
	ID           int       `json:"id"`
	ExperimentID int       `json:"experiment_id"`
	VariantKey   string    `json:"variant_key"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Weight       int       `json:"weight"`
	IsControl    bool      `json:"is_control"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ExperimentAssignment struct {
	ID             int64     `json:"id"`
	ExperimentID   int       `json:"experiment_id"`
	VariantID      int       `json:"variant_id"`
	UserIdentifier string    `json:"user_identifier"`
	RequestPath    string    `json:"request_path,omitempty"`
	BucketedAt     time.Time `json:"bucketed_at"`
}

type ExperimentEvent struct {
	ID             int64                  `json:"id"`
	ExperimentID   int                    `json:"experiment_id"`
	VariantID      int                    `json:"variant_id"`
	UserIdentifier string                 `json:"user_identifier"`
	EventName      string                 `json:"event_name"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	OccurredAt     time.Time              `json:"occurred_at"`
}

type VariantEventSummary struct {
	EventName   string `json:"event_name"`
	TotalEvents int64  `json:"total_events"`
	UniqueUsers int64  `json:"unique_users"`
}

type VariantMetrics struct {
	VariantID   int                   `json:"variant_id"`
	VariantKey  string                `json:"variant_key"`
	VariantName string                `json:"variant_name"`
	IsControl   bool                  `json:"is_control"`
	Weight      int                   `json:"weight"`
	Assignments int64                 `json:"assignments"`
	Events      []VariantEventSummary `json:"events"`
}

type ExperimentWithVariants struct {
	Experiment Experiment          `json:"experiment"`
	Variants   []ExperimentVariant `json:"variants"`
	Metrics    []VariantMetrics    `json:"metrics,omitempty"`
}

type ExperimentModel struct {
	db *sql.DB
}

func NewExperimentModel(db *sql.DB) *ExperimentModel {
	return &ExperimentModel{db: db}
}

// EnsureExperimentSchema creates the core experimentation tables when migrations haven't been applied yet.
func EnsureExperimentSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS experiments (
            id SERIAL PRIMARY KEY,
            experiment_key VARCHAR(100) UNIQUE NOT NULL,
            name VARCHAR(255) NOT NULL,
            description TEXT,
            status VARCHAR(32) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'running', 'paused', 'completed')),
            start_at TIMESTAMP WITH TIME ZONE,
            end_at TIMESTAMP WITH TIME ZONE,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE TABLE IF NOT EXISTS experiment_variants (
            id SERIAL PRIMARY KEY,
            experiment_id INTEGER NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
            variant_key VARCHAR(64) NOT NULL,
            name VARCHAR(255) NOT NULL,
            description TEXT,
            weight INTEGER NOT NULL DEFAULT 50 CHECK (weight >= 0),
            is_control BOOLEAN NOT NULL DEFAULT FALSE,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            UNIQUE (experiment_id, variant_key)
        )`,
		`CREATE TABLE IF NOT EXISTS experiment_assignments (
            id BIGSERIAL PRIMARY KEY,
            experiment_id INTEGER NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
            variant_id INTEGER NOT NULL REFERENCES experiment_variants(id) ON DELETE CASCADE,
            user_identifier VARCHAR(128) NOT NULL,
            request_path TEXT,
            bucketed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            UNIQUE (experiment_id, user_identifier)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_experiment_assignments_lookup ON experiment_assignments (experiment_id, user_identifier)`,
		`CREATE TABLE IF NOT EXISTS experiment_events (
            id BIGSERIAL PRIMARY KEY,
            experiment_id INTEGER NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
            variant_id INTEGER NOT NULL REFERENCES experiment_variants(id) ON DELETE CASCADE,
            user_identifier VARCHAR(128) NOT NULL,
            event_name VARCHAR(120) NOT NULL,
            metadata JSONB,
            occurred_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_experiment_events_lookup ON experiment_events (experiment_id, variant_id, event_name)`,
		`CREATE INDEX IF NOT EXISTS idx_experiment_events_user ON experiment_events (experiment_id, user_identifier)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (m *ExperimentModel) UpsertExperiment(ctx context.Context, exp Experiment, variants []ExperimentVariant) (*ExperimentWithVariants, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	status := exp.Status
	if status == "" {
		status = "draft"
	}

	var existingID int
	var startAt sql.NullTime
	var endAt sql.NullTime
	var createdAt time.Time
	var updatedAt time.Time

	err = tx.QueryRowContext(ctx, `
        SELECT id, start_at, end_at, created_at, updated_at
        FROM experiments
        WHERE experiment_key = $1
    `, exp.Key).Scan(&existingID, &startAt, &endAt, &createdAt, &updatedAt)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		err = tx.QueryRowContext(ctx, `
            INSERT INTO experiments (experiment_key, name, description, status, start_at, end_at, created_at, updated_at)
            VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
            RETURNING id, start_at, end_at, created_at, updated_at
        `, exp.Key, exp.Name, exp.Description, status, timePtrToNull(exp.StartAt), timePtrToNull(exp.EndAt)).
			Scan(&existingID, &startAt, &endAt, &createdAt, &updatedAt)
	case err != nil:
		return nil, err
	default:
		err = tx.QueryRowContext(ctx, `
            UPDATE experiments
            SET name = $1,
                description = $2,
                status = $3,
                start_at = $4,
                end_at = $5,
                updated_at = NOW()
            WHERE id = $6
            RETURNING start_at, end_at, updated_at
        `, exp.Name, exp.Description, status, timePtrToNull(exp.StartAt), timePtrToNull(exp.EndAt), existingID).
			Scan(&startAt, &endAt, &updatedAt)
		if err != nil {
			return nil, err
		}
	}

	if err != nil {
		return nil, err
	}

	exp.ID = existingID
	exp.Status = status
	exp.StartAt = nullTimeToPtr(startAt)
	exp.EndAt = nullTimeToPtr(endAt)
	exp.CreatedAt = createdAt
	exp.UpdatedAt = updatedAt

	variantKeys := make([]string, 0, len(variants))
	upserted := make([]ExperimentVariant, 0, len(variants))
	for _, variant := range variants {
		var variantID int
		var variantCreatedAt time.Time
		var variantUpdatedAt time.Time
		variantKeys = append(variantKeys, variant.VariantKey)

		err = tx.QueryRowContext(ctx, `
            INSERT INTO experiment_variants (
                experiment_id, variant_key, name, description, weight, is_control, created_at, updated_at
            ) VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
            ON CONFLICT (experiment_id, variant_key) DO UPDATE
            SET name = EXCLUDED.name,
                description = EXCLUDED.description,
                weight = EXCLUDED.weight,
                is_control = EXCLUDED.is_control,
                updated_at = NOW()
            RETURNING id, created_at, updated_at
        `, exp.ID, variant.VariantKey, variant.Name, variant.Description, variant.Weight, variant.IsControl).
			Scan(&variantID, &variantCreatedAt, &variantUpdatedAt)
		if err != nil {
			return nil, err
		}

		variant.ID = variantID
		variant.ExperimentID = exp.ID
		variant.CreatedAt = variantCreatedAt
		variant.UpdatedAt = variantUpdatedAt
		upserted = append(upserted, variant)
	}

	if len(variantKeys) > 0 {
		if _, err := tx.ExecContext(ctx, `
            UPDATE experiment_variants
            SET weight = 0, updated_at = NOW()
            WHERE experiment_id = $1 AND NOT (variant_key = ANY($2))
        `, exp.ID, pq.Array(variantKeys)); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &ExperimentWithVariants{
		Experiment: exp,
		Variants:   upserted,
	}, nil
}

func (m *ExperimentModel) ListExperiments(ctx context.Context) ([]ExperimentWithVariants, error) {
	rows, err := m.db.QueryContext(ctx, `
        SELECT id, experiment_key, name, description, status, start_at, end_at, created_at, updated_at
        FROM experiments
        ORDER BY created_at DESC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	experiments := make([]ExperimentWithVariants, 0)
	idToIndex := make(map[int]int)

	for rows.Next() {
		var exp Experiment
		var startAt sql.NullTime
		var endAt sql.NullTime
		if err := rows.Scan(&exp.ID, &exp.Key, &exp.Name, &exp.Description, &exp.Status, &startAt, &endAt, &exp.CreatedAt, &exp.UpdatedAt); err != nil {
			return nil, err
		}
		exp.StartAt = nullTimeToPtr(startAt)
		exp.EndAt = nullTimeToPtr(endAt)
		idToIndex[exp.ID] = len(experiments)
		experiments = append(experiments, ExperimentWithVariants{Experiment: exp})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(experiments) == 0 {
		return experiments, nil
	}

	ids := make([]int, 0, len(experiments))
	for _, exp := range experiments {
		ids = append(ids, exp.Experiment.ID)
	}

	variantRows, err := m.db.QueryContext(ctx, `
        SELECT id, experiment_id, variant_key, name, description, weight, is_control, created_at, updated_at
        FROM experiment_variants
        WHERE experiment_id = ANY($1)
        ORDER BY experiment_id, id
    `, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer variantRows.Close()

	for variantRows.Next() {
		var variant ExperimentVariant
		if err := variantRows.Scan(
			&variant.ID,
			&variant.ExperimentID,
			&variant.VariantKey,
			&variant.Name,
			&variant.Description,
			&variant.Weight,
			&variant.IsControl,
			&variant.CreatedAt,
			&variant.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if idx, ok := idToIndex[variant.ExperimentID]; ok {
			experiments[idx].Variants = append(experiments[idx].Variants, variant)
		}
	}

	return experiments, variantRows.Err()
}

func (m *ExperimentModel) GetByKey(ctx context.Context, key string) (*ExperimentWithVariants, error) {
	var exp Experiment
	var startAt sql.NullTime
	var endAt sql.NullTime
	err := m.db.QueryRowContext(ctx, `
        SELECT id, experiment_key, name, description, status, start_at, end_at, created_at, updated_at
        FROM experiments
        WHERE experiment_key = $1
        LIMIT 1
    `, key).Scan(&exp.ID, &exp.Key, &exp.Name, &exp.Description, &exp.Status, &startAt, &endAt, &exp.CreatedAt, &exp.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrExperimentNotFound
	}
	if err != nil {
		return nil, err
	}
	exp.StartAt = nullTimeToPtr(startAt)
	exp.EndAt = nullTimeToPtr(endAt)

	variants, err := m.fetchVariants(ctx, []int{exp.ID})
	if err != nil {
		return nil, err
	}

	return &ExperimentWithVariants{
		Experiment: exp,
		Variants:   variants,
	}, nil
}

func (m *ExperimentModel) DeleteByKey(ctx context.Context, key string) error {
	result, err := m.db.ExecContext(ctx, `DELETE FROM experiments WHERE experiment_key = $1`, key)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrExperimentNotFound
	}
	return nil
}

func (m *ExperimentModel) DeleteAssignment(ctx context.Context, experimentID int, userIdentifier string) error {
	_, err := m.db.ExecContext(ctx, `
        DELETE FROM experiment_assignments
        WHERE experiment_id = $1 AND user_identifier = $2
    `, experimentID, userIdentifier)
	return err
}

func (m *ExperimentModel) fetchVariants(ctx context.Context, experimentIDs []int) ([]ExperimentVariant, error) {
	rows, err := m.db.QueryContext(ctx, `
        SELECT id, experiment_id, variant_key, name, description, weight, is_control, created_at, updated_at
        FROM experiment_variants
        WHERE experiment_id = ANY($1)
        ORDER BY experiment_id, id
    `, pq.Array(experimentIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var variants []ExperimentVariant
	for rows.Next() {
		var variant ExperimentVariant
		if err := rows.Scan(
			&variant.ID,
			&variant.ExperimentID,
			&variant.VariantKey,
			&variant.Name,
			&variant.Description,
			&variant.Weight,
			&variant.IsControl,
			&variant.CreatedAt,
			&variant.UpdatedAt,
		); err != nil {
			return nil, err
		}
		variants = append(variants, variant)
	}

	return variants, rows.Err()
}

func (m *ExperimentModel) GetAssignment(ctx context.Context, experimentID int, userIdentifier string) (*ExperimentAssignment, error) {
	var assignment ExperimentAssignment
	var requestPath sql.NullString
	err := m.db.QueryRowContext(ctx, `
        SELECT id, experiment_id, variant_id, user_identifier, request_path, bucketed_at
        FROM experiment_assignments
        WHERE experiment_id = $1 AND user_identifier = $2
        LIMIT 1
    `, experimentID, userIdentifier).Scan(
		&assignment.ID,
		&assignment.ExperimentID,
		&assignment.VariantID,
		&assignment.UserIdentifier,
		&requestPath,
		&assignment.BucketedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if requestPath.Valid {
		assignment.RequestPath = requestPath.String
	}
	return &assignment, nil
}

func (m *ExperimentModel) CreateAssignment(ctx context.Context, experimentID, variantID int, userIdentifier, requestPath string) (*ExperimentAssignment, error) {
	var assignment ExperimentAssignment
	requestPathNS := sql.NullString{}
	if requestPath != "" {
		requestPathNS = sql.NullString{String: requestPath, Valid: true}
	}

	err := m.db.QueryRowContext(ctx, `
        INSERT INTO experiment_assignments (experiment_id, variant_id, user_identifier, request_path)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (experiment_id, user_identifier) DO NOTHING
        RETURNING id, experiment_id, variant_id, user_identifier, request_path, bucketed_at
    `, experimentID, variantID, userIdentifier, requestPathNS).Scan(
		&assignment.ID,
		&assignment.ExperimentID,
		&assignment.VariantID,
		&assignment.UserIdentifier,
		&requestPathNS,
		&assignment.BucketedAt,
	)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return m.GetAssignment(ctx, experimentID, userIdentifier)
	case err != nil:
		return nil, err
	}

	if requestPathNS.Valid {
		assignment.RequestPath = requestPathNS.String
	}
	return &assignment, nil
}

func (m *ExperimentModel) RecordEvent(ctx context.Context, experimentID, variantID int, userIdentifier, eventName string, metadata map[string]interface{}) (*ExperimentEvent, error) {
	metaBytes, _ := json.Marshal(metadata)
	var event ExperimentEvent

	err := m.db.QueryRowContext(ctx, `
        INSERT INTO experiment_events (experiment_id, variant_id, user_identifier, event_name, metadata, occurred_at)
        VALUES ($1, $2, $3, $4, $5, NOW())
        RETURNING id, experiment_id, variant_id, user_identifier, event_name, occurred_at
    `, experimentID, variantID, userIdentifier, eventName, metaBytes).Scan(
		&event.ID,
		&event.ExperimentID,
		&event.VariantID,
		&event.UserIdentifier,
		&event.EventName,
		&event.OccurredAt,
	)

	if err != nil {
		return nil, err
	}

	event.Metadata = metadata
	return &event, nil
}

func (m *ExperimentModel) GetMetrics(ctx context.Context, experimentID int) ([]VariantMetrics, error) {
	rows, err := m.db.QueryContext(ctx, `
        SELECT
            v.id,
            v.variant_key,
            v.name,
            v.is_control,
            v.weight,
            COALESCE(a.assignments, 0) AS assignments,
            ev.event_name,
            COALESCE(ev.events, 0) AS events,
            COALESCE(ev.unique_users, 0) AS unique_users
        FROM experiment_variants v
        LEFT JOIN (
            SELECT variant_id, COUNT(*) AS assignments
            FROM experiment_assignments
            WHERE experiment_id = $1
            GROUP BY variant_id
        ) a ON a.variant_id = v.id
        LEFT JOIN (
            SELECT variant_id, event_name, COUNT(*) AS events, COUNT(DISTINCT user_identifier) AS unique_users
            FROM experiment_events
            WHERE experiment_id = $1
            GROUP BY variant_id, event_name
        ) ev ON ev.variant_id = v.id
        WHERE v.experiment_id = $1
        ORDER BY v.id, ev.event_name
    `, experimentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	metricsByVariant := make(map[int]*VariantMetrics)

	for rows.Next() {
		var (
			variantID   int
			variantKey  string
			name        string
			isControl   bool
			weight      int
			assignments int64
			eventName   sql.NullString
			eventCount  sql.NullInt64
			uniqueUsers sql.NullInt64
		)

		if err := rows.Scan(
			&variantID,
			&variantKey,
			&name,
			&isControl,
			&weight,
			&assignments,
			&eventName,
			&eventCount,
			&uniqueUsers,
		); err != nil {
			return nil, err
		}

		vm, ok := metricsByVariant[variantID]
		if !ok {
			vm = &VariantMetrics{
				VariantID:   variantID,
				VariantKey:  variantKey,
				VariantName: name,
				IsControl:   isControl,
				Weight:      weight,
				Assignments: assignments,
				Events:      []VariantEventSummary{},
			}
			metricsByVariant[variantID] = vm
		}

		if eventName.Valid {
			vm.Events = append(vm.Events, VariantEventSummary{
				EventName:   eventName.String,
				TotalEvents: eventCount.Int64,
				UniqueUsers: uniqueUsers.Int64,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	results := make([]VariantMetrics, 0, len(metricsByVariant))
	for _, vm := range metricsByVariant {
		results = append(results, *vm)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].VariantKey < results[j].VariantKey
	})

	return results, nil
}

func timePtrToNull(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
