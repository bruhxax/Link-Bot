package handler

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"link-bot/internal/runtimeconfig"
)

const (
	CallbackNotifications        = "notifications"
	CallbackBroadcastSubscribe   = "notifications:subscribe"
	CallbackBroadcastUnsubscribe = "notifications:unsubscribe"
)

func appendNotificationsMenuButton(keyboard [][]models.InlineKeyboardButton, menu runtimeconfig.TelegramStartMenuSettings) [][]models.InlineKeyboardButton {
	if menu.NotificationsButton.Enabled {
		button := telegramButton(menu.NotificationsButton.TelegramButtonSettings, "Уведомления", "")
		button.CallbackData = CallbackNotifications
		keyboard = append(keyboard, []models.InlineKeyboardButton{button})
	}
	return keyboard
}

// Shared with the content preview; rendering never changes the preference.
func NotificationsScreen(content runtimeconfig.ContentSettings, subscribed bool) (string, [][]models.InlineKeyboardButton) {
	settings := content.Notifications
	text, buttonSettings, callback := settings.DisabledText, settings.SubscribeButton, CallbackBroadcastSubscribe
	if subscribed {
		text, buttonSettings, callback = settings.EnabledText, settings.UnsubscribeButton, CallbackBroadcastUnsubscribe
	}
	button := telegramButton(buttonSettings, buttonSettings.Text, "")
	button.CallbackData = callback
	back := telegramButton(content.Commerce.BackButton, "Назад", botBackEmojiID)
	back.CallbackData = CallbackStart
	return text, [][]models.InlineKeyboardButton{{button}, {back}}
}

func (h Handler) NotificationsCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery
	if callback == nil {
		return
	}
	// A forwarded/group menu cannot change someone else's settings or messages.
	message := callback.Message.Message
	if message == nil || message.Chat.ID != callback.From.ID || callback.From.ID <= 0 {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID})
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	fail := func(err error) {
		slog.Error("bot: notification preferences failed", "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: "Не удалось сохранить настройки. Попробуйте ещё раз.", ShowAlert: true})
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
	if callback.Data == CallbackBroadcastSubscribe || callback.Data == CallbackBroadcastUnsubscribe {
		if err := h.customerRepository.SetBroadcastSubscribed(ctx, customer.ID, callback.Data == CallbackBroadcastSubscribe); err != nil {
			fail(err)
			return
		}
	}
	subscribed, err := h.customerRepository.BroadcastSubscribed(ctx, customer.ID)
	if err != nil {
		fail(err)
		return
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID})
	settings := runtimeconfig.DefaultSettings()
	if h.runtimeSettings != nil {
		settings = h.runtimeSettings.Snapshot()
	}
	content := runtimeconfig.LocalizeTelegramDefaults(settings.Content, settings.Localization.Language)
	text, keyboard := NotificationsScreen(content, subscribed)
	h.editScreen(ctx, b, message.Chat.ID, message.ID, "", text, keyboard)
}
