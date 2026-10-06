package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

const payHotTestID = "76666666-6666-4666-8666-666666666666"
const payHotTestURL = "https://app.pay.hot/pay/75555555-5555-4555-8555-555555555555#token=phc_v2.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestPayHotCreateAndReplay(t *testing.T) {
	for _, replay := range []bool{false, true} {
		t.Run(fmt.Sprint(replay), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer phk_v2_testfixture" {
					t.Errorf("incorrect authentication or method")
				}
				if calls == 1 {
					if r.URL.Path != "/api/v2/payments" || r.Header.Get("Idempotency-Key") != "link-bot-payhot-order-4242" {
						t.Errorf("incorrect creation path/idempotency key")
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					want := map[string]any{"merchant_order_reference": "4242", "amount_minor": "17050", "currency": "RUB", "payment_method": "sbp", "customer": map[string]any{"id": "88", "email": "payer@example.com"}}
					if !reflect.DeepEqual(body, want) {
						t.Errorf("incorrect request: %#v", body)
					}
				} else if r.URL.Path != "/api/v2/payments/"+payHotTestID+"/checkout-sessions" || r.ContentLength > 0 || r.Header.Get("Idempotency-Key") != "" {
					t.Errorf("incorrect checkout reissue")
				}
				checkout := map[string]any{"url": payHotTestURL}
				if replay && calls == 1 {
					checkout = map[string]any{"reissue_required": true, "access_token_available": false}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"payment": map[string]string{"id": payHotTestID, "merchant_order_reference": "4242", "amount_minor": "17050", "currency": "RUB"}, "checkout": checkout})
			}))
			defer server.Close()
			cfg := map[string]string{"apiKey": "phk_v2_testfixture", "webhookSecret": "phws_fixture", "paymentMethod": "sbp", "apiUrl": server.URL}
			service := &Service{records: map[string]record{ProviderPayHot: {Enabled: true, Config: cfg}}}
			gateway := &Gateway{settings: service, httpClient: server.Client()}
			created, err := gateway.Create(context.Background(), CreatePaymentRequest{Provider: ProviderPayHot, PurchaseID: 4242, Amount: 170.5, CustomerID: 88, Email: "payer@example.com"})
			if err != nil || created.ExternalID != payHotTestID || created.URL != payHotTestURL {
				t.Fatalf("create: %+v, %v", created, err)
			}
			if calls != 1 && !replay || calls != 2 && replay {
				t.Fatalf("request count: %d", calls)
			}
		})
	}
}

func payHotTestHeaders(raw []byte, timestamp int64) http.Header {
	t := fmt.Sprint(timestamp)
	h := make(http.Header)
	h.Set("PayHot-Signature", "t="+t+",v2="+hmacHex(sha256.New, []byte("phws_fixture"), append([]byte(t+"."), raw...)))
	h.Set("PayHot-Event-Id", "event-1")
	return h
}

