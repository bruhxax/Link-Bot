package handler

import (
	"context"
	"fmt"
	"html"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/runtimeconfig"
)

func (h Handler) featureEnabled(name string) bool {
	return h.runtimeSettings == nil || h.runtimeSettings.FeatureEnabled(name)
}

func (h Handler) trialSettings() runtimeconfig.TrialSettings {
	if h.runtimeSettings == nil {
		return runtimeconfig.DefaultSettings().Trial
	}
	return h.runtimeSettings.TrialSettings()
}

func (h Handler) isAdminTelegramID(telegramID int64) bool {
	return telegramID != 0 && telegramID == config.GetAdminTelegramId()
}

func (h Handler) showMaintenance(ctx context.Context, b *bot.Bot, update *models.Update, chatID, telegramID int64, locale string) {
	if h.runtimeSettings == nil || h.isAdminTelegramID(telegramID) {
		return
	}
	settings := h.runtimeSettings.Maintenance()
	if !settings.Enabled {
		return
	}

	title, text, reason := maintenanceCopy(settings)
	message := fmt.Sprintf(
		"🛠 <b>%s</b>\n\n%s\n\n<b>Причина:</b> %s\n\nПожалуйста, попробуйте позже.",
		html.EscapeString(title),
		html.EscapeString(text),
		html.EscapeString(reason),
	)
	if update != nil && update.CallbackQuery != nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            title,
			ShowAlert:       true,
		})
	}
	h.deleteTrackedScreenMessage(ctx, b, chatID)
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      message,
		ParseMode: models.ParseModeHTML,
	})
	if err == nil && sent != nil {
		h.rememberScreenMessage(chatID, sent.ID)
	}
}

func maintenanceCopy(settings runtimeconfig.MaintenanceSettings) (string, string, string) {
	return settings.TitleRU, settings.TextRU, settings.ReasonRU
}
