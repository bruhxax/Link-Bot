package integrations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestPlategaTransactionBuyerIdentity(t *testing.T) {
	tests := []struct {
		name        string
		input       CreatePaymentRequest
		wantEmail   string
		wantRequest bool
	}{
		{
			name:      "Telegram buyer keeps one stable identifier",
			input:     CreatePaymentRequest{TelegramID: 5512345678, CustomerID: 8, Email: "receipt@example.com"},
			wantEmail: "5512345678@telegram.org", wantRequest: true,
		},
		{
			name:      "browser buyer uses their email instead of synthetic Telegram ID",
			input:     CreatePaymentRequest{TelegramID: 9000000000000000, TelegramIDIsSynthetic: true, CustomerID: 9, Email: "buyer@example.com"},
			wantEmail: "buyer@example.com", wantRequest: true,
		},
		{
			name:  "missing buyer identity never starts a transaction",
			input: CreatePaymentRequest{CustomerID: 10},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/v2/transaction/process" {
					t.Errorf("unexpected Platega path: %s", r.URL.Path)
				}
				var body struct {
					Metadata map[string]string `json:"metadata"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Metadata["user_email"] != tc.wantEmail || body.Metadata["userId"] != strconv.FormatInt(tc.input.CustomerID, 10) {
					t.Errorf("wrong buyer metadata: %+v", body.Metadata)
				}
				_, _ = w.Write([]byte(`{"transactionId":"payment-1","url":"https://pay.platega.io/checkout"}`))
			}))
			defer server.Close()
			gateway := &Gateway{httpClient: server.Client()}
			input := tc.input
			input.PurchaseID = 42
			input.Amount = 200
			input.Currency = "RUB"
			result, err := gateway.createPlatega(context.Background(), map[string]string{"apiUrl": server.URL}, input)
			if tc.wantRequest {
				if err != nil || result.ExternalID != "payment-1" || requests != 1 {
					t.Fatalf("create Platega payment: %+v, %v; requests=%d", result, err, requests)
				}
			} else if err == nil || requests != 0 {
				t.Fatalf("created payment without buyer identity: %+v, %v; requests=%d", result, err, requests)
			}
		})
	}
}
