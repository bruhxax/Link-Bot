package handler

import (
	"context"
	"time"

	cachepkg "link-bot/internal/cache"
	"link-bot/internal/cryptopay"
	"link-bot/internal/database"
	"link-bot/internal/operations"
	"link-bot/internal/payment"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
	"link-bot/internal/sync"
	"link-bot/internal/translation"
	"link-bot/internal/webauth"
	"link-bot/internal/yookasa"
)

type Handler struct {
	customerRepository   *database.CustomerRepository
	purchaseRepository   *database.PurchaseRepository
	cryptoPayClient      *cryptopay.Client
	yookasaClient        *yookasa.Client
	translation          *translation.Manager
	paymentService       *payment.PaymentService
	syncService          *sync.SyncService
	referralRepository   *database.ReferralRepository
	cache                *cachepkg.Cache
	channelSubCache      *cachepkg.Cache
	screenMessageCache   *cachepkg.Cache
	runtimeSettings      *runtimeconfig.Service
	errorReporter        *operations.Reporter
	webLogin             *webauth.Service
	pinnedMessageOnStart func(context.Context, int64) error
	inlineAdmin          InlineAdminService
	startSubscriptions   *database.SubscriptionRepository
	startPanel           *remnawave.Client
}

func (h *Handler) SetStartMenuDataSources(subscriptions *database.SubscriptionRepository, panel *remnawave.Client) {
	h.startSubscriptions = subscriptions
	h.startPanel = panel
}

type InlineAdminService interface {
	HasAdminPermission(context.Context, int64, ...string) bool
	ExtendInlineUser(context.Context, int64, int64, int64, time.Time) error
	CreditInlineUser(context.Context, int64, int64, int64, string) error
	FindInlinePromo(context.Context, int64, string) (string, error)
	SelectedInlineSubscriptionID(context.Context, int64) (int64, error)
}

func (h *Handler) SetInlineAdminService(service InlineAdminService) { h.inlineAdmin = service }

func (h *Handler) SetPinnedMessageOnStart(ensure func(context.Context, int64) error) {
	h.pinnedMessageOnStart = ensure
}

func NewHandler(
	syncService *sync.SyncService,
	paymentService *payment.PaymentService,
	translation *translation.Manager,
	customerRepository *database.CustomerRepository,
	purchaseRepository *database.PurchaseRepository,
	cryptoPayClient *cryptopay.Client,
	yookasaClient *yookasa.Client, referralRepository *database.ReferralRepository, cache *cachepkg.Cache,
	runtimeSettings *runtimeconfig.Service, errorReporter *operations.Reporter, webLogin *webauth.Service) *Handler {
	return &Handler{
		syncService:        syncService,
		paymentService:     paymentService,
		customerRepository: customerRepository,
		purchaseRepository: purchaseRepository,
		cryptoPayClient:    cryptoPayClient,
		yookasaClient:      yookasaClient,
		translation:        translation,
		referralRepository: referralRepository,
		cache:              cache,
		channelSubCache:    cachepkg.NewCache(20 * time.Second),
		screenMessageCache: cachepkg.NewCache(30 * 24 * time.Hour),
		runtimeSettings:    runtimeSettings,
		errorReporter:      errorReporter,
		webLogin:           webLogin,
	}
}
