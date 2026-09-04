package miniapp

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticAssetVersionChangesWithContent(t *testing.T) {
	first := fstest.MapFS{
		"app.js":     &fstest.MapFile{Data: []byte("first")},
		"styles.css": &fstest.MapFile{Data: []byte("styles")},
	}
	second := fstest.MapFS{
		"app.js":     &fstest.MapFile{Data: []byte("second")},
		"styles.css": &fstest.MapFile{Data: []byte("styles")},
	}

	firstVersion, err := staticAssetVersion(first)
	if err != nil {
		t.Fatalf("staticAssetVersion(first) error = %v", err)
	}
	secondVersion, err := staticAssetVersion(second)
	if err != nil {
		t.Fatalf("staticAssetVersion(second) error = %v", err)
	}
	if firstVersion == secondVersion {
		t.Fatalf("asset version did not change: %q", firstVersion)
	}
}

func TestServeManifestIsDynamicAndNotCached(t *testing.T) {
	handler := &Handler{assetVersion: "abc123"}
	request := httptest.NewRequest("GET", "/mini-app/manifest.webmanifest", nil)
	response := httptest.NewRecorder()

	handler.serveManifest(response, request)

	if response.Code != 200 {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var manifest pwaManifest
	if err := json.Unmarshal(response.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.Name != "Link-Bot" || manifest.ID != "/mini-app/" || len(manifest.Icons) != 2 {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
}

func TestTelegramOIDCLoginKeepsRedirectCompatibility(t *testing.T) {
	appJS, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read embedded app.js: %v", err)
	}

	source := string(appJS)
	for _, required := range []string{
		"https://oauth.telegram.org/js/telegram-login.js?6",
		"redirect_uri: `${window.location.origin}${window.location.pathname}`",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("app.js is missing Telegram OIDC redirect compatibility fragment %q", required)
		}
	}
}

func TestServeIndexInjectsAssetVersion(t *testing.T) {
	handler := &Handler{
		staticFS: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(`<title>__PAGE_TITLE__</title><meta name="description" content="__PAGE_DESCRIPTION__"><meta name="application-name" content="__PWA_NAME__"><link rel="icon" href="__FAVICON_URL__"><link rel="manifest" href="/mini-app/manifest.webmanifest?v=__PWA_BRAND_VERSION__"><script src="/mini-app/app.js?v=__ASSET_VERSION__"></script>`)},
		},
		assetVersion: "abc123",
	}
	request := httptest.NewRequest("GET", "/mini-app/", nil)
	response := httptest.NewRecorder()

	handler.serveIndex(response, request)

	if response.Code != 200 {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, "app.js?v=abc123") || !strings.Contains(body, "<title>Link-Bot</title>") || !strings.Contains(body, `application-name" content="Link-Bot`) || !strings.Contains(body, "manifest.webmanifest?v=") || !strings.Contains(body, "brand-mark.png?v=abc123") || strings.Contains(body, "__ASSET_VERSION__") || strings.Contains(body, "__PAGE_TITLE__") || strings.Contains(body, "__PWA_") {
		t.Fatalf("index body has stale asset version: %q", body)
	}
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}
}

func TestStaticHeadersOnlyMakeCurrentVersionImmutable(t *testing.T) {
	currentRequest := httptest.NewRequest("GET", "/mini-app/app.js?v=current", nil)
	currentResponse := httptest.NewRecorder()
	setStaticHeaders(currentResponse, currentRequest, "current")
	if got := currentResponse.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("current Cache-Control = %q", got)
	}

	staleRequest := httptest.NewRequest("GET", "/mini-app/app.js?v=stale", nil)
	staleResponse := httptest.NewRecorder()
	setStaticHeaders(staleResponse, staleRequest, "current")
	if got := staleResponse.Header().Get("Cache-Control"); got != "no-cache, max-age=0, must-revalidate" {
		t.Fatalf("stale Cache-Control = %q", got)
	}
}
