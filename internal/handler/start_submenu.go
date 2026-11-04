package handler

import (
	"context"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

const submenuGB = float64(1024 * 1024 * 1024)

func submenuAmount(value float64) string {
	if value <= 0 {
		return "0"
	}
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', 1, 64)
}

func submenuTraffic(bytes int64) string {
	if bytes < 0 {
		return "—"
	}
	return submenuAmount(float64(bytes) / submenuGB)
}

// Values are escaped individually, preserving the administrator's Telegram HTML.
func renderStartSubmenuText(template string, values map[string]string) string {
	result := template
	for _, key := range []string{"username", "telegram_id", "subscription_name", "plan_name", "expires_at", "days_left", "price", "traffic_used_gb", "traffic_limit_gb", "traffic", "devices_used", "devices_limit", "devices", "status", "subscription_link"} {
		value := values[key]
		if value == "" {
			value = "—"
		}
		result = strings.ReplaceAll(result, "{"+key+"}", html.EscapeString(value))
	}
	return result
}

func (h Handler) startSubmenuValues(ctx context.Context, customer *database.Customer, locale string) map[string]string {
	values := map[string]string{"traffic_used_gb": "—", "traffic_limit_gb": "—", "devices_used": "—", "devices_limit": "—"}
	if customer == nil {
		return values
	}
	values["telegram_id"] = strconv.FormatInt(customer.TelegramID, 10)
	values["username"] = values["telegram_id"]
	if customer.TelegramUsername != nil && *customer.TelegramUsername != "" {
		values["username"] = "@" + *customer.TelegramUsername
	}
	if h.startSubscriptions == nil {
		return values
	}
	subscription, err := h.startSubscriptions.ActiveForCustomer(ctx, customer)
	if err != nil || subscription == nil {
		if err != nil {
			slog.Warn("start submenu: load subscription failed", "error", err)
		}
		return values
	}
	values["subscription_name"] = subscription.DisplayName
	if subscription.SubscriptionLink != nil {
		values["subscription_link"] = *subscription.SubscriptionLink
	}
	expires := subscription.ExpireAt
	if expires == nil {
		expires = customer.ExpireAt
	}
	if expires != nil {
		values["expires_at"] = expires.In(time.FixedZone("MSK", 3*3600)).Format("02.01.2006 15:04")
		remaining := int(time.Until(*expires).Hours() / 24)
		if remaining < 0 {
			remaining = 0
		}
		values["days_left"] = strconv.Itoa(remaining)
		values["status"] = "Истекла"
		if expires.After(time.Now()) {
			values["status"] = "Активна"
		}
	}
	if customer.IsBlocked {
		values["status"] = "Заблокирована"
	}
	if h.purchaseRepository != nil {
		purchase, purchaseErr := h.purchaseRepository.FindLatestSuccessfulPurchaseBySubscription(ctx, customer.ID, subscription.ID)
		if purchaseErr != nil {
			slog.Warn("start submenu: load plan failed", "error", purchaseErr)
		} else if purchase != nil {
			currency := string(purchase.Currency)
			if currency == "RUB" {
				currency = "₽"
			}
			values["price"] = submenuAmount(purchase.Amount) + " " + currency
			if purchase.PlanID != nil && h.runtimeSettings != nil {
				values["plan_name"] = h.runtimeSettings.PlanTitle(*purchase.PlanID, locale)
			}
		}
	}
	if h.startPanel != nil && (subscription.PanelUserID != nil || subscription.PanelUserUUID != nil) {
		var panelID int64
		if subscription.PanelUserID != nil {
			panelID = *subscription.PanelUserID
		}
		panelUUID := uuid.Nil
		if subscription.PanelUserUUID != nil {
			panelUUID = *subscription.PanelUserUUID
		}
		state, stateErr := h.startPanel.GetUserStateByIdentity(ctx, panelID, panelUUID)
		if stateErr != nil {
			slog.Warn("start submenu: panel state unavailable", "error", stateErr)
		} else if state != nil && state.Exists {
			values["traffic_used_gb"] = submenuTraffic(state.UsedTrafficBytes)
			values["traffic_limit_gb"] = "∞"
			if state.TrafficLimitBytes > 0 {
				values["traffic_limit_gb"] = submenuTraffic(state.TrafficLimitBytes)
			}
			if state.DevicesLoaded {
				values["devices_used"] = strconv.Itoa(state.UsedDevices)
			}
			values["devices_limit"] = "∞"
			if state.DeviceLimit > 0 {
				values["devices_limit"] = strconv.Itoa(state.DeviceLimit)
			}
			if state.ExpireAt != nil {
				expires = state.ExpireAt
				values["expires_at"] = expires.In(time.FixedZone("MSK", 3*3600)).Format("02.01.2006 15:04")
				remaining := int(time.Until(*expires).Hours() / 24)
				if remaining < 0 {
					remaining = 0
				}
				values["days_left"] = strconv.Itoa(remaining)
				if !customer.IsBlocked {
					values["status"] = "Истекла"
					if expires.After(time.Now()) {
						values["status"] = "Активна"
					}
				}
			}
			if state.SubscriptionLink != nil {
				values["subscription_link"] = *state.SubscriptionLink
			}
		}
	}
	values["traffic"] = values["traffic_used_gb"] + "/" + values["traffic_limit_gb"] + " ГБ"
	values["devices"] = values["devices_used"] + "/" + values["devices_limit"]
	return values
}

func (h Handler) StartSubmenuCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery
	if callback == nil || callback.Message.Message == nil {
		return
	}
	msg := callback.Message.Message
	if msg.Chat.Type != models.ChatTypePrivate || msg.Chat.ID != callback.From.ID {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: "Откройте меню в личном чате с ботом", ShowAlert: true})
		return
	}
	menu := h.telegramStartMenuSettings()
	id := strings.TrimPrefix(callback.Data, "sm:")
	var selected *runtimeconfig.TelegramSubMenuSettings
	for index := range menu.Buttons {
		button := &menu.Buttons[index]
		if button.ID == id && button.Kind == "submenu" {
			selected = button.Submenu
			break
		}
	}
	if selected == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: "Меню больше недоступно", ShowAlert: true})
		return
	}
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.From.ID)
	if err != nil || customer == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: "Пользователь не найден", ShowAlert: true})
		return
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID})
	dataCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	values := h.startSubmenuValues(dataCtx, customer, callback.From.LanguageCode)
	cancel()
	text := renderStartSubmenuText(selected.Text, values)
	keyboard := h.configuredStartButtons(ctx, customer, selected.Buttons, menu, false)
	keyboard = append(keyboard, []models.InlineKeyboardButton{h.premiumBackButton(CallbackStart)})
	image := selected.Banner
	if utf8.RuneCountInString(text) > 1000 {
		image = ""
	}
	h.editScreen(ctx, b, msg.Chat.ID, msg.ID, image, text, keyboard)
}
