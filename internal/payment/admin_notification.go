package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/integrations"
	"link-bot/internal/runtimeconfig"
)

type telegramSendMessageRequest struct {
	ChatID      int64                         `json:"chat_id"`
	Text        string                        `json:"text"`
	ParseMode   string                        `json:"parse_mode,omitempty"`
	ReplyMarkup *telegramInlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

type telegramInlineKeyboardMarkup struct {
	InlineKeyboard [][]telegramInlineKeyboardButton `json:"inline_keyboard"`
}

type telegramInlineKeyboardButton struct {
	Text              string `json:"text"`
	URL               string `json:"url"`
	IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
	Style             string `json:"style,omitempty"`
}

type PaymentNotificationPreviewOptions struct {
	Settings   runtimeconfig.TelegramPaymentNotificationSettings
	TelegramID int64
	Username   string
	Customer   *database.Customer
}

func (s PaymentService) notifyAdminAboutPayment(ctx context.Context, purchase *database.Purchase, customer *database.Customer) {
	token, chatID, timezone := s.paymentNotificationConfig()
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
	settings := s.paymentNotificationSettings()
	message := buildPaymentNotificationMessageWithTemplate(settings.Text, purchase, username, method, time.Now(), orderNumber, timezone)
	panelUserID := s.paymentNotificationPanelUserID(notifyCtx, purchase, customer)
	telegramID := int64(0)
	if customer != nil {
		telegramID = customer.TelegramID
	}
	keyboard := buildPaymentNotificationKeyboard(settings, panelUserID, telegramID)

	if err := sendTelegramNotification(notifyCtx, token, chatID, message, keyboard); err != nil {
		slog.Error("failed to send payment notification", "error", err, "purchase_id", purchase.ID)
	}
}

func (s PaymentService) paymentNotificationSettings() runtimeconfig.TelegramPaymentNotificationSettings {
	settings := runtimeconfig.DefaultSettings().Content.PaymentNotification
	if s.runtimeSettings != nil {
		settings = s.runtimeSettings.Snapshot().Content.PaymentNotification
	}
	return settings
}

func (s PaymentService) paymentNotificationPanelUserID(ctx context.Context, purchase *database.Purchase, customer *database.Customer) int64 {
	if s.subscriptionRepository == nil || customer == nil {
		return 0
	}
	var (
		subscription *database.CustomerSubscription
		err          error
	)
	if purchase != nil && purchase.SubscriptionID != nil {
		subscription, err = s.subscriptionRepository.FindForCustomer(ctx, customer.ID, *purchase.SubscriptionID)
	} else {
		subscription, err = s.subscriptionRepository.PrimaryByCustomer(ctx, customer.ID)
	}
	if err != nil {
		slog.Warn("failed to resolve payment notification panel user", "error", err, "customer_id", customer.ID)
		return 0
	}
	if subscription == nil || subscription.PanelUserID == nil {
		return 0
	}
	return *subscription.PanelUserID
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
	_ = customer
	defaults := runtimeconfig.DefaultSettings().Content.PaymentNotification
	return buildPaymentNotificationMessageWithTemplate(
		defaults.Text,
		purchase,
		username,
		method,
		paidAt,
		orderNumber,
		config.PaymentNotificationTimezone(),
	)
}

func buildPaymentNotificationMessageWithTemplate(
	template string,
	purchase *database.Purchase,
	username string,
	method string,
	paidAt time.Time,
	orderNumber int64,
	timezone string,
) string {
	usernameText := "-"
	if username != "" {
		usernameText = username
		if !strings.HasPrefix(usernameText, "@") {
			usernameText = "@" + usernameText
		}
	}

	promoText := ""
	if purchase.PromoCodeSnapshot != nil {
		promoCode := strings.TrimSpace(*purchase.PromoCodeSnapshot)
		if promoCode != "" {
			discount := ""
			if purchase.PromoCodeDiscountPercent != nil && *purchase.PromoCodeDiscountPercent > 0 {
				discount = fmt.Sprintf(" (-%d%%)", *purchase.PromoCodeDiscountPercent)
			}
			promoText = promoCode + discount
		}
	}

	subscriptionText := ""
	if purchase.PurchaseKind != database.PurchaseKindExtraDevices && purchase.Month > 0 {
		subscriptionText = formatTariff(purchase.Month)
	}
	deviceText := ""
	if purchase.ExtraDevices > 0 {
		deviceText = fmt.Sprintf("+%d", purchase.ExtraDevices)
	}

	values := map[string]string{
		"data":        formatNotificationTimeInLocation(paidAt, timezone),
		"integration": strings.TrimSpace(method),
		"promo":       promoText,
		"sub":         subscriptionText,
		"username":    usernameText,
		"number":      strconv.FormatInt(orderNumber, 10),
		"price":       formatPurchaseAmount(purchase),
		"device":      deviceText,
	}
	template = strings.ReplaceAll(template, "\r\n", "\n")
	lines := strings.Split(template, "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		if (values["promo"] == "" && strings.Contains(line, "{{promo}}")) ||
			(values["device"] == "" && strings.Contains(line, "{{device}}")) ||
			(values["sub"] == "" && strings.Contains(line, "{{sub}}")) {
			continue
		}
		for name, value := range values {
			line = strings.ReplaceAll(line, "{{"+name+"}}", html.EscapeString(value))
		}
		clean = append(clean, line)
	}
	return strings.TrimSpace(strings.Join(clean, "\n"))
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

func formatNotificationTimeInLocation(t time.Time, timezone string) string {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = "Europe/Moscow"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		slog.Warn("invalid payment notification timezone, using local timezone", "timezone", timezone, "error", err)
		location = time.Local
	}

	return t.In(location).Format("15:04 | 02.01.06")
}

func (s PaymentService) paymentMethodLabel(purchase *database.Purchase) string {
	switch purchase.InvoiceType {
	case database.InvoiceTypeFree:
		return "Бесплатная активация"
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

func buildPaymentNotificationKeyboard(settings runtimeconfig.TelegramPaymentNotificationSettings, panelUserID, telegramID int64) *telegramInlineKeyboardMarkup {
	rows := make([][]telegramInlineKeyboardButton, 0, 2)
	if settings.OpenUserButton.Enabled {
		if panelURL := paymentNotificationPanelURL(panelUserID); panelURL != "" {
			rows = append(rows, []telegramInlineKeyboardButton{paymentNotificationButton(settings.OpenUserButton.TelegramButtonSettings, panelURL)})
		}
	}
	if settings.ProfileButton.Enabled && telegramID > 0 {
		rows = append(rows, []telegramInlineKeyboardButton{paymentNotificationButton(
			settings.ProfileButton.TelegramButtonSettings,
			fmt.Sprintf("tg://user?id=%d", telegramID),
		)})
	}
	if len(rows) == 0 {
		return nil
	}
	return &telegramInlineKeyboardMarkup{InlineKeyboard: rows}
}

func paymentNotificationButton(settings runtimeconfig.TelegramButtonSettings, targetURL string) telegramInlineKeyboardButton {
	return telegramInlineKeyboardButton{
		Text:              settings.Text,
		URL:               targetURL,
		IconCustomEmojiID: settings.IconCustomEmojiID,
		Style:             settings.Style,
	}
}

func paymentNotificationPanelURL(panelUserID int64) string {
	panelURL, err := url.Parse(strings.TrimSpace(config.RemnawaveUrl()))
	if err != nil || panelURL.Scheme == "" || panelURL.Host == "" {
		return ""
	}
	if panelURL.Scheme != "https" && panelURL.Scheme != "http" {
		return ""
	}
	if panelUserID > 0 {
		panelURL.Path = fmt.Sprintf("/dashboard/open/user/%d", panelUserID)
	} else {
		panelURL.Path = "/dashboard/management/users"
	}
	panelURL.RawQuery = ""
	panelURL.Fragment = ""
	return panelURL.String()
}

func (s PaymentService) SendPaymentNotificationPreview(ctx context.Context, options PaymentNotificationPreviewOptions) error {
	token, chatID, timezone := s.paymentNotificationConfig()
	if token == "" || chatID == 0 {
		return fmt.Errorf("бот уведомлений об оплате не настроен")
	}
	purchase := &database.Purchase{
		Amount:       239,
		Currency:     "RUB",
		Month:        3,
		PurchaseKind: database.PurchaseKindSubscription,
		ExtraDevices: 2,
	}
	promo := "PROMO20"
	discount := 20
	purchase.PromoCodeSnapshot = &promo
	purchase.PromoCodeDiscountPercent = &discount
	username := strings.TrimSpace(options.Username)
	if username == "" {
		username = "test_user"
	}
	previewCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	message := buildPaymentNotificationMessageWithTemplate(
		options.Settings.Text,
		purchase,
		username,
		"YooKassa",
		time.Now(),
		532,
		timezone,
	)
	panelUserID := s.paymentNotificationPanelUserID(previewCtx, nil, options.Customer)
	keyboard := buildPaymentNotificationKeyboard(options.Settings, panelUserID, options.TelegramID)
	return sendTelegramNotification(previewCtx, token, chatID, message, keyboard)
}

func sendTelegramNotification(ctx context.Context, token string, chatID int64, text string, keyboard ...*telegramInlineKeyboardMarkup) error {
	request := telegramSendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	}
	if len(keyboard) > 0 {
		request.ReplyMarkup = keyboard[0]
	}
	payload, err := json.Marshal(request)
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
