package integrations

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/database"
)

const (
	ProviderYooKassa        = "yookassa"
	ProviderCryptoPay       = "cryptopay"
	ProviderNotificationBot = "notification_bot"
	ProviderLava            = "lava"
	ProviderWata            = "wata"
	ProviderPlatega         = "platega"
	ProviderFreeKassa       = "freekassa"
	ProviderHeleket         = "heleket"
	ProviderPally           = "pally"
)

type FieldDefinition struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Help        string `json:"help,omitempty"`
}

type ProviderDefinition struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Logo        string            `json:"logo"`
	Kind        string            `json:"kind"`
	Fields      []FieldDefinition `json:"fields"`
}

type FieldView struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Help        string `json:"help,omitempty"`
	Value       string `json:"value,omitempty"`
	Configured  bool   `json:"configured,omitempty"`
}

type ProviderView struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Logo        string      `json:"logo"`
	Kind        string      `json:"kind"`
	Enabled     bool        `json:"enabled"`
	Configured  bool        `json:"configured"`
	WebhookURL  string      `json:"webhookUrl,omitempty"`
	UpdatedAt   string      `json:"updatedAt,omitempty"`
	Fields      []FieldView `json:"fields"`
}

type UpdateInput struct {
	Enabled bool              `json:"enabled"`
	Fields  map[string]string `json:"fields"`
}

type record struct {
	Enabled      bool
	Config       map[string]string
	WebhookToken string
	UpdatedAt    time.Time
}

type Service struct {
	repository  *database.PaymentIntegrationRepository
	aead        cipher.AEAD
	legacyAEADs []cipher.AEAD
	baseURL     string
	mu          sync.RWMutex
	records     map[string]record
}

var definitions = []ProviderDefinition{
	{
		ID: ProviderYooKassa, Name: "YooKassa", Description: "СБП и банковские карты", Logo: "/mini-app/assets/payment-card.png", Kind: "payment",
		Fields: []FieldDefinition{
			{Key: "shopId", Label: "Shop ID", Required: true, Placeholder: "Идентификатор магазина"},
			{Key: "secretKey", Label: "Секретный ключ", Required: true, Secret: true, Placeholder: "Ключ API"},
			{Key: "email", Label: "Email для чеков", Required: true, Placeholder: "mail@example.com"},
			{Key: "apiUrl", Label: "API URL", Required: true, Placeholder: "https://api.yookassa.ru/v3"},
		},
	},
	{
		ID: ProviderCryptoPay, Name: "Crypto Pay", Description: "Оплата криптовалютой через Telegram", Logo: "/mini-app/assets/payment-crypto.png", Kind: "payment",
		Fields: []FieldDefinition{
			{Key: "token", Label: "API token", Required: true, Secret: true, Placeholder: "Токен Crypto Pay"},
			{Key: "apiUrl", Label: "API URL", Required: true, Placeholder: "https://pay.crypt.bot"},
			{Key: "acceptedAssets", Label: "Принимаемые валюты", Placeholder: "USDT,TON,BTC,ETH,LTC,BNB,TRX,USDC"},
		},
	},
	{
		ID: ProviderNotificationBot, Name: "Бот уведомлений", Description: "Присылает администратору сведения об оплатах", Logo: "/mini-app/assets/payment-telegram.png", Kind: "notification",
		Fields: []FieldDefinition{
			{Key: "token", Label: "Токен бота", Required: true, Secret: true, Placeholder: "123456:ABC..."},
		},
	},
	{
		ID: ProviderLava, Name: "LAVA", Description: "Платёжная форма LAVA Business", Logo: "/mini-app/assets/payment-lava.png", Kind: "payment",
		Fields: []FieldDefinition{
			{Key: "shopId", Label: "Shop ID", Required: true, Placeholder: "UUID проекта"},
			{Key: "secretKey", Label: "Секретный ключ", Required: true, Secret: true},
			{Key: "additionalKey", Label: "Дополнительный ключ", Required: true, Secret: true, Help: "Используется для проверки webhook"},
		},
	},
	{
		ID: ProviderWata, Name: "WATA", Description: "Карты и СБП через WATA", Logo: "/mini-app/assets/payment-wata.png", Kind: "payment",
		Fields: []FieldDefinition{
			{Key: "accessToken", Label: "Access token", Required: true, Secret: true, Help: "JWT из кабинета мерчанта"},
			{Key: "apiUrl", Label: "API URL", Required: true, Placeholder: "https://api.wata.pro/api/h2h"},
		},
	},
	{
		ID: ProviderPlatega, Name: "Platega", Description: "Платёжная форма Platega", Logo: "/mini-app/assets/payment-platega.png", Kind: "payment",
		Fields: []FieldDefinition{
			{Key: "merchantId", Label: "Merchant ID", Required: true},
			{Key: "secretKey", Label: "API ключ", Required: true, Secret: true},
			{Key: "apiUrl", Label: "API URL", Required: true, Placeholder: "https://app.platega.io"},
		},
	},
	{
		ID: ProviderFreeKassa, Name: "FreeKassa", Description: "Платёжная форма FreeKassa", Logo: "/mini-app/assets/payment-freekassa.png", Kind: "payment",
		Fields: []FieldDefinition{
			{Key: "shopId", Label: "ID магазина", Required: true},
			{Key: "secretWord", Label: "Секретное слово", Required: true, Secret: true},
			{Key: "secretWord2", Label: "Секретное слово 2", Required: true, Secret: true, Help: "Проверка уведомлений"},
		},
	},
	{
		ID: ProviderHeleket, Name: "Heleket", Description: "Криптовалютная платёжная форма", Logo: "/mini-app/assets/payment-heleket.png", Kind: "payment",
		Fields: []FieldDefinition{
			{Key: "merchantId", Label: "Merchant UUID", Required: true},
			{Key: "apiKey", Label: "Payment API key", Required: true, Secret: true},
			{Key: "apiUrl", Label: "API URL", Required: true, Placeholder: "https://api.heleket.com"},
		},
	},
	{
		ID: ProviderPally, Name: "Pally", Description: "Карты и СБП через Pally", Logo: "/mini-app/assets/payment-pally.png", Kind: "payment",
		Fields: []FieldDefinition{
			{Key: "shopId", Label: "Shop ID", Required: true, Placeholder: "ID магазина в Pally"},
			{Key: "apiToken", Label: "API token", Required: true, Secret: true, Placeholder: "Токен API магазина"},
			{Key: "apiUrl", Label: "API URL", Required: true, Placeholder: "https://pal24.pro"},
		},
	},
}

