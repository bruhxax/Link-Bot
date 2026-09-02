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
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"link-bot/internal/cache"
	"link-bot/internal/config"
	"link-bot/internal/cryptopay"
	"link-bot/internal/database"
	"link-bot/internal/integrations"
	"link-bot/internal/operations"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
	"link-bot/internal/translation"
	"link-bot/internal/yookasa"
	"link-bot/utils"
)

var trialActivationLocks sync.Map

type PaymentService struct {
	purchaseRepository        *database.PurchaseRepository
	subscriptionRepository    *database.SubscriptionRepository
	promoCodeRepository       *database.PromoCodeRepository
	remnawaveClient           *remnawave.Client
	customerRepository        *database.CustomerRepository
	telegramBot               *bot.Bot
	translation               *translation.Manager
	cryptoPayClient           *cryptopay.Client
	yookasaClient             *yookasa.Client
	referralRepository        *database.ReferralRepository
	walletRepository          *database.WalletRepository
	cache                     *cache.Cache
	moynalogReceiptRepository *database.MoyNalogReceiptRepository
	errorReporter             *operations.Reporter
	runtimeSettings           *runtimeconfig.Service
	integrationSettings       *integrations.Service
	integrationGateway        *integrations.Gateway
}

type CreatePurchaseOptions struct {
	SubscriptionID          *int64
	AgreementAccepted       bool
	IsAutoPayment           bool
	ParentPurchaseID        *int64
	PlanID                  string
	TrafficLimitBytes       *int64
	DeviceLimitCount        *int
	PromoCodeID             *int64
	PromoCodeCode           string
	PromoDiscountPercent    int
	PurchaseKind            database.PurchaseKind
	ExtraDevices            int
	IsFreePlan              bool
	FreePlanOneTime         bool
	GiftRecipientUsername   string
	GiftRecipientCustomerID *int64
	GiftToken               *uuid.UUID
	P2PSenderReference      string
	P2PDestination          database.P2PDestinationSnapshot
	ReturnTarget            string
}

const freePlanRenewalWindow = 7 * 24 * time.Hour

var (
	ErrFreePlanAlreadyUsed = errors.New("free plan already used")
	ErrFreePlanTooEarly    = errors.New("free plan is available during the last week")
)

type SubscriptionActivatedPreviewOptions struct {
	Text              string
	Banner            string
	ButtonText        string
	IconCustomEmojiID string
	ButtonStyle       string
}

func (s PaymentService) language() string {
	if s.runtimeSettings != nil {
		return s.runtimeSettings.Language()
	}
	if s.translation != nil {
		return s.translation.ActiveLanguage()
	}
	return config.DefaultLanguage()
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
	walletRepository *database.WalletRepository,
	cache *cache.Cache,
	moynalogReceiptRepository *database.MoyNalogReceiptRepository,
	runtimeSettings *runtimeconfig.Service,
	errorReporter *operations.Reporter,
	integrationSettings *integrations.Service,
	subscriptionRepository *database.SubscriptionRepository,
) *PaymentService {
	var integrationGateway *integrations.Gateway
	if integrationSettings != nil {
		integrationGateway = integrations.NewGateway(integrationSettings)
	}
	return &PaymentService{
		purchaseRepository:        purchaseRepository,
		subscriptionRepository:    subscriptionRepository,
		promoCodeRepository:       promoCodeRepository,
		remnawaveClient:           remnawaveClient,
		customerRepository:        customerRepository,
		telegramBot:               telegramBot,
		translation:               translation,
		cryptoPayClient:           cryptoPayClient,
		yookasaClient:             yookasaClient,
		referralRepository:        referralRepository,
		walletRepository:          walletRepository,
		cache:                     cache,
		moynalogReceiptRepository: moynalogReceiptRepository,
		runtimeSettings:           runtimeSettings,
		errorReporter:             errorReporter,
		integrationSettings:       integrationSettings,
		integrationGateway:        integrationGateway,
	}
}

