package miniapp

import (
	"os"
	"strings"
	"testing"
)

func TestSubscriptionSwitcherMenuUsesNotificationGlassBackground(t *testing.T) {
	styles, err := os.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	content := string(styles)
	start := strings.Index(content, ".subscription-switcher__menu {")
	if start < 0 {
		t.Fatal("subscription switcher menu styles are missing")
	}
	end := strings.Index(content[start:], "\n}")
	if end < 0 {
		t.Fatal("subscription switcher menu style block is incomplete")
	}
	menuStyles := content[start : start+end]
	for _, expected := range []string{
		"background: color-mix(in srgb, var(--bg) 76%, transparent)",
		"backdrop-filter: blur(16px) saturate(1.08)",
		"-webkit-backdrop-filter: blur(16px) saturate(1.08)",
	} {
		if !strings.Contains(menuStyles, expected) {
			t.Fatalf("subscription switcher menu styles do not contain %q", expected)
		}
	}
	if strings.Contains(menuStyles, "background: var(--surface-strong)") {
		t.Fatal("subscription switcher menu still uses an opaque background")
	}
}
