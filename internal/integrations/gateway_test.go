package integrations

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestCreateMulenPayAndVerifyCallback(t *testing.T) {
	paymentStatus := 3
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer api-key" {
			t.Errorf("missing MulenPay authentication")
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/payments":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			sign := sha1.Sum([]byte("rub170.007secret"))
			if payload["sign"] != fmt.Sprintf("%x", sign) || payload["uuid"] != "4242" || payload["description"] != "VPN 30 days" {
				t.Errorf("incorrect MulenPay payment: %#v", payload)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"success":true,"id":99,"paymentUrl":"https://mulenpay.ru/pay/99"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v2/payments/99":
			_, _ = fmt.Fprintf(w, `{"success":true,"payment":{"id":99,"uuid":"4242","amount":"170.00","currency":"rub","status":%d}}`, paymentStatus)
		default:
			t.Errorf("unexpected MulenPay request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	gateway := &Gateway{httpClient: server.Client()}
	cfg := map[string]string{"shopId": "7", "apiKey": "api-key", "secretKey": "secret", "apiUrl": server.URL}
	created, err := gateway.createMulenPay(context.Background(), cfg, CreatePaymentRequest{PurchaseID: 4242, Amount: 170, Currency: "RUB", Description: "VPN 30 days"})
	if err != nil || created.ExternalID != "99" {
		t.Fatalf("create MulenPay payment: %+v, %v", created, err)
	}
	event, err := gateway.parseMulenPayWebhook(context.Background(), cfg, []byte(`{"id":99,"uuid":"4242","payment_status":"success"}`))
	if err != nil || event.PurchaseID != 4242 || event.ExternalID != "99" || !event.Paid {
		t.Fatalf("verify MulenPay callback: %+v, %v", event, err)
	}
	if _, err := gateway.parseMulenPayWebhook(context.Background(), cfg, []byte(`{"id":99,"uuid":"4243","payment_status":"success"}`)); err == nil {
		t.Fatal("accepted mismatched MulenPay callback")
	}
	paymentStatus = 1
	event, err = gateway.parseMulenPayWebhook(context.Background(), cfg, []byte(`{"id":99,"uuid":"4242","payment_status":"success"}`))
	if err != nil || event.Paid {
		t.Fatalf("trusted an unverified paid status instead of the API: %+v, %v", event, err)
	}
}

func TestCreateAnoreAndVerifyWebhook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/payments" || r.Header.Get("Authorization") != "Bearer an_test_key" {
			t.Fatalf("unexpected anore request: %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["shopId"] != float64(7) || payload["orderId"] != "4242" || payload["description"] != "VPN 30 days" || payload["callbackUrl"] != "https://bot.example/api/payments/webhook/anore/token" {
			t.Fatalf("unexpected anore payload: %#v", payload)
		}
		_, _ = w.Write([]byte(`{"success":true,"id":"anore-payment","paymentUrl":"https://pay.anore.cc/test"}`))
	}))
	defer server.Close()
	settings := &Service{baseURL: "https://bot.example", records: map[string]record{ProviderAnore: {WebhookToken: "token"}}}
	gateway := &Gateway{settings: settings, httpClient: server.Client()}
	created, err := gateway.createAnore(context.Background(), map[string]string{"apiKey": "an_test_key", "shopId": "7", "apiUrl": server.URL}, CreatePaymentRequest{PurchaseID: 4242, Amount: 170, Currency: "RUB", Description: "VPN 30 days"})
	if err != nil || created.ExternalID != "anore-payment" {
		t.Fatalf("create anore payment: %+v, %v", created, err)
	}
	raw := []byte(`{"event":"payment.succeeded","id":"anore-payment","orderId":"4242","amount":170,"currency":"rub","status":"paid"}`)
	headers := make(http.Header)
	headers.Set("Anore-Signature", hmacHex(sha256.New, []byte("webhook-secret"), raw))
	payment, err := parseAnoreWebhook(map[string]string{"webhookSecret": "webhook-secret"}, headers, raw)
	if err != nil || payment.PurchaseID != 4242 || payment.ExternalID != created.ExternalID || !payment.Paid || payment.Amount != 170 {
		t.Fatalf("parse anore webhook: %+v, %v", payment, err)
	}
	headers.Set("Anore-Signature", "invalid")
	if _, err := parseAnoreWebhook(map[string]string{"webhookSecret": "webhook-secret"}, headers, raw); err == nil {
		t.Fatal("expected invalid anore signature error")
	}
}

