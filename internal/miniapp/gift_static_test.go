package miniapp

import (
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
		"@media (prefers-reduced-motion: reduce)",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("styles.css does not contain %q", expected)
		}
	}
}
