package miniapp

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/integrations"
	"link-bot/internal/runtimeconfig"
)

type adminPermission struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Group    string   `json:"group"`
	Requires []string `json:"requires,omitempty"`
}

var adminPermissions = []adminPermission{
	{"status", "Статус", "Система", nil}, {"localization", "Язык и шрифт", "Система", nil},
	{"maintenance", "Режим аварии", "Система", nil}, {"diagnostics", "Диагностика", "Система", nil},
	{"push", "Push-уведомления", "Система", nil}, {"features", "Управление функциями", "Система", nil},
	{"trial", "Триал", "Система", nil}, {"grace", "Доступ после окончания", "Система", nil},
	{"subscriptions", "Привязка подписок", "Система", nil}, {"integrations", "Платёжные интеграции", "Система", nil},
	{"moynalog", "Мой налог", "Система", nil}, {"ai", "ИИ", "Система", nil},
	{"smtp", "Почта / SMTP", "Система", nil},
	{"content", "Редактор контента", "Интерфейс", nil}, {"subpage", "Sub page", "Интерфейс", nil},
	{"appearance", "Оформление", "Интерфейс", nil}, {"layout", "Конструктор UI", "Интерфейс", nil},
	{"plans", "Тарифы", "Интерфейс", nil},
	{"users", "Просмотр пользователей", "Операции", nil}, {"finance", "Финансы", "Операции", nil},
	{"analytics", "Аналитика", "Операции", nil}, {"referrals", "Рефералы и баланс", "Операции", nil},
	{"partners", "Партнёры", "Операции", nil}, {"broadcast", "Рассылка", "Операции", nil}, {"promocodes", "Промокоды", "Операции", nil},
	{"users.balance", "Изменять баланс", "Действия с пользователями", []string{"users"}},
	{"users.subscription", "Изменять и удалять подписки", "Действия с пользователями", []string{"users"}},
	{"users.block", "Блокировать пользователей", "Действия с пользователями", []string{"users"}},
	{"users.message", "Отправлять личные сообщения", "Действия с пользователями", []string{"users"}},
	{"support.view", "Просматривать все обращения", "Поддержка", nil},
	{"support.reply", "Отвечать на обращения", "Поддержка", []string{"support.view"}},
	{"support.close", "Закрывать обращения", "Поддержка", []string{"support.view"}},
	{"servers.view", "Просматривать скрытые ноды", "Другие действия", nil},
	{"servers.manage", "Скрывать и показывать ноды", "Другие действия", []string{"servers.view"}},
	{"reviews.rewards", "Настраивать награды за отзывы", "Другие действия", nil},
	{"reviews.delete", "Удалять отзывы", "Другие действия", nil},
	{"wallet.withdrawals", "Обрабатывать вывод средств", "Другие действия", []string{"referrals"}},
}

type adminAccess struct {
	IsAdmin     bool     `json:"isAdmin"`
	IsOwner     bool     `json:"isOwner"`
	Role        string   `json:"role"`
	Color       string   `json:"color"`
	Permissions []string `json:"permissions"`
}
type adminAccessContextKey struct{}
type administratorLookup interface {
	AdministratorForTelegramID(context.Context, int64) (*database.Administrator, error)
}

