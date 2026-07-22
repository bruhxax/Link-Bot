package runtimeconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"

	"link-bot/internal/config"
	"link-bot/internal/database"
	planbook "link-bot/internal/plans"
)

const CurrentVersion = 9

var (
	hexColorPattern       = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	elementIDPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	contentKeyPattern     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,99}$`)
	telegramUserPattern   = regexp.MustCompile(`^[A-Za-z0-9_]{5,32}$`)
	customEmojiIDPattern  = regexp.MustCompile(`^[0-9]{5,32}$`)
	customEmojiTagPattern = regexp.MustCompile(`(?is)<tg-emoji\s+emoji-id=["']([0-9]{5,32})["'][^>]*>.*?</tg-emoji>`)
)

var featureOrder = []string{
	"mini_app",
	"google",
	"stars",
	"trials",
	"referrals",
	"reviews",
	"support",
	"media",
	"promocodes",
	"server_status",
	"web_version",
	"pwa_install",
}

type Settings struct {
	Version     int                 `json:"version"`
	Maintenance MaintenanceSettings `json:"maintenance"`
	Features    map[string]bool     `json:"features"`
	Content     ContentSettings     `json:"content"`
	Appearance  AppearanceSettings  `json:"appearance"`
	Layout      LayoutSettings      `json:"layout"`
	Plans       []PlanSettings      `json:"plans"`
	Trial       TrialSettings       `json:"trial"`
}

type MaintenanceSettings struct {
	Enabled  bool   `json:"enabled"`
	TitleRU  string `json:"titleRu"`
	TextRU   string `json:"textRu"`
	ReasonRU string `json:"reasonRu"`
}

type FAQItem struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type CustomLink struct {
	ID      string `json:"id"`
	LabelRU string `json:"labelRu"`
	HintRU  string `json:"hintRu"`
	URL     string `json:"url"`
	Icon    string `json:"icon"`
}

type TelegramVerificationSettings struct {
	Text              string                 `json:"text"`
	Banner            string                 `json:"banner"`
	ChannelChatID     string                 `json:"channelChatId"`
	ChannelButton     TelegramButtonSettings `json:"channelButton"`
	ConfirmButton     TelegramButtonSettings `json:"confirmButton"`
	CheckFailedText   string                 `json:"checkFailedText"`
	NotSubscribedText string                 `json:"notSubscribedText"`
	VerifiedText      string                 `json:"verifiedText"`
}

type TelegramSupportSettings struct {
	NewTicketText     string                 `json:"newTicketText"`
	CustomerReplyText string                 `json:"customerReplyText"`
	AdminReplyText    string                 `json:"adminReplyText"`
	ClosedText        string                 `json:"closedText"`
	OpenButton        TelegramButtonSettings `json:"openButton"`
}

type TelegramStartMenuSettings struct {
	TrialButton     TelegramButtonSettings `json:"trialButton"`
	DashboardButton TelegramButtonSettings `json:"dashboardButton"`
	PlansButton     TelegramButtonSettings `json:"plansButton"`
	SupportButton   TelegramButtonSettings `json:"supportButton"`
}

type TelegramCommerceSettings struct {
	Banner             string                 `json:"banner"`
	TariffsText        string                 `json:"tariffsText"`
	PaymentMethodsText string                 `json:"paymentMethodsText"`
	PaymentReadyText   string                 `json:"paymentReadyText"`
	YookassaButton     TelegramButtonSettings `json:"yookassaButton"`
	CryptoButton       TelegramButtonSettings `json:"cryptoButton"`
	StarsButton        TelegramButtonSettings `json:"starsButton"`
	PayButton          TelegramButtonSettings `json:"payButton"`
	BackButton         TelegramButtonSettings `json:"backButton"`
	SuccessText        string                 `json:"successText"`
	SuccessBanner      string                 `json:"successBanner"`
	SuccessButton      TelegramButtonSettings `json:"successButton"`
}

type ContentSettings struct {
	BrandName                  string                       `json:"brandName"`
	AdminContact               string                       `json:"adminContact"`
	LogoURL                    string                       `json:"logoUrl"`
	StartTextRU                string                       `json:"startTextRu"`
	StartImage                 string                       `json:"startImage"`
	Copy                       map[string]map[string]string `json:"copy"`
	FAQ                        map[string][]FAQItem         `json:"faq"`
	Links                      map[string]string            `json:"links"`
	CustomLinks                []CustomLink                 `json:"customLinks"`
	SubscriptionReminderButton TelegramButtonSettings       `json:"subscriptionReminderButton"`
	Verification               TelegramVerificationSettings `json:"verification"`
	Support                    TelegramSupportSettings      `json:"support"`
	StartMenu                  TelegramStartMenuSettings    `json:"startMenu"`
	Commerce                   TelegramCommerceSettings     `json:"commerce"`
}

type TelegramButtonSettings struct {
	Text              string `json:"text"`
	IconCustomEmojiID string `json:"iconCustomEmojiId"`
	Style             string `json:"style"`
}

type AppearanceSettings struct {
	BackgroundMode string            `json:"backgroundMode"`
	Colors         map[string]string `json:"colors"`
	Compact        bool              `json:"compact"`
	ShowFrames     bool              `json:"showFrames"`
}

type LayoutElement struct {
	ID        string   `json:"id"`
	Area      string   `json:"area"`
	Order     int      `json:"order"`
	Visible   bool     `json:"visible"`
	Width     float64  `json:"width"`
	Height    int      `json:"height"`
	Framed    bool     `json:"framed"`
	Align     string   `json:"align"`
	OffsetX   int      `json:"offsetX"`
	OffsetY   int      `json:"offsetY"`
	PositionX *float64 `json:"positionX,omitempty"`
	PositionY *float64 `json:"positionY,omitempty"`
	Group     string   `json:"group,omitempty"`
}

type LayoutSettings struct {
	Elements    []LayoutElement `json:"elements"`
	PlanColumns int             `json:"planColumns"`
	LogoWidth   int             `json:"logoWidth"`
}

