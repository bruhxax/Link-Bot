package miniapp

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/database"
)

func (h *Handler) handleAdminBroadcastSurvey(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.requireBroadcastAdmin(w, sess) {
		return
	}
	if !sess.canAdmin("broadcast") {
		h.writeError(w, 403, "forbidden", "Недостаточно прав")
		return
	}
	var req struct {
		Survey    database.SurveyConfig `json:"survey"`
		UpdatedAt time.Time             `json:"updatedAt"`
	}
	if err := h.decodeJSONRequest(w, r, 3<<20, &req); err != nil {
		h.writeError(w, 400, "invalid_request", "Некорректный опрос")
		return
	}
	draft, err := h.broadcastService.SaveSurvey(r.Context(), sess.User.ID, req.Survey, req.UpdatedAt)
	if err != nil {
		h.writeBroadcastError(w, err)
		return
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": draft})
}
func (h *Handler) handleAdminBroadcastSurveyResults(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.requireBroadcastAdmin(w, sess) {
		return
	}
	if !sess.canAdmin("broadcast") {
		h.writeError(w, 403, "forbidden", "Недостаточно прав")
		return
	}
	var req struct {
		ID     string `json:"id"`
		Before int64  `json:"before"`
	}
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil {
		h.writeError(w, 400, "invalid_request", "Некорректный запрос")
		return
	}
	if req.Before < 0 {
		h.writeError(w, 400, "invalid_request", "Некорректная страница")
		return
	}
	if req.ID != "" {
		id, err := uuid.Parse(req.ID)
		if err != nil {
			h.writeError(w, 400, "invalid_request", "Некорректный опрос")
			return
		}
		req.ID = id.String()
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	runs, report, err := h.broadcastService.SurveyResults(ctx, req.ID, req.Before)
	if err != nil {
		h.writeError(w, 500, "survey_results_failed", "Не удалось загрузить ответы")
		return
	}
	if req.ID != "" && report == nil {
		h.writeError(w, 404, "survey_not_found", "Опрос не найден")
		return
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"runs": runs, "report": report}})
}
