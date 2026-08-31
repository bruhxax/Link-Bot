package miniapp

import (
	"os"
	"strings"
	"testing"
)

func TestReferralWalletUIIncludesSettingsPaymentsAndWithdrawals(t *testing.T) {
	appJS, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	styles, err := os.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	for _, expected := range []string{
		`state.adminSection === "referrals"`,
		`renderAdminReferralRewardFields("referrals.purchase", true)`,
		`.balancePercent`,
		`paymentMethodMeta(id)`,
		`balance: { id: "balance"`,
		`/api/mini-app/wallet/withdraw`,
		`/api/mini-app/admin/wallet/withdrawal/resolve`,
		`wallet-withdrawal-amount`,
	} {
		if !strings.Contains(string(appJS), expected) {
			t.Fatalf("app.js does not contain %q", expected)
		}
	}
	for _, expected := range []string{".referral-metrics", ".referral-share-actions", ".wallet-withdrawal", ".admin-withdrawal", ":focus-visible"} {
		if !strings.Contains(string(styles), expected) {
			t.Fatalf("styles.css does not contain %q", expected)
		}
	}
	for _, removed := range []string{"referral-reward-list", "renderReferralRewardRule("} {
		if strings.Contains(string(appJS), removed) {
			t.Fatalf("app.js still contains removed reward card marker %q", removed)
		}
	}
}
