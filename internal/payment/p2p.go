package payment

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/utils"
)

const p2PCallbackPrefix = "p2p:"

var ErrP2PPaymentAlreadyReviewed = errors.New("P2P payment has already been reviewed")

func (s PaymentService) createP2PInvoice(ctx context.Context, amount float64, months int, customer *database.Customer, options CreatePurchaseOptions) (string, int64, error) {
	reference := strings.TrimSpace(options.P2PSenderReference)
	destination := options.P2PDestination
	if customer == nil || amount <= 0 || reference == "" || destination.Title == "" || destination.Details == "" {
		return "", 0, errors.New("данные P2P-перевода заполнены не полностью")
	}
	if len([]rune(reference)) > 500 {
		return "", 0, errors.New("данные отправителя слишком длинные")
	}

	purchaseID, err := s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType: database.InvoiceTypeP2P, Status: database.PurchaseStatusPending, Amount: amount, Currency: "RUB",
		CustomerID: customer.ID, SubscriptionID: options.SubscriptionID, Month: months, PlanID: optionalTrimmedStringPointer(options.PlanID),
		TrafficLimitBytes: options.TrafficLimitBytes, DeviceLimitCount: options.DeviceLimitCount,
		AgreementAccepted: options.AgreementAccepted, IsAutoPayment: options.IsAutoPayment,
		ParentPurchaseID: options.ParentPurchaseID, PromoCodeID: options.PromoCodeID,
		PromoCodeSnapshot:        optionalTrimmedStringPointer(options.PromoCodeCode),
		PromoCodeDiscountPercent: optionalPositiveIntPointer(options.PromoDiscountPercent),
		PurchaseKind:             options.PurchaseKind,
		ExtraDevices:             options.ExtraDevices,
		IsFreePlan:               options.IsFreePlan,
		FreePlanOneTime:          options.FreePlanOneTime,
		GiftRecipientUsername:    optionalTrimmedStringPointer(options.GiftRecipientUsername),
		GiftRecipientCustomerID:  options.GiftRecipientCustomerID,
		GiftToken:                options.GiftToken,
	})
	if err != nil {
		return "", 0, err
	}

	request := &database.P2PPaymentRequest{
		PurchaseID:          purchaseID,
		DestinationSnapshot: destination,
		SenderReference:     reference,
		Status:              database.P2PPaymentStatusPending,
		SubmittedAt:         time.Now().UTC(),
	}
	if err := s.purchaseRepository.CreateP2PPaymentRequest(ctx, request); err != nil {
		_ = s.purchaseRepository.UpdateFields(ctx, purchaseID, map[string]interface{}{"status": database.PurchaseStatusCancel})
		return "", purchaseID, err
	}
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseID)
	if err != nil || purchase == nil {
		_, _ = s.purchaseRepository.RejectP2PPayment(ctx, purchaseID, 0)
		if err == nil {
			err = errors.New("созданный P2P-платёж не найден")
		}
		return "", purchaseID, err
	}
	chatID, messageID, err := s.sendP2PReviewNotification(ctx, purchase, customer, request)
	if err != nil {
		_, _ = s.purchaseRepository.RejectP2PPayment(ctx, purchaseID, 0)
		return "", purchaseID, err
	}
	if err := s.purchaseRepository.SetP2PNotificationMessage(ctx, purchaseID, chatID, messageID); err != nil {
		slog.Warn("payment: P2P review notification delivered but message id was not saved", "purchase_id", utils.MaskHalfInt64(purchaseID), "error", err)
	}
	return "", purchaseID, nil
}

