package miniapp

import (
	"encoding/json"
	"github.com/jackc/pgx/v4"
	"link-bot/internal/broadcast"
	"link-bot/internal/database"
	"net/http"
	"strings"
	"time"
)

func (h *Handler) handleBroadcastTemplates(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.canAdmin("broadcast") || h.customerRepository == nil {
		h.writeError(w, 403, "forbidden", "Недостаточно прав")
		return
	}
	var req struct {
		Action    string                     `json:"action"`
		ID        int64                      `json:"id"`
		Channel   string                     `json:"channel"`
		Name      string                     `json:"name"`
		UpdatedAt time.Time                  `json:"updatedAt"`
		Audience  database.BroadcastAudience `json:"audience"`
	}
	if err := h.decodeJSONRequest(w, r, 16384, &req); err != nil {
		h.writeError(w, 400, "invalid_request", "Некорректный запрос шаблона")
		return
	}
	if req.Action == "list" {
		items, err := h.customerRepository.BroadcastTemplates(r.Context())
		if err != nil {
			h.writeError(w, 500, "templates_failed", "Не удалось загрузить шаблоны")
			return
		}
		h.writeJSON(w, 200, map[string]any{"ok": true, "data": items})
		return
	}
	if req.Action == "delete" && req.ID > 0 {
		if err := h.customerRepository.DeleteBroadcastTemplate(r.Context(), req.ID); err != nil {
			h.writeError(w, 500, "templates_failed", "Не удалось удалить шаблон")
			return
		}
		h.writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if (req.Channel != "telegram" && req.Channel != "email") || req.UpdatedAt.IsZero() {
		h.writeError(w, 400, "invalid_template", "Обновите черновик перед применением шаблона")
		return
	}
	if req.Action == "save" {
		req.Name = strings.TrimSpace(req.Name)
		if len([]rune(req.Name)) < 1 || len([]rune(req.Name)) > 80 {
			h.writeError(w, 400, "invalid_template", "Укажите название до 80 символов")
			return
		}
		if err := req.Audience.Normalize(); err != nil {
			h.writeError(w, 400, "invalid_template", err.Error())
			return
		}
		p := database.BroadcastTemplatePayload{Audience: req.Audience}
		if req.Channel == "telegram" {
			if h.broadcastService == nil {
				h.writeError(w, 503, "broadcast_unavailable", "Рассылка недоступна")
				return
			}
			d, err := h.broadcastService.Get(r.Context())
			if err != nil || d == nil {
				h.writeError(w, 500, "templates_failed", "Не удалось прочитать сообщение")
				return
			}
			if !d.UpdatedAt.Equal(req.UpdatedAt) || d.Status == "running" {
				h.writeError(w, 409, "broadcast_draft_changed", "Черновик изменился. Обновите страницу")
				return
			}
			if !d.HasSource() {
				h.writeError(w, 400, "broadcast_message_required", "Сначала добавьте сообщение")
				return
			}
			if d.Survey != nil && d.Survey.Capture != nil {
				h.writeBroadcastError(w, broadcast.ErrSurveyCapturePending)
				return
			}
			p.SourceChatID = d.SourceChatID
			p.SourceMessageID = d.SourceMessageID
			p.SourceKind = d.SourceKind
			p.SourcePreview = d.SourcePreview
			p.SourceHTML = d.SourceHTML
			p.Buttons = d.Buttons
			p.DeleteAfterMinutes = d.DeleteAfterMinutes
			p.Survey = d.Survey
		} else {
			d, err := h.customerRepository.EmailBroadcastDraft(r.Context())
			if err != nil {
				h.writeError(w, 500, "templates_failed", "Не удалось прочитать письмо")
				return
			}
			if !d.UpdatedAt.Equal(req.UpdatedAt) || d.Status == "running" {
				h.writeError(w, 409, "broadcast_draft_changed", "Черновик изменился. Обновите страницу")
				return
			}
			if d.Subject == "" || d.Body == "" {
				h.writeError(w, 400, "email_broadcast_empty", "Сначала сохраните письмо")
				return
			}
			p.Subject = d.Subject
			p.Body = d.Body
		}
		if err := h.customerRepository.SaveBroadcastTemplate(r.Context(), req.Channel, req.Name, p, sess.User.ID); err != nil {
			h.writeError(w, 500, "templates_failed", "Не удалось сохранить шаблон")
			return
		}
		h.writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if req.Action == "apply" && req.ID > 0 {
		t, err := h.customerRepository.BroadcastTemplate(r.Context(), req.ID)
		if err != nil {
			h.writeError(w, 500, "templates_failed", "Не удалось загрузить шаблон")
			return
		}
		if t == nil || t.Channel != req.Channel {
			h.writeError(w, 404, "template_not_found", "Шаблон не найден")
			return
		}
		var p database.BroadcastTemplatePayload
		if json.Unmarshal(t.Payload, &p) != nil || p.Audience.Normalize() != nil {
			h.writeError(w, 400, "invalid_template", "Шаблон повреждён")
			return
		}
		if req.Channel == "telegram" {
			if h.broadcastService == nil {
				h.writeError(w, 503, "broadcast_unavailable", "Рассылка недоступна")
				return
			}
			d, err := h.broadcastService.RestoreTemplate(r.Context(), p, sess.User.ID, req.UpdatedAt)
			if err != nil {
				h.writeBroadcastError(w, err)
				return
			}
			h.writeJSON(w, 200, map[string]any{"ok": true, "data": d})
			return
		}
		if len([]rune(p.Subject)) < 2 || len([]rune(p.Subject)) > 160 || strings.ContainsAny(p.Subject, "\r\n") || len([]rune(p.Body)) < 2 || len([]rune(p.Body)) > 20000 {
			h.writeError(w, 400, "invalid_template", "Некорректное содержимое письма")
			return
		}
		d, err := h.customerRepository.RestoreEmailTemplate(r.Context(), p, sess.User.ID, req.UpdatedAt)
		if err == pgx.ErrNoRows {
			h.writeError(w, 409, "broadcast_draft_changed", "Черновик изменился. Обновите страницу")
			return
		}
		if err != nil {
			h.writeError(w, 500, "templates_failed", "Не удалось применить шаблон")
			return
		}
		h.writeJSON(w, 200, map[string]any{"ok": true, "data": d})
		return
	}
	h.writeError(w, 400, "invalid_template", "Некорректное действие с шаблоном")
}
