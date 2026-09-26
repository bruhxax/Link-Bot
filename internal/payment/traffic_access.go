package payment

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

func trafficLimitWithGrants(base, extra int64, unlimited bool) int64 {
	if base <= 0 || unlimited {
		return 0
	}
	return base + extra
}

func (s PaymentService) prepareSubscriptionTraffic(ctx context.Context, customer *database.Customer, subscription *database.CustomerSubscription, purchase *database.Purchase, panelState *remnawave.UserState) (int64, int64, error) {
	initial := int64(purchaseTrafficLimit(purchase))
	if panelState != nil && panelState.Exists {
		initial = maxInt64(panelState.TrafficLimitBytes, 0)
	}
	if err := s.purchaseRepository.EnsureTrafficBase(ctx, subscription.ID, initial); err != nil {
		return 0, 0, err
	}
	base, extra, unlimited, err := s.purchaseRepository.TrafficLimitState(ctx, subscription.ID, purchase.ID)
	if err != nil {
		return 0, 0, err
	}
	if purchase.PurchaseKind != database.PurchaseKindExtraTraffic && purchase.PurchaseKind != database.PurchaseKindExtraDevices {
		previousBase := base
		base = int64(purchaseTrafficLimit(purchase))
		if shouldAccumulateEntitlements(customerForSubscription(customer, subscription), panelState) {
			base = int64(mergeTrafficLimits(int(maxInt64(previousBase, 0)), int(base)))
		}
	}
	if extra > 0 && base > 0 && extra > int64(^uint64(0)>>1)-base {
		return 0, 0, errors.New("resulting traffic limit is too large")
	}
	return base, trafficLimitWithGrants(base, extra, unlimited), nil
}

func (s PaymentService) applySubscriptionTrafficLimit(ctx context.Context, customer *database.Customer, subscription *database.CustomerSubscription, limit int64) (*remnawave.PanelUser, error) {
	userID, userUUID := subscriptionPanelIdentity(subscription)
	if userID <= 0 && userUUID == uuid.Nil && !subscription.IsPrimary {
		return nil, errors.New("subscription is not active yet")
	}
	return s.remnawaveClient.SetTrafficLimit(ctx, customer.TelegramID, userID, userUUID, limit)
}

// Reconcile purchased traffic after a panel reset or other external update.
func (s PaymentService) ProcessTrafficEntitlements(ctx context.Context) error {
	if s.purchaseRepository == nil || s.subscriptionRepository == nil || s.remnawaveClient == nil {
		return nil
	}
	ids, err := s.purchaseRepository.TrafficGrantSubscriptions(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.processTrafficSubscription(ctx, id); err != nil {
			slog.Error("traffic access: reconciliation failed", "subscription_id", id, "error", err)
		}
	}
	return nil
}

func (s PaymentService) processTrafficSubscription(ctx context.Context, subscriptionID int64) error {
	release, err := s.purchaseRepository.LockDeviceLimit(ctx, subscriptionID)
	if err != nil {
		return err
	}
	defer release()
	subscription, err := s.subscriptionRepository.FindByID(ctx, subscriptionID)
	if err != nil || subscription == nil {
		return err
	}
	customer, err := s.customerRepository.FindById(ctx, subscription.CustomerID)
	if err != nil || customer == nil {
		return err
	}
	state, err := s.panelStateForSubscription(ctx, customer, subscription)
	if err != nil || state == nil || !state.Exists || !state.Active {
		return err
	}
	base, extra, unlimited, err := s.purchaseRepository.TrafficLimitState(ctx, subscriptionID, 0)
	if err != nil {
		return err
	}
	limit := trafficLimitWithGrants(base, extra, unlimited)
	if state.TrafficLimitBytes != limit || state.TrafficLimitStrategy != "NO_RESET" {
		user, err := s.applySubscriptionTrafficLimit(ctx, customer, subscription, limit)
		if err != nil {
			return err
		}
		return s.persistSubscriptionPanelState(ctx, customer, subscription, user)
	}
	return nil
}
