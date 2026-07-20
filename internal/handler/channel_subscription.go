package handler

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
	"link-bot/utils"
)

const (
	channelSubscriptionStatusSubscribed   = 1
	channelSubscriptionStatusUnsubscribed = -1
)

func (h Handler) VerifyChannelSubscriptionCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery
	if callback == nil {
		return
	}

	langCode := callback.From.LanguageCode
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.From.ID)
	if err != nil {
		slog.Error("required channel: failed to load customer", "telegramId", utils.MaskHalfInt64(callback.From.ID), "error", err)
		h.answerRequiredChannelSubscriptionCallback(ctx, b, callback, langCode, "required_channel_subscription_check_failed")
		h.showRequiredChannelSubscriptionPromptForUpdate(ctx, b, update, langCode)
		return
	}
	if customer == nil {
		slog.Warn("required channel: customer missing for verify callback", "telegramId", utils.MaskHalfInt64(callback.From.ID))
		h.answerRequiredChannelSubscriptionCallback(ctx, b, callback, langCode, "required_channel_subscription_check_failed")
		h.showRequiredChannelSubscriptionPromptForUpdate(ctx, b, update, langCode)
		return
	}

	verified, err := h.verifyRequiredChannelSubscription(ctx, b, customer, true)
	if err != nil {
		slog.Error("required channel: verify callback failed", "telegramId", utils.MaskHalfInt64(callback.From.ID), "error", err)
		h.answerRequiredChannelSubscriptionCallback(ctx, b, callback, langCode, "required_channel_subscription_check_failed")
		h.showRequiredChannelSubscriptionPromptForUpdate(ctx, b, update, langCode)
		return
	}
	if !verified {
		h.answerRequiredChannelSubscriptionCallback(ctx, b, callback, langCode, "required_channel_subscription_not_subscribed")
		h.showRequiredChannelSubscriptionPromptForUpdate(ctx, b, update, langCode)
		return
	}

	h.answerRequiredChannelSubscriptionCallback(ctx, b, callback, langCode, "required_channel_subscription_verified")

	if callback.Message.Message != nil {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    callback.Message.Message.Chat.ID,
			MessageID: callback.Message.Message.ID,
		})
		h.clearScreenMessage(callback.Message.Message.Chat.ID)
	}

	if h.hasMiniApp() {
		h.sendStartPhotoMessage(ctx, b, callback.From.ID, langCode, h.miniAppKeyboardForCustomer(ctx, customer, langCode))
		return
	}

	inlineKeyboard := h.buildStartKeyboard(customer, langCode)
	h.sendStartPhotoMessage(ctx, b, callback.From.ID, langCode, inlineKeyboard)
}

func (h Handler) maybeAutoVerifyRequiredChannelSubscription(ctx context.Context, b *bot.Bot, customer *database.Customer) (bool, error) {
	return h.verifyRequiredChannelSubscription(ctx, b, customer, false)
}

func (h Handler) verifyRequiredChannelSubscription(ctx context.Context, b *bot.Bot, customer *database.Customer, force bool) (bool, error) {
	if !config.IsRequiredChannelSubscriptionEnabled() || customer == nil {
		return true, nil
	}
	if h.shouldBypassRequiredChannelSubscription(customer.TelegramID) {
		return true, nil
	}

	if !force && h.channelSubCache != nil {
		if status, ok := h.channelSubCache.Get(customer.TelegramID); ok {
			return status == channelSubscriptionStatusSubscribed, nil
		}
	}

	subscribed, err := h.isUserSubscribedToRequiredChannel(ctx, b, customer.TelegramID)
	if err != nil {
		return false, err
	}

	if h.channelSubCache != nil {
		if subscribed {
			h.channelSubCache.Set(customer.TelegramID, channelSubscriptionStatusSubscribed)
		} else {
			h.channelSubCache.Set(customer.TelegramID, channelSubscriptionStatusUnsubscribed)
		}
	}

	if !subscribed {
		if customer.ChannelSubscriptionVerifiedAt != nil {
			if err := h.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
				"channel_subscription_verified_at": nil,
			}); err != nil {
				return false, err
			}
			customer.ChannelSubscriptionVerifiedAt = nil
		}
		return false, nil
	}

	if customer.ChannelSubscriptionVerifiedAt == nil {
		verifiedAt := time.Now().UTC()
		if err := h.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
			"channel_subscription_verified_at": verifiedAt,
		}); err != nil {
			return false, err
		}
		customer.ChannelSubscriptionVerifiedAt = &verifiedAt
	}
	return true, nil
}

func (h Handler) shouldBypassRequiredChannelSubscription(telegramID int64) bool {
	return telegramID == config.GetAdminTelegramId() || config.GetWhitelistedTelegramIds()[telegramID]
}

func (h Handler) isUserSubscribedToRequiredChannel(ctx context.Context, b *bot.Bot, telegramID int64) (bool, error) {
	if b == nil {
		return false, fmt.Errorf("telegram bot is nil")
	}

	chatID, ok := config.RequiredChannelSubscriptionChatID()
	if !ok {
		return false, fmt.Errorf("required channel subscription chat id is not configured")
	}

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	member, err := b.GetChatMember(checkCtx, &bot.GetChatMemberParams{
		ChatID: chatID,
		UserID: telegramID,
	})
	if err != nil {
		return false, err
	}

	switch member.Type {
	case models.ChatMemberTypeOwner, models.ChatMemberTypeAdministrator, models.ChatMemberTypeMember:
		return true, nil
	case models.ChatMemberTypeRestricted:
		return member.Restricted != nil && member.Restricted.IsMember, nil
	default:
		return false, nil
	}
}

