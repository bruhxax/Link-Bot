package miniapp

import (
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"net/http"
)

func (h *Handler) handleAdminUserSettings(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.canAdmin("users.subscription") {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	var req struct {
		CustomerID     int64                   `json:"customerId"`
		SubscriptionID int64                   `json:"subscriptionId"`
		Settings       *remnawave.UserSettings `json:"settings"`
	}
	if err := h.decodeJSONRequest(w, r, 65536, &req); err != nil || req.CustomerID <= 0 || req.SubscriptionID <= 0 || req.Settings == nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Выберите подписку и укажите настройки")
		return
	}
	if err := req.Settings.Validate(); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_settings", err.Error())
		return
	}
	target, subscription, err := h.adminUserSubscriptionTarget(r.Context(), req.CustomerID, req.SubscriptionID)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "admin_subscription_unavailable", err.Error())
		return
	}
	updated, err := h.remnawaveClient.UpdateAdminUserSettings(r.Context(), target.state.UserID, target.state.UserUUID, *req.Settings)
	if err != nil {
		h.writeError(w, http.StatusBadGateway, "admin_settings_failed", "Не удалось сохранить настройки в панели")
		return
	}
	if err := h.persistAdminSubscriptionPanelState(r.Context(), subscription, updated); err != nil {
		h.writeError(w, http.StatusInternalServerError, "admin_subscription_sync_failed", "Настройки изменены, но локальные данные не обновились. Обновите карточку перед повторным сохранением.")
		return
	}
	h.writeAdminUserActionResult(w, r, req.CustomerID, "Настройки пользователя сохранены")
}
