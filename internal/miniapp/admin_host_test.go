package miniapp

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/database"
)

func adminHostTestHandler(t *testing.T) *Handler {
	t.Helper()
	staticFS, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		t.Fatal(err)
	}
	return &Handler{staticFS: staticFS, adminBaseURL: "https://admin.example.com", publicBaseURL: "https://example.com", assetVersion: "test"}
}

func TestAdminEntryHostAndLegacyRedirect(t *testing.T) {
	h := adminHostTestHandler(t)
	for _, target := range []string{"https://example.com/admin?section=finance&token=private", "https://example.com/mini-app/?page=admin&section=finance&token=private"} {
		req := httptest.NewRequest("GET", target, nil)
		res := httptest.NewRecorder()
		if req.URL.Path == "/admin" {
			h.serveAdminEntry(res, req)
		} else {
			h.serveIndex(res, req)
		}
		if res.Code != 302 || res.Header().Get("Location") != "https://admin.example.com/?page=admin&section=finance" {
			t.Fatalf("redirect: %d %s", res.Code, res.Header().Get("Location"))
		}
	}
	res := httptest.NewRecorder()
	h.serveRoot(res, httptest.NewRequest("GET", "https://admin.example.com/", nil))
	if res.Code != 200 || !strings.Contains(res.Body.String(), `name="admin-entry" content="on"`) || strings.Contains(res.Body.String(), "__ADMIN_") {
		t.Fatal("admin root did not render admin entry")
	}
	res = httptest.NewRecorder()
	h.serveIndex(res, httptest.NewRequest("GET", "https://example.com/mini-app/?cabinet=1", nil))
	if !strings.Contains(res.Body.String(), `name="admin-entry" content="off"`) {
		t.Fatal("customer cabinet became admin entry")
	}
	h.adminBaseURL = ""
	res = httptest.NewRecorder()
	h.serveIndex(res, httptest.NewRequest("GET", "https://example.com/mini-app/?page=admin", nil))
	if res.Code != 200 {
		t.Fatal("empty setting broke legacy admin")
	}
}

func TestAdminHostDoesNotTrustForwardedHost(t *testing.T) {
	h := adminHostTestHandler(t)
	req := httptest.NewRequest("POST", "https://example.com/api/mini-app/admin/settings/update", strings.NewReader(`{}`))
	req.Header.Set("X-Forwarded-Host", "admin.example.com")
	res := httptest.NewRecorder()
	h.withSession(func(http.ResponseWriter, *http.Request, *session, *database.Customer) {
		t.Fatal("wrong host reached admin handler")
	})(res, req)
	if res.Code != 403 || !strings.Contains(res.Body.String(), "admin_host_required") {
		t.Fatal(res.Code, res.Body.String())
	}
	for _, host := range []string{"ADMIN.EXAMPLE.COM", "admin.example.com.attacker.test", "example.com", "admin.example.com:8080"} {
		req.Host = host
		if h.isAdminHost(req) != (host == "ADMIN.EXAMPLE.COM") {
			t.Fatalf("incorrect host match %s", host)
		}
	}
}

func TestAdminHostRejectsCustomerSession(t *testing.T) {
	initMiniAppTestConfig()
	h := adminHostTestHandler(t)
	h.administratorRepository = &testAdministratorLookup{}
	login, err := createTelegramBrowserSessionData(telegramUser{ID: 22}, config.TelegramToken(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "https://admin.example.com/api/mini-app/bootstrap", strings.NewReader(`{}`))
	req.Header.Set("X-Telegram-Login-Data", login)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.withSession(func(http.ResponseWriter, *http.Request, *session, *database.Customer) {
		t.Fatal("ordinary user entered admin host")
	})(res, req)
	if res.Code != 403 {
		t.Fatalf("customer status %d: %s", res.Code, res.Body.String())
	}
}

func TestAdminEntryMethodsAndDefaultShortcut(t *testing.T) {
	h := adminHostTestHandler(t)
	for _, path := range []string{"/", "/admin", "/mini-app/"} {
		res := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "https://admin.example.com"+path, nil)
		if path == "/" {
			h.serveRoot(res, req)
		} else if path == "/admin" {
			h.serveAdminEntry(res, req)
		} else {
			h.serveIndex(res, req)
		}
		if res.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s returned %d", path, res.Code)
		}
	}
	res := httptest.NewRecorder()
	h.serveRoot(res, httptest.NewRequest("HEAD", "https://admin.example.com/", nil))
	if res.Code != 200 || res.Body.Len() != 0 {
		t.Fatal("HEAD returned an HTML body")
	}
	h.adminBaseURL = ""
	res = httptest.NewRecorder()
	h.serveAdminEntry(res, httptest.NewRequest("GET", "https://example.com/admin?section=users", nil))
	if res.Code != 302 || res.Header().Get("Location") != "/mini-app/?page=admin&section=users" {
		t.Fatal("default shortcut did not redirect to the legacy admin entry")
	}
}

func TestAdminPWAScopeMatchesRootEntry(t *testing.T) {
	h := adminHostTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	for _, host := range []string{"admin.example.com", "example.com"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest("GET", "https://"+host+"/mini-app/manifest.webmanifest", nil))
		var manifest pwaManifest
		if err := json.Unmarshal(res.Body.Bytes(), &manifest); err != nil {
			t.Fatal(err)
		}
		want := "/mini-app/"
		if host == "admin.example.com" {
			want = "/"
		}
		if manifest.Scope != want || manifest.StartURL != want {
			t.Fatalf("%s: scope %s, start %s", host, manifest.Scope, manifest.StartURL)
		}
		res = httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest("GET", "https://"+host+"/mini-app/sw.js", nil))
		if res.Code != 200 || res.Header().Get("Service-Worker-Allowed") != want {
			t.Fatalf("%s: worker status %d scope %s", host, res.Code, res.Header().Get("Service-Worker-Allowed"))
		}
	}
}
