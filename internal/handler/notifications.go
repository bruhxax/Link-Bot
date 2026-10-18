package handler

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

const (
	CallbackNotifications        = "notifications" // Compatibility with the first 2.7.0 menu.
	CallbackBroadcastSubscribe   = "notifications:subscribe"
	CallbackBroadcastUnsubscribe = "notifications:unsubscribe"
)

func notificationsMenuButton(menu runtimeconfig.TelegramStartMenuSettings, subscribed bool) models.InlineKeyboardButton {
	settings, callback := menu.NotificationsOffButton, CallbackBroadcastSubscribe
	if subscribed {
		settings, callback = menu.NotificationsButton.TelegramButtonSettings, CallbackBroadcastUnsubscribe
	}
	button := telegramButton(settings, settings.Text, "")
	button.CallbackData = callback
	return button
}

func appendNotificationsMenuButton(keyboard [][]models.InlineKeyboardButton, menu runtimeconfig.TelegramStartMenuSettings, subscribed bool) [][]models.InlineKeyboardButton {
	if menu.NotificationsButton.Enabled {
		keyboard = append(keyboard, []models.InlineKeyboardButton{notificationsMenuButton(menu, subscribed)})
	}
	return keyboard
}

func (h Handler) appendCustomerNotificationsButton(ctx context.Context, keyboard [][]models.InlineKeyboardButton, customer *database.Customer) [][]models.InlineKeyboardButton {
	menu := h.telegramStartMenuSettings()
	if !menu.NotificationsButton.Enabled || customer == nil {
		return keyboard
	}
	subscribed, err := h.customerRepository.BroadcastSubscribed(ctx, customer.ID)
	if err != nil {
		slog.Warn("bot: load Telegram broadcast preference", "error", err)
		return keyboard
	}
	return appendNotificationsMenuButton(keyboard, menu, subscribed)
}

func updatedNotificationsMarkup(markup *models.InlineKeyboardMarkup, menu runtimeconfig.TelegramStartMenuSettings, subscribed bool) (*models.InlineKeyboardMarkup, bool) {
	if markup == nil {
		return nil, false
	}
	keyboard := make([][]models.InlineKeyboardButton, len(markup.InlineKeyboard))
	found := false
	for i, row := range markup.InlineKeyboard {
		keyboard[i] = append([]models.InlineKeyboardButton(nil), row...)
		for j, button := range row {
			if button.CallbackData == CallbackNotifications || button.CallbackData == CallbackBroadcastSubscribe || button.CallbackData == CallbackBroadcastUnsubscribe {
				keyboard[i][j] = notificationsMenuButton(menu, subscribed)
				found = true
			}
		}
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: keyboard}, found
}

func (h Handler) NotificationsCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery
	if callback == nil {
		return
	}
	message := callback.Message.Message
	if message == nil || message.Chat.ID != callback.From.ID || callback.From.ID <= 0 {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID})
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	fail := func(err error) {
		slog.Error("bot: Telegram notification preference failed", "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: "Не удалось сохранить настройки. Попробуйте ещё раз.", ShowAlert: true})
	}
	menu := h.telegramStartMenuSettings()
	if _, found := updatedNotificationsMarkup(message.ReplyMarkup, menu, true); !found {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID})
		return
	}
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.From.ID)
	if err != nil {
		fail(err)
		return
	}
	if customer == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: "Сначала отправьте /start", ShowAlert: true})
		return
	}
	// Explicit target states make duplicate callback deliveries idempotent.
	subscribed := callback.Data == CallbackBroadcastSubscribe
	if err := h.customerRepository.SetBroadcastSubscribed(ctx, customer.ID, subscribed); err != nil {
		fail(err)
		return
	}
	markup, _ := updatedNotificationsMarkup(message.ReplyMarkup, menu, subscribed)
	_, err = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{ChatID: message.Chat.ID, MessageID: message.ID, ReplyMarkup: markup})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "message is not modified") {
		slog.Warn("bot: update notifications button", "error", err)
	}
	text := "Уведомления выключены"
	if subscribed {
		text = "Уведомления включены"
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: text})
}
