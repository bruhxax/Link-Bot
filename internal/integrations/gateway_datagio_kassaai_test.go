package integrations

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDatagioPaymentProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/merchant/v1/payment-links" || r.Header.Get("X-API-Key") != "pk_test" || r.Header.Get("X-API-Secret") != "sk_test" {
			t.Errorf("incorrect Datagio request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["shop_id"] != float64(7) || body["amount"] != "170.50" || body["order_id"] != "4242" || body["description"] != "VPN 30 days" {
			t.Errorf("incorrect Datagio invoice: %#v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"link-123","checkout_url":"https://datagio.finance/pay/link-123"}`))
	}))
	defer server.Close()
	cfg := map[string]string{"apiUrl": server.URL, "shopId": "7", "apiKey": "pk_test", "apiSecret": "sk_test", "webhookSecret": "callback-secret"}
	service := &Service{records: map[string]record{ProviderDatagio: {Enabled: true, Config: cfg}}}
	gateway := &Gateway{settings: service, httpClient: server.Client()}
	created, err := gateway.Create(context.Background(), CreatePaymentRequest{Provider: ProviderDatagio, PurchaseID: 4242, Amount: 170.5, Description: "VPN 30 days"})
	if err != nil || created.ExternalID != "link-123" || created.URL != "https://datagio.finance/pay/link-123" {
		t.Fatalf("create Datagio: %+v, %v", created, err)
	}

	for _, header := range []string{"X-Webhook-Signature", "Datagio-Signature"} {
		for _, status := range []string{"paid", "pending"} {
			raw := []byte(fmt.Sprintf(`{"event":"payment.succeeded","data":{"payment_link_id":"link-123","id":"link-123","shop_id":7,"order_id":"4242","amount":"170.50","merchant_credit":"160.00","status":%q}}`, status))
			headers := make(http.Header)
			headers.Set(header, testDatagioSignature(raw))
			event, err := gateway.HandleWebhook(context.Background(), ProviderDatagio, headers, raw, nil)
			if err != nil || event.PurchaseID != 4242 || event.ExternalID != created.ExternalID || event.Amount != 170.5 || event.Currency != "RUB" || event.Paid != (status == "paid") {
				t.Fatalf("Datagio webhook: %+v, %v", event, err)
			}
			if _, err := parseDatagioWebhook(cfg, headers, append(raw, ' ')); err == nil {
				t.Fatal("accepted a tampered Datagio body")
			}
		}
	}
	if _, err := gateway.Create(context.Background(), CreatePaymentRequest{Provider: ProviderDatagio, Currency: "USD"}); err == nil {
		t.Fatal("accepted a foreign currency")
	}
}

