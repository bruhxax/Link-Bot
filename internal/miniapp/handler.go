package miniapp

import (
	"bytes"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"

	"link-bot/internal/broadcast"
	"link-bot/internal/cache"
	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/integrations"
	"link-bot/internal/notification"
	"link-bot/internal/operations"
	"link-bot/internal/payment"
	planbook "link-bot/internal/plans"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
	"link-bot/utils"
)

//go:embed static/* static/assets/*
var embeddedStatic embed.FS

var (
	promoPurchaseLocks          sync.Map
	adminSubscriptionRebindLock sync.Mutex
)

type Handler struct {
	customerRepository  *database.CustomerRepository
	purchaseRepository  *database.PurchaseRepository
	promoCodeRepository *database.PromoCodeRepository
	referralRepository  *database.ReferralRepository
	supportRepository   *database.SupportRepository
	reviewRepository    *database.ReviewRepository
	paymentService      *payment.PaymentService
	remnawaveClient     *remnawave.Client
	telegramBot         *bot.Bot
	staticFS            fs.FS
	rateLimiter         *requestRateLimiter
	channelSubCache     *cache.Cache
	runtimeSettings     *runtimeconfig.Service
	errorReporter       *operations.Reporter
	broadcastService    *broadcast.Service
	subscriptionService *notification.SubscriptionService
	integrationSettings *integrations.Service
}

