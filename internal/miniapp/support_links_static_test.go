package miniapp

import (
	"strings"
	"testing"
)

func TestSupportMessagesRenderSafeClickableLinks(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	app := string(appRaw)
	for _, fragment := range []string{
		`renderSupportMessageBody(message.body || "")`,
		`data-action="open-support-link"`,
		`function isSupportSubscriptionLink`,
		`buildSetupBridgeURL(state.selectedPlatform, appItem.id, url)`,
		`try_browser: true`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("support link behavior is missing %q", fragment)
		}
	}
	for _, fragment := range []string{`.support-message__link {`, `.support-message__link:focus-visible`} {
		if !strings.Contains(string(stylesRaw), fragment) {
			t.Fatalf("support link style is missing %q", fragment)
		}
	}
}
