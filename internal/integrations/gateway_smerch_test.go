package integrations

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/paymentchannel"
)

const smerchTestID = "81c18912-d408-43eb-8257-5dd8b2b44f84"
const smerchTestEventID = "df7d379c-0911-4b24-9f1a-ed69e484a717"
const smerchTestTransaction = `{"uuid":"` + smerchTestID + `","external_id":"42","amount":17050,"gross_amount":18500,"net_amount":16000,"currency":"RUB","payment_method":"sbp","status":"paid","payment_url":"https://pay.smerch.io/invoice/fixture"}`

func smerchTestGateway(endpoint string) (*Gateway, map[string]string) {
	cfg := map[string]string{"apiKey": "live_fixture", "webhookSecret": "fixture-webhook-secret", "paymentMethod": "sbp", "apiUrl": endpoint}
	svc := &Service{baseURL: "https://vpn.example", records: map[string]record{ProviderSmerch: {Enabled: true, WebhookToken: "fixture-route-token", Config: cfg}}}
	return NewGateway(svc), cfg
}

func smerchTestSign(key, value string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func smerchTestHeaders(raw []byte) http.Header {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	hash := sha256.Sum256(raw)
	h := make(http.Header)
	h.Set("X-Api-Key", "live_fixture")
	h.Set("X-Webhook-Version", "v1")
	h.Set("X-Webhook-Id", smerchTestEventID)
	h.Set("X-Webhook-Timestamp", timestamp)
	h.Set("X-Webhook-Key-Id", "key-1")
	h.Set("X-Webhook-Signature", smerchTestSign("fixture-webhook-secret", "v1\n"+smerchTestEventID+"\n"+timestamp+"\n"+hex.EncodeToString(hash[:])))
	return h
}

func assertSmerchRequest(t *testing.T, r *http.Request) []byte {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	requestID, timestamp := r.Header.Get("X-Smerch-Request-Id"), r.Header.Get("X-Timestamp")
	if _, err := uuid.Parse(requestID); err != nil {
		t.Error("missing unique request ID")
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || math.Abs(float64(time.Now().Unix()-seconds)) > 5 {
		t.Error("bad request timestamp")
	}
	hash := sha256.Sum256(raw)
	canonical := r.Method + "\n" + r.URL.Path + "\n\n" + requestID + "\n" + timestamp + "\n" + hex.EncodeToString(hash[:])
	if r.Header.Get("X-Api-Key") != "live_fixture" || r.Header.Get("X-Signature-Version") != "v2" || r.Header.Get("X-Signature") != smerchTestSign("live_fixture", canonical) {
		t.Error("request does not match documented Smerch HMAC v2 contract")
	}
	return raw
}

func TestSmerchCreatesSignedMinorUnitOrdersAndReusesExternalID(t *testing.T) {
	var previousBody, previousRequestID string
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw := assertSmerchRequest(t, r)
		if r.Method != "POST" || r.URL.Path != "/api/v1/integration/transactions" {
			t.Errorf("unexpected endpoint %s %s", r.Method, r.URL.Path)
		}
		var payload struct {
			Amount      int64             `json:"amount"`
			Currency    string            `json:"currency"`
			ExternalID  string            `json:"external_id"`
			Method      string            `json:"payment_method"`
			Description string            `json:"description"`
			Callback    string            `json:"callback_url"`
			Success     string            `json:"success_url"`
			Fail        string            `json:"fail_url"`
			Metadata    map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Amount != 17050 || payload.Currency != "RUB" || payload.ExternalID != "42" || payload.Method != "sbp" || payload.Description != "VPN 30 дней" || payload.Metadata["userId"] != "customer-88" || payload.Metadata["userName"] != "payer" || payload.Metadata["clientIp"] != "1.1.1.1" {
			t.Errorf("bad create payload: %s", raw)
		}
		if payload.Callback != "https://vpn.example/api/payments/webhook/smerch/fixture-route-token" || payload.Success != "https://vpn.example/return" || payload.Fail != payload.Success {
			t.Error("callback or return URL missing")
		}
		if calls > 1 && (previousBody != string(raw) || previousRequestID == r.Header.Get("X-Smerch-Request-Id")) {
			t.Error("retry must preserve order body but use a fresh anti-replay request ID")
		}
		previousBody, previousRequestID = string(raw), r.Header.Get("X-Smerch-Request-Id")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(smerchTestTransaction))
	}))
	defer server.Close()
	g, _ := smerchTestGateway(server.URL)
	for range 2 {
		got, err := g.Create(context.Background(), CreatePaymentRequest{Provider: ProviderSmerch, PurchaseID: 42, Amount: 170.5, CustomerID: 88, Username: "payer", ClientIP: "1.1.1.1", Description: "VPN 30 дней", ReturnURL: "https://vpn.example/return"})
		if err != nil || got.ExternalID != smerchTestID || got.URL != "https://pay.smerch.io/invoice/fixture" {
			t.Fatalf("bad create: %+v, %v", got, err)
		}
	}
}

