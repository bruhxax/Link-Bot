package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v4"
)

// ClaimDeviceNotifications atomically advances the stored device state for one
// subscription and returns only transitions that have not been delivered before.
func (cr *CustomerRepository) ClaimDeviceNotifications(ctx context.Context, telegramID, subscriptionID int64, deviceCount, deviceLimit int) (int, bool, error) {
	if telegramID <= 0 || subscriptionID <= 0 {
		return 0, false, nil
	}
	if deviceCount < 0 {
		deviceCount = 0
	}
	if deviceLimit < 0 {
		deviceLimit = 0
	}

	tx, err := cr.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin device notification transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var previousCount int
	var previousLimitReached bool
	err = tx.QueryRow(ctx, `
		SELECT device_count, limit_reached
		FROM device_notification_state
		WHERE telegram_id = $1 AND subscription_id = $2
		FOR UPDATE
	`, telegramID, subscriptionID).Scan(&previousCount, &previousLimitReached)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `
			INSERT INTO device_notification_state (telegram_id, subscription_id, device_count, limit_reached)
			VALUES ($1, $2, $3, $4)
		`, telegramID, subscriptionID, deviceCount, deviceLimit > 0 && deviceCount >= deviceLimit)
		if err != nil {
			return 0, false, fmt.Errorf("initialize device notification state: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return 0, false, fmt.Errorf("commit device notification state: %w", err)
		}
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("load device notification state: %w", err)
	}

	added := deviceCount - previousCount
	if added < 0 {
		added = 0
	}
	limitReached := deviceLimit > 0 && deviceCount >= deviceLimit
	newLimitReached := limitReached && !previousLimitReached
	_, err = tx.Exec(ctx, `
		UPDATE device_notification_state
		SET device_count = $3, limit_reached = $4, updated_at = NOW()
		WHERE telegram_id = $1 AND subscription_id = $2
	`, telegramID, subscriptionID, deviceCount, limitReached)
	if err != nil {
		return 0, false, fmt.Errorf("update device notification state: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("commit device notification state: %w", err)
	}
	return added, newLimitReached, nil
}
