package miniapp

import (
	"strings"
	"testing"
)

func TestFramesOffCoversEveryMiniAppBorder(t *testing.T) {
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}

	styles := string(stylesRaw)
	globalRule := `:root[data-frames="off"] body *`
	for _, fragment := range []string{
		globalRule,
		`:root[data-frames="off"] body *::before`,
		`:root[data-frames="off"] body *::after`,
		`border-color: transparent !important;`,
		`:root[data-frames="off"] .runtime-layout-item--framed`,
		`:root[data-frames="off"] .subscription-switcher__progress`,
		`button:focus-visible`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("styles.css does not contain %q", fragment)
		}
	}
	if strings.LastIndex(styles, globalRule) < strings.LastIndex(styles, `.server-tabs .tab.active`) {
		t.Fatal("global frame override must remain after component theme rules")
	}
	if !strings.Contains(string(appRaw), `document.documentElement.dataset.frames = appearance.showFrames === false ? "off" : "on"`) {
		t.Fatal("app.js does not apply the saved frame setting")
	}
}
