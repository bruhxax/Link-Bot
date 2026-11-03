package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/payment"
	"link-bot/internal/plans"
)

func inlineGiftPlanKey(planID string) string {
	hash := sha256.Sum256([]byte(planID))
	return hex.EncodeToString(hash[:8])
}

func inlineGiftPendingText(template, planTitle, sender string) string {
	return strings.NewReplacer("{plan}", html.EscapeString(planTitle), "{sender}", html.EscapeString(sender)).Replace(template)
}

func (h *Handler) InlineGiftQueryHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.InlineQuery
	if query == nil {
		return
	}
	if query.From == nil || h.runtimeSettings == nil || config.GetMiniAppURL() == "" {
		slog.Warn("inline gift query unavailable", "has_sender", query.From != nil, "has_settings", h.runtimeSettings != nil, "has_mini_app", config.GetMiniAppURL() != "")
		h.answerInlineGiftUnavailable(ctx, b, query.ID, "Подарки временно недоступны")
		return
	}
	settings := h.runtimeSettings.Snapshot().Content.Gift.Inline
	bannerURL := payment.InlineGiftBannerURL(settings.BannerURL, config.GetMiniAppURL())
	search := strings.ToLower(strings.TrimSpace(query.Query))
	results := make([]models.InlineQueryResult, 0, 50)
	visible := make([]plans.CheckoutPlan, 0)
	for _, plan := range h.runtimeSettings.CheckoutPlans() {
		if plan.PriceRub <= 0 && plan.PriceStars <= 0 {
			continue
		}
		title := h.runtimeSettings.PlanTitle(plan.ID, query.From.LanguageCode)
		if search != "" && !strings.Contains(strings.ToLower(title+" "+plan.ID+" "+plan.Name), search) {
			continue
		}
		visible = append(visible, plan)
	}
	if len(visible) == 0 && search == "" {
		slog.Warn("inline gift query has no paid plans")
		h.answerInlineGiftUnavailable(ctx, b, query.ID, "Нет доступных тарифов для подарка")
		return
	}
	offset, _ := strconv.Atoi(query.Offset)
	if offset < 0 || offset > len(visible) {
		offset = 0
	}
	end := offset + 50
	if end > len(visible) {
		end = len(visible)
	}
	senderName := "пользователь"
	if query.From.Username != "" {
		senderName = "@" + query.From.Username
	}
	for _, plan := range visible[offset:end] {
		title := h.runtimeSettings.PlanTitle(plan.ID, query.From.LanguageCode)
		if title == "" {
			title = plan.Name
		}
		caption := inlineGiftPendingText(settings.PendingText, title, senderName)
		price := fmt.Sprintf("%d ₽", plan.PriceRub)
		if plan.PriceRub <= 0 {
			price = fmt.Sprintf("%d ⭐", plan.PriceStars)
		}
		mediaKind := "t"
		if bannerURL != "" {
			mediaKind = "b"
		}
		keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: settings.PayButtonText, CallbackData: fmt.Sprintf("ig:pay:%d:%s:%s", query.From.ID, inlineGiftPlanKey(plan.ID), mediaKind)},
		}}}
		if bannerURL != "" {
			results = append(results, &models.InlineQueryResultPhoto{ID: plan.ID, PhotoURL: bannerURL, ThumbnailURL: bannerURL,
				Title: title, Description: price, Caption: caption, ParseMode: models.ParseModeHTML, ReplyMarkup: keyboard})
		} else {
			results = append(results, &models.InlineQueryResultArticle{ID: plan.ID, Title: title, Description: price,
				InputMessageContent: models.InputTextMessageContent{MessageText: caption, ParseMode: models.ParseModeHTML}, ReplyMarkup: keyboard})
		}
	}
	nextOffset := ""
	if end < len(visible) {
		nextOffset = strconv.Itoa(end)
	}
	if _, err := b.AnswerInlineQuery(ctx, &bot.AnswerInlineQueryParams{InlineQueryID: query.ID, Results: results, IsPersonal: true, CacheTime: 1, NextOffset: nextOffset}); err != nil {
		slog.Error("inline gift query failed", "error", err)
	}
}

func (h *Handler) answerInlineGiftUnavailable(ctx context.Context, b *bot.Bot, queryID, title string) {
	_, err := b.AnswerInlineQuery(ctx, &bot.AnswerInlineQueryParams{
		InlineQueryID: queryID,
		Results: []models.InlineQueryResult{&models.InlineQueryResultArticle{
			ID: "gift-unavailable", Title: title, Description: "Откройте бота позже",
			InputMessageContent: models.InputTextMessageContent{MessageText: title},
		}},
		IsPersonal: true, CacheTime: 1,
	})
	if err != nil {
		slog.Error("inline gift unavailable response failed", "error", err)
	}
}

