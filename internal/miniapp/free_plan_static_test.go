package miniapp

import (
	"strings"
	"testing"
)

func TestAdminSquadSelectorAndFreeCheckoutStaticContract(t *testing.T) {
	raw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read embedded app.js: %v", err)
	}
	script := string(raw)
	for _, expected := range []string{
		`data-action="admin-select-all-squads"`,
		`data-input="admin-plan-free-once"`,
		`action === "completed"`,
		`Получить бесплатно`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("app.js does not contain %q", expected)
		}
	}
	if strings.Contains(script, `<small>${escapeHtml(squad.uuid)}</small>`) {
		t.Fatal("admin squad selector must not render UUIDs")
	}
}
