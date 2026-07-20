package miniapp

import (
	"log/slog"
	"net/http"

	"link-bot/internal/database"
	"link-bot/internal/payment"
	"link-bot/utils"
)

type adminSuccessTestRequest struct {
	Text              string `json:"text"`
	Banner            string `json:"banner"`
	ButtonText        string `json:"buttonText"`
	IconCustomEmojiID string `json:"iconCustomEmojiId"`
	ButtonStyle       string `json:"buttonStyle"`
}

func (h *Handler) handleAdminSuccessTest(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.paymentService == nil {
		h.writeError(w, http.StatusServiceUnavailable, "payment_unavailable", "Payment messages are unavailable")
		return
	}

	var req adminSuccessTestRequest
	if err := h.decodeJSONRequest(w, r, 32<<10, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid success message preview")
		return
	}

	err := h.paymentService.SendSubscriptionActivatedPreview(r.Context(), &database.Customer{
		TelegramID: sess.User.ID,
		Language:   "ru",
	}, payment.SubscriptionActivatedPreviewOptions{
		Text:              req.Text,
		Banner:            req.Banner,
		ButtonText:        req.ButtonText,
		IconCustomEmojiID: req.IconCustomEmojiID,
		ButtonStyle:       req.ButtonStyle,
	})
	if err != nil {
		slog.Warn("mini app: send success message preview", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusBadRequest, "success_preview_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "success_preview_sent",
	})
}
