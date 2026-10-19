package handler

import (
	"strings"
	"testing"

	planbook "link-bot/internal/plans"
	"link-bot/internal/runtimeconfig"
)

func TestConfiguredPaymentKeyboardPreservesCopies(t *testing.T) {
	s := runtimeconfig.DefaultSettings()
	s.PaymentMethodVariants = map[string]runtimeconfig.PaymentMethodVariant{"sbp": {Method: "sbp", Channels: []string{"sbp", "sberbank"}}, "sbp__aabbccdd": {Method: "sbp", Channels: []string{"bank_card"}}}
	s.PaymentMethodText = map[string]runtimeconfig.PaymentMethodText{"sbp__aabbccdd": {Name: "Карта"}}
	s.PaymentMethodOrder = []string{"sbp__aabbccdd", "sbp"}
	plan := planbook.CheckoutPlan{ID: strings.Repeat("longplan", 12), PriceRub: 100}
	rows := configuredPaymentKeyboard(map[string]bool{"sbp": true, "sbp__aabbccdd": true}, plan, s, false)
	if len(rows) != 2 || rows[0][0].Text != "Карта" || !strings.Contains(rows[0][0].CallbackData, "m=sbp__aabbccdd") {
		t.Fatalf("copies collapsed: %+v", rows)
	}
	for _, row := range rows {
		if len(row[0].CallbackData) > 64 {
			t.Fatal("callback exceeds Telegram limit")
		}
	}
	channels := paymentChannelKeyboard(plan, "sbp__aabbccdd", []string{"sbp", "sberbank"})
	if len(channels) != 2 || channels[1][0].Text != "SberPay" || !strings.HasSuffix(channels[1][0].CallbackData, "&c=1") {
		t.Fatal(channels)
	}
}
