package payment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"link-bot/internal/cache"
	"link-bot/internal/config"
	"link-bot/internal/cryptopay"
	"link-bot/internal/database"
	"link-bot/internal/integrations"
	"link-bot/internal/moynalog"
	"link-bot/internal/operations"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
	"link-bot/internal/translation"
	"link-bot/internal/yookasa"
	"link-bot/utils"
	"strings"
	"sync"
	"time"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

var trialActivationLocks sync.Map

type PaymentService struct {
	purchaseRepository  *database.PurchaseRepository
	promoCodeRepository *database.PromoCodeRepository
	remnawaveClient     *remnawave.Client
	customerRepository  *database.CustomerRepository
	telegramBot         *bot.Bot
	translation         *translation.Manager
	cryptoPayClient     *cryptopay.Client
	yookasaClient       *yookasa.Client
	referralRepository  *database.ReferralRepository
	cache               *cache.Cache
	moynalogClient      *moynalog.Client
	errorReporter       *operations.Reporter
	runtimeSettings     *runtimeconfig.Service
	integrationSettings *integrations.Service
	integrationGateway  *integrations.Gateway
}

type CreatePurchaseOptions struct {
	AgreementAccepted    bool
	IsAutoPayment        bool
	ParentPurchaseID     *int64
	PlanID               string
	TrafficLimitBytes    *int64
	DeviceLimitCount     *int
	PromoCodeID          *int64
	PromoCodeCode        string
	PromoDiscountPercent int
}

type SubscriptionActivatedPreviewOptions struct {
	Text              string
	Banner            string
	ButtonText        string
	IconCustomEmojiID string
	ButtonStyle       string
}

func lockTrialActivation(telegramId int64) func() {
	actual, _ := trialActivationLocks.LoadOrStore(telegramId, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func NewPaymentService(
	translation *translation.Manager,
	purchaseRepository *database.PurchaseRepository,
	promoCodeRepository *database.PromoCodeRepository,
	remnawaveClient *remnawave.Client,
	customerRepository *database.CustomerRepository,
	telegramBot *bot.Bot,
	cryptoPayClient *cryptopay.Client,
	yookasaClient *yookasa.Client,
	referralRepository *database.ReferralRepository,
	cache *cache.Cache,
	moynalogClient *moynalog.Client,
	runtimeSettings *runtimeconfig.Service,
	errorReporter *operations.Reporter,
	integrationSettings *integrations.Service,
) *PaymentService {
	var integrationGateway *integrations.Gateway
	if integrationSettings != nil {
		integrationGateway = integrations.NewGateway(integrationSettings)
	}
	return &PaymentService{
		purchaseRepository:  purchaseRepository,
		promoCodeRepository: promoCodeRepository,
		remnawaveClient:     remnawaveClient,
		customerRepository:  customerRepository,
		telegramBot:         telegramBot,
		translation:         translation,
		cryptoPayClient:     cryptoPayClient,
		yookasaClient:       yookasaClient,
		referralRepository:  referralRepository,
		cache:               cache,
		moynalogClient:      moynalogClient,
		runtimeSettings:     runtimeSettings,
		errorReporter:       errorReporter,
		integrationSettings: integrationSettings,
		integrationGateway:  integrationGateway,
	}
}

func (s PaymentService) ProcessPurchaseById(ctx context.Context, purchaseId int64) error {
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseId)
	if err != nil {
		return err
	}
	if purchase == nil {
		return fmt.Errorf("purchase with crypto invoice id %s not found", utils.MaskHalfInt64(purchaseId))
	}

	customer, err := s.customerRepository.FindById(ctx, purchase.CustomerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return fmt.Errorf("customer %s not found", utils.MaskHalfInt64(purchase.CustomerID))
	}

	s.completePromoRedemption(ctx, purchase, customer)

	if messageId, b := s.cache.Get(purchase.ID); b {
		_, err = s.telegramBot.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    customer.TelegramID,
			MessageID: messageId,
		})
		if err != nil {
			slog.Error("Error deleting message", "error", err)
		}
	}

	trafficLimit := purchaseTrafficLimit(purchase)
	deviceLimit := purchaseDeviceLimit(purchase)
	panelState, err := s.remnawaveClient.GetUserStateByTelegramID(ctx, customer.TelegramID)
	if err != nil {
		slog.Warn("payment: load panel state before purchase update failed", "error", err, "customerId", utils.MaskHalfInt64(customer.ID))
	} else if shouldAccumulateEntitlements(customer, panelState) {
		trafficLimit = mergeTrafficLimits(int(maxInt64(panelState.TrafficLimitBytes, 0)), trafficLimit)
		deviceLimit = mergeDeviceLimits(maxInt(panelState.DeviceLimit, 0), deviceLimit)
	}
	slog.Info(
		"payment: applying subscription entitlements",
		"customerId", utils.MaskHalfInt64(customer.ID),
		"telegramId", utils.MaskHalfInt64(customer.TelegramID),
		"purchaseMonths", purchase.Month,
		"trafficLimitBytes", trafficLimit,
		"deviceLimit", deviceLimit,
	)

	provisioning := remnawave.ProvisioningOptions{}
	if purchase.PlanID != nil && s.runtimeSettings != nil {
		if plan, ok := s.runtimeSettings.CheckoutPlan(*purchase.PlanID, purchase.Month); ok {
			provisioning = remnawave.ProvisioningOptions{
				InternalSquadUUIDs:   append([]string(nil), plan.InternalSquadUUIDs...),
				ExternalSquadUUID:    plan.ExternalSquadUUID,
				TrafficResetStrategy: config.TrafficLimitResetStrategy(),
				Tag:                  config.RemnawaveTag(),
				ApplySquads:          true,
			}
		}
	}
	var user *remapi.UserItemInfo
	if provisioning.ApplySquads {
		user, err = s.remnawaveClient.CreateOrUpdateUserWithOptions(ctx, customer.ID, customer.TelegramID, trafficLimit, deviceLimit, purchase.Month*config.DaysInMonth(), provisioning)
	} else {
		user, err = s.remnawaveClient.CreateOrUpdateUser(ctx, customer.ID, customer.TelegramID, trafficLimit, deviceLimit, purchase.Month*config.DaysInMonth(), false)
	}
	if err != nil {
		return err
	}

	err = s.purchaseRepository.MarkAsPaid(ctx, purchase.ID)
	if err != nil {
		return err
	}

	customerFilesToUpdate := map[string]interface{}{
		"subscription_link": user.SubscriptionUrl,
		"expire_at":         user.ExpireAt,
	}
	for field, value := range s.buildAutoPaymentCustomerUpdates(customer, purchase) {
		customerFilesToUpdate[field] = value
	}

	err = s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate)
	if err != nil {
		return err
	}

	err = s.sendSubscriptionActivatedMessage(ctx, customer)
	if err != nil {
		return err
	}

	go s.notifyAdminAboutPayment(ctx, purchase, customer)

	slog.Info("checking conditions for Moynalog receipt", "invoice_type", purchase.InvoiceType, "moynalog_client", s.moynalogClient != nil)
	if purchase.InvoiceType == database.InvoiceTypeYookasa && s.moynalogClient != nil {
		slog.Info("attempting to send receipt to Moynalog", "purchase_id", utils.MaskHalfInt64(purchase.ID), "amount", purchase.Amount, "month", purchase.Month)
		go func() {
			moynalogCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			err := s.sendReceiptToMoynalog(moynalogCtx, purchase)
			if err != nil {
				slog.Error("error sending receipt to Moynalog", "error", err, "purchase_id", utils.MaskHalfInt64(purchase.ID))
				_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: config.GetAdminTelegramId(),
					Text:   "Ошибка при отправке чека в Мой налог. Проверьте логи.",
				})
				if err != nil {
					slog.Error("error while sending moy nalog error message", "error", err, "purchase_id", utils.MaskHalfInt64(purchase.ID))
				}
			} else {
				slog.Info("successfully sent receipt to Moynalog", "purchase_id", utils.MaskHalfInt64(purchase.ID))
			}
		}()
	} else {
		if purchase.InvoiceType != database.InvoiceTypeYookasa {
			slog.Info("not sending receipt to Moynalog - not a Yookasa invoice", "invoice_type", purchase.InvoiceType, "purchase_id", utils.MaskHalfInt64(purchase.ID))
		} else if s.moynalogClient == nil {
			slog.Error("not sending receipt to Moynalog - client is nil", "purchase_id", utils.MaskHalfInt64(purchase.ID))
		}
	}

	ctxReferee := context.Background()
	referee, err := s.referralRepository.FindByReferee(ctxReferee, customer.TelegramID)
	if referee == nil {
		return nil
	}
	if config.GetReferralDays() <= 0 && config.ReferralTrafficBonusBytes() <= 0 {
		return nil
	}
	if referee.BonusGranted {
		return nil
	}
	if err != nil {
		return err
	}
	refereeCustomer, err := s.customerRepository.FindByTelegramId(ctxReferee, referee.ReferrerID)
	if err != nil {
		return err
	}
	refereeTrafficLimit, err := s.resolveCustomerTrafficLimit(ctxReferee, refereeCustomer)
	if err != nil {
		return err
	}
	referralTrafficLimit := mergeTrafficLimits(refereeTrafficLimit, config.ReferralTrafficBonusBytes())
	if referralTrafficLimit == 0 && refereeTrafficLimit > 0 && config.ReferralTrafficBonusBytes() <= 0 {
		referralTrafficLimit = refereeTrafficLimit
	}
	refereeDeviceLimit, err := s.resolveCustomerDeviceLimit(ctxReferee, refereeCustomer)
	if err != nil {
		return err
	}
	refereeUser, err := s.remnawaveClient.CreateOrUpdateUser(ctxReferee, refereeCustomer.ID, refereeCustomer.TelegramID, referralTrafficLimit, refereeDeviceLimit, config.GetReferralDays(), false)
	if err != nil {
		return err
	}
	refereeUserFilesToUpdate := map[string]interface{}{
		"subscription_link": refereeUser.GetSubscriptionUrl(),
		"expire_at":         refereeUser.GetExpireAt(),
	}
	err = s.customerRepository.UpdateFields(ctxReferee, refereeCustomer.ID, refereeUserFilesToUpdate)
	if err != nil {
		return err
	}
	err = s.referralRepository.MarkBonusGranted(ctxReferee, referee.ID)
	if err != nil {
		return err
	}
	slog.Info("Granted referral bonus", "customer_id", utils.MaskHalfInt64(refereeCustomer.ID))
	_, err = s.telegramBot.SendMessage(ctxReferee, &bot.SendMessageParams{
		ChatID:    refereeCustomer.TelegramID,
		ParseMode: models.ParseModeHTML,
		Text:      buildReferralBonusGrantedText(refereeCustomer.Language),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: s.createConnectKeyboard(refereeCustomer),
		},
	})

	slog.Info("purchase processed", "purchase_id", utils.MaskHalfInt64(purchase.ID), "type", purchase.InvoiceType, "customer_id", utils.MaskHalfInt64(customer.ID))

	return nil
}

