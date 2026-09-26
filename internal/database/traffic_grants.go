package database

import (
	"context"
	"fmt"
)

// The subscription base excludes paid traffic packs. Purchases are the permanent
// grant ledger, so neither a renewal nor a monthly panel reset consumes them.
func (pr *PurchaseRepository) EnsureTrafficBase(ctx context.Context, subscriptionID, initialLimit int64) error {
	_, err := pr.pool.Exec(ctx, `INSERT INTO subscription_traffic_limit (subscription_id, base_limit)
		VALUES ($1, $2) ON CONFLICT (subscription_id) DO NOTHING`, subscriptionID, initialLimit)
	return err
}

func (pr *PurchaseRepository) SetTrafficBase(ctx context.Context, subscriptionID, limit int64) error {
	_, err := pr.pool.Exec(ctx, `INSERT INTO subscription_traffic_limit (subscription_id, base_limit)
		VALUES ($1, $2) ON CONFLICT (subscription_id) DO UPDATE SET base_limit = EXCLUDED.base_limit, updated_at = NOW()`, subscriptionID, limit)
	return err
}

func (pr *PurchaseRepository) TrafficLimitState(ctx context.Context, subscriptionID, pendingPurchaseID int64) (base, extra int64, unlimited bool, err error) {
	err = pr.pool.QueryRow(ctx, `SELECT l.base_limit,
		COALESCE(SUM(p.extra_traffic_bytes), 0), COALESCE(BOOL_OR(p.extra_traffic_unlimited), FALSE)
		FROM subscription_traffic_limit l LEFT JOIN purchase p ON p.subscription_id = l.subscription_id
		AND (p.status = 'paid' OR p.id = $2)
		WHERE l.subscription_id = $1 GROUP BY l.base_limit`, subscriptionID, pendingPurchaseID).Scan(&base, &extra, &unlimited)
	return
}

func (pr *PurchaseRepository) MarkPaidWithEntitlementBases(ctx context.Context, purchaseID, subscriptionID int64, deviceBase int, trafficBase int64) error {
	tx, err := pr.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE purchase SET status = 'paid', paid_at = NOW() WHERE id = $1`, purchaseID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("purchase %d not found", purchaseID)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO subscription_device_limit (subscription_id, base_limit) VALUES ($1, $2)
		ON CONFLICT (subscription_id) DO UPDATE SET base_limit = EXCLUDED.base_limit, updated_at = NOW()`, subscriptionID, deviceBase); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO subscription_traffic_limit (subscription_id, base_limit) VALUES ($1, $2)
		ON CONFLICT (subscription_id) DO UPDATE SET base_limit = EXCLUDED.base_limit, updated_at = NOW()`, subscriptionID, trafficBase); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (pr *PurchaseRepository) TrafficGrantSubscriptions(ctx context.Context) ([]int64, error) {
	rows, err := pr.pool.Query(ctx, `SELECT DISTINCT subscription_id FROM purchase WHERE status = 'paid'
		AND subscription_id IS NOT NULL AND (extra_traffic_bytes > 0 OR extra_traffic_unlimited) ORDER BY subscription_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
