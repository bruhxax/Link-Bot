package miniapp

import (
	"errors"
	"net/http"

	"link-bot/internal/integrations"
)

func (h *Handler) writeIntegrationPurchaseError(w http.ResponseWriter, sess *session, err error) bool {
	var smerchErr *integrations.SmerchPaymentError
	if errors.As(err, &smerchErr) {
		message := smerchErr.PublicMessage()
		if sess.canAdmin("integrations") || sess.canAdmin("diagnostics") {
			message = smerchErr.AdminMessage()
		}
		h.writeError(w, http.StatusBadGateway, "smerch_payment_failed", message)
		return true
	}
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
