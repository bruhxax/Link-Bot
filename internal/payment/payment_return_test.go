package payment

import "testing"

func TestNormalizePaymentReturnTargetAcceptsBrowserSurface(t *testing.T) {
	for _, value := range []string{"web", "browser", " BROWSER "} {
		if got := normalizePaymentReturnTarget(value); got != "web" {
			t.Fatalf("normalizePaymentReturnTarget(%q) = %q, want web", value, got)
		}
	}
}

func TestNormalizePaymentReturnTargetDefaultsToTelegram(t *testing.T) {
	for _, value := range []string{"", "telegram", "unexpected"} {
		if got := normalizePaymentReturnTarget(value); got != "telegram" {
			t.Fatalf("normalizePaymentReturnTarget(%q) = %q, want telegram", value, got)
		}
	}
}
