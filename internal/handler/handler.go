package handler

import (
	"time"

	cachepkg "link-bot/internal/cache"
	"link-bot/internal/cryptopay"
	"link-bot/internal/database"
	"link-bot/internal/operations"
	"link-bot/internal/payment"
	"link-bot/internal/runtimeconfig"
	"link-bot/internal/sync"
	"link-bot/internal/translation"
	"link-bot/internal/yookasa"
)

type Handler struct {
	customerRepository *database.CustomerRepository
	purchaseRepository *database.PurchaseRepository
	cryptoPayClient    *cryptopay.Client
	yookasaClient      *yookasa.Client
	translation        *translation.Manager
	paymentService     *payment.PaymentService
	syncService        *sync.SyncService
	referralRepository *database.ReferralRepository
	cache              *cachepkg.Cache
	channelSubCache    *cachepkg.Cache
	screenMessageCache *cachepkg.Cache
	runtimeSettings    *runtimeconfig.Service
	errorReporter      *operations.Reporter
}

func NewHandler(
	syncService *sync.SyncService,
	paymentService *payment.PaymentService,
	translation *translation.Manager,
	customerRepository *database.CustomerRepository,
	purchaseRepository *database.PurchaseRepository,
	cryptoPayClient *cryptopay.Client,
	yookasaClient *yookasa.Client, referralRepository *database.ReferralRepository, cache *cachepkg.Cache,
	runtimeSettings *runtimeconfig.Service, errorReporter *operations.Reporter) *Handler {
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
	}
}
