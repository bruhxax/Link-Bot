package miniapp

import (
	"link-bot/internal/runtimeconfig"
	"testing"
)

func TestPaymentMethodTextBelongsToPlanPermissions(t *testing.T) {
	current := runtimeconfig.DefaultSettings()
	next := runtimeconfig.DefaultSettings()
	next.PaymentMethodText = map[string]runtimeconfig.PaymentMethodText{"sbp": {Name: "Custom", Description: "Details"}}
	merged, err := mergeAdminSettings(current, next, "plans")
	if err != nil || merged.PaymentMethodText["sbp"].Name != "Custom" {
		t.Fatal("plan editor cannot save payment names", err)
	}
	merged, err = mergeAdminSettings(current, next, "content")
	if err != nil || len(merged.PaymentMethodText) != 0 {
		t.Fatal("unrelated permission can overwrite payment names", err)
	}
}
