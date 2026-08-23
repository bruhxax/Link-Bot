package miniapp

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSubscriptionSetupStaysInsideMiniApp(t *testing.T) {
	appJS, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}

	source := string(appJS)
	for _, expected := range []string{
		`data-value="setup"`,
		`if (action === "go-page") return setPage(value);`,
		`target.pathname = "/mini-app/open-app";`,
		`target.hash = fragment.toString();`,
		`data-input="setup-platform"`,
		`data-action="toggle-setup-platform"`,
		`data-action="select-setup-platform"`,
		`data-action="select-setup-app"`,
		`data-action="open-setup-app"`,
		`class="setup-qr-inline"`,
		`data-action="copy-access"`,
		`renderQRCodeSVG(subscriptionLink`,
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("app.js does not contain %q", expected)
		}
	}
	if strings.Contains(source, `value === "setup" ? openSubscriptionAccess()`) {
		t.Fatal("setup navigation still bypasses the in-app page")
	}
}

func TestSubscriptionSetupUsesCompactThemedTimeline(t *testing.T) {
	appJS, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	styles, err := os.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}

	appSource := string(appJS)
	for _, unexpected := range []string{
		`localizedText("ПОДКЛЮЧЕНИЕ"`,
		`localizedText("Отсканируйте его в VPN-клиенте на другом устройстве."`,
		`class="setup-guide__app-label"`,
	} {
		if strings.Contains(appSource, unexpected) {
			t.Fatalf("app.js still contains removed setup copy %q", unexpected)
		}
	}

	styleSource := string(styles)
	for _, expected := range []string{
		`.setup-platform-menu`,
		`@media (hover: hover) and (pointer: fine)`,
		`.setup-step::before`,
		`border-radius: 50%;`,
		`background: color-mix(in srgb, var(--surface-strong) 84%, var(--accent));`,
	} {
		if !strings.Contains(styleSource, expected) {
			t.Fatalf("styles.css does not contain %q", expected)
		}
	}
}

func TestSubscriptionSetupCatalogContainsSupportedPlatformsAndSafeSchemes(t *testing.T) {
	catalog, err := os.ReadFile("static/setup-apps.js")
	if err != nil {
		t.Fatalf("read setup-apps.js: %v", err)
	}
	source := string(catalog)
	for _, expected := range []string{
		`id: "ios"`,
		`id: "android"`,
		`id: "macos"`,
		`id: "windows"`,
		`id: "android-tv"`,
		`id: "apple-tv"`,
		`scheme: "happ://add/"`,
		`scheme: "incy://import/"`,
		`https://apps.apple.com/ru/app/incy/id6756943388`,
		`if (!/^https?:$/.test(parsed.protocol))`,
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("setup-apps.js does not contain %q", expected)
		}
	}
	for _, unexpected := range []string{`id: "linux"`, `id: "stash"`, `id: "flclashx"`, `id: "koala-clash"`} {
		if strings.Contains(source, unexpected) {
			t.Fatalf("setup-apps.js still contains unsupported catalog item %q", unexpected)
		}
	}
}

func TestSubscriptionAppOpenerClearsSensitiveFragment(t *testing.T) {
	openerJS, err := os.ReadFile("static/open-app.js")
	if err != nil {
		t.Fatalf("read open-app.js: %v", err)
	}
	source := string(openerJS)
	for _, expected := range []string{
		`window.history.replaceState(null, "", window.location.pathname);`,
		`window.location.assign(clientURL);`,
		`window.setTimeout(launchClient, 120);`,
		`rel="noopener noreferrer"`,
		`copyText(subscription, copy.copied)`,
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("open-app.js does not contain %q", expected)
		}
	}
}

func TestSubscriptionAppOpenerFillsViewport(t *testing.T) {
	styles, err := os.ReadFile("static/open-app.css")
	if err != nil {
		t.Fatalf("read open-app.css: %v", err)
	}
	source := string(styles)
	for _, expected := range []string{
		"html {\n  height: 100%;\n  background: var(--bg);",
		"min-height: 100dvh;",
		"place-items: center;",
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("open-app.css does not contain %q", expected)
		}
	}
}

func TestServeAppOpenerInjectsAssetVersion(t *testing.T) {
	handler := &Handler{
		staticFS: fstest.MapFS{
			"open-app.html": &fstest.MapFile{Data: []byte(`<script src="/mini-app/open-app.js?v=__ASSET_VERSION__"></script>`)},
		},
		assetVersion: "setup123",
	}
	request := httptest.NewRequest("GET", "/mini-app/open-app", nil)
	response := httptest.NewRecorder()

	handler.serveAppOpener(response, request)

	if response.Code != 200 {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, "open-app.js?v=setup123") || strings.Contains(body, "__ASSET_VERSION__") {
		t.Fatalf("opener body has stale asset version: %q", body)
	}
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}
}