func (s PaymentService) sendP2PReviewNotification(ctx context.Context, purchase *database.Purchase, customer *database.Customer, request *database.P2PPaymentRequest) (int64, int64, error) {
	token, chatID, timezone := s.paymentNotificationConfig()
	if token == "" || chatID == 0 {
		return 0, 0, errors.New("бот уведомлений для проверки P2P-платежей не настроен")
	}
	orderNumber := purchase.ID
	if assigned, err := s.purchaseRepository.GetOrAssignPaymentOrderNumber(ctx, purchase.ID); err == nil {
		orderNumber = assigned
	}
	username := usernameFromContext(ctx)
	if username == "" && customer != nil && customer.TelegramUsername != nil {
		username = strings.TrimSpace(*customer.TelegramUsername)
	}
	base := buildPaymentNotificationMessageWithTemplate(
		s.paymentNotificationSettings().Text,
		purchase,
		username,
		"P2P перевод",
		request.SubmittedAt,
		orderNumber,
		timezone,
	)
	message := buildP2PReviewNotificationMessage(base, request)
	keyboard := &telegramInlineKeyboardMarkup{InlineKeyboard: [][]telegramInlineKeyboardButton{{
		{Text: "✅ Принять", CallbackData: fmt.Sprintf("%sapprove:%d", p2PCallbackPrefix, purchase.ID), Style: "success"},
		{Text: "❌ Отклонить", CallbackData: fmt.Sprintf("%sreject:%d", p2PCallbackPrefix, purchase.ID), Style: "danger"},
	}}}
	notifyCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	messageID, err := sendTelegramNotificationMessage(notifyCtx, token, chatID, message, keyboard)
	if err != nil {
		return 0, 0, fmt.Errorf("отправить P2P-платёж на проверку: %w", err)
	}
	return chatID, messageID, nil
}

func buildP2PReviewNotificationMessage(base string, request *database.P2PPaymentRequest) string {
	if request == nil {
		return strings.TrimSpace(base)
	}
	destination := request.DestinationSnapshot
	parts := []string{
		"🕓 <b>P2P-перевод ожидает проверки</b>",
		strings.TrimSpace(base),
		"👤 <b>Данные отправителя:</b>\n<blockquote>" + html.EscapeString(request.SenderReference) + "</blockquote>",
		"🏦 <b>Куда перевёл:</b> " + html.EscapeString(destination.Title) + "\n<code>" + html.EscapeString(destination.Details) + "</code>",
	}
	if description := strings.TrimSpace(destination.Description); description != "" {
		parts = append(parts, "ℹ️ "+html.EscapeString(description))
	}
	return strings.Join(parts, "\n\n")
}

func (s *PaymentService) RegisterP2PNotificationHandlers(notificationBot *bot.Bot, allowedChatID int64) {
	if s == nil || notificationBot == nil || allowedChatID == 0 {
		return
	}
	notificationBot.RegisterHandler(bot.HandlerTypeCallbackQueryData, p2PCallbackPrefix, bot.MatchTypePrefix, func(ctx context.Context, b *bot.Bot, update *models.Update) {
		s.handleP2PReviewCallback(ctx, b, update, allowedChatID)
	})
}

func (s *PaymentService) handleP2PReviewCallback(ctx context.Context, b *bot.Bot, update *models.Update, allowedChatID int64) {
	if update == nil || update.CallbackQuery == nil {
		return
	}
	callback := update.CallbackQuery
	message := callback.Message.Message
	if message == nil || message.Chat.ID != allowedChatID || callback.From.ID != config.GetAdminTelegramId() {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: "Недостаточно прав", ShowAlert: true})
		return
	}
	action, purchaseID, ok := parseP2PCallback(callback.Data)
	if !ok {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: "Некорректная заявка", ShowAlert: true})
		return
	}
	decisionCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	var err error
	switch action {
	case "approve":
		err = s.approveP2PPayment(decisionCtx, purchaseID, callback.From.ID)
	case "reject":
		err = s.rejectP2PPayment(decisionCtx, purchaseID, callback.From.ID)
	}
	if err != nil {
		text := "Не удалось обработать платёж"
		if errors.Is(err, ErrP2PPaymentAlreadyReviewed) {
			text = "По этому платежу уже вынесен вердикт"
		}
		slog.Error("payment: P2P review failed", "purchase_id", utils.MaskHalfInt64(purchaseID), "action", action, "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: text, ShowAlert: true})
		return
	}
	_, deleteErr := b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: message.Chat.ID, MessageID: message.ID})
	if deleteErr != nil {
		slog.Warn("payment: failed to delete reviewed P2P notification", "purchase_id", utils.MaskHalfInt64(purchaseID), "error", deleteErr)
	}
	answer := "Платёж принят, подписка выдана"
	if action == "reject" {
		answer = "Платёж отклонён"
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: answer})
}

