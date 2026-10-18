package miniapp

import (
	"errors"
	"net/http"
	"strings"

	"link-bot/internal/broadcast"
	"link-bot/internal/database"
)

func (h *Handler) handleAdminPinnedMessage(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if sess == nil || !sess.canAdmin("content") {
		h.writeError(w, http.StatusForbidden, "forbidden", "Недостаточно прав")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Используйте POST")
		return
	}
	if h.broadcastService == nil {
		h.writeError(w, http.StatusServiceUnavailable, "unavailable", "Telegram недоступен")
		return
	}
	var p *database.PinnedMessage
	var err error
	switch strings.TrimPrefix(r.URL.Path, "/api/mini-app/admin/content/pinned/") {
	case "state":
		p, err = h.broadcastService.PinnedState(r.Context())
	case "capture":
		p, err = h.broadcastService.StartPinnedCapture(r.Context(), sess.User.ID)
	case "cancel":
		p, err = h.broadcastService.CancelPinnedCapture(r.Context(), sess.User.ID)
	case "toggle":
		var req struct {
			Enabled  *bool `json:"enabled"`
			Revision int64 `json:"revision"`
		}
		if h.decodeJSONRequest(w, r, 2048, &req) != nil {
			return
		}
		if req.Enabled == nil || req.Revision <= 0 {
			h.writeError(w, http.StatusBadRequest, "invalid_request", "Укажите состояние закрепа")
			return
		}
		p, err = h.broadcastService.SetPinnedEnabled(r.Context(), *req.Enabled, req.Revision)
	case "preview":
		p, err = h.broadcastService.PreviewPinnedMessage(r.Context(), sess.User.ID)
	case "retry":
		p, err = h.broadcastService.RetryPinnedMessages(r.Context())
	default:
		h.writeError(w, http.StatusNotFound, "not_found", "Действие не найдено")
		return
	}
	if err != nil {
		if errors.Is(err, database.ErrPinnedStateChanged) {
			h.writeError(w, http.StatusConflict, "pinned_changed", "Состояние изменилось. Обновите редактор; возможно, сообщение уже вводит другой администратор.")
			return
		}
		if errors.Is(err, broadcast.ErrRunning) {
			h.writeError(w, http.StatusConflict, "capture_busy", "Сначала завершите ввод сообщения для рассылки или личного сообщения.")
			return
		}
		if errors.Is(err, broadcast.ErrNoMessage) {
			h.writeError(w, http.StatusBadRequest, "no_message", "Сначала напишите сообщение для закрепления")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "pinned_failed", "Не удалось выполнить действие. Проверьте сообщение и повторите попытку.")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": p})
}