func TestSmerchWebhookVerifiesLiveStatusBeforeFulfilment(t *testing.T) {
	raw := []byte(`{"event":"transaction.status_updated","transaction":` + smerchTestTransaction + `}`)
	calls := 0
	current := smerchTestTransaction
	status := 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := assertSmerchRequest(t, r); len(got) != 0 || r.Method != "GET" || r.URL.Path != "/api/v1/integration/transactions/"+smerchTestID {
			t.Error("webhook verification did not use an authenticated GET by UUID")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(current))
	}))
	defer server.Close()
	g, _ := smerchTestGateway(server.URL)
	for range 2 { // A delivery replay resolves to the same purchase, not another order.
		event, err := g.HandleWebhook(context.Background(), ProviderSmerch, smerchTestHeaders(raw), raw, nil)
		if err != nil || !event.Paid || event.Ignored || event.PurchaseID != 42 || event.ExternalID != smerchTestID || event.Amount != 170.5 || event.Currency != "RUB" {
			t.Fatalf("invalid paid event: %+v, %v", event, err)
		}
	}
	if calls != 2 {
		t.Fatal("paid event was not verified")
	}
	for _, replacement := range []struct{ from, to string }{{`"42"`, `"43"`}, {`17050`, `17049`}, {`"RUB"`, `"USD"`}, {smerchTestID, smerchTestEventID}, {`"sbp"`, `"crypto"`}} {
		current = strings.ReplaceAll(smerchTestTransaction, replacement.from, replacement.to)
		if _, err := g.HandleWebhook(context.Background(), ProviderSmerch, smerchTestHeaders(raw), raw, nil); !errors.Is(err, ErrInvalidSmerchWebhook) {
			t.Fatalf("mismatched transaction accepted: %s, %v", current, err)
		}
	}
	current = strings.ReplaceAll(smerchTestTransaction, `"paid"`, `"refunded"`)
	if event, err := g.HandleWebhook(context.Background(), ProviderSmerch, smerchTestHeaders(raw), raw, nil); err != nil || event.Paid || !event.Ignored {
		t.Fatalf("late paid event after refund must not fulfil: %+v, %v", event, err)
	}
	current = strings.ReplaceAll(smerchTestTransaction, `"paid"`, `"pending"`)
	if _, err := g.HandleWebhook(context.Background(), ProviderSmerch, smerchTestHeaders(raw), raw, nil); err == nil || errors.Is(err, ErrInvalidSmerchWebhook) {
		t.Fatalf("paid callback racing with pending API state must be retried: %v", err)
	}
	status, current = 503, `{"error":"temporary failure, live_secret_must_not_leak"}`
	if _, err := g.HandleWebhook(context.Background(), ProviderSmerch, smerchTestHeaders(raw), raw, nil); err == nil || errors.Is(err, ErrInvalidSmerchWebhook) || strings.Contains(err.Error(), "live_secret") {
		t.Fatalf("transient API failure must be safe and retryable: %v", err)
	}
}

