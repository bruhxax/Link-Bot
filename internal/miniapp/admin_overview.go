package miniapp

import (
	"net/http"
	"time"

	"link-bot/internal/database"
)

func (h *Handler) handleAdminOverview(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.isAdministrator() {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	var req struct {
		Days int `json:"days"`
	}
	if err := h.decodeJSONRequest(w, r, 1024, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный период")
		return
	}
	if req.Days == 0 {
		req.Days = 30
	}
	if req.Days != 7 && req.Days != 30 && req.Days != 90 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Выберите период 7, 30 или 90 дней")
		return
	}
	data, err := h.customerRepository.LoadAdminOverview(r.Context(), req.Days, time.Now(), sess.canAdmin("users"), sess.canAdmin("finance"))
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "admin_overview_failed", "Не удалось загрузить статистику")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data})
}
