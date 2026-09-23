package miniapp

import (
	"os"
	"strings"
	"testing"
)

func TestPromoRewardProfileAndAdminWiring(t *testing.T) {
	appJS, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(appJS)
	for _, required := range []string{
		`data-action="open-profile-promo"`,
		`data-input="profile-promo-code"`,
		`data-action="apply-profile-promo"`,
		`Это промокод скидки, введите его при оплате`,
		`Промокод существует`,
		`Промокод не найден`,
		`profilePromoCloseTimer = window.setTimeout`,
		`["days_traffic", localizedText("Дни + ГБ"`,
		`rewardTrafficGb: rewardType === "days_traffic"`,
		`/api/mini-app/promocode/redeem`,
	} {
		if !strings.Contains(content, required) {
			t.Errorf("missing reward UI wiring %q", required)
		}
	}
}
