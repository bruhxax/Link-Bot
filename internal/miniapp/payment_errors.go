package miniapp

import (
	"errors"
	"net/http"

	"link-bot/internal/integrations"
)

func (h *Handler) writePayHotPurchaseError(w http.ResponseWriter, sess *session, err error) bool {
	var paymentErr *integrations.PayHotPaymentError
	if !errors.As(err, &paymentErr) {
		return false
	}
	message := paymentErr.PublicMessage()
	if sess.canAdmin("integrations") || sess.canAdmin("diagnostics") {
		message = paymentErr.AdminMessage()
	}
	status := http.StatusBadGateway
	if paymentErr.StatusCode == http.StatusUnprocessableEntity {
		status = http.StatusUnprocessableEntity
	}
	h.writeError(w, status, "payhot_payment_failed", message)
	return true
}