type PlanSettings struct {
	ID                 string   `json:"id"`
	Enabled            bool     `json:"enabled"`
	Months             int      `json:"months"`
	TitleRU            string   `json:"titleRu"`
	TitleEN            string   `json:"titleEn"`
	PriceRub           int      `json:"priceRub"`
	PriceStars         int      `json:"priceStars"`
	TrafficGB          int      `json:"trafficGb"`
	UnlimitedTraffic   bool     `json:"unlimitedTraffic"`
	DeviceLimit        int      `json:"deviceLimit"`
	Wide               bool     `json:"wide"`
	InternalSquadUUIDs []string `json:"internalSquadUuids"`
	ExternalSquadUUID  string   `json:"externalSquadUuid"`
}

type TrialSettings struct {
	Enabled              bool     `json:"enabled"`
	Days                 int      `json:"days"`
	TrafficGB            int      `json:"trafficGb"`
	UnlimitedTraffic     bool     `json:"unlimitedTraffic"`
	DeviceLimit          int      `json:"deviceLimit"`
	InternalSquadUUIDs   []string `json:"internalSquadUuids"`
	ExternalSquadUUID    string   `json:"externalSquadUuid"`
	TrafficResetStrategy string   `json:"trafficResetStrategy"`
	Tag                  string   `json:"tag"`
}

type Service struct {
	repository *database.RuntimeSettingsRepository
	value      atomic.Value
	mu         sync.Mutex
}

func NewService(ctx context.Context, repository *database.RuntimeSettingsRepository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("runtime settings repository is required")
	}

	service := &Service{repository: repository}
	settings := DefaultSettings()
	raw, err := repository.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load runtime settings: %w", err)
	}
	if len(raw) > 0 && string(raw) != "{}" {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return nil, fmt.Errorf("decode runtime settings: %w", err)
		}
	}
	loadedVersion := settings.Version
	if err := NormalizeAndValidate(&settings); err != nil {
		return nil, fmt.Errorf("validate runtime settings: %w", err)
	}
	if loadedVersion < CurrentVersion {
		normalized, err := json.Marshal(settings)
		if err != nil {
			return nil, fmt.Errorf("encode migrated runtime settings: %w", err)
		}
		if err := repository.Save(ctx, normalized, 0); err != nil {
			return nil, fmt.Errorf("save migrated runtime settings: %w", err)
		}
	}
	service.value.Store(settings)
	return service, nil
}

func DefaultSettings() Settings {
	features := map[string]bool{}
	for _, name := range featureOrder {
		features[name] = true
	}

	return Settings{
		Version: CurrentVersion,
		Maintenance: MaintenanceSettings{
			TitleRU:  "Технические работы",
			TextRU:   "Сервис временно недоступен. Попробуйте немного позже.",
			ReasonRU: "Плановые работы",
		},
		Features: features,
		Content: ContentSettings{
			BrandName:    "Link-Bot",
			AdminContact: "",
			LogoURL:      "/mini-app/assets/brand-mark.png",
			StartImage:   "",
			Copy:         map[string]map[string]string{"ru": {}},
			FAQ:          map[string][]FAQItem{"ru": {}},
			Links: map[string]string{
				"support": strings.TrimSpace(config.SupportURL()),
				"channel": firstNonEmpty(config.RequiredChannelSubscriptionURL(), config.ChannelURL()),
			},
			CustomLinks: []CustomLink{},
			Verification: TelegramVerificationSettings{
				Text:   "<b>Link-Bot Верификация</b>\n\nЧтобы открыть доступ к боту и mini app, подпишитесь на наш Telegram-канал <b>%s</b>.\n\nТам мы публикуем новости, обновления сервиса, важные изменения и полезные анонсы.\n\nПосле подписки нажмите кнопку ниже.",
				Banner: "",
				ChannelButton: TelegramButtonSettings{
					Text: "Link-Bot",
				},
				ConfirmButton: TelegramButtonSettings{
					Text: "✅ Я подписался",
				},
				CheckFailedText:   "Не удалось проверить подписку. Попробуйте ещё раз",
				NotSubscribedText: "Подписка пока не найдена",
				VerifiedText:      "Готово, доступ открыт",
			},
			Support: TelegramSupportSettings{
				NewTicketText:     "🆕 <b>Новое обращение #{ticket_id}</b>\n\n👤 <b>Пользователь:</b> {name}\n🔗 <b>Username:</b> {username}\n💎 <b>Подписка:</b> {subscription}\n\n💬 <b>Сообщение:</b>\n{message}",
				CustomerReplyText: "📩 <b>Обращение #{ticket_id}</b>\nПолучен новый ответ от пользователя.\n\n👤 <b>Пользователь:</b> {name}\n🔗 <b>Username:</b> {username}\n💎 <b>Подписка:</b> {subscription}\n\n{message}",
				AdminReplyText:    "📬 <b>Обращение #{ticket_id}</b>\nПоддержка ответила на ваше сообщение.\n\n{message}",
				ClosedText:        "💌 <b>Обращение #{ticket_id} закрыто.</b>\nИстория переписки доступна в Mini app.",
				OpenButton: TelegramButtonSettings{
					Text: "Открыть Mini app",
				},
			},
			StartMenu: TelegramStartMenuSettings{
				TrialButton: TelegramButtonSettings{
					Text:              "Попробовать бесплатно",
					IconCustomEmojiID: "5276422526350681413",
				},
				DashboardButton: TelegramButtonSettings{
					Text:              "Вход",
					IconCustomEmojiID: "5278413853577734640",
				},
				PlansButton: TelegramButtonSettings{
					Text:              "Тарифы",
					IconCustomEmojiID: "5206626000665868017",
				},
				SupportButton: TelegramButtonSettings{
					Text:              "Чат с поддержкой",
					IconCustomEmojiID: "5206222720416643915",
				},
			},
			Commerce: TelegramCommerceSettings{
				Banner:             "",
				TariffsText:        `<b><tg-emoji emoji-id="5206626000665868017">☺️</tg-emoji> Выберите подходящий тариф</b>`,
				PaymentMethodsText: `<b><tg-emoji emoji-id="5192678313415434135">☺️</tg-emoji> Выберите способ оплаты</b>`,
				PaymentReadyText:   `<b><tg-emoji emoji-id="5278305362703835500">☺️</tg-emoji> Подписка активируется автоматически после оплаты</b>`,
				YookassaButton: TelegramButtonSettings{
					Text:              "СБП | Карта",
					IconCustomEmojiID: "5192678313415434135",
				},
				CryptoButton: TelegramButtonSettings{
					Text:              "CryptoPay",
					IconCustomEmojiID: "5195058841988914267",
				},
				StarsButton: TelegramButtonSettings{
					Text:              "Telegram Stars",
					IconCustomEmojiID: "5242644275014951846",
				},
				PayButton: TelegramButtonSettings{
					Text:              "Оплатить",
					IconCustomEmojiID: "5206401524200145033",
				},
				BackButton: TelegramButtonSettings{
					Text:              "Назад",
					IconCustomEmojiID: "5877629862306385808",
				},
				SuccessText: "<b>Подписка успешно активирована ✅</b>\n\nПриятного и безопасного интернета!",
				SuccessButton: TelegramButtonSettings{
					Text:              "Личный кабинет",
					IconCustomEmojiID: "5278413853577734640",
				},
			},
		},
		Appearance: AppearanceSettings{
			BackgroundMode: "animated",
			Compact:        true,
			ShowFrames:     true,
			Colors: map[string]string{
				"background":     "#000000",
				"surface":        "#08090c",
				"surfaceStrong":  "#0b0d12",
				"text":           "#f3f3f3",
				"muted":          "#a0a0a0",
				"border":         "#2a2d33",
				"button":         "#0b0d12",
				"buttonText":     "#f3f3f3",
				"icon":           "#f3f3f3",
				"accent":         "#ba173d",
				"success":        "#2da44e",
				"danger":         "#f85149",
				"unlimitedBadge": "#949494",
				"gridBackground": "#000000",
				"gridLine":       "#ffffff",
				"gridGlowLeft":   "#ffffff",
				"gridGlowRight":  "#ffffff",
				"waveBackground": "#000000",
				"waveDot":        "#ebebeb",
			},
		},
		Layout: LayoutSettings{
			Elements:    defaultLayoutElements(),
			PlanColumns: 2,
			LogoWidth:   188,
		},
		Plans: defaultPlans(),
		Trial: TrialSettings{
			Enabled:              config.TrialDays() > 0,
			Days:                 config.TrialDays(),
			TrafficGB:            config.TrialTrafficLimitGB(),
			UnlimitedTraffic:     config.TrialTrafficLimitGB() == 0,
			DeviceLimit:          config.TrialDeviceLimit(),
			InternalSquadUUIDs:   uuidMapStrings(config.TrialInternalSquads()),
			ExternalSquadUUID:    uuidString(config.TrialExternalSquadUUID()),
			TrafficResetStrategy: config.TrialTrafficLimitResetStrategy(),
			Tag:                  config.TrialRemnawaveTag(),
		},
	}
}

