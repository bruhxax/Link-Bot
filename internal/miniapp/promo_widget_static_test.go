package miniapp

import (
	"os"
	"strings"
	"testing"
)

func TestPromoWidgetWiresEditorValidationAndCheckout(t *testing.T) {
	appJS, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	content := string(appJS)
	for _, expected := range []string{
		`id: "promo_widget"`,
		`data-action="admin-add-promo-widget"`,
		`data-action="admin-edit-promo-widget"`,
		`data-action="admin-validate-promo-widget"`,
		`/api/mini-app/admin/promocodes/validate`,
		`data-action="open-promo-widget-checkout"`,
		`await applyPromoCode();`,
		`class="promo-gift-widget__mark"`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("app.js does not contain %q", expected)
		}
	}
}

func TestPromoWidgetUsesThemeAwareSVGAndAccessibleControls(t *testing.T) {
	styles, err := os.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	content := string(styles)
	for _, expected := range []string{
		`.promo-gift-widget`,
		`url("/mini-app/assets/promo-gift.svg")`,
		`color: var(--accent)`,
		`.layout-editable__edit:focus-visible`,
		`outline-color: var(--accent)`,
		`@media (prefers-reduced-motion: reduce)`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("styles.css does not contain %q", expected)
		}
	}

	asset, err := os.ReadFile("static/assets/promo-gift.svg")
	if err != nil {
		t.Fatalf("read promo gift SVG: %v", err)
	}
	if !strings.Contains(string(asset), `viewBox="0 0 24 24"`) {
		t.Fatal("promo gift SVG has an unexpected viewBox")
	}
}

func TestPromoWidgetAdminValidationRouteIsRegistered(t *testing.T) {
	handler, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("read handler.go: %v", err)
	}
	content := string(handler)
	for _, expected := range []string{
		`/api/mini-app/admin/promocodes/validate`,
		`handleAdminValidatePromoCode`,
		`resolvePromoCode(r.Context(), 0, req.Code)`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("handler.go does not contain %q", expected)
		}
	}
}