func (s PaymentService) ProcessPurchaseById(ctx context.Context, purchaseId int64) error {
	releasePurchase, err := s.purchaseRepository.LockForProcessing(ctx, purchaseId)
	if err != nil {
		return err
	}
	defer func() {
		if err := releasePurchase(); err != nil {
			slog.Error("payment: release purchase processing lock failed", "purchaseId", utils.MaskHalfInt64(purchaseId), "error", err)
		}
	}()

	purchase, err := s.purchaseRepository.FindById(ctx, purchaseId)
	if err != nil {
		return err
	}
	if purchase == nil {
		return fmt.Errorf("purchase with crypto invoice id %s not found", utils.MaskHalfInt64(purchaseId))
	}
	if purchase.Status == database.PurchaseStatusPaid {
		slog.Info("payment: duplicate purchase processing skipped", "purchaseId", utils.MaskHalfInt64(purchaseId))
		return nil
	}

	customer, err := s.customerRepository.FindById(ctx, purchase.CustomerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return fmt.Errorf("customer %s not found", utils.MaskHalfInt64(purchase.CustomerID))
	}
	if purchase.PurchaseKind == database.PurchaseKindGift {
		return s.processGiftPurchase(ctx, purchase, customer)
	}
	subscription, err := s.subscriptionForPurchase(ctx, customer, purchase)
	if err != nil {
		return err
	}
	if purchase.IsFreePlan {
		planID := ""
		if purchase.PlanID != nil {
			planID = strings.TrimSpace(*purchase.PlanID)
		}
		releaseFreePlan, lockErr := s.purchaseRepository.LockFreePlanActivation(ctx, customer.ID, planID)
		if lockErr != nil {
			return lockErr
		}
		defer func() {
			if err := releaseFreePlan(); err != nil {
				slog.Error("payment: release free plan activation lock failed", "purchaseId", utils.MaskHalfInt64(purchaseId), "error", err)
			}
		}()

		// A different purchase can finish while this request waits for the
		// customer/plan lock, so refresh the selected subscription before the
		// authoritative eligibility check.
		subscription, err = s.subscriptionForPurchase(ctx, customer, purchase)
		if err != nil {
			return err
		}
		eligibilityErr := s.ValidateFreePlanEligibility(ctx, customer.ID, planID, purchase.FreePlanOneTime, subscription)
		if eligibilityErr != nil {
			// A paid device pack must never be lost if another free-plan claim was
			// fulfilled first. In that edge case, fulfill only the paid devices.
			if purchase.ExtraDevices > 0 && purchase.Amount > 0 {
				purchase.PurchaseKind = database.PurchaseKindExtraDevices
			} else {
				return eligibilityErr
			}
		}
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

	if purchase.PurchaseKind == database.PurchaseKindExtraDevices {
		var user *remnawave.PanelUser
		userID, userUUID := subscriptionPanelIdentity(subscription)
		if userID > 0 || userUUID != uuid.Nil {
			user, err = s.remnawaveClient.AddDeviceLimitByIdentity(ctx, userID, userUUID, purchase.ExtraDevices)
		} else if subscription.IsPrimary {
			user, err = s.remnawaveClient.AddDeviceLimit(ctx, customer.TelegramID, purchase.ExtraDevices)
		} else {
			err = errors.New("subscription is not active yet")
		}
		if err != nil {
			return err
		}
		if err := s.purchaseRepository.MarkAsPaid(ctx, purchase.ID); err != nil {
			return err
		}
		purchase.Status = database.PurchaseStatusPaid
		s.queueMoyNalogReceipt(purchase)
		if err := s.persistSubscriptionPanelState(ctx, customer, subscription, user); err != nil {
			return err
		}
		s.notifyAdminAboutPayment(ctx, purchase, customer)
		text := "<b>Устройства добавлены</b>\n\nЛимит подписки увеличен на <b>{devices}</b>."
		if s.runtimeSettings != nil {
			text = s.runtimeSettings.ContentText(customer.Language, "devicePurchaseSuccess", text)
		}
		text = strings.ReplaceAll(text, "{devices}", fmt.Sprintf("%d", purchase.ExtraDevices))
		if _, sendErr := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: customer.TelegramID, ParseMode: models.ParseModeHTML, Text: text,
		}); sendErr != nil {
			slog.Error("payment: extra devices customer notification failed", "error", sendErr, "purchase_id", utils.MaskHalfInt64(purchase.ID))
		}
		return nil
	}

	trafficLimit := purchaseTrafficLimit(purchase)
	deviceLimit := purchaseDeviceLimit(purchase)
	panelState, err := s.panelStateForSubscription(ctx, customer, subscription)
	if err != nil {
		slog.Warn("payment: load panel state before purchase update failed", "error", err, "customerId", utils.MaskHalfInt64(customer.ID))
	} else if shouldAccumulateEntitlements(customerForSubscription(customer, subscription), panelState) {
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
	if s.runtimeSettings != nil {
		provisioning.UsernameTemplate = s.runtimeSettings.Snapshot().Panel.UsernameTemplate
	}
	if purchase.PlanID != nil && s.runtimeSettings != nil {
		if plan, ok := s.runtimeSettings.CheckoutPlan(*purchase.PlanID, purchase.Month); ok {
			provisioning.InternalSquadUUIDs = append([]string(nil), plan.InternalSquadUUIDs...)
			provisioning.InternalSquadsConfigured = plan.InternalSquadsConfigured
			provisioning.ExternalSquadUUID = plan.ExternalSquadUUID
			provisioning.TrafficResetStrategy = config.TrafficLimitResetStrategy()
			provisioning.Tag = config.RemnawaveTag()
			provisioning.ApplySquads = true
		}
	}
	userID, userUUID := subscriptionPanelIdentity(subscription)
	user, err := s.remnawaveClient.CreateOrUpdateUserForSubscription(ctx, customer.ID, customer.TelegramID, subscription.ID, userID, userUUID, subscription.IsPrimary, trafficLimit, deviceLimit, purchase.Month*config.DaysInMonth(), provisioning)
	if err != nil {
		return err
	}

	err = s.purchaseRepository.MarkAsPaid(ctx, purchase.ID)
	if err != nil {
		return err
	}
	purchase.Status = database.PurchaseStatusPaid
	s.queueMoyNalogReceipt(purchase)

	if err = s.persistSubscriptionPanelState(ctx, customer, subscription, user); err != nil {
		return err
	}
	if subscription.IsPrimary {
		customerFilesToUpdate := s.buildAutoPaymentCustomerUpdates(customer, purchase)
		if len(customerFilesToUpdate) > 0 {
			if err = s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate); err != nil {
				return err
			}
		}
	}

	// The payment notification must not depend on Telegram accepting the
	// customer-facing confirmation message. This is especially important for
	// Stars, whose successful-payment update is the only confirmation callback.
	s.notifyAdminAboutPayment(ctx, purchase, customer)

	if err = s.sendSubscriptionActivatedMessage(ctx, customer); err != nil {
		slog.Error(
			"payment: subscription activated, but customer notification failed",
			"error", err,
			"purchase_id", utils.MaskHalfInt64(purchase.ID),
			"invoice_type", purchase.InvoiceType,
			"telegram_id", utils.MaskHalfInt64(customer.TelegramID),
		)
	}

	if rewardErr := s.grantReferralReward(context.Background(), customer, "purchase", purchase); rewardErr != nil {
		slog.Error("payment: grant referral purchase reward failed", "error", rewardErr, "purchase_id", utils.MaskHalfInt64(purchase.ID))
	}

	slog.Info("purchase processed", "purchase_id", utils.MaskHalfInt64(purchase.ID), "type", purchase.InvoiceType, "customer_id", utils.MaskHalfInt64(customer.ID))

	return nil
}

