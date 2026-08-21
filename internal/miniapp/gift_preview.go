package miniapp

import (
	"log/slog"
	"net/http"

	"link-bot/internal/database"
	"link-bot/utils"
)

func (h *Handler) handleAdminGiftTest(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	var req giftTestRequest
	if err := h.decodeJSONRequest(w, r, 32<<10, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный тест подарка")
		return
	}
	if err := h.paymentService.SendGiftPreview(r.Context(), sess.User.ID, req.Kind, req.Message); err != nil {
		slog.Warn("mini app: send gift preview", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusBadRequest, "gift_preview_failed", err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "gift_preview_sent"})
}
