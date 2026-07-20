package database

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

type RuntimeSettingsRepository struct {
	pool *pgxpool.Pool
}

type OperationalEvent struct {
	ID              int64           `json:"id"`
	Fingerprint     string          `json:"-"`
	Category        string          `json:"category"`
	Severity        string          `json:"severity"`
	Operation       string          `json:"operation"`
	Message         string          `json:"message"`
	Details         json.RawMessage `json:"details,omitempty"`
	OccurrenceCount int             `json:"occurrenceCount"`
	FirstSeenAt     time.Time       `json:"firstSeenAt"`
	LastSeenAt      time.Time       `json:"lastSeenAt"`
	ResolvedAt      *time.Time      `json:"resolvedAt,omitempty"`
}

type OperationalEventInput struct {
	Fingerprint string
	Category    string
	Severity    string
	Operation   string
	Message     string
	Details     json.RawMessage
}

func NewRuntimeSettingsRepository(pool *pgxpool.Pool) *RuntimeSettingsRepository {
	return &RuntimeSettingsRepository{pool: pool}
}

func (r *RuntimeSettingsRepository) Load(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := r.pool.QueryRow(ctx, `SELECT config FROM app_runtime_settings WHERE id = 1`).Scan(&raw)
	return raw, err
}

func (r *RuntimeSettingsRepository) Save(ctx context.Context, raw json.RawMessage, updatedBy int64) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO app_runtime_settings (id, config, updated_by, updated_at)
		VALUES (1, $1, $2, NOW())
		ON CONFLICT (id) DO UPDATE SET
			config = EXCLUDED.config,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()
	`, raw, updatedBy)
	return err
}

func (r *RuntimeSettingsRepository) RecordOperationalEvent(ctx context.Context, input OperationalEventInput) (*OperationalEvent, error) {
	details := input.Details
	if len(details) == 0 {
		details = json.RawMessage(`{}`)
	}

	row := r.pool.QueryRow(ctx, `
		INSERT INTO operational_event (
			fingerprint, category, severity, operation, message, details
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (fingerprint) WHERE resolved_at IS NULL DO UPDATE SET
			severity = EXCLUDED.severity,
			message = EXCLUDED.message,
			details = EXCLUDED.details,
			occurrence_count = operational_event.occurrence_count + 1,
			last_seen_at = NOW()
		RETURNING id, fingerprint, category, severity, operation, message, details,
			occurrence_count, first_seen_at, last_seen_at, resolved_at
	`, input.Fingerprint, input.Category, input.Severity, input.Operation, input.Message, details)

	return scanOperationalEvent(row)
}

func (r *RuntimeSettingsRepository) ListOperationalEvents(ctx context.Context, limit int, includeResolved bool) ([]OperationalEvent, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}

	query := `
		SELECT id, fingerprint, category, severity, operation, message, details,
			occurrence_count, first_seen_at, last_seen_at, resolved_at
		FROM operational_event
	`
	if !includeResolved {
		query += ` WHERE resolved_at IS NULL`
	}
	query += ` ORDER BY last_seen_at DESC LIMIT $1`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]OperationalEvent, 0, limit)
	for rows.Next() {
		var item OperationalEvent
		if err := rows.Scan(
			&item.ID,
			&item.Fingerprint,
			&item.Category,
			&item.Severity,
			&item.Operation,
			&item.Message,
			&item.Details,
			&item.OccurrenceCount,
			&item.FirstSeenAt,
			&item.LastSeenAt,
			&item.ResolvedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *RuntimeSettingsRepository) ResolveOperationalEvent(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE operational_event
		SET resolved_at = COALESCE(resolved_at, NOW())
		WHERE id = $1
	`, id)
	return err
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanOperationalEvent(row rowScanner) (*OperationalEvent, error) {
	var item OperationalEvent
	err := row.Scan(
		&item.ID,
		&item.Fingerprint,
		&item.Category,
		&item.Severity,
		&item.Operation,
		&item.Message,
		&item.Details,
		&item.OccurrenceCount,
		&item.FirstSeenAt,
		&item.LastSeenAt,
		&item.ResolvedAt,
	)
	if err != nil {
		return nil, err
	}
	return &item, nil
}
