package miniapp

import (
	"strings"
	"testing"
)

func TestMiniAppUsesThemedNavigationTrialAndDeleteIcons(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read embedded app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read embedded styles.css: %v", err)
	}

	app := string(appRaw)
	styles := string(stylesRaw)
	for _, expected := range []string{
		`buy: "shop", support: "sms"`,
		`"navigation:buy": ["\u0422\u0430\u0440\u0438\u0444\u044b", "shop"]`,
		`"navigation:support": ["\u041f\u043e\u0434\u0434\u0435\u0440\u0436\u043a\u0430", "sms"]`,
		`data-action="activate-trial">${icon("gift")}`,
		"shop: `<svg",
		"sms: `<svg",
		"trash: `<svg",
		`stroke="currentColor"`,
	} {
		if !strings.Contains(app, expected) {
			t.Fatalf("app.js does not contain %q", expected)
		}
	}

	for _, expected := range []string{
		`.device-list {`,
		`border: 0;`,
		`background: transparent;`,
		`.device-row__delete {`,
		`color: var(--danger);`,
		`.btn--trial svg {`,
		`color: var(--icon-color);`,
	} {
		if !strings.Contains(styles, expected) {
			t.Fatalf("styles.css does not contain %q", expected)
		}
	}
}
