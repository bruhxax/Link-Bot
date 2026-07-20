package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/integrations"
)

type telegramSendMessageRequest struct {
	ChatID    int64  `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

func (s PaymentService) notifyAdminAboutPayment(ctx context.Context, purchase *database.Purchase, customer *database.Customer) {
	token, chatID, _ := s.paymentNotificationConfig()
	if token == "" || chatID == 0 {
		return
	}

	username := usernameFromContext(ctx)
	method := s.paymentMethodLabel(purchase)

	notifyCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	orderNumber := purchase.ID
	if s.purchaseRepository != nil {
		assignedNumber, err := s.purchaseRepository.GetOrAssignPaymentOrderNumber(notifyCtx, purchase.ID)
		if err != nil {
			slog.Error("failed to assign payment notification order number", "error", err, "purchase_id", purchase.ID)
		} else {
			orderNumber = assignedNumber
		}
	}
	message := buildPaymentNotificationMessage(purchase, customer, username, method, time.Now(), orderNumber)

	if err := sendTelegramNotification(notifyCtx, token, chatID, message); err != nil {
		slog.Error("failed to send payment notification", "error", err, "purchase_id", purchase.ID)
	}
}

func usernameFromContext(ctx context.Context) string {
	if value, ok := ctx.Value("username").(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func buildPaymentNotificationMessage(
	purchase *database.Purchase,
	customer *database.Customer,
	username string,
	method string,
	paidAt time.Time,
	orderNumber int64,
) string {
	usernameText := "-"
	if username != "" {
		usernameText = username
		if !strings.HasPrefix(usernameText, "@") {
			usernameText = "@" + usernameText
		}
	}

	promoLine := ""
	if purchase.PromoCodeSnapshot != nil {
		promoCode := strings.TrimSpace(*purchase.PromoCodeSnapshot)
		if promoCode != "" {
			discount := ""
			if purchase.PromoCodeDiscountPercent != nil && *purchase.PromoCodeDiscountPercent > 0 {
				discount = fmt.Sprintf(" (-%d%%)", *purchase.PromoCodeDiscountPercent)
			}
			promoLine = fmt.Sprintf("\n🏷 <b>Промокод:</b> <b>%s%s</b>", html.EscapeString(promoCode), discount)
		}
	}

	return fmt.Sprintf(
		"%s <b>Оплата:</b> <b>%s</b>\n\n"+
			"%s <b>Тариф:</b> <b>%s</b>\n"+
			"%s <b>Telegram:</b> <b>%s</b>\n"+
			"%s <b>Время:</b> <b>%s</b>\n"+
			"%s <b>Способ:</b> <b>%s</b>%s\n"+
			"%s <b>Заказ:</b> <code>%d</code>",
		premiumEmoji("5258204546391351475"),
		html.EscapeString(formatPurchaseAmount(purchase)),
		premiumEmoji("5226513232549664618"),
		html.EscapeString(formatTariff(purchase.Month)),
		premiumEmoji("5258073068852485953"),
		html.EscapeString(usernameText),
		premiumEmoji("5258419835922030550"),
		html.EscapeString(formatNotificationTime(paidAt)),
		premiumEmoji("5258096772776991776"),
		html.EscapeString(method),
		promoLine,
		premiumEmoji("5258389041006518073"),
		orderNumber,
	)
}

func premiumEmoji(id string) string {
	return fmt.Sprintf(`<tg-emoji emoji-id="%s">☺️</tg-emoji>`, id)
}

func formatTariff(months int) string {
	word := "месяцев"
	lastTwo := months % 100
	last := months % 10

	if lastTwo < 11 || lastTwo > 14 {
		switch last {
		case 1:
			word = "месяц"
		case 2, 3, 4:
			word = "месяца"
		}
	}

	return fmt.Sprintf("%d %s", months, word)
}

func formatPurchaseAmount(purchase *database.Purchase) string {
	if purchase.Amount == float64(int64(purchase.Amount)) {
		return fmt.Sprintf("%.0f %s", purchase.Amount, purchase.Currency)
	}
	return fmt.Sprintf("%.2f %s", purchase.Amount, purchase.Currency)
}

func formatNotificationTime(t time.Time) string {
	location, err := time.LoadLocation(config.PaymentNotificationTimezone())
	if err != nil {
		slog.Warn("invalid payment notification timezone, using local timezone", "timezone", config.PaymentNotificationTimezone(), "error", err)
		location = time.Local
	}

	return t.In(location).Format("15:04 | 02.01.06")
}

func (s PaymentService) paymentMethodLabel(purchase *database.Purchase) string {
	switch purchase.InvoiceType {
	case database.InvoiceTypeYookasa:
		return "YooKassa"
	case database.InvoiceTypeCrypto:
		return "Crypto Pay"
	case database.InvoiceTypeTelegram:
		return "Telegram Stars"
	case database.InvoiceTypeTribute:
		return "Tribute"
	default:
		return string(purchase.InvoiceType)
	}
}

func (s PaymentService) paymentNotificationConfig() (string, int64, string) {
	if s.integrationSettings != nil {
		if cfg, ok := s.integrationSettings.Config(integrations.ProviderNotificationBot); ok {
			chatID, _ := strconv.ParseInt(strings.TrimSpace(cfg["chatId"]), 10, 64)
			if chatID == 0 {
				chatID = config.GetAdminTelegramId()
			}
			timezone := strings.TrimSpace(cfg["timezone"])
			if timezone == "" {
				timezone = "Europe/Moscow"
			}
			return strings.TrimSpace(cfg["token"]), chatID, timezone
		}
	}
	if config.IsPaymentNotificationEnabled() {
		return config.PaymentNotificationBotToken(), config.PaymentNotificationChatID(), config.PaymentNotificationTimezone()
	}
	return "", 0, "Europe/Moscow"
}

func sendTelegramNotification(ctx context.Context, token string, chatID int64, text string) error {
	payload, err := json.Marshal(telegramSendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	})
	if err != nil {
		return fmt.Errorf("marshal notification payload: %w", err)
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send notification request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("telegram notification bot returned status %d", resp.StatusCode)
	}

	return nil
}
