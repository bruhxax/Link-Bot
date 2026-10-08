package miniapp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
)

type liveSubscriptionPayload struct {
	PanelUsername string               `json:"panelUsername"`
	Subscription  subscriptionPayload  `json:"subscription"`
	Subscriptions subscriptionsPayload `json:"subscriptions"`
	Trial         trialPayload         `json:"trial"`
}

// Poll only the selected subscription. Catalogs, reviews, support, payment
// history and administrator settings stay on bootstrap/committed-change refresh.
func (h *Handler) handleSubscriptionState(w http.ResponseWriter, r *http.Request, _ *session, customer *database.Customer) {
	payload, err := h.loadLiveSubscriptionState(r.Context(), customer)
	if err != nil {
		status, code := http.StatusServiceUnavailable, "panel_state_unavailable"
		if errors.Is(err, errSubscriptionObservationChanged) {
			status, code = http.StatusConflict, "subscription_state_changed"
		}
		h.writeError(w, status, code, "Не удалось обновить подписку. Повторите запрос")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": payload})
}

func (h *Handler) loadLiveSubscriptionState(ctx context.Context, customer *database.Customer) (*liveSubscriptionPayload, error) {
	settings := runtimeconfig.DefaultSettings()
	if h.runtimeSettings != nil {
		settings = h.runtimeSettings.Snapshot()
	}
	active, subscriptions, err := h.loadCustomerSubscriptions(ctx, customer)
	if err != nil {
		return nil, err
	}
	if !settings.Features["additional_subscriptions"] {
		active, subscriptions = primarySubscriptionOnly(active, subscriptions)
	}
	viewCustomer := customerForActiveSubscription(customer, active)
	var state *remnawave.UserState
	if h.remnawaveClient != nil {
		panelCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		state, err = h.panelStateForCustomerSubscription(panelCtx, customer, active)
		if err != nil {
			return nil, err
		}
		viewCustomer, err = h.syncCustomerSubscriptionState(ctx, customer, active, state)
		if err != nil {
			return nil, err
		}
		h.trackDeviceNotifications(ctx, customer, active, state)
	}
	var purchase *database.Purchase
	if h.purchaseRepository != nil {
		purchase, err = h.purchaseRepository.FindLatestSuccessfulPurchaseBySubscription(ctx, customer.ID, active.ID)
		if err != nil {
			return nil, err
		}
		if state != nil && state.Exists && purchase != nil && !h.purchaseMatchesPanelState(purchase, state) {
			purchase = purchaseWithEntitlements(ctx, h.purchaseRepository, active.ID, purchase)
		}
	}
	trial := settings.Trial
	eligible := active.IsPrimary && bootstrapTrialEligible(trial, viewCustomer, purchase, state, false, false)
	if h.customerRepository != nil {
		email, err := h.customerRepository.EmailForCustomer(ctx, customer.ID)
		if err != nil {
			return nil, err
		}
		if emailTrialNeedsTelegram(customer, email) {
			eligible = false
		}
	}
	payload := &liveSubscriptionPayload{
		Subscription:  h.buildSubscriptionPayload(viewCustomer, purchase, state),
		Subscriptions: buildSubscriptionsPayload(active, subscriptions),
		Trial:         trialPayload{Enabled: settings.Features["trials"] && trial.Enabled && trial.Days > 0, Eligible: settings.Features["trials"] && eligible, Days: trial.Days},
	}
	if state != nil {
		payload.PanelUsername = state.PanelUsername
	}
	payload.Subscription.StateLoaded = true
	return payload, nil
}
