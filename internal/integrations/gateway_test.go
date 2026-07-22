package integrations

import (
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
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