func NewService(ctx context.Context, repository *database.PaymentIntegrationRepository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("payment integration repository is required")
	}
	aead, err := integrationAEAD("link-bot-integrations-v1:")
	if err != nil {
		return nil, err
	}
	legacyAEAD, err := integrationAEAD("bruhvpn-integrations-v1:")
	if err != nil {
		return nil, err
	}

	service := &Service{
		repository:  repository,
		aead:        aead,
		legacyAEADs: []cipher.AEAD{legacyAEAD},
		baseURL:     integrationBaseURL(),
		records:     make(map[string]record, len(definitions)),
	}
	if err := service.loadAndImport(ctx); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) loadAndImport(ctx context.Context) error {
	items, err := s.repository.List(ctx)
	if err != nil {
		return fmt.Errorf("list payment integrations: %w", err)
	}
	existing := make(map[string]database.PaymentIntegration, len(items))
	for _, item := range items {
		existing[item.Provider] = item
	}

	for _, definition := range definitions {
		item, ok := existing[definition.ID]
		if !ok {
			cfg, enabled := legacyConfig(definition.ID)
			token, tokenErr := randomToken()
			if tokenErr != nil {
				return tokenErr
			}
			encrypted, encryptErr := s.encrypt(cfg)
			if encryptErr != nil {
				return encryptErr
			}
			item = database.PaymentIntegration{Provider: definition.ID, Enabled: enabled, EncryptedConfig: encrypted, WebhookToken: token}
			if err := s.repository.Upsert(ctx, item); err != nil {
				return fmt.Errorf("create integration %s: %w", definition.ID, err)
			}
		}
		cfg, err := s.decrypt(item.EncryptedConfig)
		if err != nil {
			for _, legacyAEAD := range s.legacyAEADs {
				cfg, err = decryptWithAEAD(item.EncryptedConfig, legacyAEAD)
				if err == nil {
					reencrypted, encryptErr := s.encrypt(cfg)
					if encryptErr != nil {
						return fmt.Errorf("reencrypt integration %s: %w", definition.ID, encryptErr)
					}
					item.EncryptedConfig = reencrypted
					if upsertErr := s.repository.Upsert(ctx, item); upsertErr != nil {
						return fmt.Errorf("save reencrypted integration %s: %w", definition.ID, upsertErr)
					}
					break
				}
			}
		}
		if err != nil {
			return fmt.Errorf("decrypt integration %s: %w", definition.ID, err)
		}
		s.records[definition.ID] = record{Enabled: item.Enabled, Config: cfg, WebhookToken: item.WebhookToken, UpdatedAt: item.UpdatedAt}
	}
	return nil
}

