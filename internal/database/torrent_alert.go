package database

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
)

// RecordTorrentAlert atomically deduplicates a report and appends history only
// to accounts linked to that panel identity. Telegram IDs and usernames from
// the webhook are deliberately not used to assign ownership.
func (r *CustomerRepository) RecordTorrentAlert(ctx context.Context, key string, panelUUID uuid.UUID, panelID int64, entry UserActivity) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var inserted string
	err = tx.QueryRow(ctx, `INSERT INTO torrent_alert(event_key) VALUES($1)
		ON CONFLICT DO NOTHING RETURNING event_key`, key).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT ON (customer_id) customer_id,display_name
		FROM customer_subscription WHERE panel_user_uuid=$1
		OR (panel_user_uuid IS NULL AND panel_user_id=$2 AND $2>0)
		ORDER BY customer_id,is_primary DESC,id`, panelUUID, panelID)
	if err != nil {
		return err
	}
	type target struct {
		customer int64
		name     string
	}
	var targets []target
	for rows.Next() {
		var item target
		if err := rows.Scan(&item.customer, &item.name); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range targets {
		details := append([]UserActivityDetail{}, entry.Details...)
		details = append(details, UserActivityDetail{Label: "Подписка", Value: item.name})
		raw, err := json.Marshal(details)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_activity(customer_id,source,action,title,status,details,created_at)
			VALUES($1,'remnawave','torrent_blocker.report',$2,'info',$3,$4)`, item.customer, entry.Title, string(raw), entry.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ClaimTorrentAlert serializes Telegram delivery across replicas. An expired
// lease can be retried after a crashed process; successful deliveries persist.
func (r *CustomerRepository) ClaimTorrentAlert(ctx context.Context, key string, token uuid.UUID) (claimed, delivered bool, err error) {
	result, err := r.pool.Exec(ctx, `UPDATE torrent_alert SET notification_token=$2,
		notification_lease_until=NOW()+INTERVAL '30 seconds'
		WHERE event_key=$1 AND notified_at IS NULL
		AND (notification_lease_until IS NULL OR notification_lease_until<NOW())`, key, token)
	if err != nil {
		return false, false, err
	}
	if result.RowsAffected() > 0 {
		return true, false, nil
	}
	err = r.pool.QueryRow(ctx, `SELECT notified_at IS NOT NULL FROM torrent_alert WHERE event_key=$1`, key).Scan(&delivered)
	return false, delivered, err
}

func (r *CustomerRepository) FinishTorrentAlert(ctx context.Context, key string, token uuid.UUID, delivered bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE torrent_alert
		SET notified_at=CASE WHEN $3 THEN NOW() ELSE notified_at END,
		    notification_token=NULL,notification_lease_until=NULL
		WHERE event_key=$1 AND notification_token=$2`, key, token, delivered)
	return err
}
