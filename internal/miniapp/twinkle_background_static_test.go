package miniapp

import (
	"strings"
	"testing"
)

func TestTwinkleBackgroundControlsAndRuntimeAreBundled(t *testing.T) {
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
	twinkleRaw, err := embeddedStatic.ReadFile("static/twinkle-background.js")
	if err != nil {
		t.Fatalf("read twinkle-background.js: %v", err)
	}

	appJS := string(appRaw)
	for _, fragment := range []string{
		`["twinkle", "Мерцающие звёзды", "Маленькие светящиеся круги"]`,
		`appearance.colors.twinkleBackground`,
		`appearance.colors.twinkleStar`,
		`window.__linkBotTwinkle?.setConfig`,
		`window.__linkBotTwinkle?.setPaused`,
		`reducedBackgroundMotion`,
	} {
		if !strings.Contains(appJS, fragment) {
			t.Fatalf("app.js does not contain %q", fragment)
		}
	}

	styles := string(stylesRaw)
	for _, fragment := range []string{
		`.bg-media__twinkle-canvas`,
		`:root[data-background="twinkle"] .bg-media__twinkle`,
		`[data-preview-background="twinkle"]`,
		`drop-shadow(0 0 2px`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("styles.css does not contain %q", fragment)
		}
	}

	indexHTML := string(indexRaw)
	for _, fragment := range []string{
		`<canvas class="bg-media__twinkle-canvas"></canvas>`,
		`twinkle-background.js?v=__ASSET_VERSION__`,
	} {
		if !strings.Contains(indexHTML, fragment) {
			t.Fatalf("index.html does not contain %q", fragment)
		}
	}

	twinkleJS := string(twinkleRaw)
	for _, fragment := range []string{`requestAnimationFrame(draw)`, `targetStarCount()`, `context.arc`, `context.shadowBlur`, `setConfig`, `setPaused`, `window.__linkBotTwinkle`} {
		if !strings.Contains(twinkleJS, fragment) {
			t.Fatalf("twinkle-background.js does not contain %q", fragment)
		}
	}
	if strings.Contains(twinkleJS, "setInterval(") {
		t.Fatal("twinkle background must use the animation frame loop instead of a permanent interval")
	}
}
