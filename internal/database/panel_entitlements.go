package database

import (
	"context"
	"time"
)

// RebasePanelEntitlements runs under LockDeviceLimit before a purchase. A
// differing live limit is an administrator override, not missing paid access.
// Supersede only the affected dimension and retain the financial history.
// Accept the pending purchase's target as well, so a retried add-on callback
// does not mistake its own successful panel update for a manual override.
func (pr *PurchaseRepository) RebasePanelEntitlements(ctx context.Context, subscriptionID int64, pending *Purchase, devices int, traffic int64, now time.Time) error {
	tx, err := pr.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var deviceBase, deviceExtra int
	var trafficBase, trafficExtra int64
	var trafficUnlimited bool
	var deviceKnown, trafficKnown bool
	err = tx.QueryRow(ctx, `SELECT
		EXISTS (SELECT 1 FROM subscription_device_limit WHERE subscription_id = $1),
		COALESCE((SELECT base_limit FROM subscription_device_limit WHERE subscription_id = $1), 0),
		COALESCE((SELECT SUM(g.devices) FROM subscription_device_grant g JOIN purchase p ON p.id = g.purchase_id
		 WHERE g.subscription_id = $1 AND p.status = 'paid' AND p.device_entitlement_superseded_at IS NULL
		 AND (g.expires_at > $2 OR g.expired_applied_at IS NULL)), 0),
		EXISTS (SELECT 1 FROM subscription_traffic_limit WHERE subscription_id = $1),
		COALESCE((SELECT base_limit FROM subscription_traffic_limit WHERE subscription_id = $1), 0),
		COALESCE((SELECT SUM(extra_traffic_bytes) FROM purchase WHERE subscription_id = $1 AND status = 'paid'
		 AND traffic_entitlement_superseded_at IS NULL), 0),
		COALESCE((SELECT BOOL_OR(extra_traffic_unlimited) FROM purchase WHERE subscription_id = $1 AND status = 'paid'
		 AND traffic_entitlement_superseded_at IS NULL), FALSE)`, subscriptionID, now).Scan(
		&deviceKnown, &deviceBase, &deviceExtra, &trafficKnown, &trafficBase, &trafficExtra, &trafficUnlimited)
	if err != nil {
		return err
	}
	deviceTarget, trafficTarget := expectedPanelEntitlements(deviceBase, deviceExtra, trafficBase, trafficExtra, trafficUnlimited)
	pendingDevice, pendingTraffic := deviceTarget, trafficTarget
	prepared := false
	if pending != nil {
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM purchase WHERE id=$1 AND entitlement_prepared_at IS NOT NULL)`, pending.ID).Scan(&prepared); err != nil {
			return err
		}
	}
	if prepared && (pending.PurchaseKind == PurchaseKindExtraDevices || pending.PurchaseKind == PurchaseKindExtraTraffic) {
		if pendingDevice > 0 {
			pendingDevice += pending.ExtraDevices
		}
		if pending.ExtraTrafficUnlimited {
			pendingTraffic = 0
		} else if pendingTraffic > 0 {
			pendingTraffic += pending.ExtraTrafficBytes
		}
	}
	if devices >= 0 && deviceKnown && devices != deviceTarget && devices != pendingDevice {
		if _, err = tx.Exec(ctx, `UPDATE purchase SET device_entitlement_superseded_at = $2
			WHERE subscription_id = $1 AND status = 'paid' AND extra_devices > 0 AND device_entitlement_superseded_at IS NULL`, subscriptionID, now); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE subscription_device_limit SET base_limit = $2, updated_at = NOW() WHERE subscription_id = $1`, subscriptionID, devices); err != nil {
			return err
		}
	}
	if traffic >= 0 && trafficKnown && traffic != trafficTarget && traffic != pendingTraffic {
		if _, err = tx.Exec(ctx, `UPDATE purchase SET traffic_entitlement_superseded_at = $2
			WHERE subscription_id = $1 AND status = 'paid' AND (extra_traffic_bytes > 0 OR extra_traffic_unlimited)
			AND traffic_entitlement_superseded_at IS NULL`, subscriptionID, now); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE subscription_traffic_limit SET base_limit = $2, updated_at = NOW() WHERE subscription_id = $1`, subscriptionID, traffic); err != nil {
			return err
		}
	}
	if pending != nil {
		if _, err := tx.Exec(ctx, `UPDATE purchase SET entitlement_prepared_at=COALESCE(entitlement_prepared_at,$2) WHERE id=$1`, pending.ID, now); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func expectedPanelEntitlements(deviceBase, deviceExtra int, trafficBase, trafficExtra int64, trafficUnlimited bool) (int, int64) {
	devices, traffic := 0, int64(0)
	if deviceBase > 0 {
		devices = deviceBase + deviceExtra
	}
	if trafficBase > 0 && !trafficUnlimited {
		traffic = trafficBase + trafficExtra
	}
	return devices, traffic
}