func defaultPlans() []PlanSettings {
	return []PlanSettings{}
}

func uuidMapStrings(values map[uuid.UUID]uuid.UUID) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		if value != uuid.Nil {
			result = append(result, value.String())
		}
	}
	sort.Strings(result)
	return result
}

func uuidString(value uuid.UUID) string {
	if value == uuid.Nil {
		return ""
	}
	return value.String()
}

func defaultLayoutElements() []LayoutElement {
	profileGroups := map[string]string{
		"server_status": "main", "media": "main", "news": "main",
		"payments":  "purchases",
		"referrals": "programs", "reviews": "programs",
		"terms":         "help",
		"login_methods": "account", "web_version": "account", "pwa_install": "account",
	}
	definitions := []struct {
		area   string
		ids    []string
		width  int
		height int
		align  string
		framed bool
	}{
		{"buy", []string{"plans", "checkout"}, 100, 220, "left", false},
		{"support", []string{"actions", "tabs", "tickets"}, 100, 92, "left", false},
		{"profile", []string{"server_status", "referrals", "reviews", "payments", "media", "login_methods", "news", "web_version", "pwa_install", "terms"}, 100, 52, "left", true},
		{"navigation", []string{"dashboard", "buy", "support", "settings", "admin"}, 44, 38, "center", true},
	}

	result := make([]LayoutElement, 0, 56)
	for _, group := range definitions {
		for order, id := range group.ids {
			height := group.height
			if group.area == "dashboard" && id == "brand" {
				height = 150
			}
			if group.area == "dashboard" && id == "actions" {
				height = 92
			}
			if group.area == "buy" && id == "plans" {
				height = 330
			}
			if group.area == "buy" && id == "checkout" {
				height = 250
			}
			if group.area == "support" && id == "tabs" {
				height = 44
			}
			if group.area == "support" && id == "tickets" {
				height = 220
			}
			result = append(result, LayoutElement{
				ID: id, Area: group.area, Order: order, Visible: true,
				Width: float64(group.width), Height: height, Framed: group.framed, Align: group.align,
				Group: profileGroups[id],
			})
		}
	}

	details := []LayoutElement{
		{ID: "logo", Area: "dashboard", Order: 10, Visible: true, Width: 60, Height: 150, Align: "center"},
		{ID: "username", Area: "dashboard", Order: 11, Visible: true, Width: 100, Height: 28, Align: "center"},
		{ID: "plan_name", Area: "dashboard", Order: 12, Visible: true, Width: 48, Height: 32, Align: "left"},
		{ID: "expires", Area: "dashboard", Order: 13, Visible: true, Width: 48, Height: 32, Align: "right"},
		{ID: "traffic", Area: "dashboard", Order: 14, Visible: true, Width: 48, Height: 36, Align: "left"},
		{ID: "devices", Area: "dashboard", Order: 15, Visible: true, Width: 48, Height: 36, Align: "right"},
		{ID: "primary_action", Area: "dashboard", Order: 16, Visible: true, Width: 100, Height: 44, Align: "center"},
		{ID: "secondary_action", Area: "dashboard", Order: 17, Visible: true, Width: 100, Height: 44, Align: "center"},
		{ID: "plan_1m", Area: "buy", Order: 10, Visible: true, Width: 100, Height: 112, Align: "left"},
		{ID: "plan_1m_unlimited", Area: "buy", Order: 11, Visible: true, Width: 100, Height: 112, Align: "left"},
		{ID: "plan_3m", Area: "buy", Order: 12, Visible: true, Width: 100, Height: 112, Align: "left"},
		{ID: "plan_3m_unlimited", Area: "buy", Order: 13, Visible: true, Width: 100, Height: 112, Align: "left"},
		{ID: "plan_6m", Area: "buy", Order: 14, Visible: true, Width: 100, Height: 112, Align: "left"},
		{ID: "plan_6m_unlimited", Area: "buy", Order: 15, Visible: true, Width: 100, Height: 112, Align: "left"},
		{ID: "plan_12m", Area: "buy", Order: 16, Visible: true, Width: 100, Height: 92, Align: "left"},
		{ID: "summary", Area: "buy", Order: 20, Visible: true, Width: 100, Height: 72, Align: "left"},
		{ID: "payment", Area: "buy", Order: 21, Visible: true, Width: 100, Height: 64, Align: "left"},
		{ID: "promo", Area: "buy", Order: 22, Visible: true, Width: 100, Height: 72, Align: "left"},
		{ID: "pay_button", Area: "buy", Order: 23, Visible: true, Width: 100, Height: 44, Align: "left"},
		{ID: "new_ticket", Area: "support", Order: 10, Visible: true, Width: 100, Height: 64, Align: "left"},
		{ID: "faq", Area: "support", Order: 11, Visible: true, Width: 100, Height: 64, Align: "left"},
		{ID: "tabs_detail", Area: "support", Order: 12, Visible: true, Width: 100, Height: 44, Align: "left"},
		{ID: "tickets_detail", Area: "support", Order: 13, Visible: true, Width: 100, Height: 220, Align: "left"},
		{ID: "group_main", Area: "profile", Order: 20, Visible: true, Width: 100, Height: 28, Align: "left"},
		{ID: "group_purchases", Area: "profile", Order: 21, Visible: true, Width: 100, Height: 28, Align: "left"},
		{ID: "group_programs", Area: "profile", Order: 22, Visible: true, Width: 100, Height: 28, Align: "left"},
		{ID: "group_help", Area: "profile", Order: 23, Visible: true, Width: 100, Height: 28, Align: "left"},
		{ID: "group_account", Area: "profile", Order: 24, Visible: true, Width: 100, Height: 28, Align: "left"},
	}
	result = append(result, details...)
	return result
}

