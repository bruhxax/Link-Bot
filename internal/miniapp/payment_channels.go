package miniapp

import (
	"context"
	"net/http"
	"time"

	"link-bot/internal/database"
	"link-bot/internal/paymentchannel"
	"link-bot/internal/runtimeconfig"
)

func publicPaymentChannels() map[string]paymentchannel.Catalog {
	result := map[string]paymentchannel.Catalog{}
	for _, method := range runtimeconfig.DefaultPaymentMethodOrder() {
		result[method] = paymentchannel.For(method)
	}
	return result
}

func (h *Handler) handleAdminPaymentChannels(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.isAdministrator() {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Method string `json:"method"`
	}
	if err := h.decodeJSONRequest(w, r, 1024, &req); err != nil {
		h.writeError(w, 400, "invalid_request", "Некорректный запрос")
		return
	}
	method := req.Method
	if runtimeconfig.DefaultSettings().PaymentMethodBase(method) == "" {
		h.writeError(w, 400, "invalid_method", "Неизвестная платёжка")
		return
	}
	catalog := paymentchannel.For(method)
	if h.paymentService != nil && len(catalog.Options) > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		var err error
		catalog, err = h.paymentService.PaymentChannels(ctx, method)
		if err != nil {
			catalog.Note = "Не удалось обновить список из кассы. Показаны методы API; проверьте их подключение в платёжной системе."
		}
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": catalog})
}
