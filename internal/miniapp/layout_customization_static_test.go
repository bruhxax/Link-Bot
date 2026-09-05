package miniapp

import (
	"strings"
	"testing"
)

func TestHomeLayoutCustomizationWiresControlsAndRuntimeStyles(t *testing.T) {
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
		`data-action="admin-layout-more-toggle"`,
		`data-action="admin-add-empty-card"`,
		`data-action="admin-open-layout-style"`,
		`data-action="admin-layout-layer-back"`,
		`data-action="admin-layout-layer-front"`,
		`data-input="admin-layout-style"`,
		`cornerRadius`,
		`textScale`,
		`textOffsetX`,
		`textOffsetY`,
		`empty_card_`,
		`icon("moreHorizontal")`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("app.js does not contain %q", fragment)
		}
	}

	styles := string(stylesRaw)
	for _, fragment := range []string{
		`.admin-layout-add-menu`,
		`.modal__sheet--layout-style`,
		`--runtime-corner-radius`,
		`--runtime-text-scale`,
		`--runtime-layer`,
		`.empty-design-card`,
		`@media (prefers-reduced-motion: reduce)`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("styles.css does not contain %q", fragment)
		}
	}
}

func TestSaveButtonsDoNotRenderDecorativeIcons(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	app := string(appRaw)
	if strings.Contains(app, "button.innerHTML = `${icon(saving ?") {
		t.Fatal("save bar still injects a decorative save icon")
	}
	if !strings.Contains(app, `class="admin-save-bar__save"`) {
		t.Fatal("compact save button class is missing")
	}
}

func TestDashboardLayoutEntryDoesNotAnimateSavedCoordinates(t *testing.T) {
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
		`layout-runtime-pending`,
		`surface.classList.remove("layout-runtime-pending")`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("dashboard mount does not contain %q", fragment)
		}
	}

	styles := string(stylesRaw)
	if !strings.Contains(styles, `#page-dashboard.page--animate`) ||
		!strings.Contains(styles, `@keyframes dashboardPageIn { from { opacity: 0; } to { opacity: 1; } }`) {
		t.Fatal("dashboard page entry must use an opacity-only animation")
	}
}