func (s *Service) Snapshot() Settings {
	current, _ := s.value.Load().(Settings)
	return cloneSettings(current)
}

func (s *Service) Update(ctx context.Context, next Settings, updatedBy int64) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := NormalizeAndValidate(&next); err != nil {
		return Settings{}, err
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return Settings{}, fmt.Errorf("encode runtime settings: %w", err)
	}
	if err := s.repository.Save(ctx, raw, updatedBy); err != nil {
		return Settings{}, fmt.Errorf("save runtime settings: %w", err)
	}
	s.value.Store(next)
	return cloneSettings(next), nil
}

func (s *Service) FeatureEnabled(name string) bool {
	settings := s.Snapshot()
	enabled, ok := settings.Features[name]
	return ok && enabled
}

func (s *Service) Maintenance() MaintenanceSettings {
	return s.Snapshot().Maintenance
}

func (s *Service) TrialSettings() TrialSettings {
	return s.Snapshot().Trial
}

func (s *Service) StartText(locale, fallback string) string {
	content := s.Snapshot().Content
	if value := strings.TrimSpace(content.StartTextRU); value != "" {
		return value
	}
	return fallback
}

func (s *Service) StartImage(fallback string) string {
	if value := strings.TrimSpace(s.Snapshot().Content.StartImage); value != "" {
		return value
	}
	return fallback
}

func (s *Service) ContentText(locale, key, fallback string) string {
	settings := s.Snapshot()
	if values := settings.Content.Copy["ru"]; values != nil {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	return fallback
}

func NormalizeTelegramCustomEmojiID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	matches := customEmojiTagPattern.FindAllStringSubmatch(value, -1)
	if len(matches) > 1 {
		return "", errors.New("only one premium emoji is allowed for a button")
	}
	if len(matches) == 1 {
		value = matches[0][1]
	}
	if !customEmojiIDPattern.MatchString(value) {
		return "", errors.New("invalid premium emoji ID for a button")
	}
	return value, nil
}

func NormalizeTelegramButtonStyle(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "primary", "success", "danger":
		return value, nil
	default:
		return "", errors.New("invalid Telegram button color")
	}
}

