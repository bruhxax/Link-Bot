package webpush

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	webpushlib "github.com/SherClockHolmes/webpush-go"

	"link-bot/internal/adminnotify"
	"link-bot/internal/database"
)

const (
	defaultSubject = "https://github.com/bruhxax/Link-Bot"
	pushTTL        = 12 * 60 * 60
)

type repository interface {
	VAPIDConfig(context.Context) (*database.WebPushVAPIDConfig, error)
	SaveVAPIDConfig(context.Context, string, string) error
	UpsertSubscription(context.Context, database.WebPushSubscription) error
	DeleteSubscription(context.Context, int64, string) error
	DeleteSubscriptionByID(context.Context, int64) error
	ListSubscriptions(context.Context, int64) ([]database.WebPushSubscription, error)
	CountSubscriptions(context.Context, int64) (int, error)
	MarkSubscriptionSuccess(context.Context, int64) error
	MarkSubscriptionFailure(context.Context, int64) error
}

type sendNotificationFunc func(context.Context, []byte, *webpushlib.Subscription, *webpushlib.Options) (*http.Response, error)

type Service struct {
	repository repository
	adminID    int64
	publicKey  string
	privateKey string
	subject    string
	httpClient *http.Client
	send       sendNotificationFunc
	branding   func() Branding
}

type Branding struct {
	IconURL string
}

type SubscriptionInput struct {
	Endpoint  string
	P256DH    string
	Auth      string
	UserAgent string
}

type State struct {
	Available         bool   `json:"available"`
	PublicKey         string `json:"publicKey,omitempty"`
	SubscriptionCount int    `json:"subscriptionCount"`
}

type notificationPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag,omitempty"`
	Badge int    `json:"badge"`
	Icon  string `json:"icon,omitempty"`
}

func NewService(ctx context.Context, repository repository, adminID int64, subject string) (*Service, error) {
	if repository == nil {
		return nil, errors.New("web push repository is required")
	}
	if adminID == 0 {
		return nil, errors.New("admin Telegram ID is required")
	}

	config, err := repository.VAPIDConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load VAPID config: %w", err)
	}
	if config == nil {
		privateKey, publicKey, generateErr := webpushlib.GenerateVAPIDKeys()
		if generateErr != nil {
			return nil, fmt.Errorf("generate VAPID keys: %w", generateErr)
		}
		if err := repository.SaveVAPIDConfig(ctx, publicKey, privateKey); err != nil {
			return nil, fmt.Errorf("save VAPID config: %w", err)
		}
		config, err = repository.VAPIDConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("reload VAPID config: %w", err)
		}
	}
	if config == nil || strings.TrimSpace(config.PublicKey) == "" || strings.TrimSpace(config.PrivateKey) == "" {
		return nil, errors.New("VAPID config is empty")
	}

	return &Service{
		repository: repository,
		adminID:    adminID,
		publicKey:  config.PublicKey,
		privateKey: config.PrivateKey,
		subject:    normalizeSubject(subject),
		httpClient: &http.Client{Timeout: 8 * time.Second},
		send:       webpushlib.SendNotificationWithContext,
	}, nil
}

func normalizeSubject(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err == nil && parsed.Scheme == "https" && parsed.Host != "" {
		return parsed.Scheme + "://" + parsed.Host
	}
	return defaultSubject
}

func (s *Service) SetBrandingProvider(provider func() Branding) {
	if s != nil {
		s.branding = provider
	}
}

func (s *Service) State(ctx context.Context) (State, error) {
	if s == nil {
		return State{}, nil
	}
	count, err := s.repository.CountSubscriptions(ctx, s.adminID)
	if err != nil {
		return State{}, err
	}
	return State{Available: true, PublicKey: s.publicKey, SubscriptionCount: count}, nil
}