func TestSmerchRejectsTamperedAndMalformedWebhooksBeforeCallingAPI(t *testing.T) {
	valid := `{"event":"transaction.status_updated","transaction":` + smerchTestTransaction + `}`
	for _, body := range []string{`{`, strings.Replace(valid, `17050`, `17050.5`, 1), strings.Replace(valid, `17050`, `0`, 1), strings.Replace(valid, `"RUB"`, `"USD"`, 1), strings.Replace(valid, `"42"`, `"042"`, 1), strings.Replace(valid, smerchTestID, "not-a-uuid", 1)} {
		g, _ := smerchTestGateway("https://never-called.example")
		if _, err := g.HandleWebhook(context.Background(), ProviderSmerch, smerchTestHeaders([]byte(body)), []byte(body), nil); !errors.Is(err, ErrInvalidSmerchWebhook) {
			t.Errorf("accepted malformed body: %s, %v", body, err)
		}
	}
	raw := []byte(valid)
	for _, header := range []string{"X-Webhook-Signature", "X-Webhook-Id", "X-Webhook-Version", "X-Webhook-Timestamp"} {
		h := smerchTestHeaders(raw)
		h.Set(header, "tampered")
		if err := verifySmerchWebhook(map[string]string{"webhookSecret": "fixture-webhook-secret"}, h, raw, time.Now()); err == nil {
			t.Errorf("accepted tampered %s", header)
		}
	}
	if err := verifySmerchWebhook(map[string]string{"webhookSecret": "fixture-webhook-secret"}, smerchTestHeaders(raw), append(raw, ' '), time.Now()); err == nil {
		t.Fatal("signature checked against reformatted body")
	}
	for _, status := range []string{"pending", "failed", "expired", "refunded", "chargeback"} {
		body := []byte(strings.ReplaceAll(valid, `"paid"`, strconv.Quote(status)))
		g, _ := smerchTestGateway("https://never-called.example")
		event, err := g.HandleWebhook(context.Background(), ProviderSmerch, smerchTestHeaders(body), body, nil)
		if err != nil || event.Paid || event.Cancelled || !event.Ignored {
			t.Errorf("non-paid event %s granted or cancelled purchase: %+v %v", status, event, err)
		}
	}
	for _, environment := range []string{"test", "sandbox"} {
		body := []byte(strings.Replace(valid, `"transaction":{`, `"transaction":{"environment":"`+environment+`",`, 1))
		g, _ := smerchTestGateway("https://never-called.example")
		event, err := g.HandleWebhook(context.Background(), ProviderSmerch, smerchTestHeaders(body), body, nil)
		if err != nil || event.Paid || !event.Ignored {
			t.Errorf("sandbox event granted a purchase: %+v %v", event, err)
		}
	}
	g, _ := smerchTestGateway("https://never-called.example")
	body := []byte(`{"event":"refund.status_updated"}`)
	if event, err := g.HandleWebhook(context.Background(), ProviderSmerch, smerchTestHeaders(body), body, nil); err != nil || !event.Ignored {
		t.Errorf("unrelated signed event should be acknowledged: %+v %v", event, err)
	}
}

