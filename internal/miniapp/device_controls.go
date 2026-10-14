package miniapp

import (
	"errors"
	"net/http"
	"strings"

	"link-bot/internal/database"
	"link-bot/internal/devicequota"
)

type deviceControlRequest struct {
	SubscriptionID int64  `json:"subscriptionId"`
	Hwid           string `json:"hwid"`
	Action         string `json:"action"`
	TrafficGB      *int64 `json:"trafficGb"`
}

func (h *Handler) handleControlDevice(w http.ResponseWriter, r *http.Request, _ *session, customer *database.Customer) {
	if h.remnawaveClient == nil {
		h.writeError(w, 503, "devices_unavailable", "Устройства временно недоступны")
		return
	}
	var req deviceControlRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil || req.SubscriptionID <= 0 || strings.TrimSpace(req.Hwid) == "" {
		h.writeError(w, 400, "invalid_request", "Выберите устройство и подписку")
		return
	}
	if req.Action != "pause" && req.Action != "resume" && req.Action != "limit" && req.Action != "unblock" {
		h.writeError(w, 400, "invalid_action", "Неизвестное действие")
		return
	}
	if req.Action == "limit" && (req.TrafficGB == nil || *req.TrafficGB < 0 || *req.TrafficGB > 1000000) {
		h.writeError(w, 400, "invalid_limit", "Укажите лимит 0–1000000 GB. 0 — без лимита")
		return
	}
	active, _, err := h.loadCustomerSubscriptions(r.Context(), customer)
	if err != nil {
		h.writeError(w, 503, "subscription_unavailable", "Не удалось определить подписку")
		return
	}
	if active == nil || active.CustomerID != customer.ID || active.ID != req.SubscriptionID {
		h.writeError(w, 403, "device_subscription_mismatch", "Устройство не относится к выбранной подписке")
		return
	}
	// Resolve panel identity server-side from the authenticated customer's owned
	// subscription. A caller cannot choose another panel user or technical child.
	state, err := h.panelStateForCustomerSubscription(r.Context(), customer, active)
	if err != nil {
		h.writeError(w, 503, "devices_unavailable", "Не удалось получить данные устройств")
		return
	}
	if state == nil || !state.DeviceTrafficManaged {
		h.writeError(w, 403, "device_management_disabled", "Управление устройствами для подписки не включено")
		return
	}
	err = h.remnawaveClient.ControlDevice(r.Context(), state.UserID, state.UserUUID, strings.TrimSpace(req.Hwid), req.Action, req.TrafficGB)
	if err != nil {
		code, message := 503, "Не удалось изменить устройство. Повторите запрос"
		if errors.Is(err, devicequota.ErrAdminLimit) {
			code = 403
			message = devicequota.ErrAdminLimit.Error()
		}
		if errors.Is(err, devicequota.ErrDeviceMissing) {
			code = 404
			message = "Устройство не найдено"
		}
		if errors.Is(err, devicequota.ErrBusy) {
			code = 409
			message = "Устройства обновляются. Повторите запрос"
		}
		h.writeError(w, code, "device_control_failed", message)
		return
	}
	payload, err := h.loadLiveSubscriptionState(r.Context(), customer)
	if err != nil || payload.Subscriptions.ActiveID != req.SubscriptionID {
		h.writeError(w, 409, "subscription_state_changed", "Подписка изменилась. Обновите данные")
		return
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": payload.Subscription})
}
