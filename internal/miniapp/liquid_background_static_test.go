package miniapp

import (
	"strings"
	"testing"
)

func TestLiquidBackgroundControlsAndLayersAreBundled(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	indexRaw, err := embeddedStatic.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	liquidRaw, err := embeddedStatic.ReadFile("static/liquid-background.js")
	if err != nil {
		t.Fatalf("read liquid-background.js: %v", err)
	}

	appJS := string(appRaw)
	for _, fragment := range []string{
		`["liquid1", "Жидкое стекло 1", "Яркий перелив с зерном"]`,
		`["liquid2", "Жидкое стекло 2", "Тёмный мягкий перелив"]`,
		`appearance.backgroundMotion.${mode}.dimming`,
		`appearance.backgroundMotion.${mode}.speed`,
		`ADMIN_BACKGROUND_COLOR_FIELDS`,
		`--background-animation-duration`,
	} {
		if !strings.Contains(appJS, fragment) {
			t.Fatalf("app.js does not contain %q", fragment)
		}
	}

	styles := string(stylesRaw)
	for _, fragment := range []string{
		`.bg-media__liquid-canvas`,
		`.bg-media__shade`,
		`:root[data-background="liquid2"] .bg-media__liquid-grain`,
		`.admin-background-settings__ranges`,
		`.admin-color-field:focus-within`,
		`.admin-range-field input[type="range"]:focus-visible`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("styles.css does not contain %q", fragment)
		}
	}

	indexHTML := string(indexRaw)
	for _, fragment := range []string{`<canvas class="bg-media__liquid-canvas"></canvas>`, `<div class="bg-media__shade"></div>`, `liquid-background.js?v=__ASSET_VERSION__`} {
		if !strings.Contains(indexHTML, fragment) {
			t.Fatalf("index.html does not contain %q", fragment)
		}
	}

	liquidJS := string(liquidRaw)
	for _, fragment := range []string{`float fbm`, `uniform float uVariant`, `window.__linkBotLiquid`, `setConfig`} {
		if !strings.Contains(liquidJS, fragment) {
			t.Fatalf("liquid-background.js does not contain %q", fragment)
		}
	}
	if strings.Contains(styles, `.bg-media__liquid-field--2`) {
		t.Fatal("legacy blurred liquid fields are still bundled")
	}
}
