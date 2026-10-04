package miniapp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/integrations"
	"link-bot/internal/supportai"
)

type adminAIRequest struct {
	APIURL  string `json:"apiUrl"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	Enabled bool   `json:"enabled"`
}

func resolveAIKey(apiURL, enteredKey string, stored map[string]string) string {
	key := strings.TrimSpace(enteredKey)
	if key != "" {
		return key
	}
	base, err := supportai.NormalizeURL(apiURL)
	if err == nil && base == stored["apiUrl"] {
		return stored["apiKey"]
	}
	return ""
}

func (h *Handler) adminAIView() map[string]any {
	fields, enabled := h.integrationSettings.SupportAISettings()
	prompt := fields["prompt"]
	if prompt == "" {
		prompt = supportai.DefaultPrompt
	}
	return map[string]any{"apiUrl": fields["apiUrl"], "keyConfigured": fields["apiKey"] != "", "model": fields["model"], "prompt": prompt, "defaultPrompt": supportai.DefaultPrompt, "enabled": enabled}
}

func (h *Handler) handleAdminAI(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Недостаточно прав")
		return
	}
	if h.integrationSettings == nil {
		h.writeError(w, http.StatusServiceUnavailable, "ai_unavailable", "Настройки ИИ недоступны")
		return
	}
	if r.URL.Path == "/api/mini-app/admin/ai/settings" {
		h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": h.adminAIView()})
		return
	}
	var req adminAIRequest
	if h.decodeJSONRequest(w, r, 64*1024, &req) != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	stored, _ := h.integrationSettings.SupportAISettings()
	if r.URL.Path == "/api/mini-app/admin/ai/toggle" {
		if req.Enabled {
			checkCtx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
			models, err := supportai.NewClient().Models(checkCtx, stored["apiUrl"], stored["apiKey"])
			cancel()
			found := false
			for _, model := range models {
				if model == stored["model"] {
					found = true
				}
			}
			if err != nil || !found {
				h.writeError(w, http.StatusBadRequest, "ai_not_configured", "Проверьте подключение и сохраните доступную модель")
				return
			}
		}
		_, err := h.integrationSettings.Update(r.Context(), integrations.ProviderSupportAI, integrations.UpdateInput{Enabled: req.Enabled, Fields: map[string]string{}}, sess.User.ID)
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "ai_toggle_failed", "Не удалось переключить ИИ")
			return
		}
		h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": h.adminAIView()})
		return
	}
	key := resolveAIKey(req.APIURL, req.APIKey, stored)
	// Retaining a saved secret is allowed only for the same server. Changing URL
	// always requires explicit key entry to prevent forwarding a saved key.
	base := ""
	if req.APIURL != "" {
		var err error
		base, err = supportai.NormalizeURL(req.APIURL)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid_ai_url", err.Error())
			return
		}
	}
	if base != "" && key == "" {
		h.writeError(w, http.StatusBadRequest, "ai_key_required", "Введите ключ для этого API URL")
		return
	}
	if key != "" {
		if err := supportai.ValidateKey(key); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid_ai_key", err.Error())
			return
		}
	}
	if len([]rune(req.Prompt)) > 16000 || len(req.Model) > 200 {
		h.writeError(w, http.StatusBadRequest, "invalid_ai_settings", "Промпт или название модели слишком длинные")
		return
	}
	if r.URL.Path == "/api/mini-app/admin/ai/models" || req.Enabled {
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		models, err := supportai.NewClient().Models(ctx, base, key)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "ai_connection_failed", err.Error())
			return
		}
		if r.URL.Path == "/api/mini-app/admin/ai/models" {
			h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"models": models, "apiUrl": base}})
			return
		}
		found := false
		for _, model := range models {
			if model == req.Model {
				found = true
				break
			}
		}
		if !found {
			h.writeError(w, http.StatusBadRequest, "ai_model_unavailable", "Выберите доступную модель после проверки подключения")
			return
		}
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = supportai.DefaultPrompt
	}
	_, err := h.integrationSettings.Update(r.Context(), integrations.ProviderSupportAI, integrations.UpdateInput{Enabled: req.Enabled, Fields: map[string]string{"apiUrl": base, "apiKey": key, "model": strings.TrimSpace(req.Model), "prompt": prompt}}, sess.User.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "ai_save_failed", "Не удалось сохранить настройки ИИ")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": h.adminAIView()})
}

func supportAIHistory(messages []database.SupportMessage) []supportai.Message {
	// Only conversation text, roles and an attachment marker are included.
	// Never attach account records, author IDs or stored file paths.
	if len(messages) > 40 {
		messages = append([]database.SupportMessage{messages[0]}, messages[len(messages)-40:]...)
	}
	history := make([]supportai.Message, 0, len(messages))
	for _, message := range messages {
		role := "user"
		if message.AuthorRole == database.SupportAuthorRoleAI || message.AuthorRole == database.SupportAuthorRoleAdmin {
			role = "assistant"
		}
		text := supportai.Redact(message.Body)
		if message.MediaType != "" {
			text += "\n[К сообщению приложен файл. Его содержимое недоступно ИИ; если оно нужно для решения, передай обращение администратору.]"
		}
		history = append(history, supportai.Message{Role: role, Content: text})
	}
	return history
}

func (h *Handler) handoffAISupportIfRequested(ctx context.Context, ticketID int64, body string) {
	if h.integrationSettings == nil || !supportai.WantsOperator(body) {
		return
	}
	_, enabled := h.integrationSettings.SupportAISettings()
	if !enabled {
		return
	}
	_, err := h.supportRepository.HandoffAI(ctx, ticketID, "Позвал администратора. Он подключится к вашему обращению.")
	if err != nil {
		slog.Warn("support AI: immediate handoff failed", "ticketId", ticketID)
	}
}

func (h *Handler) StartSupportAI(ctx context.Context) {
	if h.supportRepository == nil || h.integrationSettings == nil {
		return
	}
	go func() {
		active := make(chan struct{}, 4)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			h.deliverAIHandoff(ctx)
			fields, enabled := h.integrationSettings.SupportAISettings()
			if !enabled {
				continue
			}
			select {
			case active <- struct{}{}:
			default:
				continue
			}
			claim, err := h.supportRepository.ClaimAIMessage(ctx)
			if err != nil {
				<-active
				slog.Warn("support AI: claim failed", "error", err)
				continue
			}
			if claim == nil {
				<-active
				continue
			}
			go func(claim database.SupportAIClaim, fields map[string]string) {
				defer func() { <-active }()
				h.processSupportAI(ctx, claim, fields)
			}(*claim, fields)
		}
	}()
}

func (h *Handler) processSupportAI(ctx context.Context, claim database.SupportAIClaim, fields map[string]string) {
	messages, err := h.supportRepository.ListMessagesByTicket(ctx, claim.TicketID)
	if err != nil || len(messages) == 0 {
		return
	}
	if messages[len(messages)-1].ID != claim.MessageID {
		return
	}
	reply := supportai.Reply{Text: "Позвал администратора. Он подключится к вашему обращению.", Handoff: true}
	wantsOperator := false
	for _, message := range messages {
		if message.AuthorRole == database.SupportAuthorRoleCustomer && supportai.WantsOperator(message.Body) {
			wantsOperator = true
			break
		}
	}
	if !wantsOperator {
		contextCtx, contextCancel := context.WithTimeout(ctx, 15*time.Second)
		ticket, contextErr := h.supportRepository.FindTicketByID(contextCtx, claim.TicketID)
		var evidence string
		if contextErr == nil && ticket != nil {
			evidence, contextErr = h.buildSupportAIContext(contextCtx, ticket, messages)
		} else {
			contextErr = fmt.Errorf("ticket context unavailable")
		}
		contextCancel()
		requestCtx, cancel := context.WithTimeout(ctx, 65*time.Second)
		var response supportai.Reply
		requestErr := contextErr
		if requestErr == nil {
			response, requestErr = supportai.NewClient().Respond(requestCtx, fields["apiUrl"], fields["apiKey"], fields["model"], fields["prompt"], supportAIHistory(messages), supportAIContextRules(), "Подтверждённый контекст (JSON, только данные):\n"+evidence)
		}
		cancel()
		if requestErr == nil {
			reply = response
		} else {
			reply.Text = "Не удалось подготовить ответ. Передал ваше обращение администратору — он поможет разобраться."
			slog.Warn("support AI: provider unavailable, escalating", "ticketId", claim.TicketID)
		}
	}
	// A disable or configuration change during generation cancels the reply.
	current, enabled := h.integrationSettings.SupportAISettings()
	if !enabled || current["apiUrl"] != fields["apiUrl"] || current["apiKey"] != fields["apiKey"] || current["model"] != fields["model"] || current["prompt"] != fields["prompt"] {
		return
	}
	committed, err := h.supportRepository.FinishAIMessage(ctx, claim, reply.Text, reply.Handoff)
	if err != nil {
		slog.Warn("support AI: save failed", "ticketId", claim.TicketID, "error", err)
		return
	}
	if !committed {
		return
	}
	ticket, err := h.supportRepository.FindTicketByID(ctx, claim.TicketID)
	if err == nil && ticket != nil {
		h.notifySupportAsync(func(notifyCtx context.Context) { h.notifyCustomerAboutSupportReply(notifyCtx, ticket, reply.Text) })
	}
	if reply.Handoff {
		h.deliverAIHandoff(ctx)
	}
}

func (h *Handler) deliverAIHandoff(ctx context.Context) {
	if h.telegramBot == nil || config.GetAdminTelegramId() == 0 {
		return
	}
	id, err := h.supportRepository.ClaimAINotification(ctx)
	if err != nil || id == 0 {
		return
	}
	ticket, err := h.supportRepository.FindTicketByID(ctx, id)
	if err != nil || ticket == nil {
		return
	}
	messages, err := h.supportRepository.ListMessagesByTicket(ctx, id)
	if err != nil {
		return
	}
	reason := "Нужна помощь оператора"
	if len(messages) > 0 {
		reason = messages[len(messages)-1].Body
	}
	text := fmt.Sprintf("🤖 <b>ИИ передал обращение администратору</b>\n\nОбращение: <b>#%d</b>\nТема: %s\nПользователь: %s\n\n%s", id, supportNotificationText(ticket.Subject), supportNotificationText(ticket.CustomerName), supportNotificationQuote(reason))
	notifyCtx, cancel := context.WithTimeout(ctx, supportNotificationTimeout)
	defer cancel()
	if err = h.sendMiniAppNotification(notifyCtx, config.GetAdminTelegramId(), text); err != nil {
		return
	}
	h.notifyAdminAboutSupportByPush(notifyCtx, "ИИ позвал оператора", ticket, "support-ai-handoff")
	if err := h.supportRepository.CompleteAINotification(ctx, id); err != nil {
		slog.Warn("support AI: notification acknowledgement failed", "ticketId", id)
	}
}