func (s PaymentService) sendSubscriptionActivatedMessage(ctx context.Context, customer *database.Customer) error {
	commerce := runtimeconfig.DefaultSettings().Content.Commerce
	if s.runtimeSettings != nil {
		commerce = s.runtimeSettings.Snapshot().Content.Commerce
	}
	return s.sendSubscriptionActivatedMessageWithCommerce(ctx, customer, commerce)
}

func (s PaymentService) SendSubscriptionActivatedPreview(ctx context.Context, customer *database.Customer, options SubscriptionActivatedPreviewOptions) error {
	if customer == nil || customer.TelegramID == 0 {
		return errors.New("preview customer is required")
	}
	if s.telegramBot == nil {
		return errors.New("telegram bot is unavailable")
	}
	commerce, err := normalizeSubscriptionActivatedPreview(options)
	if err != nil {
		return err
	}
	return s.sendSubscriptionActivatedMessageWithCommerce(ctx, customer, commerce)
}

func normalizeSubscriptionActivatedPreview(options SubscriptionActivatedPreviewOptions) (runtimeconfig.TelegramCommerceSettings, error) {
	commerce := runtimeconfig.DefaultSettings().Content.Commerce
	commerce.SuccessText = strings.TrimSpace(options.Text)
	if len([]rune(commerce.SuccessText)) < 1 || len([]rune(commerce.SuccessText)) > 3500 {
		return commerce, errors.New("success message must contain 1-3500 characters")
	}
	commerce.SuccessBanner = strings.TrimSpace(options.Banner)
	commerce.SuccessButton.Text = strings.TrimSpace(options.ButtonText)
	if len([]rune(commerce.SuccessButton.Text)) < 1 || len([]rune(commerce.SuccessButton.Text)) > 64 {
		return commerce, errors.New("success button text must contain 1-64 characters")
	}
	var err error
	commerce.SuccessButton.IconCustomEmojiID, err = runtimeconfig.NormalizeTelegramCustomEmojiID(options.IconCustomEmojiID)
	if err != nil {
		return commerce, err
	}
	commerce.SuccessButton.Style, err = runtimeconfig.NormalizeTelegramButtonStyle(options.ButtonStyle)
	if err != nil {
		return commerce, err
	}
	return commerce, nil
}

func (s PaymentService) sendSubscriptionActivatedMessageWithCommerce(ctx context.Context, customer *database.Customer, commerce runtimeconfig.TelegramCommerceSettings) error {
	caption := commerce.SuccessText
	replyMarkup := models.InlineKeyboardMarkup{
		InlineKeyboard: s.createConnectKeyboardWithSettings(customer, commerce.SuccessButton),
	}

	if strings.TrimSpace(commerce.SuccessBanner) != "" {
		imageData, filename, err := readTelegramImage(commerce.SuccessBanner)
		if err == nil {
			_, err = s.telegramBot.SendPhoto(ctx, &bot.SendPhotoParams{
				ChatID: customer.TelegramID,
				Photo: &models.InputFileUpload{
					Filename: filename,
					Data:     bytes.NewReader(imageData),
				},
				Caption:     caption,
				ParseMode:   models.ParseModeHTML,
				ReplyMarkup: replyMarkup,
			})
			if err == nil {
				return nil
			}
			slog.Error("payment: send subscription banner failed", "error", err, "telegramId", utils.MaskHalfInt64(customer.TelegramID))
		} else {
			slog.Error("payment: load subscription banner failed", "error", err)
		}
	}

	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      customer.TelegramID,
		Text:        caption,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: replyMarkup,
	})
	return err
}