func (s PaymentService) grantReferralReward(ctx context.Context, invitedCustomer *database.Customer, eventType string, purchase *database.Purchase) error {
	if invitedCustomer == nil || s.referralRepository == nil {
		return nil
	}
	settings := runtimeconfig.DefaultSettings().Referrals
	if s.runtimeSettings != nil {
		settings = s.runtimeSettings.Snapshot().Referrals
	}
	if eventType == "purchase" {
		if purchase == nil || purchase.Amount <= 0 || purchase.PurchaseKind != database.PurchaseKindSubscription || purchase.IsFreePlan || purchase.InvoiceType == database.InvoiceTypeFree || purchase.InvoiceType == database.InvoiceTypeBalance {
			return nil
		}
	}
	referral, err := s.referralRepository.FindByReferee(ctx, invitedCustomer.TelegramID)
	if err != nil || referral == nil {
		return err
	}
	if eventType == "purchase" && !settings.RewardEveryPurchase && referral.BonusGranted {
		return nil
	}

	rewardSettings := settings.Trial
	var purchaseID *int64
	if eventType == "purchase" {
		rewardSettings = settings.Purchase
		purchaseID = &purchase.ID
	}
	balanceCents := referralBalanceRewardCents(rewardSettings, s.referralPurchaseAmountRub(purchase))
	trafficBytes := int64(rewardSettings.TrafficGB) * 1024 * 1024 * 1024
	if rewardSettings.Days == 0 && trafficBytes == 0 && balanceCents == 0 {
		return nil
	}
	reward, claimed, err := s.referralRepository.ClaimReward(ctx, referral.ID, eventType, purchaseID, rewardSettings.Days, trafficBytes, balanceCents)
	if err != nil || !claimed {
		return err
	}

	referrer, err := s.customerRepository.FindByTelegramId(ctx, referral.ReferrerID)
	if err == nil && referrer == nil {
		err = errors.New("referrer customer not found")
	}
	if err == nil && (rewardSettings.Days > 0 || trafficBytes > 0) {
		var currentTraffic, deviceLimit int
		currentTraffic, err = s.resolveCustomerTrafficLimit(ctx, referrer)
		if err == nil {
			deviceLimit, err = s.resolveCustomerDeviceLimit(ctx, referrer)
		}
		if err == nil {
			nextTraffic := mergeTrafficLimits(currentTraffic, int(trafficBytes))
			var panelUser *remnawave.PanelUser
			panelUser, err = s.remnawaveClient.CreateOrUpdateUser(ctx, referrer.ID, referrer.TelegramID, nextTraffic, deviceLimit, rewardSettings.Days, false)
			if err == nil {
				err = s.customerRepository.UpdateFields(ctx, referrer.ID, map[string]interface{}{
					"subscription_link": panelUser.GetSubscriptionUrl(),
					"expire_at":         panelUser.GetExpireAt(),
				})
			}
		}
	}
	if err == nil && balanceCents > 0 {
		if s.walletRepository == nil {
			err = errors.New("wallet repository is unavailable")
		} else {
			_, _, err = s.walletRepository.Apply(ctx, referrer.ID, balanceCents, "referral_"+eventType, fmt.Sprintf("referral-reward:%d", reward.ID), referralRewardDescription(eventType))
		}
	}
	if err != nil {
		_ = s.referralRepository.MarkRewardFailed(ctx, reward.ID, err)
		return err
	}
	if err := s.referralRepository.MarkRewardGranted(ctx, reward.ID); err != nil {
		return err
	}
	if eventType == "purchase" && !referral.BonusGranted {
		if err := s.referralRepository.MarkBonusGranted(ctx, referral.ID); err != nil {
			return err
		}
	}
	if s.telegramBot != nil {
		_, sendErr := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: referrer.TelegramID, ParseMode: models.ParseModeHTML,
			Text:        buildReferralBonusGrantedText(s.language(), eventType, rewardSettings.Days, rewardSettings.TrafficGB, balanceCents),
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: s.createConnectKeyboard(referrer)},
		})
		if sendErr != nil {
			slog.Warn("payment: referral reward notification failed", "error", sendErr, "customer_id", utils.MaskHalfInt64(referrer.ID))
		}
	}
	slog.Info("payment: referral reward granted", "event", eventType, "customer_id", utils.MaskHalfInt64(referrer.ID), "reward_id", reward.ID)
	return nil
}