func TestSmerchWebhookSecretRotationUsesOnlySelectedKey(t *testing.T) {
	raw := []byte(`{}`)
	h := smerchTestHeaders(raw)
	cfg := map[string]string{"webhookSecret": "fixture-webhook-secret", "webhookKeyId": "key-1"}
	if err := verifySmerchWebhook(cfg, h, raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	h.Set("X-Webhook-Previous-Key-Id", "key-1")
	h.Set("X-Webhook-Previous-Signature", h.Get("X-Webhook-Signature"))
	h.Set("X-Webhook-Key-Id", "key-2")
	h.Set("X-Webhook-Signature", strings.Repeat("0", 64))
	if err := verifySmerchWebhook(cfg, h, raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	cfg["webhookKeyId"] = "unrelated-key"
	if err := verifySmerchWebhook(cfg, h, raw, time.Now()); err == nil {
		t.Fatal("used secret without matching key ID")
	}
}

func TestSmerchRejectsInvalidAmountsAndSandboxKeys(t *testing.T) {
	g, cfg := smerchTestGateway("https://never-called.example")
	for _, amount := range []float64{0, -1, math.NaN(), math.Inf(1), 0.001, 170.501, 1e20} {
		if _, err := g.createSmerch(context.Background(), cfg, CreatePaymentRequest{PurchaseID: 1, Amount: amount, Currency: "RUB"}); err == nil {
			t.Errorf("accepted invalid amount %v", amount)
		}
	}
	cfg["apiKey"] = "test_fixture"
	if _, err := g.createSmerch(context.Background(), cfg, CreatePaymentRequest{PurchaseID: 1, Amount: 100, Currency: "RUB"}); err == nil {
		t.Fatal("sandbox key accepted")
	}
	if configured(ProviderSmerch, map[string]string{}) {
		t.Fatal("unconfigured Smerch enabled")
	}
	for _, method := range []string{"sbp_recurring", "../sbp", "<script>"} {
		if paymentchannel.Valid(ProviderSmerch, method) {
			t.Errorf("accepted invalid one-off method %q", method)
		}
	}
}

func TestSmerchRejectsWrongOrderOrUnsafeCheckoutAndHidesAPIBodies(t *testing.T) {
	for _, body := range []string{
		strings.ReplaceAll(smerchTestTransaction, `"42"`, `"99"`),
		strings.ReplaceAll(smerchTestTransaction, `17050`, `17049`),
		strings.ReplaceAll(smerchTestTransaction, `"RUB"`, `"USD"`),
		strings.ReplaceAll(smerchTestTransaction, `"sbp"`, `"crypto"`),
		strings.ReplaceAll(smerchTestTransaction, `https://pay.smerch.io/invoice/fixture`, `javascript:alert(1)`),
		strings.ReplaceAll(smerchTestTransaction, `https://pay.smerch.io/invoice/fixture`, ``),
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		g, _ := smerchTestGateway(server.URL)
		if _, err := g.Create(context.Background(), CreatePaymentRequest{Provider: ProviderSmerch, PurchaseID: 42, Amount: 170.5}); err == nil {
			t.Errorf("accepted response: %s", body)
		}
		server.Close()
	}
	for _, status := range []int{401, 403, 409, 422, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = fmt.Fprint(w, `{"message":"live_secret and private payment details"}`)
		}))
		g, _ := smerchTestGateway(server.URL)
		_, err := g.Create(context.Background(), CreatePaymentRequest{Provider: ProviderSmerch, PurchaseID: 42, Amount: 170.5})
		var safe *SmerchPaymentError
		if !errors.As(err, &safe) || safe.StatusCode != status || strings.Contains(safe.AdminMessage()+safe.PublicMessage()+err.Error(), "live_secret") {
			t.Fatalf("unsafe or untyped API error: %v", err)
		}
		server.Close()
	}
}

func TestSmerchMethodEditorIncludesConfiguredMethodAndKeepsCopiesSeparate(t *testing.T) {
	g, cfg := smerchTestGateway("https://never-called.example")
	cfg["paymentMethod"] = "card_international"
	catalog, err := g.PaymentChannels(context.Background(), "smerch__aabbccdd")
	if err != nil || len(catalog.Options) != 3 || catalog.Multiple || catalog.Options[2].ID != "card_international" {
		t.Fatalf("configured method missing from editor: %+v, %v", catalog, err)
	}
	copy := channelConfig(cfg, CreatePaymentRequest{Provider: ProviderSmerch, Channels: []string{"sbp"}})
	if copy["paymentMethod"] != "sbp" || cfg["paymentMethod"] != "card_international" {
		t.Fatal("per-row selection mutated shared Smerch integration")
	}
	if _, err := paymentchannel.Resolve("smerch__aabbccdd", []string{"sbp"}, "card_international"); err == nil {
		t.Fatal("checkout escaped the row's allowed methods")
	}
}