func readTelegramImage(source string) ([]byte, string, error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(strings.ToLower(source), "https://") || strings.HasPrefix(strings.ToLower(source), "http://") {
		client := &http.Client{Timeout: 8 * time.Second}
		response, err := client.Get(source)
		if err != nil {
			return nil, "", err
		}
		defer response.Body.Close()
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return nil, "", fmt.Errorf("unexpected banner status %d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, 12<<20))
		if err != nil {
			return nil, "", err
		}
		filename := filepath.Base(response.Request.URL.Path)
		if filename == "." || filename == "/" || filename == "" {
			filename = "subscription.png"
		}
		return data, filename, nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, "", err
	}
	return data, filepath.Base(source), nil
}

func readTelegramAsset(name string) ([]byte, error) {
	paths := []string{
		"/assets/telegram/" + name,
		"assets/telegram/" + name,
	}

	var lastErr error
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}

	return nil, lastErr
}

func (s PaymentService) createConnectKeyboard(customer *database.Customer) [][]models.InlineKeyboardButton {
	settings := runtimeconfig.DefaultSettings().Content.Commerce.SuccessButton
	if s.runtimeSettings != nil {
		settings = s.runtimeSettings.Snapshot().Content.Commerce.SuccessButton
	}
	return s.createConnectKeyboardWithSettings(customer, settings)
}

func (s PaymentService) createConnectKeyboardWithSettings(customer *database.Customer, settings runtimeconfig.TelegramButtonSettings) [][]models.InlineKeyboardButton {
	fallbackText := "Личный кабинет"
	if s.translation != nil {
		fallbackText = s.translation.GetText(customer.Language, "web_app_button_text")
	}
	button := paymentTelegramButton(settings, fallbackText)
	if config.GetMiniAppURL() != "" {
		button.WebApp = &models.WebAppInfo{URL: config.GetMiniAppURL()}
		return [][]models.InlineKeyboardButton{
			{button},
		}
	}

	button.CallbackData = "connect"
	return [][]models.InlineKeyboardButton{
		{button},
	}
}

func paymentTelegramButton(settings runtimeconfig.TelegramButtonSettings, fallbackText string) models.InlineKeyboardButton {
	text := strings.TrimSpace(settings.Text)
	if text == "" {
		text = fallbackText
	}
	return models.InlineKeyboardButton{
		Text:              text,
		IconCustomEmojiID: strings.TrimSpace(settings.IconCustomEmojiID),
		Style:             strings.TrimSpace(settings.Style),
	}
}

func (s PaymentService) CreatePurchase(ctx context.Context, amount float64, months int, customer *database.Customer, invoiceType database.InvoiceType) (url string, purchaseId int64, err error) {
	return s.CreatePurchaseWithOptions(ctx, amount, months, customer, invoiceType, CreatePurchaseOptions{})
}

func (s PaymentService) CreatePurchaseWithOptions(ctx context.Context, amount float64, months int, customer *database.Customer, invoiceType database.InvoiceType, options CreatePurchaseOptions) (url string, purchaseId int64, err error) {
	switch invoiceType {
	case database.InvoiceTypeCrypto:
		url, purchaseId, err = s.createCryptoInvoice(ctx, amount, months, customer, options)
	case database.InvoiceTypeYookasa:
		url, purchaseId, err = s.createYookasaInvoice(ctx, amount, months, customer, options)
	case database.InvoiceTypeTelegram:
		url, purchaseId, err = s.createTelegramInvoice(ctx, amount, months, customer, options)
	case database.InvoiceTypeTribute:
		url, purchaseId, err = s.createTributeInvoice(ctx, amount, months, customer)
	case database.InvoiceTypeLava, database.InvoiceTypeWata, database.InvoiceTypePlatega, database.InvoiceTypeFreeKassa, database.InvoiceTypeHeleket:
		url, purchaseId, err = s.createExternalInvoice(ctx, amount, months, customer, invoiceType, options)
	default:
		err = fmt.Errorf("unknown invoice type: %s", invoiceType)
	}
	if err != nil && s.errorReporter != nil {
		category := "Платежи"
		switch invoiceType {
		case database.InvoiceTypeYookasa:
			category = "YooKassa"
		case database.InvoiceTypeCrypto:
			category = "CryptoPay"
		case database.InvoiceTypeTelegram:
			category = "Telegram Stars"
		case database.InvoiceTypeLava, database.InvoiceTypeWata, database.InvoiceTypePlatega, database.InvoiceTypeFreeKassa, database.InvoiceTypeHeleket:
			category = string(invoiceType)
		}
		s.errorReporter.Report(ctx, operations.ReportInput{
			Category:  category,
			Severity:  "critical",
			Operation: "create_payment",
			Message:   "Не удалось создать платеж",
			Err:       err,
			Details: map[string]interface{}{
				"invoiceType": invoiceType,
				"months":      months,
				"amount":      amount,
			},
		})
	}
	return url, purchaseId, err
}

var ErrCustomerNotFound = errors.New("customer not found")

func (s PaymentService) CancelTributePurchase(ctx context.Context, telegramId int64) error {
	slog.Info("Canceling tribute purchase", "telegram_id", utils.MaskHalfInt64(telegramId))
	customer, err := s.customerRepository.FindByTelegramId(ctx, telegramId)
	if err != nil {
		return err
	}
	if customer == nil {
		return ErrCustomerNotFound
	}
	tributePurchase, err := s.purchaseRepository.FindByCustomerIDAndInvoiceTypeLast(ctx, customer.ID, database.InvoiceTypeTribute)
	if err != nil {
		return err
	}
	if tributePurchase == nil {
		return errors.New("tribute purchase not found")
	}
	trafficLimit, err := s.resolveCustomerTrafficLimit(ctx, customer)
	if err != nil {
		return err
	}
	deviceLimit, err := s.resolveCustomerDeviceLimit(ctx, customer)
	if err != nil {
		return err
	}
	expireAt, err := s.remnawaveClient.DecreaseSubscription(ctx, telegramId, trafficLimit, deviceLimit, -tributePurchase.Month*config.DaysInMonth())
	if err != nil {
		return err
	}

	if err := s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
		"expire_at": expireAt,
	}); err != nil {
		return err
	}

	if err := s.purchaseRepository.UpdateFields(ctx, tributePurchase.ID, map[string]interface{}{
		"status": database.PurchaseStatusCancel,
	}); err != nil {
		return err
	}
	_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    telegramId,
		ParseMode: models.ParseModeHTML,
		Text:      s.translation.GetText(customer.Language, "tribute_cancelled"),
	})
	if err != nil {
		slog.Error("Error sending message about tribute cancelled", "error", err, "telegram_id", utils.MaskHalfInt64(telegramId))
	}
	slog.Info("Canceled tribute purchase", "purchase_id", utils.MaskHalfInt64(tributePurchase.ID), "telegram_id", utils.MaskHalfInt64(telegramId))
	return nil
}

