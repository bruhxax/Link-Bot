package miniapp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

type userActivityRepository interface {
	RecordUserActivity(context.Context, database.UserActivity) error
	ListUserActivity(context.Context, int64, int64, int) ([]database.UserActivity, bool, error)
}
type userActivityContextKey struct{}

type userActivityResponseWriter struct {
	activityResponseWriter
	code string
}

func (w *userActivityResponseWriter) RecordActivityError(code string) {
	if len(code) > 100 {
		return
	}
	for _, c := range code {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return
		}
	}
	w.code = code
}

// Only deliberate actions are logged. Bootstrap, polling, message contents,
// authentication material and unrecognized request fields are never copied.
var userActivityActions = map[string]string{
	"purchase":               "Запросил оплату подписки или пакета",
	"gifts/purchase":         "Запросил оплату подарка",
	"purchase/cancel":        "Отменил оплату",
	"trial/activate":         "Запросил пробную подписку",
	"promocode/apply":        "Применил промокод",
	"promocode/redeem":       "Активировал промокод",
	"subscriptions/select":   "Выбрал подписку",
	"subscriptions/create":   "Создал подписку",
	"subscriptions/rename":   "Переименовал подписку",
	"subscriptions/delete":   "Удалил подписку",
	"payments/autopay":       "Изменил автоплатёж",
	"payments/remove-method": "Удалил способ оплаты",
	"devices/delete":         "Удалил устройство",
	"devices/control":        "Изменил доступ устройства",
	"wallet/withdraw":        "Запросил вывод средств",
	"support/create":         "Создал обращение в поддержку",
	"support/send":           "Отправил сообщение в поддержку",
	"support/send-media":     "Отправил вложение в поддержку",
	"support/close":          "Закрыл обращение в поддержку",
	"reviews/create":         "Отправил отзыв",
}

func (h *Handler) userActivityStore() userActivityRepository {
	if h.userActivityRepository != nil {
		return h.userActivityRepository
	}
	if h.customerRepository != nil {
		return h.customerRepository
	}
	return nil
}

func (h *Handler) runUserActivity(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer, next func(http.ResponseWriter, *http.Request, *session, *database.Customer)) {
	route := strings.TrimPrefix(r.URL.Path, "/api/mini-app/")
	title, record := userActivityActions[route]
	repo := h.userActivityStore()
	if !record || repo == nil || customer == nil || customer.ID <= 0 {
		next(w, r, sess, customer)
		return
	}
	entry := &database.UserActivity{CustomerID: customer.ID, Source: "miniapp", Action: route, Title: title, Status: "failed"}
	r = r.WithContext(context.WithValue(r.Context(), userActivityContextKey{}, entry))
	writer := &userActivityResponseWriter{activityResponseWriter: activityResponseWriter{ResponseWriter: w}}
	defer func() {
		if writer.status >= 200 && writer.status < 300 {
			entry.Status = "success"
		}
		if entry.Status == "failed" {
			entry.Details = append(entry.Details, database.UserActivityDetail{Label: "Результат", Value: fmt.Sprintf("Ошибка HTTP %d", writer.status)})

			if writer.code != "" {
				entry.Details = append(entry.Details, database.UserActivityDetail{Label: "Причина", Value: writer.code})
			}
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), time.Second)
		defer cancel()
		if err := repo.RecordUserActivity(ctx, *entry); err != nil {
			slog.Warn("mini app: record user activity", "error", err)
		}
	}()
	next(writer, r, sess, customer)
}

func (h *Handler) captureUserActivityPayload(r *http.Request, target any) {
	entry, _ := r.Context().Value(userActivityContextKey{}).(*database.UserActivity)
	if entry == nil {
		return
	}
	data := activityObject(target)
	// Identity comes from the authenticated customer, never from the body.
	for _, field := range []struct{ key, label string }{
		{"planId", "Тариф"}, {"invoiceType", "Способ"}, {"paymentMethod", "Способ"},
		{"subscriptionId", "Подписка"}, {"purchaseId", "Платёж"}, {"months", "Месяцев"},
		{"devicePackId", "Пакет устройств"}, {"trafficPackId", "Пакет трафика"},
	} {
		value, ok := data[field.key]
		if !ok || value == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			continue
		}
		if len([]rune(text)) > 100 {
			text = string([]rune(text)[:100])
		}
		entry.Details = append(entry.Details, database.UserActivityDetail{Label: field.label, Value: text})
	}
}

func (h *Handler) handleAdminUserActivity(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.canAdmin("users") {
		h.writeError(w, 403, "forbidden", "Access denied")
		return
	}
	var req struct {
		CustomerID int64 `json:"customerId"`
		BeforeID   int64 `json:"beforeId"`
	}
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil || req.CustomerID <= 0 || req.BeforeID < 0 {
		h.writeError(w, 400, "invalid_request", "Выберите пользователя")
		return
	}
	repo := h.userActivityStore()
	if repo == nil {
		h.writeError(w, 503, "activity_unavailable", "История временно недоступна")
		return
	}
	items, more, err := repo.ListUserActivity(r.Context(), req.CustomerID, req.BeforeID, 30)
	if err != nil {
		slog.Error("mini app: load user activity", "error", err)
		h.writeError(w, 500, "activity_failed", "Не удалось загрузить историю")
		return
	}
	settings := runtimeconfig.Settings{}
	if h.runtimeSettings != nil {
		settings = h.runtimeSettings.Snapshot()
	}
	for i := range items {
		enrichUserActivity(&items[i], settings)
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"customerId": req.CustomerID, "items": items, "hasMore": more}})
}

func enrichUserActivity(entry *database.UserActivity, settings runtimeconfig.Settings) {
	for i := range entry.Details {
		detail := &entry.Details[i]
		if detail.Label == "Тариф" {
			for _, plan := range settings.Plans {
				if plan.ID == detail.Value {
					if plan.Name != "" {
						detail.Value = plan.Name
					} else if plan.TitleRU != "" {
						detail.Value = plan.TitleRU
					}
					break
				}
			}
		}
		if detail.Label == "Способ" {
			if text := settings.PaymentMethodText[detail.Value].Name; text != "" {
				detail.Value = text
			} else if detail.Value == "free" {
				detail.Value = "Бесплатная активация"
			} else {
				detail.Value = adminFinanceProvider(database.AdminFinancePayment{InvoiceType: database.InvoiceType(detail.Value)})
			}
		}
		if detail.Label == "Причина" {
			if title := map[string]string{"payment_failed": "Не удалось создать платёж", "payment_unavailable": "Способ оплаты недоступен", "unsupported_plan": "Тариф недоступен", "unsupported_payment_method": "Способ оплаты недоступен", "insufficient_balance": "Недостаточно средств", "active_subscription_required": "Нужна активная подписка", "panel_unavailable": "Панель временно недоступна", "invalid_request": "Некорректный запрос"}[detail.Value]; title != "" {
				detail.Value = title + " (" + detail.Value + ")"
			}
		}
		if detail.Label == "Тип покупки" {
			switch detail.Value {
			case "subscription":
				detail.Value = "Подписка"
			case "gift":
				detail.Value = "Подарок"
			case "extra_devices":
				detail.Value = "Устройства"
			case "extra_traffic":
				detail.Value = "Трафик"
			}
		}
	}
}
