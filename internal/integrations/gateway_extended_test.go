package integrations

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShopInvoiceProviders(t *testing.T) {
	for _, provider := range []string{ProviderAuraPay, ProviderParityPay} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path, header := "/invoice/create", "X-ApiKey"
				if provider == ProviderParityPay {
					path, header = "/v2/invoice/create", "X-SecretKey"
				}
				if r.Method != http.MethodPost || r.URL.Path != path || r.Header.Get(header) != "api-secret" || r.Header.Get("X-ShopId") != "shop-1" {
					t.Errorf("invalid %s request: %s %s", provider, r.Method, r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["order_id"] != "4242" || body["amount"] != float64(170) || body["comment"] != "VPN 30 days" || body["callback_url"] != "https://bot.example/api/payments/webhook/"+provider+"/token" {
					t.Errorf("invalid payment body: %#v", body)
				}
				_, _ = w.Write([]byte(`{"id":"invoice-1","link":"https://pay.example/invoice-1","payment_data":{"url":"https://pay.example/invoice-1"}}`))
			}))
			defer server.Close()
			settings := &Service{baseURL: "https://bot.example", records: map[string]record{provider: {WebhookToken: "token"}}}
			gateway := &Gateway{settings: settings, httpClient: server.Client()}
			cfg := map[string]string{"apiUrl": server.URL, "apiKey": "api-secret", "secretKey": "api-secret", "shopId": "shop-1", "webhookSecret": "callback-secret"}
			input := CreatePaymentRequest{PurchaseID: 4242, Amount: 170, Currency: "RUB", Description: "VPN 30 days"}
			var created CreatedPayment
			var err error
			if provider == ProviderAuraPay {
				created, err = gateway.createAuraPay(context.Background(), cfg, input)
			} else {
				created, err = gateway.createParityPay(context.Background(), cfg, input)
			}
			if err != nil || created.ExternalID != "invoice-1" || created.URL == "" {
				t.Fatalf("create payment: %+v, %v", created, err)
			}
			// The documented signature includes net credited money and null fields.
			// Settlement must still compare the gross amount to the purchase.
			raw := []byte(`{"status":"PAID","shop_id":"shop-1","order_id":"4242","id":"invoice-1","credited":160.5,"amount":"170.00","callback":null}`)
			headers := make(http.Header)
			headers.Set("X-Signature", hmacHex(sha256.New, []byte("callback-secret"), []byte("170.00160.5invoice-14242shop-1PAID")))
			event, err := parseShopInvoiceWebhook(cfg, headers, raw)
			if err != nil || !event.Paid || event.PurchaseID != 4242 || event.Amount != 170 || event.ExternalID != created.ExternalID {
				t.Fatalf("verify webhook: %+v, %v", event, err)
			}
			if _, err := parseShopInvoiceWebhook(map[string]string{"shopId": "another-shop", "webhookSecret": "callback-secret"}, headers, raw); err == nil {
				t.Fatal("accepted a payment for another shop")
			}
			if _, err := parseShopInvoiceWebhook(cfg, headers, []byte(`{"amount":"171.00"}`)); err == nil {
				t.Fatal("accepted a modified webhook")
			}
			raw = []byte(`{"id":"invoice-1","order_id":"4242","shop_id":"shop-1","amount":"170.00","status":"PENDING"}`)
			headers.Set("X-Signature", hmacHex(sha256.New, []byte("callback-secret"), []byte("170.00invoice-14242shop-1PENDING")))
			event, err = parseShopInvoiceWebhook(cfg, headers, raw)
			if err != nil || event.Paid || event.Cancelled {
				t.Fatalf("pending payment was completed: %+v, %v", event, err)
			}
		})
	}
}