func (s PaymentService) createCryptoInvoice(ctx context.Context, amount float64, months int, customer *database.Customer, options CreatePurchaseOptions) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:              database.InvoiceTypeCrypto,
		Status:                   database.PurchaseStatusNew,
		Amount:                   amount,
		Currency:                 "RUB",
		CustomerID:               customer.ID,
		Month:                    months,
		PlanID:                   optionalTrimmedStringPointer(options.PlanID),
		TrafficLimitBytes:        options.TrafficLimitBytes,
		DeviceLimitCount:         options.DeviceLimitCount,
		AgreementAccepted:        options.AgreementAccepted,
		IsAutoPayment:            options.IsAutoPayment,
		ParentPurchaseID:         options.ParentPurchaseID,
		PromoCodeID:              options.PromoCodeID,
		PromoCodeSnapshot:        optionalTrimmedStringPointer(options.PromoCodeCode),
		PromoCodeDiscountPercent: optionalPositiveIntPointer(options.PromoDiscountPercent),
	})
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	cryptoClient, acceptedAssets := s.currentCryptoPayClient()
	if cryptoClient == nil {
		return "", 0, errors.New("Crypto Pay не настроен")
	}
	invoice, err := cryptoClient.CreateInvoice(&cryptopay.InvoiceRequest{
		CurrencyType:   "fiat",
		Fiat:           "RUB",
		Amount:         fmt.Sprintf("%d", int(amount)),
		AcceptedAssets: acceptedAssets,
		Payload:        fmt.Sprintf("purchaseId=%d&username=%s", purchaseId, ctx.Value("username")),
		Description:    fmt.Sprintf("Subscription on %d month", months),
		PaidBtnName:    "callback",
		PaidBtnUrl:     config.BotURL(),
	})
	if err != nil {
		slog.Error("Error creating invoice", "error", err)
		return "", 0, err
	}

	updates := map[string]interface{}{
		"crypto_invoice_url": invoice.BotInvoiceUrl,
		"crypto_invoice_id":  invoice.InvoiceID,
		"status":             database.PurchaseStatusPending,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, updates)
	if err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}

	return invoice.BotInvoiceUrl, purchaseId, nil
}

func (s PaymentService) createYookasaInvoice(ctx context.Context, amount float64, months int, customer *database.Customer, options CreatePurchaseOptions) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:              database.InvoiceTypeYookasa,
		Status:                   database.PurchaseStatusNew,
		Amount:                   amount,
		Currency:                 "RUB",
		CustomerID:               customer.ID,
		Month:                    months,
		PlanID:                   optionalTrimmedStringPointer(options.PlanID),
		TrafficLimitBytes:        options.TrafficLimitBytes,
		DeviceLimitCount:         options.DeviceLimitCount,
		AgreementAccepted:        options.AgreementAccepted,
		IsAutoPayment:            options.IsAutoPayment,
		ParentPurchaseID:         options.ParentPurchaseID,
		PromoCodeID:              options.PromoCodeID,
		PromoCodeSnapshot:        optionalTrimmedStringPointer(options.PromoCodeCode),
		PromoCodeDiscountPercent: optionalPositiveIntPointer(options.PromoDiscountPercent),
	})
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	yookasaClient := s.currentYookasaClient()
	if yookasaClient == nil {
		return "", 0, errors.New("YooKassa не настроена")
	}
	invoice, err := yookasaClient.CreateInvoice(ctx, int(amount), months, customer.ID, purchaseId, s.buildYookassaReturnURL(purchaseId))
	if err != nil {
		slog.Error("Error creating invoice", "error", err)
		return "", 0, err
	}

	updates := map[string]interface{}{
		"yookasa_url": invoice.Confirmation.ConfirmationURL,
		"yookasa_id":  invoice.ID,
		"status":      database.PurchaseStatusPending,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, updates)
	if err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}

	return invoice.Confirmation.ConfirmationURL, purchaseId, nil
}

func (s PaymentService) createExternalInvoice(ctx context.Context, amount float64, months int, customer *database.Customer, invoiceType database.InvoiceType, options CreatePurchaseOptions) (string, int64, error) {
	if s.integrationGateway == nil {
		return "", 0, errors.New("платёжные интеграции не настроены")
	}
	provider, ok := integrationProviderForInvoiceType(invoiceType)
	if !ok {
		return "", 0, fmt.Errorf("unsupported external invoice type: %s", invoiceType)
	}
	purchaseID, err := s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType: invoiceType, Status: database.PurchaseStatusNew, Amount: amount, Currency: "RUB",
		CustomerID: customer.ID, Month: months, PlanID: optionalTrimmedStringPointer(options.PlanID),
		TrafficLimitBytes: options.TrafficLimitBytes, DeviceLimitCount: options.DeviceLimitCount,
		AgreementAccepted: options.AgreementAccepted, IsAutoPayment: options.IsAutoPayment,
		ParentPurchaseID: options.ParentPurchaseID, PromoCodeID: options.PromoCodeID,
		PromoCodeSnapshot:        optionalTrimmedStringPointer(options.PromoCodeCode),
		PromoCodeDiscountPercent: optionalPositiveIntPointer(options.PromoDiscountPercent),
	})
	if err != nil {
		return "", 0, err
	}
	username, _ := ctx.Value("username").(string)
	created, err := s.integrationGateway.Create(ctx, integrations.CreatePaymentRequest{
		Provider: provider, PurchaseID: purchaseID, Amount: amount, Currency: "RUB",
		Description: fmt.Sprintf("Link-Bot: подписка на %d мес.", months), CustomerID: customer.ID,
		Username: strings.TrimSpace(username), ReturnURL: s.buildYookassaReturnURL(purchaseID),
	})
	if err != nil {
		return "", 0, err
	}
	if err := s.purchaseRepository.UpdateFields(ctx, purchaseID, map[string]interface{}{
		"external_payment_id":  created.ExternalID,
		"external_payment_url": created.URL,
		"status":               database.PurchaseStatusPending,
	}); err != nil {
		return "", 0, err
	}
	return created.URL, purchaseID, nil
}

func (s PaymentService) currentCryptoPayClient() (*cryptopay.Client, string) {
	if s.integrationSettings != nil {
		if cfg, ok := s.integrationSettings.Config(integrations.ProviderCryptoPay); ok {
			assets := strings.TrimSpace(cfg["acceptedAssets"])
			if assets == "" {
				assets = "USDT,TON,BTC,ETH,LTC,BNB,TRX,USDC"
			}
			return cryptopay.NewCryptoPayClient(cfg["apiUrl"], cfg["token"]), assets
		}
	}
	return s.cryptoPayClient, config.CryptoPayAcceptedAssets()
}

func (s PaymentService) CurrentCryptoPayClient() *cryptopay.Client {
	client, _ := s.currentCryptoPayClient()
	return client
}

func (s PaymentService) IsProviderEnabled(provider string) bool {
	if s.integrationSettings != nil {
		_, ok := s.integrationSettings.Config(provider)
		return ok
	}
	switch provider {
	case integrations.ProviderYooKassa:
		return config.IsYookasaEnabled()
	case integrations.ProviderCryptoPay:
		return config.IsCryptoPayEnabled()
	default:
		return false
	}
}

func (s PaymentService) currentYookasaClient() *yookasa.Client {
	if s.integrationSettings != nil {
		if cfg, ok := s.integrationSettings.Config(integrations.ProviderYooKassa); ok {
			return yookasa.NewConfiguredClient(cfg["apiUrl"], cfg["shopId"], cfg["secretKey"], cfg["email"])
		}
	}
	return s.yookasaClient
}

