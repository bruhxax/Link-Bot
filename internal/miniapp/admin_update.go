package miniapp

import (
	"context"
	"net/http"
	"time"

	"link-bot/internal/database"
	"link-bot/internal/updater"
)

func (h *Handler) handleAdminBotUpdateStatus(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.access().IsOwner {
		h.writeError(w, 403, "forbidden", "Access denied")
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	state, err := (updater.Client{}).Request(ctx, false)
	if err != nil {
		state.Message = err.Error()
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": state})
}

func (h *Handler) handleAdminBotUpdate(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	// Status permission lets delegated administrators read health, not execute
	// software as root. Installation updates belong to the owner exclusively.
	if !sess.access().IsOwner {
		h.writeError(w, 403, "forbidden", "Обновлять бота может только главный администратор")
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := h.decodeJSONRequest(w, r, 1024, &req); err != nil || !req.Confirm {
		h.writeError(w, 400, "confirmation_required", "Подтвердите обновление бота")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	state, err := (updater.Client{}).Request(ctx, true)
	if err != nil {
		h.writeError(w, 503, "updater_unavailable", err.Error())
		return
	}
	h.writeJSON(w, 202, map[string]any{"ok": true, "data": state})
}