func (s *Service) Link(name, fallback string) string {
	if value := strings.TrimSpace(s.Snapshot().Content.Links[name]); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func FeatureNames() []string {
	return append([]string(nil), featureOrder...)
}

func NormalizeAndValidate(settings *Settings) error {
	if settings == nil {
		return errors.New("settings are required")
	}
	defaults := DefaultSettings()
	previousVersion := settings.Version
	settings.Version = CurrentVersion

	if settings.Features == nil {
		settings.Features = map[string]bool{}
	}
	for _, name := range featureOrder {
		if _, ok := settings.Features[name]; !ok {
			settings.Features[name] = defaults.Features[name]
		}
	}
	for name := range settings.Features {
		if !contains(featureOrder, name) {
			delete(settings.Features, name)
		}
	}

	normalizeMaintenance(&settings.Maintenance, defaults.Maintenance)
	if err := validateContent(&settings.Content, defaults.Content, previousVersion < CurrentVersion); err != nil {
		return err
	}
	if err := validateAppearance(&settings.Appearance, defaults.Appearance); err != nil {
		return err
	}
	if err := validateLayout(&settings.Layout, defaults.Layout); err != nil {
		return err
	}
	if err := validatePlans(&settings.Plans, defaults.Plans); err != nil {
		return err
	}
	if err := validateTrial(&settings.Trial, defaults.Trial, previousVersion < CurrentVersion); err != nil {
		return err
	}
	return nil
}

func normalizeMaintenance(value *MaintenanceSettings, defaults MaintenanceSettings) {
	if strings.TrimSpace(value.TitleRU) == "" {
		value.TitleRU = defaults.TitleRU
	}
	if strings.TrimSpace(value.TextRU) == "" {
		value.TextRU = defaults.TextRU
	}
	if strings.TrimSpace(value.ReasonRU) == "" {
		value.ReasonRU = defaults.ReasonRU
	}
	value.TitleRU = limit(value.TitleRU, 100)
	value.TextRU = limit(value.TextRU, 600)
	value.ReasonRU = limit(value.ReasonRU, 180)
}

func validateContent(value *ContentSettings, defaults ContentSettings, legacy bool) error {
	value.BrandName = strings.TrimSpace(value.BrandName)
	if value.BrandName == "" {
		value.BrandName = defaults.BrandName
	}
	value.BrandName = limit(value.BrandName, 80)
	value.AdminContact = strings.TrimSpace(value.AdminContact)
	if value.AdminContact != "" {
		value.AdminContact = strings.TrimPrefix(value.AdminContact, "https://t.me/")
		value.AdminContact = strings.TrimPrefix(value.AdminContact, "http://t.me/")
		value.AdminContact = strings.TrimPrefix(value.AdminContact, "@")
		if !telegramUserPattern.MatchString(value.AdminContact) {
			return errors.New("admin contact must be a Telegram username")
		}
		value.AdminContact = "@" + value.AdminContact
	}
	value.LogoURL = strings.TrimSpace(value.LogoURL)
	if value.LogoURL == "" {
		value.LogoURL = defaults.LogoURL
	}
	if !isSafeAssetURL(value.LogoURL) {
		return errors.New("logo must be an HTTPS URL or local /mini-app path")
	}
	value.StartTextRU = limit(strings.TrimSpace(value.StartTextRU), 3500)
	value.StartImage = strings.TrimSpace(value.StartImage)
	if value.StartImage != "" && !isSafeAssetURL(value.StartImage) {
		return errors.New("start image must be an HTTPS URL or local /mini-app path")
	}

	if value.Copy == nil {
		value.Copy = map[string]map[string]string{"ru": {}}
	}
	if value.Copy["ru"] == nil {
		value.Copy["ru"] = map[string]string{}
	}
	if len(value.Copy["ru"]) > 500 {
		return errors.New("too many ru content overrides")
	}
	cleanCopy := make(map[string]string, len(value.Copy["ru"]))
	for key, text := range value.Copy["ru"] {
		key = strings.TrimSpace(key)
		if !contentKeyPattern.MatchString(key) {
			return fmt.Errorf("invalid content key %q", key)
		}
		cleanCopy[key] = limit(strings.TrimSpace(text), 3500)
	}
	if legacy && cleanCopy["setup"] == "Установить и настроить" {
		cleanCopy["setup"] = "Подключиться"
	}
	value.Copy = map[string]map[string]string{"ru": cleanCopy}

	if value.FAQ == nil {
		value.FAQ = map[string][]FAQItem{"ru": {}}
	}
	items := value.FAQ["ru"]
	if len(items) > 100 {
		return errors.New("too many ru FAQ items")
	}
	cleanFAQ := make([]FAQItem, 0, len(items))
	for _, item := range items {
		item.Question = limit(strings.TrimSpace(item.Question), 300)
		item.Answer = limit(strings.TrimSpace(item.Answer), 3000)
		if item.Question == "" || item.Answer == "" {
			continue
		}
		cleanFAQ = append(cleanFAQ, item)
	}
	value.FAQ = map[string][]FAQItem{"ru": cleanFAQ}

	buttonEmojiID, err := NormalizeTelegramCustomEmojiID(value.SubscriptionReminderButton.IconCustomEmojiID)
	if err != nil {
		return err
	}
	buttonStyle, err := NormalizeTelegramButtonStyle(value.SubscriptionReminderButton.Style)
	if err != nil {
		return err
	}
	value.SubscriptionReminderButton.IconCustomEmojiID = buttonEmojiID
	value.SubscriptionReminderButton.Style = buttonStyle

	if err := normalizeTelegramContent(value, defaults, legacy); err != nil {
		return err
	}

	if value.Links == nil {
		value.Links = map[string]string{}
	}
	allowedLinks := map[string]bool{"support": true, "channel": true}
	for _, name := range []string{"support", "channel"} {
		link := strings.TrimSpace(value.Links[name])
		if legacy && link == "" {
			link = defaults.Links[name]
		}
		if link != "" && !isSafeWebURL(link) {
			return fmt.Errorf("invalid %s URL", name)
		}
		value.Links[name] = link
	}
	for name := range value.Links {
		if !allowedLinks[name] {
			delete(value.Links, name)
		}
	}

	if len(value.CustomLinks) > 20 {
		return errors.New("too many custom profile links")
	}
	seen := map[string]bool{}
	cleanLinks := make([]CustomLink, 0, len(value.CustomLinks))
	for _, item := range value.CustomLinks {
		item.ID = strings.ToLower(strings.TrimSpace(item.ID))
		if !elementIDPattern.MatchString(item.ID) || seen[item.ID] {
			return fmt.Errorf("invalid or duplicate custom link id %q", item.ID)
		}
		if !isSafeWebURL(strings.TrimSpace(item.URL)) {
			return fmt.Errorf("invalid custom link URL for %q", item.ID)
		}
		seen[item.ID] = true
		item.LabelRU = limit(strings.TrimSpace(item.LabelRU), 80)
		item.HintRU = limit(strings.TrimSpace(item.HintRU), 160)
		item.URL = strings.TrimSpace(item.URL)
		item.Icon = limit(strings.TrimSpace(item.Icon), 40)
		if item.LabelRU == "" {
			return fmt.Errorf("custom link %q requires a label", item.ID)
		}
		cleanLinks = append(cleanLinks, item)
	}
	value.CustomLinks = cleanLinks
	return nil
}

func normalizeTelegramContent(value *ContentSettings, defaults ContentSettings, legacy bool) error {
	value.Verification.Text = normalizedRequiredText(value.Verification.Text, defaults.Verification.Text, 3500)
	value.Verification.CheckFailedText = normalizedRequiredText(value.Verification.CheckFailedText, defaults.Verification.CheckFailedText, 300)
	value.Verification.NotSubscribedText = normalizedRequiredText(value.Verification.NotSubscribedText, defaults.Verification.NotSubscribedText, 300)
	value.Verification.VerifiedText = normalizedRequiredText(value.Verification.VerifiedText, defaults.Verification.VerifiedText, 300)
	if legacy && strings.TrimSpace(value.Verification.Banner) == "" {
		value.Verification.Banner = defaults.Verification.Banner
	}
	if err := normalizeOptionalAsset(&value.Verification.Banner, "verification banner"); err != nil {
		return err
	}
	if err := normalizeTelegramButton(&value.Verification.ChannelButton, defaults.Verification.ChannelButton); err != nil {
		return fmt.Errorf("verification channel button: %w", err)
	}
	if err := normalizeTelegramButton(&value.Verification.ConfirmButton, defaults.Verification.ConfirmButton); err != nil {
		return fmt.Errorf("verification confirm button: %w", err)
	}
	value.Verification.ChannelChatID = strings.TrimSpace(value.Verification.ChannelChatID)
	if value.Verification.ChannelChatID != "" {
		if _, ok := ParseTelegramChannelChatID(value.Verification.ChannelChatID); !ok {
			return errors.New("verification channel chat ID must be @username or a numeric Telegram chat ID")
		}
	}

	value.Support.NewTicketText = normalizedRequiredText(value.Support.NewTicketText, defaults.Support.NewTicketText, 3500)
	value.Support.CustomerReplyText = normalizedRequiredText(value.Support.CustomerReplyText, defaults.Support.CustomerReplyText, 3500)
	value.Support.AdminReplyText = normalizedRequiredText(value.Support.AdminReplyText, defaults.Support.AdminReplyText, 3500)
	value.Support.ClosedText = normalizedRequiredText(value.Support.ClosedText, defaults.Support.ClosedText, 3500)
	if err := normalizeTelegramButton(&value.Support.OpenButton, defaults.Support.OpenButton); err != nil {
		return fmt.Errorf("support mini app button: %w", err)
	}

	if err := normalizeTelegramButton(&value.StartMenu.TrialButton, defaults.StartMenu.TrialButton); err != nil {
		return fmt.Errorf("start trial button: %w", err)
	}
	if err := normalizeTelegramButton(&value.StartMenu.DashboardButton, defaults.StartMenu.DashboardButton); err != nil {
		return fmt.Errorf("start dashboard button: %w", err)
	}
	if err := normalizeTelegramButton(&value.StartMenu.PlansButton, defaults.StartMenu.PlansButton); err != nil {
		return fmt.Errorf("start plans button: %w", err)
	}
	if err := normalizeTelegramButton(&value.StartMenu.SupportButton, defaults.StartMenu.SupportButton); err != nil {
		return fmt.Errorf("start support button: %w", err)
	}

	value.Commerce.TariffsText = normalizedRequiredText(value.Commerce.TariffsText, defaults.Commerce.TariffsText, 3500)
	value.Commerce.PaymentMethodsText = normalizedRequiredText(value.Commerce.PaymentMethodsText, defaults.Commerce.PaymentMethodsText, 3500)
	value.Commerce.PaymentReadyText = normalizedRequiredText(value.Commerce.PaymentReadyText, defaults.Commerce.PaymentReadyText, 3500)
	value.Commerce.SuccessText = normalizedRequiredText(value.Commerce.SuccessText, defaults.Commerce.SuccessText, 3500)
	if legacy && strings.TrimSpace(value.Commerce.Banner) == "" {
		value.Commerce.Banner = defaults.Commerce.Banner
	}
	if err := normalizeOptionalAsset(&value.Commerce.Banner, "tariff flow banner"); err != nil {
		return err
	}
	if err := normalizeOptionalAsset(&value.Commerce.SuccessBanner, "subscription success banner"); err != nil {
		return err
	}
	buttons := []struct {
		name     string
		value    *TelegramButtonSettings
		fallback TelegramButtonSettings
	}{
		{"YooKassa button", &value.Commerce.YookassaButton, defaults.Commerce.YookassaButton},
		{"CryptoPay button", &value.Commerce.CryptoButton, defaults.Commerce.CryptoButton},
		{"Telegram Stars button", &value.Commerce.StarsButton, defaults.Commerce.StarsButton},
		{"payment button", &value.Commerce.PayButton, defaults.Commerce.PayButton},
		{"back button", &value.Commerce.BackButton, defaults.Commerce.BackButton},
		{"success button", &value.Commerce.SuccessButton, defaults.Commerce.SuccessButton},
	}
	for _, item := range buttons {
		if err := normalizeTelegramButton(item.value, item.fallback); err != nil {
			return fmt.Errorf("%s: %w", item.name, err)
		}
	}
	return nil
}

func normalizeTelegramButton(value *TelegramButtonSettings, fallback TelegramButtonSettings) error {
	value.Text = normalizedRequiredText(value.Text, fallback.Text, 64)
	emojiID, err := NormalizeTelegramCustomEmojiID(value.IconCustomEmojiID)
	if err != nil {
		return err
	}
	style, err := NormalizeTelegramButtonStyle(value.Style)
	if err != nil {
		return err
	}
	value.IconCustomEmojiID = emojiID
	value.Style = style
	return nil
}

func normalizeOptionalAsset(value *string, name string) error {
	*value = strings.TrimSpace(*value)
	if *value != "" && !isSafeAssetURL(*value) {
		return fmt.Errorf("%s must be an HTTP(S) URL or local /assets path", name)
	}
	return nil
}

func normalizedRequiredText(value, fallback string, max int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = strings.TrimSpace(fallback)
	}
	return limit(value, max)
}