func parseP2PCallback(raw string) (string, int64, bool) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 3 || parts[0] != "p2p" || (parts[1] != "approve" && parts[1] != "reject") {
		return "", 0, false
	}
	purchaseID, err := strconv.ParseInt(parts[2], 10, 64)
	return parts[1], purchaseID, err == nil && purchaseID > 0
}

func (s *PaymentService) approveP2PPayment(ctx context.Context, purchaseID, adminID int64) error {
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseID)
	if err != nil {
		return err
	}
	if purchase == nil || purchase.InvoiceType != database.InvoiceTypeP2P {
		return errors.New("P2P purchase not found")
	}
	claimed, err := s.purchaseRepository.ClaimP2PApproval(ctx, purchaseID, adminID)
	if err != nil {
		return err
	}
	if !claimed {
		return ErrP2PPaymentAlreadyReviewed
	}
	if err := s.ProcessPurchaseById(ctx, purchaseID); err != nil {
		if releaseErr := s.purchaseRepository.ReleaseP2PApproval(context.Background(), purchaseID); releaseErr != nil {
			slog.Error("payment: failed to release P2P approval", "purchase_id", utils.MaskHalfInt64(purchaseID), "error", releaseErr)
		}
		return err
	}
	return s.purchaseRepository.MarkP2PApproved(ctx, purchaseID, adminID)
}

func (s *PaymentService) rejectP2PPayment(ctx context.Context, purchaseID, adminID int64) error {
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseID)
	if err != nil {
		return err
	}
	request, err := s.purchaseRepository.FindP2PPaymentRequest(ctx, purchaseID)
	if err != nil {
		return err
	}
	if purchase == nil || request == nil || purchase.InvoiceType != database.InvoiceTypeP2P {
		return errors.New("P2P purchase not found")
	}
	rejected, err := s.purchaseRepository.RejectP2PPayment(ctx, purchaseID, adminID)
	if err != nil {
		return err
	}
	if !rejected {
		return ErrP2PPaymentAlreadyReviewed
	}
	customer, customerErr := s.customerRepository.FindById(ctx, purchase.CustomerID)
	if customerErr != nil {
		slog.Warn("payment: P2P rejected customer lookup failed", "purchase_id", utils.MaskHalfInt64(purchaseID), "error", customerErr)
	}
	if customer == nil {
		return nil
	}
	if s.telegramBot == nil {
		slog.Warn("payment: cannot send P2P rejection notification without main bot", "purchase_id", utils.MaskHalfInt64(purchaseID))
		return nil
	}
	text := buildP2PRejectedUserMessage(purchase)
	params := &bot.SendMessageParams{ChatID: customer.TelegramID, ParseMode: models.ParseModeHTML, Text: text}
	if supportURL := strings.TrimSpace(config.SupportURL()); supportURL != "" {
		params.ReplyMarkup = models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{
			Text: "Перейти в поддержку", URL: supportURL,
		}}}}
	} else if botURL := strings.TrimSpace(config.BotURL()); botURL != "" {
		params.ReplyMarkup = models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{
			Text: "Перейти в поддержку", URL: botURL,
		}}}}
	}
	if _, err := s.telegramBot.SendMessage(ctx, params); err != nil {
		slog.Error("payment: failed to send P2P rejection notification to customer", "purchase_id", utils.MaskHalfInt64(purchaseID), "error", err)
	}
	return nil
}

func buildP2PRejectedUserMessage(purchase *database.Purchase) string {
	if purchase == nil {
		return "❌ <b>Платёж отклонён администратором</b>"
	}
	return fmt.Sprintf(
		"❌ <b>P2P-платёж отклонён администратором</b>\n\n💳 <b>Сумма:</b> <b>%s</b>\n▣ <b>Заказ:</b> <code>%d</code>\n\nЕсли вы считаете это ошибкой, обратитесь в поддержку.",
		html.EscapeString(formatPurchaseAmount(purchase)),
		purchase.ID,
	)
}
