package miniapp

import (
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

func TestServeIndexInjectsAssetVersion(t *testing.T) {
	handler := &Handler{
		staticFS: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(`<script src="/mini-app/app.js?v=__ASSET_VERSION__"></script>`)},
		},
		assetVersion: "abc123",
	}
	request := httptest.NewRequest("GET", "/mini-app/", nil)
	response := httptest.NewRecorder()

	handler.serveIndex(response, request)

	if response.Code != 200 {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, "app.js?v=abc123") || strings.Contains(body, "__ASSET_VERSION__") {
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
