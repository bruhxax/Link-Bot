package notification

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/handler"
	"link-bot/internal/runtimeconfig"
	"link-bot/internal/translation"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type customerRepository interface {
	FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]database.Customer, error)
	ClaimSubscriptionNotification(ctx context.Context, customerID int64, expireAt time.Time, kind string) (bool, error)
	ReleaseSubscriptionNotification(ctx context.Context, customerID int64, expireAt time.Time, kind string) error
}

type tributeRepository interface {
	FindLatestActiveTributesByCustomerIDs(ctx context.Context, customerIDs []int64) (*[]database.Purchase, error)
}

type paymentProcessor interface {
	CreatePurchase(ctx context.Context, amount float64, months int, customer *database.Customer, invoiceType database.InvoiceType) (string, int64, error)
	ProcessPurchaseById(ctx context.Context, purchaseId int64) error
}

type SubscriptionService struct {
	customerRepository customerRepository
	purchaseRepository tributeRepository
	paymentService     paymentProcessor
	telegramBot        *bot.Bot
	tm                 *translation.Manager
	runtimeSettings    *runtimeconfig.Service
	notify             func(context.Context, database.Customer) error
	processMu          sync.Mutex
}

type TestReminderOptions struct {
	Kind              string
	Template          string
	ButtonText        string
	IconCustomEmojiID string
	ButtonStyle       string
}

type reminderButtonOptions struct {
	Text              string
	IconCustomEmojiID string
	Style             string
}

func NewSubscriptionService(customerRepository customerRepository,
	purchaseRepository tributeRepository,
	paymentService paymentProcessor,
	telegramBot *bot.Bot,
	tm *translation.Manager,
	runtimeSettings *runtimeconfig.Service) *SubscriptionService {
	svc := &SubscriptionService{customerRepository: customerRepository, purchaseRepository: purchaseRepository, paymentService: paymentService, telegramBot: telegramBot, tm: tm, runtimeSettings: runtimeSettings}
	svc.notify = svc.sendNotification
	return svc
}
func (s *SubscriptionService) ProcessSubscriptionExpiration() error {
	if !s.processMu.TryLock() {
		slog.Info("Subscription notification check is already running")
		return nil
	}
	defer s.processMu.Unlock()

	ctx := context.Background()
	customers, err := s.getCustomersWithExpiringSubscriptions()
	if err != nil {
		slog.Error("Failed to get customers with expiring subscriptions", "error", err)
		return err
	}

	expiredCustomers, err := s.getCustomersWithRecentlyExpiredSubscriptions()
	if err != nil {
		slog.Error("Failed to get customers with expired subscriptions", "error", err)
		return err
	}

	slog.Info(fmt.Sprintf("Found %d customers with expiring subscriptions and %d recently expired customers", len(*customers), len(*expiredCustomers)))
	if len(*customers) == 0 && len(*expiredCustomers) == 0 {
		return nil
	}
	now := time.Now()

	customersIds := make([]int64, len(*customers))
	for i, customer := range *customers {
		customersIds[i] = customer.ID
	}

	latestActiveTributes, err := s.purchaseRepository.FindLatestActiveTributesByCustomerIDs(ctx, customersIds)
	if err != nil {
		slog.Error("Failed to query tribute purchases", "error", err)
		return err
	}

	customerIdTributes := make(map[int64]*database.Purchase, len(*latestActiveTributes))
	for i := range *latestActiveTributes {
		p := &(*latestActiveTributes)[i]
		customerIdTributes[p.CustomerID] = p
	}

	tributesProcessed := make(map[int64]bool, len(*latestActiveTributes))
	sentExpiring := 0
	sentExpired := 0

	for _, customer := range *customers {
		daysUntilExpiration := s.getDaysUntilExpiration(now, *customer.ExpireAt)

		if p, ok := customerIdTributes[customer.ID]; ok {
			if daysUntilExpiration != 1 {
				continue
			}
			_, purchaseId, err := s.paymentService.CreatePurchase(ctx, p.Amount, p.Month, &customer, database.InvoiceTypeTribute)
			if err != nil {
				slog.Error("Failed to create tribute purchase", "error", err)
				continue
			}

			err = s.paymentService.ProcessPurchaseById(ctx, purchaseId)
			if err != nil {
				slog.Error("Failed to process tribute purchase", "error", err)
				continue
			}
			slog.Info("Tribute purchase processed successfully", "purchase_id", purchaseId)
			tributesProcessed[customer.ID] = true
		}
		if _, ok := tributesProcessed[customer.ID]; ok {
			continue
		}

		sent, err := s.sendClaimedNotification(ctx, customer, "expiring")
		if err != nil {
			slog.Error("Failed to send notification",
				"customer_id", customer.ID,
				"days_until_expiration", daysUntilExpiration,
				"error", err)
			continue
		}
		if !sent {
			continue
		}
		sentExpiring++

		slog.Info("Notification sent successfully",
			"customer_id", customer.ID,
			"days_until_expiration", daysUntilExpiration)
	}

	for _, customer := range *expiredCustomers {
		sent, err := s.sendClaimedNotification(ctx, customer, "expired")
		if err != nil {
			slog.Error("Failed to send expired subscription notification", "customer_id", customer.ID, "error", err)
			continue
		}
		if !sent {
			continue
		}
		sentExpired++
		slog.Info("Expired subscription notification sent successfully", "customer_id", customer.ID)
	}

	slog.Info(fmt.Sprintf("Processed tributes customers %d with expiring subscriptions", len(tributesProcessed)))
	slog.Info("Subscription notification check completed", "expiring_sent", sentExpiring, "expired_sent", sentExpired)
	return nil
}

