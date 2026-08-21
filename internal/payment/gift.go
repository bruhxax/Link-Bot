package payment

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"link-bot/internal/broadcast"
	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
	"link-bot/utils"
)

type GiftReceipt struct {
	PurchaseID        int64
	RecipientUsername string
	PlanLabel         string
	ShareURL          string
	Delivered         bool
	Message           string
}

func (s PaymentService) processGiftPurchase(ctx context.Context, purchase *database.Purchase, sender *database.Customer) error {
	if purchase == nil || sender == nil || purchase.GiftToken == nil || purchase.GiftRecipientUsername == nil {
		return errors.New("gift purchase metadata is incomplete")
	}

	recipient, err := s.giftRecipient(ctx, purchase)
	if err != nil {
		return err
	}
	if recipient != nil && recipient.ID == sender.ID {
		return errors.New("a subscription gift cannot be delivered to its sender")
	}

	// A gift promo belongs to the purchaser, not the recipient. Complete the
	// redemption through the same idempotent path as a regular subscription.
	s.completePromoRedemption(ctx, purchase, sender)
	if recipient == nil {
		if err := s.purchaseRepository.MarkAsPaid(ctx, purchase.ID); err != nil {
			return err
		}
		purchase.Status = database.PurchaseStatusPaid
		s.notifyAdminAboutPayment(ctx, purchase, sender)
		s.sendGiftNotification(ctx, sender, purchase, true)
		return nil
	}
	if err := s.deliverGiftEntitlements(ctx, purchase, recipient); err != nil {
		return err
	}
	if err := s.purchaseRepository.MarkGiftPaidAndDelivered(ctx, purchase.ID, recipient.ID); err != nil {
		return err
	}
	purchase.Status = database.PurchaseStatusPaid
	purchase.GiftRecipientCustomerID = &recipient.ID
	s.notifyAdminAboutPayment(ctx, purchase, sender)
	s.sendGiftNotification(ctx, sender, purchase, true)
	s.sendGiftNotification(ctx, recipient, purchase, false)
	return nil
}

func (s PaymentService) giftRecipient(ctx context.Context, purchase *database.Purchase) (*database.Customer, error) {
	if purchase.GiftRecipientCustomerID != nil && *purchase.GiftRecipientCustomerID > 0 {
		return s.customerRepository.FindById(ctx, *purchase.GiftRecipientCustomerID)
	}
	if purchase.GiftRecipientUsername == nil {
		return nil, nil
	}
	return s.customerRepository.FindByTelegramUsername(ctx, *purchase.GiftRecipientUsername)
}

func (s PaymentService) deliverGiftEntitlements(ctx context.Context, purchase *database.Purchase, recipient *database.Customer) error {
	if s.subscriptionRepository == nil {
		return errors.New("subscription repository is not configured")
	}
	subscription, err := s.subscriptionRepository.EnsurePrimary(ctx, recipient)
	if err != nil {
		return err
	}
	trafficLimit := purchaseTrafficLimit(purchase)
	deviceLimit := purchaseDeviceLimit(purchase)
	panelState, stateErr := s.panelStateForSubscription(ctx, recipient, subscription)
	if stateErr != nil {
		slog.Warn("payment: load gift recipient panel state failed", "error", stateErr, "customerId", utils.MaskHalfInt64(recipient.ID))
	} else if shouldAccumulateEntitlements(customerForSubscription(recipient, subscription), panelState) {
		trafficLimit = mergeTrafficLimits(int(maxInt64(panelState.TrafficLimitBytes, 0)), trafficLimit)
		deviceLimit = mergeDeviceLimits(maxInt(panelState.DeviceLimit, 0), deviceLimit)
	}

	provisioning := remnawave.ProvisioningOptions{}
	if s.runtimeSettings != nil {
		provisioning.UsernameTemplate = s.runtimeSettings.Snapshot().Panel.UsernameTemplate
		if purchase.PlanID != nil {
			if plan, ok := s.runtimeSettings.CheckoutPlan(*purchase.PlanID, purchase.Month); ok {
				provisioning.InternalSquadUUIDs = append([]string(nil), plan.InternalSquadUUIDs...)
				provisioning.InternalSquadsConfigured = plan.InternalSquadsConfigured
				provisioning.ExternalSquadUUID = plan.ExternalSquadUUID
				provisioning.TrafficResetStrategy = config.TrafficLimitResetStrategy()
				provisioning.Tag = config.RemnawaveTag()
				provisioning.ApplySquads = true
			}
		}
	}
	userID, userUUID := subscriptionPanelIdentity(subscription)
	user, err := s.remnawaveClient.CreateOrUpdateUserForSubscription(
		ctx,
		recipient.ID,
		recipient.TelegramID,
		subscription.ID,
		userID,
		userUUID,
		subscription.IsPrimary,
		trafficLimit,
		deviceLimit,
		purchase.Month*config.DaysInMonth(),
		provisioning,
	)
	if err != nil {
		return err
	}
	return s.persistSubscriptionPanelState(ctx, recipient, subscription, user)
}