func TestPayHotSignedWebhookValidation(t *testing.T) {
	const body = `{"api_version":"v2","event_id":"event-1","event_type":"payment.succeeded","data":{"id":"76666666-6666-4666-8666-666666666666","object":"payment","status":"succeeded","merchant_order_reference":"4242","amount_minor":"17050","captured_amount_minor":"17050","currency":"RUB"}}`
	cfg := map[string]string{"webhookSecret": "phws_fixture"}
	now := time.Unix(1791200000, 0)
	event, err := verifyPayHotWebhook(cfg, payHotTestHeaders([]byte(body), now.Unix()), []byte(body), now)
	if err != nil || !event.Paid || event.Amount != 170.5 || event.PurchaseID != 4242 || event.ExternalID != payHotTestID {
		t.Fatalf("verified event: %+v, %v", event, err)
	}
	tests := []struct {
		name, body                      string
		offset                          int64
		tamper, wrongSecret, wrongEvent bool
	}{
		{name: "tampered body", body: body, tamper: true},
		{name: "wrong secret", body: body, wrongSecret: true},
		{name: "stale signature", body: body, offset: -301},
		{name: "future signature", body: body, offset: 301},
		{name: "wrong event header", body: body, wrongEvent: true},
		{name: "sandbox event", body: strings.Replace(body, "payment.succeeded", "sandbox.payment.succeeded", 1)},
		{name: "sandbox environment", body: strings.Replace(body, `"api_version"`, `"environment":"sandbox","api_version"`, 1)},
		{name: "partial capture", body: strings.Replace(body, `"captured_amount_minor":"17050"`, `"captured_amount_minor":"17049"`, 1)},
		{name: "zero amount", body: strings.Replace(body, `"amount_minor":"17050"`, `"amount_minor":"0"`, 1)},
		{name: "decimal amount", body: strings.Replace(body, `"amount_minor":"17050"`, `"amount_minor":"170.50"`, 1)},
		{name: "overflow", body: strings.Replace(body, `"amount_minor":"17050"`, `"amount_minor":"99999999999999999999"`, 1)},
		{name: "wrong currency", body: strings.Replace(body, `"RUB"`, `"USD"`, 1)},
		{name: "wrong object", body: strings.Replace(body, `"object":"payment"`, `"object":"payout"`, 1)},
		{name: "invalid payment ID", body: strings.Replace(body, payHotTestID, "invalid", 1)},
		{name: "invalid order", body: strings.Replace(body, `"4242"`, `"-1"`, 1)},
		{name: "wrong version", body: strings.Replace(body, `"v2"`, `"v1"`, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := []byte(tt.body)
			headers := payHotTestHeaders(raw, now.Unix()+tt.offset)
			config := map[string]string{"webhookSecret": "phws_fixture"}
			if tt.tamper {
				raw = append(raw, ' ')
			}
			if tt.wrongSecret {
				config["webhookSecret"] = "another-secret"
			}
			if tt.wrongEvent {
				headers.Set("PayHot-Event-Id", "event-2")
			}
			if _, err := verifyPayHotWebhook(config, headers, raw, now); err == nil {
				t.Fatal("accepted invalid event")
			}
		})
	}
	for _, status := range []string{"authorized", "partially_captured", "processing", "failed", "cancelled", "refunded", "manual_reconciliation"} {
		raw := []byte(strings.Replace(body, `"status":"succeeded"`, `"status":"`+status+`"`, 1))
		event, err := verifyPayHotWebhook(cfg, payHotTestHeaders(raw, now.Unix()), raw, now)
		if err != nil || event.Paid || event.Cancelled {
			t.Errorf("non-success %s: %+v %v", status, event, err)
		}
	}
}

func TestPayHotRejectsInvalidCreationAndConfiguration(t *testing.T) {
	gateway := &Gateway{}
	for _, amount := range []float64{0, -1, 0.001, math.NaN(), math.Inf(1), 1e18} {
		if _, err := gateway.createPayHot(context.Background(), map[string]string{"apiKey": "phk_v2_fixture"}, CreatePaymentRequest{PurchaseID: 1, Amount: amount, Currency: "RUB"}); err == nil {
			t.Errorf("accepted amount %v", amount)
		}
	}
	for _, method := range []string{"sbp", "card", "sberpay", "crypto", "applepay", "googlepay"} {
		if err := normalizePayHotConfig(map[string]string{"paymentMethod": method}); err != nil {
			t.Errorf("%s: %v", method, err)
		}
	}
	for _, cfg := range []map[string]string{{"apiKey": "phk_test_v2_fixture"}, {"paymentMethod": "unknown"}} {
		if normalizePayHotConfig(cfg) == nil {
			t.Fatal("accepted invalid configuration")
		}
	}
	if configured(ProviderPayHot, map[string]string{}) {
		t.Fatal("configured without credentials")
	}
}

