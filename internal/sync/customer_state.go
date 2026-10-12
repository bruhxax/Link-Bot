package sync

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

// RefreshCustomerSubscription gives the bot the same selected subscription as
// the cabinet, using a live identity read rather than last minute's cache.
func (s SyncService) RefreshCustomerSubscription(ctx context.Context, customer *database.Customer, primaryOnly bool) (*database.Customer, error) {
	if customer == nil || s.client == nil || s.stateRepository == nil {
		return customer, nil
	}
	subscription, err := s.stateRepository.ActiveSubscriptionForCustomer(ctx, customer, primaryOnly)
	if err != nil {
		return customer, err
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	id, identity := int64(0), uuid.Nil
	if subscription.PanelUserID != nil {
		id = *subscription.PanelUserID
	}
	if subscription.PanelUserUUID != nil {
		identity = *subscription.PanelUserUUID
	}
	var state *remnawave.UserState
	if id > 0 || identity != uuid.Nil {
		state, err = s.client.GetUserStateByIdentity(ctx, id, identity)
	} else if subscription.IsPrimary {
		state, err = s.client.GetUserStateByTelegramID(ctx, customer.TelegramID)
	}
	if err != nil {
		return customer, err
	}
	var link *string
	var expiry *time.Time
	if state != nil && state.Exists {
		id, identity, link, expiry = state.UserID, state.UserUUID, state.SubscriptionLink, state.ExpireAt
	}
	applied, err := s.stateRepository.SyncPanelObservation(ctx, database.PanelSyncCandidate{Customer: *customer, Subscription: *subscription}, id, identity, link, expiry)
	if err != nil {
		return customer, err
	}
	if !applied {
		return customer, fmt.Errorf("subscription changed during bot refresh")
	}
	view := *customer
	if !subscription.IsPrimary {
		view.TrialUsed = true
	}
	view.SubscriptionLink, view.ExpireAt = link, expiry
	// Preserve the real expiry in storage, but do not advertise disabled access.
	if state == nil || !state.Active {
		view.ExpireAt = nil
	}
	return &view, nil
}
