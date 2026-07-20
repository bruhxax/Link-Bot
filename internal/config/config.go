package config

import (
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

type config struct {
	telegramToken                                             string
	price1, price3, price6, price12                           int
	starsPrice1, starsPrice3, starsPrice6, starsPrice12       int
	remnawaveUrl, remnawaveToken, remnawaveMode, remnawaveTag string
	googleClientID                                            string
	defaultLanguage                                           string
	databaseURL                                               string
	cryptoPayURL, cryptoPayToken, cryptoPayAcceptedAssets     string
	botURL                                                    string
	paymentNotificationBotToken                               string
	paymentNotificationChatID                                 int64
	paymentNotificationTimezone                               string
	yookasaURL, yookasaShopId, yookasaSecretKey, yookasaEmail string
	moynalogURL, moynalogUsername, moynalogPassword           string
	trafficLimit                                              int
	trafficLimit3                                             int
	trafficLimit6                                             int
	trafficLimit12                                            int
	deviceLimit                                               int
	deviceLimit3                                              int
	deviceLimit6                                              int
	deviceLimit12                                             int
	trialDeviceLimit                                          int
	trialTrafficLimit                                         int
	channelURL                                                string
	requiredChannelSubscriptionURL                            string
	requiredChannelSubscriptionTitle                          string
	supportURL                                                string
	isYookasaEnabled                                          bool
	isCryptoEnabled                                           bool
	isTelegramStarsEnabled                                    bool
	isMoynalogEnabled                                         bool
	requiredChannelSubscriptionEnabled                        bool
	adminTelegramId                                           int64
	trialDays                                                 int
	trialRemnawaveTag                                         string
	squadUUIDs                                                map[uuid.UUID]uuid.UUID
	referralDays                                              int
	referralTrafficLimit                                      int
	miniApp                                                   string
	publicBaseURL                                             string
	enableAutoPayment                                         bool
	paymentMethodDemoEnabled                                  bool
	paymentMethodDemoTitle                                    string
	paymentMethodDemoType                                     string
	healthCheckPort                                           int
	tributeWebhookUrl, tributeAPIKey, tributePaymentUrl       string
	isWebAppLinkEnabled                                       bool
	daysInMonth                                               int
	externalSquadUUID                                         uuid.UUID
	blockedTelegramIds                                        map[int64]bool
	whitelistedTelegramIds                                    map[int64]bool
	requirePaidPurchaseForStars                               bool
	trialInternalSquads                                       map[uuid.UUID]uuid.UUID
	trialExternalSquadUUID                                    uuid.UUID
	remnawaveHeaders                                          map[string]string
	trialTrafficLimitResetStrategy                            string
	trafficLimitResetStrategy                                 string
}

var conf config

func RemnawaveTag() string {
	return conf.remnawaveTag
}

func TrialRemnawaveTag() string {
	if conf.trialRemnawaveTag != "" {
		return conf.trialRemnawaveTag
	}
	return conf.remnawaveTag
}

func DefaultLanguage() string {
	return conf.defaultLanguage
}
func GoogleClientID() string {
	return conf.googleClientID
}
func GetTributeWebHookUrl() string {
	return conf.tributeWebhookUrl
}
func GetTributeAPIKey() string {
	return conf.tributeAPIKey
}

func GetTributePaymentUrl() string {
	return conf.tributePaymentUrl
}

func GetReferralDays() int {
	return conf.referralDays
}

func ReferralTrafficBonusBytes() int {
	return conf.referralTrafficLimit * bytesInGigabyte
}

func GetMiniAppURL() string {
	return conf.miniApp
}

func PublicBaseURL() string {
	return conf.publicBaseURL
}

func EnableAutoPayment() bool {
	return conf.enableAutoPayment
}

func PaymentMethodDemoEnabled() bool {
	return conf.paymentMethodDemoEnabled
}

func PaymentMethodDemoTitle() string {
	return conf.paymentMethodDemoTitle
}

func PaymentMethodDemoType() string {
	return conf.paymentMethodDemoType
}

func SquadUUIDs() map[uuid.UUID]uuid.UUID {
	return conf.squadUUIDs
}

func GetBlockedTelegramIds() map[int64]bool {
	return conf.blockedTelegramIds
}

func GetWhitelistedTelegramIds() map[int64]bool {
	return conf.whitelistedTelegramIds
}

func TrialInternalSquads() map[uuid.UUID]uuid.UUID {
	if conf.trialInternalSquads != nil && len(conf.trialInternalSquads) > 0 {
		return conf.trialInternalSquads
	}
	return conf.squadUUIDs
}

func TrialExternalSquadUUID() uuid.UUID {
	if conf.trialExternalSquadUUID != uuid.Nil {
		return conf.trialExternalSquadUUID
	}
	return conf.externalSquadUUID
}

func TrialTrafficLimit() int {
	return conf.trialTrafficLimit * bytesInGigabyte
}

func TrialTrafficLimitGB() int {
	return conf.trialTrafficLimit
}

func TrialDeviceLimit() int {
	return conf.trialDeviceLimit
}

func TrialDays() int {
	return conf.trialDays
}
func ChannelURL() string {
	return conf.channelURL
}

func RequiredChannelSubscriptionURL() string {
	return conf.requiredChannelSubscriptionURL
}

func RequiredChannelSubscriptionTitle() string {
	return conf.requiredChannelSubscriptionTitle
}

func IsRequiredChannelSubscriptionEnabled() bool {
	return conf.requiredChannelSubscriptionEnabled && conf.requiredChannelSubscriptionURL != ""
}

func RequiredChannelSubscriptionChatID() (any, bool) {
	raw := strings.TrimSpace(conf.requiredChannelSubscriptionURL)
	if raw == "" {
		return nil, false
	}

	if strings.HasPrefix(raw, "-100") {
		chatID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, false
		}
		return chatID, true
	}

	if strings.HasPrefix(raw, "@") {
		return raw, true
	}

	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return nil, false
		}
		raw = strings.Trim(parsed.Path, "/")
	}

	raw = strings.TrimSpace(strings.TrimPrefix(raw, "t.me/"))
	raw = strings.TrimPrefix(raw, "@")
	raw = strings.Trim(raw, "/")
	if raw == "" || strings.HasPrefix(raw, "+") {
		return nil, false
	}

	if idx := strings.Index(raw, "/"); idx >= 0 {
		raw = raw[:idx]
	}
	if raw == "" {
		return nil, false
	}

	return "@" + raw, true
}

