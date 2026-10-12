package miniapp

import (
	"net/http"
	"time"

	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

func (h *Handler) handleAdminOverview(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.isAdministrator() {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	var req struct {
		Days  int  `json:"days"`
		Force bool `json:"force"`
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
	now := time.Now()
	data, err := h.customerRepository.LoadAdminOverview(r.Context(), req.Days, now, false, sess.canAdmin("finance"))
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "admin_overview_failed", "Не удалось загрузить статистику")
		return
	}
	if sess.canAdmin("users") {
		users, panelErr := h.overviewPanelUsers(r.Context(), req.Force)
		if panelErr != nil {
			data.PanelError = "Не удалось загрузить пользователей панели. Повторите обновление."
		} else {
			var observations []database.PanelPlanObservation
			if h.customerRepository != nil {
				observations, err = h.customerRepository.PanelPlanObservations(r.Context())
			}
			if err != nil {
				data.PanelError = "Не удалось определить тарифы подписок. Повторите обновление."
			} else {
				settings := runtimeconfig.DefaultSettings()
				if h.runtimeSettings != nil {
					settings = h.runtimeSettings.Snapshot()
				}
				data.Users, data.Subscriptions = panelOverview(users, observations, settings, req.Days, now)
			}
		}
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data})
}
