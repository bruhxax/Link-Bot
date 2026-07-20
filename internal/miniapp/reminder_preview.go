package miniapp

import (
	"log/slog"
	"net/http"

	"link-bot/internal/database"
	"link-bot/internal/notification"
	"link-bot/utils"
)

type adminReminderTestRequest struct {
	Kind              string `json:"kind"`
	Template          string `json:"template"`
	ButtonText        string `json:"buttonText"`
	IconCustomEmojiID string `json:"iconCustomEmojiId"`
	ButtonStyle       string `json:"buttonStyle"`
}

func (h *Handler) handleAdminReminderTest(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.subscriptionService == nil {
		h.writeError(w, http.StatusServiceUnavailable, "reminders_unavailable", "Subscription reminders are unavailable")
		return
	}

	var req adminReminderTestRequest
	if err := h.decodeJSONRequest(w, r, 32<<10, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid reminder preview")
		return
	}

	err := h.subscriptionService.SendTestReminder(r.Context(), sess.User.ID, notification.TestReminderOptions{
		Kind:              req.Kind,
		Template:          req.Template,
		ButtonText:        req.ButtonText,
		IconCustomEmojiID: req.IconCustomEmojiID,
		ButtonStyle:       req.ButtonStyle,
	})
	if err != nil {
		slog.Warn("mini app: send reminder preview", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusBadRequest, "reminder_preview_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "reminder_preview_sent",
	})
}
