package webpush

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	webpushlib "github.com/SherClockHolmes/webpush-go"

	"link-bot/internal/adminnotify"
	"link-bot/internal/database"
)

type fakeRepository struct {
	mu           sync.Mutex
	items        []database.WebPushSubscription
	successIDs   []int64
	failureIDs   []int64
	deletedIDs   []int64
	subscribed   *database.WebPushSubscription
	unsubscribed string
}

func (r *fakeRepository) VAPIDConfig(context.Context) (*database.WebPushVAPIDConfig, error) {
	return &database.WebPushVAPIDConfig{PublicKey: "public", PrivateKey: "private"}, nil
}
func (r *fakeRepository) SaveVAPIDConfig(context.Context, string, string) error { return nil }
func (r *fakeRepository) UpsertSubscription(_ context.Context, item database.WebPushSubscription) error {
	r.subscribed = &item
	return nil
}
func (r *fakeRepository) DeleteSubscription(_ context.Context, _ int64, endpoint string) error {
	r.unsubscribed = endpoint
	return nil
}
func (r *fakeRepository) DeleteSubscriptionByID(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deletedIDs = append(r.deletedIDs, id)
	return nil
}
func (r *fakeRepository) ListSubscriptions(context.Context, int64) ([]database.WebPushSubscription, error) {
	return r.items, nil
}
func (r *fakeRepository) CountSubscriptions(context.Context, int64) (int, error) {
	return len(r.items), nil
}
func (r *fakeRepository) MarkSubscriptionSuccess(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.successIDs = append(r.successIDs, id)
	return nil
}
func (r *fakeRepository) MarkSubscriptionFailure(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failureIDs = append(r.failureIDs, id)
	return nil
}

func TestNotifyBuildsVisiblePayloadAndMarksSuccess(t *testing.T) {
	repository := &fakeRepository{items: []database.WebPushSubscription{{ID: 7, Endpoint: "https://push.example/sub", P256DH: "p", Auth: "a"}}}
	service := &Service{
		repository: repository,
		adminID:    42,
		publicKey:  "public",
		privateKey: "private",
		subject:    defaultSubject,
		httpClient: http.DefaultClient,
	}
	var delivered notificationPayload
	service.send = func(_ context.Context, payload []byte, _ *webpushlib.Subscription, options *webpushlib.Options) (*http.Response, error) {
		if err := json.Unmarshal(payload, &delivered); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if options.TTL != pushTTL || options.Urgency != webpushlib.UrgencyHigh {
			t.Fatalf("unexpected delivery options: %+v", options)
		}
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(""))}, nil
	}

	err := service.Notify(context.Background(), adminnotify.Event{
		Title: "Новая оплата",
		Body:  "350 RUB · 1 месяц · @admin",
		URL:   "/mini-app/?page=admin&section=finance",
		Tag:   "payment-7",
	})
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if delivered.Title != "Новая оплата" || delivered.Badge != 1 || delivered.URL != "/mini-app/?page=admin&section=finance" {
		t.Fatalf("unexpected payload: %+v", delivered)
	}
	if len(repository.successIDs) != 1 || repository.successIDs[0] != 7 {
		t.Fatalf("success IDs = %v", repository.successIDs)
	}
}

func TestNotifyRemovesExpiredSubscription(t *testing.T) {
	repository := &fakeRepository{items: []database.WebPushSubscription{{ID: 9, Endpoint: "https://push.example/expired", P256DH: "p", Auth: "a"}}}
	service := &Service{repository: repository, adminID: 42, publicKey: "public", privateKey: "private", subject: defaultSubject, httpClient: http.DefaultClient}
	service.send = func(context.Context, []byte, *webpushlib.Subscription, *webpushlib.Options) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusGone, Body: io.NopCloser(strings.NewReader("expired"))}, nil
	}

	if err := service.Notify(context.Background(), adminnotify.Event{Title: "Ошибка"}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if len(repository.deletedIDs) != 1 || repository.deletedIDs[0] != 9 {
		t.Fatalf("deleted IDs = %v", repository.deletedIDs)
	}
}

func TestSubscribeRejectsNonHTTPSAndStoresValidSubscription(t *testing.T) {
	repository := &fakeRepository{}
	service := &Service{repository: repository, adminID: 42}
	if err := service.Subscribe(context.Background(), SubscriptionInput{Endpoint: "http://push.example", P256DH: "p", Auth: "a"}); err == nil {
		t.Fatal("expected non-HTTPS endpoint to be rejected")
	}
	if err := service.Subscribe(context.Background(), SubscriptionInput{Endpoint: "https://push.example/sub", P256DH: "p", Auth: "a", UserAgent: "Safari"}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if repository.subscribed == nil || repository.subscribed.AdminTelegramID != 42 || repository.subscribed.UserAgent != "Safari" {
		t.Fatalf("stored subscription = %+v", repository.subscribed)
	}
}