func referralBalanceRewardCents(settings runtimeconfig.ReferralRewardSettings, purchaseAmountRub float64) int64 {
	switch settings.BalanceMode {
	case "fixed":
		return int64(settings.BalanceRub) * 100
	case "percent":
		if purchaseAmountRub > 0 {
			return int64(math.Round(purchaseAmountRub * float64(settings.BalancePercent)))
		}
	}
	return 0
}

func (s PaymentService) referralPurchaseAmountRub(purchase *database.Purchase) float64 {
	if purchase == nil || purchase.Amount <= 0 {
		return 0
	}
	if purchase.InvoiceType != database.InvoiceTypeTelegram && !strings.EqualFold(purchase.Currency, "STARS") && !strings.EqualFold(purchase.Currency, "XTR") {
		return purchase.Amount
	}
	if s.runtimeSettings == nil || purchase.PlanID == nil {
		return 0
	}
	plan, ok := s.runtimeSettings.CheckoutPlan(*purchase.PlanID, purchase.Month)
	if !ok || plan.PriceRub <= 0 {
		return 0
	}
	priceRub := plan.PriceRub
	if purchase.ExtraDevices > 0 {
		for _, pack := range s.runtimeSettings.Snapshot().DevicePacks {
			if pack.Enabled && pack.Devices == purchase.ExtraDevices {
				priceRub += pack.PriceRub
				break
			}
		}
	}
	if purchase.PromoCodeDiscountPercent != nil && *purchase.PromoCodeDiscountPercent > 0 {
		priceRub = int(math.Round(float64(priceRub) * float64(100-*purchase.PromoCodeDiscountPercent) / 100))
		if priceRub < 1 {
			priceRub = 1
		}
	}
	return float64(priceRub)
}

func referralRewardDescription(eventType string) string {
	if eventType == "trial" {
		return "Реферальная награда за активацию пробного периода"
	}
	return "Реферальная награда за покупку приглашённого пользователя"
}

