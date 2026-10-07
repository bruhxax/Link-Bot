package miniapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"link-bot/internal/database"
)

type adminActivityRepository interface {
	CreateAdminActivity(context.Context, database.AdminActivity) (int64, error)
	FinishAdminActivity(context.Context, database.AdminActivity) error
	ListAdminActivity(context.Context, database.AdminActivityQuery) ([]database.AdminActivity, bool, error)
}
type activityContextKey struct{}
type activityCapture struct {
	entry          database.AdminActivity
	settingsBefore map[string]any
	section        string
}
type activityAction struct{ title, category, fields string }

// Polling/state reads do not drown out actual work. All other admin endpoints
// are recorded by default, so newly added mutations cannot silently escape it.
var activityReadRoutes = map[string]bool{
	"status": true, "finance": true, "analytics": true, "ai/settings": true, "ai/models": true,
	"smtp/settings": true, "moynalog/state": true, "partners/state": true, "push/state": true,
	"broadcast/state": true, "broadcast/email/state": true, "users/message/state": true,
	"users/search": true, "administrators/list": true, "administrators/logs": true,
	"users/detail": true, "subscriptions/find/refresh": true, "subscriptions/target/refresh": true,
}
var activityActions = map[string]activityAction{
	"settings/update":            {"Изменил настройки", "settings", "section"},
	"users/open":                 {"Открыл карточку пользователя", "users", "customerId"},
	"users/balance":              {"Изменил баланс пользователя", "users", "customerId balanceAction amountRub"},
	"users/block":                {"Изменил блокировку пользователя", "users", "customerId blocked reason deleteSubscription"},
	"users/subscription":         {"Изменил подписку пользователя", "users", "customerId subscriptionId days trafficGb"},
	"users/subscription/select":  {"Выбрал подписку пользователя", "users", "customerId subscriptionId"},
	"users/subscription/delete":  {"Удалил подписку пользователя", "users", "customerId subscriptionId"},
	"users/subscription/reissue": {"Перевыпустил подписку пользователя", "users", "customerId subscriptionId"},
	"users/message/capture":      {"Начал создание личного сообщения", "communication", "customerId"},
	"users/message/preview":      {"Проверил личное сообщение", "communication", "customerId"},
	"users/message/send":         {"Отправил личное сообщение", "communication", "customerId"},
	"servers/visibility":         {"Изменил видимость ноды", "servers", "id hidden"},
	"support/send":               {"Ответил на обращение", "support", "ticketId message"},
	"support/send-media":         {"Отправил вложение в обращение", "support", "ticketId"},
	"support/close":              {"Закрыл обращение", "support", "ticketId"},
	"support/operator":           {"Взял обращение в работу", "support", "ticketId release"},
	"reviews/rewards":            {"Изменил вознаграждение за отзывы", "settings", ""},
	"reviews/delete":             {"Удалил отзыв", "users", "id"},
	"wallet/withdrawal/resolve":  {"Обработал вывод средств", "users", "id approve"},
	"promocodes/create":          {"Создал промокод", "settings", "code discountPercent rewardType rewardValue rewardTrafficGb maxRedemptions expiresAt"},
	"promocodes/delete":          {"Удалил промокод", "settings", "id code"},
	"promocodes/validate":        {"Проверил промокод", "settings", "code"},
	"subscriptions/find":         {"Нашёл подписку в панели", "users", "query"},
	"subscriptions/target":       {"Проверил пользователя для привязки подписки", "users", "targetTelegramId userId"},
	"subscriptions/rebind":       {"Перепривязал подписку", "users", "targetTelegramId targetSubscriptionId userId"},
	"integrations/update":        {"Изменил интеграцию", "settings", "provider enabled fields"},
	"ai/update":                  {"Изменил настройки ИИ", "settings", "enabled model handoffAfter apiKey prompt"},
	"ai/toggle":                  {"Переключил ИИ", "settings", "enabled"},
	"smtp/update":                {"Изменил настройки почты", "settings", "host port user from enabled password proxyUrl clearProxy"},
	"smtp/check":                 {"Проверил подключение почты", "communication", "host port"},
	"smtp/test":                  {"Отправил тестовое письмо", "communication", "email"},
	"broadcast/capture/start":    {"Начал создание рассылки", "communication", ""},
	"broadcast/buttons":          {"Изменил кнопки рассылки", "communication", "buttons"},
	"broadcast/preview":          {"Проверил рассылку", "communication", ""},
	"broadcast/send":             {"Запустил рассылку в Telegram", "communication", ""},
	"broadcast/reset":            {"Очистил черновик рассылки", "communication", ""},
	"broadcast/email/save":       {"Сохранил письмо для рассылки", "communication", "subject body"},
	"broadcast/email/preview":    {"Отправил тестовую email-рассылку", "communication", "email"},
	"broadcast/email/send":       {"Запустил email-рассылку", "communication", ""},
	"partners/review":            {"Рассмотрел заявку партнёра", "users", "applicationId approve percent"},
	"partners/create":            {"Добавил партнёра", "users", "telegramId percent"},
	"partners/update":            {"Изменил партнёра", "users", "partnerId percent active action"},
	"administrators/save":        {"Изменил роль администратора", "access", "customerId role color permissions"},
	"administrators/remove":      {"Снял права администратора", "access", "customerId"},
	"logo/upload":                {"Загрузил логотип", "settings", ""}, "favicon/upload": {"Загрузил иконку сайта", "settings", ""},
	"banner/upload": {"Загрузил баннер", "settings", ""}, "events/resolve": {"Закрыл событие диагностики", "system", "id"},
	"moynalog/test": {"Проверил подключение «Мой налог»", "system", ""}, "moynalog/retry": {"Повторил отправку чека", "system", "purchaseId"},
	"push/subscribe": {"Включил push-уведомления", "system", ""}, "push/unsubscribe": {"Выключил push-уведомления", "system", ""},
	"push/test":                  {"Отправил тестовое push-уведомление", "system", ""},
	"reminders/test":             {"Проверил напоминание о подписке", "communication", "kind"},
	"success/test":               {"Проверил уведомление об успешной оплате", "communication", "kind"},
	"payment-notifications/test": {"Проверил уведомление об оплате", "communication", ""},
	"gifts/test":                 {"Проверил уведомление о подарке", "communication", "kind"},
}

