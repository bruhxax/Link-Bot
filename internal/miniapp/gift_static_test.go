package miniapp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestGiftMiniAppWiresProfileCheckoutReceiptAndEditor(t *testing.T) {
	appJS, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	content := string(appJS)
	for _, expected := range []string{
		`pageGift: "Подарить"`,
		`value: "gift", icon: "gift", feature: "gifts"`,
		`data-input="gift-username"`,
		`data-action="select-gift-plan"`,
		`data-action="start-gift-payment"`,
		`id="gift-promo-code"`,
		`promoCode: getActivePromo()?.code || ""`,
		`app.querySelectorAll("[data-promo-status]")`,
		`getGiftPlanTitle(plan, state.locale)`,
		`/api/mini-app/gifts/purchase`,
		`/api/mini-app/gifts/seen`,
		`data-action="copy-gift-link"`,
		`["gift", "Подарок"]`,
		`content.gift.${kind}.text`,
		`admin-test-gift`,
		`/api/mini-app/admin/gifts/test`,
		`iconCustomEmojiId`,
		`Цвет кнопки`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("app.js does not contain %q", expected)
		}
	}
}

func TestGiftPurchaseRequestAcceptsPromoCode(t *testing.T) {
	var request giftPurchaseRequest
	if err := json.Unmarshal([]byte(`{"username":"friend_user","planId":"gift-1m","months":1,"paymentMethod":"stars","promoCode":"GIFT20"}`), &request); err != nil {
		t.Fatalf("decode gift purchase request: %v", err)
	}
	if request.PromoCode != "GIFT20" {
		t.Fatalf("gift promo code = %q, want GIFT20", request.PromoCode)
	}
}

func TestGiftMiniAppHasCompactReferenceStylesAndFocusStates(t *testing.T) {
	styles, err := os.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	content := string(styles)
	for _, expected := range []string{
		".gift-plan-row",
		".gift-plan-row.is-selected",
		".gift-username:focus-within",
		".gift-plan-row:focus-visible",
		".modal__sheet--gift-receipt",
		".gift-receipt__share",
		"background: var(--accent)",
		"color: var(--accent)",
		"@media (prefers-reduced-motion: reduce)",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("styles.css does not contain %q", expected)
		}
	}

	start := strings.Index(content, "/* Gift checkout:")
	end := strings.Index(content, ".admin-gift-editor")
	if start < 0 || end <= start {
		t.Fatal("gift style section boundaries not found")
	}
	giftStyles := content[start:end]
	for _, forbidden := range []string{"#1264e8", "#4e94ff", "#67a0ff", "#2f78ef", "#0b1d3a", "#0d1a2e"} {
		if strings.Contains(giftStyles, forbidden) {
			t.Fatalf("gift styles still contain hardcoded blue %q", forbidden)
		}
	}
}