func (s PaymentService) sendSubscriptionActivatedMessage(ctx context.Context, customer *database.Customer) error {
	commerce := runtimeconfig.DefaultSettings().Content.Commerce
	if s.runtimeSettings != nil {
		settings := s.runtimeSettings.Snapshot()
		commerce = runtimeconfig.LocalizeTelegramDefaults(settings.Content, settings.Localization.Language).Commerce
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
		runtime := s.runtimeSettings.Snapshot()
		settings = runtimeconfig.LocalizeTelegramDefaults(runtime.Content, runtime.Localization.Language).Commerce.SuccessButton
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
	case database.InvoiceTypeFree:
		url, purchaseId, err = s.createFreePlanPurchase(ctx, months, customer, options)
	case database.InvoiceTypeBalance:
		url, purchaseId, err = s.createBalancePurchase(ctx, amount, months, customer, options)
	case database.InvoiceTypeCrypto:
		url, purchaseId, err = s.createCryptoInvoice(ctx, amount, months, customer, options)
	case database.InvoiceTypeYookasa:
		url, purchaseId, err = s.createYookasaInvoice(ctx, amount, months, customer, options)
	case database.InvoiceTypeTelegram:
		url, purchaseId, err = s.createTelegramInvoice(ctx, amount, months, customer, options)
	case database.InvoiceTypeTribute:
		url, purchaseId, err = s.createTributeInvoice(ctx, amount, months, customer)
	case database.InvoiceTypeP2P:
		url, purchaseId, err = s.createP2PInvoice(ctx, amount, months, customer, options)
	case database.InvoiceTypeLava, database.InvoiceTypeWata, database.InvoiceTypePlatega, database.InvoiceTypeFreeKassa, database.InvoiceTypeHeleket, database.InvoiceTypePally:
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
		case database.InvoiceTypeP2P:
			category = "P2P перевод"
		case database.InvoiceTypeBalance:
			category = "Баланс"
		case database.InvoiceTypeLava, database.InvoiceTypeWata, database.InvoiceTypePlatega, database.InvoiceTypeFreeKassa, database.InvoiceTypeHeleket, database.InvoiceTypePally:
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

func validateFreePlanEligibility(oneTime, previouslyUsed bool, expireAt *time.Time, now time.Time) error {
	if oneTime && previouslyUsed {
		return ErrFreePlanAlreadyUsed
	}
	if expireAt != nil && expireAt.After(now.Add(freePlanRenewalWindow)) {
		return ErrFreePlanTooEarly
	}
	return nil
}

func (s PaymentService) ValidateFreePlanEligibility(ctx context.Context, customerID int64, planID string, oneTime bool, subscription *database.CustomerSubscription) error {
	previouslyUsed := false
	var err error
	if oneTime {
		previouslyUsed, err = s.purchaseRepository.HasSuccessfulFreePlanPurchase(ctx, customerID, planID, 0)
		if err != nil {
			return err
		}
	}
	var expireAt *time.Time
	if subscription != nil {
		expireAt = subscription.ExpireAt
	}
	return validateFreePlanEligibility(oneTime, previouslyUsed, expireAt, time.Now())
}

func (s PaymentService) createFreePlanPurchase(ctx context.Context, months int, customer *database.Customer, options CreatePurchaseOptions) (string, int64, error) {
	purchaseID, err := s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:       database.InvoiceTypeFree,
		Status:            database.PurchaseStatusNew,
		Amount:            0,
		Currency:          "RUB",
		CustomerID:        customer.ID,
		SubscriptionID:    options.SubscriptionID,
		Month:             months,
		PlanID:            optionalTrimmedStringPointer(options.PlanID),
		TrafficLimitBytes: options.TrafficLimitBytes,
		DeviceLimitCount:  options.DeviceLimitCount,
		AgreementAccepted: options.AgreementAccepted,
		PurchaseKind:      options.PurchaseKind,
		IsFreePlan:        true,
		FreePlanOneTime:   options.FreePlanOneTime,
	})
	if err != nil {
		return "", 0, err
	}
	if err := s.ProcessPurchaseById(ctx, purchaseID); err != nil {
		if cancelErr := s.purchaseRepository.UpdateFields(ctx, purchaseID, map[string]interface{}{"status": database.PurchaseStatusCancel}); cancelErr != nil {
			slog.Error("payment: cancel failed free plan purchase", "purchase_id", utils.MaskHalfInt64(purchaseID), "error", cancelErr)
		}
		return "", purchaseID, err
	}
	return "", purchaseID, nil
}