func SupportURL() string {
	return conf.supportURL
}

func YookasaEmail() string {
	return conf.yookasaEmail
}

func Price1() int {
	return conf.price1
}

func Price3() int {
	return conf.price3
}

func Price6() int {
	return conf.price6
}

func Price12() int {
	return conf.price12
}

func DaysInMonth() int {
	return conf.daysInMonth
}

func ExternalSquadUUID() uuid.UUID {
	return conf.externalSquadUUID
}

func Price(month int) int {
	switch month {
	case 1:
		return conf.price1
	case 3:
		return conf.price3
	case 6:
		return conf.price6
	case 12:
		return conf.price12
	default:
		return conf.price1
	}
}

func StarsPrice(month int) int {
	switch month {
	case 1:
		return conf.starsPrice1
	case 3:
		return conf.starsPrice3
	case 6:
		return conf.starsPrice6
	case 12:
		return conf.starsPrice12
	default:
		return conf.starsPrice1
	}
}
func TelegramToken() string {
	return conf.telegramToken
}
func RemnawaveUrl() string {
	return conf.remnawaveUrl
}
func DadaBaseUrl() string {
	return conf.databaseURL
}
func RemnawaveToken() string {
	return conf.remnawaveToken
}
func RemnawaveMode() string {
	return conf.remnawaveMode
}
func CryptoPayUrl() string {
	return conf.cryptoPayURL
}
func CryptoPayToken() string {
	return conf.cryptoPayToken
}
func CryptoPayAcceptedAssets() string {
	return conf.cryptoPayAcceptedAssets
}
func BotURL() string {
	return conf.botURL
}
func SetBotURL(botURL string) {
	conf.botURL = botURL
}
func PaymentNotificationBotToken() string {
	return conf.paymentNotificationBotToken
}
func PaymentNotificationChatID() int64 {
	return conf.paymentNotificationChatID
}
func PaymentNotificationTimezone() string {
	return conf.paymentNotificationTimezone
}
func IsPaymentNotificationEnabled() bool {
	return conf.paymentNotificationBotToken != "" && conf.paymentNotificationChatID != 0
}
func YookasaUrl() string {
	return conf.yookasaURL
}
func YookasaShopId() string {
	return conf.yookasaShopId
}
func YookasaSecretKey() string {
	return conf.yookasaSecretKey
}
func TrafficLimit() int {
	return conf.trafficLimit * bytesInGigabyte
}

