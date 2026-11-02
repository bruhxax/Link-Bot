package database

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

var ErrThemePresetExists = errors.New("theme preset already exists")
var ErrThemePresetLimit = errors.New("theme preset limit reached")

type ThemePreset struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	Appearance json.RawMessage `json:"appearance"`
	CreatedAt  time.Time       `json:"createdAt"`
}

type ThemePresetRepository struct{ pool *pgxpool.Pool }

func NewThemePresetRepository(pool *pgxpool.Pool) *ThemePresetRepository {
	return &ThemePresetRepository{pool: pool}
}

func (r *ThemePresetRepository) List(ctx context.Context) ([]ThemePreset, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,name,appearance,created_at FROM theme_preset ORDER BY created_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ThemePreset, 0)
	for rows.Next() {
		var item ThemePreset
		if err := rows.Scan(&item.ID, &item.Name, &item.Appearance, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *ThemePresetRepository) Create(ctx context.Context, name string, appearance json.RawMessage) (*ThemePreset, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// Serialize concurrent additions so the shared library keeps its 20-item limit.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(248005)`); err != nil {
		return nil, err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM theme_preset`).Scan(&count); err != nil {
		return nil, err
	}
	if count >= 20 {
		return nil, ErrThemePresetLimit
	}
	var item ThemePreset
	err = tx.QueryRow(ctx, `INSERT INTO theme_preset(name,appearance) VALUES($1,$2) ON CONFLICT DO NOTHING RETURNING id,name,appearance,created_at`, name, appearance).Scan(&item.ID, &item.Name, &item.Appearance, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrThemePresetExists
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *ThemePresetRepository) Delete(ctx context.Context, id int64) (bool, error) {
	result, err := r.pool.Exec(ctx, `DELETE FROM theme_preset WHERE id=$1`, id)
	return err == nil && result.RowsAffected() > 0, err
}