func (s PaymentService) createBalancePurchase(ctx context.Context, amount float64, months int, customer *database.Customer, options CreatePurchaseOptions) (string, int64, error) {
	if s.walletRepository == nil || customer == nil || amount <= 0 {
		return "", 0, errors.New("balance payments are unavailable")
	}
	amountCents := int64(math.Round(amount * 100))
	purchaseID, err := s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:              database.InvoiceTypeBalance,
		Status:                   database.PurchaseStatusNew,
		Amount:                   amount,
		Currency:                 "RUB",
		CustomerID:               customer.ID,
		SubscriptionID:           options.SubscriptionID,
		Month:                    months,
		PlanID:                   optionalTrimmedStringPointer(options.PlanID),
		TrafficLimitBytes:        options.TrafficLimitBytes,
		DeviceLimitCount:         options.DeviceLimitCount,
		AgreementAccepted:        options.AgreementAccepted,
		PurchaseKind:             options.PurchaseKind,
		ExtraDevices:             options.ExtraDevices,
		IsFreePlan:               options.IsFreePlan,
		FreePlanOneTime:          options.FreePlanOneTime,
		PromoCodeID:              options.PromoCodeID,
		PromoCodeSnapshot:        optionalTrimmedStringPointer(options.PromoCodeCode),
		PromoCodeDiscountPercent: optionalPositiveIntPointer(options.PromoDiscountPercent),
	})
	if err != nil {
		return "", 0, err
	}
	_, _, err = s.walletRepository.Apply(ctx, customer.ID, -amountCents, "purchase", fmt.Sprintf("balance-purchase:%d", purchaseID), "Оплата покупки с баланса")
	if err != nil {
		_ = s.purchaseRepository.UpdateFields(ctx, purchaseID, map[string]interface{}{"status": database.PurchaseStatusCancel})
		return "", purchaseID, err
	}
	if err := s.ProcessPurchaseById(ctx, purchaseID); err != nil {
		current, findErr := s.purchaseRepository.FindById(ctx, purchaseID)
		if findErr == nil && current != nil && current.Status != database.PurchaseStatusPaid {
			_, _, _ = s.walletRepository.Apply(ctx, customer.ID, amountCents, "purchase_refund", fmt.Sprintf("balance-purchase-refund:%d", purchaseID), "Возврат за неисполненную покупку")
			_ = s.purchaseRepository.UpdateFields(ctx, purchaseID, map[string]interface{}{"status": database.PurchaseStatusCancel})
		}
		return "", purchaseID, err
	}
	return "", purchaseID, nil
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
		SubscriptionID:           options.SubscriptionID,
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
		PurchaseKind:             options.PurchaseKind,
		ExtraDevices:             options.ExtraDevices,
		IsFreePlan:               options.IsFreePlan,
		FreePlanOneTime:          options.FreePlanOneTime,
		GiftRecipientUsername:    optionalTrimmedStringPointer(options.GiftRecipientUsername),
		GiftRecipientCustomerID:  options.GiftRecipientCustomerID,
		GiftToken:                options.GiftToken,
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
		SubscriptionID:           options.SubscriptionID,
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
		PurchaseKind:             options.PurchaseKind,
		ExtraDevices:             options.ExtraDevices,
		IsFreePlan:               options.IsFreePlan,
		FreePlanOneTime:          options.FreePlanOneTime,
		GiftRecipientUsername:    optionalTrimmedStringPointer(options.GiftRecipientUsername),
		GiftRecipientCustomerID:  options.GiftRecipientCustomerID,
		GiftToken:                options.GiftToken,
	})
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	yookasaClient := s.currentYookasaClient()
	if yookasaClient == nil {
		return "", 0, errors.New("YooKassa не настроена")
	}
	invoice, err := yookasaClient.CreateInvoice(ctx, int(amount), months, customer.ID, purchaseId, s.buildYookassaReturnURL(purchaseId, options.ReturnTarget))
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
	username, _ := ctx.Value("username").(string)
	created, err := s.integrationGateway.Create(ctx, integrations.CreatePaymentRequest{
		Provider: provider, PurchaseID: purchaseID, Amount: amount, Currency: "RUB",
		Description: fmt.Sprintf("Link-Bot: подписка на %d мес.", months), CustomerID: customer.ID,
		Username: strings.TrimSpace(username), ReturnURL: s.buildYookassaReturnURL(purchaseID, options.ReturnTarget),
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
		database.InvoiceTypeHeleket: integrations.ProviderHeleket, database.InvoiceTypePally: integrations.ProviderPally,
	}
	provider, ok := providers[invoiceType]
	return provider, ok
}

