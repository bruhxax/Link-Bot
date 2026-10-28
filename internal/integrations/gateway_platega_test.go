package integrations

import (
	"fmt"
	"net/http"
	"testing"
)

func TestPlategaWebhookStatuses(t *testing.T) {
	cfg := map[string]string{"merchantId": "merchant", "secretKey": "secret"}
	headers := make(http.Header)
	headers.Set("X-MerchantId", cfg["merchantId"])
	headers.Set("X-Secret", cfg["secretKey"])
	for _, tc := range []struct {
		status                   string
		paid, cancelled, ignored bool
	}{
		{"CONFIRMED", true, false, false},
		{" confirmed ", true, false, false},
		{"CANCELED", false, true, false},
		{"CHARGEBACKED", false, true, false},
		{"DISPUTED", false, false, true},
		{"disputed", false, false, true},
		{"PARTIALLY_REFUNDED", false, false, true},
		{"PENDING", false, false, true},
		{"SUBSCRIPTION_ACTIVATED", false, false, true},
		{"UNKNOWN", false, false, true},
	} {
		t.Run(tc.status, func(t *testing.T) {
			raw := []byte(fmt.Sprintf(`{"id":"transaction-id","amount":211,"currency":"RUB","status":%q,"payload":"purchaseId=999"}`, tc.status))
			event, err := parsePlategaWebhook(cfg, headers, raw)
			if err != nil || event.Paid != tc.paid || event.Cancelled != tc.cancelled || event.Ignored != tc.ignored {
				t.Fatalf("unexpected status result: %+v, %v", event, err)
			}
			if event.ExternalID != "transaction-id" || event.PurchaseID != 0 || event.Amount != 211 || event.Currency != "RUB" {
				t.Fatalf("transaction identity or amount changed: %+v", event)
			}
		})
	}
	for _, status := range []string{"CONFIRMED", "DISPUTED"} {
		raw := []byte(fmt.Sprintf(`{"id":"transaction-id","status":%q}`, status))
		for _, key := range []string{"X-MerchantId", "X-Secret"} {
			invalid := headers.Clone()
			invalid.Set(key, "wrong")
			if _, err := parsePlategaWebhook(cfg, invalid, raw); err == nil {
				t.Fatalf("accepted %s with invalid %s", status, key)
			}
		}
	}
	for _, raw := range []string{`{`, `{"status":"CONFIRMED"}`, `{"id":" ","status":"DISPUTED"}`} {
		if _, err := parsePlategaWebhook(cfg, headers, []byte(raw)); err == nil {
			t.Fatalf("accepted malformed callback: %s", raw)
		}
	}
}