func activityRoute(path string) (string, bool) {
	if strings.HasPrefix(path, "/api/mini-app/admin/") {
		route := strings.TrimPrefix(path, "/api/mini-app/admin/")
		return route, !activityReadRoutes[route]
	}
	route := strings.TrimPrefix(path, "/api/mini-app/")
	_, ok := activityActions[route]
	return route, ok && strings.HasPrefix(route, "support/")
}
func (h *Handler) activityRepository() adminActivityRepository {
	if h.adminActivityRepository != nil {
		return h.adminActivityRepository
	}
	if h.customerRepository != nil {
		return h.customerRepository
	}
	return nil
}

type activityResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *activityResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *activityResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
func (w *activityResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (h *Handler) runAdminActivity(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer, next func(http.ResponseWriter, *http.Request, *session, *database.Customer)) {
	route, record := activityRoute(r.URL.Path)
	repo := h.activityRepository()
	if !record || !sess.isAdministrator() || repo == nil {
		next(w, r, sess, customer)
		return
	}
	// Serialize runtime-setting writes across sections: another administrator's
	// concurrent change must never be attributed to this request's before/after.
	if route == "settings/update" || route == "reviews/rewards" || route == "servers/visibility" {
		h.activitySettingsMu.Lock()
		defer h.activitySettingsMu.Unlock()
	}
	info, ok := activityActions[route]
	if !ok {
		info = activityAction{"Выполнил действие в админке", "system", ""}
	}
	capture := &activityCapture{entry: database.AdminActivity{ActorTelegramID: sess.User.ID, ActorName: strings.TrimSpace(sess.User.Username), ActorRole: sess.access().Role, Action: route, Category: info.category, Title: info.title, Status: "pending"}}
	if (route == "settings/update" || route == "reviews/rewards") && h.runtimeSettings != nil {
		capture.settingsBefore = activityObject(h.runtimeSettings.Snapshot())
	}
	id, err := repo.CreateAdminActivity(r.Context(), capture.entry)
	if err != nil {
		slog.Error("mini app: start administrator activity", "error", err)
		h.writeError(w, 503, "activity_unavailable", "Не удалось записать действие в журнал. Попробуйте ещё раз")
		return
	}
	capture.entry.ID = id
	r = r.WithContext(context.WithValue(r.Context(), activityContextKey{}, capture))
	writer := &activityResponseWriter{ResponseWriter: w}
	defer func() {
		capture.entry.Status = "failed"
		if writer.status >= 200 && writer.status < 300 {
			capture.entry.Status = "success"
		}
		if (route == "settings/update" || route == "reviews/rewards") && capture.entry.Status == "success" && h.runtimeSettings != nil {
			after := activityObject(h.runtimeSettings.Snapshot())
			fields := adminSettingsFields[capture.section]
			if route == "reviews/rewards" {
				fields = []string{"reviewRewards"}
			} else if capture.section == "" {
				for key := range after {
					if key != "version" {
						fields = append(fields, key)
					}
				}
				sort.Strings(fields)
			}
			for _, field := range fields {
				activityDiff(&capture.entry.Details, field, capture.settingsBefore[field], after[field])
			}
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		defer cancel()
		if err := repo.FinishAdminActivity(ctx, capture.entry); err != nil {
			slog.Error("mini app: finish administrator activity", "activityId", id, "error", err)
		}
	}()
	next(writer, r, sess, customer)
}

func activityObject(value any) map[string]any {
	data, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(data, &result)
	return result
}

func (h *Handler) captureAdminActivityPayload(r *http.Request, target any) {
	capture, _ := r.Context().Value(activityContextKey{}).(*activityCapture)
	if capture == nil {
		return
	}
	data := activityObject(target)
	entry := &capture.entry
	info := activityActions[entry.Action]
	for _, key := range strings.Fields(info.fields) {
		value, exists := data[key]
		if !exists {
			continue
		}
		if key == "section" {
			capture.section, _ = value.(string)
			continue
		}
		if key == "customerId" {
			entry.TargetCustomerID = activityInt(value)
			continue
		}
		if key == "targetTelegramId" || key == "telegramId" {
			entry.TargetTelegramID = activityInt(value)
			continue
		}
		if activitySensitive(key) {
			if value != "" && value != nil {
				entry.Details = append(entry.Details, database.AdminActivityDetail{Label: activityLabel(key), Value: "Обновлено"})
			}
			continue
		}
		if key == "fields" {
			if fields, ok := value.(map[string]any); ok {
				names := make([]string, 0, len(fields))
				for name := range fields {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					entry.Details = append(entry.Details, database.AdminActivityDetail{Label: activityLabel(name), Value: "Обновлено"})
				}
			}
			continue
		}
		if key == "buttons" {
			activityDiff(&entry.Details, "buttons", nil, value)
			continue
		}
		if key == "permissions" {
			if values, ok := value.([]any); ok {
				names := make([]string, 0, len(values))
				for _, id := range values {
					for _, permission := range adminPermissions {
						if permission.ID == id {
							names = append(names, permission.Label)
						}
					}
				}
				entry.Details = append(entry.Details, database.AdminActivityDetail{Label: "Права доступа", Value: strings.Join(names, ", ")})
			}
			continue
		}
		if key == "message" || key == "body" || key == "prompt" {
			entry.Details = append(entry.Details, database.AdminActivityDetail{Label: activityLabel(key), Value: fmt.Sprintf("%d символов", utf8.RuneCountInString(fmt.Sprint(value)))})
			continue
		}
		if key == "query" && strings.Contains(fmt.Sprint(value), "://") {
			entry.Details = append(entry.Details, database.AdminActivityDetail{Label: "Поиск", Value: "По ссылке"})
			continue
		}
		if key == "balanceAction" {
			label := map[string]string{"": "Пополнение", "credit": "Пополнение", "debit": "Списание"}[fmt.Sprint(value)]
			if label != "" {
				value = label
			}
		}
		entry.Details = append(entry.Details, database.AdminActivityDetail{Label: activityLabel(key), Value: activityValue(value)})
	}
	if entry.Action == "users/block" {
		if data["blocked"] == true {
			entry.Title = "Заблокировал пользователя"
		} else {
			entry.Title = "Разблокировал пользователя"
		}
	}
	if entry.Action == "support/operator" && data["release"] == true {
		entry.Title = "Освободил обращение"
	}
	if entry.Action == "partners/review" {
		if data["approve"] == true {
			entry.Title = "Одобрил заявку партнёра"
		} else {
			entry.Title = "Отклонил заявку партнёра"
		}
	}
	if entry.Action == "wallet/withdrawal/resolve" {
		if data["approve"] == true {
			entry.Title = "Одобрил вывод средств"
		} else {
			entry.Title = "Отклонил вывод средств"
		}
	}
	if entry.Action == "servers/visibility" {
		if data["hidden"] == true {
			entry.Title = "Скрыл ноду"
		} else {
			entry.Title = "Показал ноду"
		}
	}
	if capture.section != "" {
		entry.Title = "Изменил настройки: " + activityLabel(capture.section)
	}
	if ticketID := activityInt(data["ticketId"]); ticketID > 0 && h.supportRepository != nil {
		if ticket, err := h.supportRepository.FindTicketByID(r.Context(), ticketID); err == nil && ticket != nil {
			entry.TargetCustomerID = ticket.CustomerID
		}
	}
	if h.customerRepository != nil {
		var target *database.Customer
		if entry.TargetCustomerID > 0 {
			target, _ = h.customerRepository.FindById(r.Context(), entry.TargetCustomerID)
		} else if entry.TargetTelegramID > 0 {
			target, _ = h.customerRepository.FindByTelegramId(r.Context(), entry.TargetTelegramID)
		}
		if target != nil {
			entry.TargetTelegramID = target.TelegramID
			if target.TelegramUsername != nil {
				entry.TargetName = *target.TelegramUsername
			}
		}
	}
}

func activityInt(value any) int64 {
	if n, ok := value.(float64); ok {
		return int64(n)
	}
	n, _ := strconv.ParseInt(fmt.Sprint(value), 10, 64)
	return n
}
func activitySensitive(key string) bool {
	key = strings.ToLower(key)
	for _, part := range []string{"password", "secret", "token", "apikey", "initdata", "logindata", "useruuid", "subscriptionlink", "proxyurl", "privatekey"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}
func activityValue(value any) string {
	if n, ok := value.(float64); ok {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	if value == nil {
		return "Не задано"
	}
	if b, ok := value.(bool); ok {
		if b {
			return "Включено"
		}
		return "Выключено"
	}
	if values, ok := value.([]any); ok {
		parts := make([]string, 0, len(values))
		for _, v := range values {
			parts = append(parts, activityValue(v))
		}
		return strings.Join(parts, ", ")
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return "Не задано"
	}
	if len([]rune(text)) > 500 {
		text = string([]rune(text)[:500]) + "…"
	}
	return text
}
func activityDiff(details *[]database.AdminActivityDetail, path string, before, after any) {
	if activitySensitive(path) {
		return
	}
	oldJSON, _ := json.Marshal(before)
	newJSON, _ := json.Marshal(after)
	if string(oldJSON) == string(newJSON) {
		return
	}
	if strings.Contains(strings.ToLower(path), "url") || strings.Contains(strings.ToLower(path), "link") {
		*details = append(*details, database.AdminActivityDetail{Label: activityLabel(path), Value: "Изменено"})
		return
	}
	oldMap, _ := before.(map[string]any)
	newMap, ok := after.(map[string]any)
	if ok || oldMap != nil {
		keys := map[string]bool{}
		for k := range oldMap {
			keys[k] = true
		}
		for k := range newMap {
			keys[k] = true
		}
		ordered := make([]string, 0, len(keys))
		for k := range keys {
			ordered = append(ordered, k)
		}
		sort.Strings(ordered)
		for _, k := range ordered {
			activityDiff(details, path+"."+k, oldMap[k], newMap[k])
		}
		return
	}
	oldList, _ := before.([]any)
	newList, ok := after.([]any)
	if ok || oldList != nil {
		length := len(oldList)
		if len(newList) > length {
			length = len(newList)
		}
		for i := 0; i < length; i++ {
			var a, b any
			if i < len(oldList) {
				a = oldList[i]
			}
			if i < len(newList) {
				b = newList[i]
			}
			activityDiff(details, fmt.Sprintf("%s.%d", path, i+1), a, b)
		}
		return
	}
	*details = append(*details, database.AdminActivityDetail{Label: activityLabel(path), Before: activityValue(before), After: activityValue(after)})
}

func activityLabel(path string) string {
	labels := map[string]string{
		"bottomNavStyle": "Дизайн нижнего меню",
		"devicePacks":    "Пакеты устройств", "trafficPacks": "Пакеты трафика", "deviceAccess": "Устройства и уведомления", "paymentMethodOrder": "Порядок способов оплаты", "hiddenServerNodes": "Скрытые ноды", "panel": "Панель", "maxRedemptions": "Лимит использований", "rewardType": "Тип награды", "rewardValue": "Размер награды", "titleRu": "Заголовок", "textRu": "Текст", "reasonRu": "Причина", "fontFamily": "Шрифт", "labelRu": "Название", "hintRu": "Описание", "mini_app": "Мини-приложение", "expiryDays": "Срок действия, дней", "newDeviceNotification": "Уведомления о новых устройствах", "expiryNotification": "Напоминания об окончании", "deviceLimitNotification": "Уведомления о лимите устройств", "expiryTemplate": "Текст напоминания", "brandName": "Название бренда", "logoUrl": "Логотип", "faviconUrl": "Иконка сайта", "speed": "Скорость", "dimming": "Затемнение", "url": "Ссылка", "amount": "Сумма", "template": "Шаблон", "internalSquads": "Внутренние группы", "externalSquad": "Внешняя группа", "settings": "Настройки", "localization": "Язык и шрифт", "maintenance": "Режим аварии", "features": "Функции", "content": "Контент", "appearance": "Оформление", "layout": "Конструктор UI", "subPage": "Sub page", "subpage": "Sub page", "plans": "Тарифы", "trial": "Триал", "referrals": "Реферальная система", "grace": "Доступ после окончания", "reviewRewards": "Награды за отзывы",
		"enabled": "Включение", "active": "Активность", "hidden": "Скрытая нода", "blocked": "Блокировка", "approve": "Одобрение", "release": "Освобождение", "deleteSubscription": "Удалить подписку", "reason": "Причина", "amountRub": "Сумма, ₽", "balanceAction": "Действие с балансом", "days": "Дни", "trafficGb": "Трафик, ГБ", "subscriptionId": "Подписка №", "targetSubscriptionId": "Целевая подписка №", "ticketId": "Обращение №", "purchaseId": "Покупка №", "applicationId": "Заявка №", "partnerId": "Партнёр №", "userId": "Пользователь панели №", "id": "Объект №", "query": "Поиск", "role": "Роль", "color": "Цвет", "permissions": "Права", "code": "Промокод", "kind": "Тип", "provider": "Интеграция", "fields": "Поля", "host": "SMTP-сервер", "port": "Порт", "user": "Логин", "from": "Отправитель", "password": "Пароль", "apiKey": "API-ключ", "proxyUrl": "Прокси", "clearProxy": "Сброс прокси", "email": "Получатель", "subject": "Тема письма", "body": "Текст письма", "message": "Сообщение", "prompt": "Инструкция ИИ", "buttons": "Кнопки", "model": "Модель", "handoffAfter": "Ответов до оператора", "percent": "Процент", "action": "Действие", "discountPercent": "Скидка, %", "maxUses": "Лимит использований", "expiresAt": "Срок действия", "planMonths": "Месяцы", "rewardDays": "Бонусные дни", "rewardTrafficGb": "Бонусный трафик, ГБ", "rewardBalanceRub": "Бонус на баланс, ₽",
		"colors": "Цвета", "background": "Фон", "accent": "Акцент", "text": "Текст", "muted": "Вторичный текст", "border": "Рамки", "button": "Кнопки", "buttonText": "Текст кнопок", "icon": "Иконки", "backgroundMode": "Тип фона", "showFrames": "Рамки", "glow": "Свечение", "glass": "Стекло", "language": "Язык", "fontFamilyRu": "Русский шрифт", "fontFamilyEn": "Английский шрифт", "name": "Название", "visible": "Видимость", "price": "Цена", "priceRub": "Цена, ₽", "months": "Месяцы", "trafficLimitGb": "Лимит трафика, ГБ", "deviceLimit": "Лимит устройств", "elements": "Элементы", "pages": "Страницы", "dashboard": "Главная", "support": "Поддержка", "servers": "Серверы", "buy": "Тарифы", "balanceRub": "Баланс, ₽", "support_ai": "ИИ поддержки", "smtp": "Почта", "servers.view": "Список нод", "support.reply": "Ответы на обращения",
	}
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if label, ok := labels[part]; ok {
			parts[i] = label
		} else if _, err := strconv.Atoi(part); err == nil {
			parts[i] = "№" + part
		}
	}
	return strings.Join(parts, " → ")
}

func (h *Handler) handleAdminActivity(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.access().IsOwner {
		h.writeError(w, 403, "forbidden", "Журнал доступен только главному администратору")
		return
	}
	repo := h.activityRepository()
	if repo == nil {
		h.writeError(w, 503, "unavailable", "Журнал недоступен")
		return
	}
	var q database.AdminActivityQuery
	if err := h.decodeJSONRequest(w, r, 4096, &q); err != nil || q.TelegramID <= 0 || q.BeforeID < 0 || utf8.RuneCountInString(q.Query) > 100 {
		h.writeError(w, 400, "invalid_request", "Некорректный запрос журнала")
		return
	}
	if q.Limit <= 0 || q.Limit > 50 {
		q.Limit = 30
	}
	switch q.Category {
	case "", "settings", "users", "support", "communication", "servers", "access", "system":
	default:
		h.writeError(w, 400, "invalid_request", "Некорректный фильтр")
		return
	}
	q.Query = strings.TrimSpace(q.Query)
	items, more, err := repo.ListAdminActivity(r.Context(), q)
	if err != nil {
		h.writeError(w, 500, "list_failed", "Не удалось загрузить журнал")
		return
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"items": items, "hasMore": more}})
}