func (s *Service) ListAdmin() []ProviderView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	views := make([]ProviderView, 0, len(definitions))
	for _, definition := range definitions {
		rec := s.records[definition.ID]
		fields := make([]FieldView, 0, len(definition.Fields))
		configured := true
		for _, field := range definition.Fields {
			value := strings.TrimSpace(rec.Config[field.Key])
			if field.Required && value == "" {
				configured = false
			}
			view := FieldView{Key: field.Key, Label: field.Label, Placeholder: field.Placeholder, Secret: field.Secret, Required: field.Required, Help: field.Help, Configured: value != ""}
			if !field.Secret {
				view.Value = value
			}
			fields = append(fields, view)
		}
		updatedAt := ""
		if !rec.UpdatedAt.IsZero() {
			updatedAt = rec.UpdatedAt.UTC().Format(time.RFC3339)
		}
		webhookURL := ""
		if definition.Kind == "payment" && rec.WebhookToken != "" {
			webhookURL = fmt.Sprintf("%s/api/payments/webhook/%s/%s", s.baseURL, definition.ID, rec.WebhookToken)
		}
		views = append(views, ProviderView{ID: definition.ID, Name: definition.Name, Description: definition.Description, Logo: definition.Logo, Kind: definition.Kind, Enabled: rec.Enabled, Configured: configured, WebhookURL: webhookURL, UpdatedAt: updatedAt, Fields: fields})
	}
	return views
}

