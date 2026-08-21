package handler

import (
	"strings"

	"github.com/go-telegram/bot/models"

	"link-bot/internal/runtimeconfig"
)

const (
	botPayEmojiID  = "5206401524200145033"
	botBackEmojiID = "5877629862306385808"
)

func (h Handler) premiumBackButton(callbackData string) models.InlineKeyboardButton {
	settings := runtimeconfig.TelegramButtonSettings{Text: "Назад", IconCustomEmojiID: botBackEmojiID}
	if h.runtimeSettings != nil {
		runtime := h.runtimeSettings.Snapshot()
		settings = runtimeconfig.LocalizeTelegramDefaults(runtime.Content, runtime.Localization.Language).Commerce.BackButton
	}
	button := telegramButton(settings, settings.Text, botBackEmojiID)
	button.CallbackData = callbackData
	return button
}

func (h Handler) premiumPayURLButton(url string) models.InlineKeyboardButton {
	settings := runtimeconfig.TelegramButtonSettings{Text: "Оплатить", IconCustomEmojiID: botPayEmojiID}
	if h.runtimeSettings != nil {
		runtime := h.runtimeSettings.Snapshot()
		settings = runtimeconfig.LocalizeTelegramDefaults(runtime.Content, runtime.Localization.Language).Commerce.PayButton
	}
	button := telegramButton(settings, settings.Text, botPayEmojiID)
	button.URL = url
	return button
}

func telegramButton(settings runtimeconfig.TelegramButtonSettings, fallbackText, fallbackEmojiID string) models.InlineKeyboardButton {
	text := strings.TrimSpace(settings.Text)
	if text == "" {
		text = fallbackText
	}
	emojiID := strings.TrimSpace(settings.IconCustomEmojiID)
	if emojiID == "" {
		emojiID = fallbackEmojiID
	}
	return models.InlineKeyboardButton{
		Text:              text,
		IconCustomEmojiID: emojiID,
		Style:             strings.TrimSpace(settings.Style),
	}
}
