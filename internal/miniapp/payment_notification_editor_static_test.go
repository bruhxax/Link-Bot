package miniapp

import (
	"os"
	"strings"
	"testing"
)

func TestPaymentNotificationEditorWiresSettingsAndPreview(t *testing.T) {
	appJS, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	content := string(appJS)
	for _, expected := range []string{
		"Уведомления оплат",
		"content.paymentNotification.text",
		"content.paymentNotification.openUserButton",
		"content.paymentNotification.profileButton",
		"admin-test-payment-notification",
		"/api/mini-app/admin/payment-notifications/test",
		"{{data}}",
		"{{integration}}",
		"{{promo}}",
		"{{sub}}",
		"{{username}}",
		"{{number}}",
		"{{price}}",
		"{{device}}",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("app.js does not contain %q", expected)
		}
	}
}