func (s *SubscriptionService) sendClaimedNotification(ctx context.Context, customer database.Customer, kind string) (bool, error) {
	if customer.ExpireAt == nil {
		return false, nil
	}
	claimed, err := s.customerRepository.ClaimSubscriptionNotification(ctx, customer.ID, *customer.ExpireAt, kind)
	if err != nil || !claimed {
		return false, err
	}

	send := s.notify
	if send == nil {
		send = s.sendNotification
	}
	if err := send(ctx, customer); err != nil {
		if isPermanentNotificationDeliveryError(err) {
			return false, err
		}
		if releaseErr := s.customerRepository.ReleaseSubscriptionNotification(ctx, customer.ID, *customer.ExpireAt, kind); releaseErr != nil {
			slog.Error("Failed to release subscription notification claim", "customer_id", customer.ID, "kind", kind, "error", releaseErr)
		}
		return false, err
	}
	return true, nil
}

func isPermanentNotificationDeliveryError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"forbidden", "bot was blocked", "chat not found", "user is deactivated"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (s *SubscriptionService) getCustomersWithExpiringSubscriptions() (*[]database.Customer, error) {
	now := time.Now()
	endDate := now.AddDate(0, 0, 3)

	dbCustomers, err := s.customerRepository.FindByExpirationRange(context.Background(), now, endDate)
	if err != nil {
		return nil, err
	}

	return dbCustomers, nil
}

func (s *SubscriptionService) getCustomersWithRecentlyExpiredSubscriptions() (*[]database.Customer, error) {
	now := time.Now()
	return s.customerRepository.FindByExpirationRange(context.Background(), now.Add(-24*time.Hour), now)
}

func (s *SubscriptionService) getDaysUntilExpiration(now time.Time, expireAt time.Time) int {
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	expireDate := time.Date(expireAt.Year(), expireAt.Month(), expireAt.Day(), 0, 0, 0, 0, expireAt.Location())

	duration := expireDate.Sub(nowDate)
	return int(duration.Hours() / 24)
}

func (s *SubscriptionService) sendNotification(ctx context.Context, customer database.Customer) error {
	return s.sendNotificationWithOptions(ctx, customer, nil)
}

