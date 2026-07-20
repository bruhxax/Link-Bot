package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"link-bot/internal/broadcast"
	"link-bot/internal/cache"
	"link-bot/internal/config"
	"link-bot/internal/cryptopay"
	"link-bot/internal/database"
	"link-bot/internal/handler"
	"link-bot/internal/integrations"
	"link-bot/internal/miniapp"
	"link-bot/internal/moynalog"
	"link-bot/internal/notification"
	"link-bot/internal/operations"
	"link-bot/internal/payment"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
	"link-bot/internal/sync"
	"link-bot/internal/translation"
	"link-bot/internal/tribute"
	"link-bot/internal/yookasa"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/robfig/cron/v3"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	config.InitConfig()
	slog.Info("Application starting", "version", Version, "commit", Commit, "buildDate", BuildDate)

	// Check if Moynalog is enabled
	var moynalogClient *moynalog.Client
	if config.IsMoynalogEnabled() {
		var err error
		moynalogClient, err = moynalog.NewClient(config.MoynalogUrl(), config.MoynalogUsername(), config.MoynalogPassword())
		if err != nil {
			log.Fatalf("Moynalog initialization error: %v", err)
		}

		slog.Info("Moynalog authentication successful")
	}

	tm := translation.GetInstance()
	err := tm.InitTranslations("./translations", config.DefaultLanguage())
	if err != nil {
		panic(err)
	}

	pool, err := initDatabase(ctx, config.DadaBaseUrl())
	if err != nil {
		panic(err)
	}

	err = database.RunMigrations(ctx, &database.MigrationConfig{Direction: "up", MigrationsPath: "./db/migrations", Steps: 0}, pool)
	if err != nil {
		panic(err)
	}
	cache := cache.NewCache(30 * time.Minute)
	customerRepository := database.NewCustomerRepository(pool)
	purchaseRepository := database.NewPurchaseRepository(pool)
	promoCodeRepository := database.NewPromoCodeRepository(pool)
	referralRepository := database.NewReferralRepository(pool)
	supportRepository := database.NewSupportRepository(pool)
	reviewRepository := database.NewReviewRepository(pool)
	runtimeSettingsRepository := database.NewRuntimeSettingsRepository(pool)
	broadcastRepository := database.NewBroadcastRepository(pool)
	paymentIntegrationRepository := database.NewPaymentIntegrationRepository(pool)
	runtimeSettings, err := runtimeconfig.NewService(ctx, runtimeSettingsRepository)
	if err != nil {
		panic(err)
	}
	integrationSettings, err := integrations.NewService(ctx, paymentIntegrationRepository)
	if err != nil {
		panic(err)
	}

	cryptoPayClient := cryptopay.NewCryptoPayClient(config.CryptoPayUrl(), config.CryptoPayToken())
	remnawaveClient := remnawave.NewClient(config.RemnawaveUrl(), config.RemnawaveToken(), config.RemnawaveMode())
	yookasaClient := yookasa.NewClient(config.YookasaUrl(), config.YookasaShopId(), config.YookasaSecretKey())
	b, err := bot.New(config.TelegramToken(), bot.WithWorkers(3))
	if err != nil {
		panic(err)
	}
	errorReporter := operations.NewReporter(runtimeSettingsRepository, b, config.GetAdminTelegramId())
	broadcastService := broadcast.NewService(broadcastRepository, customerRepository, promoCodeRepository, b)
	if err := broadcastService.RecoverInterrupted(ctx); err != nil {
		slog.Warn("broadcast recovery failed", "error", err)
	}

	paymentService := payment.NewPaymentService(tm, purchaseRepository, promoCodeRepository, remnawaveClient, customerRepository, b, cryptoPayClient, yookasaClient, referralRepository, cache, moynalogClient, runtimeSettings, errorReporter, integrationSettings)

	cronScheduler := setupInvoiceChecker(customerRepository, purchaseRepository, paymentService)
	if cronScheduler != nil {
		cronScheduler.Start()
		defer cronScheduler.Stop()
	}

	subService := notification.NewSubscriptionService(customerRepository, purchaseRepository, paymentService, b, tm, runtimeSettings)
	syncService := sync.NewSyncService(remnawaveClient, customerRepository)

	subscriptionNotificationCronScheduler := subscriptionChecker(syncService, subService)
	subscriptionNotificationCronScheduler.Start()
	defer subscriptionNotificationCronScheduler.Stop()
	go runSubscriptionCheck(syncService, subService)

	h := handler.NewHandler(syncService, paymentService, tm, customerRepository, purchaseRepository, cryptoPayClient, yookasaClient, referralRepository, cache, runtimeSettings, errorReporter)
	miniAppHandler := miniapp.NewHandler(customerRepository, purchaseRepository, promoCodeRepository, referralRepository, supportRepository, reviewRepository, paymentService, remnawaveClient, b, broadcastService, subService, runtimeSettings, errorReporter, integrationSettings)
	operations.StartHealthMonitor(ctx, pool, remnawaveClient, errorReporter)

	me, err := b.GetMe(ctx)
	if err != nil {
		panic(err)
	}

	if config.GetMiniAppURL() != "" {
		_, err = b.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
			MenuButton: &models.MenuButtonWebApp{
				Type: models.MenuButtonTypeWebApp,
				Text: tm.GetText(config.DefaultLanguage(), "web_app_button_text"),
				WebApp: models.WebAppInfo{
					URL: config.GetMiniAppURL(),
				},
			},
		})
	} else {
		_, err = b.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
			MenuButton: &models.MenuButtonCommands{
				Type: models.MenuButtonTypeCommands,
			},
		})
	}

	// Set bot commands for Russian
	_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "start", Description: "Начать работу с ботом"},
			{Command: "connect", Description: "Подключиться"},
		},
		LanguageCode: "ru",
	})

	// Set bot commands for English
	_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "start", Description: "Start using the bot"},
			{Command: "connect", Description: "Connect"},
		},
		LanguageCode: "en",
	})

	config.SetBotURL(fmt.Sprintf("https://t.me/%s", me.Username))
	startPaymentNotificationBot(ctx, integrationSettings)

	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, h.StartCommandHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/connect", bot.MatchTypeExact, h.ConnectCommandHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/sync", bot.MatchTypeExact, h.SyncUsersCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/getfileid", bot.MatchTypePrefix, h.GetFileIDCommandHandler, isAdminMiddleware)

	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackCancelStarsUI, bot.MatchTypePrefix, h.CancelStarsUIHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackVerifyChannel, bot.MatchTypeExact, h.VerifyChannelSubscriptionCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackReferral, bot.MatchTypeExact, h.ReferralCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBuy, bot.MatchTypeExact, h.BuyCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackTrial, bot.MatchTypeExact, h.TrialCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackActivateTrial, bot.MatchTypeExact, h.ActivateTrialCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackStart, bot.MatchTypeExact, h.StartCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackSell, bot.MatchTypePrefix, h.SellCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackConnect, bot.MatchTypeExact, h.ConnectCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPayment, bot.MatchTypePrefix, h.PaymentCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		if update.Message == nil {
			return false
		}

		t := strings.TrimSpace(update.Message.Text)
		c := strings.TrimSpace(update.Message.Caption)

		// ловим /allcom и в тексте, и в подписи к медиа
		return (t != "" && (strings.HasPrefix(t, "/allcom") || strings.HasPrefix(t, "/allcom@"))) ||
			(c != "" && (strings.HasPrefix(c, "/allcom") || strings.HasPrefix(c, "/allcom@")))
	}, h.AllcomCommandHandler, isAdminMiddleware)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil && update.Message.From != nil &&
			update.Message.From.ID == config.GetAdminTelegramId()
	}, func(ctx context.Context, b *bot.Bot, update *models.Update) {
		captured, err := broadcastService.CaptureMessage(ctx, update.Message)
		if err != nil {
			slog.Error("broadcast message capture failed", "error", err)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: "Не удалось сохранить сообщение для рассылки. Попробуйте ещё раз."})
			return
		}
		if captured {
			slog.Info("broadcast source message captured", "messageId", update.Message.ID)
		}
	})

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.PreCheckoutQuery != nil
	}, h.PreCheckoutCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil && update.Message.SuccessfulPayment != nil
	}, h.SuccessPaymentHandler, h.SuspiciousUserFilterMiddleware)

	mux := http.NewServeMux()
	mux.Handle("/healthcheck", fullHealthHandler(pool, remnawaveClient))
	miniAppHandler.Register(mux)
	if config.GetTributeWebHookUrl() != "" {
		tributeHandler := tribute.NewClient(paymentService, customerRepository)
		mux.Handle(config.GetTributeWebHookUrl(), tributeHandler.WebHookHandler())
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.GetHealthCheckPort()),
		Handler: mux,
	}
	go func() {
		log.Printf("Server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	slog.Info("Bot is starting...")
	b.Start(ctx)

	log.Println("Shutting down health server…")
	shutdownCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Health server shutdown error: %v", err)
	}
}

