package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/translation"
	"link-bot/utils"
)

func (h Handler) ConnectCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	customer, err := h.customerRepository.FindByTelegramId(ctx, update.Message.Chat.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "telegramId", utils.MaskHalfInt64(update.Message.Chat.ID), "error", err)
		return
	}

	langCode := update.Message.From.LanguageCode

	if h.hasMiniApp() {
		err = h.sendMiniAppEntry(ctx, b, update.Message.Chat.ID, langCode, buildConnectText(customer, langCode))
		if err != nil {
			slog.Error("Error sending mini app connect entry", "error", err)
		}
		return
	}

	var markup [][]models.InlineKeyboardButton
	if config.GetMiniAppURL() != "" {
		markup = append(markup, []models.InlineKeyboardButton{{
			Text: h.translation.GetText(langCode, "connect_button"),
			WebApp: &models.WebAppInfo{
				URL: config.GetMiniAppURL(),
			},
		}})
	} else if config.IsWepAppLinkEnabled() {
		if customer.SubscriptionLink != nil && customer.ExpireAt.After(time.Now()) {
			markup = append(markup, []models.InlineKeyboardButton{{
				Text: h.translation.GetText(langCode, "connect_press"),
				WebApp: &models.WebAppInfo{
					URL: *customer.SubscriptionLink,
				},
			}})
		}
	}
	markup = append(markup, []models.InlineKeyboardButton{h.premiumBackButton(CallbackStart)})

	isDisabled := true
	msg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      buildConnectText(customer, langCode),
		ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &isDisabled,
		},
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: markup,
		},
	})
	if err != nil {
		slog.Error("Error sending connect message", "error", err)
		return
	}
	h.rememberScreenMessage(update.Message.Chat.ID, msg.ID)
}

func (h Handler) ConnectCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.CallbackQuery.Message.Message
	if msg == nil {
		slog.Error("ConnectCallbackHandler: callback message is nil")
		return
	}

	customer, err := h.customerRepository.FindByTelegramId(ctx, msg.Chat.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "telegramId", utils.MaskHalfInt64(msg.Chat.ID), "error", err)
		return
	}

	langCode := update.CallbackQuery.From.LanguageCode

	if h.hasMiniApp() {
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Text:      buildConnectText(customer, langCode),
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: h.miniAppKeyboard(langCode),
			},
		})
		if err != nil {
			slog.Error("Error editing mini app connect entry", "error", err)
		}
		return
	}

	var markup [][]models.InlineKeyboardButton
	if config.GetMiniAppURL() != "" {
		markup = append(markup, []models.InlineKeyboardButton{{
			Text:   h.translation.GetText(langCode, "connect_button"),
			WebApp: &models.WebAppInfo{URL: config.GetMiniAppURL()},
		}})
	} else if config.IsWepAppLinkEnabled() {
		if customer.SubscriptionLink != nil && customer.ExpireAt.After(time.Now()) {
			markup = append(markup, []models.InlineKeyboardButton{{
				Text:   h.translation.GetText(langCode, "connect_press"),
				WebApp: &models.WebAppInfo{URL: *customer.SubscriptionLink},
			}})
		}
	}
	markup = append(markup, []models.InlineKeyboardButton{h.premiumBackButton(CallbackStart)})

	// ✅ Обновляем одно меню-сообщение (картинка + caption + кнопки), без удаления сообщений
	h.editScreen(ctx, b,
		msg.Chat.ID,
		msg.ID,
		h.translation.GetText(langCode, "image_connect_url"),
		buildConnectText(customer, langCode),
		markup,
	)
}

func buildConnectText(customer *database.Customer, langCode string) string {
	var info strings.Builder
	tm := translation.GetInstance()

	// ExpireAt у тебя *time.Time
	if customer.ExpireAt == nil {
		info.WriteString(tm.GetText(langCode, "no_subscription"))
		return info.String()
	}

	currentTime := time.Now()

	// Активна
	if currentTime.Before(*customer.ExpireAt) {
		formattedDate := customer.ExpireAt.Format("02.01.2006 15:04")

		subscriptionActiveText := tm.GetText(langCode, "subscription_active")
		info.WriteString(fmt.Sprintf(subscriptionActiveText, formattedDate))

		if customer.SubscriptionLink != nil && *customer.SubscriptionLink != "" {
			if config.GetMiniAppURL() != "" || config.IsWepAppLinkEnabled() {
				// ничего
			} else {
				subscriptionLinkText := tm.GetText(langCode, "subscription_link")
				info.WriteString(fmt.Sprintf(subscriptionLinkText, *customer.SubscriptionLink))
			}
		}

		return info.String()
	}

	// Просрочена
	info.WriteString(tm.GetText(langCode, "no_subscription"))
	return info.String()
}
