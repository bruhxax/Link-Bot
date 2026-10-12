package payment

import (
	"context"
	"errors"

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
