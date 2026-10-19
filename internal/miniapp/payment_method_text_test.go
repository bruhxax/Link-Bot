package miniapp

import (
	"link-bot/internal/runtimeconfig"
	"testing"
)

func TestPaymentMethodTextBelongsToPlanPermissions(t *testing.T) {
	current := runtimeconfig.DefaultSettings()
	next := runtimeconfig.DefaultSettings()
	next.PaymentMethodText = map[string]runtimeconfig.PaymentMethodText{"sbp": {Name: "Custom", Description: "Details"}}
	next.PaymentMethodVariants = map[string]runtimeconfig.PaymentMethodVariant{"sbp__aabbccdd": {Method: "sbp", Channels: []string{"sberbank"}}}
	merged, err := mergeAdminSettings(current, next, "plans")
	if err != nil || merged.PaymentMethodText["sbp"].Name != "Custom" {
		t.Fatal("plan editor cannot save payment names", err)
	}
	if merged.PaymentMethodBase("sbp__aabbccdd") != "sbp" {
		t.Fatal("plan editor cannot save payment variants")
	}
	merged, err = mergeAdminSettings(current, next, "content")
	if err != nil || len(merged.PaymentMethodText) != 0 || len(merged.PaymentMethodVariants) != 0 {
		t.Fatal("unrelated permission can overwrite payment names", err)
	}
}