func testDatagioSignature(raw []byte) string {
	mac := hmac.New(sha256.New, []byte("callback-secret"))
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestDatagioRejectsInvalidSignedPayments(t *testing.T) {
	cfg := map[string]string{"shopId": "7", "webhookSecret": "callback-secret"}
	for _, data := range []string{
		`"shop_id":8,"amount":"170.50","id":"link-123"`,
		`"shop_id":7,"amount":"0","id":"link-123"`,
		`"shop_id":7,"amount":"-10","id":"link-123"`,
		`"shop_id":7,"amount":"NaN","id":"link-123"`,
		`"shop_id":7,"amount":"170.50"`,
		`"shop_id":7,"amount":"170.50","id":"link-123","payment_link_id":"other"`,
		`"shop_id":7,"amount":"170.50","id":"link-123","order_id":"-1"`,
	} {
		raw := []byte(`{"event":"payment.succeeded","data":{` + data + `}}`)
		headers := make(http.Header)
		headers.Set("X-Webhook-Signature", testDatagioSignature(raw))
		if _, err := parseDatagioWebhook(cfg, headers, raw); err == nil {
			t.Errorf("accepted invalid signed payload: %s", raw)
		}
	}
	if _, err := parseDatagioWebhook(cfg, nil, []byte(`{}`)); err == nil {
		t.Fatal("accepted unsigned callback")
	}
}

func TestKassaAIPaymentProtocol(t *testing.T) {
	var previousNonce json.Number
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/orders/create" {
			t.Errorf("incorrect Kassa AI endpoint: %s", r.URL.Path)
		}
		var body map[string]any
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&body); err != nil {
			t.Error(err)
		}
		nonce := body["nonce"].(json.Number)
		if previousNonce != "" && nonce.String() <= previousNonce.String() {
			t.Error("nonce did not increase")
		}
		previousNonce = nonce
		if fmt.Sprint(body["amount"]) != "170.5" || fmt.Sprint(body["shopId"]) != "7" || body["paymentId"] != "4242" || body["email"] != "88@telegram.org" || body["notification_url"] != "https://vpn.example/api/payments/webhook/kassaai/hook-token" {
			t.Errorf("incorrect Kassa AI invoice: %#v", body)
		}
		// Independently construct the documented alphabetical value sequence.
		canonical := strings.Join([]string{"170.5", "RUB", "88@telegram.org", "https://vpn.example/return", fmt.Sprint(body["i"]), fmt.Sprint(body["ip"]), nonce.String(), "https://vpn.example/api/payments/webhook/kassaai/hook-token", "4242", "7", "https://vpn.example/return"}, "|")
		mac := hmac.New(sha256.New, []byte("api-secret"))
		_, _ = mac.Write([]byte(canonical))
		if body["signature"] != hex.EncodeToString(mac.Sum(nil)) {
			t.Error("incorrect Kassa AI request signature")
		}
		_, _ = w.Write([]byte(`{"type":"success","orderId":98765,"location":"https://pay.fk.money/form/98765"}`))
	}))
	defer server.Close()
	cfg := map[string]string{"apiUrl": server.URL, "shopId": "7", "apiKey": "api-secret", "secretWord2": "callback-secret", "paymentSystemId": "44", "clientIP": "203.0.113.8"}
	service := &Service{baseURL: "https://vpn.example", records: map[string]record{ProviderKassaAI: {Enabled: true, Config: cfg, WebhookToken: "hook-token"}}}
	gateway := &Gateway{settings: service, httpClient: server.Client()}
	for _, method := range []string{"44", "36", "43"} {
		cfg["paymentSystemId"] = method
		created, err := gateway.Create(context.Background(), CreatePaymentRequest{Provider: ProviderKassaAI, PurchaseID: 4242, Amount: 170.5, CustomerID: 88, ReturnURL: "https://vpn.example/return"})
		if err != nil || created.ExternalID != "98765" {
			t.Fatalf("create Kassa AI %s: %+v, %v", method, created, err)
		}
	}
	form := url.Values{"MERCHANT_ID": {"7"}, "AMOUNT": {"170.50"}, "MERCHANT_ORDER_ID": {"4242"}, "intid": {"98765"}}
	sign := md5.Sum([]byte("7:170.50:callback-secret:4242"))
	form.Set("SIGN", fmt.Sprintf("%x", sign))
	event, err := gateway.HandleWebhook(context.Background(), ProviderKassaAI, nil, nil, form)
	if err != nil || !event.Paid || event.Amount != 170.5 || event.ExternalID != "98765" || event.PurchaseID != 4242 {
		t.Fatalf("Kassa AI webhook: %+v, %v", event, err)
	}
	form.Set("AMOUNT", "1")
	if _, err := parseKassaAIWebhook(cfg, form); err == nil {
		t.Fatal("accepted a tampered Kassa AI amount")
	}
	for _, amount := range []string{"", "invalid", "NaN", "+Inf", "0", "-5"} {
		form.Set("AMOUNT", amount)
		sign := md5.Sum([]byte("7:" + amount + ":callback-secret:4242"))
		form.Set("SIGN", fmt.Sprintf("%x", sign))
		if _, err := parseKassaAIWebhook(cfg, form); err == nil {
			t.Errorf("accepted signed invalid amount %q", amount)
		}
	}
}

func TestNewProvidersRequireCredentialsAndAcceptOnlyHTTPSCheckout(t *testing.T) {
	for _, provider := range []string{ProviderDatagio, ProviderKassaAI} {
		if configured(provider, map[string]string{}) {
			t.Errorf("%s configured without credentials", provider)
		}
	}
	for _, value := range []string{"", "javascript:alert(1)", "http://pay.example", "https://user:password@pay.example"} {
		if validCheckoutURL(value) {
			t.Errorf("accepted unsafe checkout %q", value)
		}
	}
}
