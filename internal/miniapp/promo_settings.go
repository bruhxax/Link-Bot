package miniapp

import (
	"errors"
	"net/http"
	"time"

	"link-bot/internal/database"
)

func (h *Handler) handleAdminUpdatePromoSettings(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.canAdmin("promocodes") {
		h.writeError(w, 403, "forbidden", "Недостаточно прав")
		return
	}
	if h.promoCodeRepository == nil {
		h.writeError(w, 503, "promo_unavailable", "Промокоды недоступны")
		return
	}
	var req struct {
		ID             int64     `json:"id"`
		IsActive       *bool     `json:"isActive"`
		MaxRedemptions int       `json:"maxRedemptions"`
		ExpiresAt      string    `json:"expiresAt"`
		UpdatedAt      time.Time `json:"updatedAt"`
	}
	if h.decodeJSONRequest(w, r, 4096, &req) != nil || req.ID <= 0 || req.IsActive == nil || req.UpdatedAt.IsZero() || req.MaxRedemptions < 0 || req.MaxRedemptions > 2147483647 {
		h.writeError(w, 400, "invalid_request", "Проверьте настройки промокода")
		return
	}
	expiry, err := parsePromoExpiry(req.ExpiresAt)
	if err != nil {
		h.writeError(w, 400, "promo_invalid_expiry", "Проверьте дату окончания")
		return
	}
	var limit *int
	if req.MaxRedemptions > 0 {
		limit = &req.MaxRedemptions
	}
	err = h.promoCodeRepository.UpdateSettings(r.Context(), req.ID, *req.IsActive, limit, expiry, req.UpdatedAt)
	if errors.Is(err, database.ErrPromoCodePersonalLimit) {
		h.writeError(w, 400, "promo_personal_limit", "Персональный код можно использовать только один раз")
		return
	}
	if errors.Is(err, database.ErrPromoCodeChanged) {
		h.writeError(w, 409, "promo_changed", "Промокод изменился. Обновите список и откройте его заново")
		return
	}
	if err != nil {
		h.writeError(w, 500, "promo_update_failed", "Не удалось сохранить промокод")
		return
	}
	payload, err := h.buildAdminPayload(r.Context())
	if err != nil {
		h.writeError(w, 500, "promo_update_failed", "Настройки сохранены, но не удалось обновить список")
		return
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": payload})
}
