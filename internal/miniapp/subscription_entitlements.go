package miniapp

import (
	"context"
	"time"

	"link-bot/internal/database"
)

type subscriptionEntitlementReader interface {
	DeviceLimitState(context.Context, int64, int64, time.Time) (int, int, error)
	TrafficLimitState(context.Context, int64, int64) (int64, int64, bool, error)
}

// Purchased extra devices/traffic change current limits without replacing the
// underlying plan. Compare that complete entitlement with the panel snapshot.
// This only changes presentation; it never repairs or writes panel limits.
func purchaseWithEntitlements(ctx context.Context, store subscriptionEntitlementReader, subscriptionID int64, purchase *database.Purchase) *database.Purchase {
	if store == nil || purchase == nil {
		return purchase
	}
	copy := *purchase
	if base, extra, err := store.DeviceLimitState(ctx, subscriptionID, 0, time.Now().UTC()); err == nil && extra > 0 {
		limit := base
		if base > 0 {
			limit += extra
		}
		copy.DeviceLimitCount = &limit
	}
	if base, extra, unlimited, err := store.TrafficLimitState(ctx, subscriptionID, 0); err == nil && (extra > 0 || unlimited) {
		limit := int64(0)
		if base > 0 && !unlimited {
			limit = base + extra
		}
		copy.TrafficLimitBytes = &limit
	}
	return &copy
}
