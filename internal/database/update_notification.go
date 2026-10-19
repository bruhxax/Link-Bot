package database

import (
	"context"
	"github.com/jackc/pgx/v4/pgxpool"
)

type UpdateNotificationRepository struct{ pool *pgxpool.Pool }

func NewUpdateNotificationRepository(pool *pgxpool.Pool) *UpdateNotificationRepository {
	return &UpdateNotificationRepository{pool: pool}
}

// Claim uses a short delivery lease; failed Telegram sends can be retried while
// restarts and concurrent checks do not repeatedly notify about the same head.
func (r *UpdateNotificationRepository) Claim(ctx context.Context, revision string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `INSERT INTO app_update_notification_state (id,revision,lease_until) VALUES(1,$1,NOW()+INTERVAL '2 minutes')
ON CONFLICT(id) DO UPDATE SET revision=EXCLUDED.revision,notified_at=NULL,lease_until=EXCLUDED.lease_until
WHERE (app_update_notification_state.lease_until IS NULL OR app_update_notification_state.lease_until<NOW())
AND (app_update_notification_state.revision<>EXCLUDED.revision OR app_update_notification_state.notified_at IS NULL)`, revision)
	return err == nil && tag.RowsAffected() == 1, err
}
func (r *UpdateNotificationRepository) Finish(ctx context.Context, revision string, delivered bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE app_update_notification_state SET lease_until=NULL,notified_at=CASE WHEN $2 THEN NOW() ELSE NULL END WHERE id=1 AND revision=$1`, revision, delivered)
	return err
}
