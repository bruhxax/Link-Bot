package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
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
	URL               string `json:"url,omitempty"`
	CallbackData      string `json:"callback_data,omitempty"`
	IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
	Style             string `json:"style,omitempty"`
}

type telegramAPIResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code,omitempty"`
	Description string `json:"description,omitempty"`
	Parameters  struct {
		RetryAfter int `json:"retry_after,omitempty"`
	} `json:"parameters,omitempty"`
	Result struct {
		MessageID int64 `json:"message_id,omitempty"`
	} `json:"result,omitempty"`
}

type telegramAPIError struct {
	StatusCode  int
	ErrorCode   int
	Description string
	RetryAfter  time.Duration
}

func (e *telegramAPIError) Error() string {
	description := strings.TrimSpace(e.Description)
	if description == "" {
		description = "unknown Telegram API error"
	}
	return fmt.Sprintf("telegram notification bot returned status %d, code %d: %s", e.StatusCode, e.ErrorCode, description)
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
	profileUsername := ""
	if customer != nil && customer.TelegramUsername != nil {
		profileUsername = strings.TrimSpace(*customer.TelegramUsername)
		if username == "" {
			username = profileUsername
		}
	}
	method := s.paymentMethodLabel(purchase)

	notifyCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
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
	keyboard := buildPaymentNotificationKeyboard(settings, panelUserID, profileUsername)

	if err := sendTelegramNotification(notifyCtx, token, chatID, message, keyboard); err != nil {
		if isTelegramButtonUserInvalid(err) && paymentNotificationProfileURL(profileUsername) != "" {
			fallbackSettings := settings
			fallbackSettings.ProfileButton.Enabled = false
			fallbackKeyboard := buildPaymentNotificationKeyboard(fallbackSettings, panelUserID, "")
			slog.Warn("payment notification profile button rejected; retrying without it", "error", err, "purchase_id", purchase.ID)
			if fallbackErr := sendTelegramNotification(notifyCtx, token, chatID, message, fallbackKeyboard); fallbackErr == nil {
				return
			} else {
				err = fmt.Errorf("profile button rejected (%v); fallback delivery failed: %w", err, fallbackErr)
			}
		}
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
	case database.InvoiceTypeP2P:
		return "P2P перевод"
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

func buildPaymentNotificationKeyboard(settings runtimeconfig.TelegramPaymentNotificationSettings, panelUserID int64, profileUsername string) *telegramInlineKeyboardMarkup {
	rows := make([][]telegramInlineKeyboardButton, 0, 2)
	if settings.OpenUserButton.Enabled {
		if panelURL := paymentNotificationPanelURL(panelUserID); panelURL != "" {
			rows = append(rows, []telegramInlineKeyboardButton{paymentNotificationButton(settings.OpenUserButton.TelegramButtonSettings, panelURL)})
		}
	}
	if profileURL := paymentNotificationProfileURL(profileUsername); settings.ProfileButton.Enabled && profileURL != "" {
		rows = append(rows, []telegramInlineKeyboardButton{paymentNotificationButton(
			settings.ProfileButton.TelegramButtonSettings,
			profileURL,
		)})
	}
	if len(rows) == 0 {
		return nil
	}
	return &telegramInlineKeyboardMarkup{InlineKeyboard: rows}
}

func paymentNotificationProfileURL(username string) string {
	username = strings.TrimSpace(username)
	for _, prefix := range []string{"https://t.me/", "http://t.me/", "t.me/", "@"} {
		if strings.HasPrefix(strings.ToLower(username), prefix) {
			username = username[len(prefix):]
			break
		}
	}
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 64 {
		return ""
	}
	for _, character := range username {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' {
			continue
		}
		return ""
	}
	return "https://t.me/" + username
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
	keyboard := buildPaymentNotificationKeyboard(options.Settings, panelUserID, username)
	return sendTelegramNotification(previewCtx, token, chatID, message, keyboard)
}

func sendTelegramNotification(ctx context.Context, token string, chatID int64, text string, keyboard ...*telegramInlineKeyboardMarkup) error {
	_, err := sendTelegramNotificationMessage(ctx, token, chatID, text, keyboard...)
	return err
}

func sendTelegramNotificationMessage(ctx context.Context, token string, chatID int64, text string, keyboard ...*telegramInlineKeyboardMarkup) (int64, error) {
	request := telegramSendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	}
	if len(keyboard) > 0 {
		request.ReplyMarkup = keyboard[0]
	}
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	return sendTelegramNotificationMessageRequest(ctx, http.DefaultClient, endpoint, request)
}

func sendTelegramNotificationRequest(ctx context.Context, client *http.Client, endpoint string, request telegramSendMessageRequest) error {
	_, err := sendTelegramNotificationMessageRequest(ctx, client, endpoint, request)
	return err
}

func sendTelegramNotificationMessageRequest(ctx context.Context, client *http.Client, endpoint string, request telegramSendMessageRequest) (int64, error) {
	if client == nil {
		client = http.DefaultClient
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return 0, fmt.Errorf("marshal notification payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		messageID, requestErr := performTelegramNotificationMessageRequest(ctx, client, endpoint, payload)
		lastErr = requestErr
		if lastErr == nil || !isRetryableTelegramNotificationError(lastErr) || attempt == 2 {
			return messageID, lastErr
		}
		delay := telegramNotificationRetryDelay(attempt, lastErr)
		slog.Warn("retrying Telegram payment notification", "attempt", attempt+2, "delay", delay, "error", lastErr)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
	return 0, lastErr
}

func performTelegramNotificationRequest(ctx context.Context, client *http.Client, endpoint string, payload []byte) error {
	_, err := performTelegramNotificationMessageRequest(ctx, client, endpoint, payload)
	return err
}

func performTelegramNotificationMessageRequest(ctx context.Context, client *http.Client, endpoint string, payload []byte) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("create notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send notification request: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	if readErr != nil {
		return 0, fmt.Errorf("read notification response: %w", readErr)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		apiResponse := telegramAPIResponse{}
		_ = json.Unmarshal(body, &apiResponse)
		description := strings.TrimSpace(apiResponse.Description)
		if description == "" {
			description = strings.TrimSpace(string(body))
		}
		return 0, &telegramAPIError{
			StatusCode:  resp.StatusCode,
			ErrorCode:   apiResponse.ErrorCode,
			Description: description,
			RetryAfter:  time.Duration(apiResponse.Parameters.RetryAfter) * time.Second,
		}
	}
	apiResponse := telegramAPIResponse{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &apiResponse); err != nil {
			return 0, fmt.Errorf("decode notification response: %w", err)
		}
		if !apiResponse.OK {
			return 0, &telegramAPIError{StatusCode: resp.StatusCode, ErrorCode: apiResponse.ErrorCode, Description: apiResponse.Description}
		}
	}
	return apiResponse.Result.MessageID, nil
}

func isTelegramButtonUserInvalid(err error) bool {
	var apiErr *telegramAPIError
	return errors.As(err, &apiErr) && strings.Contains(strings.ToUpper(apiErr.Description), "BUTTON_USER_INVALID")
}

func isRetryableTelegramNotificationError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *telegramAPIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= http.StatusInternalServerError
	}
	return true
}

func telegramNotificationRetryDelay(attempt int, err error) time.Duration {
	var apiErr *telegramAPIError
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		if apiErr.RetryAfter > 3*time.Second {
			return 3 * time.Second
		}
		return apiErr.RetryAfter
	}
	if attempt <= 0 {
		return 250 * time.Millisecond
	}
	return 750 * time.Millisecond
}
