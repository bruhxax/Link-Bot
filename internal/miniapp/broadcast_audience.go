package miniapp

import (
	"context"
	"errors"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"net/http"
	"strings"
	"time"
)

func matchesBroadcastCustomer(c database.BroadcastCustomer, users []remnawave.PanelUser, a database.BroadcastAudience, trialTag string, now time.Time, panelConfigured bool) bool {
	if c.Unsubscribed && !a.IgnoreUnsubscribe {
		return false
	}
	if a.Language != "" && c.Language != a.Language {
		return false
	}
	active, expired, trial, scope := false, false, false, false
	for _, u := range users {
		live := strings.EqualFold(u.Status, "ACTIVE") && (u.ExpireAt.IsZero() || u.ExpireAt.After(now))
		active = active || live
		expired = expired || strings.EqualFold(u.Status, "EXPIRED") || !u.ExpireAt.IsZero() && !u.ExpireAt.After(now)
		trial = trial || live && u.Tag != nil && trialTag != "" && strings.EqualFold(*u.Tag, trialTag)
		internal := len(a.InternalSquads) == 0
		for _, s := range u.ActiveInternalSquads {
			for _, id := range a.InternalSquads {
				if s.UUID.String() == id {
					internal = true
				}
			}
		}
		external := len(a.ExternalSquads) == 0
		if u.ExternalSquadUUID != nil {
			for _, id := range a.ExternalSquads {
				if u.ExternalSquadUUID.String() == id {
					external = true
				}
			}
		}
		tag := a.Tag == "" || u.Tag != nil && strings.EqualFold(*u.Tag, a.Tag)
		scope = scope || internal && external && tag
	}
	if !panelConfigured {
		for _, s := range c.Subscriptions {
			active = active || s.ExpireAt != nil && s.ExpireAt.After(now)
			expired = expired || s.ExpireAt != nil && !s.ExpireAt.After(now)
		}
		active = active || c.ExpireAt != nil && c.ExpireAt.After(now)
		expired = expired || c.ExpireAt != nil && !c.ExpireAt.After(now)
		trial = active && c.TrialUsed && !c.Paid
	}
	if len(a.InternalSquads) > 0 || len(a.ExternalSquads) > 0 || a.Tag != "" {
		if !scope {
			return false
		}
	}
	switch a.Category {
	case "active":
		return active
	case "none":
		return !active
	case "expired":
		return !active && expired
	case "trial":
		return trial
	case "trial_used":
		return c.TrialUsed
	case "paid":
		return c.Paid
	default:
		return true
	}
}

func (h *Handler) broadcastCustomers(ctx context.Context, a database.BroadcastAudience) ([]database.BroadcastCustomer, error) {
	if err := a.Normalize(); err != nil {
		return nil, err
	}
	customers, err := h.customerRepository.BroadcastCustomers(ctx)
	if err != nil {
		return nil, err
	}
	needPanel := a.Category == "active" || a.Category == "none" || a.Category == "expired" || a.Category == "trial" || len(a.InternalSquads) > 0 || len(a.ExternalSquads) > 0 || a.Tag != ""
	if !needPanel {
		result := customers[:0]
		for _, c := range customers {
			if matchesBroadcastCustomer(c, nil, a, "", time.Now(), false) {
				result = append(result, c)
			}
		}
		return result, nil
	}
	if h.remnawaveClient == nil && (len(a.InternalSquads) > 0 || len(a.ExternalSquads) > 0 || a.Tag != "") {
		return nil, errors.New("Для отбора по сквадам и тегам подключите панель")
	}
	byID := map[int64]remnawave.PanelUser{}
	byUUID := map[string]remnawave.PanelUser{}
	byTelegram := map[int64][]remnawave.PanelUser{}
	if h.remnawaveClient != nil {
		users, err := h.remnawaveClient.GetUsers(ctx)
		if err != nil {
			return nil, errors.New("Не удалось проверить аудиторию в панели. Отправка не запущена")
		}
		for _, u := range *users {
			byID[u.ID] = u
			byUUID[u.UUID.String()] = u
			if u.TelegramID != nil {
				byTelegram[*u.TelegramID] = append(byTelegram[*u.TelegramID], u)
			}
		}
	}
	result := []database.BroadcastCustomer{}
	now := time.Now()
	for _, c := range customers {
		users := append([]remnawave.PanelUser{}, byTelegram[c.TelegramID]...)
		for _, s := range c.Subscriptions {
			matched := false
			if s.PanelID != nil {
				if u, ok := byID[*s.PanelID]; ok {
					users = append(users, u)
					matched = true
				}
			}
			if !matched && s.PanelUUID != nil {
				if u, ok := byUUID[s.PanelUUID.String()]; ok {
					users = append(users, u)
				}
			}
		}
		if matchesBroadcastCustomer(c, users, a, h.trialSettings().Tag, now, h.remnawaveClient != nil) {
			result = append(result, c)
		}
	}
	return result, nil
}

