package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type WebPushVAPIDConfig struct {
	PublicKey  string
	PrivateKey string
}

type WebPushSubscription struct {
	ID              int64
	AdminTelegramID int64
	Endpoint        string
	P256DH          string
	Auth            string
	UserAgent       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastSuccessAt   *time.Time
	FailureCount    int
}

type WebPushRepository struct {
	pool *pgxpool.Pool
}

func NewWebPushRepository(pool *pgxpool.Pool) *WebPushRepository {
	return &WebPushRepository{pool: pool}
}

func (r *WebPushRepository) VAPIDConfig(ctx context.Context) (*WebPushVAPIDConfig, error) {
	var result WebPushVAPIDConfig
	err := r.pool.QueryRow(ctx, `
		SELECT public_key, private_key
		FROM admin_web_push_config
		WHERE singleton = TRUE
	`).Scan(&result.PublicKey, &result.PrivateKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *WebPushRepository) SaveVAPIDConfig(ctx context.Context, publicKey, privateKey string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO admin_web_push_config (singleton, public_key, private_key)
		VALUES (TRUE, $1, $2)
		ON CONFLICT (singleton) DO NOTHING
	`, publicKey, privateKey)
	return err
}

func (r *WebPushRepository) UpsertSubscription(ctx context.Context, subscription WebPushSubscription) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO admin_web_push_subscription (
			admin_telegram_id, endpoint, p256dh, auth, user_agent
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (endpoint) DO UPDATE SET
			admin_telegram_id = EXCLUDED.admin_telegram_id,
			p256dh = EXCLUDED.p256dh,
			auth = EXCLUDED.auth,
			user_agent = EXCLUDED.user_agent,
			updated_at = NOW(),
			failure_count = 0
	`, subscription.AdminTelegramID, subscription.Endpoint, subscription.P256DH, subscription.Auth, subscription.UserAgent)
	return err
}

func (r *WebPushRepository) DeleteSubscription(ctx context.Context, adminTelegramID int64, endpoint string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM admin_web_push_subscription
		WHERE admin_telegram_id = $1 AND endpoint = $2
	`, adminTelegramID, endpoint)
	return err
}

func (r *WebPushRepository) DeleteSubscriptionByID(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM admin_web_push_subscription WHERE id = $1`, id)
	return err
}

func (r *WebPushRepository) ListSubscriptions(ctx context.Context, adminTelegramID int64) ([]WebPushSubscription, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, admin_telegram_id, endpoint, p256dh, auth, user_agent,
		       created_at, updated_at, last_success_at, failure_count
		FROM admin_web_push_subscription
		WHERE admin_telegram_id = $1
		ORDER BY updated_at DESC
	`, adminTelegramID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]WebPushSubscription, 0)
	for rows.Next() {
		var item WebPushSubscription
		if err := rows.Scan(
			&item.ID,
			&item.AdminTelegramID,
			&item.Endpoint,
			&item.P256DH,
			&item.Auth,
			&item.UserAgent,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LastSuccessAt,
			&item.FailureCount,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *WebPushRepository) CountSubscriptions(ctx context.Context, adminTelegramID int64) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM admin_web_push_subscription
		WHERE admin_telegram_id = $1
	`, adminTelegramID).Scan(&count)
	return count, err
}

func (r *WebPushRepository) MarkSubscriptionSuccess(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE admin_web_push_subscription
		SET last_success_at = NOW(), failure_count = 0, updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

func (r *WebPushRepository) MarkSubscriptionFailure(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE admin_web_push_subscription
		SET failure_count = failure_count + 1, updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}
