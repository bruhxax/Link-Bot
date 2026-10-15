package handler

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
	"link-bot/utils"
)

func (h Handler) TrialCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	trial := h.trialSettings()
	if !h.featureEnabled("trials") || !trial.Enabled || trial.Days == 0 {
		return
	}

	c, err := h.customerRepository.FindByTelegramId(ctx, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if c == nil {
		slog.Error("customer not exist", "telegramId", utils.MaskHalfInt64(update.CallbackQuery.From.ID), "error", err)
		return
	}
	trialAllowed, err := h.canOfferTrial(ctx, c)
	if err != nil {
		slog.Error("Error checking trial eligibility", "error", err)
		return
	}
	if !trialAllowed {
		return
	}

	msg := update.CallbackQuery.Message.Message
	if msg == nil {
		slog.Error("TrialCallbackHandler: callback message is nil")
		return
	}

	content := h.trialMessageContent()
	keyboard := TrialScreenKeyboard(content, false, config.GetMiniAppURL())

	// ✅ Обновляем одно меню-сообщение (картинка + caption + кнопки), без удаления сообщений
	h.editScreen(ctx, b,
		msg.Chat.ID,
		msg.ID,
		content.Trial.Banner,
		content.Trial.Text,
		keyboard,
	)
}

func (h Handler) ActivateTrialCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	trial := h.trialSettings()
	if !h.featureEnabled("trials") || !trial.Enabled || trial.Days == 0 {
		return
	}

	c, err := h.customerRepository.FindByTelegramId(ctx, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if c == nil {
		slog.Error("customer not exist", "telegramId", utils.MaskHalfInt64(update.CallbackQuery.From.ID), "error", err)
		return
	}
	trialAllowed, err := h.canOfferTrial(ctx, c)
	if err != nil {
		slog.Error("Error checking trial eligibility", "error", err)
		return
	}
	if !trialAllowed {
		return
	}

	msg := update.CallbackQuery.Message.Message
	if msg == nil {
		slog.Error("ActivateTrialCallbackHandler: callback message is nil")
		return
	}

	ctxWithProfile := contextWithTelegramProfile(ctx, update.CallbackQuery.From)
	_, err = h.paymentService.ActivateTrial(ctxWithProfile, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("Error activating trial", "error", err)
		return
	}

	content := h.trialMessageContent()
	connectKb := TrialScreenKeyboard(content, true, config.GetMiniAppURL())

	// ✅ Обновляем одно меню-сообщение (картинка + caption + кнопки), без удаления сообщений
	h.editScreen(ctx, b,
		msg.Chat.ID,
		msg.ID,
		content.Trial.ActivatedBanner,
		content.Trial.ActivatedText,
		connectKb,
	)
}

func (h Handler) trialMessageContent() runtimeconfig.ContentSettings {
	settings := runtimeconfig.DefaultSettings()
	if h.runtimeSettings != nil {
		settings = h.runtimeSettings.Snapshot()
	}
	return runtimeconfig.LocalizeTelegramDefaults(settings.Content, settings.Localization.Language)
}

// TrialScreenKeyboard is shared with the content preview so configured button
// labels, emoji and colors match the real trial screens.
func TrialScreenKeyboard(content runtimeconfig.ContentSettings, activated bool, miniAppURL string) [][]models.InlineKeyboardButton {
	button := telegramButton(content.Trial.ActivateButton, "Активировать бесплатно", "")
	button.CallbackData = CallbackActivateTrial
	if activated {
		button = telegramButton(content.Trial.ConnectButton, "Подписка", "")
		if miniAppURL != "" {
			button.WebApp = &models.WebAppInfo{URL: miniAppURL}
		} else {
			button.CallbackData = CallbackConnect
		}
	}
	back := telegramButton(content.Commerce.BackButton, "Назад", botBackEmojiID)
	back.CallbackData = CallbackStart
	return [][]models.InlineKeyboardButton{{button}, {back}}
}

func (h Handler) canOfferTrial(ctx context.Context, customer *database.Customer) (bool, error) {
	trial := h.trialSettings()
	if !h.featureEnabled("trials") || !trial.Enabled || trial.Days == 0 || customer == nil || customer.TrialUsed {
		return false, nil
	}

	if customer.ExpireAt != nil && customer.ExpireAt.After(time.Now()) {
		return false, nil
	}

	purchase, err := h.purchaseRepository.FindSuccessfulPurchaseByCustomer(ctx, customer.ID)
	if err != nil {
		return false, err
	}

	return purchase == nil, nil
}
