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
		`data-setting-path="appearance.design"`,
		`document.documentElement.dataset.design = appearance.design === "glass" ? "glass" : "classic";`,
	} {
		if !strings.Contains(appJS, fragment) {
			t.Errorf("app.js is missing glass design contract %q", fragment)
		}
	}

	for _, fragment := range []string{
		`:root[data-design="glass"]`,
		`--glass-blur:`,
		`.pricing-card`,
		`.bottom-nav`,
		`.admin-editor__section`,
		`:focus-visible`,
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
}