func integrationProviderForInvoiceType(invoiceType database.InvoiceType) (string, bool) {
	providers := map[database.InvoiceType]string{
		database.InvoiceTypeLava: integrations.ProviderLava, database.InvoiceTypeWata: integrations.ProviderWata,
		database.InvoiceTypePlatega: integrations.ProviderPlatega, database.InvoiceTypeFreeKassa: integrations.ProviderFreeKassa,
		database.InvoiceTypeHeleket: integrations.ProviderHeleket,
	}
	provider, ok := providers[invoiceType]
	return provider, ok
}

func invoiceTypeForIntegrationProvider(provider string) (database.InvoiceType, bool) {
	providers := map[string]database.InvoiceType{
		integrations.ProviderLava: database.InvoiceTypeLava, integrations.ProviderWata: database.InvoiceTypeWata,
		integrations.ProviderPlatega: database.InvoiceTypePlatega, integrations.ProviderFreeKassa: database.InvoiceTypeFreeKassa,
		integrations.ProviderHeleket: database.InvoiceTypeHeleket,
	}
	invoiceType, ok := providers[provider]
	return invoiceType, ok
}

func (s PaymentService) ProcessExternalWebhook(ctx context.Context, provider string, headers http.Header, raw []byte, form url.Values) (string, error) {
	if s.integrationGateway == nil {
		return "", errors.New("payment gateway is not configured")
	}
	event, err := s.integrationGateway.HandleWebhook(ctx, provider, headers, raw, form)
	if err != nil {
		return "", err
	}
	invoiceType, ok := invoiceTypeForIntegrationProvider(provider)
	if !ok {
		return "", fmt.Errorf("unsupported webhook provider: %s", provider)
	}
	var purchase *database.Purchase
	if event.PurchaseID > 0 {
		purchase, err = s.purchaseRepository.FindById(ctx, event.PurchaseID)
	} else if event.ExternalID != "" {
		purchase, err = s.purchaseRepository.FindByExternalPaymentID(ctx, invoiceType, event.ExternalID)
	}
	if err != nil {
		return "", err
	}
	if purchase == nil || purchase.InvoiceType != invoiceType {
		return "", errors.New("purchase not found")
	}
	if purchase.ExternalPaymentID != nil && event.ExternalID != "" && *purchase.ExternalPaymentID != event.ExternalID {
		return "", errors.New("external payment ID mismatch")
	}
	if event.Amount > 0 && math.Abs(event.Amount-purchase.Amount) > 0.01 {
		return "", errors.New("payment amount mismatch")
	}
	if event.Currency != "" && !strings.EqualFold(event.Currency, purchase.Currency) {
		return "", errors.New("payment currency mismatch")
	}
	if event.Paid && purchase.Status != database.PurchaseStatusPaid {
		if err := s.ProcessPurchaseById(ctx, purchase.ID); err != nil {
			return "", err
		}
	} else if event.Cancelled && purchase.Status != database.PurchaseStatusPaid && purchase.Status != database.PurchaseStatusCancel {
		if err := s.purchaseRepository.UpdateFields(ctx, purchase.ID, map[string]interface{}{"status": database.PurchaseStatusCancel}); err != nil {
			return "", err
		}
	}
	if provider == integrations.ProviderFreeKassa {
		return "YES", nil
	}
	return "OK", nil
}

func (s PaymentService) createTelegramInvoice(ctx context.Context, amount float64, months int, customer *database.Customer, options CreatePurchaseOptions) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:              database.InvoiceTypeTelegram,
		Status:                   database.PurchaseStatusNew,
		Amount:                   amount,
		Currency:                 "STARS",
		CustomerID:               customer.ID,
		Month:                    months,
		PlanID:                   optionalTrimmedStringPointer(options.PlanID),
		TrafficLimitBytes:        options.TrafficLimitBytes,
		DeviceLimitCount:         options.DeviceLimitCount,
		AgreementAccepted:        options.AgreementAccepted,
		PromoCodeID:              options.PromoCodeID,
		PromoCodeSnapshot:        optionalTrimmedStringPointer(options.PromoCodeCode),
		PromoCodeDiscountPercent: optionalPositiveIntPointer(options.PromoDiscountPercent),
	})
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, nil
	}

	invoiceUrl, err := s.telegramBot.CreateInvoiceLink(ctx, &bot.CreateInvoiceLinkParams{
		Title:    s.translation.GetText(customer.Language, "invoice_title"),
		Currency: "XTR",
		Prices: []models.LabeledPrice{
			{
				Label:  s.translation.GetText(customer.Language, "invoice_label"),
				Amount: int(amount),
			},
		},
		Description: s.translation.GetText(customer.Language, "invoice_description"),
		Payload:     fmt.Sprintf("%d&%s", purchaseId, ctx.Value("username")),
	})

	updates := map[string]interface{}{
		"status": database.PurchaseStatusPending,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, updates)
	if err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}

	return invoiceUrl, purchaseId, nil
}

func (s PaymentService) ActivateTrial(ctx context.Context, telegramId int64) (string, error) {
	trial := runtimeconfig.DefaultSettings().Trial
	if s.runtimeSettings != nil {
		trial = s.runtimeSettings.TrialSettings()
	}
	if !trial.Enabled || trial.Days == 0 {
		return "", nil
	}

	unlockTrial := lockTrialActivation(telegramId)
	defer unlockTrial()

	customer, err := s.customerRepository.FindByTelegramId(ctx, telegramId)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return "", err
	}
	if customer == nil {
		return "", fmt.Errorf("customer %d not found", telegramId)
	}
	if customer.TrialUsed {
		return "", fmt.Errorf("trial already used")
	}
	trafficLimit := trial.TrafficGB * 1024 * 1024 * 1024
	if trial.UnlimitedTraffic {
		trafficLimit = 0
	}
	user, err := s.remnawaveClient.CreateOrUpdateUserWithOptions(ctx, customer.ID, telegramId, trafficLimit, trial.DeviceLimit, trial.Days, remnawave.ProvisioningOptions{
		InternalSquadUUIDs:   append([]string(nil), trial.InternalSquadUUIDs...),
		ExternalSquadUUID:    trial.ExternalSquadUUID,
		TrafficResetStrategy: trial.TrafficResetStrategy,
		Tag:                  trial.Tag,
		ApplySquads:          true,
	})
	if err != nil {
		slog.Error("Error creating user", "error", err)
		return "", err
	}

	customerFilesToUpdate := map[string]interface{}{
		"subscription_link": user.GetSubscriptionUrl(),
		"expire_at":         user.GetExpireAt(),
		"trial_used":        true,
	}

	err = s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate)
	if err != nil {
		return "", err
	}

	return user.GetSubscriptionUrl(), nil

}

