package miniapp

import (
	"os"
	"strings"
	"testing"
)

func TestSubscriptionSwitchUsesPendingExitAndEntranceStates(t *testing.T) {
	appJS, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	content := string(appJS)
	for _, expected := range []string{
		`subscriptionSwitchAnimation: ""`,
		`markSubscriptionSwitchPending(id)`,
		`state.subscriptionSwitchAnimation = ` + "`out-${direction}`",
		`state.subscriptionSwitchAnimation = ` + "`in-${direction}`",
		`class="subscription-switcher__progress"`,
		`aria-busy="${Boolean(switchingID)}"`,
		`document.documentElement.dir === "rtl"`,
		`!state.subscriptionMenuOpen && !state.subscriptionMenuClosing`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("app.js does not contain %q", expected)
		}
	}
}

func TestSubscriptionSwitchMotionIsDirectionalAndReducedMotionSafe(t *testing.T) {
	styles, err := os.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	content := string(styles)
	for _, expected := range []string{
		`.subscription-switcher__item.is-switching`,
		`@keyframes subscriptionSwitchProgress`,
		`@keyframes subscriptionContentOutNext`,
		`@keyframes subscriptionContentOutPrevious`,
		`@keyframes subscriptionContentInNext`,
		`@keyframes subscriptionContentInPrevious`,
		`#page-dashboard[class*="subscription-switch--"]`,
		`animation: none !important`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("styles.css does not contain %q", expected)
		}
	}
}
