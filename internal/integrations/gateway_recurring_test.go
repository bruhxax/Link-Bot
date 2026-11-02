package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCisPayRecurringLifecycle(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Header.Get("X-Shop-ID") != "shop" || r.Header.Get("X-API-Key") != "key" {
			t.Fatal("cisPay credentials missing")
		}
		switch r.URL.Path {
		case "/subscriptions":
			var body struct {
				Amount        int    `json:"amount"`
				IntervalDays  int    `json:"interval_days"`
				PaymentMethod string `json:"payment_method"`
				CustomerID    string `json:"customer_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Amount != 19900 || body.IntervalDays != 60 || body.PaymentMethod != "SBP" || body.CustomerID != "77" {
				t.Fatalf("wrong cisPay subscription: %+v", body)
			}
			_, _ = w.Write([]byte(`{"id":"sub-1","payment_url":"https://cis.example/pay"}`))
		case "/subscriptions/sub-1/cancel":
			_, _ = w.Write([]byte(`{"status":"CANCELLED"}`))
		default:
			t.Fatalf("unexpected cisPay path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	g := &Gateway{httpClient: server.Client()}
	cfg := map[string]string{"apiUrl": server.URL, "apiKey": "key", "shopId": "shop", "paymentMethod": "SBP"}
	result, err := g.createCisPay(context.Background(), cfg, CreatePaymentRequest{PurchaseID: 42, CustomerID: 77, Amount: 199, Currency: "RUB", Description: "Two months", ReturnURL: "https://bot.example/return", RecurringMonths: 2, RecurringDaysInMonth: 30})
	if err != nil || result.RecurringReference != "sub-1" {
		t.Fatalf("create cisPay subscription: %+v %v", result, err)
	}
	if _, err := g.createCisPay(context.Background(), map[string]string{"paymentMethod": "CARD"}, CreatePaymentRequest{RecurringMonths: 1, RecurringDaysInMonth: 30, Amount: 199}); err == nil {
		t.Fatal("card subscription must be rejected")
	}
	raw := []byte(`{"id":"pay-2","order_id":"","status":"PAID","amount":19900,"currency":"RUB","subscription_id":"sub-1"}`)
	headers := make(http.Header)
	headers.Set("X-Signature", hmacHex(sha256.New, []byte("key"), raw))
	event, err := parseCisPayWebhook(cfg, headers, raw)
	if err != nil || !event.Paid || event.SubscriptionID != "sub-1" || event.ExternalID != "pay-2" || event.Amount != 199 {
		t.Fatalf("parse cisPay renewal: %+v %v", event, err)
	}
	g.settings = &Service{}
	g.settings.records = map[string]record{ProviderCisPay: {Enabled: true, Config: cfg}}
	if err := g.StopRecurringSubscription(context.Background(), ProviderCisPay, "sub-1"); err != nil {
		t.Fatalf("stop cisPay subscription: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("cisPay requests: %v", requests)
	}
}

func TestLavaRecurringCheckoutAndStatus(t *testing.T) {
	const shop = "shop-1"
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["shopId"] != shop || r.Header.Get("Signature") == "" {
			t.Fatalf("invalid LAVA request: %s %#v", r.URL.Path, body)
		}
		switch r.URL.Path {
		case "/business/recurrent/product/list":
			_, _ = w.Write([]byte(`{"status_check":true,"data":[{"id":"product-1","price":299,"period":"one_month","periodDays":30,"freeDays":0,"shopId":"shop-1","isActive":true}]}`))
		case "/business/recurrent/consumer/create":
			if body["consumerId"] != "linkbot-42" {
				t.Fatal("wrong LAVA consumer")
			}
			_, _ = w.Write([]byte(`{"status_check":true,"data":{"consumerId":"linkbot-42"}}`))
		case "/business/recurrent/subscription/subscribe":
			if body["productId"] != "product-1" || body["orderId"] != "42" {
				t.Fatal("wrong LAVA product or order")
			}
			_, _ = w.Write([]byte(`{"status_check":true,"data":{"subscriptionId":"sub-1","url":"https://lava.example/pay","amount":299}}`))
		case "/business/recurrent/subscription/status":
			_, _ = w.Write([]byte(`{"status_check":true,"data":{"subscriptionId":"sub-1","orderId":"42","consumerId":"linkbot-42","shopId":"shop-1","status":"activated","amount":299,"last_pay_time":"2026-11-02T10:00:00Z"}}`))
		case "/business/recurrent/subscription/unsubscribe":
			_, _ = w.Write([]byte(`{"status_check":true,"data":{"unsubscribed":true}}`))
		default:
			t.Fatalf("unexpected LAVA request: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	cfg := map[string]string{"shopId": shop, "secretKey": "secret", "additionalKey": "webhook-secret", "apiUrl": server.URL}
	g := &Gateway{httpClient: server.Client(), settings: &Service{records: map[string]record{ProviderLava: {Enabled: true, Config: cfg}}}}
	created, err := g.createLavaSubscription(context.Background(), cfg, CreatePaymentRequest{PurchaseID: 42, CustomerID: 7, Email: "person@example.com", Amount: 299, RecurringMonths: 1, RecurringDaysInMonth: 30})
	if err != nil || created.RecurringReference != "sub-1" {
		t.Fatalf("create LAVA subscription: %+v %v", created, err)
	}
	status, err := g.LavaRecurringStatus(context.Background(), "sub-1", 42)
	if err != nil || !status.Paid || status.ExternalID != "sub-1:2026-11-02T10:00:00Z" {
		t.Fatalf("LAVA charge status: %+v %v", status, err)
	}
	if err := g.StopRecurringSubscription(context.Background(), ProviderLava, "sub-1"); err != nil {
		t.Fatalf("cancel LAVA subscription: %v", err)
	}
	if len(paths) != 5 {
		t.Fatalf("LAVA API calls: %v", paths)
	}
}

func TestCreateWataRecurringIncludesConsentTerms(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/links" || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("unexpected WATA request: %s", r.URL.Path)
		}
		var body struct {
			Email        string `json:"email"`
			Phone        string `json:"phone"`
			Subscription struct {
				Period     int    `json:"period"`
				Interval   string `json:"interval"`
				MaxPeriods int    `json:"maxPeriods"`
			} `json:"subscription"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Email != "person@example.com" || body.Phone != "79991234567" || body.Subscription.Period != 3 || body.Subscription.Interval != "Month" || body.Subscription.MaxPeriods <= 0 {
			t.Fatalf("invalid WATA subscription request: %+v", body)
		}
		_, _ = w.Write([]byte(`{"id":"link-id","url":"https://wata.example/link-id"}`))
	}))
	defer server.Close()
	g := &Gateway{httpClient: server.Client()}
	result, err := g.createWata(context.Background(), map[string]string{"apiUrl": server.URL, "accessToken": "token"}, CreatePaymentRequest{PurchaseID: 42, Amount: 300, Currency: "RUB", Email: "person@example.com", Phone: "+79991234567", RecurringMonths: 3})
	if err != nil || result.URL == "" {
		t.Fatalf("create WATA subscription: %+v %v", result, err)
	}
}