func TestAntiloPaySigningAndGrossAmount(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/payment/create" || r.Header.Get("X-Apay-Secret-Id") != "merchant" || r.Header.Get("X-Apay-Sign-Version") != "1" {
			t.Error("incorrect AntiloPay authentication")
		}
		raw, _ := io.ReadAll(r.Body)
		digest := sha256.Sum256(raw)
		signature, _ := base64.StdEncoding.DecodeString(r.Header.Get("X-Apay-Sign"))
		if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
			t.Errorf("invalid outgoing signature: %v", err)
		}
		var payload struct {
			OrderID     string `json:"order_id"`
			Description string `json:"description"`
			Customer    struct {
				Email string `json:"email"`
				IP    string `json:"ip"`
			} `json:"customer"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Error(err)
		}
		if payload.OrderID != "4242" || payload.Description != "VPN 30 days" || payload.Customer.Email != "buyer@example.com" || payload.Customer.IP != "1.1.1.1" {
			t.Errorf("incorrect buyer or description: %+v", payload)
		}
		_, _ = w.Write([]byte(`{"code":0,"payment_id":"antilo-1","payment_url":"https://pay.example/antilo-1"}`))
	}))
	defer server.Close()
	cfg := map[string]string{"apiUrl": server.URL, "secretId": "merchant", "projectId": "project", "privateKey": base64.StdEncoding.EncodeToString(privateDER), "publicKey": base64.StdEncoding.EncodeToString(publicDER)}
	gateway := &Gateway{httpClient: server.Client()}
	input := CreatePaymentRequest{PurchaseID: 4242, Amount: 170, Currency: "RUB", Description: "VPN 30 days", Email: "buyer@example.com", ClientIP: "1.1.1.1"}
	created, err := gateway.createAntiloPay(context.Background(), cfg, input)
	if err != nil || created.ExternalID != "antilo-1" {
		t.Fatalf("create AntiloPay: %+v, %v", created, err)
	}
	input.Email = ""
	if _, err := gateway.createAntiloPay(context.Background(), cfg, input); err == nil {
		t.Fatal("created AntiloPay payment without a buyer email")
	}
	raw := []byte(`{"type":"payment","payment_id":"antilo-1","order_id":"4242","original_amount":170,"amount":160,"currency":"rub","status":"SUCCESS"}`)
	digest := sha256.Sum256(raw)
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	headers := make(http.Header)
	headers.Set("X-Apay-Callback-Version", "1")
	headers.Set("X-Apay-Callback", base64.StdEncoding.EncodeToString(signature))
	event, err := parseAntiloPayWebhook(cfg, headers, raw)
	if err != nil || !event.Paid || event.Amount != 170 || event.PurchaseID != 4242 {
		t.Fatalf("callback gross amount: %+v, %v", event, err)
	}
	if _, err := parseAntiloPayWebhook(cfg, headers, append(raw, ' ')); err == nil {
		t.Fatal("accepted a modified RSA-signed body")
	}
}

func TestTributeShopOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shop/orders" || r.Header.Get("Api-Key") != "api-key" {
			t.Error("invalid Tribute authentication")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["amount"] != float64(17050) || payload["shopId"] != float64(7) || payload["period"] != "onetime" || payload["description"] != "VPN 30 days" || payload["comment"] != "4242" {
			t.Errorf("invalid Tribute order: %#v", payload)
		}
		_, _ = w.Write([]byte(`{"uuid":"shop-order-1","paymentUrl":"https://pay.example/shop-order-1"}`))
	}))
	defer server.Close()
	gateway := &Gateway{httpClient: server.Client()}
	cfg := map[string]string{"apiUrl": server.URL, "apiKey": "api-key", "shopId": "7"}
	created, err := gateway.createTribute(context.Background(), cfg, CreatePaymentRequest{PurchaseID: 4242, CustomerID: 8, Amount: 170.5, Currency: "RUB", Description: "VPN 30 days"})
	if err != nil || created.ExternalID != "shop-order-1" {
		t.Fatalf("create Tribute order: %+v, %v", created, err)
	}
	for _, status := range []string{"paid", "pending"} {
		raw, _ := json.Marshal(map[string]any{"name": "shop_order", "payload": map[string]any{"uuid": "shop-order-1", "amount": 17050, "currency": "rub", "status": status}})
		headers := make(http.Header)
		headers.Set("trbt-signature", hmacHex(sha256.New, []byte("api-key"), raw))
		event, err := parseTributeWebhook(cfg, headers, raw)
		if err != nil || event.Amount != 170.5 || event.ExternalID != created.ExternalID || event.Paid != (status == "paid") {
			t.Fatalf("Tribute %s callback: %+v, %v", status, event, err)
		}
		headers.Set("trbt-signature", "invalid")
		if _, err := parseTributeWebhook(cfg, headers, raw); err == nil {
			t.Fatal("accepted an unsigned Tribute callback")
		}
	}
}

func TestUnavailableProvidersCannotBeEnabled(t *testing.T) {
	service := &Service{}
	for _, provider := range []string{"paycore"} {
		if configured(provider, map[string]string{}) {
			t.Errorf("%s is configured without an implemented API", provider)
		}
		if _, err := service.Update(context.Background(), provider, UpdateInput{Enabled: true}, 1); err == nil {
			t.Errorf("%s can be enabled without an implemented API", provider)
		}
	}
}