func invoiceTypeForIntegrationProvider(provider string) (database.InvoiceType, bool) {
	providers := map[string]database.InvoiceType{
		integrations.ProviderLava: database.InvoiceTypeLava, integrations.ProviderWata: database.InvoiceTypeWata,
		integrations.ProviderPlatega: database.InvoiceTypePlatega, integrations.ProviderFreeKassa: database.InvoiceTypeFreeKassa,
		integrations.ProviderHeleket: database.InvoiceTypeHeleket, integrations.ProviderPally: database.InvoiceTypePally,
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
	if !webhookAmountMatches(provider, event.Amount, purchase.Amount) {
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

func webhookAmountMatches(provider string, paidAmount, expectedAmount float64) bool {
	if paidAmount <= 0 {
		return true
	}
	if provider == integrations.ProviderPlatega {
		return paidAmount+0.01 >= expectedAmount
	}
	return math.Abs(paidAmount-expectedAmount) <= 0.01
}

func (s PaymentService) createTelegramInvoice(ctx context.Context, amount float64, months int, customer *database.Customer, options CreatePurchaseOptions) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:              database.InvoiceTypeTelegram,
		Status:                   database.PurchaseStatusNew,
		Amount:                   amount,
		Currency:                 "STARS",
		CustomerID:               customer.ID,
		SubscriptionID:           options.SubscriptionID,
		Month:                    months,
		PlanID:                   optionalTrimmedStringPointer(options.PlanID),
		TrafficLimitBytes:        options.TrafficLimitBytes,
		DeviceLimitCount:         options.DeviceLimitCount,
		AgreementAccepted:        options.AgreementAccepted,
		PromoCodeID:              options.PromoCodeID,
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
		InternalSquadUUIDs:       append([]string(nil), trial.InternalSquadUUIDs...),
		InternalSquadsConfigured: trial.InternalSquadsConfigured,
		ExternalSquadUUID:        trial.ExternalSquadUUID,
		TrafficResetStrategy:     trial.TrafficResetStrategy,
		Tag:                      trial.Tag,
		ApplySquads:              true,
		UsernameTemplate:         panelUsernameTemplate(s.runtimeSettings),
	})
	if err != nil {
		slog.Error("Error creating user", "error", err)
		return "", err
	}
	if s.subscriptionRepository != nil {
		primary, primaryErr := s.subscriptionRepository.EnsurePrimary(ctx, customer)
		if primaryErr != nil {
			return "", primaryErr
		}
		if primaryErr = s.subscriptionRepository.UpdatePanelState(ctx, primary, user.ID, user.UUID, user.SubscriptionURL, user.ExpireAt); primaryErr != nil {
			return "", primaryErr
		}
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
	if rewardErr := s.grantReferralReward(context.Background(), customer, "trial", nil); rewardErr != nil {
		slog.Error("payment: grant referral trial reward failed", "error", rewardErr, "customer_id", utils.MaskHalfInt64(customer.ID))
	}

	return user.GetSubscriptionUrl(), nil

}

func panelUsernameTemplate(settings *runtimeconfig.Service) string {
	if settings == nil {
		return ""
	}
	return settings.Snapshot().Panel.UsernameTemplate
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

func (s PaymentService) subscriptionForPurchase(ctx context.Context, customer *database.Customer, purchase *database.Purchase) (*database.CustomerSubscription, error) {
	if customer == nil {
		return nil, errors.New("customer is required")
	}
	if s.subscriptionRepository == nil {
		return &database.CustomerSubscription{
			CustomerID: customer.ID, DisplayName: "Основная", Position: 1, IsPrimary: true,
			SubscriptionLink: customer.SubscriptionLink, ExpireAt: customer.ExpireAt,
		}, nil
	}
	var subscription *database.CustomerSubscription
	var err error
	if purchase != nil && purchase.SubscriptionID != nil {
		subscription, err = s.subscriptionRepository.FindForCustomer(ctx, customer.ID, *purchase.SubscriptionID)
		if err != nil {
			return nil, err
		}
		if subscription == nil {
			return nil, database.ErrCustomerSubscriptionNotFound
		}
	} else {
		subscription, err = s.subscriptionRepository.EnsurePrimary(ctx, customer)
		if err != nil {
			return nil, err
		}
		if purchase != nil && subscription != nil {
			if err := s.purchaseRepository.UpdateFields(ctx, purchase.ID, map[string]interface{}{"subscription_id": subscription.ID}); err != nil {
				return nil, err
			}
			purchase.SubscriptionID = &subscription.ID
		}
	}
	return subscription, nil
}

func subscriptionPanelIdentity(subscription *database.CustomerSubscription) (int64, uuid.UUID) {
	if subscription == nil {
		return 0, uuid.Nil
	}
	userID := int64(0)
	if subscription.PanelUserID != nil {
		userID = *subscription.PanelUserID
	}
	userUUID := uuid.Nil
	if subscription.PanelUserUUID != nil {
		userUUID = *subscription.PanelUserUUID
	}
	return userID, userUUID
}

func (s PaymentService) panelStateForSubscription(ctx context.Context, customer *database.Customer, subscription *database.CustomerSubscription) (*remnawave.UserState, error) {
	userID, userUUID := subscriptionPanelIdentity(subscription)
	if userID > 0 || userUUID != uuid.Nil {
		return s.remnawaveClient.GetUserStateByIdentity(ctx, userID, userUUID)
	}
	if subscription != nil && subscription.IsPrimary {
		return s.remnawaveClient.GetUserStateByTelegramID(ctx, customer.TelegramID)
	}
	return nil, nil
}

func (s PaymentService) persistSubscriptionPanelState(ctx context.Context, customer *database.Customer, subscription *database.CustomerSubscription, user *remnawave.PanelUser) error {
	if user == nil {
		return errors.New("panel user is required")
	}
	if s.subscriptionRepository != nil && subscription != nil && subscription.ID > 0 {
		return s.subscriptionRepository.UpdatePanelState(ctx, subscription, user.ID, user.UUID, user.SubscriptionURL, user.ExpireAt)
	}
	return s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
		"subscription_link": user.SubscriptionURL,
		"expire_at":         user.ExpireAt,
	})
}

