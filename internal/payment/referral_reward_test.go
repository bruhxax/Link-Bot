package payment

import (
	"testing"

	"link-bot/internal/runtimeconfig"
)

func TestReferralBalanceRewardCents(t *testing.T) {
	tests := []struct {
		name     string
		settings runtimeconfig.ReferralRewardSettings
		want     int64
	}{
		{name: "none", settings: runtimeconfig.ReferralRewardSettings{BalanceMode: "none"}, want: 0},
		{name: "fixed", settings: runtimeconfig.ReferralRewardSettings{BalanceMode: "fixed", BalanceRub: 75}, want: 7500},
		{name: "percent", settings: runtimeconfig.ReferralRewardSettings{BalanceMode: "percent", BalancePercent: 10}, want: 3499},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := referralBalanceRewardCents(tt.settings, 349.90); got != tt.want {
				t.Fatalf("referralBalanceRewardCents() = %d, want %d", got, tt.want)
			}
		})
	}
}