func (s PaymentService) resolveCustomerTrafficLimit(ctx context.Context, customer *database.Customer) (int, error) {
	if customer == nil {
		return config.TrafficLimit(), nil
	}

	panelState, err := s.remnawaveClient.GetUserStateByTelegramID(ctx, customer.TelegramID)
	if err != nil {
		slog.Warn("payment: resolve traffic limit from panel failed", "error", err, "customerId", utils.MaskHalfInt64(customer.ID))
	} else if panelState != nil && panelState.Exists {
		if panelState.TrafficLimitBytes <= 0 {
			return 0, nil
		}
		return int(panelState.TrafficLimitBytes), nil
	}

	lastPurchase, err := s.purchaseRepository.FindHighestSuccessfulPurchaseByCustomer(ctx, customer.ID)
	if err != nil {
		return 0, err
	}
	if lastPurchase != nil {
		if lastPurchase.TrafficLimitBytes != nil {
			return int(*lastPurchase.TrafficLimitBytes), nil
		}
		if lastPurchase.Month > 0 {
			return config.TrafficLimitForMonths(lastPurchase.Month), nil
		}
	}
	if customer.TrialUsed {
		trial := runtimeconfig.DefaultSettings().Trial
		if s.runtimeSettings != nil {
			trial = s.runtimeSettings.TrialSettings()
		}
		if trial.UnlimitedTraffic {
			return 0, nil
		}
		return trial.TrafficGB * 1024 * 1024 * 1024, nil
	}

	return config.TrafficLimit(), nil
}

func (s PaymentService) resolveCustomerDeviceLimit(ctx context.Context, customer *database.Customer) (int, error) {
	if customer == nil {
		return config.DeviceLimitForMonths(1), nil
	}

	panelState, err := s.remnawaveClient.GetUserStateByTelegramID(ctx, customer.TelegramID)
	if err != nil {
		slog.Warn("payment: resolve device limit from panel failed", "error", err, "customerId", utils.MaskHalfInt64(customer.ID))
	} else if panelState != nil && panelState.Exists {
		if panelState.DeviceLimit <= 0 {
			return 0, nil
		}
		return panelState.DeviceLimit, nil
	}

	lastPurchase, err := s.purchaseRepository.FindHighestSuccessfulPurchaseByCustomer(ctx, customer.ID)
	if err != nil {
		return 0, err
	}
	if lastPurchase != nil {
		if lastPurchase.DeviceLimitCount != nil {
			return *lastPurchase.DeviceLimitCount, nil
		}
		if lastPurchase.Month > 0 {
			return config.DeviceLimitForMonths(lastPurchase.Month), nil
		}
	}
	if customer.TrialUsed {
		return config.TrialDeviceLimit(), nil
	}

	return config.DeviceLimitForMonths(1), nil
}

func shouldAccumulateEntitlements(customer *database.Customer, panelState *remnawave.UserState) bool {
	if panelState != nil {
		if panelState.Exists && panelState.Active {
			return true
		}
		if panelState.Exists && panelState.ExpireAt != nil && panelState.ExpireAt.After(time.Now().UTC()) {
			return true
		}
	}
	return customer != nil && customer.ExpireAt != nil && customer.ExpireAt.After(time.Now().UTC())
}

func mergeTrafficLimits(currentLimit int, addedLimit int) int {
	if currentLimit <= 0 || addedLimit <= 0 {
		return 0
	}
	return currentLimit + addedLimit
}

func mergeDeviceLimits(currentLimit int, addedLimit int) int {
	if currentLimit <= 0 || addedLimit <= 0 {
		return 0
	}
	if currentLimit > addedLimit {
		return currentLimit
	}
	return addedLimit
}

func purchaseTrafficLimit(purchase *database.Purchase) int {
	if purchase != nil && purchase.TrafficLimitBytes != nil {
		return int(*purchase.TrafficLimitBytes)
	}
	if purchase == nil {
		return config.TrafficLimit()
	}
	return config.TrafficLimitForMonths(purchase.Month)
}

func purchaseDeviceLimit(purchase *database.Purchase) int {
	if purchase != nil && purchase.DeviceLimitCount != nil {
		return *purchase.DeviceLimitCount
	}
	if purchase == nil {
		return config.DeviceLimitForMonths(1)
	}
	return config.DeviceLimitForMonths(purchase.Month)
}

func maxInt(value, fallback int) int {
	if value < 0 {
		return fallback
	}
	return value
}

func maxInt64(value, fallback int64) int64 {
	if value < 0 {
		return fallback
	}
	return value
}

func (s PaymentService) buildAutoPaymentCustomerUpdates(customer *database.Customer, purchase *database.Purchase) map[string]interface{} {
	if !config.EnableAutoPayment() || customer == nil || purchase == nil || purchase.InvoiceType != database.InvoiceTypeYookasa {
		return nil
	}

	if purchase.YookasaPaymentMethodID == nil || !purchase.YookasaPaymentMethodSaved {
		return nil
	}

	updates := map[string]interface{}{
		"yookasa_payment_method_id":       purchase.YookasaPaymentMethodID,
		"yookasa_payment_method_type":     purchase.YookasaPaymentMethodType,
		"yookasa_payment_method_title":    purchase.YookasaPaymentMethodTitle,
		"yookasa_payment_method_saved_at": time.Now().UTC(),
		"autopay_plan_months":             purchase.Month,
		"yookasa_last_charge_at":          time.Now().UTC(),
		"yookasa_last_charge_status":      string(database.PurchaseStatusPaid),
		"yookasa_last_charge_error":       nil,
	}

	if customer.YookasaPaymentMethodID == nil || *customer.YookasaPaymentMethodID == uuid.Nil {
		updates["autopay_enabled"] = true
	}

	return updates
}

func (s PaymentService) persistYookassaPaymentMethod(ctx context.Context, purchase *database.Purchase, invoice *yookasa.Payment) error {
	if purchase == nil || invoice == nil {
		return nil
	}

	updates := map[string]interface{}{}
	if invoice.PaymentMethod.Saved && invoice.PaymentMethod.ID != uuid.Nil {
		updates["yookasa_payment_method_id"] = invoice.PaymentMethod.ID
	}
	if methodType := strings.TrimSpace(invoice.PaymentMethod.Type); methodType != "" {
		updates["yookasa_payment_method_type"] = methodType
	}
	if title := strings.TrimSpace(buildYookassaPaymentMethodTitleSafe(invoice)); title != "" {
		updates["yookasa_payment_method_title"] = title
	}
	updates["yookasa_payment_method_saved"] = invoice.PaymentMethod.Saved

	if len(updates) == 0 {
		return nil
	}

	if err := s.purchaseRepository.UpdateFields(ctx, purchase.ID, updates); err != nil {
		return err
	}

	if value, ok := updates["yookasa_payment_method_id"]; ok {
		id := value.(uuid.UUID)
		purchase.YookasaPaymentMethodID = &id
	}
	if value, ok := updates["yookasa_payment_method_type"]; ok {
		methodType := value.(string)
		purchase.YookasaPaymentMethodType = &methodType
	}
	if value, ok := updates["yookasa_payment_method_title"]; ok {
		title := value.(string)
		purchase.YookasaPaymentMethodTitle = &title
	}
	if value, ok := updates["yookasa_payment_method_saved"]; ok {
		purchase.YookasaPaymentMethodSaved = value.(bool)
	}

	return nil
}

func buildYookassaPaymentMethodTitle(invoice *yookasa.Payment) string {
	if invoice == nil {
		return ""
	}

	title := strings.TrimSpace(invoice.PaymentMethod.Title)
	if title != "" {
		return title
	}
	if invoice.PaymentMethod.Card != nil && strings.TrimSpace(invoice.PaymentMethod.Card.Last4) != "" {
		cardType := strings.TrimSpace(invoice.PaymentMethod.Card.CardType)
		if cardType == "" {
			cardType = "card"
		}
		return fmt.Sprintf("%s •••• %s", strings.ToUpper(cardType), strings.TrimSpace(invoice.PaymentMethod.Card.Last4))
	}
	return strings.TrimSpace(invoice.PaymentMethod.Type)
}

