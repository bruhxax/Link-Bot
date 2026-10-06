package miniapp

import (
	"link-bot/internal/database"
	"net/http"
	"strconv"
	"strings"
)

func supportAIHandoffAfter(fields map[string]string) int {
	n, _ := strconv.Atoi(fields["handoffAfter"])
	if n < 1 || n > 20 {
		return 4
	}
	return n
}
func supportAIAnswerCount(messages []database.SupportMessage) int {
	n := 0
	for _, m := range messages {
		if m.AuthorRole == database.SupportAuthorRoleAI {
			n++
		}
	}
	return n
}

func (h *Handler) handleSupportOperator(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !sess.canAdmin("support.reply") {
		h.writeError(w, http.StatusForbidden, "forbidden", "Недостаточно прав")
		return
	}
	var req struct {
		TicketID int64 `json:"ticketId"`
		Release  bool  `json:"release"`
	}
	if h.decodeJSONRequest(w, r, 4096, &req) != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	if h.supportRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "support_unavailable", "Поддержка недоступна")
		return
	}
	ticket, err := h.loadSupportTicketForViewer(r.Context(), sess, customer, req.TicketID)
	if err != nil {
		h.writeError(w, 500, "support_failed", "Не удалось загрузить обращение")
		return
	}
	if ticket == nil {
		h.writeError(w, 404, "support_ticket_not_found", "Обращение не найдено")
		return
	}
	name := strings.TrimSpace(sess.User.FirstName + " " + sess.User.LastName)
	if name == "" {
		name = strings.TrimPrefix(sess.User.Username, "@")
	}
	if name == "" {
		name = "Оператор"
	}
	if err = h.supportRepository.SetOperator(r.Context(), ticket.ID, sess.User.ID, name, req.Release, sess.access().IsOwner); err != nil {
		if err == database.ErrSupportOperatorConflict {
			h.writeError(w, 409, "operator_conflict", "Обращение уже занято другим оператором или закрыто")
			return
		}
		h.writeError(w, 500, "support_failed", "Не удалось изменить оператора")
		return
	}
	h.handleSupportThreadResult(w, r, sess, customer, ticket.ID)
}

func (h *Handler) handleSupportHandoff(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	var req supportTicketRequest
	if h.decodeJSONRequest(w, r, 4096, &req) != nil {
		h.writeError(w, 400, "invalid_request", "Некорректный запрос")
		return
	}
	if h.supportRepository == nil {
		h.writeError(w, 503, "support_unavailable", "Поддержка недоступна")
		return
	}
	ticket, err := h.loadSupportTicketForViewer(r.Context(), sess, customer, req.TicketID)
	if err != nil {
		h.writeError(w, 500, "support_failed", "Не удалось загрузить обращение")
		return
	}
	if ticket == nil || customer == nil || ticket.CustomerID != customer.ID || sess.canAdmin("support.view") {
		h.writeError(w, 404, "support_ticket_not_found", "Обращение не найдено")
		return
	}
	thread, err := h.buildSupportThreadPayload(r.Context(), sess, customer, ticket)
	if err != nil {
		h.writeError(w, 500, "support_failed", "Не удалось загрузить обращение")
		return
	}
	if !thread.HandedOff && !thread.CanHandoff {
		h.writeError(w, 409, "handoff_unavailable", "Вызов оператора пока недоступен")
		return
	}
	if !thread.HandedOff {
		if _, err = h.supportRepository.HandoffAI(r.Context(), ticket.ID, "Позвал администратора. Он подключится к вашему обращению."); err != nil {
			h.writeError(w, 500, "support_failed", "Не удалось позвать оператора")
			return
		}
	}
	h.handleSupportThreadResult(w, r, sess, customer, ticket.ID)
}

func (h *Handler) handleSupportThreadResult(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer, id int64) {
	ticket, err := h.loadSupportTicketForViewer(r.Context(), sess, customer, id)
	if err != nil || ticket == nil {
		h.writeError(w, 500, "support_failed", "Не удалось обновить обращение")
		return
	}
	thread, err := h.buildSupportThreadPayload(r.Context(), sess, customer, ticket)
	if err != nil {
		h.writeError(w, 500, "support_failed", "Не удалось обновить переписку")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": thread})
}