func (s *SubscriptionService) SendTestReminder(ctx context.Context, chatID int64, options TestReminderOptions) error {
	expireAt := time.Now().AddDate(0, 0, 2)
	switch options.Kind {
	case "expiring":
	case "expired":
		expireAt = time.Now().AddDate(0, 0, -1)
	default:
		return fmt.Errorf("unsupported reminder kind %q", options.Kind)
	}
	options.Template = strings.TrimSpace(options.Template)
	options.ButtonText = resolveTestReminderButtonText(options.ButtonText, s.tm, s.runtimeSettings)
	if options.Template == "" {
		return fmt.Errorf("reminder template is required")
	}
	if len([]rune(options.ButtonText)) < 1 || len([]rune(options.ButtonText)) > 64 {
		return fmt.Errorf("reminder button text must contain 1-64 characters")
	}
	var err error
	options.IconCustomEmojiID, err = runtimeconfig.NormalizeTelegramCustomEmojiID(options.IconCustomEmojiID)
	if err != nil {
		return err
	}
	options.ButtonStyle, err = runtimeconfig.NormalizeTelegramButtonStyle(options.ButtonStyle)
	if err != nil {
		return err
	}

	return s.sendNotificationWithOptions(ctx, database.Customer{
		TelegramID: chatID,
		Language:   "ru",
		ExpireAt:   &expireAt,
	}, &options)
}

func resolveTestReminderButtonText(value string, tm *translation.Manager, runtimeSettings *runtimeconfig.Service) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	if tm != nil {
		if value = strings.TrimSpace(resolveReminderButton("ru", tm, runtimeSettings).Text); value != "" {
			return value
		}
	}
	return "Продлить подписку"
}

func (s *SubscriptionService) sendNotificationWithOptions(ctx context.Context, customer database.Customer, testOptions *TestReminderOptions) error {
	expireDate := customer.ExpireAt.Format("02.01.2006")
	expired := customer.ExpireAt.Before(time.Now())
	translationKey := "subscription_expiring"
	runtimeKey := "subscriptionExpiringTemplate"
	if expired {
		translationKey = "subscription_expired"
		runtimeKey = "subscriptionExpiredTemplate"
	}

	messageText := fmt.Sprintf(s.tm.GetText(customer.Language, translationKey), expireDate)
	if s.runtimeSettings != nil {
		template := s.runtimeSettings.ContentText(customer.Language, runtimeKey, "")
		if template != "" {
			messageText = strings.ReplaceAll(template, "{date}", expireDate)
		}
	}
	if testOptions != nil {
		messageText = strings.ReplaceAll(testOptions.Template, "{date}", expireDate)
	}

	buttonOptions := resolveReminderButton(customer.Language, s.tm, s.runtimeSettings)
	if testOptions != nil {
		buttonOptions = reminderButtonOptions{
			Text:              testOptions.ButtonText,
			IconCustomEmojiID: testOptions.IconCustomEmojiID,
			Style:             testOptions.ButtonStyle,
		}
	}

	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    customer.TelegramID,
		Text:      messageText,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: buildRenewKeyboardWithOptions(buttonOptions),
		},
	})

	return err
}

func buildRenewKeyboard(lang string, tm *translation.Manager, runtimeSettings *runtimeconfig.Service) [][]models.InlineKeyboardButton {
	return buildRenewKeyboardWithOptions(resolveReminderButton(lang, tm, runtimeSettings))
}

func resolveReminderButton(lang string, tm *translation.Manager, runtimeSettings *runtimeconfig.Service) reminderButtonOptions {
	buttonText := tm.GetText(lang, "renew_subscription_button")
	button := reminderButtonOptions{Text: buttonText}
	if runtimeSettings != nil {
		buttonText = runtimeSettings.ContentText(lang, "subscriptionRenewButton", buttonText)
		settings := runtimeSettings.Snapshot().Content.SubscriptionReminderButton
		button.IconCustomEmojiID = settings.IconCustomEmojiID
		button.Style = settings.Style
	}
	button.Text = buttonText
	return button
}

func buildRenewKeyboardWithOptions(buttonOptions reminderButtonOptions) [][]models.InlineKeyboardButton {
	button := models.InlineKeyboardButton{
		Text:              buttonOptions.Text,
		IconCustomEmojiID: buttonOptions.IconCustomEmojiID,
		Style:             buttonOptions.Style,
	}
	if config.GetMiniAppURL() != "" {
		button.WebApp = &models.WebAppInfo{URL: miniAppTariffsURL(config.GetMiniAppURL())}
		return [][]models.InlineKeyboardButton{
			{button},
		}
	}

	button.CallbackData = handler.CallbackBuy
	return [][]models.InlineKeyboardButton{
		{button},
	}
}

func miniAppTariffsURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	query.Set("page", "buy")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
