package miniapp

import (
	"strings"
	"testing"
)

func TestSupportMessagesRenderSafeClickableLinksAndUsernames(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	tokenizerRaw, err := embeddedStatic.ReadFile("static/support-message.mjs")
	if err != nil {
		t.Fatalf("read support-message.mjs: %v", err)
	}
	app := string(appRaw)
	for _, fragment := range []string{
		`renderSupportMessageBody(message.body || "")`,
		`import { tokenizeSupportMessage } from "./support-message.mjs";`,
		`data-action="open-support-link"`,
		`data-action="open-support-username"`,
		`https://t.me/${username}`,
		`if (action === "open-support-username") return openExternal(value);`,
		`function isSupportSubscriptionLink`,
		`buildSetupBridgeURL(state.selectedPlatform, appItem.id, url)`,
		`try_browser: true`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("support link behavior is missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		`function isStandaloneTelegramUsername`,
		`|@[a-z][a-z0-9_]{4,31}/giu`,
		`username ? "username" : "link"`,
	} {
		if !strings.Contains(string(tokenizerRaw), fragment) {
			t.Fatalf("support username tokenizer is missing %q", fragment)
		}
	}
	for _, fragment := range []string{`.support-message__link {`, `.support-message__link:focus-visible`} {
		if !strings.Contains(string(stylesRaw), fragment) {
			t.Fatalf("support link style is missing %q", fragment)
		}
	}
}
