package handler

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"

	"link-bot/internal/database"
	"link-bot/internal/payment"
	planbook "link-bot/internal/plans"
	"link-bot/internal/runtimeconfig"
)

// ✅ НУЖНО ДЛЯ cmd/app/main.go (роут уже у тебя прописан)
const (
	CallbackCancelStarsUI = "cancel_stars_ui"

	botPaymentCardEmojiID   = "5192678313415434135"
	botPaymentCryptoEmojiID = "5195058841988914267"
	botPaymentStarsEmojiID  = "5242644275014951846"
)

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

func (h Handler) BuyCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.CallbackQuery.Message.Message
	if msg == nil {
		slog.Error("BuyCallbackHandler: callback message is nil")
		return
	}
	langCode := update.CallbackQuery.From.LanguageCode

	keyboard := h.buildPlanSelectionKeyboard(langCode)
	if len(keyboard) == 0 {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Тарифы временно недоступны",
			ShowAlert:       true,
		})
		return
	}
	keyboard = append(keyboard, []models.InlineKeyboardButton{h.premiumBackButton(CallbackStart)})
	commerce := h.telegramCommerceSettings()

	h.sendScreenPhotoReplacing(
		ctx,
		b,
		msg.Chat.ID,
		msg.ID,
		commerce.Banner,
		commerce.TariffsText,
		keyboard,
	)

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

func (h Handler) SellCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	if callback == nil {
		slog.Error("SellCallbackHandler: callback message is nil")
		return
	}

	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	langCode := update.CallbackQuery.From.LanguageCode
	plan, ok := h.planFromCallbackQuery(callbackQuery)
	if !ok {
		slog.Error("SellCallbackHandler: unsupported plan", "query", update.CallbackQuery.Data)
		return
	}

	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	customer, err := h.customerRepository.FindByTelegramId(ctx2, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("SellCallbackHandler: find customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("SellCallbackHandler: customer not found", "telegramId", update.CallbackQuery.From.ID)
		return
	}

	keyboard, err := h.buildPaymentMethodsKeyboard(ctx2, langCode, customer, plan)
	if err != nil {
		slog.Error("SellCallbackHandler: build payment methods", "error", err)
		return
	}

	h.editScreenTextAndMarkup(ctx2, b, callback.Chat.ID, callback.ID, h.telegramCommerceSettings().PaymentMethodsText, keyboard)
}

