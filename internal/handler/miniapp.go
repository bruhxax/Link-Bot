package handler

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

const (
	miniAppDashboardEmojiID = "5278413853577734640"
	miniAppTariffsEmojiID   = "5206626000665868017"
	miniAppSupportEmojiID   = "5206222720416643915"
	miniAppTrialEmojiID     = "5276422526350681413"
)

func (h Handler) hasMiniApp() bool {
	return h.featureEnabled("mini_app") && config.GetMiniAppURL() != ""
}

func (h Handler) miniAppURL(page string) string {
	base := config.GetMiniAppURL()
	if base == "" || page == "" {
		return base
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return base
	}

	query := parsed.Query()
	query.Set("page", page)
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func (h Handler) miniAppKeyboard(lang string) [][]models.InlineKeyboardButton {
	if !h.hasMiniApp() {
		return nil
	}

	settings := h.telegramStartMenuSettings()
	dashboardButton := telegramButton(settings.DashboardButton, h.translation.GetText(lang, "web_app_button_text"), miniAppDashboardEmojiID)
	dashboardButton.WebApp = &models.WebAppInfo{URL: h.miniAppURL("dashboard")}
	plansButton := telegramButton(settings.PlansButton, h.miniAppTariffsButtonText(lang), miniAppTariffsEmojiID)
	plansButton.CallbackData = CallbackBuy
	supportButton := telegramButton(settings.SupportButton, h.translation.GetText(lang, "support_chat_button_text"), miniAppSupportEmojiID)
	supportButton.WebApp = &models.WebAppInfo{URL: h.miniAppURL("support")}

	return [][]models.InlineKeyboardButton{{dashboardButton}, {plansButton}, {supportButton}}
}

func (h Handler) miniAppKeyboardForCustomer(ctx context.Context, customer *database.Customer, lang string) [][]models.InlineKeyboardButton {
	keyboard := h.miniAppKeyboard(lang)
	if len(keyboard) == 0 || customer == nil {
		return keyboard
	}

	trialAllowed, err := h.canOfferTrial(ctx, customer)
	if err != nil {
		slog.Error("Error checking trial eligibility for start keyboard", "error", err)
		return keyboard
	}
	if !trialAllowed {
		return keyboard
	}

	trialButton := telegramButton(h.telegramStartMenuSettings().TrialButton, "Попробовать бесплатно", miniAppTrialEmojiID)
	trialButton.CallbackData = CallbackActivateTrial
	trialRow := []models.InlineKeyboardButton{trialButton}

	return append([][]models.InlineKeyboardButton{trialRow}, keyboard...)
}

func (h Handler) telegramStartMenuSettings() runtimeconfig.TelegramStartMenuSettings {
	if h.runtimeSettings == nil {
		return runtimeconfig.DefaultSettings().Content.StartMenu
	}
	return h.runtimeSettings.Snapshot().Content.StartMenu
}

func (h Handler) miniAppTariffsButtonText(lang string) string {
	text := h.translation.GetText(lang, "buy_button")
	text = strings.ReplaceAll(text, "🚀", "")
	return strings.TrimSpace(text)
}

func (h Handler) sendMiniAppEntry(ctx context.Context, b *bot.Bot, chatID int64, langCode string, text string) error {
	msg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: h.miniAppKeyboard(langCode),
		},
	})
	if err == nil && msg != nil {
		h.rememberScreenMessage(chatID, msg.ID)
	}

	return err
}
