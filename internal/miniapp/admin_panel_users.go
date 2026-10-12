package miniapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

func panelBinding(user remnawave.PanelUser, observations []database.PanelPlanObservation) database.PanelPlanObservation {
	return panelBindingLookup(observations)(user)
}

func panelBindingLookup(observations []database.PanelPlanObservation) func(remnawave.PanelUser) database.PanelPlanObservation {
	byID := map[int64]database.PanelPlanObservation{}
	byUUID := map[uuid.UUID]database.PanelPlanObservation{}
	byLink := map[string]database.PanelPlanObservation{}
	for _, o := range observations {
		if o.PanelID != nil {
			byID[*o.PanelID] = o
		}
		if o.PanelUUID != nil {
			byUUID[*o.PanelUUID] = o
		}
		if o.Link != "" {
			byLink[o.Link] = o
		}
	}
	return func(user remnawave.PanelUser) database.PanelPlanObservation {
		// UUID wins; a reused numeric ID must never select another subscription.
		if o, ok := byUUID[user.UUID]; ok && user.UUID != uuid.Nil {
			return o
		}
		if o, ok := byID[user.ID]; ok && (user.UUID == uuid.Nil || o.PanelUUID == nil || *o.PanelUUID == uuid.Nil || *o.PanelUUID == user.UUID) {
			return o
		}
		if o, ok := byLink[user.SubscriptionURL]; ok && user.SubscriptionURL != "" {
			return o
		}
		return database.PanelPlanObservation{}
	}
}

func panelSummary(user remnawave.PanelUser, binding database.PanelPlanObservation) adminUserSummaryPayload {
	s := adminUserSummaryPayload{Key: fmt.Sprintf("panel:%d:%s", user.ID, user.UUID), PanelID: user.ID, PanelUUID: user.UUID, CustomerID: binding.CustomerID, SubscriptionID: binding.SubscriptionID,
		Username: user.Username, PanelUsername: user.Username, SubscriptionName: user.Username, SubscriptionCount: 1, SubscriptionLink: user.SubscriptionURL,
		SubscriptionStatus: strings.ToLower(user.Status), TrafficLoaded: true, TrafficLimitBytes: user.TrafficLimitBytes, TrafficLimitStrategy: user.TrafficLimitStrategy,
		UsedTrafficBytes: user.UserTraffic.UsedTrafficBytes, LifetimeUsedTrafficBytes: user.UserTraffic.LifetimeUsedTrafficBytes}
	if user.TelegramID != nil {
		s.TelegramID = *user.TelegramID
	}
	if user.Description != nil {
		s.Description = *user.Description
	}
	if user.Tag != nil {
		s.Tag = *user.Tag
	}
	if !user.CreatedAt.IsZero() {
		s.CreatedAt = user.CreatedAt.UTC().Format(time.RFC3339)
		s.PanelCreatedAt = s.CreatedAt
	}
	if !user.ExpireAt.IsZero() {
		s.ExpiresAt = user.ExpireAt.UTC().Format(time.RFC3339)
	}
	if user.UserTraffic.OnlineAt != nil {
		s.OnlineAt = user.UserTraffic.OnlineAt.UTC().Format(time.RFC3339)
	}
	traffic := "Лимитный"
	if user.TrafficLimitBytes <= 0 {
		traffic = "Безлимит"
	}
	if binding.Months > 0 {
		s.PlanTitle = fmt.Sprintf("%d мес. · %s", binding.Months, traffic)
	} else if binding.Days > 0 {
		s.PlanTitle = fmt.Sprintf("%d дн. · %s", binding.Days, traffic)
	} else {
		s.PlanTitle = "Индивидуальный · " + traffic
	}
	return s
}

func panelUserRows(catalog panelCatalogSnapshot, query string) ([]adminUserSummaryPayload, *remnawave.PanelUserCounts) {
	items := []adminUserSummaryPayload{}
	counts := &remnawave.PanelUserCounts{StatusCounts: map[string]int64{}}
	query = strings.ToLower(strings.TrimSpace(query))
	bindingFor := panelBindingLookup(catalog.Observations)
	for _, user := range catalog.Users {
		if strings.TrimSpace(user.SubscriptionURL) == "" && user.ExpireAt.IsZero() {
			continue
		}
		counts.TotalUsers++
		counts.StatusCounts[strings.ToUpper(user.Status)]++
		s := panelSummary(user, bindingFor(user))
		if query != "" && !strings.Contains(strings.ToLower(fmt.Sprintf("%s %s %s %s %s %d %d", s.PanelUsername, s.Description, s.Tag, s.SubscriptionLink, s.PlanTitle, s.TelegramID, s.PanelID)), query) {
			continue
		}
		items = append(items, s)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt != items[j].CreatedAt {
			return items[i].CreatedAt > items[j].CreatedAt
		}
		return items[i].Key > items[j].Key
	})
	return items, counts
}

