package integrations

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"link-bot/internal/paymentchannel"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type checkoutChannelTransport func(*http.Request) (*http.Response, error)

func (f checkoutChannelTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGatewaySendsConfiguredChannelsToEverySupportedAPI(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	for _, tc := range []struct {
		provider string
		channels []string
		field    string
		want     any
	}{
		{ProviderLava, []string{"sbp", "sber_pay"}, "includeService", []any{"sbp", "sber_pay"}},
		{ProviderPlatega, []string{"11"}, "paymentMethod", float64(11)},
		{ProviderHeleket, []string{"USDT:tron"}, "currencies", []any{map[string]any{"currency": "USDT", "network": "tron"}}},
		{ProviderPally, []string{"SBP"}, "payment_method", "SBP"},
		{ProviderRollyPay, []string{"crypto"}, "payment_method", "crypto"},
		{ProviderCisPay, []string{"SBP"}, "payment_method", "SBP"},
		{ProviderAnore, []string{"sbp", "card"}, "methods", []any{"sbp", "card"}},
		{ProviderAuraPay, []string{"sbp"}, "service", "sbp"},
		{ProviderParityPay, []string{"card"}, "service", "card"},
		{ProviderAntiloPay, []string{"SBP", "SBER_PAY"}, "prefer_methods", []any{"SBP", "SBER_PAY"}},
		{ProviderTribute, []string{"sbp"}, "preferredPaymentMethods", []any{map[string]any{"type": "sbp"}}},
		{ProviderKassaAI, []string{"43"}, "i", float64(43)},
		{ProviderFreeKassa, []string{"36"}, "i", float64(36)},
		{ProviderPayHot, []string{"sberpay"}, "payment_method", "sberpay"},
		{ProviderSmerch, []string{"crypto"}, "payment_method", "crypto"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			cfg := map[string]string{"shopId": "123", "apiKey": "phk_v2_fixture", "apiUrl": "https://api.example", "secretKey": "fixture", "paymentMethod": "CARD", "paymentSystemId": "44", "privateKey": base64.StdEncoding.EncodeToString(der)}
			definition, _ := definitionByID(tc.provider)
			for _, field := range definition.Fields {
				if cfg[field.Key] == "" {
					cfg[field.Key] = "fixture"
				}
			}
			service := &Service{baseURL: "https://vpn.example", records: map[string]record{tc.provider: {Enabled: true, Config: cfg}}}
			calls := 0
			g := NewGateway(service)
			g.httpClient = &http.Client{Transport: checkoutChannelTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				raw, _ := io.ReadAll(r.Body)
				payload := map[string]any{}
				if tc.provider == ProviderPally {
					form, _ := url.ParseQuery(string(raw))
					payload["payment_method"] = form.Get("payment_method")
				} else if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(payload[tc.field], tc.want) {
					t.Errorf("%s = %#v, want %#v", tc.field, payload[tc.field], tc.want)
				}
				if tc.provider == ProviderLava && r.Header.Get("Signature") != hmacHex(sha256.New, []byte(cfg["secretKey"]), raw) {
					t.Error("LAVA signed payload without the selected methods")
				}
				response := `{"id":"external-1","uuid":"external-1","url":"https://pay.example/1","link":"https://pay.example/1","success":true,"link_page_url":"https://pay.example/1","bill_id":123,"payment_id":"external-1","pay_url":"https://pay.example/1","payment_url":"https://pay.example/1","paymentUrl":"https://pay.example/1","code":0,"transactionId":"external-1","state":0,"result":{"uuid":"external-1","url":"https://pay.example/1"},"data":{"id":"external-1","url":"https://pay.example/1"},"payment_data":{"url":"https://pay.example/1"},"type":"success","orderId":123,"location":"https://pay.example/1"}`
				if tc.provider == ProviderSmerch {
					response = `{"uuid":"` + smerchTestID + `","external_id":"42","amount":17000,"currency":"RUB","payment_method":"crypto","payment_url":"https://pay.smerch.io/invoice/fixture"}`
				}
				if tc.provider == ProviderPayHot {
					response = `{"payment":{"id":"` + payHotTestID + `","merchant_order_reference":"42","amount_minor":"17000","currency":"RUB"},"checkout":{"url":"` + payHotTestURL + `"}}`
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
			})}
			_, err := g.Create(context.Background(), CreatePaymentRequest{Provider: tc.provider, Channels: tc.channels, PurchaseID: 42, CustomerID: 8, Amount: 170, Currency: "RUB", ClientIP: "203.0.113.1", Email: "payer@example.com", ReturnURL: "https://vpn.example/return"})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("HTTP calls %d", calls)
			}
			if cfg["paymentMethod"] != "CARD" || cfg["paymentSystemId"] != "44" {
				t.Fatal("per-row channel changed shared integration settings")
			}
		})
	}
}

