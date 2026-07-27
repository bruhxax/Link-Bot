package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type ReleaseNotificationRepository struct {
	pool *pgxpool.Pool
}

func NewReleaseNotificationRepository(pool *pgxpool.Pool) *ReleaseNotificationRepository {
	return &ReleaseNotificationRepository{pool: pool}
}

func (r *ReleaseNotificationRepository) LastNotifiedRevision(ctx context.Context) (string, bool, error) {
	var revision string
	err := r.pool.QueryRow(ctx, `
		SELECT revision
		FROM app_release_notification_state
		WHERE id = 1
	`).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return revision, true, nil
}

func (r *ReleaseNotificationRepository) MarkNotified(ctx context.Context, revision string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO app_release_notification_state (id, revision, notified_at)
		VALUES (1, $1, NOW())
		ON CONFLICT (id) DO UPDATE SET
			revision = EXCLUDED.revision,
			notified_at = NOW()
	`, revision)
	return err
}
