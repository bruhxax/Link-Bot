package database

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"time"
)

type PanelSyncCandidate struct {
	Customer     Customer
	Subscription CustomerSubscription
}

func (cr *CustomerRepository) ActiveSubscriptionForCustomer(ctx context.Context, customer *Customer, primaryOnly bool) (*CustomerSubscription, error) {
	if primaryOnly {
		return NewSubscriptionRepository(cr.pool).EnsurePrimary(ctx, customer)
	}
	return NewSubscriptionRepository(cr.pool).ActiveForCustomer(ctx, customer)
}

// Take the local snapshot before requesting the remote catalog, so concurrent
// purchases/rewards can invalidate it rather than being overwritten afterwards.
func (cr *CustomerRepository) PanelSyncCandidates(ctx context.Context) ([]PanelSyncCandidate, error) {
	rows, err := cr.pool.Query(ctx, `SELECT c.id, c.telegram_id, c.expire_at, c.subscription_link,
		s.id, s.is_primary, s.panel_user_id, s.panel_user_uuid, s.subscription_link, s.expire_at, s.updated_at
		FROM customer c JOIN customer_subscription s ON s.customer_id = c.id`)
	if err != nil {
		return nil, fmt.Errorf("load panel sync candidates: %w", err)
	}
	defer rows.Close()
	items := []PanelSyncCandidate{}
	for rows.Next() {
		var item PanelSyncCandidate
		if err := rows.Scan(&item.Customer.ID, &item.Customer.TelegramID, &item.Customer.ExpireAt, &item.Customer.SubscriptionLink,
			&item.Subscription.ID, &item.Subscription.IsPrimary, &item.Subscription.PanelUserID, &item.Subscription.PanelUserUUID, &item.Subscription.SubscriptionLink, &item.Subscription.ExpireAt, &item.Subscription.UpdatedAt); err != nil {
			return nil, err
		}
		item.Subscription.CustomerID = item.Customer.ID
		items = append(items, item)
	}
	return items, rows.Err()
}

func (cr *CustomerRepository) SyncPanelObservation(ctx context.Context, candidate PanelSyncCandidate, userID int64, userUUID uuid.UUID, link *string, expire *time.Time) (bool, error) {
	return NewSubscriptionRepository(cr.pool).SyncPanelAccess(ctx, &candidate.Customer, &candidate.Subscription, userID, userUUID, link, expire)
}