func ownerAccess() adminAccess {
	return adminAccess{IsAdmin: true, IsOwner: true, Role: "Главный администратор", Color: "#69a8d4", Permissions: []string{}}
}
func (a adminAccess) can(id string) bool {
	if a.IsOwner {
		return true
	}
	if !a.IsAdmin || id == "administrators" {
		return false
	}
	found := false
	for _, p := range a.Permissions {
		if p == id {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	for _, p := range adminPermissions {
		if p.ID == id {
			for _, dep := range p.Requires {
				if !a.can(dep) {
					return false
				}
			}
			return true
		}
	}
	return false
}
func (s *session) access() adminAccess {
	if s == nil {
		return adminAccess{}
	}
	if s.User.ID != 0 && s.User.ID == config.GetAdminTelegramId() {
		return ownerAccess()
	}
	return s.AdminAccess
}
func (s *session) isAdministrator() bool   { return s.access().IsAdmin }
func (s *session) canAdmin(id string) bool { return s.access().can(id) }
func (h *Handler) resolveAdminAccess(ctx context.Context, sess *session) error {
	if sess.access().IsOwner {
		sess.AdminAccess = ownerAccess()
		return nil
	}
	sess.AdminAccess = adminAccess{Permissions: []string{}}
	lookup := h.administratorRepository
	if lookup == nil && h.customerRepository != nil {
		lookup = h.customerRepository
	}
	if lookup == nil {
		return nil
	}
	a, err := lookup.AdministratorForTelegramID(ctx, sess.User.ID)
	if err != nil {
		return err
	}
	if a != nil {
		sess.AdminAccess = adminAccess{IsAdmin: true, Role: a.Role, Color: a.Color, Permissions: a.Permissions}
	}
	return nil
}

// Bot message capture shares the same current permissions as the mini app.
func (h *Handler) HasAdminPermission(ctx context.Context, telegramID int64, permissions ...string) bool {
	if telegramID == 0 || config.GetBlockedTelegramIds()[telegramID] {
		return false
	}
	sess := &session{User: telegramUser{ID: telegramID}}
	if h.resolveAdminAccess(ctx, sess) != nil {
		return false
	}
	for _, permission := range permissions {
		if sess.canAdmin(permission) {
			return true
		}
	}
	return false
}

var adminSettingsFields = map[string][]string{
	"localization": {"localization"}, "maintenance": {"maintenance"}, "features": {"features"},
	"content": {"content", "panel"}, "appearance": {"appearance"}, "layout": {"layout"}, "subpage": {"subPage"},
	"plans": {"plans", "devicePacks", "trafficPacks", "deviceAccess", "paymentMethodOrder"},
	"trial": {"trial"}, "referrals": {"referrals"}, "grace": {"grace"},
}

// Unknown admin routes are denied by default, including future endpoints.
func adminRouteAllowed(a adminAccess, path string) bool {
	if !strings.HasPrefix(path, "/api/mini-app/admin/") {
		return true
	}
	if a.IsOwner {
		return true
	}
	if !a.IsAdmin {
		return false
	}
	route := strings.TrimPrefix(path, "/api/mini-app/admin/")
	switch route {
	case "settings/update":
		for section := range adminSettingsFields {
			if a.can(section) {
				return true
			}
		}
		return false
	case "integrations/update":
		return a.can("integrations") || a.can("analytics") || a.can("moynalog")
	case "status", "finance", "analytics":
		return a.can(route)
	case "reviews/rewards":
		return a.can("reviews.rewards")
	case "reviews/delete":
		return a.can("reviews.delete")
	case "servers/visibility":
		return a.can("servers.manage")
	case "wallet/withdrawal/resolve":
		return a.can("wallet.withdrawals")
	case "logo/upload", "favicon/upload", "banner/upload", "reminders/test", "success/test", "payment-notifications/test", "gifts/test":
		return a.can("content")
	case "events/resolve":
		return a.can("diagnostics")
	case "users/search", "users/detail":
		return a.can("users")
	case "users/balance":
		return a.can("users.balance")
	case "users/block":
		return a.can("users.block")
	case "users/subscription", "users/subscription/select", "users/subscription/delete", "users/subscription/reissue":
		return a.can("users.subscription")
	}
	for permission, routes := range map[string][]string{
		"ai":            {"ai/settings", "ai/models", "ai/update", "ai/toggle"},
		"smtp":          {"smtp/settings", "smtp/update", "smtp/check", "smtp/test"},
		"promocodes":    {"promocodes/create", "promocodes/validate", "promocodes/delete"},
		"subscriptions": {"subscriptions/find", "subscriptions/target", "subscriptions/rebind"},
		"moynalog":      {"moynalog/state", "moynalog/test", "moynalog/retry"},
		"partners":      {"partners/state", "partners/review", "partners/create", "partners/update"},
		"push":          {"push/state", "push/subscribe", "push/unsubscribe", "push/test"},
		"broadcast":     {"broadcast/state", "broadcast/capture/start", "broadcast/buttons", "broadcast/preview", "broadcast/send", "broadcast/reset", "broadcast/email/state", "broadcast/email/save", "broadcast/email/preview", "broadcast/email/send"},
		"users.message": {"users/message/state", "users/message/capture", "users/message/preview", "users/message/send"},
	} {
		for _, knownRoute := range routes {
			if route == knownRoute {
				return a.can(permission)
			}
		}
	}
	return false
}

func adminIntegrationPermission(provider string) string {
	switch strings.TrimSpace(provider) {
	case "google_analytics", "yandex_metrika":
		return "analytics"
	case integrations.ProviderMoyNalog:
		return "moynalog"
	case integrations.ProviderSupportAI:
		return "ai"
	case integrations.ProviderSMTP:
		return "smtp"
	default:
		return "integrations"
	}
}

func validateAdministrator(a *database.Administrator) error {
	a.Role = strings.TrimSpace(a.Role)
	a.Color = strings.ToLower(strings.TrimSpace(a.Color))
	if a.CustomerID <= 0 || utf8.RuneCountInString(a.Role) < 1 || utf8.RuneCountInString(a.Role) > 60 || strings.ContainsAny(a.Role, "\r\n\t") {
		return errors.New("укажите пользователя и название роли до 60 символов")
	}
	if !regexp.MustCompile(`^#[0-9a-f]{6}$`).MatchString(a.Color) {
		return errors.New("выберите цвет роли")
	}
	known := map[string]adminPermission{}
	for _, p := range adminPermissions {
		known[p.ID] = p
	}
	chosen := map[string]bool{}
	for _, id := range a.Permissions {
		if _, ok := known[id]; !ok {
			return errors.New("неизвестное право доступа")
		}
		chosen[id] = true
	}
	for id := range chosen {
		for _, dep := range known[id].Requires {
			if !chosen[dep] {
				return errors.New("для выбранного действия нужен доступ к разделу: " + known[dep].Label)
			}
		}
	}
	a.Permissions = []string{}
	for _, p := range adminPermissions {
		if chosen[p.ID] {
			a.Permissions = append(a.Permissions, p.ID)
		}
	}
	if len(a.Permissions) == 0 {
		return errors.New("выберите хотя бы одно право доступа")
	}
	return nil
}

func (h *Handler) handleAdministrators(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.access().IsOwner {
		h.writeError(w, http.StatusForbidden, "forbidden", "Только главный администратор может назначать роли")
		return
	}
	if h.customerRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "unavailable", "Пользователи недоступны")
		return
	}
	switch r.URL.Path {
	case "/api/mini-app/admin/administrators/save":
		var a database.Administrator
		if err := h.decodeJSONRequest(w, r, 16<<10, &a); err != nil {
			h.writeError(w, 400, "invalid_request", "Некорректная роль")
			return
		}
		if err := validateAdministrator(&a); err != nil {
			h.writeError(w, 400, "invalid_role", err.Error())
			return
		}
		target, err := h.customerRepository.FindById(r.Context(), a.CustomerID)
		if err != nil || target == nil || target.TelegramID == config.GetAdminTelegramId() || target.IsBlocked || config.GetBlockedTelegramIds()[target.TelegramID] {
			h.writeError(w, 400, "invalid_administrator", "Этот пользователь недоступен для назначения")
			return
		}
		if err := h.customerRepository.SaveAdministrator(r.Context(), a, sess.User.ID); err != nil {
			h.writeError(w, 500, "save_failed", "Не удалось сохранить роль")
			return
		}
	case "/api/mini-app/admin/administrators/remove":
		var req struct {
			CustomerID int64 `json:"customerId"`
		}
		if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil || req.CustomerID <= 0 {
			h.writeError(w, 400, "invalid_request", "Некорректный пользователь")
			return
		}
		if err := h.customerRepository.RemoveAdministrator(r.Context(), req.CustomerID, sess.User.ID); err != nil {
			h.writeError(w, 400, "remove_failed", err.Error())
			return
		}
	case "/api/mini-app/admin/administrators/list":
		var req adminUserSearchRequest
		if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil || utf8.RuneCountInString(req.Query) > 100 {
			h.writeError(w, 400, "invalid_request", "Некорректный поиск")
			return
		}
		if req.Limit <= 0 || req.Limit > 50 {
			req.Limit = 30
		}
		if req.Offset < 0 {
			req.Offset = 0
		}
		items, total, err := h.customerRepository.ListAdministrators(r.Context(), sess.User.ID, req.Query, req.Limit, req.Offset)
		if err != nil {
			h.writeError(w, 500, "list_failed", "Не удалось загрузить администраторов")
			return
		}
		h.writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"items": items, "total": total, "permissions": adminPermissions}})
		return
	default:
		h.writeError(w, 404, "not_found", "Not found")
		return
	}
	if h.realtime != nil {
		h.realtime.publish()
	}
	h.writeJSON(w, 200, map[string]any{"ok": true})
}

// Public runtime settings already contain the appearance/content fields; private
// operational collections and integration configuration are loaded only with access.
func accessFromContext(ctx context.Context) adminAccess {
	if a, ok := ctx.Value(adminAccessContextKey{}).(adminAccess); ok {
		return a
	}
	return adminAccess{}
}

func mergeAdminSettings(current, next runtimeconfig.Settings, section string) (runtimeconfig.Settings, error) {
	fields, ok := adminSettingsFields[section]
	if !ok {
		return current, errors.New("unknown settings section")
	}
	return runtimeconfig.MergeSettingsFields(current, next, fields)
}

func adminSettingsForAccess(settings runtimeconfig.Settings, access adminAccess) runtimeconfig.Settings {
	if !access.can("servers.view") {
		settings.HiddenServerNodes = nil
	}
	return settings
}
