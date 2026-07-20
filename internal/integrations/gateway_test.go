package integrations

import (
	"crypto/sha256"
	"net/http"
	"testing"
)

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
