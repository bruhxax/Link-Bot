package miniapp

import (
	"strings"
	"testing"
)

func TestBrowserAndTelegramLayoutSurfaces(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read embedded app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read embedded styles.css: %v", err)
	}

	appJS := string(appRaw)
	styles := string(stylesRaw)
	for _, fragment := range []string{
		`const clientSurface = String(tg?.initData || "").trim() ? "telegram" : "browser";`,
		`const paymentReturnTarget = clientSurface === "telegram" ? "telegram" : "web";`,
		`const standaloneWebApp = clientSurface === "browser"`,
		`document.documentElement.dataset.client = clientSurface;`,
		`document.documentElement.dataset.displayMode = standaloneWebApp ? "standalone" : "browser";`,
		`returnTarget: paymentReturnTarget,`,
	} {
		if !strings.Contains(appJS, fragment) {
			t.Fatalf("browser surface detection fragment is missing: %q", fragment)
		}
	}

	for _, fragment := range []string{
		`:root[data-client="browser"] #app`,
		`width: min(100%, 430px);`,
		`@media (min-width: 760px)`,
		`:root[data-client="browser"] .app-shell`,
		`grid-template-columns: clamp(224px, 20vw, 292px) minmax(0, 1fr);`,
		`:root[data-client="browser"] .desktop-sidebar`,
		`:root[data-client="browser"] .bottom-nav:not(.bottom-nav--editor) { display: none; }`,
		`:root[data-client="browser"][data-display-mode="standalone"] .page-scroll`,
		`padding-top: calc(12px + var(--safe-top));`,
		`height: 100vh;`,
		`:root[data-client="browser"] .modal`,
		`align-items: center;`,
		`justify-content: center;`,
		`:root[data-client="browser"] .modal__sheet`,
		`max-width: 430px;`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("browser layout fragment is missing: %q", fragment)
		}
	}
	if !strings.Contains(appJS, `${renderDesktopSidebar()}`) || !strings.Contains(appJS, `function renderDesktopSidebar()`) {
		t.Fatal("wide browser navigation must be rendered in the app shell")
	}

	if strings.Contains(styles, `:root[data-client="telegram"] #app`) ||
		strings.Contains(styles, `:root[data-client="telegram"] .modal`) {
		t.Fatal("browser layout fix must not override the Telegram Mini App")
	}

	dashboardCoordinatePlane := `#page-dashboard.page.active {
  width: min(100%, 360px);
  margin-inline: auto;
}`
	if !strings.Contains(styles, dashboardCoordinatePlane) {
		t.Fatal("narrow browsers and Telegram must retain the 360px dashboard coordinate plane")
	}
	for _, fragment := range []string{
		`:root[data-client="browser"] .app-shell:not(.app-shell--layout-editor) #page-dashboard.page.active {`,
		`width: min(100%, 860px);`,
		`grid-template-columns: repeat(2, minmax(0, 1fr));`,
		`.subscription-switcher__create`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("wide browser dashboard layout is missing %q", fragment)
		}
	}
	if !strings.Contains(appJS, `if (state.currentPage === "dashboard" && (!isWideBrowserCabinet() || state.adminLayoutEditing))`) {
		t.Fatal("wide browser dashboard must use its fluid layout while the editor keeps saved coordinates")
	}
	if strings.Contains(appJS, `desktop-page-heading`) || strings.Contains(styles, `.desktop-page-heading`) {
		t.Fatal("the extra desktop page heading must be removed")
	}
	if strings.Contains(styles, `:root[data-client="telegram"] #page-dashboard`) {
		t.Fatal("wide browser layout must not override the Telegram dashboard")
	}

	if !strings.Contains(styles, ".modal__sheet--thread {\n  margin-inline: auto;\n}") {
		t.Fatal("support thread sheet must split its horizontal margin evenly")
	}
}