func (h *Handler) broadcastTelegramRecipients(ctx context.Context, a database.BroadcastAudience) ([]int64, error) {
	customers, err := h.broadcastCustomers(ctx, a)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for _, c := range customers {
		if c.TelegramID > 0 && !c.Synthetic && !seen[c.TelegramID] {
			ids = append(ids, c.TelegramID)
			seen[c.TelegramID] = true
		}
	}
	return ids, nil
}
func (h *Handler) broadcastEmailRecipients(ctx context.Context, a database.BroadcastAudience) ([]string, error) {
	// The /start preference belongs only to Telegram, never to email.
	a.IgnoreUnsubscribe = true
	customers, err := h.broadcastCustomers(ctx, a)
	if err != nil {
		return nil, err
	}
	emails := []string{}
	seen := map[string]bool{}
	for _, c := range customers {
		key := strings.ToLower(c.Email)
		if key != "" && !seen[key] {
			emails = append(emails, c.Email)
			seen[key] = true
		}
	}
	return emails, nil
}

func (h *Handler) handleAdminBroadcastAudience(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.canAdmin("broadcast") {
		h.writeError(w, 403, "forbidden", "Недостаточно прав")
		return
	}
	var req struct {
		Channel  string                     `json:"channel"`
		Audience database.BroadcastAudience `json:"audience"`
	}
	if err := h.decodeJSONRequest(w, r, 32<<10, &req); err != nil {
		return
	}
	if req.Channel != "telegram" && req.Channel != "email" {
		h.writeError(w, 400, "invalid_channel", "Выберите тип рассылки")
		return
	}
	if req.Channel == "email" {
		req.Audience.IgnoreUnsubscribe = false
	}
	if err := req.Audience.Normalize(); err != nil {
		h.writeError(w, 400, "invalid_audience", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	count := 0
	var err error
	if req.Channel == "email" {
		var targets []string
		targets, err = h.broadcastEmailRecipients(ctx, req.Audience)
		count = len(targets)
	} else {
		var targets []int64
		targets, err = h.broadcastTelegramRecipients(ctx, req.Audience)
		for _, id := range targets {
			if id != sess.User.ID {
				count++
			}
		}
	}
	if err != nil {
		h.writeError(w, 502, "audience_unavailable", err.Error())
		return
	}
	var draft any
	if req.Channel == "email" {
		draft, err = h.customerRepository.SaveEmailAudience(ctx, req.Audience, sess.User.ID)
	} else if h.broadcastService != nil {
		draft, err = h.broadcastService.SaveAudience(ctx, sess.User.ID, req.Audience)
	} else {
		err = errors.New("Рассылка недоступна")
	}
	if err != nil {
		h.writeError(w, 409, "audience_not_saved", err.Error())
		return
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"draft": draft, "count": count, "audience": req.Audience}})
}

func (h *Handler) handleAdminBroadcastAudienceOptions(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.canAdmin("broadcast") {
		h.writeError(w, 403, "forbidden", "Недостаточно прав")
		return
	}
	if h.remnawaveClient == nil {
		h.writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"internal": []any{}, "external": []any{}}})
		return
	}
	options, err := h.remnawaveClient.ListSquads(r.Context())
	if err != nil {
		h.writeError(w, 502, "squads_unavailable", "Не удалось загрузить сквады")
		return
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": options})
}
