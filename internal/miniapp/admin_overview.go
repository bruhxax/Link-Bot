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
	data := &database.AdminOverview{Days: req.Days, GeneratedAt: now.UTC(), Subscriptions: []database.OverviewPlan{}}
	if sess.canAdmin("finance") && h.customerRepository != nil {
		cached, pending, err := h.cachedRevenue(r.Context(), req.Force)
		data.Pending = pending
		if cached != nil {
			data.Revenue = cached.Revenue
		}
		if err != nil {
			data.RevenueError = "Не удалось обновить доходы. Повторите обновление."
		}
	}
	if sess.canAdmin("users") {
		catalog, pending, err := h.requestPanelCatalog(r.Context(), req.Force, "")
		data.Pending = data.Pending || pending
		if err != nil {
			data.PanelError = "Не удалось обновить пользователей панели. Повторите обновление."
		}
		if catalog.Users != nil {
			h.overviewPanel.mu.Lock()
			planErr := h.overviewPanel.planErr
			h.overviewPanel.mu.Unlock()
			if planErr != nil {
				data.PanelError = "Пользователи загружены, но тарифы не удалось обновить. Повторите обновление."
			}
			settings := runtimeconfig.DefaultSettings()
			if h.runtimeSettings != nil {
				settings = h.runtimeSettings.Snapshot()
			}
			data.Users, data.Subscriptions = panelOverview(catalog.Users, catalog.Observations, settings, req.Days, now)
		} else if h.remnawaveClient != nil {
			h.overviewPanel.mu.Lock()
			counts := h.overviewPanel.counts
			h.overviewPanel.mu.Unlock()
			if counts != nil {
				u := &database.OverviewUsers{Source: "panel", Total: int(counts.TotalUsers), Active: int(counts.StatusCounts["ACTIVE"]), Expired: int(counts.StatusCounts["EXPIRED"]), Limited: int(counts.StatusCounts["LIMITED"]), Disabled: int(counts.StatusCounts["DISABLED"])}
				u.Other = max(0, u.Total-u.Active-u.Expired-u.Limited-u.Disabled)
				data.Users = u
			}
		}
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data})
}