func (h *Handler) InlineGiftPayHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery
	if callback == nil {
		return
	}
	answer := func(message string) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: message, ShowAlert: message != ""})
	}
	parts := strings.Split(callback.Data, ":")
	if len(parts) != 5 || callback.InlineMessageID == "" || h.runtimeSettings == nil || (parts[4] != "t" && parts[4] != "b") {
		answer("Не удалось открыть подарок")
		return
	}
	senderID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || senderID != callback.From.ID {
		answer("Оплатить может только отправитель подарка")
		return
	}
	var selected *plans.CheckoutPlan
	for _, plan := range h.runtimeSettings.CheckoutPlans() {
		if inlineGiftPlanKey(plan.ID) == parts[3] && (plan.PriceRub > 0 || plan.PriceStars > 0) {
			selected = &plan
			break
		}
	}
	if selected == nil {
		answer("Тариф больше недоступен")
		return
	}
	draft, err := h.purchaseRepository.GetOrCreateInlineGift(ctx, database.InlineGift{
		ID: uuid.New(), SenderTelegramID: senderID, PlanID: selected.ID,
		InlineMessageID: callback.InlineMessageID,
		HasBanner:       parts[4] == "b",
	})
	if err != nil || draft == nil {
		slog.Error("inline gift draft failed", "error", err)
		answer("Не удалось подготовить оплату")
		return
	}
	if draft.SenderTelegramID != senderID || draft.PlanID != selected.ID {
		answer("Этот подарок уже занят")
		return
	}
	if draft.PurchaseID != nil {
		answer("Оплата этого подарка уже создана")
		return
	}
	deepLink, err := url.Parse(config.BotURL())
	if err != nil || deepLink.Host == "" {
		answer("Бот временно недоступен")
		return
	}
	deepLink.RawQuery = "start=ig_" + draft.ID.String()
	_, err = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		InlineMessageID: draft.InlineMessageID,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: h.runtimeSettings.Snapshot().Content.Gift.Inline.PayButtonText, URL: deepLink.String()},
		}}},
	})
	if err != nil {
		slog.Error("inline gift payment link failed", "error", err)
		answer("Не удалось открыть оплату")
		return
	}
	answer("Откройте бота, чтобы выбрать способ оплаты")
}

func (h *Handler) sendInlineGiftPaymentEntry(ctx context.Context, b *bot.Bot, chatID int64, startParam string) {
	id, err := uuid.Parse(strings.TrimPrefix(startParam, "ig_"))
	if err != nil || h.purchaseRepository == nil {
		return
	}
	draft, err := h.purchaseRepository.FindInlineGift(ctx, id)
	if err != nil || draft == nil || draft.SenderTelegramID != chatID {
		return
	}
	if draft.PurchaseID != nil {
		purchase, findErr := h.purchaseRepository.FindById(ctx, *draft.PurchaseID)
		if findErr == nil && purchase != nil {
			if purchase.Status == database.PurchaseStatusPaid {
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Подарок оплачен. Получатель может забрать его в чате."})
				return
			}
			message := "Платёж уже создан. Дождитесь его подтверждения."
			var markup *models.InlineKeyboardMarkup
			if purchase.ExternalPaymentURL != nil {
				if link, parseErr := url.Parse(*purchase.ExternalPaymentURL); parseErr == nil && link.Scheme == "https" && link.Host != "" {
					message = "Платёж уже создан. Продолжите оплату по ссылке."
					markup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Продолжить оплату", URL: link.String()}}}}
				}
			}
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: message, ReplyMarkup: markup})
			return
		}
	}
	appURL, err := url.Parse(config.GetMiniAppURL())
	if err != nil || appURL.Host == "" {
		return
	}
	query := appURL.Query()
	query.Set("inlineGift", id.String())
	query.Set("planId", draft.PlanID)
	appURL.RawQuery = query.Encode()
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID,
		Text: h.runtimeSettings.Snapshot().Content.Gift.Inline.PaymentText,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: h.runtimeSettings.Snapshot().Content.Gift.Inline.PayButtonText, WebApp: &models.WebAppInfo{URL: appURL.String()}},
		}}},
	})
	if err != nil {
		slog.Error("inline gift Mini App entry failed", "error", err)
	}
}
