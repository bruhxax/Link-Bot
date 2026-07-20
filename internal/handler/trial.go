package handler

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/database"
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

	langCode := update.CallbackQuery.From.LanguageCode

	keyboard := [][]models.InlineKeyboardButton{
		{{Text: h.translation.GetText(langCode, "activate_trial_button"), CallbackData: CallbackActivateTrial}},
		{h.premiumBackButton(CallbackStart)},
	}

	// ✅ Обновляем одно меню-сообщение (картинка + caption + кнопки), без удаления сообщений
	h.editScreen(ctx, b,
		msg.Chat.ID,
		msg.ID,
		h.translation.GetText(langCode, "image_trial_url"),
		h.translation.GetText(langCode, "trial_text"),
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

	langCode := update.CallbackQuery.From.LanguageCode
	connectKb := h.createConnectKeyboard(langCode)

	// ✅ Обновляем одно меню-сообщение (картинка + caption + кнопки), без удаления сообщений
	h.editScreen(ctx, b,
		msg.Chat.ID,
		msg.ID,
		h.translation.GetText(langCode, "image_connect_url"),
		h.translation.GetText(langCode, "trial_activated"),
		connectKb,
	)
}

func (h Handler) createConnectKeyboard(lang string) [][]models.InlineKeyboardButton {
	var inlineCustomerKeyboard [][]models.InlineKeyboardButton

	inlineCustomerKeyboard = append(inlineCustomerKeyboard, h.resolveConnectButton(lang))

	inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{h.premiumBackButton(CallbackStart)})

	return inlineCustomerKeyboard
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