func (h Handler) PaymentCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	menuMsg := update.CallbackQuery.Message.Message
	if menuMsg == nil {
		slog.Error("PaymentCallbackHandler: callback message is nil")
		return
	}

	q := parseCallbackData(update.CallbackQuery.Data)
	invoiceType := database.InvoiceType(q["invoiceType"])
	langCode := update.CallbackQuery.From.LanguageCode
	plan, ok := h.planFromCallbackQuery(q)
	if !ok {
		slog.Error("PaymentCallbackHandler: unsupported plan", "query", update.CallbackQuery.Data)
		return
	}

	ctx2, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	customer, err := h.customerRepository.FindByTelegramId(ctx2, menuMsg.Chat.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "chatID", menuMsg.Chat.ID)
		return
	}

	allowedMethods, err := h.availablePaymentMethods(ctx2, customer)
	if err != nil {
		slog.Error("Error resolving payment methods", "error", err)
		return
	}
	if !h.isPaymentMethodAllowedForPlan(allowedMethods, plan, invoiceType) {
		_, _ = b.AnswerCallbackQuery(ctx2, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Этот способ оплаты недоступен для выбранного тарифа",
			ShowAlert:       false,
		})
		return
	}

	price, ok := planbook.AmountForInvoice(plan, invoiceType)
	if !ok {
		slog.Error("PaymentCallbackHandler: price unavailable for plan", "planId", plan.ID, "invoiceType", invoiceType)
		return
	}

	ctxWithProfile := contextWithTelegramProfile(ctx2, update.CallbackQuery.From)
	paymentURL, purchaseId, err := h.paymentService.CreatePurchaseWithOptions(ctxWithProfile, float64(price), plan.Months, customer, invoiceType, payment.CreatePurchaseOptions{
		PlanID:            plan.ID,
		TrafficLimitBytes: &plan.TrafficLimitBytes,
		DeviceLimitCount:  &plan.DeviceLimitCount,
	})
	if err != nil {
		slog.Error("Error creating payment", "error", err)
		return
	}

	// ✅ STARS
	if invoiceType == database.InvoiceTypeTelegram {
		// 1) Отправляем инвойс Stars
		payload := fmt.Sprintf("%d&%s", purchaseId, update.CallbackQuery.From.Username)

		invMsg, err := b.SendInvoice(ctx2, &bot.SendInvoiceParams{
			ChatID:      menuMsg.Chat.ID,
			Title:       h.translation.GetText(langCode, "invoice_title"),
			Description: h.translation.GetText(langCode, "invoice_description"),
			Payload:     payload,
			Currency:    "XTR",
			Prices: []models.LabeledPrice{
				{Label: h.translation.GetText(langCode, "invoice_label"), Amount: price},
			},
			ProviderToken: "",
		})
		if err != nil {
			slog.Error("Error sending stars invoice", "error", err)
			return
		}

		cancelCb := fmt.Sprintf("%s?inv=%d&planId=%s", CallbackCancelStarsUI, invMsg.ID, plan.ID)

		cancelMsg, _ := b.SendMessage(ctx2, &bot.SendMessageParams{
			ChatID: menuMsg.Chat.ID,
			Text:   "\u2063",
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{
						{Text: "❌ Отмена", CallbackData: cancelCb},
					},
				},
			},
		})

		// 3) УДАЛЯЕМ верхнее меню-сообщение (чтобы не оставалось “Назад” и т.д.)
		_, _ = b.DeleteMessage(ctx2, &bot.DeleteMessageParams{
			ChatID:    menuMsg.Chat.ID,
			MessageID: menuMsg.ID,
		})

		_ = cancelMsg

		return
	}

	keyboard := [][]models.InlineKeyboardButton{
		{
			h.premiumPayURLButton(paymentURL),
		},
		{
			h.premiumBackButton(fmt.Sprintf("%s?planId=%s", CallbackSell, plan.ID)),
		},
	}

	h.editScreenTextAndMarkup(ctx2, b, menuMsg.Chat.ID, menuMsg.ID, h.telegramCommerceSettings().PaymentReadyText, keyboard)

	h.cache.Set(purchaseId, menuMsg.ID)

	_ = paymentURL
}

// ✅ handler для cmd/app/main.go
func (h Handler) CancelStarsUIHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	msg := cb.Message.Message
	if msg == nil {
		slog.Error("CancelStarsUIHandler: callback message is nil")
		return
	}

	langCode := cb.From.LanguageCode
	q := parseCallbackData(cb.Data)

	invID, _ := strconv.Atoi(q["inv"])
	plan, ok := h.planFromCallbackQuery(q)

	if invID > 0 {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    msg.Chat.ID,
			MessageID: invID,
		})
	}

	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
	})

	keyboard := h.buildPlanSelectionKeyboard(langCode)
	commerce := h.telegramCommerceSettings()
	caption := commerce.TariffsText
	if ok {
		ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		customer, err := h.customerRepository.FindByTelegramId(ctx2, cb.From.ID)
		if err != nil {
			slog.Error("CancelStarsUIHandler: find customer", "error", err)
		} else if customer != nil {
			if paymentKeyboard, err := h.buildPaymentMethodsKeyboard(ctx2, langCode, customer, plan); err != nil {
				slog.Error("CancelStarsUIHandler: build payment methods", "error", err)
			} else {
				keyboard = paymentKeyboard
				caption = commerce.PaymentMethodsText
			}
		}
	}

	h.sendScreenPhoto(ctx, b,
		msg.Chat.ID,
		commerce.Banner,
		caption,
		keyboard,
	)

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cb.ID,
		Text:            "Ок, отменено.",
		ShowAlert:       false,
	})
}

func (h Handler) PreCheckoutCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, err := b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
		PreCheckoutQueryID: update.PreCheckoutQuery.ID,
		OK:                 true,
	})
	if err != nil {
		slog.Error("Error sending answer pre checkout query", "error", err)
	}
}