func optionalTrimmedStringPointer(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func optionalPositiveIntPointer(value int) *int {
	if value <= 0 {
		return nil
	}
	result := value
	return &result
}

func buildReferralBonusGrantedText(langCode string) string {
	days := config.GetReferralDays()
	trafficGb := config.ReferralTrafficBonusBytes() / (1024 * 1024 * 1024)

	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(langCode)), "en") {
		return fmt.Sprintf(
			"You have received a referral bonus: +%d days and +%d GB.\nThe reward is credited after your invited friend pays for a subscription.",
			days,
			trafficGb,
		)
	}

	return fmt.Sprintf(
		"<tg-emoji emoji-id='5258362837411045098'>☺️</tg-emoji> Вам начислен реферальный бонус: +%d дней и +%d ГБ.\nБонус выдаётся после покупки тарифа приглашённым пользователем.",
		days,
		trafficGb,
	)
}

func (s PaymentService) completePromoRedemption(ctx context.Context, purchase *database.Purchase, customer *database.Customer) {
	if purchase == nil || purchase.PromoCodeID == nil || *purchase.PromoCodeID == 0 {
		return
	}
	if s.promoCodeRepository == nil || customer == nil {
		return
	}

	err := s.promoCodeRepository.CompleteRedemption(ctx, &database.PromoCode{ID: *purchase.PromoCodeID}, customer.ID, purchase.ID)
	if err == nil {
		return
	}

	switch {
	case errors.Is(err, database.ErrPromoCodeAlreadyUsed):
		slog.Warn("payment: promo redemption already recorded", "purchaseId", utils.MaskHalfInt64(purchase.ID), "customerId", utils.MaskHalfInt64(customer.ID), "promoCodeId", utils.MaskHalfInt64(*purchase.PromoCodeID))
	case errors.Is(err, database.ErrPromoCodeLimitReached):
		slog.Warn("payment: promo redemption skipped because limit reached", "purchaseId", utils.MaskHalfInt64(purchase.ID), "customerId", utils.MaskHalfInt64(customer.ID), "promoCodeId", utils.MaskHalfInt64(*purchase.PromoCodeID))
	default:
		slog.Error("payment: promo redemption failed", "error", err, "purchaseId", utils.MaskHalfInt64(purchase.ID), "customerId", utils.MaskHalfInt64(customer.ID), "promoCodeId", utils.MaskHalfInt64(*purchase.PromoCodeID))
	}
}

func (s PaymentService) CancelYookassaPayment(purchaseId int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseId)
	if err != nil {
		return err
	}
	if purchase == nil {
		return fmt.Errorf("purchase with crypto invoice id %s not found", utils.MaskHalfInt64(purchaseId))
	}

	purchaseFieldsToUpdate := map[string]interface{}{
		"status": database.PurchaseStatusCancel,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, purchaseFieldsToUpdate)
	if err != nil {
		return err
	}

	return nil
}

func (s PaymentService) createTributeInvoice(ctx context.Context, amount float64, months int, customer *database.Customer) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType: database.InvoiceTypeTribute,
		Status:      database.PurchaseStatusPending,
		Amount:      amount,
		Currency:    "RUB",
		CustomerID:  customer.ID,
		Month:       months,
	})
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	return "", purchaseId, nil
}

func (s PaymentService) ProcessAutoPayment(ctx context.Context, customer *database.Customer) error {
	if !config.EnableAutoPayment() || customer == nil {
		return nil
	}
	now := time.Now().UTC()
	if !customer.AutoPaymentEnabled || customer.YookasaPaymentMethodID == nil || customer.AutoPaymentPlanMonths == nil {
		return nil
	}
	if *customer.YookasaPaymentMethodID == uuid.Nil || *customer.AutoPaymentPlanMonths <= 0 {
		return nil
	}
	if customer.ExpireAt == nil || customer.ExpireAt.After(now) {
		return nil
	}
	if shouldDelayAutoPaymentRetry(customer, now) {
		return nil
	}

	hasPending, err := s.purchaseRepository.HasPendingAutoPaymentByCustomer(ctx, customer.ID)
	if err != nil {
		return err
	}
	if hasPending {
		return nil
	}

	months := *customer.AutoPaymentPlanMonths
	amount := config.Price(months)
	if amount <= 0 {
		return fmt.Errorf("autopay plan %d has no configured price", months)
	}

	lastPurchase, err := s.purchaseRepository.FindSuccessfulPurchaseByCustomer(ctx, customer.ID)
	if err != nil {
		return err
	}

	purchase := &database.Purchase{
		InvoiceType:               database.InvoiceTypeYookasa,
		Status:                    database.PurchaseStatusNew,
		Amount:                    float64(amount),
		Currency:                  "RUB",
		CustomerID:                customer.ID,
		Month:                     months,
		AgreementAccepted:         true,
		IsAutoPayment:             true,
		YookasaPaymentMethodID:    customer.YookasaPaymentMethodID,
		YookasaPaymentMethodType:  customer.YookasaPaymentMethodType,
		YookasaPaymentMethodTitle: customer.YookasaPaymentMethodTitle,
		YookasaPaymentMethodSaved: true,
	}
	if lastPurchase != nil {
		purchase.ParentPurchaseID = &lastPurchase.ID
	}

	purchaseID, err := s.purchaseRepository.Create(ctx, purchase)
	if err != nil {
		return err
	}
	purchase.ID = purchaseID

	yookasaClient := s.currentYookasaClient()
	if yookasaClient == nil {
		return errors.New("YooKassa не настроена")
	}
	charge, err := yookasaClient.ChargeSavedPaymentMethod(ctx, amount, months, customer.ID, purchaseID, *customer.YookasaPaymentMethodID)
	if err != nil {
		_ = s.purchaseRepository.UpdateFields(ctx, purchaseID, map[string]interface{}{
			"status": database.PurchaseStatusCancel,
		})
		_ = s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
			"yookasa_last_charge_at":     time.Now().UTC(),
			"yookasa_last_charge_status": string(database.PurchaseStatusCancel),
			"yookasa_last_charge_error":  trimAutoPaymentError(err.Error()),
		})
		s.notifyAutoPaymentFailure(ctx, customer, trimAutoPaymentError(err.Error()))
		return err
	}

	updates := map[string]interface{}{
		"yookasa_id": charge.ID,
		"status":     database.PurchaseStatusPending,
	}
	if err := s.purchaseRepository.UpdateFields(ctx, purchaseID, updates); err != nil {
		return err
	}
	purchase.YookasaID = &charge.ID
	purchase.Status = database.PurchaseStatusPending

	if err := s.persistYookassaPaymentMethod(ctx, purchase, charge); err != nil {
		return err
	}

	if charge.IsCancelled() {
		_ = s.purchaseRepository.UpdateFields(ctx, purchaseID, map[string]interface{}{
			"status": database.PurchaseStatusCancel,
		})
		_ = s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
			"yookasa_last_charge_at":     time.Now().UTC(),
			"yookasa_last_charge_status": string(database.PurchaseStatusCancel),
			"yookasa_last_charge_error":  "autopay_canceled",
		})
		s.notifyAutoPaymentFailure(ctx, customer, "")
		return nil
	}

	if charge.Paid {
		if err := s.ProcessPurchaseById(ctx, purchaseID); err != nil {
			return err
		}
		s.notifyAutoPaymentSuccess(ctx, customer, months)
		return nil
	}

	_ = s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
		"yookasa_last_charge_at":     time.Now().UTC(),
		"yookasa_last_charge_status": string(database.PurchaseStatusPending),
		"yookasa_last_charge_error":  nil,
	})

	return nil
}

