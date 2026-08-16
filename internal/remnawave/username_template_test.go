package remnawave

import (
	"strings"
	"testing"
)

func TestGenerateUsername(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		template   string
		customerID int64
		telegramID int64
		want       string
	}{
		{name: "default", customerID: 15, telegramID: 6402520205, want: "15_6402520205"},
		{name: "telegram prefix", template: "tg_{{telegram_id}}", customerID: 15, telegramID: 6402520205, want: "tg_6402520205"},
		{name: "sanitize", template: " VPN / {{customer_id}} / {{telegram_id}} ", customerID: 15, telegramID: 20, want: "VPN_15_20"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := generateUsername(tt.template, tt.customerID, tt.telegramID); got != tt.want {
				t.Fatalf("generateUsername() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateUsernameLimitsLength(t *testing.T) {
	t.Parallel()

	got := generateUsername(strings.Repeat("a", 80)+"{{telegram_id}}", 1, 2)
	if len(got) != 64 {
		t.Fatalf("generateUsername() length = %d, want 64", len(got))
	}
}
