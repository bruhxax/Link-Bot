package payment

import (
	"testing"

	"link-bot/internal/integrations"
)

func TestWebhookAmountMatches(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		paid     float64
		expected float64
		want     bool
	}{
		{name: "missing amount is accepted", provider: integrations.ProviderPlatega, paid: 0, expected: 200, want: true},
		{name: "exact Platega amount", provider: integrations.ProviderPlatega, paid: 200, expected: 200, want: true},
		{name: "Platega customer commission", provider: integrations.ProviderPlatega, paid: 211, expected: 200, want: true},
		{name: "Platega underpayment", provider: integrations.ProviderPlatega, paid: 199, expected: 200, want: false},
		{name: "other provider exact amount", provider: integrations.ProviderLava, paid: 200, expected: 200, want: true},
		{name: "other provider overpayment", provider: integrations.ProviderLava, paid: 211, expected: 200, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := webhookAmountMatches(test.provider, test.paid, test.expected); got != test.want {
				t.Fatalf("webhookAmountMatches(%q, %.2f, %.2f) = %v, want %v", test.provider, test.paid, test.expected, got, test.want)
			}
		})
	}
}
