package miniapp

import (
	"strings"
	"testing"
)

func TestBrowserLayoutIsCenteredWithoutChangingTelegramMiniApp(t *testing.T) {
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
			t.Fatalf("centered browser layout fragment is missing: %q", fragment)
		}
	}

	if strings.Contains(styles, `:root[data-client="telegram"] #app`) ||
		strings.Contains(styles, `:root[data-client="telegram"] .modal`) {
		t.Fatal("browser layout fix must not override the Telegram Mini App")
	}

	if !strings.Contains(styles, ".modal__sheet--thread {\n  margin-inline: auto;\n}") {
		t.Fatal("support thread sheet must split its horizontal margin evenly")
	}
}