func lockPromoPurchase(code string) func() {
	normalized := database.NormalizePromoCode(code)
	if normalized == "" {
		return func() {}
	}

	actual, _ := promoPurchaseLocks.LoadOrStore(normalized, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

type authRequest struct {
	InitData      string `json:"initData"`
	LoginData     string `json:"loginData"`
	GoogleIDToken string `json:"googleIdToken"`
}

type bootstrapResponse struct {
	Brand          brandPayload           `json:"brand"`
	User           userPayload            `json:"user"`
	Subscription   subscriptionPayload    `json:"subscription"`
	Trial          trialPayload           `json:"trial"`
	Referral       referralPayload        `json:"referral"`
	Reviews        reviewsPayload         `json:"reviews"`
	Servers        serversPayload         `json:"servers"`
	Support        supportPayload         `json:"support"`
	Payments       paymentsPayload        `json:"payments"`
	Admin          *adminPayload          `json:"admin,omitempty"`
	Plans          []planPayload          `json:"plans"`
	PaymentMethods []paymentMethodPayload `json:"paymentMethods"`
	Links          linksPayload           `json:"links"`
	Meta           metaPayload            `json:"meta"`
	Runtime        runtimeconfig.Settings `json:"runtime"`
}

type brandPayload struct {
	Name    string `json:"name"`
	LogoURL string `json:"logoUrl"`
}

type userPayload struct {
	ID             int64  `json:"id"`
	FirstName      string `json:"firstName"`
	Username       string `json:"username"`
	PanelUsername  string `json:"panelUsername,omitempty"`
	PhotoURL       string `json:"photoUrl,omitempty"`
	LanguageCode   string `json:"languageCode"`
	AuthProvider   string `json:"authProvider"`
	GoogleEmail    string `json:"googleEmail,omitempty"`
	GoogleLinked   bool   `json:"googleLinked"`
	TelegramLinked bool   `json:"telegramLinked"`
}

type subscriptionPayload struct {
	Status            string          `json:"status"`
	DaysLeft          int             `json:"daysLeft"`
	PlanMonths        int             `json:"planMonths,omitempty"`
	PlanLabel         string          `json:"planLabel,omitempty"`
	IsTrial           bool            `json:"isTrial,omitempty"`
	UserUUID          string          `json:"userUuid,omitempty"`
	ExpiresAt         string          `json:"expiresAt,omitempty"`
	SubscriptionLink  string          `json:"subscriptionLink,omitempty"`
	HasAccessLink     bool            `json:"hasAccessLink"`
	TrafficUsedBytes  int64           `json:"trafficUsedBytes"`
	TrafficLimitBytes int64           `json:"trafficLimitBytes"`
	DeviceUsedCount   int             `json:"deviceUsedCount"`
	DeviceLimitCount  int             `json:"deviceLimitCount"`
	Devices           []devicePayload `json:"devices"`
}

type devicePayload struct {
	Hwid        string `json:"hwid"`
	Platform    string `json:"platform,omitempty"`
	OSVersion   string `json:"osVersion,omitempty"`
	DeviceModel string `json:"deviceModel,omitempty"`
	UserAgent   string `json:"userAgent,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

type trialPayload struct {
	Enabled  bool `json:"enabled"`
	Eligible bool `json:"eligible"`
	Days     int  `json:"days"`
}

type referralPayload struct {
	Enabled           bool   `json:"enabled"`
	Count             int    `json:"count"`
	BonusDays         int    `json:"bonusDays"`
	BonusTrafficBytes int64  `json:"bonusTrafficBytes"`
	ShareURL          string `json:"shareUrl,omitempty"`
}

type reviewsPayload struct {
	Count              int                 `json:"count"`
	Average            float64             `json:"average"`
	CanCreate          bool                `json:"canCreate"`
	RewardDays         int                 `json:"rewardDays"`
	RewardTrafficBytes int64               `json:"rewardTrafficBytes"`
	Items              []reviewItemPayload `json:"items"`
	MyReview           *reviewItemPayload  `json:"myReview,omitempty"`
}

type reviewItemPayload struct {
	ID            int64  `json:"id"`
	Username      string `json:"username"`
	Rating        int    `json:"rating"`
	Comment       string `json:"comment"`
	CreatedAt     string `json:"createdAt"`
	RewardGranted bool   `json:"rewardGranted,omitempty"`
	IsMine        bool   `json:"isMine,omitempty"`
}

type serversPayload struct {
	Items []serverNodePayload `json:"items"`
}

type paymentsPayload struct {
	Enabled               bool                       `json:"enabled"`
	HasPaymentMethod      bool                       `json:"hasPaymentMethod"`
	AutoPaymentEnabled    bool                       `json:"autoPaymentEnabled"`
	AutoPaymentPlanMonths int                        `json:"autoPaymentPlanMonths,omitempty"`
	Method                *savedPaymentMethodPayload `json:"method,omitempty"`
	History               []paymentHistoryPayload    `json:"history"`
}

type adminPayload struct {
	PromoCodes   []promoCodePayload          `json:"promoCodes"`
	Settings     runtimeconfig.Settings      `json:"settings"`
	Events       []database.OperationalEvent `json:"events"`
	Integrations []integrations.ProviderView `json:"integrations"`
	Squads       remnawave.SquadCatalog      `json:"squads"`
}

type adminSubscriptionPayload struct {
	ID                int64  `json:"id"`
	UserUUID          string `json:"userUuid"`
	Username          string `json:"username"`
	CurrentTelegramID *int64 `json:"currentTelegramId,omitempty"`
	Status            string `json:"status"`
	ExpiresAt         string `json:"expiresAt,omitempty"`
	SubscriptionLink  string `json:"subscriptionLink,omitempty"`
}

type promoCodePayload struct {
	ID              int64  `json:"id"`
	Code            string `json:"code"`
	DiscountPercent int    `json:"discountPercent"`
	ExpiresAt       string `json:"expiresAt,omitempty"`
	MaxRedemptions  int    `json:"maxRedemptions,omitempty"`
	RedemptionCount int    `json:"redemptionCount"`
	Status          string `json:"status"`
	CreatedAt       string `json:"createdAt"`
}

type savedPaymentMethodPayload struct {
	Title   string `json:"title"`
	Type    string `json:"type,omitempty"`
	SavedAt string `json:"savedAt,omitempty"`
	Demo    bool   `json:"demo,omitempty"`
}

type paymentHistoryPayload struct {
	ID                 int64   `json:"id"`
	Months             int     `json:"months"`
	PlanLabel          string  `json:"planLabel"`
	Amount             float64 `json:"amount"`
	Currency           string  `json:"currency"`
	Status             string  `json:"status"`
	InvoiceType        string  `json:"invoiceType"`
	PaymentMethodTitle string  `json:"paymentMethodTitle,omitempty"`
	IsAutoPayment      bool    `json:"isAutoPayment"`
	CreatedAt          string  `json:"createdAt"`
	PaidAt             string  `json:"paidAt,omitempty"`
}

type serverNodePayload struct {
	Name        string `json:"name"`
	Address     string `json:"address,omitempty"`
	CountryCode string `json:"countryCode,omitempty"`
	Online      bool   `json:"online"`
}

type paymentMethodPayload struct {
	ID string `json:"id"`
}

type supportPayload struct {
	IsAdmin        bool                   `json:"isAdmin"`
	OpenTickets    []supportTicketPayload `json:"openTickets"`
	HistoryTickets []supportTicketPayload `json:"historyTickets"`
}

type supportTicketPayload struct {
	ID                int64  `json:"id"`
	Subject           string `json:"subject"`
	Preview           string `json:"preview"`
	Status            string `json:"status"`
	UpdatedAt         string `json:"updatedAt"`
	CreatedAt         string `json:"createdAt"`
	UnreadCount       int    `json:"unreadCount"`
	CustomerName      string `json:"customerName,omitempty"`
	CustomerUsername  string `json:"customerUsername,omitempty"`
	SubscriptionLabel string `json:"subscriptionLabel,omitempty"`
}

type supportMessagePayload struct {
	ID         int64  `json:"id"`
	AuthorRole string `json:"authorRole"`
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
}

type supportThreadPayload struct {
	Ticket   supportTicketPayload    `json:"ticket"`
	Messages []supportMessagePayload `json:"messages"`
	CanReply bool                    `json:"canReply"`
	CanClose bool                    `json:"canClose"`
}

type planPayload struct {
	ID                string `json:"id"`
	Months            int    `json:"months"`
	PriceRub          int    `json:"priceRub"`
	PriceStars        int    `json:"priceStars"`
	TrafficLimitBytes int64  `json:"trafficLimitBytes"`
	DeviceLimitCount  int    `json:"deviceLimitCount"`
	Variant           string `json:"variant,omitempty"`
	SavingsPercent    int    `json:"savingsPercent"`
	Recommended       bool   `json:"recommended"`
	Wide              bool   `json:"wide,omitempty"`
	TitleRU           string `json:"titleRu,omitempty"`
	TitleEN           string `json:"titleEn,omitempty"`
}

type linksPayload struct {
	Support string `json:"support,omitempty"`
	Channel string `json:"channel,omitempty"`
}

type metaPayload struct {
	Now                    string `json:"now"`
	BotURL                 string `json:"botUrl,omitempty"`
	MiniAppURL             string `json:"miniAppUrl,omitempty"`
	GoogleClientID         string `json:"googleClientId,omitempty"`
	StarsNeedPriorPurchase bool   `json:"starsNeedPriorPurchase"`
}

type purchaseRequest struct {
	PlanID            string `json:"planId,omitempty"`
	Months            int    `json:"months"`
	PaymentMethod     string `json:"paymentMethod"`
	AgreementAccepted bool   `json:"agreementAccepted"`
	PromoCode         string `json:"promoCode,omitempty"`
}

type purchaseActionRequest struct {
	PurchaseID int64 `json:"purchaseId"`
}

type purchaseResponse struct {
	Action     string `json:"action"`
	URL        string `json:"url"`
	PurchaseID int64  `json:"purchaseId"`
}

type purchaseActionResponse struct {
	Status string `json:"status"`
}

type supportCreateRequest struct {
	Subject string `json:"subject"`
	Message string `json:"message"`
}

type supportTicketRequest struct {
	TicketID int64 `json:"ticketId"`
}

type supportSendRequest struct {
	TicketID int64  `json:"ticketId"`
	Message  string `json:"message"`
}

type deviceDeleteRequest struct {
	UserUUID string `json:"userUuid"`
	Hwid     string `json:"hwid"`
}

type autoPaymentToggleRequest struct {
	Enabled bool `json:"enabled"`
}

type reviewCreateRequest struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

type reviewDeleteRequest struct {
	ID int64 `json:"id"`
}

type promoCodeApplyRequest struct {
	Code string `json:"code"`
}

type promoCodeCreateRequest struct {
	Code            string `json:"code"`
	DiscountPercent int    `json:"discountPercent"`
	ExpiresAt       string `json:"expiresAt"`
	MaxRedemptions  int    `json:"maxRedemptions"`
}

type promoCodeDeleteRequest struct {
	ID int64 `json:"id"`
}

type adminSubscriptionFindRequest struct {
	Query string `json:"query"`
}

type adminSubscriptionRebindRequest struct {
	UserUUID         string `json:"userUuid"`
	TargetTelegramID int64  `json:"targetTelegramId"`
}

type adminSettingsUpdateRequest struct {
	Settings runtimeconfig.Settings `json:"settings"`
}

type adminEventResolveRequest struct {
	ID int64 `json:"id"`
}

type adminIntegrationUpdateRequest struct {
	Provider string            `json:"provider"`
	Enabled  bool              `json:"enabled"`
	Fields   map[string]string `json:"fields"`
}

type googleLinkRequest struct {
	GoogleIDToken string `json:"googleIdToken"`
}

const (
	reviewRewardDays         = 2
	reviewRewardTrafficBytes = int64(20 * 1024 * 1024 * 1024)
	reviewListLimit          = 100
)

func NewHandler(
	customerRepository *database.CustomerRepository,
	purchaseRepository *database.PurchaseRepository,
	promoCodeRepository *database.PromoCodeRepository,
	referralRepository *database.ReferralRepository,
	supportRepository *database.SupportRepository,
	reviewRepository *database.ReviewRepository,
	paymentService *payment.PaymentService,
	remnawaveClient *remnawave.Client,
	telegramBot *bot.Bot,
	broadcastService *broadcast.Service,
	subscriptionService *notification.SubscriptionService,
	runtimeSettings *runtimeconfig.Service,
	errorReporter *operations.Reporter,
	integrationSettings *integrations.Service,
) *Handler {
	staticFS, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		panic(err)
	}

	return &Handler{
		customerRepository:  customerRepository,
		purchaseRepository:  purchaseRepository,
		promoCodeRepository: promoCodeRepository,
		referralRepository:  referralRepository,
		supportRepository:   supportRepository,
		reviewRepository:    reviewRepository,
		paymentService:      paymentService,
		remnawaveClient:     remnawaveClient,
		telegramBot:         telegramBot,
		broadcastService:    broadcastService,
		subscriptionService: subscriptionService,
		staticFS:            staticFS,
		rateLimiter:         newRequestRateLimiter(),
		channelSubCache:     cache.NewCache(30 * time.Minute),
		runtimeSettings:     runtimeSettings,
		errorReporter:       errorReporter,
		integrationSettings: integrationSettings,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	fileServer := http.FileServer(http.FS(h.staticFS))

	mux.HandleFunc("/", h.serveRoot)
	mux.HandleFunc("/mini-app", h.serveIndex)
	mux.HandleFunc("/mini-app/payment-return", h.handlePaymentReturnRedirect)
	mux.HandleFunc("/mini-app/google/callback", h.serveGoogleLinkCallback)
	mux.HandleFunc("/mini-app/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mini-app/" {
			h.serveIndex(w, r)
			return
		}

		setStaticHeaders(w, r)
		r.URL.Path = path.Clean(strings.TrimPrefix(r.URL.Path, "/mini-app"))
		fileServer.ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/mini-app/public-config", h.handlePublicConfig)
	mux.HandleFunc("/api/mini-app/bootstrap", h.withSession(h.handleBootstrap))
	mux.HandleFunc("/api/mini-app/auth/google/link/start", h.withSession(h.handleStartGoogleLink))
	mux.HandleFunc("/api/mini-app/auth/google/link/complete", h.handleCompleteGoogleLink)
	mux.HandleFunc("/api/mini-app/auth/google/link", h.withSession(h.handleLinkGoogle))
	mux.HandleFunc("/api/mini-app/trial/activate", h.withSession(h.handleActivateTrial))
	mux.HandleFunc("/api/mini-app/purchase", h.withSession(h.handleCreatePurchaseV2))
	mux.HandleFunc("/api/mini-app/purchase/cancel", h.withSession(h.handleCancelPurchase))
	mux.HandleFunc("/api/mini-app/promocode/apply", h.withSession(h.handleApplyPromoCode))
	mux.HandleFunc("/api/mini-app/payments/autopay", h.withSession(h.handleToggleAutoPayment))
	mux.HandleFunc("/api/mini-app/payments/remove-method", h.withSession(h.handleRemovePaymentMethod))
	mux.HandleFunc("/api/mini-app/devices/delete", h.withSession(h.handleDeleteDeviceExact))
	mux.HandleFunc("/api/mini-app/reviews/create", h.withSession(h.handleCreateReview))
	mux.HandleFunc("/api/mini-app/admin/reviews/delete", h.withSession(h.handleAdminDeleteReview))
	mux.HandleFunc("/api/mini-app/admin/promocodes/create", h.withSession(h.handleAdminCreatePromoCode))
	mux.HandleFunc("/api/mini-app/admin/promocodes/delete", h.withSession(h.handleAdminDeletePromoCode))
	mux.HandleFunc("/api/mini-app/admin/subscriptions/find", h.withSession(h.handleAdminFindSubscription))
	mux.HandleFunc("/api/mini-app/admin/subscriptions/rebind", h.withSession(h.handleAdminRebindSubscription))
	mux.HandleFunc("/api/mini-app/admin/settings/update", h.withSession(h.handleAdminSettingsUpdate))
	mux.HandleFunc("/api/mini-app/admin/reminders/test", h.withSession(h.handleAdminReminderTest))
	mux.HandleFunc("/api/mini-app/admin/success/test", h.withSession(h.handleAdminSuccessTest))
	mux.HandleFunc("/api/mini-app/admin/events/resolve", h.withSession(h.handleAdminEventResolve))
	mux.HandleFunc("/api/mini-app/admin/integrations/update", h.withSession(h.handleAdminIntegrationUpdate))
	mux.HandleFunc("/api/mini-app/admin/broadcast/state", h.withSession(h.handleAdminBroadcastState))
	mux.HandleFunc("/api/mini-app/admin/broadcast/capture/start", h.withSession(h.handleAdminBroadcastCaptureStart))
	mux.HandleFunc("/api/mini-app/admin/broadcast/buttons", h.withSession(h.handleAdminBroadcastButtons))
	mux.HandleFunc("/api/mini-app/admin/broadcast/preview", h.withSession(h.handleAdminBroadcastPreview))
	mux.HandleFunc("/api/mini-app/admin/broadcast/send", h.withSession(h.handleAdminBroadcastSend))
	mux.HandleFunc("/api/mini-app/admin/broadcast/reset", h.withSession(h.handleAdminBroadcastReset))
	mux.HandleFunc("/api/mini-app/support/refresh", h.withSession(h.handleSupportRefresh))
	mux.HandleFunc("/api/mini-app/support/create", h.withSession(h.handleSupportCreate))
	mux.HandleFunc("/api/mini-app/support/thread", h.withSession(h.handleSupportThread))
	mux.HandleFunc("/api/mini-app/support/send", h.withSession(h.handleSupportSend))
	mux.HandleFunc("/api/mini-app/support/close", h.withSession(h.handleSupportClose))
	mux.HandleFunc("/api/payments/webhook/", h.handlePaymentIntegrationWebhook)
}

func (h *Handler) handlePublicConfig(w http.ResponseWriter, r *http.Request) {
	setAPIHeaders(w)
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	settings := runtimeconfig.DefaultSettings()
	if h.runtimeSettings != nil {
		settings = h.runtimeSettings.Snapshot()
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": settings})
}

func (h *Handler) serveRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, "/mini-app/", http.StatusFound)
}

func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(h.staticFS, "index.html")
	if err != nil {
		http.Error(w, "mini app is unavailable", http.StatusInternalServerError)
		return
	}

	setHTMLSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	data = bytes.ReplaceAll(data, []byte("__TELEGRAM_BOT_USERNAME__"), []byte(html.EscapeString(telegramBotUsername())))
	data = bytes.ReplaceAll(data, []byte("__TELEGRAM_BOT_ID__"), []byte(html.EscapeString(telegramBotID())))
	data = bytes.ReplaceAll(data, []byte("__GOOGLE_CLIENT_ID__"), []byte(html.EscapeString(config.GoogleClientID())))
	_, _ = w.Write(data)
}

func (h *Handler) withSession(next func(http.ResponseWriter, *http.Request, *session, *database.Customer)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setAPIHeaders(w)

		if r.Method != http.MethodPost {
			h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
			return
		}
		if contentType := strings.TrimSpace(r.Header.Get("Content-Type")); contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
			h.writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "JSON required")
			return
		}

		initData, loginData, googleToken, err := extractAuthData(r)
		if err != nil {
			slog.Warn("mini app: failed to extract auth data", "error", err)
			h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
			return
		}

		var sess *session
		var customer *database.Customer
		if strings.TrimSpace(googleToken) != "" && strings.TrimSpace(initData) == "" && strings.TrimSpace(loginData) == "" {
			if h.runtimeSettings != nil && !h.runtimeSettings.FeatureEnabled("google") {
				h.writeError(w, http.StatusServiceUnavailable, "feature_disabled", "Gmail login is temporarily unavailable")
				return
			}
			sess, customer, err = h.sessionFromGoogleLogin(r.Context(), googleToken)
		} else {
			if strings.TrimSpace(initData) != "" {
				sess, err = parseAndValidateInitData(initData, config.TelegramToken())
			} else {
				sess, err = parseAndValidateLoginData(loginData, config.TelegramToken())
			}
		}
		if err != nil {
			slog.Warn("mini app: auth validation failed", "error", err, "path", r.URL.Path)
			if errors.Is(err, errGoogleAuthNotConfigured) {
				h.writeError(w, http.StatusServiceUnavailable, "google_not_configured", "Gmail login is not configured")
				return
			}
			if errors.Is(err, errGoogleAuthNotLinked) {
				h.writeError(w, http.StatusUnauthorized, "google_not_linked", "Link Gmail in the mini app first")
				return
			}
			h.writeError(w, http.StatusUnauthorized, "unauthorized", "Authorize with Telegram")
			return
		}

		if config.GetBlockedTelegramIds()[sess.User.ID] {
			h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
			return
		}
		if h.runtimeSettings != nil {
			maintenance := h.runtimeSettings.Maintenance()
			if maintenance.Enabled && !h.isAdmin(sess.User.ID) {
				h.writeErrorWithMeta(w, http.StatusServiceUnavailable, "maintenance", "Service is under maintenance", maintenance)
				return
			}
			if !h.runtimeSettings.FeatureEnabled("mini_app") && !h.isAdmin(sess.User.ID) {
				h.writeError(w, http.StatusServiceUnavailable, "feature_disabled", "Mini app is temporarily unavailable")
				return
			}
			if feature := runtimeFeatureForPath(r.URL.Path); feature != "" && !h.runtimeSettings.FeatureEnabled(feature) {
				h.writeError(w, http.StatusServiceUnavailable, "feature_disabled", "This function is temporarily unavailable")
				return
			}
		}
		if !h.rateLimiter.Allow(rateLimitKey(r.URL.Path, sess.User.ID), miniAppRateLimitRule(r.URL.Path), time.Now().UTC()) {
			h.writeError(w, http.StatusTooManyRequests, "too_many_requests", "Слишком много запросов, попробуйте чуть позже")
			return
		}

		if customer == nil {
			customer, err = h.ensureCustomer(r.Context(), sess)
			if err != nil {
				slog.Error("mini app: ensure customer", "error", err)
				h.writeError(w, http.StatusInternalServerError, "customer_sync_failed", "Не удалось обновить профиль")
				return
			}
		}
		forceChannelCheck := strings.TrimSpace(r.Header.Get("X-Force-Channel-Check")) == "1"
		verified, err := h.verifyRequiredChannelSubscription(r.Context(), customer, forceChannelCheck)
		if err != nil {
			slog.Error("mini app: required channel verification failed", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
			h.writeErrorWithMeta(w, http.StatusForbidden, "channel_subscription_check_failed", "Не удалось проверить подписку", h.requiredChannelSubscriptionMeta())
			return
		}
		if !verified {
			h.writeErrorWithMeta(w, http.StatusForbidden, "channel_subscription_required", "Подпишитесь на канал, чтобы продолжить", h.requiredChannelSubscriptionMeta())
			return
		}

		next(w, r, sess, customer)
	}
}

func extractAuthData(r *http.Request) (string, string, string, error) {
	if headerValue := strings.TrimSpace(r.Header.Get("X-Telegram-Init-Data")); headerValue != "" {
		return headerValue, "", "", nil
	}
	if headerValue := strings.TrimSpace(r.Header.Get("X-Telegram-Login-Data")); headerValue != "" {
		return "", headerValue, "", nil
	}
	if headerValue := strings.TrimSpace(r.Header.Get("X-Google-ID-Token")); headerValue != "" {
		return "", "", headerValue, nil
	}

	if r.Body == nil {
		return "", "", "", nil
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return "", "", "", err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	if len(bytes.TrimSpace(body)) == 0 {
		return "", "", "", nil
	}

	var payload authRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", "", err
	}

	return strings.TrimSpace(payload.InitData), strings.TrimSpace(payload.LoginData), strings.TrimSpace(payload.GoogleIDToken), nil
}

func (h *Handler) sessionFromGoogleLogin(ctx context.Context, idToken string) (*session, *database.Customer, error) {
	identity, err := validateGoogleIDToken(ctx, idToken, config.GoogleClientID())
	if err != nil {
		return nil, nil, err
	}

	customer, err := h.customerRepository.FindByGoogleSubject(ctx, identity.Subject)
	if err != nil {
		return nil, nil, err
	}
	if customer == nil {
		customer, err = h.customerRepository.FindByGoogleEmail(ctx, identity.Email)
		if err != nil {
			return nil, nil, err
		}
	}
	if customer == nil {
		return nil, nil, errGoogleAuthNotLinked
	}

	return &session{
		AuthDate: time.Now().UTC(),
		Provider: sessionProviderGoogle,
		User: telegramUser{
			ID:           customer.TelegramID,
			FirstName:    identity.Name,
			PhotoURL:     identity.Picture,
			LanguageCode: customer.Language,
		},
		GoogleSubject:       identity.Subject,
		GoogleEmail:         identity.Email,
		GoogleEmailVerified: identity.EmailVerified,
	}, customer, nil
}

func telegramBotUsername() string {
	botURL := strings.TrimSpace(config.BotURL())
	if botURL == "" {
		return ""
	}

	parsed, err := url.Parse(botURL)
	if err == nil && parsed.Host != "" {
		return strings.Trim(strings.TrimPrefix(parsed.Path, "/"), " /")
	}

	return strings.Trim(strings.TrimPrefix(strings.TrimPrefix(botURL, "https://t.me/"), "http://t.me/"), " /")
}

func telegramBotID() string {
	token := strings.TrimSpace(config.TelegramToken())
	index := strings.Index(token, ":")
	if index <= 0 {
		return ""
	}

	return strings.TrimSpace(token[:index])
}

func (h *Handler) handleBootstrap(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	started := time.Now()
	fast := strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Bootstrap-Mode")), "fast")
	payload, err := h.buildBootstrapResponseMode(r.Context(), sess, customer, fast)
	if err != nil {
		slog.Error("mini app: bootstrap", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID), "duration", time.Since(started))
		h.writeError(w, http.StatusInternalServerError, "bootstrap_failed", "Не удалось загрузить данные")
		return
	}

	slog.Info("mini app: bootstrap success", "telegramId", utils.MaskHalfInt64(sess.User.ID), "duration", time.Since(started))
	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"data": payload,
	})
}

func (h *Handler) handleLinkGoogle(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.runtimeSettings != nil && !h.runtimeSettings.FeatureEnabled("google") {
		h.writeError(w, http.StatusServiceUnavailable, "feature_disabled", "Gmail login is temporarily unavailable")
		return
	}
	var req googleLinkRequest
	if err := h.decodeJSONRequest(w, r, 1<<20, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}

	identity, err := validateGoogleIDToken(r.Context(), req.GoogleIDToken, config.GoogleClientID())
	if err != nil {
		slog.Warn("mini app: google link validation failed", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		if errors.Is(err, errGoogleAuthNotConfigured) {
			h.writeError(w, http.StatusServiceUnavailable, "google_not_configured", "Gmail login is not configured")
			return
		}
		h.writeError(w, http.StatusUnauthorized, "google_invalid", "Gmail authorization failed")
		return
	}

	customer, err = h.linkGoogleIdentity(r.Context(), customer, identity)
	if err != nil {
		slog.Error("mini app: google link failed", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		if errors.Is(err, errGoogleAlreadyLinked) {
			h.writeError(w, http.StatusConflict, "google_already_linked", "This Gmail is already linked")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "google_link_failed", "Could not link Gmail")
		return
	}

	payload, err := h.buildBootstrapResponse(r.Context(), sess, customer)
	if err != nil {
		slog.Error("mini app: google link bootstrap failed", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusInternalServerError, "bootstrap_failed", "Could not load data")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Gmail linked",
		"data":    payload,
	})
}

func (h *Handler) linkGoogleIdentity(ctx context.Context, customer *database.Customer, identity *googleIdentity) (*database.Customer, error) {
	if customer == nil || identity == nil {
		return nil, fmt.Errorf("invalid google link context")
	}

	existingBySubject, err := h.customerRepository.FindByGoogleSubject(ctx, identity.Subject)
	if err != nil {
		return nil, fmt.Errorf("google subject lookup failed: %w", err)
	}
	if existingBySubject != nil && existingBySubject.ID != customer.ID {
		return nil, errGoogleAlreadyLinked
	}

	existingByEmail, err := h.customerRepository.FindByGoogleEmail(ctx, identity.Email)
	if err != nil {
		return nil, fmt.Errorf("google email lookup failed: %w", err)
	}
	if existingByEmail != nil && existingByEmail.ID != customer.ID {
		return nil, errGoogleAlreadyLinked
	}

	customer, err = h.customerRepository.LinkGoogleIdentity(ctx, customer.ID, identity.Subject, identity.Email, identity.EmailVerified)
	if err != nil {
		if strings.Contains(err.Error(), "idx_customer_google") {
			return nil, errGoogleAlreadyLinked
		}
		return nil, err
	}

	return customer, nil
}

func (h *Handler) handleActivateTrial(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.runtimeSettings != nil && !h.runtimeSettings.FeatureEnabled("trials") {
		h.writeError(w, http.StatusServiceUnavailable, "feature_disabled", "Trial activation is temporarily unavailable")
		return
	}
	customer = h.syncCustomerState(r.Context(), customer)
	trialEligible, err := h.canActivateTrial(r.Context(), customer)
	if err != nil {
		slog.Error("mini app: trial eligibility check failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "trial_failed", "Не удалось проверить пробный период")
		return
	}
	if !trialEligible {
		h.writeError(w, http.StatusBadRequest, "trial_unavailable", "Пробный период недоступен")
		return
	}

	ctxWithProfile := contextWithSessionTelegramProfile(r.Context(), sess)
	if _, err := h.paymentService.ActivateTrial(ctxWithProfile, sess.User.ID); err != nil {
		slog.Error("mini app: activate trial", "error", err)
		h.writeError(w, http.StatusInternalServerError, "trial_failed", "Не удалось активировать пробный период")
		return
	}

	updatedCustomer, err := h.customerRepository.FindByTelegramId(r.Context(), sess.User.ID)
	if err != nil || updatedCustomer == nil {
		h.writeError(w, http.StatusInternalServerError, "trial_failed", "Не удалось обновить состояние подписки")
		return
	}

	payload, err := h.buildBootstrapResponse(r.Context(), sess, updatedCustomer)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "trial_failed", "Не удалось обновить интерфейс")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Пробный период активирован",
		"data":    payload,
	})
}

func (h *Handler) handleCreatePurchase(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	var req purchaseRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	invoiceType, err := mapPaymentMethod(req.PaymentMethod)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "unsupported_payment_method", "Этот способ оплаты недоступен")
		return
	}

	allowedMethods, err := h.availablePaymentMethods(r.Context(), customer)
	if err != nil {
		slog.Error("mini app: resolve payment methods", "error", err)
		h.writeError(w, http.StatusInternalServerError, "payment_config_failed", "Не удалось определить способы оплаты")
		return
	}
	if !allowedMethods[req.PaymentMethod] {
		h.writeError(w, http.StatusBadRequest, "unsupported_payment_method", "Этот способ оплаты недоступен")
		return
	}

	plan, ok := h.checkoutPlanForRequest(req.PlanID, req.Months)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "unsupported_plan", "Этот тариф недоступен")
		return
	}
	price, ok := checkoutAmountForPlan(plan, invoiceType)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "unsupported_plan", "Этот тариф недоступен")
		return
	}
	if req.PromoCode != "" {
		_, _, _, err := h.resolvePromoCode(r.Context(), customer.ID, req.PromoCode)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "unsupported_plan", "Оплата в Stars для этого тарифа отключена")
			return
		}
	}

	ctxWithProfile := contextWithSessionTelegramProfile(r.Context(), sess)
	paymentURL, purchaseID, err := h.paymentService.CreatePurchaseWithOptions(ctxWithProfile, float64(price), plan.Months, customer, invoiceType, payment.CreatePurchaseOptions{
		AgreementAccepted: true,
		PlanID:            plan.ID,
		TrafficLimitBytes: &plan.TrafficLimitBytes,
		DeviceLimitCount:  &plan.DeviceLimitCount,
	})
	if err != nil {
		slog.Error("mini app: create purchase", "error", err, "method", req.PaymentMethod, "months", req.Months)
		h.writeError(w, http.StatusInternalServerError, "purchase_failed", "Не удалось создать оплату")
		return
	}

	action := "open_link"
	switch invoiceType {
	case database.InvoiceTypeTelegram:
		action = "open_invoice"
	case database.InvoiceTypeYookasa:
		action = "open_in_app"
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": purchaseResponse{
			Action:     action,
			URL:        paymentURL,
			PurchaseID: purchaseID,
		},
	})
}

func (h *Handler) handleCreatePurchaseV2(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	var req purchaseRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}

	invoiceType, err := mapPaymentMethod(req.PaymentMethod)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "unsupported_payment_method", "Unsupported payment method")
		return
	}

	allowedMethods, err := h.availablePaymentMethods(r.Context(), customer)
	if err != nil {
		slog.Error("mini app: resolve payment methods", "error", err)
		h.writeError(w, http.StatusInternalServerError, "payment_config_failed", "Failed to resolve payment methods")
		return
	}
	if !allowedMethods[req.PaymentMethod] {
		h.writeError(w, http.StatusBadRequest, "unsupported_payment_method", "Unsupported payment method")
		return
	}

	plan, ok := h.checkoutPlanForRequest(req.PlanID, req.Months)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "unsupported_plan", "Unsupported plan")
		return
	}
	price, ok := checkoutAmountForPlan(plan, invoiceType)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "unsupported_plan", "Unsupported plan")
		return
	}

	var promo *database.PromoCode
	if req.PromoCode != "" {
		unlockPromo := lockPromoPurchase(req.PromoCode)
		defer unlockPromo()

		promo, normalizedCode, promoErrCode, err := h.resolvePromoCode(r.Context(), customer.ID, req.PromoCode)
		if err != nil {
			slog.Error("mini app: resolve promo code", "error", err, "method", req.PaymentMethod, "months", req.Months)
			h.writeError(w, http.StatusInternalServerError, "promo_failed", promoErrorMessage("promo_failed"))
			return
		}
		if promoErrCode != "" {
			h.writeError(w, http.StatusBadRequest, promoErrCode, promoErrorMessage(promoErrCode))
			return
		}

		req.PromoCode = normalizedCode
		price = applyDiscount(price, promo.DiscountPercent)
	}

	ctxWithProfile := contextWithSessionTelegramProfile(r.Context(), sess)
	paymentURL, purchaseID, err := h.paymentService.CreatePurchaseWithOptions(ctxWithProfile, float64(price), plan.Months, customer, invoiceType, payment.CreatePurchaseOptions{
		AgreementAccepted:    true,
		PlanID:               plan.ID,
		TrafficLimitBytes:    &plan.TrafficLimitBytes,
		DeviceLimitCount:     &plan.DeviceLimitCount,
		PromoCodeID:          promoCodeIDOrNil(promo),
		PromoCodeCode:        req.PromoCode,
		PromoDiscountPercent: promoDiscountPercentOrZero(promo),
	})
	if err != nil {
		slog.Error("mini app: create purchase", "error", err, "method", req.PaymentMethod, "months", req.Months)
		h.writeError(w, http.StatusInternalServerError, "purchase_failed", "Failed to create purchase")
		return
	}

	action := "open_link"
	switch invoiceType {
	case database.InvoiceTypeTelegram:
		action = "open_invoice"
	case database.InvoiceTypeYookasa:
		action = "open_in_app"
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": purchaseResponse{
			Action:     action,
			URL:        paymentURL,
			PurchaseID: purchaseID,
		},
	})
}

func (h *Handler) handleApplyPromoCode(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	var req promoCodeApplyRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}

	promo, normalizedCode, promoErrCode, err := h.resolvePromoCode(r.Context(), customer.ID, req.Code)
	if err != nil {
		slog.Error("mini app: apply promo code", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusInternalServerError, "promo_failed", promoErrorMessage("promo_failed"))
		return
	}
	if promoErrCode != "" {
		h.writeError(w, http.StatusBadRequest, promoErrCode, promoErrorMessage(promoErrCode))
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": promoCodePayload{
			ID:              promo.ID,
			Code:            normalizedCode,
			DiscountPercent: promo.DiscountPercent,
			ExpiresAt:       formatOptionalTime(timeOrNil(promo.ExpiresAt)),
			MaxRedemptions:  optionalIntValue(promo.MaxRedemptions),
			RedemptionCount: promo.RedemptionCount,
			Status:          promoStatus(promo, time.Now().UTC()),
			CreatedAt:       promo.CreatedAt.UTC().Format(time.RFC3339),
		},
	})
}

func (h *Handler) handleAdminCreatePromoCode(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.promoCodeRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "promo_unavailable", promoErrorMessage("promo_unavailable"))
		return
	}

	var req promoCodeCreateRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}

	code := database.NormalizePromoCode(req.Code)
	if code == "" {
		h.writeError(w, http.StatusBadRequest, "promo_code_required", promoErrorMessage("promo_code_required"))
		return
	}
	if !database.IsValidPromoCode(code) {
		h.writeError(w, http.StatusBadRequest, "promo_invalid_format", promoErrorMessage("promo_invalid_format"))
		return
	}
	if req.DiscountPercent <= 0 || req.DiscountPercent >= 100 {
		h.writeError(w, http.StatusBadRequest, "promo_invalid_discount", promoErrorMessage("promo_invalid_discount"))
		return
	}
	if req.MaxRedemptions < 0 {
		h.writeError(w, http.StatusBadRequest, "promo_invalid_limit", promoErrorMessage("promo_invalid_limit"))
		return
	}

	expiresAt, err := parsePromoExpiry(req.ExpiresAt)
	if err != nil || (expiresAt != nil && !expiresAt.After(time.Now().UTC())) {
		h.writeError(w, http.StatusBadRequest, "promo_invalid_expiry", promoErrorMessage("promo_invalid_expiry"))
		return
	}

	var maxRedemptions *int
	if req.MaxRedemptions > 0 {
		maxRedemptions = &req.MaxRedemptions
	}

	_, err = h.promoCodeRepository.Create(r.Context(), &database.PromoCode{
		Code:                code,
		DiscountPercent:     req.DiscountPercent,
		IsActive:            true,
		ExpiresAt:           expiresAt,
		MaxRedemptions:      maxRedemptions,
		CreatedByTelegramID: sess.User.ID,
	})
	if err != nil {
		switch {
		case errors.Is(err, database.ErrPromoCodeAlreadyExists):
			h.writeError(w, http.StatusConflict, "promo_already_exists", promoErrorMessage("promo_already_exists"))
			return
		case errors.Is(err, database.ErrPromoCodeInvalidFormat):
			h.writeError(w, http.StatusBadRequest, "promo_invalid_format", promoErrorMessage("promo_invalid_format"))
			return
		default:
			slog.Error("mini app: create promo code", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
			h.writeError(w, http.StatusInternalServerError, "promo_create_failed", promoErrorMessage("promo_create_failed"))
			return
		}
	}

	payload, err := h.buildAdminPayload(r.Context())
	if err != nil {
		slog.Error("mini app: build admin payload after promo create", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusInternalServerError, "promo_create_failed", promoErrorMessage("promo_create_failed"))
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "promo_created",
		"data":    payload,
	})
}

func (h *Handler) handleAdminDeletePromoCode(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.promoCodeRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "promo_unavailable", promoErrorMessage("promo_unavailable"))
		return
	}

	var req promoCodeDeleteRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	if req.ID <= 0 {
		h.writeError(w, http.StatusBadRequest, "promo_delete_failed", promoErrorMessage("promo_delete_failed"))
		return
	}

	if err := h.promoCodeRepository.Delete(r.Context(), req.ID); err != nil {
		slog.Error("mini app: delete promo code", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID), "promoId", req.ID)
		h.writeError(w, http.StatusInternalServerError, "promo_delete_failed", promoErrorMessage("promo_delete_failed"))
		return
	}

	payload, err := h.buildAdminPayload(r.Context())
	if err != nil {
		slog.Error("mini app: build admin payload after promo delete", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusInternalServerError, "promo_delete_failed", promoErrorMessage("promo_delete_failed"))
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "promo_deleted",
		"data":    payload,
	})
}

func (h *Handler) handleAdminFindSubscription(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.remnawaveClient == nil {
		h.writeError(w, http.StatusServiceUnavailable, "subscription_rebind_failed", "Subscription service is unavailable")
		return
	}

	var req adminSubscriptionFindRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_subscription_query", "Invalid subscription query")
		return
	}
	query := strings.TrimSpace(req.Query)
	if query == "" || len(query) > 128 {
		h.writeError(w, http.StatusBadRequest, "invalid_subscription_query", "Enter a panel ID or username")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	subscription, err := h.remnawaveClient.FindUserByIDOrUsername(ctx, query)
	if err != nil {
		if errors.Is(err, remnawave.ErrAdminSubscriptionNotFound) {
			h.writeError(w, http.StatusNotFound, "subscription_not_found", "Subscription not found")
			return
		}
		slog.Error("mini app: find admin subscription", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusBadGateway, "subscription_rebind_failed", "Failed to load subscription")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"data": adminSubscriptionToPayload(subscription),
	})
}

func (h *Handler) handleAdminRebindSubscription(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.remnawaveClient == nil || h.customerRepository == nil || h.telegramBot == nil {
		h.writeError(w, http.StatusServiceUnavailable, "subscription_rebind_failed", "Subscription service is unavailable")
		return
	}

	var req adminSubscriptionRebindRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	userUUID, err := uuid.Parse(strings.TrimSpace(req.UserUUID))
	if err != nil || userUUID == uuid.Nil || req.TargetTelegramID <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid subscription or Telegram ID")
		return
	}

	targetCustomer, err := h.customerRepository.FindByTelegramId(r.Context(), req.TargetTelegramID)
	if err != nil {
		slog.Error("mini app: load subscription transfer target", "error", err, "targetTelegramId", utils.MaskHalfInt64(req.TargetTelegramID))
		h.writeError(w, http.StatusInternalServerError, "subscription_rebind_failed", "Failed to load target account")
		return
	}
	if targetCustomer == nil {
		h.writeError(w, http.StatusNotFound, "target_not_registered", "Target account must start the bot first")
		return
	}

	profileCtx, profileCancel := context.WithTimeout(r.Context(), 8*time.Second)
	targetChat, err := h.telegramBot.GetChat(profileCtx, &bot.GetChatParams{ChatID: req.TargetTelegramID})
	profileCancel()
	if err != nil || targetChat == nil {
		slog.Warn("mini app: load subscription transfer Telegram profile", "error", err, "targetTelegramId", utils.MaskHalfInt64(req.TargetTelegramID))
		h.writeError(w, http.StatusConflict, "target_not_registered", "Target account must open the bot and send /start")
		return
	}
	targetDescription := remnawave.FormatTelegramDescription(
		buildTelegramDisplayName(targetChat.FirstName, targetChat.LastName),
		targetChat.Username,
	)

	adminSubscriptionRebindLock.Lock()
	defer adminSubscriptionRebindLock.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rebindResult, err := h.remnawaveClient.RebindUserTelegramID(ctx, userUUID, req.TargetTelegramID, targetDescription)
	if err != nil {
		switch {
		case errors.Is(err, remnawave.ErrAdminSubscriptionNotFound):
			h.writeError(w, http.StatusNotFound, "subscription_not_found", "Subscription not found")
		default:
			slog.Error("mini app: rebind admin subscription", "error", err, "targetTelegramId", utils.MaskHalfInt64(req.TargetTelegramID))
			h.writeError(w, http.StatusBadGateway, "subscription_rebind_failed", "Failed to rebind subscription")
		}
		return
	}
	updated := rebindResult.Subscription
	previousTelegramID := rebindResult.PreviousTelegramID

	oldTelegramID := int64(0)
	if previousTelegramID != nil {
		oldTelegramID = *previousTelegramID
	}
	if err := h.customerRepository.TransferSubscriptionCache(
		r.Context(),
		oldTelegramID,
		req.TargetTelegramID,
		updated.ExpireAt,
		updated.SubscriptionLink,
	); err != nil {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 15*time.Second)
		rollbackErr := h.remnawaveClient.RestoreAdminRebind(rollbackCtx, userUUID, previousTelegramID, rebindResult.PreviousDescription, rebindResult.DisplacedSubscription)
		rollbackCancel()
		if rollbackErr != nil {
			slog.Error("mini app: rollback admin subscription rebind", "error", rollbackErr, "userUuid", userUUID.String())
		}
		slog.Error("mini app: transfer subscription cache", "error", err, "targetTelegramId", utils.MaskHalfInt64(req.TargetTelegramID))
		if errors.Is(err, database.ErrSubscriptionTransferTargetNotFound) {
			h.writeError(w, http.StatusNotFound, "target_not_registered", "Target account must start the bot first")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "subscription_rebind_failed", "Failed to save subscription owner")
		return
	}

	slog.Info(
		"mini app: admin subscription rebound",
		"adminTelegramId", utils.MaskHalfInt64(sess.User.ID),
		"oldTelegramId", utils.MaskHalfInt64(oldTelegramID),
		"newTelegramId", utils.MaskHalfInt64(req.TargetTelegramID),
		"userUuid", userUUID.String(),
		"displacedSubscription", rebindResult.DisplacedSubscription != nil,
	)
	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "subscription_rebound",
		"data":    adminSubscriptionToPayload(updated),
	})
}

func adminSubscriptionToPayload(subscription *remnawave.AdminSubscription) adminSubscriptionPayload {
	if subscription == nil {
		return adminSubscriptionPayload{}
	}

	payload := adminSubscriptionPayload{
		ID:                subscription.ID,
		UserUUID:          subscription.UUID.String(),
		Username:          subscription.Username,
		CurrentTelegramID: subscription.TelegramID,
		Status:            subscription.Status,
		SubscriptionLink:  subscription.SubscriptionLink,
	}
	if !subscription.ExpireAt.IsZero() {
		payload.ExpiresAt = subscription.ExpireAt.UTC().Format(time.RFC3339)
	}
	return payload
}

func (h *Handler) handleAdminSettingsUpdate(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.runtimeSettings == nil {
		h.writeError(w, http.StatusServiceUnavailable, "settings_unavailable", "Runtime settings are unavailable")
		return
	}

	var req adminSettingsUpdateRequest
	if err := h.decodeJSONRequest(w, r, 1<<20, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid settings")
		return
	}
	settings, err := h.runtimeSettings.Update(r.Context(), req.Settings, sess.User.ID)
	if err != nil {
		slog.Warn("mini app: invalid runtime settings", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusBadRequest, "invalid_settings", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "settings_updated",
		"data":    settings,
	})
}

func (h *Handler) handleAdminIntegrationUpdate(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.integrationSettings == nil {
		h.writeError(w, http.StatusServiceUnavailable, "integrations_unavailable", "Интеграции недоступны")
		return
	}
	var req adminIntegrationUpdateRequest
	if err := h.decodeJSONRequest(w, r, 64<<10, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректные настройки")
		return
	}
	view, err := h.integrationSettings.Update(r.Context(), strings.TrimSpace(req.Provider), integrations.UpdateInput{Enabled: req.Enabled, Fields: req.Fields}, sess.User.ID)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_integration", err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "integration_updated", "data": view})
}

func (h *Handler) handlePaymentIntegrationWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || h.integrationSettings == nil || h.paymentService == nil {
		h.writeError(w, http.StatusNotFound, "not_found", "Not found")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/payments/webhook/"), "/"), "/")
	if len(parts) != 2 {
		h.writeError(w, http.StatusNotFound, "not_found", "Not found")
		return
	}
	provider, token := parts[0], parts[1]
	expected := h.integrationSettings.WebhookToken(provider)
	if expected == "" || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
		h.writeError(w, http.StatusUnauthorized, "invalid_webhook", "Invalid webhook")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_webhook", "Invalid webhook")
		return
	}
	form := url.Values{}
	if provider == integrations.ProviderFreeKassa {
		form, err = url.ParseQuery(string(raw))
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid_webhook", "Invalid webhook")
			return
		}
	}
	ack, err := h.paymentService.ProcessExternalWebhook(r.Context(), provider, r.Header, raw, form)
	if err != nil {
		slog.Warn("payment integration webhook rejected", "provider", provider, "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_webhook", "Invalid webhook")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(ack))
}

func (h *Handler) handleAdminEventResolve(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.errorReporter == nil {
		h.writeError(w, http.StatusServiceUnavailable, "diagnostics_unavailable", "Diagnostics are unavailable")
		return
	}

	var req adminEventResolveRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil || req.ID <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid event")
		return
	}
	if err := h.errorReporter.Resolve(r.Context(), req.ID); err != nil {
		slog.Error("mini app: resolve operational event", "error", err, "eventId", req.ID)
		h.writeError(w, http.StatusInternalServerError, "event_resolve_failed", "Failed to resolve event")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "event_resolved"})
}

func (h *Handler) handleCancelPurchase(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	var req purchaseActionRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	purchase, err := h.purchaseRepository.FindById(r.Context(), req.PurchaseID)
	if err != nil {
		slog.Error("mini app: find purchase for cancel", "error", err, "purchaseId", req.PurchaseID)
		h.writeError(w, http.StatusInternalServerError, "purchase_lookup_failed", "Не удалось получить оплату")
		return
	}
	if purchase == nil || purchase.CustomerID != customer.ID {
		h.writeError(w, http.StatusNotFound, "purchase_not_found", "Оплата не найдена")
		return
	}

	status := purchase.Status
	if purchase.Status == database.PurchaseStatusPending || purchase.Status == database.PurchaseStatusNew {
		switch purchase.InvoiceType {
		case database.InvoiceTypeYookasa:
			if err := h.paymentService.CancelYookassaPayment(purchase.ID); err != nil {
				slog.Error("mini app: cancel yookassa purchase", "error", err, "purchaseId", purchase.ID)
				h.writeError(w, http.StatusInternalServerError, "cancel_failed", "Не удалось отменить оплату")
				return
			}
			status = database.PurchaseStatusCancel
		default:
			if err := h.purchaseRepository.UpdateFields(r.Context(), purchase.ID, map[string]any{"status": database.PurchaseStatusCancel}); err != nil {
				slog.Error("mini app: cancel purchase", "error", err, "purchaseId", purchase.ID)
				h.writeError(w, http.StatusInternalServerError, "cancel_failed", "Не удалось отменить оплату")
				return
			}
			status = database.PurchaseStatusCancel
		}
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": purchaseActionResponse{
			Status: string(status),
		},
	})
}

func (h *Handler) handleToggleAutoPayment(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	var req autoPaymentToggleRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	if !config.EnableAutoPayment() {
		h.writeError(w, http.StatusBadRequest, "autopay_unavailable", "Автоплатежи временно недоступны")
		return
	}

	hasPaymentMethod := customer.YookasaPaymentMethodID != nil && *customer.YookasaPaymentMethodID != uuid.Nil
	if req.Enabled && !hasPaymentMethod {
		h.writeError(w, http.StatusBadRequest, "payment_method_required", "Сначала оплатите заказ картой, чтобы сохранить способ оплаты")
		return
	}

	updates := map[string]any{
		"autopay_enabled": req.Enabled,
	}

	if req.Enabled && (customer.AutoPaymentPlanMonths == nil || *customer.AutoPaymentPlanMonths <= 0) {
		latestPurchase, err := h.purchaseRepository.FindLatestSuccessfulYookasaPurchaseByCustomer(r.Context(), customer.ID)
		if err != nil {
			slog.Error("mini app: load latest yookassa purchase", "error", err, "customerId", utils.MaskHalfInt64(customer.ID))
			h.writeError(w, http.StatusInternalServerError, "payments_failed", "Не удалось обновить автоплатеж")
			return
		}
		if latestPurchase == nil || latestPurchase.Month <= 0 {
			h.writeError(w, http.StatusBadRequest, "plan_required", "Не удалось определить тариф для автоплатежа")
			return
		}
		updates["autopay_plan_months"] = latestPurchase.Month
	}

	if err := h.customerRepository.UpdateFields(r.Context(), customer.ID, updates); err != nil {
		slog.Error("mini app: toggle autopay", "error", err, "customerId", utils.MaskHalfInt64(customer.ID))
		h.writeError(w, http.StatusInternalServerError, "payments_failed", "Не удалось обновить автоплатеж")
		return
	}

	updatedCustomer, err := h.customerRepository.FindById(r.Context(), customer.ID)
	if err != nil || updatedCustomer == nil {
		h.writeError(w, http.StatusInternalServerError, "payments_failed", "Не удалось обновить платежи")
		return
	}

	payload, err := h.buildPaymentsPayload(r.Context(), updatedCustomer)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "payments_failed", "Не удалось обновить платежи")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "autopay_updated",
		"data":    payload,
	})
}

func (h *Handler) handleRemovePaymentMethod(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	hadRealPaymentMethod := customer.YookasaPaymentMethodID != nil && *customer.YookasaPaymentMethodID != uuid.Nil
	updates := map[string]any{
		"autopay_enabled":                 false,
		"autopay_plan_months":             nil,
		"yookasa_payment_method_id":       nil,
		"yookasa_payment_method_type":     nil,
		"yookasa_payment_method_title":    nil,
		"yookasa_payment_method_saved_at": nil,
		"yookasa_last_charge_at":          nil,
		"yookasa_last_charge_status":      nil,
		"yookasa_last_charge_error":       nil,
	}

	if err := h.customerRepository.UpdateFields(r.Context(), customer.ID, updates); err != nil {
		slog.Error("mini app: remove payment method", "error", err, "customerId", utils.MaskHalfInt64(customer.ID))
		h.writeError(w, http.StatusInternalServerError, "payments_failed", "Не удалось удалить способ оплаты")
		return
	}

	updatedCustomer, err := h.customerRepository.FindById(r.Context(), customer.ID)
	if err != nil || updatedCustomer == nil {
		h.writeError(w, http.StatusInternalServerError, "payments_failed", "Не удалось обновить платежи")
		return
	}

	payload, err := h.buildPaymentsPayload(r.Context(), updatedCustomer)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "payments_failed", "Не удалось обновить платежи")
		return
	}
	if !hadRealPaymentMethod && config.PaymentMethodDemoEnabled() {
		payload.HasPaymentMethod = false
		payload.AutoPaymentEnabled = false
		payload.Method = nil
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "payment_method_removed",
		"data":    payload,
	})
}

func (h *Handler) handleDeleteDevice(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.remnawaveClient == nil {
		h.writeError(w, http.StatusServiceUnavailable, "devices_unavailable", "Устройства временно недоступны")
		return
	}

	var req deviceDeleteRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	hwid := strings.TrimSpace(req.Hwid)
	if hwid == "" {
		h.writeError(w, http.StatusBadRequest, "hwid_required", "Не удалось определить устройство")
		return
	}

	userUUID, err := uuid.Parse(strings.TrimSpace(req.UserUUID))
	if err != nil || userUUID == uuid.Nil {
		h.writeError(w, http.StatusInternalServerError, "device_delete_failed", "Не удалось удалить устройство")
		return
	}

	customer = h.syncCustomerState(r.Context(), customer)

	highestPurchase, err := h.purchaseRepository.FindHighestSuccessfulPurchaseByCustomer(r.Context(), customer.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "device_delete_failed", "Не удалось обновить подписку")
		return
	}

	panelState, err := h.remnawaveClient.GetUserStateByTelegramID(r.Context(), customer.TelegramID)
	if err != nil {
		slog.Warn("mini app: reload panel state after delete failed", "error", err, "telegramId", utils.MaskHalfInt64(customer.TelegramID))
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"data": h.buildSubscriptionPayload(customer, highestPurchase, panelState),
	})
}

func (h *Handler) handleDeleteDeviceExact(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.remnawaveClient == nil {
		h.writeError(w, http.StatusServiceUnavailable, "devices_unavailable", "Устройства временно недоступны")
		return
	}

	var req deviceDeleteRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	hwid := strings.TrimSpace(req.Hwid)
	if hwid == "" {
		h.writeError(w, http.StatusBadRequest, "hwid_required", "Не удалось определить устройство")
		return
	}

	userUUID, err := uuid.Parse(strings.TrimSpace(req.UserUUID))
	if err != nil || userUUID == uuid.Nil {
		h.writeError(w, http.StatusBadRequest, "user_uuid_required", "Не удалось определить подписку")
		return
	}

	updatedDevices, err := h.remnawaveClient.DeleteUserHWIDDevice(r.Context(), userUUID, hwid)
	if err != nil {
		slog.Error("mini app: exact delete device failed", "error", err, "telegramId", utils.MaskHalfInt64(customer.TelegramID), "userUuid", userUUID.String())
		h.writeError(w, http.StatusInternalServerError, "device_delete_failed", "Не удалось удалить устройство")
		return
	}

	customer = h.syncCustomerState(r.Context(), customer)

	highestPurchase, err := h.purchaseRepository.FindHighestSuccessfulPurchaseByCustomer(r.Context(), customer.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "device_delete_failed", "Не удалось обновить подписку")
		return
	}

	panelState, err := h.remnawaveClient.GetUserStateByTelegramID(r.Context(), customer.TelegramID)
	if err != nil {
		slog.Error("mini app: exact reload panel state after delete failed", "error", err, "telegramId", utils.MaskHalfInt64(customer.TelegramID), "userUuid", userUUID.String())
		h.writeError(w, http.StatusInternalServerError, "device_delete_failed", "Панель не подтвердила удаление устройства")
		return
	}
	if panelState != nil {
		panelState.Devices = updatedDevices
		panelState.UsedDevices = len(updatedDevices)
		if panelState.UserUUID == uuid.Nil {
			panelState.UserUUID = userUUID
		}
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"data": h.buildSubscriptionPayload(customer, highestPurchase, panelState),
	})
}

func (h *Handler) handleCreateReview(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.reviewRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "reviews_unavailable", "Отзывы временно недоступны")
		return
	}

	var req reviewCreateRequest
	if err := h.decodeJSONRequest(w, r, 8192, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	rating := req.Rating
	if rating < 1 || rating > 5 {
		h.writeError(w, http.StatusBadRequest, "invalid_rating", "Выберите оценку от 1 до 5")
		return
	}

	comment := strings.TrimSpace(req.Comment)
	if comment == "" {
		h.writeError(w, http.StatusBadRequest, "comment_required", "Напишите комментарий к отзыву")
		return
	}
	if len([]rune(comment)) > 2000 {
		h.writeError(w, http.StatusBadRequest, "comment_too_long", "Комментарий слишком длинный")
		return
	}

	existingReview, err := h.reviewRepository.FindAnyByCustomerID(r.Context(), customer.ID)
	if err != nil {
		slog.Error("mini app: review lookup failed", "error", err, "telegramId", utils.MaskHalfInt64(customer.TelegramID))
		h.writeError(w, http.StatusInternalServerError, "review_failed", "Не удалось сохранить отзыв")
		return
	}

	if existingReview != nil {
		if !existingReview.RewardGranted {
			customer = h.syncCustomerState(r.Context(), customer)
			if err := h.grantReviewReward(contextWithSessionTelegramProfile(r.Context(), sess), customer, existingReview.ID); err != nil {
				slog.Error("mini app: review reward retry failed", "error", err, "reviewId", existingReview.ID, "telegramId", utils.MaskHalfInt64(customer.TelegramID))
				h.writeError(w, http.StatusInternalServerError, "review_reward_failed", "Отзыв уже сохранён, но подарок пока не удалось выдать")
				return
			}
		}

		if updatedCustomer, findErr := h.customerRepository.FindByTelegramId(r.Context(), sess.User.ID); findErr == nil && updatedCustomer != nil {
			customer = updatedCustomer
		}

		payload, buildErr := h.buildBootstrapResponse(r.Context(), sess, customer)
		if buildErr != nil {
			h.writeError(w, http.StatusInternalServerError, "review_exists", "Отзыв уже существует")
			return
		}

		h.writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"message": "Подарок за отзыв уже был получен",
			"data":    payload,
		})
		return
	}

	review, err := h.reviewRepository.Create(r.Context(), &database.Review{
		CustomerID:       customer.ID,
		TelegramID:       customer.TelegramID,
		TelegramUsername: buildReviewUsername(sess),
		Rating:           rating,
		Comment:          comment,
	})
	if err != nil {
		if errors.Is(err, database.ErrReviewAlreadyExists) {
			h.writeError(w, http.StatusBadRequest, "review_exists", "Отзыв можно оставить только один раз")
			return
		}
		slog.Error("mini app: review create failed", "error", err, "telegramId", utils.MaskHalfInt64(customer.TelegramID))
		h.writeError(w, http.StatusInternalServerError, "review_failed", "Не удалось сохранить отзыв")
		return
	}

	customer = h.syncCustomerState(r.Context(), customer)
	if err := h.grantReviewReward(contextWithSessionTelegramProfile(r.Context(), sess), customer, review.ID); err != nil {
		slog.Error("mini app: review reward failed", "error", err, "reviewId", review.ID, "telegramId", utils.MaskHalfInt64(customer.TelegramID))
		h.writeError(w, http.StatusInternalServerError, "review_reward_failed", "Отзыв сохранён, но подарок пока не удалось выдать")
		return
	}

	if updatedCustomer, findErr := h.customerRepository.FindByTelegramId(r.Context(), sess.User.ID); findErr == nil && updatedCustomer != nil {
		customer = updatedCustomer
	}

	payload, err := h.buildBootstrapResponse(r.Context(), sess, customer)
	if err != nil {
		slog.Error("mini app: rebuild bootstrap after review failed", "error", err, "reviewId", review.ID)
		h.writeError(w, http.StatusInternalServerError, "review_failed", "Отзыв сохранён, но не удалось обновить интерфейс")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Подарок за отзыв получен: +2 дня и +20 ГБ",
		"data":    payload,
	})
}

func (h *Handler) handleAdminDeleteReview(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.reviewRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "reviews_unavailable", "Отзывы временно недоступны")
		return
	}

	var req reviewDeleteRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	if req.ID <= 0 {
		h.writeError(w, http.StatusBadRequest, "review_delete_failed", "Не удалось удалить отзыв")
		return
	}

	if err := h.reviewRepository.SoftDeleteByID(r.Context(), req.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.writeError(w, http.StatusNotFound, "review_not_found", "Отзыв не найден")
			return
		}
		slog.Error("mini app: delete review", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID), "reviewId", req.ID)
		h.writeError(w, http.StatusInternalServerError, "review_delete_failed", "Не удалось удалить отзыв")
		return
	}

	payload, err := h.buildReviewsPayload(r.Context(), customer)
	if err != nil {
		slog.Error("mini app: build reviews payload after review delete", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID), "reviewId", req.ID)
		h.writeError(w, http.StatusInternalServerError, "review_delete_failed", "Не удалось обновить отзывы")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "review_deleted",
		"data":    payload,
	})
}

func (h *Handler) handleSupportRefresh(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.supportRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "support_unavailable", "Поддержка временно недоступна")
		return
	}

	customer = h.syncCustomerState(r.Context(), customer)
	highestPurchase, err := h.purchaseRepository.FindHighestSuccessfulPurchaseByCustomer(r.Context(), customer.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "support_refresh_failed", "Не удалось обновить обращения")
		return
	}

	payload, err := h.buildSupportPayload(r.Context(), sess, customer, highestPurchase)
	if err != nil {
		slog.Error("mini app: support refresh failed", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusInternalServerError, "support_refresh_failed", "Не удалось обновить обращения")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"data": payload,
	})
}

func (h *Handler) handleSupportCreate(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.supportRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "support_unavailable", "Поддержка временно недоступна")
		return
	}

	var req supportCreateRequest
	if err := h.decodeJSONRequest(w, r, 8192, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	subject := strings.TrimSpace(req.Subject)
	message := strings.TrimSpace(req.Message)
	if message == "" {
		h.writeError(w, http.StatusBadRequest, "message_required", "Опишите вопрос или проблему")
		return
	}
	if len([]rune(subject)) > 120 || len([]rune(message)) > 2000 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	highestPurchase, err := h.purchaseRepository.FindHighestSuccessfulPurchaseByCustomer(r.Context(), customer.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "support_create_failed", "Не удалось создать обращение")
		return
	}
	panelUsername := h.resolveSupportPanelUsername(r.Context(), customer)

	ticket, err := h.supportRepository.CreateTicket(r.Context(), &database.SupportTicket{
		CustomerID:        customer.ID,
		Subject:           subject,
		CustomerName:      panelUsername,
		CustomerUsername:  "",
		SubscriptionLabel: h.buildSubscriptionLabel(customer, highestPurchase),
	}, &database.SupportMessage{
		AuthorTelegramID: customer.TelegramID,
		Body:             message,
	})
	if err != nil {
		slog.Error("mini app: support create failed", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusInternalServerError, "support_create_failed", "Не удалось создать обращение")
		return
	}

	createdTicket, err := h.supportRepository.FindTicketByID(r.Context(), ticket.ID)
	if err != nil || createdTicket == nil {
		h.writeError(w, http.StatusInternalServerError, "support_create_failed", "Не удалось обновить обращение")
		return
	}

	threadPayload, err := h.buildSupportThreadPayload(r.Context(), sess, customer, createdTicket)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "support_create_failed", "Не удалось открыть обращение")
		return
	}

	h.notifySupportAsync(func(ctx context.Context) {
		h.notifyAdminAboutSupportTicket(ctx, createdTicket, message)
	})

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"data": threadPayload,
	})
}

func (h *Handler) handleSupportThread(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.supportRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "support_unavailable", "Поддержка временно недоступна")
		return
	}

	var req supportTicketRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	ticket, err := h.loadSupportTicketForViewer(r.Context(), sess, customer, req.TicketID)
	if err != nil {
		slog.Error("mini app: support thread failed", "error", err, "ticketId", req.TicketID)
		h.writeError(w, http.StatusInternalServerError, "support_thread_failed", "Не удалось открыть обращение")
		return
	}
	if ticket == nil {
		h.writeError(w, http.StatusNotFound, "support_ticket_not_found", "Обращение не найдено")
		return
	}

	threadPayload, err := h.buildSupportThreadPayload(r.Context(), sess, customer, ticket)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "support_thread_failed", "Не удалось загрузить переписку")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"data": threadPayload,
	})
}

func (h *Handler) handleSupportSend(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.supportRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "support_unavailable", "Поддержка временно недоступна")
		return
	}

	var req supportSendRequest
	if err := h.decodeJSONRequest(w, r, 8192, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	body := strings.TrimSpace(req.Message)
	if body == "" {
		h.writeError(w, http.StatusBadRequest, "message_required", "Сообщение не может быть пустым")
		return
	}
	if len([]rune(body)) > 2000 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	ticket, err := h.loadSupportTicketForViewer(r.Context(), sess, customer, req.TicketID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "support_send_failed", "Не удалось отправить сообщение")
		return
	}
	if ticket == nil {
		h.writeError(w, http.StatusNotFound, "support_ticket_not_found", "Обращение не найдено")
		return
	}
	if ticket.Status == database.SupportTicketStatusClosed {
		h.writeError(w, http.StatusBadRequest, "support_ticket_closed", "Обращение уже закрыто")
		return
	}

	isAdmin := h.isAdmin(sess.User.ID)
	if isAdmin {
		if _, err := h.supportRepository.AddAdminMessage(r.Context(), ticket.ID, sess.User.ID, body); err != nil {
			slog.Error("mini app: support admin reply failed", "error", err, "ticketId", ticket.ID)
			h.writeError(w, http.StatusInternalServerError, "support_send_failed", "Не удалось отправить сообщение")
			return
		}
		h.notifySupportAsync(func(ctx context.Context) {
			h.notifyCustomerAboutSupportReply(ctx, ticket, body)
		})
	} else {
		highestPurchase, err := h.purchaseRepository.FindHighestSuccessfulPurchaseByCustomer(r.Context(), customer.ID)
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "support_send_failed", "Не удалось отправить сообщение")
			return
		}
		panelUsername := h.resolveSupportPanelUsername(r.Context(), customer)
		if _, err := h.supportRepository.AddCustomerMessage(
			r.Context(),
			ticket.ID,
			sess.User.ID,
			body,
			panelUsername,
			"",
			h.buildSubscriptionLabel(customer, highestPurchase),
		); err != nil {
			slog.Error("mini app: support customer reply failed", "error", err, "ticketId", ticket.ID)
			h.writeError(w, http.StatusInternalServerError, "support_send_failed", "Не удалось отправить сообщение")
			return
		}
		h.notifySupportAsync(func(ctx context.Context) {
			h.notifyAdminAboutSupportReply(ctx, ticket, body)
		})
	}

	updatedTicket, err := h.supportRepository.FindTicketByID(r.Context(), ticket.ID)
	if err != nil || updatedTicket == nil {
		h.writeError(w, http.StatusInternalServerError, "support_send_failed", "Не удалось обновить переписку")
		return
	}

	threadPayload, err := h.buildSupportThreadPayload(r.Context(), sess, customer, updatedTicket)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "support_send_failed", "Не удалось обновить переписку")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"data": threadPayload,
	})
}

func (h *Handler) handleSupportClose(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.supportRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "support_unavailable", "Поддержка временно недоступна")
		return
	}

	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Недостаточно прав")
		return
	}

	var req supportTicketRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}

	ticket, err := h.supportRepository.FindTicketByID(r.Context(), req.TicketID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "support_close_failed", "Не удалось закрыть обращение")
		return
	}
	if ticket == nil {
		h.writeError(w, http.StatusNotFound, "support_ticket_not_found", "Обращение не найдено")
		return
	}

	if err := h.supportRepository.CloseTicket(r.Context(), ticket.ID); err != nil {
		slog.Error("mini app: support close failed", "error", err, "ticketId", ticket.ID)
		h.writeError(w, http.StatusInternalServerError, "support_close_failed", "Не удалось закрыть обращение")
		return
	}

	updatedTicket, err := h.supportRepository.FindTicketByID(r.Context(), ticket.ID)
	if err != nil || updatedTicket == nil {
		h.writeError(w, http.StatusInternalServerError, "support_close_failed", "Не удалось обновить обращение")
		return
	}

	threadPayload, err := h.buildSupportThreadPayload(r.Context(), sess, customer, updatedTicket)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "support_close_failed", "Не удалось обновить обращение")
		return
	}

	h.notifySupportAsync(func(ctx context.Context) {
		h.notifyCustomerAboutSupportClosed(ctx, updatedTicket)
	})

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"data": threadPayload,
	})
}

func (h *Handler) handlePaymentReturnRedirect(w http.ResponseWriter, r *http.Request) {
	purchaseID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("purchaseId")), 10, 64)
	status := "cancel"

	if purchaseID > 0 {
		resolvedStatus, err := h.paymentService.SyncYookassaPurchaseStatus(r.Context(), purchaseID)
		if err != nil {
			slog.Warn("mini app: payment return sync failed", "error", err, "purchaseId", purchaseID)
		}

		if resolvedStatus == database.PurchaseStatusPaid {
			status = string(resolvedStatus)
		} else {
			purchase, findErr := h.purchaseRepository.FindById(r.Context(), purchaseID)
			if findErr != nil {
				slog.Warn("mini app: payment return purchase lookup failed", "error", findErr, "purchaseId", purchaseID)
			} else if purchase != nil &&
				purchase.InvoiceType == database.InvoiceTypeYookasa &&
				(purchase.Status == database.PurchaseStatusPending || purchase.Status == database.PurchaseStatusNew) {
				if cancelErr := h.paymentService.CancelYookassaPayment(purchase.ID); cancelErr != nil {
					slog.Warn("mini app: payment return cancel failed", "error", cancelErr, "purchaseId", purchase.ID)
				}
			}

			if resolvedStatus != "" && resolvedStatus != database.PurchaseStatusPending && resolvedStatus != database.PurchaseStatusNew {
				status = string(resolvedStatus)
			}
		}
	}

	target := buildPaymentReturnTarget(purchaseID, status)
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handler) buildBootstrapResponse(ctx context.Context, sess *session, customer *database.Customer) (*bootstrapResponse, error) {
	return h.buildBootstrapResponseMode(ctx, sess, customer, false)
}

func (h *Handler) buildBootstrapResponseMode(ctx context.Context, sess *session, customer *database.Customer, fast bool) (*bootstrapResponse, error) {
	ctx = contextWithSessionTelegramProfile(ctx, sess)
	settings := runtimeconfig.DefaultSettings()
	if h.runtimeSettings != nil {
		settings = h.runtimeSettings.Snapshot()
	}

	var panelState *remnawave.UserState
	panelLookupFailed := false
	if !fast && h.remnawaveClient != nil {
		panelCtx, panelCancel := context.WithTimeout(ctx, 4*time.Second)
		var err error
		panelState, err = h.remnawaveClient.GetUserStateByTelegramID(panelCtx, customer.TelegramID)
		panelCancel()
		if err != nil {
			panelLookupFailed = true
			slog.Warn("mini app: load panel state failed", "error", err, "telegramId", utils.MaskHalfInt64(customer.TelegramID))
		} else {
			customer = h.syncCustomerStateFromPanelState(ctx, customer, panelState)
		}
	}

	referralEnabled := settings.Features["referrals"] && (config.GetReferralDays() > 0 || config.ReferralTrafficBonusBytes() > 0)
	referralCount := 0
	if referralEnabled {
		count, err := h.referralRepository.CountGrantedByReferrer(ctx, customer.TelegramID)
		if err != nil {
			return nil, err
		}
		referralCount = count
	}

	allowedMethods, err := h.availablePaymentMethods(ctx, customer)
	if err != nil {
		return nil, err
	}

	highestPurchase, err := h.purchaseRepository.FindHighestSuccessfulPurchaseByCustomer(ctx, customer.ID)
	if err != nil {
		return nil, err
	}
	trialEligible := bootstrapTrialEligible(settings.Trial, customer, highestPurchase, panelState, panelLookupFailed, fast)

	supportData := supportPayload{
		IsAdmin:        h.isAdmin(sess.User.ID),
		OpenTickets:    []supportTicketPayload{},
		HistoryTickets: []supportTicketPayload{},
	}
	if !fast && settings.Features["support"] {
		supportData, err = h.buildSupportPayload(ctx, sess, customer, highestPurchase)
		if err != nil {
			return nil, err
		}
	}

	serverStatus := serversPayload{Items: []serverNodePayload{}}
	if !fast && settings.Features["server_status"] {
		serverCtx, serverCancel := context.WithTimeout(ctx, 3*time.Second)
		serverStatus, err = h.buildServersPayload(serverCtx)
		serverCancel()
		if err != nil {
			slog.Warn("mini app: load server status failed", "error", err)
			serverStatus = serversPayload{Items: []serverNodePayload{}}
		}
	}

	reviewsData := reviewsPayload{Items: []reviewItemPayload{}}
	var reviewsErr error
	if settings.Features["reviews"] {
		reviewsData, reviewsErr = h.buildReviewsPayload(ctx, customer)
	}
	if reviewsErr != nil {
		slog.Warn("mini app: load reviews failed", "error", reviewsErr, "telegramId", utils.MaskHalfInt64(customer.TelegramID))
		reviewsData = reviewsPayload{
			Count:              0,
			Average:            0,
			CanCreate:          true,
			RewardDays:         reviewRewardDays,
			RewardTrafficBytes: reviewRewardTrafficBytes,
			Items:              []reviewItemPayload{},
		}
	}

	paymentsData, err := h.buildPaymentsPayload(ctx, customer)
	if err != nil {
		slog.Warn("mini app: load payments failed", "error", err, "telegramId", utils.MaskHalfInt64(customer.TelegramID))
		paymentsData = paymentsPayload{
			Enabled: config.EnableAutoPayment(),
			History: []paymentHistoryPayload{},
		}
	}

	var adminData *adminPayload
	if h.isAdmin(sess.User.ID) {
		adminData, err = h.buildAdminPayload(ctx)
		if err != nil {
			return nil, err
		}
	}

	if !fast && panelState != nil {
		panelState, err = h.syncSubscriptionTrafficLimit(ctx, customer, highestPurchase, panelState)
		if err != nil {
			slog.Warn("mini app: sync traffic limit failed", "error", err, "telegramId", utils.MaskHalfInt64(customer.TelegramID))
		}
	}

	subscriptionData := h.buildSubscriptionPayload(customer, highestPurchase, panelState)
	if subscriptionData.Status == "active" && highestPurchase == nil && reviewsData.MyReview != nil && reviewsData.MyReview.RewardGranted {
		subscriptionData.IsTrial = false
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(customer.Language)), "en") {
			subscriptionData.PlanLabel = "Bonus"
		} else {
			subscriptionData.PlanLabel = "Бонус"
		}
	}

	return &bootstrapResponse{
		Brand: brandPayload{
			Name:    settings.Content.BrandName,
			LogoURL: settings.Content.LogoURL,
		},
		User: userPayload{
			ID:        customer.TelegramID,
			FirstName: sess.User.FirstName,
			Username:  sess.User.Username,
			PanelUsername: func() string {
				if panelState == nil {
					return ""
				}
				return strings.TrimSpace(panelState.PanelUsername)
			}(),
			PhotoURL:       sess.User.PhotoURL,
			LanguageCode:   customer.Language,
			AuthProvider:   fallbackText(sess.Provider, sessionProviderTelegram),
			GoogleEmail:    customerGoogleEmail(customer),
			GoogleLinked:   customerGoogleSubject(customer) != "",
			TelegramLinked: customer.TelegramID > 0,
		},
		Subscription: subscriptionData,
		Trial: trialPayload{
			Enabled:  settings.Features["trials"] && settings.Trial.Enabled && settings.Trial.Days > 0,
			Eligible: settings.Features["trials"] && trialEligible,
			Days:     settings.Trial.Days,
		},
		Referral: referralPayload{
			Enabled:           referralEnabled,
			Count:             referralCount,
			BonusDays:         config.GetReferralDays(),
			BonusTrafficBytes: int64(config.ReferralTrafficBonusBytes()),
			ShareURL:          buildReferralShareURL(customer.TelegramID),
		},
		Reviews:        reviewsData,
		Servers:        serverStatus,
		Support:        supportData,
		Payments:       paymentsData,
		Admin:          adminData,
		Plans:          h.buildPlans(),
		PaymentMethods: mapPaymentMethods(allowedMethods),
		Links: linksPayload{
			Support: h.runtimeLink("support", config.SupportURL()),
			Channel: h.runtimeLink("channel", config.ChannelURL()),
		},
		Meta: metaPayload{
			Now:                    time.Now().UTC().Format(time.RFC3339),
			BotURL:                 config.BotURL(),
			MiniAppURL:             config.GetMiniAppURL(),
			GoogleClientID:         enabledValue(settings.Features["google"], config.GoogleClientID()),
			StarsNeedPriorPurchase: config.RequirePaidPurchaseForStars(),
		},
		Runtime: settings,
	}, nil
}

func (h *Handler) syncSubscriptionTrafficLimit(ctx context.Context, customer *database.Customer, highestPurchase *database.Purchase, panelState *remnawave.UserState) (*remnawave.UserState, error) {
	if h.remnawaveClient == nil || customer == nil || panelState == nil || !panelState.Exists {
		return panelState, nil
	}

	if customer.ExpireAt == nil || !customer.ExpireAt.After(time.Now().UTC()) {
		return panelState, nil
	}

	trial := h.trialSettings()
	expectedLimitBytes := int64(-1)
	switch {
	case highestPurchase != nil && highestPurchase.TrafficLimitBytes != nil:
		expectedLimitBytes = *highestPurchase.TrafficLimitBytes
	case highestPurchase != nil && highestPurchase.Month > 0:
		if plan, ok := h.checkoutPlanForRequest("", highestPurchase.Month); ok {
			expectedLimitBytes = plan.TrafficLimitBytes
		} else {
			expectedLimitBytes = int64(config.TrafficLimitForMonths(highestPurchase.Month))
		}
	case customer.TrialUsed:
		expectedLimitBytes = trialTrafficLimitBytes(trial)
	default:
		return panelState, nil
	}

	currentLimitBytes := maxInt64(panelState.TrafficLimitBytes, 0)
	expectedDeviceLimit := -1
	switch {
	case highestPurchase != nil && highestPurchase.DeviceLimitCount != nil:
		expectedDeviceLimit = *highestPurchase.DeviceLimitCount
	case highestPurchase != nil && highestPurchase.Month > 0:
		if plan, ok := h.checkoutPlanForRequest("", highestPurchase.Month); ok {
			expectedDeviceLimit = plan.DeviceLimitCount
		} else {
			expectedDeviceLimit = config.DeviceLimitForMonths(highestPurchase.Month)
		}
	case customer.TrialUsed:
		expectedDeviceLimit = trial.DeviceLimit
	}

	currentDeviceLimit := maxInt(panelState.DeviceLimit, 0)
	needsTrafficRepair := false
	switch {
	case expectedLimitBytes == 0 && currentLimitBytes > 0:
		needsTrafficRepair = true
	case expectedLimitBytes > 0 && currentLimitBytes > 0 && currentLimitBytes < expectedLimitBytes:
		needsTrafficRepair = true
	case expectedLimitBytes > 0 && currentLimitBytes < 0:
		needsTrafficRepair = true
	}

	needsDeviceRepair := false
	switch {
	case expectedDeviceLimit == 0 && currentDeviceLimit > 0:
		needsDeviceRepair = true
	case expectedDeviceLimit > 0 && currentDeviceLimit > 0 && currentDeviceLimit < expectedDeviceLimit:
		needsDeviceRepair = true
	case expectedDeviceLimit > 0 && currentDeviceLimit < 0:
		needsDeviceRepair = true
	}

	if !needsTrafficRepair && !needsDeviceRepair {
		return panelState, nil
	}

	slog.Info(
		"mini app: repairing panel traffic limit",
		"telegramId", utils.MaskHalfInt64(customer.TelegramID),
		"currentLimitBytes", currentLimitBytes,
		"expectedLimitBytes", expectedLimitBytes,
		"currentDeviceLimit", currentDeviceLimit,
		"expectedDeviceLimit", expectedDeviceLimit,
	)

	if _, err := h.remnawaveClient.CreateOrUpdateUser(ctx, customer.ID, customer.TelegramID, int(expectedLimitBytes), expectedDeviceLimit, 0, false); err != nil {
		return panelState, err
	}

	refreshedState, err := h.remnawaveClient.GetUserStateByTelegramID(ctx, customer.TelegramID)
	if err != nil {
		return panelState, err
	}
	if refreshedState == nil {
		return panelState, nil
	}

	return refreshedState, nil
}

func (h *Handler) buildSubscriptionPayload(customer *database.Customer, highestPurchase *database.Purchase, panelState *remnawave.UserState) subscriptionPayload {
	payload := subscriptionPayload{Status: "inactive", Devices: []devicePayload{}}
	language := ""
	if customer != nil {
		language = customer.Language
	}
	resolvedPlanMonths := h.resolveSubscriptionPlanMonths(highestPurchase, panelState)

	if panelState != nil {
		if panelState.UserUUID != uuid.Nil {
			payload.UserUUID = panelState.UserUUID.String()
		}
		payload.TrafficUsedBytes = maxInt64(panelState.UsedTrafficBytes, 0)
		payload.TrafficLimitBytes = maxInt64(panelState.TrafficLimitBytes, 0)
		payload.DeviceUsedCount = maxInt(panelState.UsedDevices, 0)
		payload.DeviceLimitCount = maxInt(panelState.DeviceLimit, 0)
		payload.Devices = buildDevicePayloads(panelState.Devices)
	}
	if customer.ExpireAt == nil || !customer.ExpireAt.After(time.Now()) {
		return payload
	}

	payload.Status = "active"
	payload.DaysLeft = daysLeft(*customer.ExpireAt)
	payload.ExpiresAt = customer.ExpireAt.UTC().Format(time.RFC3339)
	if resolvedPlanMonths > 0 {
		payload.PlanMonths = resolvedPlanMonths
		payload.PlanLabel = planLabelForPurchase(highestPurchase, resolvedPlanMonths, language)
	} else if h.isTrialSubscription(customer, panelState) {
		payload.IsTrial = true
	}
	payload.HasAccessLink = customer.SubscriptionLink != nil && strings.TrimSpace(*customer.SubscriptionLink) != ""
	if payload.HasAccessLink {
		payload.SubscriptionLink = *customer.SubscriptionLink
	}
	fallbackTrafficLimit := int64(config.TrafficLimit())
	fallbackDeviceLimit := config.DeviceLimitForMonths(1)
	if payload.IsTrial {
		trial := h.trialSettings()
		fallbackTrafficLimit = trialTrafficLimitBytes(trial)
		fallbackDeviceLimit = trial.DeviceLimit
	} else if highestPurchase != nil && highestPurchase.TrafficLimitBytes != nil {
		fallbackTrafficLimit = *highestPurchase.TrafficLimitBytes
		if highestPurchase.DeviceLimitCount != nil {
			fallbackDeviceLimit = *highestPurchase.DeviceLimitCount
		}
	} else if resolvedPlanMonths > 0 {
		if plan, ok := h.checkoutPlanForRequest("", resolvedPlanMonths); ok {
			fallbackTrafficLimit = plan.TrafficLimitBytes
			fallbackDeviceLimit = plan.DeviceLimitCount
		} else {
			fallbackTrafficLimit = int64(config.TrafficLimitForMonths(resolvedPlanMonths))
			fallbackDeviceLimit = config.DeviceLimitForMonths(resolvedPlanMonths)
		}
	}
	if panelState == nil {
		payload.TrafficLimitBytes = maxInt64(fallbackTrafficLimit, 0)
	} else if payload.TrafficLimitBytes < 0 {
		payload.TrafficLimitBytes = maxInt64(fallbackTrafficLimit, 0)
	}
	if panelState == nil {
		payload.DeviceLimitCount = maxInt(fallbackDeviceLimit, 0)
	} else if payload.DeviceLimitCount < 0 {
		payload.DeviceLimitCount = maxInt(fallbackDeviceLimit, 0)
	}

	return payload
}

// Compatibility wrappers keep the pure helpers available for existing tests and
// callers that intentionally use legacy defaults without a runtime service.
func buildSubscriptionPayload(customer *database.Customer, highestPurchase *database.Purchase, panelState *remnawave.UserState) subscriptionPayload {
	return (&Handler{}).buildSubscriptionPayload(customer, highestPurchase, panelState)
}

func (h *Handler) resolveSubscriptionPlanMonths(highestPurchase *database.Purchase, panelState *remnawave.UserState) int {
	if highestPurchase != nil && highestPurchase.Month > 0 {
		return highestPurchase.Month
	}

	return h.inferPlanMonthsFromPanelState(panelState)
}

func resolveSubscriptionPlanMonths(highestPurchase *database.Purchase, panelState *remnawave.UserState) int {
	return (&Handler{}).resolveSubscriptionPlanMonths(highestPurchase, panelState)
}

func (h *Handler) inferPlanMonthsFromPanelState(panelState *remnawave.UserState) int {
	if panelState == nil || !panelState.Exists {
		return 0
	}

	trafficLimitBytes := maxInt64(panelState.TrafficLimitBytes, 0)
	deviceLimit := panelState.DeviceLimit

	for _, plan := range h.checkoutPlans() {
		if trafficLimitBytes == plan.TrafficLimitBytes && deviceLimit == plan.DeviceLimitCount {
			return plan.Months
		}
	}

	return 0
}

func (h *Handler) isTrialPanelState(panelState *remnawave.UserState) bool {
	if panelState == nil || !panelState.Exists {
		return false
	}

	trial := h.trialSettings()
	return maxInt64(panelState.TrafficLimitBytes, 0) == trialTrafficLimitBytes(trial) &&
		maxInt(panelState.DeviceLimit, 0) == trial.DeviceLimit
}

func (h *Handler) isTrialSubscription(customer *database.Customer, panelState *remnawave.UserState) bool {
	if h.isTrialPanelState(panelState) {
		return true
	}

	return customer != nil && customer.TrialUsed
}

func buildDevicePayloads(devices []remnawave.UserDevice) []devicePayload {
	if len(devices) == 0 {
		return []devicePayload{}
	}

	payload := make([]devicePayload, 0, len(devices))
	for _, device := range devices {
		payload = append(payload, devicePayload{
			Hwid:        strings.TrimSpace(device.Hwid),
			Platform:    strings.TrimSpace(device.Platform),
			OSVersion:   strings.TrimSpace(device.OSVersion),
			DeviceModel: strings.TrimSpace(device.DeviceModel),
			UserAgent:   strings.TrimSpace(device.UserAgent),
			CreatedAt:   formatOptionalTime(device.CreatedAt),
			UpdatedAt:   formatOptionalTime(device.UpdatedAt),
		})
	}

	return payload
}

func (h *Handler) syncCustomerState(ctx context.Context, customer *database.Customer) *database.Customer {
	if h.remnawaveClient == nil || customer == nil {
		return customer
	}

	panelState, err := h.remnawaveClient.GetUserStateByTelegramID(ctx, customer.TelegramID)
	if err != nil {
		slog.Warn("mini app: remnawave sync failed", "error", err, "telegramId", utils.MaskHalfInt64(customer.TelegramID))
		return customer
	}
	return h.syncCustomerStateFromPanelState(ctx, customer, panelState)
}

func (h *Handler) syncCustomerStateFromPanelState(ctx context.Context, customer *database.Customer, panelState *remnawave.UserState) *database.Customer {
	if customer == nil {
		return customer
	}

	updates := map[string]any{}
	if panelState != nil && panelState.Exists && !customer.TrialUsed && h.isTrialPanelState(panelState) {
		updates["trial_used"] = true
	}
	if panelState == nil || !panelState.Active {
		if customer.ExpireAt != nil {
			updates["expire_at"] = nil
		}
		if customer.SubscriptionLink != nil {
			updates["subscription_link"] = nil
		}
	} else {
		if !timePtrEqual(customer.ExpireAt, panelState.ExpireAt) {
			updates["expire_at"] = panelState.ExpireAt
		}
		if !stringPtrEqual(customer.SubscriptionLink, panelState.SubscriptionLink) {
			updates["subscription_link"] = panelState.SubscriptionLink
		}
	}

	if len(updates) == 0 {
		return customer
	}

	if err := h.customerRepository.UpdateFields(ctx, customer.ID, updates); err != nil {
		slog.Warn("mini app: failed to persist synced customer state", "error", err, "telegramId", utils.MaskHalfInt64(customer.TelegramID))
		return customer
	}

	if value, ok := updates["expire_at"]; ok {
		if value == nil {
			customer.ExpireAt = nil
		} else if expireAt, ok := value.(*time.Time); ok {
			customer.ExpireAt = expireAt
		}
	}

	if value, ok := updates["subscription_link"]; ok {
		if value == nil {
			customer.SubscriptionLink = nil
		} else if link, ok := value.(*string); ok {
			customer.SubscriptionLink = link
		}
	}
	if value, ok := updates["trial_used"]; ok {
		if trialUsed, ok := value.(bool); ok {
			customer.TrialUsed = trialUsed
		}
	}

	return customer
}

func bootstrapTrialEligible(trial runtimeconfig.TrialSettings, customer *database.Customer, highestPurchase *database.Purchase, panelState *remnawave.UserState, panelLookupFailed, fast bool) bool {
	if !trial.Enabled || trial.Days == 0 || customer == nil || customer.TrialUsed || highestPurchase != nil {
		return false
	}
	if customer.ExpireAt != nil && customer.ExpireAt.After(time.Now()) {
		return false
	}
	if !fast && panelLookupFailed {
		return false
	}
	return fast || panelState == nil || !panelState.Exists
}

func (h *Handler) canActivateTrial(ctx context.Context, customer *database.Customer) (bool, error) {
	trial := runtimeconfig.DefaultSettings().Trial
	if h.runtimeSettings != nil {
		trial = h.runtimeSettings.TrialSettings()
	}
	if !trial.Enabled || trial.Days == 0 || customer == nil || customer.TrialUsed {
		return false, nil
	}

	if customer.ExpireAt != nil && customer.ExpireAt.After(time.Now()) {
		return false, nil
	}

	purchase, err := h.purchaseRepository.FindSuccessfulPurchaseByCustomer(ctx, customer.ID)
	if err != nil {
		return false, err
	}

	if h.remnawaveClient != nil {
		panelState, err := h.remnawaveClient.GetUserStateByTelegramID(ctx, customer.TelegramID)
		if err != nil {
			return false, err
		}
		if panelState != nil && panelState.Exists {
			return false, nil
		}
	}

	return purchase == nil, nil
}

func (h *Handler) buildServersPayload(ctx context.Context) (serversPayload, error) {
	if h.remnawaveClient == nil {
		return serversPayload{Items: []serverNodePayload{}}, nil
	}

	nodes, err := h.remnawaveClient.GetNodesStatus(ctx)
	if err != nil {
		return serversPayload{}, err
	}

	items := make([]serverNodePayload, 0, len(nodes))
	for _, node := range nodes {
		items = append(items, serverNodePayload{
			Name:        node.Name,
			Address:     node.Address,
			CountryCode: node.CountryCode,
			Online:      node.IsOnline,
		})
	}

	return serversPayload{Items: items}, nil
}

func (h *Handler) buildReviewsPayload(ctx context.Context, customer *database.Customer) (reviewsPayload, error) {
	result := reviewsPayload{
		Count:              0,
		Average:            0,
		CanCreate:          true,
		RewardDays:         reviewRewardDays,
		RewardTrafficBytes: reviewRewardTrafficBytes,
		Items:              []reviewItemPayload{},
	}
	if h.reviewRepository == nil || customer == nil {
		return result, nil
	}

	summary, err := h.reviewRepository.GetSummary(ctx)
	if err != nil {
		return result, err
	}
	if summary != nil {
		result.Count = maxInt(summary.Count, 0)
		result.Average = roundRating(summary.Average)
	}

	items, err := h.reviewRepository.ListLatest(ctx, reviewListLimit)
	if err != nil {
		return result, err
	}

	payload := make([]reviewItemPayload, 0, len(items))
	for _, item := range items {
		mapped := reviewItemPayload{
			ID:            item.ID,
			Username:      strings.TrimSpace(item.TelegramUsername),
			Rating:        item.Rating,
			Comment:       strings.TrimSpace(item.Comment),
			CreatedAt:     formatOptionalTime(item.CreatedAt),
			RewardGranted: item.RewardGranted,
			IsMine:        item.CustomerID == customer.ID,
		}
		if mapped.Username == "" {
			mapped.Username = "user"
		}
		payload = append(payload, mapped)
		if mapped.IsMine {
			itemCopy := mapped
			result.MyReview = &itemCopy
			result.CanCreate = false
		}
	}
	result.Items = payload

	if result.MyReview == nil {
		myReview, err := h.reviewRepository.FindAnyByCustomerID(ctx, customer.ID)
		if err != nil {
			return result, err
		}
		if myReview != nil {
			result.CanCreate = false
			result.MyReview = &reviewItemPayload{
				ID:            myReview.ID,
				Username:      strings.TrimSpace(myReview.TelegramUsername),
				Rating:        myReview.Rating,
				Comment:       strings.TrimSpace(myReview.Comment),
				CreatedAt:     formatOptionalTime(myReview.CreatedAt),
				RewardGranted: myReview.RewardGranted,
				IsMine:        true,
			}
		}
	}

	return result, nil
}

func (h *Handler) buildPaymentsPayload(ctx context.Context, customer *database.Customer) (paymentsPayload, error) {
	result := paymentsPayload{
		Enabled: config.EnableAutoPayment(),
		History: []paymentHistoryPayload{},
	}
	if customer == nil {
		return result, nil
	}

	realPaymentMethod := customer.YookasaPaymentMethodID != nil && *customer.YookasaPaymentMethodID != uuid.Nil
	result.HasPaymentMethod = realPaymentMethod
	result.AutoPaymentEnabled = config.EnableAutoPayment() && realPaymentMethod && customer.AutoPaymentEnabled

	latestYookasaPurchase, err := h.purchaseRepository.FindLatestSuccessfulYookasaPurchaseByCustomer(ctx, customer.ID)
	if err != nil {
		return result, err
	}

	if customer.AutoPaymentPlanMonths != nil && *customer.AutoPaymentPlanMonths > 0 {
		result.AutoPaymentPlanMonths = *customer.AutoPaymentPlanMonths
	} else if latestYookasaPurchase != nil && latestYookasaPurchase.Month > 0 {
		result.AutoPaymentPlanMonths = latestYookasaPurchase.Month
	}

	if realPaymentMethod {
		methodTitle := strings.TrimSpace(valueOrEmpty(customer.YookasaPaymentMethodTitle))
		if methodTitle == "" && latestYookasaPurchase != nil {
			methodTitle = paymentMethodTitleForPurchase(latestYookasaPurchase, customer.Language)
		}
		if methodTitle == "" {
			methodTitle = paymentMethodFallbackTitle(database.InvoiceTypeYookasa, customer.Language)
		}

		result.Method = &savedPaymentMethodPayload{
			Title:   methodTitle,
			Type:    strings.TrimSpace(valueOrEmpty(customer.YookasaPaymentMethodType)),
			SavedAt: formatOptionalTime(timeOrNil(customer.YookasaPaymentMethodSavedAt)),
		}
	} else if config.PaymentMethodDemoEnabled() {
		methodTitle := strings.TrimSpace(config.PaymentMethodDemoTitle())
		if methodTitle == "" {
			methodTitle = paymentMethodFallbackTitle(database.InvoiceTypeYookasa, customer.Language)
		}
		result.HasPaymentMethod = true
		result.Method = &savedPaymentMethodPayload{
			Title: methodTitle,
			Type:  strings.TrimSpace(config.PaymentMethodDemoType()),
			Demo:  true,
		}
	}

	purchases, err := h.purchaseRepository.ListByCustomer(ctx, customer.ID, 30)
	if err != nil {
		return result, err
	}

	history := make([]paymentHistoryPayload, 0, len(purchases))
	for _, purchase := range purchases {
		history = append(history, paymentHistoryPayload{
			ID:                 purchase.ID,
			Months:             purchase.Month,
			PlanLabel:          planLabelForPurchase(&purchase, purchase.Month, customer.Language),
			Amount:             purchase.Amount,
			Currency:           strings.TrimSpace(purchase.Currency),
			Status:             string(purchase.Status),
			InvoiceType:        string(purchase.InvoiceType),
			PaymentMethodTitle: paymentMethodTitleForPurchase(&purchase, customer.Language),
			IsAutoPayment:      purchase.IsAutoPayment,
			CreatedAt:          formatOptionalTime(purchase.CreatedAt),
			PaidAt:             formatOptionalTime(timeOrNil(purchase.PaidAt)),
		})
	}
	result.History = history

	return result, nil
}

func (h *Handler) buildAdminPayload(ctx context.Context) (*adminPayload, error) {
	payload := &adminPayload{
		PromoCodes:   []promoCodePayload{},
		Settings:     runtimeconfig.DefaultSettings(),
		Events:       []database.OperationalEvent{},
		Integrations: []integrations.ProviderView{},
		Squads:       remnawave.SquadCatalog{Internal: []remnawave.SquadOption{}, External: []remnawave.SquadOption{}},
	}
	if h.integrationSettings != nil {
		payload.Integrations = h.integrationSettings.ListAdmin()
	}
	if h.runtimeSettings != nil {
		payload.Settings = h.runtimeSettings.Snapshot()
	}
	if h.remnawaveClient != nil {
		squads, err := h.remnawaveClient.ListSquads(ctx)
		if err != nil {
			slog.Warn("mini app: load squads for admin failed", "error", err)
		} else {
			payload.Squads = squads
		}
	}
	if h.errorReporter != nil {
		events, err := h.errorReporter.List(ctx, 60, false)
		if err != nil {
			return nil, err
		}
		payload.Events = events
	}

	if h.promoCodeRepository != nil {
		items, err := h.promoCodeRepository.ListLatest(ctx, 40)
		if err != nil {
			return nil, err
		}
		payload.PromoCodes = make([]promoCodePayload, 0, len(items))
		now := time.Now().UTC()
		for _, item := range items {
			status := promoStatus(&item, now)
			if item.MaxRedemptions != nil {
				reserved, err := h.purchaseRepository.CountActivePromoReservations(ctx, item.ID)
				if err != nil {
					return nil, err
				}
				if item.RedemptionCount+reserved >= *item.MaxRedemptions {
					status = "exhausted"
				}
			}
			payload.PromoCodes = append(payload.PromoCodes, promoCodePayload{
				ID:              item.ID,
				Code:            item.Code,
				DiscountPercent: item.DiscountPercent,
				ExpiresAt:       formatOptionalTime(timeOrNil(item.ExpiresAt)),
				MaxRedemptions:  optionalIntValue(item.MaxRedemptions),
				RedemptionCount: item.RedemptionCount,
				Status:          status,
				CreatedAt:       item.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
	}

	return payload, nil
}

type checkoutPlan = planbook.CheckoutPlan

func (h *Handler) trialSettings() runtimeconfig.TrialSettings {
	if h.runtimeSettings != nil {
		return h.runtimeSettings.TrialSettings()
	}
	return runtimeconfig.DefaultSettings().Trial
}

func trialTrafficLimitBytes(trial runtimeconfig.TrialSettings) int64 {
	if trial.UnlimitedTraffic || trial.TrafficGB <= 0 {
		return 0
	}
	return int64(trial.TrafficGB) * 1024 * 1024 * 1024
}

func (h *Handler) checkoutPlans() []checkoutPlan {
	if h.runtimeSettings != nil {
		return h.runtimeSettings.CheckoutPlans()
	}
	return planbook.All()
}

func (h *Handler) checkoutPlanForRequest(planID string, months int) (checkoutPlan, bool) {
	if h.runtimeSettings != nil {
		return h.runtimeSettings.CheckoutPlan(planID, months)
	}
	return planbook.ForIDOrMonths(planID, months)
}

func checkoutAmountForPlan(plan checkoutPlan, invoiceType database.InvoiceType) (int, bool) {
	return planbook.AmountForInvoice(plan, invoiceType)
}

func (h *Handler) resolvePromoCode(ctx context.Context, customerID int64, rawCode string) (*database.PromoCode, string, string, error) {
	if h.promoCodeRepository == nil {
		return nil, "", "promo_unavailable", nil
	}

	code := database.NormalizePromoCode(rawCode)
	if code == "" {
		return nil, "", "promo_code_required", nil
	}
	if !database.IsValidPromoCode(code) {
		return nil, "", "promo_invalid_format", nil
	}

	promo, err := h.promoCodeRepository.FindByCode(ctx, code)
	if err != nil {
		return nil, "", "", err
	}
	if promo == nil {
		return nil, code, "promo_not_found", nil
	}
	if !promo.IsActive {
		return nil, code, "promo_inactive", nil
	}
	if promo.ExpiresAt != nil && !promo.ExpiresAt.After(time.Now().UTC()) {
		return nil, code, "promo_expired", nil
	}
	if customerID > 0 {
		used, err := h.promoCodeRepository.HasCustomerRedemption(ctx, promo.ID, customerID)
		if err != nil {
			return nil, "", "", err
		}
		if used {
			return nil, code, "promo_already_used", nil
		}
		pending, err := h.purchaseRepository.HasPendingPromoPurchaseByCustomer(ctx, customerID, promo.ID)
		if err != nil {
			return nil, "", "", err
		}
		if pending {
			return nil, code, "promo_pending", nil
		}
	}
	if promo.MaxRedemptions != nil {
		reserved, err := h.purchaseRepository.CountActivePromoReservations(ctx, promo.ID)
		if err != nil {
			return nil, "", "", err
		}
		if promo.RedemptionCount+reserved >= *promo.MaxRedemptions {
			return nil, code, "promo_limit_reached", nil
		}
	}

	return promo, code, "", nil
}

func promoStatus(promo *database.PromoCode, now time.Time) string {
	if promo == nil {
		return "inactive"
	}
	if !promo.IsActive {
		return "inactive"
	}
	if promo.ExpiresAt != nil && !promo.ExpiresAt.After(now) {
		return "expired"
	}
	if promo.MaxRedemptions != nil && promo.RedemptionCount >= *promo.MaxRedemptions {
		return "exhausted"
	}
	return "active"
}

func parsePromoExpiry(value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			utc := parsed.UTC()
			return &utc, nil
		}
	}

	return nil, fmt.Errorf("invalid promo expiry: %s", trimmed)
}

func applyDiscount(amount int, discountPercent int) int {
	if amount <= 0 {
		return 0
	}
	if discountPercent <= 0 {
		return amount
	}

	discounted := int(math.Round(float64(amount) * float64(100-discountPercent) / 100))
	if discounted < 1 {
		return 1
	}
	return discounted
}

func promoErrorMessage(code string) string {
	switch code {
	case "promo_code_required":
		return "Promo code is required"
	case "promo_invalid_format":
		return "Promo code format is invalid"
	case "promo_not_found":
		return "Promo code was not found"
	case "promo_expired":
		return "Promo code has expired"
	case "promo_inactive":
		return "Promo code is inactive"
	case "promo_limit_reached":
		return "Promo code limit reached"
	case "promo_already_used":
		return "Promo code already used"
	case "promo_pending":
		return "Promo code is already attached to a pending purchase"
	case "promo_invalid_discount":
		return "Discount percent is invalid"
	case "promo_invalid_expiry":
		return "Promo expiry date is invalid"
	case "promo_invalid_limit":
		return "Promo usage limit is invalid"
	case "promo_already_exists":
		return "Promo code already exists"
	case "promo_create_failed":
		return "Failed to create promo code"
	case "promo_delete_failed":
		return "Failed to delete promo code"
	case "promo_unavailable":
		return "Promo codes are temporarily unavailable"
	default:
		return "Promo code is unavailable"
	}
}

func optionalIntValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func promoCodeIDOrNil(promo *database.PromoCode) *int64 {
	if promo == nil || promo.ID == 0 {
		return nil
	}
	id := promo.ID
	return &id
}

func promoDiscountPercentOrZero(promo *database.PromoCode) int {
	if promo == nil {
		return 0
	}
	return promo.DiscountPercent
}

func planLabelForMonths(months int, language string) string {
	isEnglish := strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en")
	switch months {
	case 1:
		if isEnglish {
			return "1 month"
		}
		return "Месяц"
	case 3:
		if isEnglish {
			return "3 months"
		}
		return "3 Месяца"
	case 6:
		if isEnglish {
			return "6 months"
		}
		return "6 Месяцев"
	case 12:
		if isEnglish {
			return "Annual"
		}
		return "Годовой"
	default:
		if months <= 0 {
			if isEnglish {
				return "Purchase"
			}
			return "Покупка"
		}
		if isEnglish {
			return fmt.Sprintf("%d months", months)
		}
		return fmt.Sprintf("%d мес.", months)
	}
}

func planLabelForPurchase(purchase *database.Purchase, months int, language string) string {
	label := planLabelForMonths(months, language)
	if purchase == nil || purchase.PlanID == nil || !strings.Contains(strings.ToLower(strings.TrimSpace(*purchase.PlanID)), planbook.VariantUnlimited) {
		return label
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en") {
		return label + " · Unlimited"
	}
	return label + " · Безлимит"
}

func paymentMethodTitleForPurchase(purchase *database.Purchase, language string) string {
	if purchase == nil {
		return ""
	}

	if title := strings.TrimSpace(valueOrEmpty(purchase.YookasaPaymentMethodTitle)); title != "" {
		return title
	}
	if methodType := strings.TrimSpace(valueOrEmpty(purchase.YookasaPaymentMethodType)); methodType != "" && purchase.InvoiceType != database.InvoiceTypeYookasa {
		return methodType
	}
	return paymentMethodFallbackTitle(purchase.InvoiceType, language)
}

func paymentMethodFallbackTitle(invoiceType database.InvoiceType, language string) string {
	isEnglish := strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en")
	switch invoiceType {
	case database.InvoiceTypeYookasa:
		if isEnglish {
			return "Bank card"
		}
		return "Банковская карта"
	case database.InvoiceTypeTelegram:
		if isEnglish {
			return "Telegram Stars"
		}
		return "Telegram Stars"
	case database.InvoiceTypeCrypto:
		if isEnglish {
			return "Crypto Pay"
		}
		return "Crypto Pay"
	case database.InvoiceTypeTribute:
		if isEnglish {
			return "Tribute"
		}
		return "Tribute"
	default:
		if isEnglish {
			return "Payment"
		}
		return "Оплата"
	}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func timeOrNil(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func (h *Handler) grantReviewReward(ctx context.Context, customer *database.Customer, reviewID int64) error {
	if h.reviewRepository == nil || h.remnawaveClient == nil || customer == nil {
		return fmt.Errorf("review reward is unavailable")
	}

	review, err := h.reviewRepository.FindAnyByCustomerID(ctx, customer.ID)
	if err != nil {
		return err
	}
	if review == nil {
		return fmt.Errorf("review %d not found", reviewID)
	}
	if review.RewardGranted {
		return nil
	}

	panelState, err := h.remnawaveClient.GetUserStateByTelegramID(ctx, customer.TelegramID)
	if err != nil {
		return err
	}

	trafficLimit := int(reviewRewardTrafficBytes)
	deviceLimit := config.DeviceLimitForMonths(1)
	if panelState != nil && panelState.Exists {
		if panelState.Active {
			if maxInt64(panelState.TrafficLimitBytes, 0) <= 0 {
				trafficLimit = 0
			} else {
				trafficLimit = mergeReviewTrafficLimit(panelState.TrafficLimitBytes)
			}
			deviceLimit = normalizeReviewDeviceLimit(panelState.DeviceLimit, true)
		} else {
			trafficLimit = mergeReviewTrafficLimit(maxInt64(panelState.TrafficLimitBytes, 0))
			deviceLimit = normalizeReviewDeviceLimit(panelState.DeviceLimit, false)
		}
	}

	user, err := h.remnawaveClient.CreateOrUpdateUser(ctx, customer.ID, customer.TelegramID, trafficLimit, deviceLimit, reviewRewardDays, false)
	if err != nil {
		return err
	}

	updates := map[string]any{
		"subscription_link": user.SubscriptionUrl,
		"expire_at":         user.ExpireAt,
	}
	if err := h.customerRepository.UpdateFields(ctx, customer.ID, updates); err != nil {
		return err
	}
	if err := h.reviewRepository.MarkRewardGranted(ctx, reviewID, reviewRewardDays, reviewRewardTrafficBytes); err != nil {
		return err
	}

	customer.SubscriptionLink = &user.SubscriptionUrl
	expireAt := user.ExpireAt
	customer.ExpireAt = &expireAt

	return nil
}

func buildReviewUsername(sess *session) string {
	if sess == nil {
		return "user"
	}

	username := strings.TrimSpace(sess.User.Username)
	if username != "" {
		if strings.HasPrefix(username, "@") {
			return username
		}
		return "@" + username
	}

	firstName := strings.TrimSpace(sess.User.FirstName)
	if firstName != "" {
		return firstName
	}

	return "user"
}

func contextWithSessionTelegramProfile(ctx context.Context, sess *session) context.Context {
	if sess == nil {
		return ctx
	}
	if sess.Provider != sessionProviderTelegram {
		return ctx
	}

	ctx = context.WithValue(ctx, "username", strings.TrimSpace(sess.User.Username))
	ctx = context.WithValue(ctx, "telegramName", buildTelegramDisplayName(sess.User.FirstName, sess.User.LastName))
	return ctx
}

func buildTelegramDisplayName(firstName, lastName string) string {
	return strings.TrimSpace(strings.TrimSpace(firstName) + " " + strings.TrimSpace(lastName))
}

func mergeReviewTrafficLimit(currentBytes int64) int {
	current := maxInt64(currentBytes, 0)
	if current <= 0 {
		return int(reviewRewardTrafficBytes)
	}
	return int(current + reviewRewardTrafficBytes)
}

func normalizeReviewDeviceLimit(currentLimit int, active bool) int {
	if active {
		if currentLimit <= 0 {
			return 0
		}
		return currentLimit
	}

	if currentLimit > 0 {
		return currentLimit
	}

	return config.DeviceLimitForMonths(1)
}

func roundRating(value float64) float64 {
	if value <= 0 {
		return 0
	}
	return math.Round(value*10) / 10
}

func (h *Handler) buildSupportPayload(ctx context.Context, sess *session, customer *database.Customer, highestPurchase *database.Purchase) (supportPayload, error) {
	if h.supportRepository == nil {
		return supportPayload{
			IsAdmin:        h.isAdmin(sess.User.ID),
			OpenTickets:    []supportTicketPayload{},
			HistoryTickets: []supportTicketPayload{},
		}, nil
	}

	isAdmin := h.isAdmin(sess.User.ID)
	var (
		openTickets    []database.SupportTicket
		historyTickets []database.SupportTicket
		err            error
	)

	if isAdmin {
		openTickets, err = h.supportRepository.ListTicketsForAdmin(ctx, database.SupportTicketStatusOpen)
		if err != nil {
			return supportPayload{}, err
		}
		historyTickets, err = h.supportRepository.ListTicketsForAdmin(ctx, database.SupportTicketStatusClosed)
		if err != nil {
			return supportPayload{}, err
		}
	} else {
		openTickets, err = h.supportRepository.ListTicketsByCustomer(ctx, customer.ID, database.SupportTicketStatusOpen)
		if err != nil {
			return supportPayload{}, err
		}
		historyTickets, err = h.supportRepository.ListTicketsByCustomer(ctx, customer.ID, database.SupportTicketStatusClosed)
		if err != nil {
			return supportPayload{}, err
		}
	}

	subscriptionLabel := h.buildSubscriptionLabel(customer, highestPurchase)
	h.enrichSupportTickets(ctx, openTickets)
	h.enrichSupportTickets(ctx, historyTickets)
	return supportPayload{
		IsAdmin:        isAdmin,
		OpenTickets:    h.buildSupportTicketListPayload(openTickets, isAdmin, subscriptionLabel),
		HistoryTickets: h.buildSupportTicketListPayload(historyTickets, isAdmin, subscriptionLabel),
	}, nil
}

func (h *Handler) buildSupportThreadPayload(ctx context.Context, sess *session, customer *database.Customer, ticket *database.SupportTicket) (*supportThreadPayload, error) {
	h.enrichSupportTicket(ctx, ticket)
	messages, err := h.supportRepository.ListMessagesByTicket(ctx, ticket.ID)
	if err != nil {
		return nil, err
	}

	isAdmin := h.isAdmin(sess.User.ID)
	if isAdmin {
		if err := h.supportRepository.MarkSeenByAdmin(ctx, ticket.ID); err != nil {
			slog.Warn("mini app: failed to mark admin ticket as seen", "error", err, "ticketId", ticket.ID)
		}
		ticket.AdminUnreadCount = 0
	} else {
		if err := h.supportRepository.MarkSeenByCustomer(ctx, ticket.ID); err != nil {
			slog.Warn("mini app: failed to mark customer ticket as seen", "error", err, "ticketId", ticket.ID)
		}
		ticket.CustomerUnreadCount = 0
	}

	return &supportThreadPayload{
		Ticket:   h.buildSupportTicketPayload(*ticket, isAdmin, ""),
		Messages: buildSupportMessagePayloads(messages),
		CanReply: ticket.Status == database.SupportTicketStatusOpen,
		CanClose: isAdmin && ticket.Status == database.SupportTicketStatusOpen,
	}, nil
}

func (h *Handler) enrichSupportTickets(ctx context.Context, tickets []database.SupportTicket) {
	for i := range tickets {
		h.enrichSupportTicket(ctx, &tickets[i])
	}
}

func (h *Handler) enrichSupportTicket(ctx context.Context, ticket *database.SupportTicket) {
	if ticket == nil {
		return
	}

	customer, err := h.customerRepository.FindById(ctx, ticket.CustomerID)
	if err != nil || customer == nil {
		return
	}

	highestPurchase, err := h.purchaseRepository.FindHighestSuccessfulPurchaseByCustomer(ctx, customer.ID)
	if err == nil {
		ticket.SubscriptionLabel = h.buildSubscriptionLabel(customer, highestPurchase)
	}
}

func (h *Handler) buildSupportTicketListPayload(tickets []database.SupportTicket, isAdmin bool, subscriptionLabel string) []supportTicketPayload {
	payload := make([]supportTicketPayload, 0, len(tickets))
	for _, ticket := range tickets {
		payload = append(payload, h.buildSupportTicketPayload(ticket, isAdmin, subscriptionLabel))
	}
	return payload
}

func (h *Handler) buildSupportTicketPayload(ticket database.SupportTicket, isAdmin bool, fallbackSubscriptionLabel string) supportTicketPayload {
	subscriptionLabel := strings.TrimSpace(ticket.SubscriptionLabel)
	if subscriptionLabel == "" {
		subscriptionLabel = strings.TrimSpace(fallbackSubscriptionLabel)
	}

	unreadCount := ticket.CustomerUnreadCount
	if isAdmin {
		unreadCount = ticket.AdminUnreadCount
	}

	return supportTicketPayload{
		ID:                ticket.ID,
		Subject:           strings.TrimSpace(ticket.Subject),
		Preview:           strings.TrimSpace(ticket.LastMessagePreview),
		Status:            string(ticket.Status),
		UpdatedAt:         ticket.UpdatedAt.UTC().Format(time.RFC3339),
		CreatedAt:         ticket.CreatedAt.UTC().Format(time.RFC3339),
		UnreadCount:       unreadCount,
		CustomerName:      strings.TrimSpace(ticket.CustomerName),
		CustomerUsername:  strings.TrimSpace(ticket.CustomerUsername),
		SubscriptionLabel: subscriptionLabel,
	}
}

func buildSupportMessagePayloads(messages []database.SupportMessage) []supportMessagePayload {
	payload := make([]supportMessagePayload, 0, len(messages))
	for _, message := range messages {
		payload = append(payload, supportMessagePayload{
			ID:         message.ID,
			AuthorRole: string(message.AuthorRole),
			Body:       message.Body,
			CreatedAt:  message.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return payload
}

func (h *Handler) loadSupportTicketForViewer(ctx context.Context, sess *session, customer *database.Customer, ticketID int64) (*database.SupportTicket, error) {
	if ticketID == 0 || h.supportRepository == nil {
		return nil, nil
	}

	ticket, err := h.supportRepository.FindTicketByID(ctx, ticketID)
	if err != nil || ticket == nil {
		return ticket, err
	}

	if h.isAdmin(sess.User.ID) {
		return ticket, nil
	}
	if customer != nil && ticket.CustomerID == customer.ID {
		return ticket, nil
	}

	return nil, nil
}

func (h *Handler) isAdmin(telegramID int64) bool {
	return telegramID != 0 && telegramID == config.GetAdminTelegramId()
}

func (h *Handler) buildSubscriptionLabel(customer *database.Customer, highestPurchase *database.Purchase) string {
	if customer == nil || customer.ExpireAt == nil || !customer.ExpireAt.After(time.Now()) {
		return "Нет подписки"
	}

	if highestPurchase != nil && highestPurchase.Month > 0 {
		switch highestPurchase.Month {
		case 1:
			return "Месяц"
		case 3:
			return "3 Месяца"
		case 6:
			return "6 Месяцев"
		case 12:
			return "Годовой"
		}
	}

	if customer.TrialUsed {
		return "Пробный"
	}

	return "Подписка"
}

func (h *Handler) resolveSupportPanelUsername(ctx context.Context, customer *database.Customer) string {
	if h.remnawaveClient == nil || customer == nil {
		return ""
	}

	panelState, err := h.remnawaveClient.GetUserStateByTelegramID(ctx, customer.TelegramID)
	if err != nil || panelState == nil {
		return ""
	}

	return strings.TrimSpace(panelState.PanelUsername)
}

func sessionDisplayName(sess *session) string {
	if sess == nil {
		return ""
	}
	name := strings.TrimSpace(strings.TrimSpace(sess.User.FirstName + " " + sess.User.LastName))
	if name != "" {
		return name
	}
	if username := strings.TrimSpace(sess.User.Username); username != "" {
		return "@" + username
	}
	return strconv.FormatInt(sess.User.ID, 10)
}

func (h *Handler) notifyAdminAboutSupportTicket(ctx context.Context, ticket *database.SupportTicket, firstMessage string) {
	if h.telegramBot == nil || config.GetAdminTelegramId() == 0 || ticket == nil {
		return
	}

	text := fmt.Sprintf(
		"🆕 <b>Новое обращение #%d</b>\n\n👤 <b>Пользователь:</b> %s\n🔗 <b>Username:</b> %s\n💎 <b>Подписка:</b> %s\n\n💬 <b>Сообщение:</b>\n%s",
		ticket.ID,
		supportNotificationText(fallbackText(ticket.CustomerName, "Без имени")),
		supportNotificationText(formatUsername(ticket.CustomerUsername)),
		supportNotificationText(fallbackText(ticket.SubscriptionLabel, "Нет подписки")),
		supportNotificationQuote(firstMessage),
	)
	h.sendMiniAppNotification(ctx, config.GetAdminTelegramId(), text)
}

func (h *Handler) notifySupportAsync(fn func(context.Context)) {
	if fn == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		fn(ctx)
	}()
}

func (h *Handler) notifyAdminAboutSupportReply(ctx context.Context, ticket *database.SupportTicket, message string) {
	if h.telegramBot == nil || config.GetAdminTelegramId() == 0 || ticket == nil {
		return
	}

	text := fmt.Sprintf(
		"📩 <b>Обращение #%d</b>\nЕсть новый ответ:\n\n👤 <b>Пользователь:</b> %s\n🔗 <b>Username:</b> %s\n💎 <b>Подписка:</b> %s\n\n%s",
		ticket.ID,
		supportNotificationText(fallbackText(ticket.CustomerName, "Без имени")),
		supportNotificationText(formatUsername(ticket.CustomerUsername)),
		supportNotificationText(fallbackText(ticket.SubscriptionLabel, "Нет подписки")),
		supportNotificationQuote(message),
	)
	h.sendMiniAppNotification(ctx, config.GetAdminTelegramId(), text)
}

func (h *Handler) notifyCustomerAboutSupportReply(ctx context.Context, ticket *database.SupportTicket, message string) {
	if h.telegramBot == nil || ticket == nil {
		return
	}

	customer, err := h.customerRepository.FindById(ctx, ticket.CustomerID)
	if err != nil || customer == nil {
		return
	}

	text := fmt.Sprintf(
		"📬 <b>Обращение #%d</b>\nЕсть ответ:\n\n%s",
		ticket.ID,
		supportNotificationQuote(message),
	)
	h.sendMiniAppNotification(ctx, customer.TelegramID, text)
}

func (h *Handler) notifyCustomerAboutSupportClosed(ctx context.Context, ticket *database.SupportTicket) {
	if h.telegramBot == nil || ticket == nil {
		return
	}

	customer, err := h.customerRepository.FindById(ctx, ticket.CustomerID)
	if err != nil || customer == nil {
		return
	}

	text := fmt.Sprintf("💌 <b>Обращение #%d закрыто.</b>\nИстория переписки доступна в Mini-app.", ticket.ID)
	h.sendMiniAppNotification(ctx, customer.TelegramID, text)
}

func (h *Handler) sendMiniAppNotification(ctx context.Context, telegramID int64, text string) {
	if h.telegramBot == nil || telegramID == 0 {
		return
	}

	params := &bot.SendMessageParams{
		ChatID:    telegramID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}

	if miniAppURL := config.GetMiniAppURL(); miniAppURL != "" {
		params.ReplyMarkup = models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{
						Text: "🚀 Открыть Mini-app",
						WebApp: &models.WebAppInfo{
							URL: miniAppURL,
						},
					},
				},
			},
		}
	}

	_, err := h.telegramBot.SendMessage(ctx, params)
	if err != nil {
		slog.Warn("mini app: failed to send support notification", "error", err, "telegramId", utils.MaskHalfInt64(telegramID))
	}
}

func supportNotificationText(value string) string {
	return html.EscapeString(strings.TrimSpace(value))
}

func supportNotificationQuote(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		text = "Без текста"
	}
	runes := []rune(text)
	if len(runes) > 700 {
		text = string(runes[:700]) + "..."
	}
	return "<blockquote>" + html.EscapeString(text) + "</blockquote>"
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func customerGoogleSubject(customer *database.Customer) string {
	if customer == nil || customer.GoogleSubject == nil {
		return ""
	}
	return strings.TrimSpace(*customer.GoogleSubject)
}

func customerGoogleEmail(customer *database.Customer) string {
	if customer == nil || customer.GoogleEmail == nil {
		return ""
	}
	return strings.TrimSpace(*customer.GoogleEmail)
}

func formatUsername(username string) string {
	value := strings.TrimSpace(username)
	if value == "" {
		return "—"
	}
	if strings.HasPrefix(value, "@") {
		return value
	}
	return "@" + value
}

func maxInt(value, fallback int) int {
	if value < fallback {
		return fallback
	}
	return value
}

func maxInt64(value, fallback int64) int64 {
	if value < fallback {
		return fallback
	}
	return value
}

func (h *Handler) ensureCustomer(ctx context.Context, sess *session) (*database.Customer, error) {
	langCode := sess.User.LanguageCode
	if langCode == "" {
		langCode = config.DefaultLanguage()
	}

	customer, err := h.customerRepository.FindByTelegramId(ctx, sess.User.ID)
	if err != nil {
		return nil, err
	}

	if customer == nil {
		customer, err = h.customerRepository.Create(ctx, &database.Customer{
			TelegramID: sess.User.ID,
			Language:   langCode,
		})
		if err != nil {
			return nil, err
		}

		if err := h.tryAttachReferral(ctx, sess, customer); err != nil {
			slog.Warn("mini app: attach referral", "error", err)
		}

		return customer, nil
	}

	if customer.Language != langCode {
		if err := h.customerRepository.UpdateFields(ctx, customer.ID, map[string]any{
			"language": langCode,
		}); err != nil {
			return nil, err
		}
		customer.Language = langCode
	}

	return customer, nil
}

func (h *Handler) tryAttachReferral(ctx context.Context, sess *session, customer *database.Customer) error {
	if !strings.HasPrefix(sess.StartParam, "ref_") {
		return nil
	}

	existingReferral, err := h.referralRepository.FindByReferee(ctx, customer.TelegramID)
	if err != nil || existingReferral != nil {
		return err
	}

	referrerID, err := strconv.ParseInt(strings.TrimPrefix(sess.StartParam, "ref_"), 10, 64)
	if err != nil || referrerID == customer.TelegramID {
		return err
	}

	referrer, err := h.customerRepository.FindByTelegramId(ctx, referrerID)
	if err != nil || referrer == nil {
		return err
	}

	_, err = h.referralRepository.Create(ctx, referrerID, customer.TelegramID)
	return err
}

func (h *Handler) availablePaymentMethods(ctx context.Context, customer *database.Customer) (map[string]bool, error) {
	yookassaEnabled := config.IsYookasaEnabled()
	cryptoEnabled := config.IsCryptoPayEnabled()
	if h.integrationSettings != nil {
		yookassaEnabled = h.integrationSettings.Enabled(integrations.ProviderYooKassa)
		cryptoEnabled = h.integrationSettings.Enabled(integrations.ProviderCryptoPay)
	}
	yookassaEnabled = yookassaEnabled && h.runtimeFeatureEnabled("yookassa")
	methods := map[string]bool{
		"sbp":    yookassaEnabled,
		"card":   yookassaEnabled,
		"crypto": cryptoEnabled && h.runtimeFeatureEnabled("crypto"),
		"stars":  h.runtimeFeatureEnabled("stars"),
	}
	if h.integrationSettings != nil {
		for _, provider := range integrations.SortedPaymentProviders() {
			if provider != integrations.ProviderYooKassa && provider != integrations.ProviderCryptoPay {
				methods[provider] = h.integrationSettings.Enabled(provider)
			}
		}
	}

	if methods["stars"] && config.RequirePaidPurchaseForStars() {
		paidPurchase, err := h.purchaseRepository.FindSuccessfulPaidPurchaseByCustomer(ctx, customer.ID)
		if err != nil {
			return nil, err
		}
		methods["stars"] = paidPurchase != nil
	}

	return methods, nil
}

func (h *Handler) buildPlans() []planPayload {
	checkout := h.checkoutPlans()
	baseMonthly := 0
	for _, plan := range checkout {
		if plan.Months == 1 && plan.Variant == planbook.VariantRegular && plan.PriceRub > 0 && (baseMonthly <= 0 || plan.PriceRub < baseMonthly) {
			baseMonthly = plan.PriceRub
		}
	}

	plans := make([]planPayload, 0, len(checkout))
	bestIndex := -1
	bestSavings := -1

	for _, plan := range checkout {
		if plan.PriceRub == 0 {
			continue
		}

		savings := 0
		if baseMonthly > 0 && plan.Months > 1 {
			fullPrice := baseMonthly * plan.Months
			savings = int(math.Round((1 - float64(plan.PriceRub)/float64(fullPrice)) * 100))
			if savings < 0 {
				savings = 0
			}
		}

		plans = append(plans, planPayload{
			ID:                plan.ID,
			Months:            plan.Months,
			PriceRub:          plan.PriceRub,
			PriceStars:        plan.PriceStars,
			TrafficLimitBytes: plan.TrafficLimitBytes,
			DeviceLimitCount:  plan.DeviceLimitCount,
			Variant:           plan.Variant,
			SavingsPercent:    savings,
			Wide:              plan.Wide,
			TitleRU:           h.planTitle(plan.ID, "ru"),
			TitleEN:           h.planTitle(plan.ID, "en"),
		})

		if savings > bestSavings {
			bestSavings = savings
			bestIndex = len(plans) - 1
		}
	}

	if bestIndex >= 0 && bestSavings > 0 {
		plans[bestIndex].Recommended = true
	}

	return plans
}

func (h *Handler) runtimeFeatureEnabled(name string) bool {
	if h.runtimeSettings == nil {
		return true
	}
	return h.runtimeSettings.FeatureEnabled(name)
}

func (h *Handler) runtimeLink(name, fallback string) string {
	if h.runtimeSettings == nil {
		return trimQuotes(fallback)
	}
	return trimQuotes(h.runtimeSettings.Link(name, fallback))
}

func (h *Handler) planTitle(planID, locale string) string {
	if h.runtimeSettings == nil {
		return ""
	}
	return h.runtimeSettings.PlanTitle(planID, locale)
}

func enabledValue(enabled bool, value string) string {
	if !enabled {
		return ""
	}
	return value
}

func runtimeFeatureForPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/mini-app/auth/google/"):
		return "google"
	case strings.HasPrefix(path, "/api/mini-app/trial/"):
		return "trials"
	case strings.HasPrefix(path, "/api/mini-app/promocode/"):
		return "promocodes"
	case strings.HasPrefix(path, "/api/mini-app/reviews/"):
		return "reviews"
	case strings.HasPrefix(path, "/api/mini-app/support/"):
		return "support"
	default:
		return ""
	}
}

func mapPaymentMethods(methods map[string]bool) []paymentMethodPayload {
	order := []string{"sbp", "card", "stars", "crypto", "lava", "wata", "platega", "freekassa", "heleket"}
	payload := make([]paymentMethodPayload, 0, len(order))
	for _, method := range order {
		if methods[method] {
			payload = append(payload, paymentMethodPayload{ID: method})
		}
	}
	return payload
}

func mapPaymentMethod(method string) (database.InvoiceType, error) {
	switch method {
	case "sbp", "card":
		return database.InvoiceTypeYookasa, nil
	case "stars":
		return database.InvoiceTypeTelegram, nil
	case "crypto":
		return database.InvoiceTypeCrypto, nil
	case "lava":
		return database.InvoiceTypeLava, nil
	case "wata":
		return database.InvoiceTypeWata, nil
	case "platega":
		return database.InvoiceTypePlatega, nil
	case "freekassa":
		return database.InvoiceTypeFreeKassa, nil
	case "heleket":
		return database.InvoiceTypeHeleket, nil
	default:
		return "", fmt.Errorf("unknown payment method: %s", method)
	}
}

func buildReferralShareURL(refCode int64) string {
	if config.GetReferralDays() == 0 || config.BotURL() == "" {
		return ""
	}

	referralTarget := fmt.Sprintf("%s?start=ref_%d", config.BotURL(), refCode)
	return fmt.Sprintf(
		"https://t.me/share/url?url=%s&text=%s",
		url.QueryEscape(referralTarget),
		url.QueryEscape("Подключайся к Link-Bot и забирай быстрый доступ без лишних настроек."),
	)
}

func buildPaymentReturnTarget(purchaseID int64, status string) string {
	target := &url.URL{Path: "/mini-app/"}
	query := target.Query()
	query.Set("paymentReturn", "1")
	query.Set("paymentStatus", status)
	if purchaseID > 0 {
		query.Set("purchaseId", strconv.FormatInt(purchaseID, 10))
	}
	query.Set("provider", "yookasa")
	target.RawQuery = query.Encode()
	return target.String()
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func daysLeft(expireAt time.Time) int {
	remaining := time.Until(expireAt)
	if remaining <= 0 {
		return 0
	}

	return int(math.Ceil(remaining.Hours() / 24))
}

func timePtrEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return left.UTC().Equal(right.UTC())
}

func stringPtrEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return strings.TrimSpace(*left) == strings.TrimSpace(*right)
}

func trimQuotes(value string) string {
	return strings.Trim(value, "\"")
}

func stringPtr(value string) *string {
	return &value
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	setAPIHeaders(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *Handler) decodeJSONRequest(w http.ResponseWriter, r *http.Request, maxBytes int64, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}

	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("request body must contain a single JSON object")
	}

	return nil
}

func (h *Handler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeErrorWithMeta(w, status, code, message, nil)
}

func (h *Handler) writeErrorWithMeta(w http.ResponseWriter, status int, code, message string, meta any) {
	h.writeJSON(w, status, map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":    code,
			"message": message,
			"meta":    meta,
		},
	})
}