func (s *Service) Subscribe(ctx context.Context, input SubscriptionInput) error {
	if s == nil {
		return errors.New("web push is unavailable")
	}
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.P256DH = strings.TrimSpace(input.P256DH)
	input.Auth = strings.TrimSpace(input.Auth)
	input.UserAgent = truncate(strings.TrimSpace(input.UserAgent), 500)
	parsed, err := url.Parse(input.Endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || len(input.Endpoint) > 2048 {
		return errors.New("invalid push endpoint")
	}
	if input.P256DH == "" || input.Auth == "" || len(input.P256DH) > 512 || len(input.Auth) > 512 {
		return errors.New("invalid push encryption keys")
	}
	return s.repository.UpsertSubscription(ctx, database.WebPushSubscription{
		AdminTelegramID: s.adminID,
		Endpoint:        input.Endpoint,
		P256DH:          input.P256DH,
		Auth:            input.Auth,
		UserAgent:       input.UserAgent,
	})
}

func (s *Service) Unsubscribe(ctx context.Context, endpoint string) error {
	if s == nil {
		return errors.New("web push is unavailable")
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || len(endpoint) > 2048 {
		return errors.New("invalid push endpoint")
	}
	return s.repository.DeleteSubscription(ctx, s.adminID, endpoint)
}

func (s *Service) Notify(ctx context.Context, event adminnotify.Event) error {
	if s == nil {
		return nil
	}
	event.Title = truncate(strings.Join(strings.Fields(event.Title), " "), 80)
	event.Body = truncate(strings.Join(strings.Fields(event.Body), " "), 320)
	if event.Title == "" {
		event.Title = "Новое событие"
	}
	if event.URL == "" || !strings.HasPrefix(event.URL, "/mini-app/") {
		event.URL = "/mini-app/"
	}

	branding := Branding{}
	if s.branding != nil {
		branding = s.branding()
	}
	payload, err := json.Marshal(notificationPayload{
		Title: event.Title,
		Body:  event.Body,
		URL:   event.URL,
		Tag:   truncate(event.Tag, 64),
		Badge: 1,
		Icon:  safeIconURL(branding.IconURL),
	})
	if err != nil {
		return err
	}
	subscriptions, err := s.repository.ListSubscriptions(ctx, s.adminID)
	if err != nil || len(subscriptions) == 0 {
		return err
	}

	var wg sync.WaitGroup
	errorsCh := make(chan error, len(subscriptions))
	for _, item := range subscriptions {
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			if sendErr := s.sendOne(ctx, payload, event.Tag, item); sendErr != nil {
				errorsCh <- sendErr
			}
		}()
	}
	wg.Wait()
	close(errorsCh)

	var deliveryErrors []error
	for deliveryErr := range errorsCh {
		deliveryErrors = append(deliveryErrors, deliveryErr)
	}
	return errors.Join(deliveryErrors...)
}

func safeIconURL(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "/mini-app/") {
		return value
	}
	parsed, err := url.Parse(value)
	if err == nil && parsed.Scheme == "https" && parsed.Host != "" {
		return value
	}
	return ""
}

func (s *Service) sendOne(ctx context.Context, payload []byte, topic string, item database.WebPushSubscription) error {
	response, err := s.send(ctx, payload, &webpushlib.Subscription{
		Endpoint: item.Endpoint,
		Keys: webpushlib.Keys{
			P256dh: item.P256DH,
			Auth:   item.Auth,
		},
	}, &webpushlib.Options{
		HTTPClient:      s.httpClient,
		Subscriber:      s.subject,
		Topic:           safeTopic(topic),
		TTL:             pushTTL,
		Urgency:         webpushlib.UrgencyHigh,
		VAPIDPublicKey:  s.publicKey,
		VAPIDPrivateKey: s.privateKey,
	})
	if err != nil {
		s.markFailure(item.ID)
		return fmt.Errorf("deliver web push subscription %d: %w", item.ID, err)
	}
	if response == nil {
		s.markFailure(item.ID)
		return fmt.Errorf("web push subscription %d returned an empty response", item.ID)
	}
	if response.Body != nil {
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		s.markSuccess(item.ID)
		return nil
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		s.deleteExpired(item.ID)
		return nil
	}
	s.markFailure(item.ID)
	return fmt.Errorf("web push subscription %d returned status %d", item.ID, response.StatusCode)
}

func (s *Service) markSuccess(id int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.repository.MarkSubscriptionSuccess(ctx, id); err != nil {
		slog.Warn("web push: mark delivery success failed", "subscriptionId", id, "error", err)
	}
}

func (s *Service) markFailure(id int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.repository.MarkSubscriptionFailure(ctx, id); err != nil {
		slog.Warn("web push: mark delivery failure failed", "subscriptionId", id, "error", err)
	}
}

func (s *Service) deleteExpired(id int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.repository.DeleteSubscriptionByID(ctx, id); err != nil {
		slog.Warn("web push: delete expired subscription failed", "subscriptionId", id, "error", err)
	}
}

func safeTopic(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "admin-event"
	}
	var result strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			result.WriteRune(char)
		}
		if result.Len() >= 32 {
			break
		}
	}
	if result.Len() == 0 {
		return "admin-event"
	}
	return result.String()
}

func truncate(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum-1]) + "…"
}
