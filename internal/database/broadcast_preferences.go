package database

import (
	"context"
	"errors"
)

var ErrBroadcastUnsubscribed = errors.New("recipient unsubscribed or is unavailable")

func (r *CustomerRepository) BroadcastTelegramAllowed(ctx context.Context, telegramID int64, ignoreUnsubscribe bool) (bool, error) {
	var allowed bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer c WHERE c.telegram_id=$1
		AND NOT c.is_blocked AND NOT c.telegram_id_is_synthetic AND c.telegram_id>0
		AND ($2 OR NOT EXISTS(SELECT 1 FROM customer_broadcast_unsubscribe u WHERE u.customer_id=c.id)))`, telegramID, ignoreUnsubscribe).Scan(&allowed)
	return allowed, err
}

func (r *CustomerRepository) BroadcastEmailAllowed(ctx context.Context, email string, ignoreUnsubscribe bool) (bool, error) {
	var allowed bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer c JOIN customer_email_auth e ON e.customer_id=c.id
		WHERE e.email=$1 AND NOT c.is_blocked
		AND ($2 OR NOT EXISTS(SELECT 1 FROM customer_broadcast_unsubscribe u WHERE u.customer_id=c.id)))`, email, ignoreUnsubscribe).Scan(&allowed)
	return allowed, err
}

// No row means subscribed, so existing customers keep their current behavior.
func (r *CustomerRepository) BroadcastSubscribed(ctx context.Context, customerID int64) (bool, error) {
	var subscribed bool
	err := r.pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM customer_broadcast_unsubscribe WHERE customer_id=$1)`, customerID).Scan(&subscribed)
	return subscribed, err
}

func (r *CustomerRepository) SetBroadcastSubscribed(ctx context.Context, customerID int64, subscribed bool) error {
	if customerID <= 0 {
		return errors.New("invalid broadcast customer")
	}
	if subscribed {
		_, err := r.pool.Exec(ctx, `DELETE FROM customer_broadcast_unsubscribe WHERE customer_id=$1`, customerID)
		return err
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO customer_broadcast_unsubscribe(customer_id) VALUES($1) ON CONFLICT(customer_id) DO NOTHING`, customerID)
	return err
}