func validateAppearance(value *AppearanceSettings, defaults AppearanceSettings) error {
	value.BackgroundMode = strings.ToLower(strings.TrimSpace(value.BackgroundMode))
	if value.BackgroundMode == "" {
		value.BackgroundMode = defaults.BackgroundMode
	}
	if value.BackgroundMode != "animated" && value.BackgroundMode != "grid" && value.BackgroundMode != "solid" {
		return errors.New("background mode must be animated, grid or solid")
	}
	if value.Colors == nil {
		value.Colors = map[string]string{}
	}
	for name, fallback := range defaults.Colors {
		color := strings.ToLower(strings.TrimSpace(value.Colors[name]))
		if color == "" {
			color = fallback
		}
		if !hexColorPattern.MatchString(color) {
			return fmt.Errorf("invalid color %q", name)
		}
		value.Colors[name] = color
	}
	for name := range value.Colors {
		if _, ok := defaults.Colors[name]; !ok {
			delete(value.Colors, name)
		}
	}
	return nil
}

func validateLayout(value *LayoutSettings, defaults LayoutSettings) error {
	// Dashboard sections are structural wrappers. Keeping their legacy geometry
	// would make a wrapper and its children move at the same time in the editor.
	filtered := value.Elements[:0]
	for _, item := range value.Elements {
		if item.Area == "dashboard" && contains([]string{"brand", "subscription", "actions"}, item.ID) {
			continue
		}
		filtered = append(filtered, item)
	}
	value.Elements = filtered
	if value.PlanColumns < 1 || value.PlanColumns > 2 {
		value.PlanColumns = defaults.PlanColumns
	}
	if value.LogoWidth < 48 || value.LogoWidth > 220 {
		value.LogoWidth = defaults.LogoWidth
	}
	if len(value.Elements) == 0 {
		value.Elements = defaults.Elements
	}
	defaultByKey := make(map[string]LayoutElement, len(defaults.Elements))
	present := make(map[string]bool, len(value.Elements))
	for _, item := range defaults.Elements {
		defaultByKey[item.Area+":"+item.ID] = item
	}
	for _, item := range value.Elements {
		present[strings.ToLower(strings.TrimSpace(item.Area))+":"+strings.ToLower(strings.TrimSpace(item.ID))] = true
	}
	for _, item := range defaults.Elements {
		if !present[item.Area+":"+item.ID] {
			value.Elements = append(value.Elements, item)
		}
	}
	if len(value.Elements) > 100 {
		return errors.New("too many layout elements")
	}

	seen := map[string]bool{}
	for index := range value.Elements {
		item := &value.Elements[index]
		item.ID = strings.ToLower(strings.TrimSpace(item.ID))
		item.Area = strings.ToLower(strings.TrimSpace(item.Area))
		key := item.Area + ":" + item.ID
		if !elementIDPattern.MatchString(item.ID) || !contains([]string{"dashboard", "buy", "support", "profile", "navigation"}, item.Area) || seen[key] {
			return fmt.Errorf("invalid or duplicate layout element %q", key)
		}
		seen[key] = true
		fallback, hasFallback := defaultByKey[key]
		if !hasFallback {
			fallback = LayoutElement{Width: 100, Height: 52, Align: "left"}
			if item.Area == "navigation" {
				fallback.Width, fallback.Height, fallback.Align = 44, 38, "center"
			}
		}
		if item.Order < 0 {
			item.Order = 0
		}
		item.Align = strings.ToLower(strings.TrimSpace(item.Align))
		item.Group = strings.ToLower(strings.TrimSpace(item.Group))
		if item.Area == "profile" && !strings.HasPrefix(item.ID, "group_") {
			if item.Group == "" {
				item.Group = fallback.Group
			}
			if !contains([]string{"main", "purchases", "programs", "help", "account"}, item.Group) {
				return fmt.Errorf("invalid profile group for %q", key)
			}
		} else {
			item.Group = ""
		}
		if item.Align == "" && hasFallback {
			item.Align = fallback.Align
		}
		if !contains([]string{"left", "center", "right"}, item.Align) {
			return fmt.Errorf("invalid layout alignment for %q", key)
		}
		if item.OffsetX < -1000 {
			item.OffsetX = -1000
		} else if item.OffsetX > 1000 {
			item.OffsetX = 1000
		}
		if item.OffsetY < -1000 {
			item.OffsetY = -1000
		} else if item.OffsetY > 1000 {
			item.OffsetY = 1000
		}
		if item.PositionX != nil {
			if math.IsNaN(*item.PositionX) || math.IsInf(*item.PositionX, 0) {
				item.PositionX = nil
			} else {
				value := math.Max(-2000, math.Min(2000, *item.PositionX))
				item.PositionX = &value
			}
		}
		if item.PositionY != nil {
			if math.IsNaN(*item.PositionY) || math.IsInf(*item.PositionY, 0) {
				item.PositionY = nil
			} else {
				value := math.Max(-2000, math.Min(4000, *item.PositionY))
				item.PositionY = &value
			}
		}
		if item.Area == "navigation" {
			if item.Width < 28 || item.Width > 100 {
				item.Width = fallback.Width
			}
			if item.Height < 24 || item.Height > 96 {
				item.Height = fallback.Height
			}
		} else if item.Area == "profile" {
			if item.Width < 10 || item.Width > 150 {
				item.Width = fallback.Width
			}
			if item.Height < 20 || item.Height > 180 {
				item.Height = fallback.Height
			}
		} else {
			if item.Width < 10 || item.Width > 150 {
				item.Width = fallback.Width
			}
			if item.Height < 20 || item.Height > 720 {
				item.Height = fallback.Height
			}
		}
	}
	sort.SliceStable(value.Elements, func(i, j int) bool {
		if value.Elements[i].Area == value.Elements[j].Area {
			return value.Elements[i].Order < value.Elements[j].Order
		}
		return value.Elements[i].Area < value.Elements[j].Area
	})
	return nil
}

