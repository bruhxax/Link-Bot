package handler

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/utils"
)

func (h Handler) StartCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	ctxWithTime, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	langCode := update.Message.From.LanguageCode
	existingCustomer, err := h.customerRepository.FindByTelegramId(ctx, update.Message.Chat.ID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
		return
	}

	if existingCustomer == nil {
		existingCustomer, err = h.customerRepository.Create(ctxWithTime, &database.Customer{
			TelegramID: update.Message.Chat.ID,
			Language:   langCode,
		})
		if err != nil {
			slog.Error("error creating customer", "error", err)
			return
		}

		if strings.Contains(update.Message.Text, "ref_") {
			arg := strings.Split(update.Message.Text, " ")[1]
			if strings.HasPrefix(arg, "ref_") {
				code := strings.TrimPrefix(arg, "ref_")
				referrerId, err := strconv.ParseInt(code, 10, 64)
				if err != nil {
					slog.Error("error parsing referrer id", "error", err)
					return
				}
				_, err = h.customerRepository.FindByTelegramId(ctx, referrerId)
				if err == nil {
					_, err := h.referralRepository.Create(ctx, referrerId, existingCustomer.TelegramID)
					if err != nil {
						slog.Error("error creating referral", "error", err)
						return
					}
					slog.Info("referral created", "referrerId", utils.MaskHalfInt64(referrerId), "refereeId", utils.MaskHalfInt64(existingCustomer.TelegramID))
				}
			}
		}
	} else {
		updates := map[string]interface{}{
			"language": langCode,
		}

		err = h.customerRepository.UpdateFields(ctx, existingCustomer.ID, updates)
		if err != nil {
			slog.Error("Error updating customer", "error", err)
			return
		}
	}

	h.deleteTrackedScreenMessage(ctxWithTime, b, update.Message.Chat.ID)

	verified, err := h.maybeAutoVerifyRequiredChannelSubscription(ctxWithTime, b, existingCustomer)
	if err != nil {
		slog.Error("required channel verification failed on /start", "telegramId", utils.MaskHalfInt64(update.Message.Chat.ID), "error", err)
		h.showRequiredChannelSubscriptionPrompt(ctx, b, update.Message.Chat.ID, 0, langCode)
		return
	}
	if !verified {
		h.showRequiredChannelSubscriptionPrompt(ctx, b, update.Message.Chat.ID, 0, langCode)
		return
	}

	if h.hasMiniApp() {
		h.sendStartPhotoMessage(ctx, b, update.Message.Chat.ID, langCode, h.miniAppKeyboardForCustomer(ctxWithTime, existingCustomer, langCode))
		return
	}

	inlineKeyboard := h.buildStartKeyboard(existingCustomer, langCode)

	// Remove old reply keyboard (if any)
	m, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "🧹",
		ReplyMarkup: models.ReplyKeyboardRemove{
			RemoveKeyboard: true,
		},
	})
	if err != nil {
		slog.Error("Error sending removing reply keyboard", "error", err)
		return
	}

	_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    update.Message.Chat.ID,
		MessageID: m.ID,
	})
	if err != nil {
		slog.Error("Error deleting message", "error", err)
		return
	}

	// Send start screen (photo + caption + keyboard)
	h.sendStartPhotoMessage(ctx, b, update.Message.Chat.ID, langCode, inlineKeyboard)
}

func (h Handler) StartCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	ctxWithTime, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	callback := update.CallbackQuery
	langCode := callback.From.LanguageCode

	existingCustomer, err := h.customerRepository.FindByTelegramId(ctxWithTime, callback.From.ID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
		return
	}

	if h.hasMiniApp() {
		msg := callback.Message.Message
		if msg == nil {
			slog.Error("StartCallbackHandler: callback message is inaccessible (Message is nil)")
			return
		}

		h.editScreen(ctxWithTime, b,
			msg.Chat.ID,
			msg.ID,
			h.startImage(langCode),
			h.startText(langCode),
			h.miniAppKeyboardForCustomer(ctxWithTime, existingCustomer, langCode),
		)
		return
	}

	inlineKeyboard := h.buildStartKeyboard(existingCustomer, langCode)

	// ✅ Без удаления: обновляем это же сообщение (картинку + caption + кнопки)
	if callback.Message.Message != nil {
		msg := callback.Message.Message
		h.editScreen(ctxWithTime, b,
			msg.Chat.ID,
			msg.ID,
			h.startImage(langCode),
			h.startText(langCode),
			inlineKeyboard,
		)
		return
	}

	slog.Error("StartCallbackHandler: callback message is inaccessible (Message is nil)")
}

