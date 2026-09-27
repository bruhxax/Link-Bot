package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var ErrSubscriptionReissueConflict = errors.New("subscription changed during reissue")

type SubscriptionReissue struct {
	ID                        int64
	TelegramID                int64
	OldPanelUserID            *int64
	OldPanelUUID              *uuid.UUID
	NewLink                   string
	DeleteAfter               time.Time
	NotifiedAt                *time.Time
	LastNotificationAttemptAt *time.Time
	DeletedAt                 *time.Time
}

// SwapPanelAccessForReissue records the old identity before exposing the new one.
// Both operations commit together so a restart cannot lose the deletion task.
func (sr *SubscriptionRepository) SwapPanelAccessForReissue(ctx context.Context, subscription *CustomerSubscription, oldID int64, oldUUID uuid.UUID, newID int64, newUUID uuid.UUID, link string, expireAt time.Time, telegramID int64) error {
	if subscription == nil || subscription.ID <= 0 || (oldID <= 0 && oldUUID == uuid.Nil) || (newID <= 0 && newUUID == uuid.Nil) || link == "" || expireAt.IsZero() {
		return errors.New("incomplete subscription reissue")
	}
	tx, err := sr.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var previousID *int64
	var previousUUID *uuid.UUID
	err = tx.QueryRow(ctx, `SELECT panel_user_id, panel_user_uuid FROM customer_subscription WHERE id=$1 AND customer_id=$2 FOR UPDATE`, subscription.ID, subscription.CustomerID).Scan(&previousID, &previousUUID)
	if err != nil {
		return err
	}
	if !sameInt64Pointer(previousID, subscription.PanelUserID) || !sameUUIDPointer(previousUUID, subscription.PanelUserUUID) {
		return ErrSubscriptionReissueConflict
	}
	var storedOldID any
	var storedOldUUID any
	var storedNewID any
	var storedNewUUID any
	if oldID > 0 {
		storedOldID = oldID
	}
	if oldUUID != uuid.Nil {
		storedOldUUID = oldUUID
	}
	if newID > 0 {
		storedNewID = newID
	}
	if newUUID != uuid.Nil {
		storedNewUUID = newUUID
	}
	if _, err = tx.Exec(ctx, `INSERT INTO subscription_reissue (subscription_id, telegram_id, old_panel_user_id, old_panel_user_uuid, new_link, delete_after) VALUES ($1,$2,$3,$4,$5,$6)`, subscription.ID, telegramID, storedOldID, storedOldUUID, link, time.Now().UTC().Add(10*time.Minute)); err != nil {
		return fmt.Errorf("queue old subscription deletion: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE customer_subscription SET panel_user_id=$2, panel_user_uuid=$3, subscription_link=$4, expire_at=$5, updated_at=NOW() WHERE id=$1`, subscription.ID, storedNewID, storedNewUUID, link, expireAt.UTC()); err != nil {
		return fmt.Errorf("switch subscription: %w", err)
	}
	if subscription.IsPrimary {
		if _, err = tx.Exec(ctx, `UPDATE customer SET subscription_link=$2, expire_at=$3 WHERE id=$1`, subscription.CustomerID, link, expireAt.UTC()); err != nil {
			return fmt.Errorf("switch primary subscription: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func sameInt64Pointer(a, b *int64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
func sameUUIDPointer(a, b *uuid.UUID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func (sr *SubscriptionRepository) ListPendingReissues(ctx context.Context, limit int) ([]SubscriptionReissue, error) {
	rows, err := sr.pool.Query(ctx, `SELECT id, telegram_id, old_panel_user_id, old_panel_user_uuid, new_link, delete_after, notified_at, last_notification_attempt_at, deleted_at FROM subscription_reissue WHERE (notified_at IS NULL AND (last_notification_attempt_at IS NULL OR last_notification_attempt_at <= NOW() - INTERVAL '5 minutes')) OR (deleted_at IS NULL AND delete_after <= NOW()) ORDER BY CASE WHEN deleted_at IS NULL AND delete_after <= NOW() THEN 0 ELSE 1 END, id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SubscriptionReissue, 0)
	for rows.Next() {
		var item SubscriptionReissue
		if err := rows.Scan(&item.ID, &item.TelegramID, &item.OldPanelUserID, &item.OldPanelUUID, &item.NewLink, &item.DeleteAfter, &item.NotifiedAt, &item.LastNotificationAttemptAt, &item.DeletedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (sr *SubscriptionRepository) MarkReissueNotified(ctx context.Context, id int64) error {
	_, err := sr.pool.Exec(ctx, `UPDATE subscription_reissue SET notified_at=NOW() WHERE id=$1 AND notified_at IS NULL`, id)
	return err
}

func (sr *SubscriptionRepository) MarkReissueNotificationAttempt(ctx context.Context, id int64) error {
	_, err := sr.pool.Exec(ctx, `UPDATE subscription_reissue SET last_notification_attempt_at=NOW() WHERE id=$1 AND notified_at IS NULL`, id)
	return err
}

func (sr *SubscriptionRepository) MarkReissueDeleted(ctx context.Context, id int64) error {
	_, err := sr.pool.Exec(ctx, `UPDATE subscription_reissue SET deleted_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id)
	return err
}
