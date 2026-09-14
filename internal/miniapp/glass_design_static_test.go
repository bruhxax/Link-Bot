package miniapp

import (
	"strings"
	"testing"
)

func TestGlassDesignStaticContract(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	glassRaw, err := embeddedStatic.ReadFile("static/glass.css")
	if err != nil {
		t.Fatalf("read glass.css: %v", err)
	}
	indexRaw, err := embeddedStatic.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}

	appJS := string(appRaw)
	glassCSS := string(glassRaw)
	indexHTML := string(indexRaw)

	for _, fragment := range []string{
		`appearance: { design: "classic"`,
		`state.adminSection === "design"`,
		`function renderAdminDesignPage()`,
		`function useGlassInterface()`,
		`function renderGlassNavigation()`,
		`function renderGlassAdminNavigation()`,
		`function renderGlassTopbar()`,
		`function renderGlassDashboardPage()`,
		`function renderGlassProfileItem(item)`,
		`function switchInterfaceDesign(nextDesign)`,
		`document.startViewTransition`,
		`data-setting-path="appearance.design"`,
		`document.documentElement.dataset.design = isGlassDesign() ? "glass" : "classic";`,
	} {
		if !strings.Contains(appJS, fragment) {
			t.Errorf("app.js is missing glass design contract %q", fragment)
		}
	}

	for _, fragment := range []string{
		`:root[data-design="glass"]`,
		`--next-sidebar-width:`,
		`.app-shell--glass`,
		`.glass-rail`,
		`.glass-workspace`,
		`.glass-mobile-nav`,
		`.glass-admin-nav-item`,
		`.glass-access-card`,
		`.glass-dashboard__layout`,
		`.glass-command-panel`,
		`.glass-command-grid`,
		`.glass-profile__groups`,
		`.glass-admin-home`,
		`.pricing-card`,
		`.admin-editor__section`,
		`::view-transition-new(root)`,
		`.design-switching-fallback #app`,
		`:focus-visible`,
		`@media (max-width: 899px)`,
		`@media (prefers-reduced-motion: reduce)`,
	} {
		if !strings.Contains(glassCSS, fragment) {
			t.Errorf("glass.css is missing %q", fragment)
		}
	}

	if !strings.Contains(indexHTML, `data-design="classic"`) {
		t.Error("index.html must start with the classic design to prevent an unstyled flash")
	}
	if !strings.Contains(indexHTML, `/mini-app/glass.css?v=__ASSET_VERSION__`) {
		t.Error("index.html does not load the versioned glass stylesheet")
	}
	if strings.Contains(appJS, `class="${glassInterface ? "glass-workspace" : "app-workspace"}"`) {
		t.Error("classic design must not be wrapped in an unconstrained workspace that disables page scrolling")
	}
	if !strings.Contains(appJS, ": `<div class=\"page-scroll\">${renderPages()}</div>`}") {
		t.Error("classic design must keep page-scroll as a direct flex child of app-shell")
	}
	if strings.Contains(glassCSS, "backdrop-filter: blur") {
		t.Error("alternate design must not use expensive backdrop blur")
	}
	if strings.Contains(glassCSS, "infinite") {
		t.Error("alternate design must not use continuous animations")
	}
}