func (h Handler) sendStartPhotoMessage(
	ctx context.Context,
	b *bot.Bot,
	chatID int64,
	langCode string,
	inlineKeyboard [][]models.InlineKeyboardButton,
) {
	img := h.startImage(langCode)
	text := h.startText(langCode)
	if strings.TrimSpace(img) == "" {
		h.sendStartTextFallback(ctx, b, chatID, langCode, inlineKeyboard, 0)
		return
	}

	data, filename, err := readImageSource(img)
	if err != nil {
		slog.Error("start image download failed", "error", err)
		h.sendStartTextFallback(ctx, b, chatID, langCode, inlineKeyboard, 0)
		return
	}

	msg, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID: chatID,
		Photo: &models.InputFileUpload{
			Filename: filename,
			Data:     bytes.NewReader(data),
		},
		Caption:   text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
	})
	if err != nil {
		slog.Error("Error sending /start photo message", "error", err)
		h.sendStartTextFallback(ctx, b, chatID, langCode, inlineKeyboard, 0)
		return
	}
	h.rememberScreenMessage(chatID, msg.ID)
}

// fallback: если картинку не удалось отправить, просто покажем текст и кнопки.
// Если messageID > 0 — попробуем отредактировать caption этого сообщения (без удаления).
func (h Handler) sendStartTextFallback(
	ctx context.Context,
	b *bot.Bot,
	chatID int64,
	langCode string,
	inlineKeyboard [][]models.InlineKeyboardButton,
	messageID int,
) {
	text := h.startText(langCode)

	if messageID > 0 {
		_, err := b.EditMessageCaption(ctx, &bot.EditMessageCaptionParams{
			ChatID:    chatID,
			MessageID: messageID,
			Caption:   text,
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: inlineKeyboard,
			},
		})
		if err == nil {
			h.rememberScreenMessage(chatID, messageID)
			return
		}
	}

	msg, _ := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
		Text: text,
	})
	if msg != nil {
		h.rememberScreenMessage(chatID, msg.ID)
	}
}

func (h Handler) startText(langCode string) string {
	fallback := h.translation.GetText(langCode, "greeting")
	if h.runtimeSettings == nil {
		return fallback
	}
	return h.runtimeSettings.StartText(langCode, fallback)
}

func (h Handler) startImage(langCode string) string {
	fallback := h.translation.GetText(langCode, "image_start_url")
	if h.runtimeSettings == nil {
		return fallback
	}
	return h.runtimeSettings.StartImage(fallback)
}

func (h Handler) resolveConnectButton(lang string) []models.InlineKeyboardButton {
	var inlineKeyboard []models.InlineKeyboardButton

	if config.GetMiniAppURL() != "" {
		inlineKeyboard = []models.InlineKeyboardButton{
			{Text: h.translation.GetText(lang, "connect_button"), WebApp: &models.WebAppInfo{
				URL: config.GetMiniAppURL(),
			}},
		}
	} else {
		inlineKeyboard = []models.InlineKeyboardButton{
			{Text: h.translation.GetText(lang, "connect_button"), CallbackData: CallbackConnect},
		}
	}
	return inlineKeyboard
}

func (h Handler) buildStartKeyboard(existingCustomer *database.Customer, langCode string) [][]models.InlineKeyboardButton {
	var inlineKeyboard [][]models.InlineKeyboardButton

	// Conditions
	trial := h.trialSettings()
	showTrial := h.featureEnabled("trials") && trial.Enabled && existingCustomer.SubscriptionLink == nil && !existingCustomer.TrialUsed && trial.Days > 0
	showConnect := existingCustomer.SubscriptionLink != nil && existingCustomer.ExpireAt.After(time.Now())
	showReferral := h.featureEnabled("referrals") && config.GetReferralDays() > 0

	// Row 0: Trial alone on top if available
	if showTrial {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "trial_button"), CallbackData: CallbackTrial},
		})
	}

	// Row 1: Buy + Connect (if connect available), else only Buy
	paymentsEnabled := h.featureEnabled("yookassa") || h.featureEnabled("crypto") || h.featureEnabled("stars")
	buyBtn := models.InlineKeyboardButton{Text: h.translation.GetText(langCode, "buy_button"), CallbackData: CallbackBuy}
	if showConnect && paymentsEnabled {
		connectBtn := h.resolveConnectButton(langCode)[0]
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{buyBtn, connectBtn})
	} else if paymentsEnabled {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{buyBtn})
	} else if showConnect {
		inlineKeyboard = append(inlineKeyboard, h.resolveConnectButton(langCode))
	}

	// Row 2: Referral alone
	if showReferral {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "referral_button"), CallbackData: CallbackReferral},
		})
	}

	// Row 3: News left, Support right
	var row3 []models.InlineKeyboardButton
	channelURL := config.ChannelURL()
	supportURL := config.SupportURL()
	if h.runtimeSettings != nil {
		channelURL = h.runtimeSettings.Link("channel", channelURL)
		supportURL = h.runtimeSettings.Link("support", supportURL)
	}
	if channelURL != "" {
		row3 = append(row3, models.InlineKeyboardButton{Text: h.translation.GetText(langCode, "channel_button"), URL: channelURL})
	}
	if h.featureEnabled("support") && supportURL != "" {
		row3 = append(row3, models.InlineKeyboardButton{Text: h.translation.GetText(langCode, "support_button"), URL: supportURL})
	}
	if len(row3) > 0 {
		inlineKeyboard = append(inlineKeyboard, row3)
	}

	return inlineKeyboard
}