func TestUnsupportedGatewayChannelsNeverCreateAnInvoice(t *testing.T) {
	g := NewGateway(&Service{records: map[string]record{ProviderWata: {Enabled: true, Config: map[string]string{"accessToken": "fixture", "apiUrl": "https://api.example"}}}})
	g.httpClient = &http.Client{Transport: checkoutChannelTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("request made for unsupported channel")
		return nil, nil
	})}
	if _, err := g.Create(context.Background(), CreatePaymentRequest{Provider: ProviderWata, Channels: []string{"card"}}); err == nil {
		t.Fatal("unsupported restriction silently ignored")
	}
}

func TestDiscoveryReadsMerchantMethodsAndFiltersUnavailable(t *testing.T) {
	for _, tc := range []struct {
		provider, body, want string
		count                int
	}{
		{ProviderHeleket, `{"state":0,"result":[{"currency":"USDT","network":"tron","is_available":true},{"currency":"BTC","network":"btc","is_available":false}]}`, "USDT:tron", 2},
		{ProviderLava, `{"status":200,"data":[{"service_id":"card_ru","service_name":"Карты РФ","status":200},{"service_id":"sbp","service_name":"СБП","status":400}]}`, "card_ru", 1},
		{ProviderFreeKassa, `{"type":"success","currencies":[{"id":44,"name":"СБП","currency":"RUB","is_enabled":1},{"id":36,"name":"Карта","currency":"RUB","is_enabled":0}]}`, "44", 1},
		{ProviderKassaAI, `{"type":"success","currencies":[{"id":43,"name":"SberPay","currency":"RUB","is_enabled":1}]}`, "43", 1},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			cfg := map[string]string{"shopId": "123", "apiKey": "fixture", "apiUrl": "https://api.example", "merchantId": "fixture", "secretKey": "fixture", "additionalKey": "fixture", "secretWord": "fixture", "secretWord2": "fixture", "paymentSystemId": "44"}
			g := NewGateway(&Service{records: map[string]record{tc.provider: {Enabled: true, Config: cfg}}})
			g.httpClient = &http.Client{Transport: checkoutChannelTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != "POST" {
					t.Error("unexpected discovery method")
				}
				if strings.Contains(r.URL.Path, "/orders/create") || r.URL.Path == "/v1/payment" {
					t.Fatal("discovery created a real invoice")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			catalog, err := g.PaymentChannels(context.Background(), tc.provider)
			if err != nil {
				t.Fatal(err)
			}
			if !catalog.Live || len(catalog.Options) != tc.count {
				t.Fatalf("unexpected catalog %+v", catalog)
			}
			found := false
			for _, option := range catalog.Options {
				if option.ID == tc.want {
					found = true
				}
				if !paymentchannel.Valid(tc.provider, option.ID) {
					t.Error("discovered method rejected by checkout")
				}
			}
			if !found {
				t.Fatal("merchant method lost")
			}
		})
	}
}
