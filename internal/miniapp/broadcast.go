package miniapp

import (
	"errors"
	"net/http"

	"link-bot/internal/broadcast"
	"link-bot/internal/database"
)

type adminBroadcastButtonsRequest struct {
	Buttons []database.BroadcastButton `json:"buttons"`
}

func (h *Handler) handleAdminBroadcastState(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.requireBroadcastAdmin(w, sess) {
		return
	}
	draft, err := h.broadcastService.Get(r.Context())
	if err != nil {
		h.writeBroadcastError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": draft})
}

func (h *Handler) handleAdminBroadcastCaptureStart(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.requireBroadcastAdmin(w, sess) {
		return
	}
	draft, err := h.broadcastService.StartCapture(r.Context(), sess.User.ID)
	if err != nil {
		h.writeBroadcastError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "broadcast_capture_started", "data": draft})
}

func (h *Handler) handleAdminBroadcastButtons(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.requireBroadcastAdmin(w, sess) {
		return
	}
	var req adminBroadcastButtonsRequest
	if err := h.decodeJSONRequest(w, r, 32<<10, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный список кнопок")
		return
	}
	draft, err := h.broadcastService.SaveButtons(r.Context(), sess.User.ID, req.Buttons)
	if err != nil {
		h.writeBroadcastError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "broadcast_buttons_saved", "data": draft})
}

func (h *Handler) handleAdminBroadcastPreview(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.requireBroadcastAdmin(w, sess) {
		return
	}
	draft, err := h.broadcastService.Preview(r.Context(), sess.User.ID)
	if err != nil {
		h.writeBroadcastError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "broadcast_preview_sent", "data": draft})
}

func (h *Handler) handleAdminBroadcastSend(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.requireBroadcastAdmin(w, sess) {
		return
	}
	draft, err := h.broadcastService.Start(r.Context(), sess.User.ID)
	if err != nil {
		h.writeBroadcastError(w, err)
		return
	}
	h.writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "message": "broadcast_started", "data": draft})
}

func (h *Handler) handleAdminBroadcastReset(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.requireBroadcastAdmin(w, sess) {
		return
	}
	draft, err := h.broadcastService.Reset(r.Context(), sess.User.ID)
	if err != nil {
		h.writeBroadcastError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "broadcast_reset", "data": draft})
}

func (h *Handler) requireBroadcastAdmin(w http.ResponseWriter, sess *session) bool {
	if sess == nil || !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Недостаточно прав")
		return false
	}
	if h.broadcastService == nil {
		h.writeError(w, http.StatusServiceUnavailable, "broadcast_unavailable", "Рассылка временно недоступна")
		return false
	}
	return true
}

func (h *Handler) writeBroadcastError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, broadcast.ErrRunning):
		h.writeError(w, http.StatusConflict, "broadcast_running", "Рассылка уже запущена")
	case errors.Is(err, broadcast.ErrNoMessage):
		h.writeError(w, http.StatusBadRequest, "broadcast_message_required", "Сначала добавьте сообщение")
	case errors.Is(err, broadcast.ErrInvalidButton):
		h.writeError(w, http.StatusBadRequest, "broadcast_invalid_button", err.Error())
	default:
		h.writeError(w, http.StatusInternalServerError, "broadcast_failed", "Не удалось выполнить действие с рассылкой")
	}
}
