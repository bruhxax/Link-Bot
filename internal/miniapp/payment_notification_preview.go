package miniapp

import (
	"log/slog"
	"net/http"

	"link-bot/internal/database"
	"link-bot/internal/payment"
	"link-bot/internal/runtimeconfig"
	"link-bot/utils"
)

type adminPaymentNotificationTestRequest struct {
	Text           string                                       `json:"text"`
	OpenUserButton runtimeconfig.OptionalTelegramButtonSettings `json:"openUserButton"`
	ProfileButton  runtimeconfig.OptionalTelegramButtonSettings `json:"profileButton"`
}

func (h *Handler) handleAdminPaymentNotificationTest(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.paymentService == nil {
		h.writeError(w, http.StatusServiceUnavailable, "payment_unavailable", "Payment notifications are unavailable")
		return
	}

	var req adminPaymentNotificationTestRequest
	if err := h.decodeJSONRequest(w, r, 32<<10, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid payment notification preview")
		return
	}
	settings := runtimeconfig.DefaultSettings()
	settings.Content.PaymentNotification = runtimeconfig.TelegramPaymentNotificationSettings{
		Text:           req.Text,
		OpenUserButton: req.OpenUserButton,
		ProfileButton:  req.ProfileButton,
	}
	if err := runtimeconfig.NormalizeAndValidate(&settings); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_payment_notification", err.Error())
		return
	}

	err := h.paymentService.SendPaymentNotificationPreview(r.Context(), payment.PaymentNotificationPreviewOptions{
		Settings:   settings.Content.PaymentNotification,
		TelegramID: sess.User.ID,
		Username:   sess.User.Username,
		Customer:   customer,
	})
	if err != nil {
		slog.Warn("mini app: send payment notification preview", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusBadRequest, "payment_notification_preview_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "payment_notification_preview_sent",
	})
}
