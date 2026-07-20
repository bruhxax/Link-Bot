package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type PaymentIntegration struct {
	Provider        string
	Enabled         bool
	EncryptedConfig string
	WebhookToken    string
	UpdatedBy       *int64
	UpdatedAt       time.Time
}

type PaymentIntegrationRepository struct {
	pool *pgxpool.Pool
}

func NewPaymentIntegrationRepository(pool *pgxpool.Pool) *PaymentIntegrationRepository {
	return &PaymentIntegrationRepository{pool: pool}
}

func (r *PaymentIntegrationRepository) List(ctx context.Context) ([]PaymentIntegration, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT provider, enabled, encrypted_config, webhook_token, updated_by, updated_at
		FROM payment_integration
		ORDER BY provider
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]PaymentIntegration, 0, 8)
	for rows.Next() {
		var item PaymentIntegration
		if err := rows.Scan(&item.Provider, &item.Enabled, &item.EncryptedConfig, &item.WebhookToken, &item.UpdatedBy, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PaymentIntegrationRepository) Find(ctx context.Context, provider string) (*PaymentIntegration, error) {
	var item PaymentIntegration
	err := r.pool.QueryRow(ctx, `
		SELECT provider, enabled, encrypted_config, webhook_token, updated_by, updated_at
		FROM payment_integration
		WHERE provider = $1
	`, provider).Scan(&item.Provider, &item.Enabled, &item.EncryptedConfig, &item.WebhookToken, &item.UpdatedBy, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *PaymentIntegrationRepository) Upsert(ctx context.Context, item PaymentIntegration) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO payment_integration (
			provider, enabled, encrypted_config, webhook_token, updated_by, updated_at
		) VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (provider) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			encrypted_config = EXCLUDED.encrypted_config,
			webhook_token = EXCLUDED.webhook_token,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()
	`, item.Provider, item.Enabled, item.EncryptedConfig, item.WebhookToken, item.UpdatedBy)
	return err
}