func customerForSubscription(customer *database.Customer, subscription *database.CustomerSubscription) *database.Customer {
	if customer == nil || subscription == nil || subscription.IsPrimary {
		return customer
	}
	copyCustomer := *customer
	copyCustomer.SubscriptionLink = subscription.SubscriptionLink
	copyCustomer.ExpireAt = subscription.ExpireAt
	return &copyCustomer
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

func buildReferralBonusGrantedText(langCode, eventType string, days, trafficGB int, balanceCents int64) string {
	eventRU := "покупку подписки приглашённым пользователем"
	eventEN := "your invited friend's subscription purchase"
	eventFA := "خرید اشتراک کاربر دعوت‌شده"
	if eventType == "trial" {
		eventRU = "активацию пробного периода приглашённым пользователем"
		eventEN = "your invited friend's trial activation"
		eventFA = "فعال‌سازی دوره آزمایشی کاربر دعوت‌شده"
	}
	rewardRU := referralRewardSummary(days, trafficGB, balanceCents, "дн.", "ГБ", "₽")
	rewardEN := referralRewardSummary(days, trafficGB, balanceCents, "days", "GB", "RUB")
	rewardFA := referralRewardSummary(days, trafficGB, balanceCents, "روز", "گیگابایت", "روبل")
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(langCode)), "en") {
		return fmt.Sprintf("You received a referral reward for %s: %s.", eventEN, rewardEN)
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(langCode)), "fa") {
		return fmt.Sprintf("پاداش دعوت برای %s ثبت شد: %s.", eventFA, rewardFA)
	}
	return fmt.Sprintf("<tg-emoji emoji-id='5258362837411045098'>☺️</tg-emoji> Вам начислена награда за %s: %s.", eventRU, rewardRU)
}

func referralRewardSummary(days, trafficGB int, balanceCents int64, daysUnit, trafficUnit, moneyUnit string) string {
	parts := make([]string, 0, 3)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("+%d %s", days, daysUnit))
	}
	if trafficGB > 0 {
		parts = append(parts, fmt.Sprintf("+%d %s", trafficGB, trafficUnit))
	}
	if balanceCents > 0 {
		parts = append(parts, fmt.Sprintf("+%.2f %s", float64(balanceCents)/100, moneyUnit))
	}
	return strings.Join(parts, ", ")
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

func (s PaymentService) buildYookassaReturnURL(purchaseID int64, returnTarget string) string {
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
	query.Set("returnTarget", normalizePaymentReturnTarget(returnTarget))
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func normalizePaymentReturnTarget(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "web", "browser":
		return "web"
	}
	return "telegram"
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
	language := s.language()
	if strings.HasPrefix(language, "en") {
		text = fmt.Sprintf("Auto payment succeeded. Your subscription was renewed for %d month(s).", months)
	} else if strings.HasPrefix(language, "fa") {
		text = fmt.Sprintf("پرداخت خودکار با موفقیت انجام شد. اشتراک شما برای %d ماه تمدید شد.", months)
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
	language := s.language()
	if strings.HasPrefix(language, "en") {
		text = "Auto payment failed. Check your payment method in the Payments section."
		if reason != "" {
			text = fmt.Sprintf("%s\n\n%s", text, reason)
		}
	} else if strings.HasPrefix(language, "fa") {
		text = "پرداخت خودکار انجام نشد. روش پرداخت را در بخش «پرداخت‌ها» بررسی کنید."
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
