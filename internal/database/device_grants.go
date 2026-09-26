package database

import (
	"context"
	"fmt"
	"time"
)

type DeviceGrant struct {
	PurchaseID         int64
	SubscriptionID     int64
	Devices            int
	ExpiresAt          time.Time
	ExpiredAppliedAt   *time.Time
	PurchaseNotifiedAt *time.Time
	ReminderNotifiedAt *time.Time
	ExpiryNotifiedAt   *time.Time
}

// Serialize purchases, renewals and expiry jobs for the same subscription.
// Absolute panel limits make retrying a failed payment callback idempotent.
func (pr *PurchaseRepository) LockDeviceLimit(ctx context.Context, subscriptionID int64) (func() error, error) {
	conn, err := pr.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	key := int64(827364117000000000) + subscriptionID
	// A session lock must not occupy pool capacity while the protected work
	// acquires other connections (several concurrent subscriptions could deadlock).
	lockConn := conn.Hijack()
	if _, err = lockConn.Exec(ctx, "SELECT pg_advisory_lock($1)", key); err != nil {
		_ = lockConn.Close(context.Background())
		return nil, err
	}
	return func() error {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Closing the dedicated session also releases the lock if unlock fails.
		_, unlockErr := lockConn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", key)
		closeErr := lockConn.Close(context.Background())
		if unlockErr != nil {
			return unlockErr
		}
		return closeErr
	}, nil
}

func (pr *PurchaseRepository) EnsureDeviceBase(ctx context.Context, subscriptionID int64, initialLimit int) error {
	_, err := pr.pool.Exec(ctx, `INSERT INTO subscription_device_limit (subscription_id, base_limit)
        VALUES ($1, $2) ON CONFLICT (subscription_id) DO NOTHING`, subscriptionID, initialLimit)
	return err
}

func (pr *PurchaseRepository) SetDeviceBase(ctx context.Context, subscriptionID int64, limit int) error {
	_, err := pr.pool.Exec(ctx, `INSERT INTO subscription_device_limit (subscription_id, base_limit)
        VALUES ($1, $2) ON CONFLICT (subscription_id) DO UPDATE SET base_limit = EXCLUDED.base_limit, updated_at = NOW()`, subscriptionID, limit)
	return err
}

func (pr *PurchaseRepository) MarkPaidWithDeviceBase(ctx context.Context, purchaseID, subscriptionID int64, base int) error {
	result, err := pr.pool.Exec(ctx, `WITH paid AS (
        UPDATE purchase SET status = 'paid', paid_at = NOW() WHERE id = $1 RETURNING id
    ) INSERT INTO subscription_device_limit (subscription_id, base_limit)
        SELECT $2, $3 FROM paid ON CONFLICT (subscription_id)
        DO UPDATE SET base_limit = EXCLUDED.base_limit, updated_at = NOW()`, purchaseID, subscriptionID, base)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("purchase %d not found", purchaseID)
	}
	return nil
}

func (pr *PurchaseRepository) DeviceLimitState(ctx context.Context, subscriptionID, pendingPurchaseID int64, now time.Time) (base, extra int, err error) {
	err = pr.pool.QueryRow(ctx, `SELECT l.base_limit, COALESCE((SELECT SUM(g.devices)
        FROM subscription_device_grant g JOIN purchase p ON p.id = g.purchase_id
        WHERE g.subscription_id = l.subscription_id AND g.expires_at > $3
          AND (p.status = 'paid' OR p.id = $2)), 0)
        FROM subscription_device_limit l WHERE l.subscription_id = $1`, subscriptionID, pendingPurchaseID, now).Scan(&base, &extra)
	return
}

func (pr *PurchaseRepository) AddDeviceGrant(ctx context.Context, purchaseID, subscriptionID int64, devices int, expiresAt time.Time) error {
	_, err := pr.pool.Exec(ctx, `INSERT INTO subscription_device_grant (purchase_id, subscription_id, devices, expires_at)
        VALUES ($1, $2, $3, $4) ON CONFLICT (purchase_id) DO NOTHING`, purchaseID, subscriptionID, devices, expiresAt)
	return err
}

func (pr *PurchaseRepository) PendingDeviceSubscriptions(ctx context.Context, reminderBefore time.Time) ([]int64, error) {
	rows, err := pr.pool.Query(ctx, `SELECT DISTINCT g.subscription_id FROM subscription_device_grant g
        JOIN purchase p ON p.id = g.purchase_id WHERE p.status = 'paid' AND (
        g.purchase_notified_at IS NULL OR (g.expires_at <= NOW() AND
        (g.expired_applied_at IS NULL OR g.expiry_notified_at IS NULL)) OR
        (g.reminder_notified_at IS NULL AND g.expires_at > NOW() AND g.expires_at <= $1))
        ORDER BY g.subscription_id`, reminderBefore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (pr *PurchaseRepository) DeviceGrants(ctx context.Context, subscriptionID int64) ([]DeviceGrant, error) {
	rows, err := pr.pool.Query(ctx, `SELECT g.purchase_id, g.subscription_id, g.devices, g.expires_at,
        g.expired_applied_at, g.purchase_notified_at, g.reminder_notified_at, g.expiry_notified_at
        FROM subscription_device_grant g JOIN purchase p ON p.id = g.purchase_id
        WHERE g.subscription_id = $1 AND p.status = 'paid' ORDER BY g.purchase_id`, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DeviceGrant{}
	for rows.Next() {
		var g DeviceGrant
		if err := rows.Scan(&g.PurchaseID, &g.SubscriptionID, &g.Devices, &g.ExpiresAt, &g.ExpiredAppliedAt,
			&g.PurchaseNotifiedAt, &g.ReminderNotifiedAt, &g.ExpiryNotifiedAt); err != nil {
			return nil, err
		}
		result = append(result, g)
	}
	return result, rows.Err()
}

func (pr *PurchaseRepository) MarkDeviceGrantsExpired(ctx context.Context, subscriptionID int64, now time.Time) error {
	_, err := pr.pool.Exec(ctx, `UPDATE subscription_device_grant g SET expired_applied_at = $2
        FROM purchase p WHERE p.id = g.purchase_id AND p.status = 'paid' AND g.subscription_id = $1
        AND g.expires_at <= $2 AND g.expired_applied_at IS NULL`, subscriptionID, now)
	return err
}

func (pr *PurchaseRepository) MarkDeviceGrantNotified(ctx context.Context, purchaseID int64, kind string) error {
	column := ""
	switch kind {
	case "purchase":
		column = "purchase_notified_at"
	case "reminder":
		column = "reminder_notified_at"
	case "expiry":
		column = "expiry_notified_at"
	default:
		return fmt.Errorf("unknown device notification %q", kind)
	}
	_, err := pr.pool.Exec(ctx, "UPDATE subscription_device_grant SET "+column+" = NOW() WHERE purchase_id = $1", purchaseID)
	return err
}
