package database

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// DeviceNotificationSnapshot describes a complete, successful HWID read.
// A nil DeviceHWIDs slice means unknown; an empty non-nil slice means no devices.
type DeviceNotificationSnapshot struct {
	PanelUserKey string
	DeviceHWIDs  []string
	DeviceLimit  int
	ObservedAt   time.Time
}

type deviceNotificationState struct {
	PanelUserKey string
	DeviceHWIDs  []string
	LimitReached bool
	ObservedAt   *time.Time
}

func normalizeDeviceNotificationSnapshot(snapshot DeviceNotificationSnapshot) (DeviceNotificationSnapshot, bool) {
	snapshot.PanelUserKey = strings.TrimSpace(snapshot.PanelUserKey)
	if snapshot.PanelUserKey == "" || snapshot.DeviceHWIDs == nil || snapshot.ObservedAt.IsZero() {
		return snapshot, false
	}
	hwids := make([]string, 0, len(snapshot.DeviceHWIDs))
	seen := make(map[string]bool, len(snapshot.DeviceHWIDs))
	for _, raw := range snapshot.DeviceHWIDs {
		hwid := strings.TrimSpace(raw)
		if hwid == "" {
			return snapshot, false
		}
		if !seen[hwid] {
			hwids = append(hwids, hwid)
			seen[hwid] = true
		}
	}
	sort.Strings(hwids)
	snapshot.DeviceHWIDs = hwids
	snapshot.ObservedAt = snapshot.ObservedAt.UTC().Truncate(time.Microsecond)
	if snapshot.DeviceLimit < 0 {
		snapshot.DeviceLimit = 0
	}
	return snapshot, true
}

func deviceNotificationChanges(previous deviceNotificationState, snapshot DeviceNotificationSnapshot) (added int, limitReached, fresh bool) {
	// Concurrent cabinet refreshes can finish out of order. An older read must
	// not restore deleted devices or erase a newer snapshot.
	if previous.ObservedAt != nil && !snapshot.ObservedAt.After(*previous.ObservedAt) {
		return 0, false, false
	}
	if previous.DeviceHWIDs == nil || previous.PanelUserKey != snapshot.PanelUserKey {
		// First read, migration or subscription reissue: existing devices are a
		// baseline, not new connections.
		return 0, false, true
	}
	known := make(map[string]bool, len(previous.DeviceHWIDs))
	for _, hwid := range previous.DeviceHWIDs {
		known[hwid] = true
	}
	for _, hwid := range snapshot.DeviceHWIDs {
		if !known[hwid] {
			added++
		}
	}
	// A successful connection already includes the used/available count. Do not
	// send a second alert for the same connection (including the first 1/1).
	limitReached = added == 0 && snapshot.DeviceLimit > 0 && len(snapshot.DeviceHWIDs) >= snapshot.DeviceLimit && !previous.LimitReached
	return added, limitReached, true
}

// ClaimDeviceNotifications atomically stores a subscription's confirmed HWIDs
// and claims each new-device/limit transition once, across concurrent requests.
func (cr *CustomerRepository) ClaimDeviceNotifications(ctx context.Context, telegramID, subscriptionID int64, snapshot DeviceNotificationSnapshot) (int, bool, error) {
	if telegramID <= 0 || subscriptionID <= 0 {
		return 0, false, nil
	}
	snapshot, valid := normalizeDeviceNotificationSnapshot(snapshot)
	if !valid {
		return 0, false, nil
	}
	deviceCount := len(snapshot.DeviceHWIDs)
	limitReached := snapshot.DeviceLimit > 0 && deviceCount >= snapshot.DeviceLimit

	tx, err := cr.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin device notification transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// ON CONFLICT also serializes simultaneous first reads, which cannot be
	// locked with SELECT FOR UPDATE before the row exists.
	initialized, err := tx.Exec(ctx, `
		INSERT INTO device_notification_state
		    (telegram_id, subscription_id, device_count, limit_reached, device_hwids, panel_user_key, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (telegram_id, subscription_id) DO NOTHING
	`, telegramID, subscriptionID, deviceCount, limitReached, snapshot.DeviceHWIDs, snapshot.PanelUserKey, snapshot.ObservedAt)
	if err != nil {
		return 0, false, fmt.Errorf("initialize device notification state: %w", err)
	}
	if initialized.RowsAffected() == 1 {
		if err := tx.Commit(ctx); err != nil {
			return 0, false, fmt.Errorf("commit device notification state: %w", err)
		}
		return 0, false, nil
	}

	var previous deviceNotificationState
	err = tx.QueryRow(ctx, `
		SELECT device_hwids, panel_user_key, limit_reached, observed_at
		FROM device_notification_state
		WHERE telegram_id = $1 AND subscription_id = $2
		FOR UPDATE
	`, telegramID, subscriptionID).Scan(&previous.DeviceHWIDs, &previous.PanelUserKey, &previous.LimitReached, &previous.ObservedAt)
	if err != nil {
		return 0, false, fmt.Errorf("load device notification state: %w", err)
	}

	added, newLimitReached, fresh := deviceNotificationChanges(previous, snapshot)
	if fresh {
		_, err = tx.Exec(ctx, `
			UPDATE device_notification_state
			SET device_count = $3, limit_reached = $4, device_hwids = $5,
			    panel_user_key = $6, observed_at = $7, updated_at = NOW()
			WHERE telegram_id = $1 AND subscription_id = $2
		`, telegramID, subscriptionID, deviceCount, limitReached, snapshot.DeviceHWIDs, snapshot.PanelUserKey, snapshot.ObservedAt)
		if err != nil {
			return 0, false, fmt.Errorf("update device notification state: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("commit device notification state: %w", err)
	}
	return added, newLimitReached, nil
}
