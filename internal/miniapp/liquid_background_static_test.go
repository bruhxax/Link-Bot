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

	appJS := string(appRaw)
	for _, fragment := range []string{
		`["liquid1", "Жидкое стекло 1", "Яркий перелив с зерном"]`,
		`["liquid2", "Жидкое стекло 2", "Тёмный мягкий перелив"]`,
		`appearance.liquid.${mode}.dimming`,
		`appearance.liquid.${mode}.speed`,
		`--liquid-duration`,
	} {
		if !strings.Contains(appJS, fragment) {
			t.Fatalf("app.js does not contain %q", fragment)
		}
	}

	styles := string(stylesRaw)
	for _, fragment := range []string{
		`.bg-media__liquid-field--2`,
		`@keyframes runtime-liquid-drift-a`,
		`:root[data-background="liquid2"] .bg-media__liquid-grain`,
		`@media (prefers-reduced-motion: reduce)`,
		`.admin-range-field input[type="range"]:focus-visible`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("styles.css does not contain %q", fragment)
		}
	}

	if !strings.Contains(string(indexRaw), `<div class="bg-media__liquid">`) {
		t.Fatal("index.html does not contain the liquid background layer")
	}
}