func TrafficLimitGBForMonths(month int) int {
	switch month {
	case 1:
		return conf.trafficLimit
	case 3:
		return conf.trafficLimit3
	case 6:
		return conf.trafficLimit6
	case 12:
		return conf.trafficLimit12
	default:
		return conf.trafficLimit
	}
}

func TrafficLimitForMonths(month int) int {
	return TrafficLimitGBForMonths(month) * bytesInGigabyte
}

func DeviceLimitForMonths(month int) int {
	switch month {
	case 1:
		return conf.deviceLimit
	case 3:
		return conf.deviceLimit3
	case 6:
		return conf.deviceLimit6
	case 12:
		return conf.deviceLimit12
	default:
		return conf.deviceLimit
	}
}

func IsCryptoPayEnabled() bool {
	return conf.isCryptoEnabled
}

func IsYookasaEnabled() bool {
	return conf.isYookasaEnabled
}

func IsTelegramStarsEnabled() bool {
	return conf.isTelegramStarsEnabled
}

func RequirePaidPurchaseForStars() bool {
	return conf.requirePaidPurchaseForStars
}

func GetAdminTelegramId() int64 {
	return conf.adminTelegramId
}

func GetHealthCheckPort() int {
	return conf.healthCheckPort
}

func IsWepAppLinkEnabled() bool {
	return conf.isWebAppLinkEnabled
}

func RemnawaveHeaders() map[string]string {
	return conf.remnawaveHeaders
}

func TrialTrafficLimitResetStrategy() string {
	return conf.trialTrafficLimitResetStrategy
}

func TrafficLimitResetStrategy() string {
	return conf.trafficLimitResetStrategy
}

const bytesInGigabyte = 1073741824

func MoynalogUrl() string {
	return conf.moynalogURL
}

func MoynalogUsername() string {
	return conf.moynalogUsername
}

func MoynalogPassword() string {
	return conf.moynalogPassword
}

func IsMoynalogEnabled() bool {
	return conf.isMoynalogEnabled
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Panicf("env %q not set", key)
	}
	return v
}

func mustEnvInt(key string) int {
	v := mustEnv(key)
	i, err := strconv.Atoi(v)
	if err != nil {
		log.Panicf("invalid int in %q: %v", key, err)
	}
	return i
}

func envIntDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		log.Panicf("invalid int in %q: %v", key, err)
	}
	return i
}

func envInt64Default(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		log.Panicf("invalid int64 in %q: %v", key, err)
	}
	return i
}

