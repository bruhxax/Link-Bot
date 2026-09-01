package miniapp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/utils"
)

const adminUserTrafficGB = int64(1024 * 1024 * 1024)

type adminUserSearchRequest struct {
	Query  string `json:"query"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type adminUserDetailRequest struct {
	CustomerID int64 `json:"customerId"`
}

type adminUserActionRequest struct {
	CustomerID     int64 `json:"customerId"`
	SubscriptionID int64 `json:"subscriptionId"`
	AmountRub      int64 `json:"amountRub"`
	Days           int   `json:"days"`
	TrafficGB      int64 `json:"trafficGb"`
	Blocked        bool  `json:"blocked"`
}

type adminUserSearchPayload struct {
	Items  []adminUserSummaryPayload `json:"items"`
	Total  int                       `json:"total"`
	Limit  int                       `json:"limit"`
	Offset int                       `json:"offset"`
}

type adminUserSummaryPayload struct {
	CustomerID         int64  `json:"customerId"`
	TelegramID         int64  `json:"telegramId"`
	Username           string `json:"username"`
	AvatarURL          string `json:"avatarUrl,omitempty"`
	SubscriptionName   string `json:"subscriptionName,omitempty"`
	SubscriptionStatus string `json:"subscriptionStatus"`
	CreatedAt          string `json:"createdAt"`
	IsBlocked          bool   `json:"isBlocked"`
}

type adminUserDetailPayload struct {
	CustomerID    int64                          `json:"customerId"`
	TelegramID    int64                          `json:"telegramId"`
	Username      string                         `json:"username"`
	AvatarURL     string                         `json:"avatarUrl,omitempty"`
	CreatedAt     string                         `json:"createdAt"`
	IsBlocked     bool                           `json:"isBlocked"`
	TrialUsed     bool                           `json:"trialUsed"`
	Referrals     adminUserReferralPayload       `json:"referrals"`
	Subscriptions []adminUserSubscriptionPayload `json:"subscriptions"`
}

type adminUserReferralPayload struct {
	BalanceCents int64 `json:"balanceCents"`
	Invited      int   `json:"invited"`
	Purchased    int   `json:"purchased"`
	Trial        int   `json:"trial"`
}

type adminUserSubscriptionPayload struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	IsPrimary         bool   `json:"isPrimary"`
	IsSelected        bool   `json:"isSelected"`
	Status            string `json:"status"`
	ExpiresAt         string `json:"expiresAt,omitempty"`
	TrafficLimitBytes int64  `json:"trafficLimitBytes"`
	UsedTrafficBytes  int64  `json:"usedTrafficBytes"`
	DeviceLimit       int    `json:"deviceLimit"`
	UsedDevices       int    `json:"usedDevices"`
}

func adminUserAvatarURL(username string) string {
	username = database.NormalizeTelegramUsername(username)
	if username == "" {
		return ""
	}
	return "https://t.me/i/userpic/320/" + url.PathEscape(username) + ".jpg"
}

func adminSubscriptionStatus(expireAt *time.Time, blocked bool) string {
	if blocked {
		return "blocked"
	}
	if expireAt == nil || expireAt.IsZero() {
		return "none"
	}
	if expireAt.After(time.Now().UTC()) {
		return "active"
	}
	return "expired"
}

func (h *Handler) handleAdminUsersSearch(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	var req adminUserSearchRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	if len([]rune(strings.TrimSpace(req.Query))) > 100 {
		h.writeError(w, http.StatusBadRequest, "invalid_user_search", "Слишком длинный запрос")
		return
	}
	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 30
	}
	items, total, err := h.customerRepository.SearchAdminUsers(r.Context(), req.Query, req.Limit, req.Offset)
	if err != nil {
		slog.Error("mini app: search admin users", "error", err)
		h.writeError(w, http.StatusInternalServerError, "admin_users_failed", "Не удалось загрузить пользователей")
		return
	}
	payload := adminUserSearchPayload{Items: make([]adminUserSummaryPayload, 0, len(items)), Total: total, Limit: req.Limit, Offset: req.Offset}
	for _, item := range items {
		payload.Items = append(payload.Items, adminUserSummaryPayload{
			CustomerID:         item.CustomerID,
			TelegramID:         item.TelegramID,
			Username:           strings.TrimSpace(item.TelegramUsername),
			AvatarURL:          adminUserAvatarURL(item.TelegramUsername),
			SubscriptionName:   strings.TrimSpace(item.SubscriptionName),
			SubscriptionStatus: adminSubscriptionStatus(item.ExpireAt, item.IsBlocked),
			CreatedAt:          item.CreatedAt.UTC().Format(time.RFC3339),
			IsBlocked:          item.IsBlocked || config.GetBlockedTelegramIds()[item.TelegramID],
		})
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": payload})
}

func (h *Handler) handleAdminUserDetail(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	var req adminUserDetailRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil || req.CustomerID <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Выберите пользователя")
		return
	}
	payload, err := h.loadAdminUserDetail(r.Context(), req.CustomerID)
	if err != nil {
		slog.Error("mini app: load admin user", "error", err, "customerId", utils.MaskHalfInt64(req.CustomerID))
		h.writeError(w, http.StatusInternalServerError, "admin_user_failed", "Не удалось загрузить пользователя")
		return
	}
	if payload == nil {
		h.writeError(w, http.StatusNotFound, "admin_user_not_found", "Пользователь не найден")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": payload})
}

func (h *Handler) handleAdminUserBalance(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	var req adminUserActionRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil || req.CustomerID <= 0 || req.AmountRub <= 0 || req.AmountRub > 1000000 {
		h.writeError(w, http.StatusBadRequest, "invalid_admin_balance", "Укажите сумму от 1 до 1 000 000 ₽")
		return
	}
	if h.walletRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "wallet_unavailable", "Баланс временно недоступен")
		return
	}
	if _, created, err := h.walletRepository.Apply(r.Context(), req.CustomerID, req.AmountRub*100, "admin_credit", "admin-credit:"+uuid.NewString(), "Пополнение администратором"); err != nil || !created {
		slog.Error("mini app: credit admin user balance", "error", err, "customerId", utils.MaskHalfInt64(req.CustomerID))
		h.writeError(w, http.StatusInternalServerError, "admin_balance_failed", "Не удалось пополнить баланс")
		return
	}
	h.writeAdminUserActionResult(w, r, req.CustomerID, "Баланс пополнен")
}

func (h *Handler) handleAdminUserSubscription(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	var req adminUserActionRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil || req.CustomerID <= 0 || req.SubscriptionID <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Выберите подписку")
		return
	}
	if (req.Days <= 0) == (req.TrafficGB <= 0) {
		h.writeError(w, http.StatusBadRequest, "invalid_admin_subscription", "Укажите срок или трафик")
		return
	}
	if req.Days > 3650 || req.TrafficGB > 1000000 {
		h.writeError(w, http.StatusBadRequest, "invalid_admin_subscription", "Значение превышает допустимый лимит")
		return
	}
	target, subscription, err := h.adminUserSubscriptionTarget(r.Context(), req.CustomerID, req.SubscriptionID)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "admin_subscription_unavailable", err.Error())
		return
	}
	if target.customer.IsBlocked {
		h.writeError(w, http.StatusConflict, "admin_user_blocked", "Сначала разблокируйте пользователя")
		return
	}
	updated, err := h.remnawaveClient.AdjustUserAccess(r.Context(), target.state.UserID, target.state.UserUUID, req.Days, req.TrafficGB*adminUserTrafficGB)
	if err != nil {
		message := "Не удалось изменить подписку"
		if strings.Contains(err.Error(), "unlimited traffic") {
			message = "У этой подписки уже безлимитный трафик"
		}
		slog.Error("mini app: adjust admin user subscription", "error", err, "customerId", utils.MaskHalfInt64(req.CustomerID))
		h.writeError(w, http.StatusBadGateway, "admin_subscription_failed", message)
		return
	}
	if err := h.persistAdminSubscriptionPanelState(r.Context(), subscription, updated); err != nil {
		slog.Error("mini app: persist admin subscription", "error", err, "subscriptionId", subscription.ID)
		h.writeError(w, http.StatusInternalServerError, "admin_subscription_sync_failed", "Подписка изменена, но локальные данные не обновились")
		return
	}
	message := "Подписка продлена"
	if req.TrafficGB > 0 {
		message = "Трафик добавлен"
	}
	h.writeAdminUserActionResult(w, r, req.CustomerID, message)
}

func (h *Handler) handleAdminUserBlock(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	var req adminUserActionRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil || req.CustomerID <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Выберите пользователя")
		return
	}
	customer, err := h.customerRepository.FindById(r.Context(), req.CustomerID)
	if err != nil || customer == nil {
		h.writeError(w, http.StatusNotFound, "admin_user_not_found", "Пользователь не найден")
		return
	}
	if customer.TelegramID == sess.User.ID {
		h.writeError(w, http.StatusConflict, "admin_self_block", "Нельзя заблокировать свой аккаунт администратора")
		return
	}
	if !req.Blocked && config.GetBlockedTelegramIds()[customer.TelegramID] {
		h.writeError(w, http.StatusConflict, "admin_user_env_blocked", "Пользователь заблокирован в переменных окружения")
		return
	}
	if h.subscriptionRepository == nil || h.remnawaveClient == nil {
		h.writeError(w, http.StatusServiceUnavailable, "admin_subscription_unavailable", "Панель подписок недоступна")
		return
	}
	subscriptions, err := h.subscriptionRepository.ListByCustomer(r.Context(), customer.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "admin_user_failed", "Не удалось загрузить подписки")
		return
	}
	for i := range subscriptions {
		panelState, stateErr := h.panelStateForCustomerSubscription(r.Context(), customer, &subscriptions[i])
		if stateErr != nil {
			h.writeError(w, http.StatusBadGateway, "admin_subscription_failed", "Не удалось отключить подписки пользователя")
			return
		}
		if panelState == nil || !panelState.Exists {
			continue
		}
		if _, stateErr = h.remnawaveClient.SetUserBlocked(r.Context(), panelState.UserID, panelState.UserUUID, req.Blocked); stateErr != nil {
			slog.Error("mini app: set panel user block", "error", stateErr, "customerId", utils.MaskHalfInt64(customer.ID), "subscriptionId", subscriptions[i].ID)
			h.writeError(w, http.StatusBadGateway, "admin_subscription_failed", "Не удалось изменить доступ в панели")
			return
		}
	}
	if err := h.customerRepository.SetBlocked(r.Context(), customer.ID, req.Blocked); err != nil {
		h.writeError(w, http.StatusInternalServerError, "admin_block_failed", "Не удалось сохранить блокировку")
		return
	}
	message := "Пользователь разблокирован"
	if req.Blocked {
		message = "Пользователь и его подписки заблокированы"
	}
	h.writeAdminUserActionResult(w, r, req.CustomerID, message)
}

type adminSubscriptionTarget struct {
	customer *database.Customer
	state    *remnawave.UserState
}

func (h *Handler) adminUserSubscriptionTarget(ctx context.Context, customerID, subscriptionID int64) (*adminSubscriptionTarget, *database.CustomerSubscription, error) {
	if h.remnawaveClient == nil || h.subscriptionRepository == nil {
		return nil, nil, errors.New("Панель подписок недоступна")
	}
	customer, err := h.customerRepository.FindById(ctx, customerID)
	if err != nil || customer == nil {
		return nil, nil, errors.New("Пользователь не найден")
	}
	subscription, err := h.subscriptionRepository.FindForCustomer(ctx, customerID, subscriptionID)
	if err != nil || subscription == nil {
		return nil, nil, errors.New("Подписка не найдена")
	}
	state, err := h.panelStateForCustomerSubscription(ctx, customer, subscription)
	if err != nil || state == nil || !state.Exists {
		return nil, nil, errors.New("Подписка ещё не создана в панели")
	}
	return &adminSubscriptionTarget{customer: customer, state: state}, subscription, nil
}

func (h *Handler) persistAdminSubscriptionPanelState(ctx context.Context, subscription *database.CustomerSubscription, user *remnawave.PanelUser) error {
	if subscription == nil || user == nil {
		return errors.New("subscription state is missing")
	}
	link := strings.TrimSpace(user.SubscriptionURL)
	if link == "" && subscription.SubscriptionLink != nil {
		link = strings.TrimSpace(*subscription.SubscriptionLink)
	}
	return h.subscriptionRepository.UpdatePanelState(ctx, subscription, user.ID, user.UUID, link, user.ExpireAt)
}

func (h *Handler) writeAdminUserActionResult(w http.ResponseWriter, r *http.Request, customerID int64, message string) {
	payload, err := h.loadAdminUserDetail(r.Context(), customerID)
	if err != nil {
		slog.Error("mini app: refresh admin user after action", "error", err, "customerId", utils.MaskHalfInt64(customerID))
		h.writeError(w, http.StatusInternalServerError, "admin_user_refresh_failed", "Изменение сохранено, но карточка не обновилась")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": message, "data": payload})
}

func (h *Handler) loadAdminUserDetail(ctx context.Context, customerID int64) (*adminUserDetailPayload, error) {
	customer, err := h.customerRepository.FindById(ctx, customerID)
	if err != nil || customer == nil {
		return nil, err
	}
	result := &adminUserDetailPayload{
		CustomerID:    customer.ID,
		TelegramID:    customer.TelegramID,
		Username:      strings.TrimSpace(adminStringPointerValue(customer.TelegramUsername)),
		CreatedAt:     customer.CreatedAt.UTC().Format(time.RFC3339),
		IsBlocked:     customer.IsBlocked || config.GetBlockedTelegramIds()[customer.TelegramID],
		TrialUsed:     customer.TrialUsed,
		Subscriptions: []adminUserSubscriptionPayload{},
	}
	result.AvatarURL = adminUserAvatarURL(result.Username)
	if h.walletRepository != nil {
		result.Referrals.BalanceCents, err = h.walletRepository.Balance(ctx, customer.ID)
		if err != nil {
			return nil, err
		}
	}
	if h.referralRepository != nil && !customer.TelegramIDIsSynthetic {
		result.Referrals.Invited, err = h.referralRepository.CountByReferrer(ctx, customer.TelegramID)
		if err != nil {
			return nil, err
		}
		result.Referrals.Trial, err = h.referralRepository.CountGrantedRewards(ctx, customer.TelegramID, "trial")
		if err != nil {
			return nil, err
		}
		result.Referrals.Purchased, err = h.referralRepository.CountGrantedRewards(ctx, customer.TelegramID, "purchase")
		if err != nil {
			return nil, err
		}
	}
	if h.subscriptionRepository == nil {
		return result, nil
	}
	active, err := h.subscriptionRepository.ActiveForCustomer(ctx, customer)
	if err != nil {
		return nil, err
	}
	subscriptions, err := h.subscriptionRepository.ListByCustomer(ctx, customer.ID)
	if err != nil {
		return nil, err
	}
	for i := range subscriptions {
		subscription := &subscriptions[i]
		item := adminUserSubscriptionPayload{
			ID:          subscription.ID,
			Name:        subscription.DisplayName,
			IsPrimary:   subscription.IsPrimary,
			IsSelected:  active != nil && active.ID == subscription.ID,
			Status:      adminSubscriptionStatus(subscription.ExpireAt, result.IsBlocked),
			DeviceLimit: -1,
		}
		if subscription.ExpireAt != nil {
			item.ExpiresAt = subscription.ExpireAt.UTC().Format(time.RFC3339)
		}
		if h.remnawaveClient != nil {
			panelState, stateErr := h.panelStateForCustomerSubscription(ctx, customer, subscription)
			if stateErr != nil {
				slog.Warn("mini app: load panel state for admin user", "error", stateErr, "subscriptionId", subscription.ID)
				if item.Status != "blocked" {
					item.Status = "unavailable"
				}
			} else if panelState != nil && panelState.Exists {
				item.TrafficLimitBytes = panelState.TrafficLimitBytes
				item.UsedTrafficBytes = panelState.UsedTrafficBytes
				item.DeviceLimit = panelState.DeviceLimit
				item.UsedDevices = panelState.UsedDevices
				if panelState.ExpireAt != nil {
					item.ExpiresAt = panelState.ExpireAt.UTC().Format(time.RFC3339)
				}
				if result.IsBlocked {
					item.Status = "blocked"
				} else if panelState.Active {
					item.Status = "active"
				} else {
					item.Status = "expired"
				}
			}
		}
		result.Subscriptions = append(result.Subscriptions, item)
	}
	return result, nil
}

func adminStringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
