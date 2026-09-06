package miniapp

import (
	"strings"
	"testing"
)

func TestMorphicBackgroundControlsAndRuntimeAreBundled(t *testing.T) {
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
	morphicRaw, err := embeddedStatic.ReadFile("static/morphic-background.js")
	if err != nil {
		t.Fatalf("read morphic-background.js: %v", err)
	}

	appJS := string(appRaw)
	for _, fragment := range []string{
		`["morphic", "Морфинг", "Мягкие поднимающиеся капли"]`,
		`appearance.colors.morphicBackground`,
		`appearance.colors.morphicBall`,
		`window.__linkBotMorphic?.setConfig`,
		`window.__linkBotMorphic?.setPaused`,
		`reducedBackgroundMotion`,
	} {
		if !strings.Contains(appJS, fragment) {
			t.Fatalf("app.js does not contain %q", fragment)
		}
	}

	styles := string(stylesRaw)
	for _, fragment := range []string{
		`.bg-media__morphic-particles`,
		`.bg-media__morphic-particle`,
		`:root[data-background="morphic"] .bg-media__morphic`,
		`[data-preview-background="morphic"]`,
		`:root[data-performance="low"] .bg-media__morphic-particles`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("styles.css does not contain %q", fragment)
		}
	}

	indexHTML := string(indexRaw)
	for _, fragment := range []string{
		`<filter id="morphic-goo"`,
		`<div class="bg-media__morphic-particles"></div>`,
		`morphic-background.js?v=__ASSET_VERSION__`,
	} {
		if !strings.Contains(indexHTML, fragment) {
			t.Fatalf("index.html does not contain %q", fragment)
		}
	}

	morphicJS := string(morphicRaw)
	for _, fragment := range []string{`requestAnimationFrame(frame)`, `particleLimit()`, `setConfig`, `setPaused`, `window.__linkBotMorphic`} {
		if !strings.Contains(morphicJS, fragment) {
			t.Fatalf("morphic-background.js does not contain %q", fragment)
		}
	}
	if strings.Contains(morphicJS, "setInterval(") {
		t.Fatal("morphic background must use the animation frame loop instead of a permanent interval")
	}
}
