package miniapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/google/uuid"
	"link-bot/internal/database"
)

type torrentMemory struct {
	mu        sync.Mutex
	entries   map[string]database.UserActivity
	tokens    map[string]uuid.UUID
	delivered map[string]bool
	err       error
}

func newTorrentMemory() *torrentMemory {
	return &torrentMemory{entries: map[string]database.UserActivity{}, tokens: map[string]uuid.UUID{}, delivered: map[string]bool{}}
}
func (m *torrentMemory) RecordTorrentAlert(_ context.Context, key string, _ uuid.UUID, _ int64, entry database.UserActivity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.entries[key] = entry
	return nil
}
func (m *torrentMemory) ClaimTorrentAlert(_ context.Context, key string, token uuid.UUID) (bool, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.delivered[key] {
		return false, true, nil
	}
	if m.tokens[key] != uuid.Nil {
		return false, false, nil
	}
	m.tokens[key] = token
	return true, false, nil
}
func (m *torrentMemory) FinishTorrentAlert(_ context.Context, key string, token uuid.UUID, delivered bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tokens[key] == token {
		m.delivered[key] = delivered
		delete(m.tokens, key)
	}
	return nil
}

func torrentFixture(t *testing.T, blocked bool) []byte {
	t.Helper()
	data := map[string]any{
		"node": map[string]any{"uuid": "11111111-1111-4111-8111-111111111111", "name": "Node <one>", "secretKey": "node-secret"},
		"user": map[string]any{"uuid": "22222222-2222-4222-8222-222222222222", "id": 42, "username": "<alice>", "telegramId": 999, "subscriptionUrl": "https://private.example/token", "vlessUuid": "user-secret"},
		"report": map[string]any{
			"actionReport": map[string]any{"blocked": blocked, "ip": "2001:db8::1", "blockDuration": 3600, "processedAt": "2026-10-28T10:00:00Z", "willUnblockAt": "2026-10-28T11:00:00Z", "userId": "untrusted"},
			"xrayReport":   map[string]any{"protocol": "bittorrent", "network": "tcp", "ts": 1793181600, "email": "private-email", "source": "private-source", "destination": "private-destination"},
		},
	}
	raw, err := json.Marshal(map[string]any{"scope": "torrent_blocker", "event": "torrent_blocker.report", "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "data": data})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func torrentRequest(body []byte, secret string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/remnawave/webhook", strings.NewReader(string(body)))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	r.Header.Set("X-Remnawave-Signature", hex.EncodeToString(mac.Sum(nil)))
	return r
}

func TestTorrentWebhookAuthenticationAndDelivery(t *testing.T) {
	initMiniAppTestConfig()
	secret := strings.Repeat("s", 32)
	t.Setenv("REMNAWAVE_WEBHOOK_SECRET", secret)
	var messages []string
	fail := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(502)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		if r.FormValue("chat_id") != "1" || r.FormValue("parse_mode") != "HTML" {
			t.Error("alert must go only to the owner as escaped HTML")
		}
		messages = append(messages, r.FormValue("text"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":1,"type":"private"}}}`))
	}))
	defer api.Close()
	tg, err := bot.New("test-token", bot.WithSkipGetMe(), bot.WithServerURL(api.URL))
	if err != nil {
		t.Fatal(err)
	}
	repo := newTorrentMemory()
	h := &Handler{telegramBot: tg, torrentAlertRepository: repo}
	call := func(r *http.Request) int {
		w := httptest.NewRecorder()
		h.handleRemnawaveSecurityWebhook(w, r)
		return w.Code
	}
	body := torrentFixture(t, true)
	if code := call(torrentRequest(body, "wrong-secret")); code != 401 || len(repo.entries) != 0 || len(messages) != 0 {
		t.Fatalf("forged report accepted: %d", code)
	}
	if code := call(torrentRequest(body, secret)); code != 204 || len(repo.entries) != 1 || len(messages) != 1 {
		t.Fatalf("first report: %d entries=%d messages=%d", code, len(repo.entries), len(messages))
	}
	for i := 0; i < 2; i++ {
		if code := call(torrentRequest(torrentFixture(t, true), secret)); code != 204 || len(messages) != 1 {
			t.Fatalf("duplicate with a new envelope timestamp: %d messages=%d", code, len(messages))
		}
	}
	if !strings.Contains(messages[0], "&lt;alice&gt;") || !strings.Contains(messages[0], "IP заблокирован на ноде") {
		t.Fatal(messages[0])
	}
	stored, _ := json.Marshal(repo.entries)
	for _, private := range []string{"node-secret", "user-secret", "private", "untrusted", "999"} {
		if strings.Contains(messages[0]+string(stored), private) {
			t.Fatalf("report leaked %s", private)
		}
	}
	// A failed delivery can be retried without creating another history entry.
	fail = true
	if code := call(torrentRequest(torrentFixture(t, false), secret)); code != 503 || len(repo.entries) != 2 {
		t.Fatalf("delivery failure did not remain retryable: %d", code)
	}
	fail = false
	if code := call(torrentRequest(torrentFixture(t, false), secret)); code != 204 || len(repo.entries) != 2 || len(messages) != 2 || !strings.Contains(messages[1], "IP не заблокирован") {
		t.Fatalf("failed delivery recovery: %d entries=%d messages=%d", code, len(repo.entries), len(messages))
	}
	for _, mutate := range []func(map[string]any){
		func(e map[string]any) { e["timestamp"] = time.Now().Add(-25 * time.Hour).Format(time.RFC3339Nano) },
		func(e map[string]any) { e["data"].(map[string]any)["user"].(map[string]any)["uuid"] = "bad" },
		func(e map[string]any) {
			delete(e["data"].(map[string]any)["report"].(map[string]any)["actionReport"].(map[string]any), "blocked")
		},
		func(e map[string]any) {
			e["data"].(map[string]any)["report"].(map[string]any)["actionReport"].(map[string]any)["ip"] = "invalid"
		},
	} {
		var event map[string]any
		json.Unmarshal(body, &event)
		mutate(event)
		raw, _ := json.Marshal(event)
		if code := call(torrentRequest(raw, secret)); code != 400 || len(repo.entries) != 2 {
			t.Fatalf("invalid event changed history: %d", code)
		}
	}
	r := torrentRequest(body, secret)
	r.Header.Set("X-Remnawave-Timestamp", time.Now().Add(time.Hour).Format(time.RFC3339Nano))
	if code := call(r); code != 400 {
		t.Fatalf("timestamp mismatch accepted: %d", code)
	}
	// Existing service notifications continue to use the same signed endpoint.
	login, _ := json.Marshal(map[string]any{"scope": "service", "event": "service.login_attempt_failed", "timestamp": time.Now().UTC(), "data": map[string]any{
		"loginAttempt": map[string]string{"username": "<admin>", "ip": "127.0.0.1", "userAgent": "Windows Firefox/1", "description": "Denied"},
	}})
	if code := call(torrentRequest(login, secret)); code != 204 || len(messages) != 3 || !strings.Contains(messages[2], "Неудачная попытка входа") || !strings.Contains(messages[2], "&lt;admin&gt;") {
		t.Fatalf("existing panel login alert regressed: %d", code)
	}
	repo.err = errors.New("database down")
	if code := call(torrentRequest(body, secret)); code != 503 || len(messages) != 3 {
		t.Fatalf("database error was not retryable: %d", code)
	}
}

func TestTorrentConcurrentDeliveryIsClaimedOnce(t *testing.T) {
	initMiniAppTestConfig()
	secret := strings.Repeat("s", 32)
	t.Setenv("REMNAWAVE_WEBHOOK_SECRET", secret)
	started, release := make(chan struct{}), make(chan struct{})
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":1,"type":"private"}}}`))
	}))
	defer api.Close()
	tg, _ := bot.New("test-token", bot.WithSkipGetMe(), bot.WithServerURL(api.URL))
	h := &Handler{telegramBot: tg, torrentAlertRepository: newTorrentMemory()}
	body := torrentFixture(t, true)
	done := make(chan int, 1)
	go func() {
		w := httptest.NewRecorder()
		h.handleRemnawaveSecurityWebhook(w, torrentRequest(body, secret))
		done <- w.Code
	}()
	<-started
	w := httptest.NewRecorder()
	h.handleRemnawaveSecurityWebhook(w, torrentRequest(body, secret))
	close(release)
	if w.Code != 503 || <-done != 204 {
		t.Fatalf("concurrent report not serialized: %d", w.Code)
	}
}