func validatePlans(value *[]PlanSettings, defaults []PlanSettings) error {
	if *value == nil {
		*value = append([]PlanSettings(nil), defaults...)
	}
	defaultByID := map[string]PlanSettings{}
	for _, item := range defaults {
		defaultByID[item.ID] = item
	}
	provided := map[string]PlanSettings{}
	result := make([]PlanSettings, 0, len(*value))
	for _, item := range *value {
		item.ID = strings.ToLower(strings.TrimSpace(item.ID))
		if !elementIDPattern.MatchString(item.ID) {
			return fmt.Errorf("invalid plan %q", item.ID)
		}
		if _, exists := provided[item.ID]; exists {
			return fmt.Errorf("duplicate plan %q", item.ID)
		}
		fallback := defaultByID[item.ID]
		if item.Months == 0 {
			item.Months = fallback.Months
			if item.Months == 0 {
				item.Months = inferPlanMonths(item.ID)
			}
		}
		if item.Months < 0 || item.Months > 120 || (item.Enabled && item.Months == 0) {
			return fmt.Errorf("invalid duration for plan %q", item.ID)
		}
		if item.PriceRub < 0 || item.PriceRub > 1000000 {
			return fmt.Errorf("invalid price for plan %q", item.ID)
		}
		item.PriceStars = planbook.StarsForRub(item.PriceRub)
		if item.TrafficGB < 0 || item.TrafficGB > 1000000 || item.DeviceLimit < 0 || item.DeviceLimit > 1000 {
			return fmt.Errorf("invalid limits for plan %q", item.ID)
		}
		item.TitleRU = limit(strings.TrimSpace(item.TitleRU), 80)
		item.TitleEN = limit(strings.TrimSpace(item.TitleEN), 80)
		if item.TitleRU == "" {
			item.TitleRU = fallback.TitleRU
			if item.TitleRU == "" && item.Months > 0 {
				item.TitleRU = planTitleRU(item.Months)
			}
		}
		if item.TitleEN == "" {
			item.TitleEN = fallback.TitleEN
			if item.TitleEN == "" && item.Months > 0 {
				item.TitleEN = planTitleEN(item.Months)
			}
		}
		var err error
		item.InternalSquadUUIDs, err = normalizeUUIDStrings(item.InternalSquadUUIDs)
		if err != nil {
			return fmt.Errorf("invalid internal squad for plan %q: %w", item.ID, err)
		}
		item.ExternalSquadUUID, err = normalizeOptionalUUID(item.ExternalSquadUUID)
		if err != nil {
			return fmt.Errorf("invalid external squad for plan %q: %w", item.ID, err)
		}
		provided[item.ID] = item
		result = append(result, item)
	}
	if len(result) > 100 {
		return errors.New("too many plans")
	}
	*value = result
	return nil
}

