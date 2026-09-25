package miniapp

import (
	"strings"
	"testing"
)

func TestBlockedUserHasDedicatedStateScreen(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	app := string(appRaw)
	for _, expected := range []string{
		`error?.code === "user_blocked"`,
		`renderStateScreen("blocked", "", state.blocked)`,
		`localizedText("Доступ ограничен", "Access restricted"`,
		`localizedText("Причина:", "Reason:"`,
	} {
		if !strings.Contains(app, expected) {
			t.Fatalf("blocked user screen fragment is missing: %q", expected)
		}
	}
}
