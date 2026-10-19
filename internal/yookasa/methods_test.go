package yookasa

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestCheckoutMethodAndMerchantDiscovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing merchant authentication")
		}
		if r.URL.Path == "/me" {
			_, _ = w.Write([]byte(`{"payment_methods":["bank_card","sbp","sberbank"]}`))
			return
		}
		var payload PaymentRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.PaymentMethodData == nil || payload.PaymentMethodData.Type != "sberbank" {
			t.Error("SberPay restriction missing")
		}
		if payload.Amount.Currency != "RUB" || payload.Amount.Value != "170" {
			t.Error("amount changed")
		}
		_, _ = w.Write([]byte(`{"id":"11111111-1111-4111-8111-111111111111","confirmation":{"confirmation_url":"https://pay.example/1"}}`))
	}))
	defer server.Close()
	client := NewConfiguredClient(server.URL, "shop", "secret", "receipt@example.com")
	methods, err := client.AvailablePaymentMethods(context.Background())
	if err != nil || !reflect.DeepEqual(methods, []string{"bank_card", "sbp", "sberbank"}) {
		t.Fatal(methods, err)
	}
	if _, err := client.CreateInvoiceWithMethod(context.Background(), 170, 5, 42, "https://vpn.example/return", "Test", "sberbank"); err != nil {
		t.Fatal(err)
	}
}
