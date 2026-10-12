package handler

import (
	"context"
	"log/slog"

	"link-bot/internal/database"
)

func (h Handler) refreshSubscriptionForBot(ctx context.Context, customer *database.Customer) *database.Customer {
	if h.syncService == nil || customer == nil {
		return customer
	}
	view, err := h.syncService.RefreshCustomerSubscription(ctx, customer, !h.featureEnabled("additional_subscriptions"))
	if err != nil {
		slog.Warn("bot: subscription refresh failed", "customer_id", customer.ID, "error", err)
		return customer
	}
	return view
}