func (h Handler) SuccessPaymentHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	payload := strings.Split(update.Message.SuccessfulPayment.InvoicePayload, "&")
	purchaseId, err := strconv.Atoi(payload[0])
	username := payload[1]
	if err != nil {
		slog.Error("Error parsing purchase id", "error", err)
		return
	}

	ctxWithProfile := ctx
	if update.Message.From != nil {
		ctxWithProfile = contextWithTelegramProfile(ctxWithProfile, *update.Message.From)
	}
	if strings.TrimSpace(username) != "" {
		ctxWithProfile = context.WithValue(ctxWithProfile, "username", strings.TrimSpace(username))
	}
	err = h.paymentService.ProcessPurchaseById(ctxWithProfile, int64(purchaseId))
	if err != nil {
		slog.Error("Error processing purchase", "error", err)
	}
}

func (h Handler) buildPaymentMethodsKeyboard(ctx context.Context, langCode string, customer *database.Customer, plan planbook.CheckoutPlan) ([][]models.InlineKeyboardButton, error) {
	methods, err := h.availablePaymentMethods(ctx, customer)
	if err != nil {
		return nil, err
	}

	var firstRow []models.InlineKeyboardButton
	commerce := h.telegramCommerceSettings()
	if h.isPaymentMethodAllowedForPlan(methods, plan, database.InvoiceTypeYookasa) {
		button := telegramButton(commerce.YookassaButton, "СБП | Карта", botPaymentCardEmojiID)
		button.CallbackData = fmt.Sprintf("%s?planId=%s&invoiceType=%s", CallbackPayment, plan.ID, database.InvoiceTypeYookasa)
		firstRow = append(firstRow, button)
	}
	if h.isPaymentMethodAllowedForPlan(methods, plan, database.InvoiceTypeCrypto) {
		button := telegramButton(commerce.CryptoButton, "CryptoPay", botPaymentCryptoEmojiID)
		button.CallbackData = fmt.Sprintf("%s?planId=%s&invoiceType=%s", CallbackPayment, plan.ID, database.InvoiceTypeCrypto)
		firstRow = append(firstRow, button)
	}

	kb := [][]models.InlineKeyboardButton{}
	if len(firstRow) > 0 {
		kb = append(kb, firstRow)
	}
	if h.isPaymentMethodAllowedForPlan(methods, plan, database.InvoiceTypeTelegram) {
		button := telegramButton(commerce.StarsButton, "Telegram Stars", botPaymentStarsEmojiID)
		button.CallbackData = fmt.Sprintf("%s?planId=%s&invoiceType=%s", CallbackPayment, plan.ID, database.InvoiceTypeTelegram)
		kb = append(kb, []models.InlineKeyboardButton{
			button,
		})
	}
	kb = append(kb, []models.InlineKeyboardButton{h.premiumBackButton(CallbackBuy)})

	return kb, nil
}

func (h Handler) sendScreenPhoto(
	ctx context.Context,
	b *bot.Bot,
	chatID int64,
	imageURL string,
	caption string,
	inlineKeyboard [][]models.InlineKeyboardButton,
) *models.Message {
	if strings.TrimSpace(imageURL) == "" {
		return h.sendScreenTextFallback(ctx, b, chatID, caption, inlineKeyboard)
	}
	data, filename, err := readImageSource(imageURL)
	if err != nil {
		slog.Error("screen image download failed", "error", err)
		return h.sendScreenTextFallback(ctx, b, chatID, caption, inlineKeyboard)
	}
	msg, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID: chatID,
		Photo: &models.InputFileUpload{
			Filename: filename,
			Data:     bytes.NewReader(data),
		},
		Caption:   caption,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
	})
	if err != nil {
		slog.Error("Error sending screen photo", "error", err)
		return h.sendScreenTextFallback(ctx, b, chatID, caption, inlineKeyboard)
	}
	h.rememberScreenMessage(chatID, msg.ID)
	return msg
}