func TestCreateCloudPaymentsAndVerifyWebhook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orders/create" || r.Header.Get("Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("pk:test-secret")) {
			t.Fatalf("unexpected CloudPayments request: %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["InvoiceId"] != "4242" || payload["Description"] != "VPN 30 days" || payload["SendEmail"] != false {
			t.Fatalf("unexpected CloudPayments payload: %#v", payload)
		}
		_, _ = w.Write([]byte(`{"Success":true,"Model":{"Id":"cloud-order","Url":"https://orders.cloudpayments.ru/d/cloud-order"}}`))
	}))
	defer server.Close()
	gateway := &Gateway{httpClient: server.Client()}
	created, err := gateway.createCloudPayments(context.Background(), map[string]string{"publicId": "pk", "apiSecret": "test-secret", "apiUrl": server.URL}, CreatePaymentRequest{PurchaseID: 4242, Amount: 170, Currency: "RUB", Description: "VPN 30 days"})
	if err != nil || created.ExternalID != "cloud-order" {
		t.Fatalf("create CloudPayments order: %+v, %v", created, err)
	}
	form := url.Values{"InvoiceId": {"4242"}, "TransactionId": {"987"}, "Amount": {"170.00"}, "Currency": {"RUB"}, "Status": {"Completed"}, "OperationType": {"Payment"}}
	raw := []byte(form.Encode())
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write(raw)
	headers := make(http.Header)
	headers.Set("Content-HMAC", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	payment, err := parseCloudPaymentsWebhook(map[string]string{"apiSecret": "test-secret"}, headers, raw, form)
	if err != nil || payment.PurchaseID != 4242 || !payment.Paid || payment.Amount != 170 {
		t.Fatalf("parse CloudPayments webhook: %+v, %v", payment, err)
	}
	headers.Set("Content-HMAC", "invalid")
	if _, err := parseCloudPaymentsWebhook(map[string]string{"apiSecret": "test-secret"}, headers, raw, form); err == nil {
		t.Fatal("expected invalid CloudPayments signature error")
	}
}

func TestParseLavaWebhookAuthorizationSignature(t *testing.T) {
	raw := []byte(`{
		"status":"success",
		"amount":"170.00",
		"order_id":4242,
		"invoice_id":"lava-invoice-1"
	}`)
	canonical, err := sortedJSONObject(raw)
	if err != nil {
		t.Fatalf("canonicalize webhook: %v", err)
	}

	config := map[string]string{"additionalKey": "webhook-secret"}
	headers := make(http.Header)
	headers.Set("Authorization", hmacHex(sha256.New, []byte(config["additionalKey"]), canonical))

	payment, err := parseLavaWebhook(config, headers, raw)
	if err != nil {
		t.Fatalf("parse webhook: %v", err)
	}
	if payment.PurchaseID != 4242 {
		t.Fatalf("unexpected purchase id: %d", payment.PurchaseID)
	}
	if payment.ExternalID != "lava-invoice-1" {
		t.Fatalf("unexpected external id: %q", payment.ExternalID)
	}
	if payment.Amount != 170 || payment.Currency != "RUB" || !payment.Paid || payment.Cancelled {
		t.Fatalf("unexpected payment: %+v", payment)
	}
}

func TestParseLavaWebhookRejectsInvalidSignature(t *testing.T) {
	raw := []byte(`{"invoice_id":"lava-invoice-1","order_id":"4242","status":"success","amount":170}`)
	headers := make(http.Header)
	headers.Set("Authorization", "invalid")

	if _, err := parseLavaWebhook(map[string]string{"additionalKey": "webhook-secret"}, headers, raw); err == nil {
		t.Fatal("expected invalid signature error")
	}
}

func TestParsePallyWebhookSignatureAndPaymentData(t *testing.T) {
	config := map[string]string{"apiToken": "pally-secret"}
	amount, invoiceID := "170.00", "4242"
	signature := fmt.Sprintf("%X", md5.Sum([]byte(amount+":"+invoiceID+":"+config["apiToken"])))
	form := url.Values{
		"InvId":          {invoiceID},
		"OutSum":         {amount},
		"CurrencyIn":     {"RUB"},
		"Status":         {"SUCCESS"},
		"SignatureValue": {signature},
	}

	payment, err := parsePallyWebhook(config, form)
	if err != nil {
		t.Fatalf("parse webhook: %v", err)
	}
	if payment.PurchaseID != 4242 || payment.Amount != 170 || payment.Currency != "RUB" || !payment.Paid || payment.Cancelled {
		t.Fatalf("unexpected payment: %+v", payment)
	}
}

func TestParsePallyWebhookRejectsInvalidSignature(t *testing.T) {
	form := url.Values{"InvId": {"4242"}, "OutSum": {"170.00"}, "CurrencyIn": {"RUB"}, "Status": {"SUCCESS"}, "SignatureValue": {"invalid"}}
	if _, err := parsePallyWebhook(map[string]string{"apiToken": "pally-secret"}, form); err == nil {
		t.Fatal("expected invalid signature error")
	}
}