func fullHealthHandler(pool *pgxpool.Pool, rw *remnawave.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := map[string]string{
			"status":    "ok",
			"db":        "ok",
			"rw":        "ok",
			"time":      time.Now().Format(time.RFC3339),
			"version":   Version,
			"commit":    Commit,
			"buildDate": BuildDate,
		}

		dbCtx, dbCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer dbCancel()
		if err := pool.Ping(dbCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			status["status"] = "fail"
			status["db"] = "error: " + err.Error()
		}

		rwCtx, rwCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer rwCancel()
		if err := rw.Ping(rwCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			status["status"] = "fail"
			status["rw"] = "error: " + err.Error()
		}

		if status["status"] == "ok" {
			w.WriteHeader(http.StatusOK)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"%s","db":"%s","remnawave":"%s","time":"%s","version":"%s","commit":"%s","buildDate":"%s"}`,
			status["status"], status["db"], status["rw"], status["time"], Version, Commit, BuildDate)
	})
}

func isAdminMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message != nil && update.Message.From.ID == config.GetAdminTelegramId() {
			next(ctx, b, update)
		} else {
			return
		}
	}
}

func startPaymentNotificationBot(ctx context.Context, settings *integrations.Service) {
	if settings == nil {
		return
	}
	notificationConfig, ok := settings.Config(integrations.ProviderNotificationBot)
	if !ok {
		return
	}

	token := strings.TrimSpace(notificationConfig["token"])
	if token == "" {
		return
	}
	allowedChatID, _ := strconv.ParseInt(strings.TrimSpace(notificationConfig["chatId"]), 10, 64)
	if allowedChatID == 0 {
		allowedChatID = config.GetAdminTelegramId()
	}
	timezone := strings.TrimSpace(notificationConfig["timezone"])
	if timezone == "" {
		timezone = "Europe/Moscow"
	}

	notificationBot, err := bot.New(token, bot.WithWorkers(1))
	if err != nil {
		slog.Error("payment notification bot initialization failed", "error", err)
		return
	}

	_, err = notificationBot.DeleteWebhook(ctx, &bot.DeleteWebhookParams{DropPendingUpdates: true})
	if err != nil {
		slog.Warn("payment notification bot webhook cleanup failed", "error", err)
	}

	commands := []models.BotCommand{
		{Command: "ping", Description: "Проверить уведомления"},
	}
	if _, err = notificationBot.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: commands}); err != nil {
		slog.Warn("payment notification bot default command setup failed", "error", err)
	}
	if _, err = notificationBot.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: commands, LanguageCode: "ru"}); err != nil {
		slog.Warn("payment notification bot ru command setup failed", "error", err)
	}

	pingHandler := func(ctx context.Context, b *bot.Bot, update *models.Update) {
		paymentNotificationPingHandler(ctx, b, update, allowedChatID, timezone)
	}
	notificationBot.RegisterHandler(bot.HandlerTypeMessageText, "/ping", bot.MatchTypeExact, pingHandler)
	notificationBot.RegisterHandler(bot.HandlerTypeMessageText, "/ping@", bot.MatchTypePrefix, pingHandler)

	go func() {
		slog.Info("Payment notification bot is starting")
		notificationBot.Start(ctx)
		slog.Info("Payment notification bot stopped")
	}()
}

func paymentNotificationPingHandler(ctx context.Context, b *bot.Bot, update *models.Update, allowedChatID int64, timezone string) {
	if update.Message == nil {
		return
	}

	if update.Message.Chat.ID != allowedChatID {
		slog.Warn("payment notification bot rejected ping", "chat_id", update.Message.Chat.ID)
		return
	}

	now := time.Now()
	if location, locationErr := time.LoadLocation(timezone); locationErr == nil {
		now = now.In(location)
	} else {
		slog.Warn("invalid payment notification timezone, using local timezone", "timezone", timezone, "error", locationErr)
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		ParseMode: models.ParseModeHTML,
		Text: fmt.Sprintf(
			"✅ <b>Уведомления работают</b>\n\n"+
				"🤖 Второй бот на связи\n"+
				"🕒 Время сервера: <b>%s</b>",
			now.Format("02.01.2006 15:04:05 MST"),
		),
	})
	if err != nil {
		slog.Error("payment notification bot ping response failed", "error", err)
	}
}

func subscriptionChecker(syncService *sync.SyncService, subService *notification.SubscriptionService) *cron.Cron {
	c := cron.New(
		cron.WithSeconds(),
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
	)

	_, err := c.AddFunc("0 * * * * *", func() {
		runSubscriptionCheck(syncService, subService)
	})

	if err != nil {
		panic(err)
	}
	return c
}

func runSubscriptionCheck(syncService *sync.SyncService, subService *notification.SubscriptionService) {
	if err := syncService.RefreshSubscriptionState(); err != nil {
		slog.Error("Error refreshing subscriptions before notification check", "error", err)
		return
	}
	if err := subService.ProcessSubscriptionExpiration(); err != nil {
		slog.Error("Error sending subscription notifications", "error", err)
	}
}

func initDatabase(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	config.MaxConns = 20
	config.MinConns = 5

	return pgxpool.ConnectConfig(ctx, config)
}

func setupInvoiceChecker(
	customerRepository *database.CustomerRepository,
	purchaseRepository *database.PurchaseRepository,
	paymentService *payment.PaymentService) *cron.Cron {
	c := cron.New(cron.WithSeconds())

	_, err := c.AddFunc("*/5 * * * * *", func() {
		ctx := context.Background()
		checkCryptoPayInvoice(ctx, purchaseRepository, paymentService)
	})

	if err != nil {
		panic(err)
	}

	_, err = c.AddFunc("*/5 * * * * *", func() {
		ctx := context.Background()
		checkYookasaInvoice(ctx, purchaseRepository, paymentService)
	})

	if err != nil {
		panic(err)
	}

	if config.IsYookasaEnabled() && config.EnableAutoPayment() {
		_, err := c.AddFunc("15 * * * * *", func() {
			ctx := context.Background()
			checkAutoPayments(ctx, customerRepository, paymentService)
		})

		if err != nil {
			panic(err)
		}
	}

	return c
}

func checkYookasaInvoice(
	ctx context.Context,
	purchaseRepository *database.PurchaseRepository,
	paymentService *payment.PaymentService,
) {
	if !paymentService.IsProviderEnabled(integrations.ProviderYooKassa) {
		return
	}

	pendingPurchases, err := purchaseRepository.FindByInvoiceTypeAndStatus(
		ctx,
		database.InvoiceTypeYookasa,
		database.PurchaseStatusPending,
	)
	if err != nil {
		log.Printf("Error finding pending purchases: %v", err)
		return
	}
	if len(*pendingPurchases) == 0 {
		return
	}

	for _, purchase := range *pendingPurchases {
		status, err := paymentService.SyncYookassaPurchaseStatus(ctx, purchase.ID)
		if err != nil {
			slog.Error("Error syncing YooKassa invoice", "purchaseId", purchase.ID, "invoiceId", purchase.YookasaID, "error", err)
			continue
		}

		if status != database.PurchaseStatusPending && status != database.PurchaseStatusNew {
			slog.Info("YooKassa invoice synced", "purchaseId", purchase.ID, "invoiceId", purchase.YookasaID, "status", status)
		}
	}
}

func checkAutoPayments(
	ctx context.Context,
	customerRepository *database.CustomerRepository,
	paymentService *payment.PaymentService,
) {
	customers, err := customerRepository.FindAutoPaymentEligible(ctx, time.Now().UTC())
	if err != nil {
		slog.Error("Error finding auto payment customers", "error", err)
		return
	}

	for i := range customers {
		customer := customers[i]
		if err := paymentService.ProcessAutoPayment(ctx, &customer); err != nil {
			slog.Error("Error processing auto payment", "customerId", customer.ID, "telegramId", customer.TelegramID, "error", err)
		}
	}
}

func checkCryptoPayInvoice(
	ctx context.Context,
	purchaseRepository *database.PurchaseRepository,
	paymentService *payment.PaymentService,
) {
	if !paymentService.IsProviderEnabled(integrations.ProviderCryptoPay) {
		return
	}
	cryptoPayClient := paymentService.CurrentCryptoPayClient()
	if cryptoPayClient == nil {
		return
	}

	pendingPurchases, err := purchaseRepository.FindByInvoiceTypeAndStatus(
		ctx,
		database.InvoiceTypeCrypto,
		database.PurchaseStatusPending,
	)
	if err != nil {
		log.Printf("Error finding pending purchases: %v", err)
		return
	}
	if len(*pendingPurchases) == 0 {
		return
	}

	var invoiceIDs []string

	for _, purchase := range *pendingPurchases {
		if purchase.CryptoInvoiceID != nil {
			invoiceIDs = append(invoiceIDs, fmt.Sprintf("%d", *purchase.CryptoInvoiceID))
		}
	}

	if len(invoiceIDs) == 0 {
		return
	}

	stringInvoiceIDs := strings.Join(invoiceIDs, ",")
	invoices, err := cryptoPayClient.GetInvoices("", "", "", stringInvoiceIDs, 0, 0)
	if err != nil {
		log.Printf("Error getting invoices: %v", err)
		return
	}

	for _, invoice := range *invoices {
		if invoice.InvoiceID != nil && invoice.IsPaid() {
			payload := strings.Split(invoice.Payload, "&")
			purchaseID, err := strconv.Atoi(strings.Split(payload[0], "=")[1])
			username := strings.Split(payload[1], "=")[1]
			ctxWithUsername := context.WithValue(ctx, "username", username)
			err = paymentService.ProcessPurchaseById(ctxWithUsername, int64(purchaseID))
			if err != nil {
				slog.Error("Error processing invoice", "invoiceId", invoice.InvoiceID, "error", err)
			} else {
				slog.Info("Invoice processed", "invoiceId", invoice.InvoiceID, "purchaseId", purchaseID)
			}

		}
	}

}
