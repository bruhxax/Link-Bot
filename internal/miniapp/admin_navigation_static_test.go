package miniapp

import (
	"os"
	"strings"
	"testing"
)

func TestActiveAdminTabReturnsFromSectionToAdminHome(t *testing.T) {
	appJS, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	content := string(appJS)
	for _, expected := range []string{
		`if (action === "close-admin-section") return closeAdminSection();`,
		`function closeAdminSection() {`,
		`state.adminSection = "home";`,
		`state.adminBroadcastConfirmOpen = false;`,
		`if (samePage && nextPage === "admin" && state.adminSection !== "home") return closeAdminSection();`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("app.js does not contain %q", expected)
		}
	}
}
