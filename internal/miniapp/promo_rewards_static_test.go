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

func TestProfilePromoAnimationDoesNotRemountOnRefresh(t *testing.T) {
	appJS, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(appJS)
	renderStart := strings.Index(content, "function render({")
	renderEnd := strings.Index(content, "function renderSidebar(")
	openStart := strings.Index(content, "function openProfilePromo()")
	closeStart := strings.Index(content, "function closeProfilePromo()")
	validationStart := strings.Index(content, "function syncProfilePromoValidation()")
	applyStart := strings.Index(content, "async function applyProfilePromo()")
	applyEnd := strings.Index(content, "function promoRewardDescription(")
	if renderStart < 0 || renderEnd <= renderStart || openStart < 0 || closeStart <= openStart || validationStart <= closeStart || applyStart <= validationStart || applyEnd <= applyStart {
		t.Fatal("profile promo lifecycle functions are missing or reordered")
	}
	if strings.Contains(content[renderStart:renderEnd], "renderProfilePromoModal()") {
		t.Fatal("dashboard refresh must not remount the promo dialog")
	}
	open := content[openStart:closeStart]
	close := content[closeStart:validationStart]
	apply := content[applyStart:applyEnd]
	if !strings.Contains(open, `document.body.insertAdjacentHTML("beforeend", renderProfilePromoModal())`) || strings.Contains(open, "render({") {
		t.Fatal("opening the promo dialog must mount it once without rerendering the whole app")
	}
	if !strings.Contains(close, `modal?.classList.add("modal--closing")`) || strings.Contains(close, "requestModalClose(") {
		t.Fatal("closing the promo dialog must animate the existing node")
	}
	if !strings.Contains(apply, "form.outerHTML = renderProfilePromoSuccess()") || strings.Contains(apply, "render({") {
		t.Fatal("reward success must update the dialog content without replaying its entrance")
	}
	if !strings.Contains(close, "if (redeemed) void safeRefresh()") {
		t.Fatal("refresh must occur after the closing transition")
	}
}
