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

func TestEditorDockUsesIconOnlySaveAndCancelActions(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	app := string(appRaw)
	for _, fragment := range []string{
		`bottom-nav--editor ${entering ?`,
		`admin-save-bar ${className}`,
		`class="admin-save-bar__save"`,
		`aria-label="${escapeAttribute(saveLabel)}"`,
		`icon("check")`,
		`"admin-cancel-settings"`,
		`renderEditorScreenSwitches(dockModeChanged)`,
		`data-value="dashboard"`,
		`data-value="settings"`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("editor dock does not contain %q", fragment)
		}
	}
	if strings.Contains(app, "button.innerHTML = `<span>") {
		t.Fatal("DOM synchronization restores a text save button")
	}
}

func TestEditorKeepsConfiguredBackgroundAndUsesViewportSizedMenu(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	app, styles := string(appRaw), string(stylesRaw)
	for _, fragment := range []string{
		`</nav>${renderAdminLayoutAddMenu()}`,
		`id="admin-editor-more-menu"`,
		`const paused = document.hidden || state.adminPlanEditing;`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("editor script does not contain %q", fragment)
		}
	}
	for _, fragment := range []string{
		`.app-shell > .admin-layout-add-menu {`,
		`width: min(320px, calc(100vw - 24px));`,
		`background: var(--surface-strong);`,
		`body.is-layout-editing .app-shell::before`,
		`--studio-grid-pattern:`,
		`body.is-layout-editing .layout-editor-grid { background: none; }`,
		`padding-bottom: calc(88px + var(--safe-bottom, 0px));`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("editor styles do not contain %q", fragment)
		}
	}
	if strings.Contains(styles, `body.is-layout-editing :is(.bg-media__canvas`) {
		t.Fatal("editor still hides the configured background")
	}
	if strings.Contains(styles, `radial-gradient(circle, color-mix(in srgb, var(--icon-color) 9%`) {
		t.Fatal("editor grid still contains decorative dots")
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

func TestDashboardRuntimeAndEditorUseTheSamePositioningSurface(t *testing.T) {
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	styles := string(stylesRaw)

	start := strings.Index(styles, `.page.active.layout-runtime-surface {`)
	if start < 0 {
		t.Fatal("dashboard runtime positioning surface styles are missing")
	}
	end := strings.Index(styles[start:], "\n}")
	if end < 0 {
		t.Fatal("dashboard runtime positioning surface style block is incomplete")
	}
	rule := styles[start : start+end]
	for _, fragment := range []string{`position: relative`, `isolation: isolate`} {
		if !strings.Contains(rule, fragment) {
			t.Fatalf("dashboard runtime positioning surface does not contain %q", fragment)
		}
	}
}

func TestSubscriptionSwitcherStaysAboveCustomDashboardLayers(t *testing.T) {
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	styles := string(stylesRaw)

	start := strings.Index(styles, `.subscription-switcher {`)
	if start < 0 {
		t.Fatal("subscription switcher styles are missing")
	}
	end := strings.Index(styles[start:], "\n}")
	if end < 0 {
		t.Fatal("subscription switcher style block is incomplete")
	}
	rule := styles[start : start+end]
	if !strings.Contains(rule, `z-index: 200`) {
		t.Fatal("subscription switcher must stay above the maximum custom dashboard layer")
	}
}

func TestNewDashboardElementsReceiveAVisibleInitialPosition(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	app := string(appRaw)

	for _, fragment := range []string{
		`function positionNewDashboardElement(item, cascade = 0)`,
		`app.querySelector(".admin-save-bar--layout-editor")`,
		`positionNewDashboardElement(item, sequence - 1)`,
		`localizedText("Пустая карточка добавлена"`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("new dashboard element placement does not contain %q", fragment)
		}
	}
	if strings.Count(app, `positionNewDashboardElement(item`) < 4 {
		t.Fatal("empty, promo and notification dashboard elements must all receive an initial visible position")
	}
}