func (h *Handler) handlePanelUsersSearch(w http.ResponseWriter, r *http.Request, req adminUserSearchRequest) {
	catalog, pending, err := h.requestPanelCatalog(r.Context(), false, req.Snapshot)
	if err != nil && catalog.Users == nil {
		code := http.StatusBadGateway
		errorCode := "panel_users_failed"
		if req.Snapshot != "" {
			code = http.StatusConflict
			errorCode = "panel_snapshot_expired"
		}
		h.writeError(w, code, errorCode, "Не удалось загрузить пользователей панели. Обновите список.")
		return
	}
	items, counts := panelUserRows(catalog, req.Query)
	offset := max(0, min(req.Offset, len(items)))
	end := min(offset+req.Limit, len(items))
	payload := adminUserSearchPayload{Source: "panel", Snapshot: catalog.Revision, Pending: pending, Items: items[offset:end], Total: len(items), Limit: req.Limit, Offset: offset}
	if catalog.Users != nil {
		payload.PanelCounts = counts
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": payload})
}

func panelOnlyDetail(user *remnawave.PanelUser) *adminUserDetailPayload {
	s := panelSummary(*user, database.PanelPlanObservation{})
	devices := 0
	if user.HwidDeviceLimit != nil {
		devices = *user.HwidDeviceLimit
	}
	subscriptionID := user.ID
	if subscriptionID <= 0 {
		subscriptionID = 1
	}
	return &adminUserDetailPayload{PanelOnly: true, PanelID: user.ID, PanelUUID: user.UUID, TelegramID: s.TelegramID, Username: user.Username, CreatedAt: s.CreatedAt,
		Subscriptions: []adminUserSubscriptionPayload{{ID: subscriptionID, PanelID: user.ID, Name: user.Username, IsSelected: true, IsPrimary: true, Status: s.SubscriptionStatus, ExpiresAt: s.ExpiresAt,
			TrafficLimitBytes: s.TrafficLimitBytes, UsedTrafficBytes: s.UsedTrafficBytes, LifetimeUsedTrafficBytes: s.LifetimeUsedTrafficBytes, DeviceLimit: devices, PanelUsername: user.Username,
			SubscriptionLink: user.SubscriptionURL, Settings: remnawave.UserSettingsFromPanelUser(user)}}}
}

func (h *Handler) handlePanelUserDetail(w http.ResponseWriter, r *http.Request, req adminUserDetailRequest) {
	if h.remnawaveClient == nil {
		h.writeError(w, 503, "panel_unavailable", "Панель недоступна")
		return
	}
	user, err := h.remnawaveClient.GetAdminUserOverview(r.Context(), req.PanelID, req.PanelUUID, 0)
	if err != nil || user == nil {
		h.writeError(w, 502, "panel_user_failed", "Не удалось загрузить пользователя панели")
		return
	}
	var observations []database.PanelPlanObservation
	if h.customerRepository != nil {
		observations, err = h.customerRepository.PanelPlanObservations(r.Context())
		if err != nil {
			h.writeError(w, 503, "panel_binding_failed", "Не удалось проверить подписку пользователя")
			return
		}
	}
	binding := panelBinding(*user, observations)
	payload := panelOnlyDetail(user)
	if binding.CustomerID > 0 {
		payload, err = h.loadAdminUserDetail(r.Context(), binding.CustomerID)
		if err != nil || payload == nil {
			h.writeError(w, 502, "admin_user_failed", "Не удалось загрузить пользователя")
			return
		}
		payload.PanelID = user.ID
		payload.PanelUUID = user.UUID
		for i := range payload.Subscriptions {
			payload.Subscriptions[i].IsSelected = payload.Subscriptions[i].ID == binding.SubscriptionID
		}
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": payload})
}

func (h *Handler) handlePanelUserSettings(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !sess.canAdmin("users.subscription") {
		h.writeError(w, 403, "forbidden", "Access denied")
		return
	}
	var req struct {
		CustomerID int64                   `json:"customerId"`
		PanelID    int64                   `json:"panelId"`
		PanelUUID  uuid.UUID               `json:"panelUuid"`
		Settings   *remnawave.UserSettings `json:"settings"`
	}
	if err := h.decodeJSONRequest(w, r, 65536, &req); err != nil || req.Settings == nil || (req.PanelID <= 0 && req.PanelUUID == uuid.Nil) {
		h.writeError(w, 400, "invalid_request", "Выберите пользователя и настройки")
		return
	}
	if err := req.Settings.Validate(); err != nil {
		h.writeError(w, 400, "invalid_settings", err.Error())
		return
	}
	if h.remnawaveClient == nil || h.customerRepository == nil {
		h.writeError(w, 503, "panel_unavailable", "Панель недоступна")
		return
	}
	user, err := h.remnawaveClient.GetAdminUserOverview(r.Context(), req.PanelID, req.PanelUUID, 0)
	if err != nil || user == nil {
		h.writeError(w, 502, "panel_user_failed", "Не удалось проверить пользователя панели")
		return
	}
	observations, err := h.customerRepository.PanelPlanObservations(r.Context())
	if err != nil {
		h.writeError(w, 503, "panel_binding_failed", "Не удалось проверить подписку пользователя")
		return
	}
	binding := panelBinding(*user, observations)
	if binding.CustomerID > 0 {
		// Reuse locking and manual-override protection for subscriptions linked to the bot.
		body, _ := json.Marshal(map[string]any{"customerId": binding.CustomerID, "subscriptionId": binding.SubscriptionID, "settings": req.Settings})
		cloned := r.Clone(r.Context())
		cloned.Body = io.NopCloser(bytes.NewReader(body))
		h.handleAdminUserSettings(w, cloned, sess, customer)
		return
	}
	updated, err := h.remnawaveClient.UpdateAdminUserSettings(r.Context(), user.ID, user.UUID, *req.Settings)
	if err != nil {
		h.writeError(w, 502, "admin_settings_failed", "Не удалось сохранить настройки в панели")
		return
	}
	h.panelCatalog(true, "")
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": panelOnlyDetail(updated), "message": "Настройки пользователя сохранены"})
}
