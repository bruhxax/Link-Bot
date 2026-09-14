package miniapp

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAdminPanelUsesDedicatedBrowserSurface(t *testing.T) {
	appRaw, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := os.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	handlerRaw, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("read handler.go: %v", err)
	}

	appJS := string(appRaw)
	styles := string(stylesRaw)
	handler := string(handlerRaw)

	for _, fragment := range []string{
		`const adminWebSurface = clientSurface === "browser"`,
		`/^\/admin(?:\/|$)/.test(window.location.pathname)`,
		`document.documentElement.dataset.adminSurface = adminWebSurface ? "web" : "none";`,
		`const BOTTOM_NAV = ["dashboard", "buy", "support", "settings"];`,
		`adminWebSurface ? renderAdminPage() : ""`,
		`function renderAdminWebShell()`,
		`function renderAdminWebOverviewPage()`,
		`function isAdminWebShellActive()`,
	} {
		if !strings.Contains(appJS, fragment) {
			t.Fatalf("browser-only admin fragment is missing: %q", fragment)
		}
	}

	if strings.Contains(appJS, `const BOTTOM_NAV = ["dashboard", "buy", "support", "settings", "admin"]`) {
		t.Fatal("Telegram Mini App navigation must not expose the admin panel")
	}

	for _, fragment := range []string{
		`html[data-admin-surface="web"] body.is-admin-web`,
		`.admin-web-shell`,
		`.admin-web-sidebar`,
		`.admin-web-overview`,
		`@media (max-width: 920px)`,
		`@media (prefers-reduced-motion: reduce)`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("admin web stylesheet fragment is missing: %q", fragment)
		}
	}

	for _, fragment := range []string{
		`mux.HandleFunc("/admin", h.serveAdminIndex)`,
		`mux.HandleFunc("/admin/", h.serveAdminIndex)`,
		`func (h *Handler) serveAdminIndex`,
	} {
		if !strings.Contains(handler, fragment) {
			t.Fatalf("admin web route fragment is missing: %q", fragment)
		}
	}
}

func TestAdminWebEntryRoute(t *testing.T) {
	staticFS, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		t.Fatalf("open embedded static files: %v", err)
	}
	handler := &Handler{staticFS: staticFS, assetVersion: "test"}

	redirect := httptest.NewRecorder()
	handler.serveAdminIndex(redirect, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if redirect.Code != http.StatusMovedPermanently || redirect.Header().Get("Location") != "/admin/" {
		t.Fatalf("unexpected /admin redirect: status=%d location=%q", redirect.Code, redirect.Header().Get("Location"))
	}

	response := httptest.NewRecorder()
	handler.serveAdminIndex(response, httptest.NewRequest(http.MethodGet, "/admin/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected /admin/ status: %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `id="app"`) {
		t.Fatal("admin entry does not serve the application shell")
	}
}