func (h Handler) showRequiredChannelSubscriptionPromptForUpdate(ctx context.Context, b *bot.Bot, update *models.Update, langCode string) {
	switch {
	case update.CallbackQuery != nil && update.CallbackQuery.Message.Message != nil:
		msg := update.CallbackQuery.Message.Message
		h.showRequiredChannelSubscriptionPrompt(ctx, b, msg.Chat.ID, msg.ID, langCode)
	case update.CallbackQuery != nil:
		h.showRequiredChannelSubscriptionPrompt(ctx, b, update.CallbackQuery.From.ID, 0, langCode)
	case update.Message != nil:
		h.showRequiredChannelSubscriptionPrompt(ctx, b, update.Message.Chat.ID, 0, langCode)
	}
}

func (h Handler) showRequiredChannelSubscriptionPrompt(ctx context.Context, b *bot.Bot, chatID int64, replaceMessageID int, langCode string) {
	if replaceMessageID > 0 {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    chatID,
			MessageID: replaceMessageID,
		})
	}

	keyboard := models.InlineKeyboardMarkup{
		InlineKeyboard: h.buildRequiredChannelSubscriptionKeyboard(langCode),
	}

	banner := strings.TrimSpace(h.telegramVerificationSettings().Banner)
	if banner != "" {
		data, filename, err := readImageSource(banner)
		if err == nil {
			msg, sendErr := b.SendPhoto(ctx, &bot.SendPhotoParams{
				ChatID: chatID,
				Photo: &models.InputFileUpload{
					Filename: filename,
					Data:     bytes.NewReader(data),
				},
				Caption:     h.requiredChannelSubscriptionText(langCode),
				ParseMode:   models.ParseModeHTML,
				ReplyMarkup: keyboard,
			})
			if sendErr == nil {
				h.rememberScreenMessage(chatID, msg.ID)
				return
			}
			err = sendErr
		}
		slog.Error("required channel: failed to send verification banner", "chatId", utils.MaskHalfInt64(chatID), "error", err)
	}

	fallbackMsg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        h.requiredChannelSubscriptionText(langCode),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		slog.Error("required channel: failed to send verification text fallback", "chatId", utils.MaskHalfInt64(chatID), "error", err)
		return
	}
	h.rememberScreenMessage(chatID, fallbackMsg.ID)
}

func (h Handler) buildRequiredChannelSubscriptionKeyboard(langCode string) [][]models.InlineKeyboardButton {
	settings := h.telegramVerificationSettings()
	channelButton := telegramButton(settings.ChannelButton, h.requiredChannelSubscriptionTitle(), "")
	channelButton.URL = config.RequiredChannelSubscriptionURL()
	confirmButton := telegramButton(settings.ConfirmButton, h.translation.GetText(langCode, "required_channel_subscription_confirm_button"), "")
	confirmButton.CallbackData = CallbackVerifyChannel
	return [][]models.InlineKeyboardButton{
		{channelButton},
		{confirmButton},
	}
}

func (h Handler) requiredChannelSubscriptionTitle() string {
	if title := strings.TrimSpace(config.RequiredChannelSubscriptionTitle()); title != "" {
		return title
	}

	raw := strings.TrimSpace(config.RequiredChannelSubscriptionURL())
	raw = strings.TrimPrefix(raw, "https://t.me/")
	raw = strings.TrimPrefix(raw, "http://t.me/")
	raw = strings.TrimPrefix(raw, "t.me/")
	raw = strings.TrimPrefix(raw, "@")
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return "Channel"
	}

	return raw
}

func (h Handler) requiredChannelSubscriptionText(langCode string) string {
	text := strings.TrimSpace(h.telegramVerificationSettings().Text)
	if text == "" {
		text = h.translation.GetText(langCode, "required_channel_subscription_text")
	}
	if strings.Contains(text, "%s") {
		return strings.ReplaceAll(text, "%s", h.requiredChannelSubscriptionTitle())
	}
	return text
}

func (h Handler) telegramVerificationSettings() runtimeconfig.TelegramVerificationSettings {
	if h.runtimeSettings == nil {
		return runtimeconfig.DefaultSettings().Content.Verification
	}
	return h.runtimeSettings.Snapshot().Content.Verification
}

func (h Handler) answerRequiredChannelSubscriptionCallback(ctx context.Context, b *bot.Bot, callback *models.CallbackQuery, langCode string, key string) {
	if callback == nil {
		return
	}

	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
		Text:            h.requiredChannelSubscriptionNotice(langCode, key),
		ShowAlert:       false,
	})
	if err != nil {
		slog.Debug("required channel: answer callback failed", "telegramId", utils.MaskHalfInt64(callback.From.ID), "error", err)
	}
}

func (h Handler) requiredChannelSubscriptionNotice(langCode, key string) string {
	settings := h.telegramVerificationSettings()
	switch key {
	case "required_channel_subscription_check_failed":
		return settings.CheckFailedText
	case "required_channel_subscription_not_subscribed":
		return settings.NotSubscribedText
	case "required_channel_subscription_verified":
		return settings.VerifiedText
	default:
		return h.translation.GetText(langCode, key)
	}
}
