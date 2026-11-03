package miniapp

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"link-bot/internal/database"
)

type adminReferralRequest struct {
	Mode       string `json:"mode"`
	Search     string `json:"search"`
	ReferrerID int64  `json:"referrerId"`
	Offset     int    `json:"offset"`
}

func (h *Handler) handleAdminReferrals(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if !sess.isAdministrator() || !sess.access().can("referrals") {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.referralRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "referrals_unavailable", "История рефералов недоступна")
		return
	}
	var req adminReferralRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	if req.Offset < 0 || req.Offset > 1000000 || req.ReferrerID < 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_page", "Некорректная страница")
		return
	}
	search := strings.TrimSpace(req.Search)
	if len(search) > 100 {
		h.writeError(w, http.StatusBadRequest, "invalid_search", "Слишком длинный поиск")
		return
	}
	const limit = 25
	if req.Mode == "history" {
		items, total, err := h.referralRepository.ListAdminHistory(r.Context(), search, req.ReferrerID, limit, req.Offset)
		if err != nil {
			slog.Error("admin referrals: list history", "error", err)
			h.writeError(w, http.StatusInternalServerError, "referrals_failed", "Не удалось загрузить историю")
			return
		}
		h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"items": items, "total": total, "offset": req.Offset, "limit": limit}})
		return
	}
	if req.Mode != "leaders" {
		h.writeError(w, http.StatusBadRequest, "invalid_mode", "Некорректный режим")
		return
	}
	items, total, err := h.referralRepository.ListAdminLeaders(r.Context(), search, limit, req.Offset)
	if err != nil {
		slog.Error("admin referrals: list leaders", "error", err)
		h.writeError(w, http.StatusInternalServerError, "referrals_failed", "Не удалось загрузить рефоводов")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"items": items, "total": total, "offset": req.Offset, "limit": limit}})
}
