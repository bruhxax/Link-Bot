package miniapp

import (
	"strings"
	"testing"
)

func TestClassicInterfaceKeepsScrollablePageContainer(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}

	appJS := string(appRaw)
	stylesCSS := string(stylesRaw)
	if !strings.Contains(appJS, `<div class="page-scroll">${renderPages()}</div>`) {
		t.Error("page-scroll must remain a direct child of app-shell")
	}
	for _, fragment := range []string{
		`.page-scroll {`,
		`overflow-y: auto;`,
		`-webkit-overflow-scrolling: touch;`,
	} {
		if !strings.Contains(stylesCSS, fragment) {
			t.Errorf("styles.css is missing scroll contract %q", fragment)
		}
	}
}

func TestAlternateInterfaceIsNotExposed(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	indexRaw, err := embeddedStatic.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}

	for name, content := range map[string]string{
		"app.js":     string(appRaw),
		"index.html": string(indexRaw),
	} {
		for _, fragment := range []string{
			"appearance.design",
			"data-design",
			"renderAdminDesignPage",
			"useGlassInterface",
			"switchInterfaceDesign",
			"glass.css",
		} {
			if strings.Contains(content, fragment) {
				t.Errorf("%s still exposes alternate interface marker %q", name, fragment)
			}
		}
	}
	if _, err := embeddedStatic.ReadFile("static/glass.css"); err == nil {
		t.Error("alternate interface stylesheet must not be embedded")
	}
}
