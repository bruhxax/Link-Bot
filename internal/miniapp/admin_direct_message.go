package miniapp

import (
	"errors"
	"log/slog"
	"net/http"

	"link-bot/internal/broadcast"
	"link-bot/internal/database"
)

func (h *Handler) directMessageRequest(w http.ResponseWriter, r *http.Request, sess *session) (int64, bool) {
	if !h.requireBroadcastAdmin(w, sess) {
		return 0, false
	}
	var req adminUserDetailRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil || req.CustomerID <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Выберите пользователя")
		return 0, false
	}
	return req.CustomerID, true
}

func (h *Handler) handleAdminDirectMessageState(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	customerID, ok := h.directMessageRequest(w, r, sess)
	if !ok {
		return
	}
	draft, err := h.broadcastService.GetDirect(r.Context(), sess.User.ID)
	if err != nil {
		h.writeDirectMessageError(w, err)
		return
	}
	if draft != nil && draft.CustomerID != customerID {
		draft = nil
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": draft})
}

func (h *Handler) handleAdminDirectMessageCapture(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	customerID, ok := h.directMessageRequest(w, r, sess)
	if !ok {
		return
	}
	draft, err := h.broadcastService.StartDirectCapture(r.Context(), sess.User.ID, customerID)
	if err != nil {
		h.writeDirectMessageError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": draft})
}

func (h *Handler) handleAdminDirectMessagePreview(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	customerID, ok := h.directMessageRequest(w, r, sess)
	if !ok {
		return
	}
	draft, err := h.broadcastService.PreviewDirect(r.Context(), sess.User.ID, customerID)
	if err != nil {
		h.writeDirectMessageError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": draft})
}

func (h *Handler) handleAdminDirectMessageSend(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	customerID, ok := h.directMessageRequest(w, r, sess)
	if !ok {
		return
	}
	draft, err := h.broadcastService.SendDirect(r.Context(), sess.User.ID, customerID)
	if err != nil {
		h.writeDirectMessageError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": draft})
}

func (h *Handler) writeDirectMessageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, broadcast.ErrDirectRecipient):
		h.writeError(w, http.StatusBadRequest, "recipient_unavailable", "У пользователя нет Telegram-чата с ботом")
	case errors.Is(err, broadcast.ErrNoMessage):
		h.writeError(w, http.StatusBadRequest, "message_required", "Сначала напишите сообщение боту")
	case errors.Is(err, broadcast.ErrDirectPreview):
		h.writeError(w, http.StatusBadRequest, "preview_required", "Сначала нажмите «Проверить»")
	case errors.Is(err, broadcast.ErrDirectState), errors.Is(err, broadcast.ErrRunning):
		h.writeError(w, http.StatusConflict, "direct_message_changed", "Черновик изменился или уже отправляется. Обновите карточку")
	default:
		slog.Error("direct message action failed", "error", err)
		h.writeError(w, http.StatusBadGateway, "direct_message_failed", "Не удалось отправить сообщение через Telegram. Проверьте чат и повторите")
	}
}
