package handler

import (
	"context"

	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/utils"
)

func (h Handler) CreateCustomerIfNotExistMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		var telegramId int64
		var langCode string
		if update.Message != nil {
			telegramId = update.Message.From.ID
			langCode = update.Message.From.LanguageCode
		} else if update.CallbackQuery != nil {
			telegramId = update.CallbackQuery.From.ID
			langCode = update.CallbackQuery.From.LanguageCode
		}
		existingCustomer, err := h.customerRepository.FindByTelegramId(ctx, telegramId)
		if err != nil {
			slog.Error("error finding customer by telegram id", "error", err)
			return
		}

		if existingCustomer == nil {
			existingCustomer, err = h.customerRepository.Create(ctx, &database.Customer{
				TelegramID: telegramId,
				Language:   langCode,
			})
			if err != nil {
				slog.Error("error creating customer", "error", err)
				return
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

		if update.CallbackQuery != nil && update.CallbackQuery.Data == CallbackVerifyChannel {
			next(ctx, b, update)
			return
		}

		verified, err := h.maybeAutoVerifyRequiredChannelSubscription(ctx, b, existingCustomer)
		if err != nil {
			slog.Error("required channel verification failed", "telegramId", utils.MaskHalfInt64(existingCustomer.TelegramID), "error", err)
			h.answerRequiredChannelSubscriptionCallback(ctx, b, update.CallbackQuery, langCode, "required_channel_subscription_check_failed")
			h.showRequiredChannelSubscriptionPromptForUpdate(ctx, b, update, langCode)
			return
		}
		if !verified {
			h.answerRequiredChannelSubscriptionCallback(ctx, b, update.CallbackQuery, langCode, "required_channel_subscription_required_notice")
			h.showRequiredChannelSubscriptionPromptForUpdate(ctx, b, update, langCode)
			return
		}

		next(ctx, b, update)
	}
}

func (h Handler) SuspiciousUserFilterMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		var userID int64
		var chatID int64
		var langCode string

		if update.Message != nil {
			userID = update.Message.From.ID
			chatID = update.Message.Chat.ID
			langCode = update.Message.From.LanguageCode
		} else if update.CallbackQuery != nil {
			userID = update.CallbackQuery.From.ID
			chatID = update.CallbackQuery.Message.Message.Chat.ID
			langCode = update.CallbackQuery.From.LanguageCode
		} else {
			next(ctx, b, update)
			return
		}

		if h.runtimeSettings != nil && h.runtimeSettings.Maintenance().Enabled && !h.isAdminTelegramID(userID) {
			h.showMaintenance(ctx, b, update, chatID, userID, langCode)
			return
		}

		if config.GetBlockedTelegramIds()[userID] {
			slog.Warn("blocked user by telegram id", "userId", utils.MaskHalfInt64(userID))
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    chatID,
				Text:      h.translation.GetText(langCode, "access_denied"),
				ParseMode: models.ParseModeHTML,
			})
			if err != nil {
				slog.Error("error sending blocked user message", "error", err)
			}
			return
		}

		if config.GetWhitelistedTelegramIds()[userID] {
			slog.Info("whitelisted user allowed", "userId", utils.MaskHalfInt64(userID))
			next(ctx, b, update)
			return
		}

		next(ctx, b, update)
	}
}