func (h Handler) telegramCommerceSettings() runtimeconfig.TelegramCommerceSettings {
	if h.runtimeSettings == nil {
		return runtimeconfig.DefaultSettings().Content.Commerce
	}
	return h.runtimeSettings.Snapshot().Content.Commerce
}

func (h Handler) sendScreenPhotoReplacing(
	ctx context.Context,
	b *bot.Bot,
	chatID int64,
	replaceMessageID int,
	imageURL string,
	caption string,
	inlineKeyboard [][]models.InlineKeyboardButton,
) {
	msg := h.sendScreenPhoto(ctx, b, chatID, imageURL, caption, inlineKeyboard)
	if msg == nil || replaceMessageID <= 0 || msg.ID == replaceMessageID {
		return
	}

	if _, err := b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: replaceMessageID,
	}); err != nil {
		slog.Warn("Error deleting replaced screen message", "error", err)
	}
}

func (h Handler) sendScreenTextFallback(
	ctx context.Context,
	b *bot.Bot,
	chatID int64,
	text string,
	inlineKeyboard [][]models.InlineKeyboardButton,
) *models.Message {
	msg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
	})
	if err != nil {
		slog.Error("Error sending screen text fallback", "error", err)
		msg, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   plainTelegramText(text),
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: inlineKeyboard,
			},
		})
		if err != nil {
			slog.Error("Error sending plain screen text fallback", "error", err)
			return nil
		}
	}
	if msg != nil {
		h.rememberScreenMessage(chatID, msg.ID)
	}
	return msg
}

func (h Handler) editScreenTextAndMarkup(
	ctx context.Context,
	b *bot.Bot,
	chatID int64,
	messageID int,
	text string,
	inlineKeyboard [][]models.InlineKeyboardButton,
) {
	markup := models.InlineKeyboardMarkup{InlineKeyboard: inlineKeyboard}
	_, captionErr := b.EditMessageCaption(ctx, &bot.EditMessageCaptionParams{
		ChatID:      chatID,
		MessageID:   messageID,
		Caption:     text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: markup,
	})
	if captionErr == nil {
		h.rememberScreenMessage(chatID, messageID)
		return
	}

	_, textErr := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: markup,
	})
	if textErr == nil {
		h.rememberScreenMessage(chatID, messageID)
		return
	}

	slog.Error("Error editing screen text", "captionError", captionErr, "textError", textErr)
	_, markupErr := b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: markup,
	})
	if markupErr != nil {
		slog.Error("Error editing screen markup fallback", "error", markupErr)
	}
}

func plainTelegramText(text string) string {
	text = htmlTagPattern.ReplaceAllString(text, "")
	text = strings.NewReplacer(
		"&nbsp;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
	).Replace(text)
	text = strings.TrimSpace(text)
	if text == "" {
		return "Выберите подходящий тариф."
	}
	return text
}

func parseCallbackData(data string) map[string]string {
	result := make(map[string]string)

	parts := strings.Split(data, "?")
	if len(parts) < 2 {
		return result
	}

	params := strings.Split(parts[1], "&")
	for _, param := range params {
		kv := strings.SplitN(param, "=", 2)
		if len(kv) == 2 {
			result[kv[0]] = kv[1]
		}
	}

	return result
}

func (h Handler) buildPlanSelectionKeyboard(langCode string) [][]models.InlineKeyboardButton {
	plans := planbook.All()
	if h.runtimeSettings != nil {
		plans = h.runtimeSettings.CheckoutPlans()
	}
	keyboard := make([][]models.InlineKeyboardButton, 0, (len(plans)+1)/2)
	row := make([]models.InlineKeyboardButton, 0, 2)
	for _, plan := range plans {
		if plan.PriceRub <= 0 && plan.PriceStars <= 0 {
			continue
		}
		row = append(row, models.InlineKeyboardButton{
			Text:         h.botPlanButtonText(plan, langCode),
			CallbackData: fmt.Sprintf("%s?planId=%s", CallbackSell, plan.ID),
		})
		if len(row) == 2 {
			keyboard = append(keyboard, row)
			row = make([]models.InlineKeyboardButton, 0, 2)
		}
	}
	if len(row) > 0 {
		keyboard = append(keyboard, row)
	}

	return keyboard
}