// ClaimPendingGifts delivers paid gifts addressed to the authenticated
// Telegram username or to the one-time deep-link token from /start.
func (s PaymentService) ClaimPendingGifts(ctx context.Context, recipient *database.Customer, username, rawToken string) (int, error) {
	if recipient == nil {
		return 0, errors.New("gift recipient is required")
	}
	token := uuid.Nil
	rawToken = strings.TrimSpace(strings.TrimPrefix(rawToken, "gift_"))
	if rawToken != "" {
		parsed, err := uuid.Parse(rawToken)
		if err == nil {
			token = parsed
		}
	}
	items, err := s.purchaseRepository.ListClaimableGifts(ctx, recipient.ID, username, token)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for index := range items {
		purchaseID := items[index].ID
		release, lockErr := s.purchaseRepository.LockForProcessing(ctx, purchaseID)
		if lockErr != nil {
			return delivered, lockErr
		}
		purchase, findErr := s.purchaseRepository.FindById(ctx, purchaseID)
		if findErr == nil && purchase != nil && purchase.Status == database.PurchaseStatusPaid && purchase.GiftDeliveredAt == nil {
			assigned, assignErr := s.purchaseRepository.AssignGiftRecipient(ctx, purchase.ID, recipient.ID)
			if assignErr != nil {
				findErr = assignErr
			} else if assigned {
				if deliverErr := s.deliverGiftEntitlements(ctx, purchase, recipient); deliverErr != nil {
					findErr = deliverErr
				} else if markErr := s.purchaseRepository.MarkGiftPaidAndDelivered(ctx, purchase.ID, recipient.ID); markErr != nil {
					findErr = markErr
				} else {
					purchase.GiftRecipientCustomerID = &recipient.ID
					s.sendGiftNotification(ctx, recipient, purchase, false)
					delivered++
				}
			}
		}
		if releaseErr := release(); releaseErr != nil && findErr == nil {
			findErr = releaseErr
		}
		if findErr != nil {
			return delivered, findErr
		}
	}
	return delivered, nil
}

func (s PaymentService) LatestGiftReceipt(ctx context.Context, sender *database.Customer) (*GiftReceipt, error) {
	if sender == nil {
		return nil, nil
	}
	purchase, err := s.purchaseRepository.FindLatestUnseenGiftBySender(ctx, sender.ID)
	if err != nil || purchase == nil {
		return nil, err
	}
	settings := s.giftSettings().Sender
	replacements := s.giftReplacements(purchase, sender, nil)
	return &GiftReceipt{
		PurchaseID:        purchase.ID,
		RecipientUsername: formatGiftUsername(pointerValue(purchase.GiftRecipientUsername)),
		PlanLabel:         s.giftPlanLabel(purchase),
		ShareURL:          GiftShareURL(purchase),
		Delivered:         purchase.GiftDeliveredAt != nil,
		Message:           renderGiftTemplate(settings.Text, replacements),
	}, nil
}

func (s PaymentService) MarkGiftReceiptSeen(ctx context.Context, purchaseID int64, sender *database.Customer) error {
	if sender == nil {
		return errors.New("gift sender is required")
	}
	return s.purchaseRepository.MarkGiftSenderSeen(ctx, purchaseID, sender.ID)
}

func (s PaymentService) SendGiftPreview(ctx context.Context, chatID int64, kind string, message runtimeconfig.TelegramGiftMessageSettings) error {
	defaults := runtimeconfig.DefaultSettings().Content.Gift
	fallback := defaults.Recipient
	if strings.EqualFold(strings.TrimSpace(kind), "sender") {
		fallback = defaults.Sender
		kind = "sender"
	} else {
		kind = "recipient"
	}
	if err := runtimeconfig.NormalizeGiftMessageSettings(&message, fallback); err != nil {
		return err
	}
	replacements := map[string]string{
		"{sender}":    "@sender_example",
		"{recipient}": "@recipient_example",
		"{plan}":      "3 месяца",
		"{gift_url}":  "https://t.me/example_bot?start=gift_example",
		"{gift_link}": "https://t.me/example_bot?start=gift_example",
	}
	buttons := prepareGiftButtons(message.Buttons, replacements["{gift_url}"])
	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID, ParseMode: models.ParseModeHTML,
		Text: renderGiftTemplate(message.Text, replacements), ReplyMarkup: broadcast.BuildKeyboard(buttons),
	})
	return err
}

