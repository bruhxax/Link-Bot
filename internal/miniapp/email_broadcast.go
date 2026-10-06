package miniapp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"link-bot/internal/database"
)

type emailBroadcastSaveRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type emailBroadcastPreviewRequest struct {
	Email string `json:"email"`
}

func (h *Handler) requireEmailBroadcastAdmin(w http.ResponseWriter, sess *session) bool {
	if sess == nil || !sess.isAdministrator() {
		h.writeError(w, http.StatusForbidden, "forbidden", "Недостаточно прав")
		return false
	}
	return true
}

func (h *Handler) handleAdminEmailBroadcastState(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.requireEmailBroadcastAdmin(w, sess) {
		return
	}
	draft, err := h.customerRepository.EmailBroadcastDraft(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_broadcast_failed", "Не удалось загрузить рассылку")
		return
	}
	_, configured := h.emailSMTPSettings()
	recipients, err := h.customerRepository.EmailBroadcastRecipients(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_broadcast_failed", "Не удалось посчитать адреса")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"draft": draft, "configured": configured, "availableRecipients": len(recipients)}})
}

func (h *Handler) handleAdminEmailBroadcastSave(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.requireEmailBroadcastAdmin(w, sess) {
		return
	}
	var request emailBroadcastSaveRequest
	if err := h.decodeJSONRequest(w, r, 32<<10, &request); err != nil {
		return
	}
	request.Subject = strings.TrimSpace(request.Subject)
	request.Body = strings.TrimSpace(request.Body)
	if len([]rune(request.Subject)) < 2 || len([]rune(request.Subject)) > 160 || strings.ContainsAny(request.Subject, "\r\n") ||
		len([]rune(request.Body)) < 2 || len([]rune(request.Body)) > 20000 {
		h.writeError(w, http.StatusBadRequest, "email_broadcast_invalid", "Укажите тему до 160 символов и текст до 20 000 символов")
		return
	}
	draft, err := h.customerRepository.SaveEmailBroadcastDraft(r.Context(), request.Subject, request.Body, sess.User.ID)
	if errors.Is(err, database.ErrEmailBroadcastRunning) {
		h.writeError(w, http.StatusConflict, "email_broadcast_running", "Рассылка уже запущена")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_broadcast_failed", "Не удалось сохранить письмо")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": draft})
}

func (h *Handler) handleAdminEmailBroadcastPreview(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.requireEmailBroadcastAdmin(w, sess) {
		return
	}
	var request emailBroadcastPreviewRequest
	if err := h.decodeJSONRequest(w, r, 2048, &request); err != nil {
		return
	}
	email, err := normalizeAuthEmail(request.Email)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_email", "Укажите адрес для предпросмотра")
		return
	}
	settings, configured := h.emailSMTPSettings()
	if !configured {
		h.writeError(w, http.StatusServiceUnavailable, "email_not_configured", "SMTP не настроен")
		return
	}
	draft, err := h.customerRepository.EmailBroadcastDraft(r.Context())
	if err != nil || draft.Subject == "" || draft.Body == "" {
		h.writeError(w, http.StatusBadRequest, "email_broadcast_empty", "Сначала сохраните письмо")
		return
	}
	if err = sendSMTPMessage(r.Context(), settings, email, draft.Subject, draft.Body); err != nil {
		slog.Error("email broadcast preview failed", "error", err)
		h.writeError(w, http.StatusBadGateway, "email_delivery_failed", "Не удалось отправить предпросмотр")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": draft})
}

func (h *Handler) handleAdminEmailBroadcastSend(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.requireEmailBroadcastAdmin(w, sess) {
		return
	}
	settings, configured := h.emailSMTPSettings()
	if !configured {
		h.writeError(w, http.StatusServiceUnavailable, "email_not_configured", "SMTP не настроен")
		return
	}
	recipients, err := h.customerRepository.EmailBroadcastRecipients(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_broadcast_failed", "Не удалось загрузить адреса")
		return
	}
	if len(recipients) == 0 {
		h.writeError(w, http.StatusBadRequest, "email_broadcast_empty", "Пока нет пользователей с привязанной почтой")
		return
	}
	draft, err := h.customerRepository.BeginEmailBroadcast(r.Context(), len(recipients), sess.User.ID)
	if errors.Is(err, database.ErrEmailBroadcastRunning) {
		h.writeError(w, http.StatusConflict, "email_broadcast_running", "Сохраните письмо или дождитесь завершения рассылки")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_broadcast_failed", "Не удалось запустить рассылку")
		return
	}
	go h.runEmailBroadcast(settings, recipients, draft.Subject, draft.Body)
	h.writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "data": draft})
}

func (h *Handler) runEmailBroadcast(settings emailSMTPSettings, recipients []string, subject, body string) {
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	sent, failed := 0, 0
	lastError := ""
	for _, recipient := range recipients {
		if ctx.Err() != nil {
			break
		}
		if err := sendSMTPMessage(ctx, settings, recipient, subject, body); err != nil {
			failed++
			lastError = "Некоторые письма не доставлены. Проверьте SMTP и лимиты почтового сервиса."
			slog.Warn("email broadcast delivery failed", "error", err)
		} else {
			sent++
		}
		if err := h.customerRepository.UpdateEmailBroadcastProgress(ctx, sent, failed, lastError); err != nil {
			slog.Error("email broadcast progress update failed", "error", err)
		}
		select {
		case <-ctx.Done():
			break
		case <-time.After(200 * time.Millisecond):
		}
	}
	status := "finished"
	if ctx.Err() != nil {
		status = "failed"
		lastError = "Время рассылки истекло"
	}
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finishCancel()
	if err := h.customerRepository.FinishEmailBroadcast(finishCtx, status, sent, failed, lastError); err != nil {
		slog.Error("email broadcast finish failed", "error", err)
	}
}