func (s *Service) Update(ctx context.Context, provider string, input UpdateInput, updatedBy int64) (ProviderView, error) {
	definition, ok := definitionByID(provider)
	if !ok {
		return ProviderView{}, fmt.Errorf("unknown integration: %s", provider)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.records[provider]
	if rec.Config == nil {
		rec.Config = map[string]string{}
	}
	allowed := make(map[string]FieldDefinition, len(definition.Fields))
	for _, field := range definition.Fields {
		allowed[field.Key] = field
		if raw, exists := input.Fields[field.Key]; exists {
			value := strings.TrimSpace(raw)
			if field.Secret && value == "" {
				continue
			}
			rec.Config[field.Key] = value
		}
	}
	for key := range input.Fields {
		if _, exists := allowed[key]; !exists {
			return ProviderView{}, fmt.Errorf("unknown field %q", key)
		}
	}
	if input.Enabled {
		for _, field := range definition.Fields {
			if field.Required && strings.TrimSpace(rec.Config[field.Key]) == "" {
				return ProviderView{}, fmt.Errorf("заполните поле «%s»", field.Label)
			}
		}
	}
	if rec.WebhookToken == "" {
		token, err := randomToken()
		if err != nil {
			return ProviderView{}, err
		}
		rec.WebhookToken = token
	}
	rec.Enabled = input.Enabled
	rec.UpdatedAt = time.Now().UTC()
	encrypted, err := s.encrypt(rec.Config)
	if err != nil {
		return ProviderView{}, err
	}
	actor := updatedBy
	if err := s.repository.Upsert(ctx, database.PaymentIntegration{Provider: provider, Enabled: rec.Enabled, EncryptedConfig: encrypted, WebhookToken: rec.WebhookToken, UpdatedBy: &actor}); err != nil {
		return ProviderView{}, err
	}
	s.records[provider] = rec
	return s.adminViewLocked(*definition, rec), nil
}

func (s *Service) Enabled(provider string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[provider]
	return ok && rec.Enabled && configured(provider, rec.Config)
}

func (s *Service) Config(provider string) (map[string]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[provider]
	if !ok || !rec.Enabled || !configured(provider, rec.Config) {
		return nil, false
	}
	clone := make(map[string]string, len(rec.Config))
	for key, value := range rec.Config {
		clone[key] = value
	}
	return clone, true
}

func (s *Service) WebhookToken(provider string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.records[provider].WebhookToken
}

func (s *Service) WebhookURL(provider string) string {
	token := s.WebhookToken(provider)
	if token == "" {
		return ""
	}
	return fmt.Sprintf("%s/api/payments/webhook/%s/%s", s.baseURL, provider, token)
}

func (s *Service) adminViewLocked(definition ProviderDefinition, rec record) ProviderView {
	fields := make([]FieldView, 0, len(definition.Fields))
	complete := true
	for _, field := range definition.Fields {
		value := strings.TrimSpace(rec.Config[field.Key])
		if field.Required && value == "" {
			complete = false
		}
		view := FieldView{Key: field.Key, Label: field.Label, Placeholder: field.Placeholder, Secret: field.Secret, Required: field.Required, Help: field.Help, Configured: value != ""}
		if !field.Secret {
			view.Value = value
		}
		fields = append(fields, view)
	}
	webhookURL := ""
	if definition.Kind == "payment" {
		webhookURL = fmt.Sprintf("%s/api/payments/webhook/%s/%s", s.baseURL, definition.ID, rec.WebhookToken)
	}
	return ProviderView{ID: definition.ID, Name: definition.Name, Description: definition.Description, Logo: definition.Logo, Kind: definition.Kind, Enabled: rec.Enabled, Configured: complete, WebhookURL: webhookURL, UpdatedAt: rec.UpdatedAt.UTC().Format(time.RFC3339), Fields: fields}
}

func configured(provider string, cfg map[string]string) bool {
	definition, ok := definitionByID(provider)
	if !ok {
		return false
	}
	for _, field := range definition.Fields {
		if field.Required && strings.TrimSpace(cfg[field.Key]) == "" {
			return false
		}
	}
	return true
}

func definitionByID(provider string) (*ProviderDefinition, bool) {
	for i := range definitions {
		if definitions[i].ID == provider {
			return &definitions[i], true
		}
	}
	return nil, false
}

func (s *Service) encrypt(cfg map[string]string) (string, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := s.aead.Seal(nonce, nonce, raw, nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (s *Service) decrypt(value string) (map[string]string, error) {
	return decryptWithAEAD(value, s.aead)
}

func integrationAEAD(prefix string) (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(prefix + config.TelegramToken()))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func decryptWithAEAD(value string, aead cipher.AEAD) (map[string]string, error) {
	if strings.TrimSpace(value) == "" {
		return map[string]string{}, nil
	}
	raw, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	if len(raw) < aead.NonceSize() {
		return nil, errors.New("encrypted integration value is too short")
	}
	nonce, ciphertext := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	cfg := map[string]string{}
	if err := json.Unmarshal(plain, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func integrationBaseURL() string {
	raw := strings.TrimSpace(config.GetMiniAppURL())
	if parsed, err := url.Parse(raw); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed.Scheme + "://" + parsed.Host
	}
	return "https://example.com"
}

func legacyConfig(provider string) (map[string]string, bool) {
	switch provider {
	case ProviderYooKassa:
		return map[string]string{"shopId": config.YookasaShopId(), "secretKey": config.YookasaSecretKey(), "email": config.YookasaEmail(), "apiUrl": firstNonEmpty(config.YookasaUrl(), "https://api.yookassa.ru/v3")}, config.IsYookasaEnabled()
	case ProviderCryptoPay:
		return map[string]string{"token": config.CryptoPayToken(), "apiUrl": firstNonEmpty(config.CryptoPayUrl(), "https://pay.crypt.bot"), "acceptedAssets": config.CryptoPayAcceptedAssets()}, config.IsCryptoPayEnabled()
	case ProviderNotificationBot:
		return map[string]string{"token": config.PaymentNotificationBotToken(), "chatId": strconv.FormatInt(config.PaymentNotificationChatID(), 10), "timezone": firstNonEmpty(config.PaymentNotificationTimezone(), "Europe/Moscow")}, config.IsPaymentNotificationEnabled()
	case ProviderWata:
		return map[string]string{"apiUrl": "https://api.wata.pro/api/h2h"}, false
	case ProviderPlatega:
		return map[string]string{"apiUrl": "https://app.platega.io"}, false
	case ProviderHeleket:
		return map[string]string{"apiUrl": "https://api.heleket.com"}, false
	case ProviderPally:
		return map[string]string{"apiUrl": "https://pal24.pro"}, false
	default:
		return map[string]string{}, false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func SortedPaymentProviders() []string {
	items := []string{ProviderYooKassa, ProviderLava, ProviderWata, ProviderPlatega, ProviderFreeKassa, ProviderCryptoPay, ProviderHeleket, ProviderPally}
	sort.Strings(items)
	return items
}