func TestCreateRollyPay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/payments" || r.Header.Get("X-API-Key") != "rolly-key" || r.Header.Get("X-Nonce") == "" {
			t.Fatalf("unexpected RollyPay request: path=%q headers=%v", r.URL.Path, r.Header)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["amount"] != "170.00" || payload["payment_currency"] != "RUB" || payload["order_id"] != "4242" {
			t.Fatalf("unexpected RollyPay payload: %#v", payload)
		}
		_, _ = w.Write([]byte(`{"payment_id":"pay-1","pay_url":"https://pay.example/pay-1"}`))
	}))
	defer server.Close()

	gateway := &Gateway{httpClient: server.Client()}
	payment, err := gateway.createRollyPay(context.Background(), map[string]string{"apiKey": "rolly-key", "apiUrl": server.URL}, CreatePaymentRequest{PurchaseID: 4242, Amount: 170, Currency: "RUB", Description: "test", ReturnURL: "https://bot.example/return"})
	if err != nil {
		t.Fatalf("create RollyPay payment: %v", err)
	}
	if payment.ExternalID != "pay-1" || payment.URL != "https://pay.example/pay-1" {
		t.Fatalf("unexpected payment: %+v", payment)
	}
}

func TestParseRollyPayWebhookSignatureAndPaymentData(t *testing.T) {
	config := map[string]string{"signingSecret": "rolly-secret"}
	timestamp := "1760000000"
	raw := []byte(`{"payment_id":"pay-1","order_id":"4242","status":"paid","amount":"170.00","currency":"RUB"}`)
	headers := make(http.Header)
	headers.Set("X-Timestamp", timestamp)
	headers.Set("X-Signature", hmacHex(sha256.New, []byte(config["signingSecret"]), append([]byte(timestamp+"."), raw...)))

	payment, err := parseRollyPayWebhook(config, headers, raw)
	if err != nil {
		t.Fatalf("parse RollyPay webhook: %v", err)
	}
	if payment.PurchaseID != 4242 || payment.ExternalID != "pay-1" || payment.Amount != 170 || payment.Currency != "RUB" || !payment.Paid || payment.Cancelled {
		t.Fatalf("unexpected payment: %+v", payment)
	}
	headers.Set("X-Signature", "invalid")
	if _, err := parseRollyPayWebhook(config, headers, raw); err == nil {
		t.Fatal("expected invalid RollyPay signature error")
	}
}

func TestCreateCisPay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/payments" || r.Header.Get("X-Shop-ID") != "shop-id" || r.Header.Get("X-API-Key") != "cis-key" {
			t.Fatalf("unexpected cisPay request: path=%q headers=%v", r.URL.Path, r.Header)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["amount"] != float64(17025) || payload["payment_method"] != "SBP" || payload["customer_id"] != "77" {
			t.Fatalf("unexpected cisPay payload: %#v", payload)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"cis-payment-1","payment_url":"https://pay.example/cis-payment-1"}`))
	}))
	defer server.Close()

	gateway := &Gateway{httpClient: server.Client()}
	payment, err := gateway.createCisPay(context.Background(), map[string]string{"shopId": "shop-id", "apiKey": "cis-key", "paymentMethod": "SBP", "apiUrl": server.URL}, CreatePaymentRequest{PurchaseID: 4242, CustomerID: 77, Amount: 170.25, Currency: "RUB", Description: "test", ReturnURL: "https://bot.example/return"})
	if err != nil {
		t.Fatalf("create cisPay payment: %v", err)
	}
	if payment.ExternalID != "cis-payment-1" || payment.URL != "https://pay.example/cis-payment-1" {
		t.Fatalf("unexpected payment: %+v", payment)
	}
}

func TestParseCisPayWebhookSignatureAndPaymentData(t *testing.T) {
	config := map[string]string{"apiKey": "cis-secret"}
	raw := []byte(`{"id":"cis-payment-1","order_id":"4242","status":"PAID","amount":17025,"currency":"RUB"}`)
	headers := make(http.Header)
	headers.Set("X-Signature", hmacHex(sha256.New, []byte(config["apiKey"]), raw))

	payment, err := parseCisPayWebhook(config, headers, raw)
	if err != nil {
		t.Fatalf("parse cisPay webhook: %v", err)
	}
	if payment.PurchaseID != 4242 || payment.ExternalID != "cis-payment-1" || payment.Amount != 170.25 || payment.Currency != "RUB" || !payment.Paid || payment.Cancelled {
		t.Fatalf("unexpected payment: %+v", payment)
	}
	headers.Set("X-Signature", "invalid")
	if _, err := parseCisPayWebhook(config, headers, raw); err == nil {
		t.Fatal("expected invalid cisPay signature error")
	}
}