func TestPayHotRejectsMismatchedPaymentAndUnsafeCheckout(t *testing.T) {
	for _, tc := range []struct{ name, id, order, amount, currency, link string }{
		{"wrong order", payHotTestID, "2", "10000", "RUB", payHotTestURL},
		{"wrong amount", payHotTestID, "1", "9999", "RUB", payHotTestURL},
		{"wrong currency", payHotTestID, "1", "10000", "USD", payHotTestURL},
		{"wrong ID", "invalid", "1", "10000", "RUB", payHotTestURL},
		{"missing token", payHotTestID, "1", "10000", "RUB", "https://app.pay.hot/pay/1"},
		{"empty token", payHotTestID, "1", "10000", "RUB", "https://app.pay.hot/pay/1#token="},
		{"HTTP checkout", payHotTestID, "1", "10000", "RUB", "http://app.pay.hot/pay/1#token=test"},
		{"malformed checkout path", payHotTestID, "1", "10000", "RUB", "https://example.com/pay/1#token=test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"payment": map[string]string{"id": tc.id, "merchant_order_reference": tc.order, "amount_minor": tc.amount, "currency": tc.currency}, "checkout": map[string]string{"url": tc.link}})
			}))
			defer server.Close()
			gateway := &Gateway{httpClient: server.Client()}
			if _, err := gateway.createPayHot(context.Background(), map[string]string{"apiKey": "phk_v2_fixture", "apiUrl": server.URL}, CreatePaymentRequest{PurchaseID: 1, Amount: 100, Currency: "RUB"}); err == nil {
				t.Fatal("accepted invalid payment response")
			}
		})
	}
}

func TestPayHotUsesCheckoutDomainSelectedByProvider(t *testing.T) {
	for _, host := range []string{"app.pay.hot", "checkout.pay.hot", "checkout.example.com"} {
		t.Run(host, func(t *testing.T) {
			link := strings.Replace(payHotTestURL, "app.pay.hot", host, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"payment": map[string]string{"id": payHotTestID, "merchant_order_reference": "1", "amount_minor": "10000", "currency": "RUB"}, "checkout": map[string]string{"url": link}})
			}))
			defer server.Close()
			gateway := &Gateway{httpClient: server.Client()}
			created, err := gateway.createPayHot(context.Background(), map[string]string{"apiKey": "phk_v2_fixture", "apiUrl": server.URL}, CreatePaymentRequest{PurchaseID: 1, Amount: 100, Currency: "RUB"})
			if err != nil || created.URL != link {
				t.Fatalf("valid provider checkout rejected: %v", err)
			}
		})
	}
}

func TestPayHotAPIErrorsAreSafeAndActionable(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		status                int
		body, code, requestID string
	}{
		{"amount limit", 422, `{"error":{"code":"payment_amount_out_of_range","message":"private-fixture-secret","request_id":"req-100","details":{"api_key":"private-fixture-secret"}}}`, "payment_amount_out_of_range", "req-100"},
		{"authentication", 401, `{"error":{"code":"authentication_failed","request_id":"req-101"}}`, "authentication_failed", "req-101"},
		{"permissions", 403, `{"error":{"code":"permission_denied","request_id":"req-102"}}`, "permission_denied", "req-102"},
		{"non-JSON failure", 503, `<html>private-fixture-secret</html>`, "payment_api_error", ""},
		{"malformed diagnostic fields", 422, `{"error":{"code":"private-fixture-secret with spaces","request_id":"private-fixture-secret with spaces"}}`, "payment_api_error", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			gateway := &Gateway{httpClient: server.Client()}
			_, err := gateway.createPayHot(context.Background(), map[string]string{"apiKey": "phk_v2_fixture", "apiUrl": server.URL}, CreatePaymentRequest{PurchaseID: 1, Amount: 100, Currency: "RUB"})
			var paymentErr *PayHotPaymentError
			if !errors.As(err, &paymentErr) || paymentErr.StatusCode != tc.status || paymentErr.Code != tc.code || paymentErr.RequestID != tc.requestID {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Contains(err.Error()+paymentErr.AdminMessage()+paymentErr.PublicMessage(), "private-fixture-secret") {
				t.Fatal("exposed raw provider error")
			}
			if tc.name == "amount limit" && !strings.Contains(paymentErr.PublicMessage(), "сумму") {
				t.Fatal("did not explain amount limit")
			}
		})
	}
}