func envStringDefault(key string, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func envBool(key string) bool {
	return os.Getenv(key) == "true"
}

func InitConfig() {
	if os.Getenv("DISABLE_ENV_FILE") != "true" {
		if err := godotenv.Load(".env"); err != nil {
			log.Println("No .env loaded:", err)
		}
	}
	var err error
	conf.adminTelegramId, err = strconv.ParseInt(os.Getenv("ADMIN_TELEGRAM_ID"), 10, 64)
	if err != nil {
		panic("ADMIN_TELEGRAM_ID .env variable not set")
	}

	conf.telegramToken = mustEnv("TELEGRAM_TOKEN")
	conf.paymentNotificationBotToken = envStringDefault("PAYMENT_NOTIFICATION_BOT_TOKEN", "")
	conf.paymentNotificationChatID = envInt64Default("PAYMENT_NOTIFICATION_CHAT_ID", conf.adminTelegramId)
	conf.paymentNotificationTimezone = envStringDefault("PAYMENT_NOTIFICATION_TIMEZONE", "Europe/Moscow")

	conf.isWebAppLinkEnabled = func() bool {
		isWebAppLinkEnabled := os.Getenv("IS_WEB_APP_LINK") == "true"
		return isWebAppLinkEnabled
	}()

	conf.publicBaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/")
	if conf.publicBaseURL != "" {
		parsed, parseErr := url.Parse(conf.publicBaseURL)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" {
			panic("PUBLIC_BASE_URL must be a valid HTTPS origin")
		}
	}
	conf.miniApp = strings.TrimSpace(os.Getenv("MINI_APP_URL"))
	if conf.miniApp == "" && conf.publicBaseURL != "" {
		conf.miniApp = conf.publicBaseURL + "/mini-app/"
	}

	conf.remnawaveTag = envStringDefault("REMNAWAVE_TAG", "")

	conf.trialRemnawaveTag = envStringDefault("TRIAL_REMNAWAVE_TAG", "")

	conf.trialTrafficLimitResetStrategy = envStringDefault("TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY", "MONTH")
	conf.trafficLimitResetStrategy = envStringDefault("TRAFFIC_LIMIT_RESET_STRATEGY", "MONTH")

	conf.defaultLanguage = envStringDefault("DEFAULT_LANGUAGE", "ru")
	conf.googleClientID = envStringDefault("GOOGLE_CLIENT_ID", "")

	conf.daysInMonth = envIntDefault("DAYS_IN_MONTH", 30)

	externalSquadUUIDStr := os.Getenv("EXTERNAL_SQUAD_UUID")
	if externalSquadUUIDStr != "" {
		parsedUUID, err := uuid.Parse(externalSquadUUIDStr)
		if err != nil {
			panic(fmt.Sprintf("invalid EXTERNAL_SQUAD_UUID format: %v", err))
		}
		conf.externalSquadUUID = parsedUUID
	} else {
		conf.externalSquadUUID = uuid.Nil
	}

	conf.trialTrafficLimit = envIntDefault("TRIAL_TRAFFIC_LIMIT", 10)
	conf.trialDeviceLimit = envIntDefault("TRIAL_HWID_DEVICE_LIMIT", 5)

	conf.healthCheckPort = envIntDefault("HEALTH_CHECK_PORT", 8080)

	conf.trialDays = envIntDefault("TRIAL_DAYS", 3)

	conf.enableAutoPayment = envBool("ENABLE_AUTO_PAYMENT")
	conf.paymentMethodDemoEnabled = envBool("PAYMENT_METHOD_DEMO_ENABLED")
	conf.paymentMethodDemoTitle = envStringDefault("PAYMENT_METHOD_DEMO_TITLE", "Visa **** 1111")
	conf.paymentMethodDemoType = envStringDefault("PAYMENT_METHOD_DEMO_TYPE", "bank_card")

	conf.price1 = envIntDefault("PRICE_1", 89)
	conf.price3 = envIntDefault("PRICE_3", 239)
	conf.price6 = envIntDefault("PRICE_6", 350)
	conf.price12 = envIntDefault("PRICE_12", 700)

	conf.isTelegramStarsEnabled = envBool("TELEGRAM_STARS_ENABLED")
	if conf.isTelegramStarsEnabled {
		conf.starsPrice1 = envIntDefault("STARS_PRICE_1", conf.price1)
		conf.starsPrice3 = envIntDefault("STARS_PRICE_3", conf.price3)
		conf.starsPrice6 = envIntDefault("STARS_PRICE_6", conf.price6)
		conf.starsPrice12 = envIntDefault("STARS_PRICE_12", conf.price12)

	}

	conf.requirePaidPurchaseForStars = envBool("REQUIRE_PAID_PURCHASE_FOR_STARS")

	conf.remnawaveUrl = mustEnv("REMNAWAVE_URL")

	conf.remnawaveMode = func() string {
		v := os.Getenv("REMNAWAVE_MODE")
		if v != "" {
			if v != "remote" && v != "local" {
				panic("REMNAWAVE_MODE .env variable must be either 'remote' or 'local'")
			} else {
				return v
			}
		} else {
			return "remote"
		}
	}()

	conf.remnawaveToken = mustEnv("REMNAWAVE_TOKEN")

	conf.databaseURL = mustEnv("DATABASE_URL")

	conf.isCryptoEnabled = envBool("CRYPTO_PAY_ENABLED")
	if conf.isCryptoEnabled {
		conf.cryptoPayURL = mustEnv("CRYPTO_PAY_URL")
		conf.cryptoPayToken = mustEnv("CRYPTO_PAY_TOKEN")
		conf.cryptoPayAcceptedAssets = strings.ToUpper(strings.ReplaceAll(envStringDefault("CRYPTO_PAY_ACCEPTED_ASSETS", "USDT"), " ", ""))
	}

	conf.isYookasaEnabled = envBool("YOOKASA_ENABLED")
	if conf.isYookasaEnabled {
		conf.yookasaURL = mustEnv("YOOKASA_URL")
		conf.yookasaShopId = mustEnv("YOOKASA_SHOP_ID")
		conf.yookasaSecretKey = mustEnv("YOOKASA_SECRET_KEY")
		conf.yookasaEmail = mustEnv("YOOKASA_EMAIL")
	}

	baseTrafficLimit := envIntDefault("TRAFFIC_LIMIT", 150)
	conf.trafficLimit = envIntDefault("TRAFFIC_LIMIT_1", baseTrafficLimit)
	conf.trafficLimit3 = envIntDefault("TRAFFIC_LIMIT_3", 500)
	conf.trafficLimit6 = envIntDefault("TRAFFIC_LIMIT_6", 1000)
	conf.trafficLimit12 = envIntDefault("TRAFFIC_LIMIT_12", 0)
	conf.deviceLimit = envIntDefault("HWID_DEVICE_LIMIT_1", 5)
	conf.deviceLimit3 = envIntDefault("HWID_DEVICE_LIMIT_3", 7)
	conf.deviceLimit6 = envIntDefault("HWID_DEVICE_LIMIT_6", 10)
	conf.deviceLimit12 = envIntDefault("HWID_DEVICE_LIMIT_12", 0)
	conf.referralDays = mustEnvInt("REFERRAL_DAYS")
	conf.referralTrafficLimit = envIntDefault("REFERRAL_TRAFFIC_GB", 50)

	conf.supportURL = os.Getenv("SUPPORT_URL")
	conf.channelURL = os.Getenv("CHANNEL_URL")
	conf.requiredChannelSubscriptionEnabled = envBool("REQUIRED_CHANNEL_SUBSCRIPTION_ENABLED")
	conf.requiredChannelSubscriptionURL = envStringDefault("REQUIRED_CHANNEL_SUBSCRIPTION_URL", conf.channelURL)
	conf.requiredChannelSubscriptionTitle = envStringDefault("REQUIRED_CHANNEL_SUBSCRIPTION_TITLE", "")

	conf.squadUUIDs = func() map[uuid.UUID]uuid.UUID {
		v := os.Getenv("SQUAD_UUIDS")
		if v != "" {
			uuids := strings.Split(v, ",")
			var inboundsMap = make(map[uuid.UUID]uuid.UUID)
			for _, value := range uuids {
				uuid, err := uuid.Parse(value)
				if err != nil {
					panic(err)
				}
				inboundsMap[uuid] = uuid
			}
			slog.Info("Loaded squad UUIDs", "uuids", uuids)
			return inboundsMap
		} else {
			slog.Info("No squad UUIDs specified, all will be used")
			return map[uuid.UUID]uuid.UUID{}
		}
	}()

	conf.tributeWebhookUrl = os.Getenv("TRIBUTE_WEBHOOK_URL")
	if conf.tributeWebhookUrl != "" {
		conf.tributeAPIKey = mustEnv("TRIBUTE_API_KEY")
		conf.tributePaymentUrl = mustEnv("TRIBUTE_PAYMENT_URL")
	}

	conf.blockedTelegramIds = func() map[int64]bool {
		v := os.Getenv("BLOCKED_TELEGRAM_IDS")
		if v != "" {
			ids := strings.Split(v, ",")
			var blockedMap = make(map[int64]bool)
			for _, idStr := range ids {
				id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
				if err != nil {
					panic(fmt.Sprintf("invalid telegram ID in BLOCKED_TELEGRAM_IDS: %v", err))
				}
				blockedMap[id] = true
			}
			slog.Info("Loaded blocked telegram IDs", "count", len(blockedMap))
			return blockedMap
		} else {
			slog.Info("No blocked telegram IDs specified")
			return map[int64]bool{}
		}
	}()

	conf.whitelistedTelegramIds = func() map[int64]bool {
		v := os.Getenv("WHITELISTED_TELEGRAM_IDS")
		if v != "" {
			ids := strings.Split(v, ",")
			var whitelistedMap = make(map[int64]bool)
			for _, idStr := range ids {
				id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
				if err != nil {
					panic(fmt.Sprintf("invalid telegram ID in WHITELISTED_TELEGRAM_IDS: %v", err))
				}
				whitelistedMap[id] = true
			}
			slog.Info("Loaded whitelisted telegram IDs", "count", len(whitelistedMap))
			return whitelistedMap
		} else {
			slog.Info("No whitelisted telegram IDs specified")
			return map[int64]bool{}
		}
	}()

	conf.trialInternalSquads = func() map[uuid.UUID]uuid.UUID {
		v := os.Getenv("TRIAL_INTERNAL_SQUADS")
		if v != "" {
			uuids := strings.Split(v, ",")
			var trialSquadsMap = make(map[uuid.UUID]uuid.UUID)
			for _, value := range uuids {
				parsedUUID, err := uuid.Parse(strings.TrimSpace(value))
				if err != nil {
					panic(fmt.Sprintf("invalid UUID in TRIAL_INTERNAL_SQUADS: %v", err))
				}
				trialSquadsMap[parsedUUID] = parsedUUID
			}
			slog.Info("Loaded trial internal squad UUIDs", "uuids", uuids)
			return trialSquadsMap
		} else {
			slog.Info("No trial internal squads specified, will use regular SQUAD_UUIDS for trial users")
			return map[uuid.UUID]uuid.UUID{}
		}
	}()

	trialExternalSquadUUIDStr := os.Getenv("TRIAL_EXTERNAL_SQUAD_UUID")
	if trialExternalSquadUUIDStr != "" {
		parsedUUID, err := uuid.Parse(trialExternalSquadUUIDStr)
		if err != nil {
			panic(fmt.Sprintf("invalid TRIAL_EXTERNAL_SQUAD_UUID format: %v", err))
		}
		conf.trialExternalSquadUUID = parsedUUID
		slog.Info("Loaded trial external squad UUID", "uuid", trialExternalSquadUUIDStr)
	} else {
		conf.trialExternalSquadUUID = uuid.Nil
		slog.Info("No trial external squad specified, will use regular EXTERNAL_SQUAD_UUID for trial users")
	}

	conf.remnawaveHeaders = func() map[string]string {
		v := os.Getenv("REMNAWAVE_HEADERS")
		if v != "" {
			headers := make(map[string]string)
			pairs := strings.Split(v, ";")
			for _, pair := range pairs {
				parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					if key != "" && value != "" {
						headers[key] = value
					}
				}
			}
			if len(headers) > 0 {
				slog.Info("Loaded remnawave headers", "count", len(headers))
				return headers
			}
		}
		return map[string]string{}
	}()

	conf.isMoynalogEnabled = envBool("MOYNALOG_ENABLED")
	if conf.isMoynalogEnabled {
		conf.moynalogURL = envStringDefault("MOYNALOG_URL", "https://moynalog.ru/api/v1")
		conf.moynalogUsername = mustEnv("MOYNALOG_USERNAME")
		conf.moynalogPassword = mustEnv("MOYNALOG_PASSWORD")
	}
}