func validateTrial(value *TrialSettings, defaults TrialSettings, legacy bool) error {
	if legacy {
		*value = defaults
	}
	if value.Days < 0 || value.Days > 365 {
		return errors.New("invalid trial duration")
	}
	if value.TrafficGB < 0 || value.TrafficGB > 1000000 || value.DeviceLimit < 0 || value.DeviceLimit > 1000 {
		return errors.New("invalid trial limits")
	}
	if value.Days == 0 {
		value.Enabled = false
	}
	if value.UnlimitedTraffic {
		value.TrafficGB = 0
	}
	strategy := strings.ToUpper(strings.TrimSpace(value.TrafficResetStrategy))
	switch strategy {
	case "NO_RESET", "DAY", "WEEK", "MONTH":
		value.TrafficResetStrategy = strategy
	default:
		value.TrafficResetStrategy = "MONTH"
	}
	value.Tag = limit(strings.TrimSpace(value.Tag), 64)
	var err error
	value.InternalSquadUUIDs, err = normalizeUUIDStrings(value.InternalSquadUUIDs)
	if err != nil {
		return fmt.Errorf("invalid trial internal squad: %w", err)
	}
	value.ExternalSquadUUID, err = normalizeOptionalUUID(value.ExternalSquadUUID)
	if err != nil {
		return fmt.Errorf("invalid trial external squad: %w", err)
	}
	return nil
}

func normalizeUUIDStrings(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value, err := normalizeOptionalUUID(raw)
		if err != nil {
			return nil, err
		}
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeOptionalUUID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	value, err := uuid.Parse(raw)
	if err != nil || value == uuid.Nil {
		return "", errors.New("invalid UUID")
	}
	return value.String(), nil
}

func inferPlanMonths(id string) int {
	prefix := strings.SplitN(id, "m", 2)[0]
	months, err := strconv.Atoi(prefix)
	if err != nil || months < 1 || months > 120 {
		return 0
	}
	return months
}

func planTitleRU(months int) string {
	ending := "месяцев"
	if months%10 == 1 && months%100 != 11 {
		ending = "месяц"
	} else if months%10 >= 2 && months%10 <= 4 && (months%100 < 12 || months%100 > 14) {
		ending = "месяца"
	}
	return fmt.Sprintf("%d %s", months, ending)
}

func planTitleEN(months int) string {
	ending := "months"
	if months == 1 {
		ending = "month"
	}
	return fmt.Sprintf("%d %s", months, ending)
}

func cloneSettings(value Settings) Settings {
	raw, err := json.Marshal(value)
	if err != nil {
		return DefaultSettings()
	}
	var clone Settings
	if err := json.Unmarshal(raw, &clone); err != nil {
		return DefaultSettings()
	}
	return clone
}

func isSafeAssetURL(value string) bool {
	if strings.HasPrefix(value, "/mini-app/") || strings.HasPrefix(value, "/assets/") || strings.HasPrefix(value, "assets/") {
		return true
	}
	return isSafeWebURL(value)
}

func isSafeWebURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "https" || parsed.Scheme == "http"
}

func ParseTelegramChannelChatID(raw string) (any, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	if strings.HasPrefix(raw, "-100") {
		chatID, err := strconv.ParseInt(raw, 10, 64)
		return chatID, err == nil
	}
	if strings.HasPrefix(raw, "@") {
		username := strings.TrimPrefix(raw, "@")
		if telegramUserPattern.MatchString(username) {
			return "@" + username, true
		}
		return nil, false
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		parsed, err := url.Parse(raw)
		if err != nil || !strings.EqualFold(parsed.Host, "t.me") {
			return nil, false
		}
		raw = strings.Trim(parsed.Path, "/")
	}
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "t.me/"))
	raw = strings.TrimPrefix(raw, "@")
	if index := strings.Index(raw, "/"); index >= 0 {
		raw = raw[:index]
	}
	if !telegramUserPattern.MatchString(raw) {
		return nil, false
	}
	return "@" + raw, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func limit(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