func (s PaymentService) sendGiftNotification(ctx context.Context, customer *database.Customer, purchase *database.Purchase, senderMessage bool) {
	settings := s.giftSettings().Recipient
	var recipient *database.Customer
	if senderMessage {
		settings = s.giftSettings().Sender
	} else {
		recipient = customer
	}
	sender, _ := s.customerRepository.FindById(ctx, purchase.CustomerID)
	text := renderGiftTemplate(settings.Text, s.giftReplacements(purchase, sender, recipient))
	buttons := prepareGiftButtons(settings.Buttons, GiftShareURL(purchase))
	if _, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: customer.TelegramID, ParseMode: models.ParseModeHTML,
		Text: text, ReplyMarkup: broadcast.BuildKeyboard(buttons),
	}); err != nil {
		slog.Error("payment: gift notification failed", "error", err, "purchase_id", utils.MaskHalfInt64(purchase.ID), "telegram_id", utils.MaskHalfInt64(customer.TelegramID))
	}
}

func (s PaymentService) giftSettings() runtimeconfig.TelegramGiftSettings {
	settings := runtimeconfig.DefaultSettings().Content.Gift
	if s.runtimeSettings != nil {
		settings = s.runtimeSettings.Snapshot().Content.Gift
	}
	return settings
}

func (s PaymentService) giftReplacements(purchase *database.Purchase, sender, recipient *database.Customer) map[string]string {
	senderName := "пользователь"
	if sender != nil && sender.TelegramUsername != nil {
		senderName = formatGiftUsername(*sender.TelegramUsername)
	}
	recipientName := formatGiftUsername(pointerValue(purchase.GiftRecipientUsername))
	if recipient != nil && recipient.TelegramUsername != nil {
		recipientName = formatGiftUsername(*recipient.TelegramUsername)
	}
	return map[string]string{
		"{sender}":    senderName,
		"{recipient}": recipientName,
		"{plan}":      s.giftPlanLabel(purchase),
		"{gift_url}":  GiftShareURL(purchase),
		"{gift_link}": GiftShareURL(purchase),
	}
}

func (s PaymentService) giftPlanLabel(purchase *database.Purchase) string {
	if purchase != nil && purchase.PlanID != nil && s.runtimeSettings != nil {
		if title := strings.TrimSpace(s.runtimeSettings.PlanTitle(*purchase.PlanID, s.language())); title != "" {
			return title
		}
	}
	months := 0
	if purchase != nil {
		months = purchase.Month
	}
	return fmt.Sprintf("%d %s", months, russianMonthWord(months))
}

func russianMonthWord(months int) string {
	lastTwo := months % 100
	last := months % 10
	if lastTwo >= 11 && lastTwo <= 14 {
		return "месяцев"
	}
	if last == 1 {
		return "месяц"
	}
	if last >= 2 && last <= 4 {
		return "месяца"
	}
	return "месяцев"
}

func renderGiftTemplate(template string, replacements map[string]string) string {
	result := strings.TrimSpace(template)
	for key, value := range replacements {
		result = strings.ReplaceAll(result, key, html.EscapeString(strings.TrimSpace(value)))
	}
	return result
}

func prepareGiftButtons(buttons []database.BroadcastButton, giftURL string) []database.BroadcastButton {
	result := append([]database.BroadcastButton(nil), buttons...)
	for index := range result {
		if result[index].Type == "gift" {
			result[index].Type = "url"
			result[index].URL = giftURL
		}
	}
	return result
}

func GiftShareURL(purchase *database.Purchase) string {
	if purchase == nil || purchase.GiftToken == nil {
		return ""
	}
	base := strings.TrimSpace(config.BotURL())
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return base
	}
	query := parsed.Query()
	query.Set("start", "gift_"+purchase.GiftToken.String())
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func formatGiftUsername(value string) string {
	value = database.NormalizeTelegramUsername(value)
	if value == "" {
		return "@username"
	}
	return "@" + value
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
