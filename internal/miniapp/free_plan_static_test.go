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
		`aria-pressed="${everySelected}"`,
		`syncAdminSquadSelector(inputKey, next)`,
		`internalSquadsConfigured = true`,
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
	toolbarButton := strings.Index(script, `<div class="admin-squads__toolbar"><button`)
	toolbarCounter := strings.Index(script, `<span>${selectedCount} из ${squads.length}</span>`)
	if toolbarButton < 0 || toolbarCounter < toolbarButton {
		t.Fatal("admin squad selector must render the select-all button on the left and counter on the right")
	}
}
