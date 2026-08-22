package miniapp

import (
	"os"
	"strings"
	"testing"
)

func TestNotificationWidgetWiresEditorUnreadStateAndPositionedPopover(t *testing.T) {
	appJS, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	content := string(appJS)
	for _, expected := range []string{
		`id: "notification_widget"`,
		`data-action="admin-add-notification-widget"`,
		`data-action="admin-edit-notification-widget"`,
		`data-action="admin-apply-notification-widget"`,
		`data-action="admin-remove-notification-widget"`,
		`data-input="admin-notification-widget-text"`,
		`data-input="admin-notification-widget-bubble"`,
		`data-action="open-notification-widget"`,
		`notificationWidgetFingerprint`,
		`STORAGE_KEYS.notificationRead`,
		`notification-widget__unread`,
		`window.innerWidth * 0.38`,
		`window.innerWidth * 0.62`,
		`return { width: 36, height: 36 }`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("app.js does not contain %q", expected)
		}
	}
}

func TestNotificationWidgetUsesProvidedThemeAwareBellAndAccessibleMotion(t *testing.T) {
	styles, err := os.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	content := string(styles)
	for _, expected := range []string{
		`url("/mini-app/assets/notification-bell.svg")`,
		`.notification-widget--plain`,
		`.notification-popover--start`,
		`.notification-popover--center`,
		`.notification-popover--end`,
		`background: #ff3b30`,
		`color: var(--icon-color)`,
		`.home-widget:hover`,
		`.home-widget:active`,
		`@media (prefers-reduced-motion: reduce)`,
		`:has(.notification-widget-shell.is-expanded)`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("styles.css does not contain %q", expected)
		}
	}

	asset, err := os.ReadFile("static/assets/notification-bell.svg")
	if err != nil {
		t.Fatalf("read notification bell SVG: %v", err)
	}
	assetContent := string(asset)
	for _, expected := range []string{`viewBox="0 0 24 24"`, `fill="currentColor"`} {
		if !strings.Contains(assetContent, expected) {
			t.Fatalf("notification bell SVG does not contain %q", expected)
		}
	}
}