func shouldDelayAutoPaymentRetry(customer *database.Customer, now time.Time) bool {
	if customer == nil || customer.YookasaLastChargeAt == nil || customer.YookasaLastChargeStatus == nil {
		return false
	}

	status := strings.TrimSpace(strings.ToLower(*customer.YookasaLastChargeStatus))
	if status == "" || status == string(database.PurchaseStatusPaid) {
		return false
	}

	lastAttempt := customer.YookasaLastChargeAt.UTC()
	if lastAttempt.IsZero() || lastAttempt.After(now) {
		return false
	}

	elapsed := now.Sub(lastAttempt)
	switch status {
	case string(database.PurchaseStatusPending), string(database.PurchaseStatusNew):
		return elapsed < 15*time.Minute
	default:
		return elapsed < 12*time.Hour
	}
}

func buildYookassaPaymentMethodTitleSafe(invoice *yookasa.Payment) string {
	if invoice == nil {
		return ""
	}

	title := strings.TrimSpace(invoice.PaymentMethod.Title)
	if title != "" {
		return title
	}

	if invoice.PaymentMethod.Card != nil {
		last4 := strings.TrimSpace(invoice.PaymentMethod.Card.Last4)
		if last4 != "" {
			cardType := strings.TrimSpace(invoice.PaymentMethod.Card.CardType)
			if cardType == "" {
				cardType = "card"
			}
			return strings.ToUpper(cardType) + " **** " + last4
		}
	}

	return strings.TrimSpace(invoice.PaymentMethod.Type)
}

func (s PaymentService) SyncYookassaPurchaseStatus(ctx context.Context, purchaseID int64) (database.PurchaseStatus, error) {
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseID)
	if err != nil {
		return "", err
	}
	if purchase == nil {
		return "", fmt.Errorf("purchase %s not found", utils.MaskHalfInt64(purchaseID))
	}
	if purchase.InvoiceType != database.InvoiceTypeYookasa {
		return purchase.Status, nil
	}
	if purchase.Status != database.PurchaseStatusPending {
		return purchase.Status, nil
	}
	if purchase.YookasaID == nil {
		return purchase.Status, nil
	}

	yookasaClient := s.currentYookasaClient()
	if yookasaClient == nil {
		return purchase.Status, errors.New("YooKassa не настроена")
	}
	invoice, err := yookasaClient.GetPayment(ctx, *purchase.YookasaID)
	if err != nil {
		return purchase.Status, err
	}

	if invoice.IsCancelled() {
		if purchase.IsAutoPayment {
			_ = s.customerRepository.UpdateFields(ctx, purchase.CustomerID, map[string]interface{}{
				"yookasa_last_charge_at":     time.Now().UTC(),
				"yookasa_last_charge_status": string(database.PurchaseStatusCancel),
				"yookasa_last_charge_error":  "autopay_canceled",
			})
		}
		if err := s.CancelYookassaPayment(purchase.ID); err != nil {
			return purchase.Status, err
		}
		return database.PurchaseStatusCancel, nil
	}

	if !invoice.Paid {
		return database.PurchaseStatusPending, nil
	}

	if err := s.persistYookassaPaymentMethod(ctx, purchase, invoice); err != nil {
		return purchase.Status, err
	}

	ctxWithProfile := ctx
	if username, ok := invoice.Metadata["username"]; ok {
		ctxWithProfile = context.WithValue(ctxWithProfile, "username", username)
	}
	if telegramName, ok := invoice.Metadata["telegramName"]; ok {
		ctxWithProfile = context.WithValue(ctxWithProfile, "telegramName", telegramName)
	}
	if err := s.ProcessPurchaseById(ctxWithProfile, purchase.ID); err != nil {
		return purchase.Status, err
	}

	return database.PurchaseStatusPaid, nil
}

func (s PaymentService) buildYookassaReturnURL(purchaseID int64) string {
	base := config.GetMiniAppURL()
	if base == "" {
		base = config.BotURL()
	}
	if base == "" {
		return ""
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return base
	}

	if parsed.Path == "" {
		parsed.Path = "/mini-app/"
	}

	if strings.HasSuffix(parsed.Path, "/mini-app/") || strings.HasSuffix(parsed.Path, "/mini-app") {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/payment-return"
	}

	query := parsed.Query()
	query.Set("purchaseId", fmt.Sprintf("%d", purchaseID))
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func trimAutoPaymentError(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	if len(message) > 240 {
		return message[:240]
	}
	return message
}

func (s PaymentService) notifyAutoPaymentSuccess(ctx context.Context, customer *database.Customer, months int) {
	if s.telegramBot == nil || customer == nil {
		return
	}

	text := fmt.Sprintf("Автоплатёж выполнен успешно. Подписка продлена на %d мес.", months)
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(customer.Language)), "en") {
		text = fmt.Sprintf("Auto payment succeeded. Your subscription was renewed for %d month(s).", months)
	}

	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: customer.TelegramID,
		Text:   text,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: s.createConnectKeyboard(customer),
		},
	})
	if err != nil {
		slog.Warn("payment: failed to notify autopay success", "error", err, "customerId", utils.MaskHalfInt64(customer.ID))
	}
}

func (s PaymentService) notifyAutoPaymentFailure(ctx context.Context, customer *database.Customer, reason string) {
	if s.telegramBot == nil || customer == nil {
		return
	}

	text := "Автоплатёж не прошёл. Проверьте способ оплаты в разделе «Платежи»."
	if reason != "" {
		text = fmt.Sprintf("%s\n\n%s", text, reason)
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(customer.Language)), "en") {
		text = "Auto payment failed. Check your payment method in the Payments section."
		if reason != "" {
			text = fmt.Sprintf("%s\n\n%s", text, reason)
		}
	}

	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: customer.TelegramID,
		Text:   text,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: s.createConnectKeyboard(customer),
		},
	})
	if err != nil {
		slog.Warn("payment: failed to notify autopay failure", "error", err, "customerId", utils.MaskHalfInt64(customer.ID))
	}
}

func (s PaymentService) sendReceiptToMoynalog(ctx context.Context, purchase *database.Purchase) error {
	if s.moynalogClient == nil {
		return fmt.Errorf("moynalog client not initialized")
	}

	var monthString string
	switch purchase.Month {
	case 1:
		monthString = "месяц"
	case 3, 4:
		monthString = "месяца"
	default:
		monthString = "месяцев"
	}
	comment := fmt.Sprintf("Подписка на %d %s", purchase.Month, monthString)
	amount := purchase.Amount

	_, err := s.moynalogClient.CreateIncome(ctx, amount, comment)
	if err != nil {
		return fmt.Errorf("failed to create income in Moynalog: %w", err)
	}

	slog.Info("Receipt sent to Moynalog", "purchase_id", utils.MaskHalfInt64(purchase.ID), "amount", amount, "comment", comment)
	return nil
}