func TestCreatePlategaRecurringRequiresConfirmation(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.Header.Get("X-MerchantId") != "merchant" || r.Header.Get("X-Secret") != "secret" {
			t.Fatal("Platega credentials missing")
		}
		if strings.HasSuffix(r.URL.Path, "/transaction/process") {
			var body struct {
				PaymentMethod  int `json:"paymentMethod"`
				PaymentDetails struct {
					Interval      int `json:"interval"`
					IntervalCount int `json:"intervalCount"`
				} `json:"paymentDetails"`
				Metadata map[string]string `json:"metadata"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.PaymentMethod != 6 || body.PaymentDetails.Interval != 3 || body.PaymentDetails.IntervalCount != 1 || body.Metadata["user_email"] != "123@telegram.org" {
				t.Fatalf("wrong Platega terms: %+v", body)
			}
			_, _ = w.Write([]byte(`{"transactionId":"sub-1"}`))
			return
		}
		if r.URL.Path != "/v2/subscription/sub-1/start" {
			t.Fatalf("unexpected Platega path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"confirmationUrl":"https://platega.example/confirm"}`))
	}))
	defer server.Close()
	g := &Gateway{httpClient: server.Client()}
	result, err := g.createPlatega(context.Background(), map[string]string{"apiUrl": server.URL, "merchantId": "merchant", "secretKey": "secret"}, CreatePaymentRequest{TelegramID: 123, Amount: 200, Currency: "RUB", RecurringMonths: 1})
	if err != nil || result.RecurringReference != "sub-1" || result.URL != "https://platega.example/confirm" || len(requests) != 2 {
		t.Fatalf("Platega subscription: %+v %v", result, err)
	}
}

func TestPlategaSubscriptionCancellationCallback(t *testing.T) {
	config := map[string]string{"merchantId": "merchant", "secretKey": "secret"}
	headers := make(http.Header)
	headers.Set("X-MerchantId", "merchant")
	headers.Set("X-Secret", "secret")
	event, err := parsePlategaWebhook(config, headers, []byte(`{"id":"sub-1","subscriptionId":"sub-1","status":"SUBSCRIPTION_CANCELLED","amount":200,"currency":"RUB"}`))
	if err != nil || event.SubscriptionID != "sub-1" || !event.SubscriptionStopped || event.Paid {
		t.Fatalf("Platega subscription cancellation: %+v %v", event, err)
	}
}

func TestCreateRollyPayRecurringMatchesConfiguredPlan(t *testing.T) {
	const terminalID = "d290f1ee-6c54-4b01-90e6-d701748f0851"
	created := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "key" || r.Header.Get("X-Nonce") == "" {
			t.Fatal("RollyPay authentication missing")
		}
		if r.Method == http.MethodGet {
			if r.URL.Query().Get("terminal_id") != terminalID {
				t.Fatal("wrong terminal")
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"plan-1","interval":"month","merchant_amount_rub":"150.00","payer_amount_rub":"150.00"}]}`))
			return
		}
		created = true
		if r.Header.Get("Idempotency-Key") != "linkbot-recurring-42" {
			t.Fatal("missing recurring idempotency key")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["plan_id"] != "plan-1" || body["payer_id"] != "tg_123" {
			t.Fatalf("wrong RollyPay plan: %#v", body)
		}
		_, _ = w.Write([]byte(`{"id":"sub-1","pay_url":"https://rollypay.example/confirm"}`))
	}))
	defer server.Close()
	g := &Gateway{httpClient: server.Client()}
	cfg := map[string]string{"apiUrl": server.URL, "apiKey": "key", "terminalId": terminalID}
	result, err := g.createRollyPay(context.Background(), cfg, CreatePaymentRequest{PurchaseID: 42, TelegramID: 123, Amount: 150, Currency: "RUB", RecurringMonths: 1})
	if err != nil || !created || result.RecurringReference != "sub-1" {
		t.Fatalf("RollyPay subscription: %+v %v", result, err)
	}
	created = false
	if _, err := g.createRollyPay(context.Background(), cfg, CreatePaymentRequest{PurchaseID: 43, TelegramID: 123, Amount: 200, Currency: "RUB", RecurringMonths: 1}); err == nil || created {
		t.Fatal("mismatched price must not create a subscription")
	}
}