func (h Handler) availablePaymentMethods(ctx context.Context, customer *database.Customer) (map[string]bool, error) {
	yookassaEnabled := h.paymentService != nil && h.paymentService.IsProviderEnabled("yookassa")
	cryptoEnabled := h.paymentService != nil && h.paymentService.IsProviderEnabled("cryptopay")
	methods := map[string]bool{
		"card":   h.featureEnabled("yookassa") && yookassaEnabled,
		"crypto": h.featureEnabled("crypto") && cryptoEnabled,
		"stars":  h.featureEnabled("stars"),
	}

	return methods, nil
}

func (h Handler) planFromCallbackQuery(query map[string]string) (planbook.CheckoutPlan, bool) {
	months, _ := strconv.Atoi(strings.TrimSpace(query["month"]))
	if h.runtimeSettings != nil {
		return h.runtimeSettings.CheckoutPlan(query["planId"], months)
	}
	return planbook.ForIDOrMonths(query["planId"], months)
}

func (h Handler) isPaymentMethodAllowedForPlan(methods map[string]bool, plan planbook.CheckoutPlan, invoiceType database.InvoiceType) bool {
	switch invoiceType {
	case database.InvoiceTypeYookasa:
		return methods["card"] && plan.PriceRub > 0
	case database.InvoiceTypeCrypto:
		return methods["crypto"] && plan.PriceRub > 0
	case database.InvoiceTypeTelegram:
		return methods["stars"] && plan.PriceStars > 0
	default:
		return false
	}
}

func (h Handler) botPlanButtonText(plan planbook.CheckoutPlan, langCode string) string {
	priceLabel := fmt.Sprintf("%d ₽", plan.PriceRub)
	if strings.HasPrefix(strings.ToLower(langCode), "en") {
		priceLabel = fmt.Sprintf("%d RUB", plan.PriceRub)
	}

	if plan.Variant == planbook.VariantUnlimited {
		if strings.HasPrefix(strings.ToLower(langCode), "en") {
			return fmt.Sprintf("∞ %s | %s", h.botPlanMonthsLabel(plan.Months, langCode), priceLabel)
		}
		return fmt.Sprintf("∞ %s | %s", h.botPlanMonthsLabel(plan.Months, langCode), priceLabel)
	}

	switch plan.Months {
	case 12:
		if strings.HasPrefix(strings.ToLower(langCode), "en") {
			return fmt.Sprintf("🔥 1 Year | %s", priceLabel)
		}
		return fmt.Sprintf("🔥 1 Год | %s", priceLabel)
	default:
		if strings.HasPrefix(strings.ToLower(langCode), "en") {
			return fmt.Sprintf("%s | %s", h.botPlanMonthsLabel(plan.Months, langCode), priceLabel)
		}
		return fmt.Sprintf("%s | %s", h.botPlanMonthsLabel(plan.Months, langCode), priceLabel)
	}
}

func (h Handler) botPlanMonthsLabel(months int, langCode string) string {
	if strings.HasPrefix(strings.ToLower(langCode), "en") {
		switch months {
		case 1:
			return "1 Mo"
		case 3:
			return "3 Mo"
		case 6:
			return "6 Mo"
		case 12:
			return "1 Year"
		default:
			return fmt.Sprintf("%d Mo", months)
		}
	}

	switch months {
	case 1:
		return "1 Мес"
	case 3:
		return "3 Мес"
	case 6:
		return "6 Мес"
	case 12:
		return "1 Год"
	default:
		return fmt.Sprintf("%d Мес", months)
	}
}

func (h Handler) buildPlanRow(langCode string, plansByID map[string]planbook.CheckoutPlan, ids ...string) []models.InlineKeyboardButton {
	row := make([]models.InlineKeyboardButton, 0, len(ids))
	for _, id := range ids {
		plan, ok := plansByID[id]
		if !ok {
			continue
		}
		row = append(row, models.InlineKeyboardButton{
			Text:         h.botPlanButtonText(plan, langCode),
			CallbackData: fmt.Sprintf("%s?planId=%s", CallbackSell, plan.ID),
		})
	}

	return row
}
