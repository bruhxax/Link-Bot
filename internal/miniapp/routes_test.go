package miniapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplicationRoutesServeDeepLinksAndRejectUnknownPaths(t *testing.T) {
	h := adminHostTestHandler(t)
	h.cabinetBaseURL = "https://my.example.com"
	mux := http.NewServeMux()
	h.Register(mux)
	for route := range cabinetRoutes {
		for _, target := range []string{"https://my.example.com/" + route, "https://example.com/cabinet/" + route} {
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, target, nil))
			if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `name="admin-entry" content="off"`) {
				t.Fatalf("%s: %d", target, res.Code)
			}
		}
	}
	for section := range adminSections {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "https://admin.example.com/"+section, nil))
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `name="admin-entry" content="on"`) {
			t.Fatalf("admin section %s: %d", section, res.Code)
		}
	}
	for _, target := range []string{"https://my.example.com/unknown", "https://admin.example.com/unknown", "https://example.com/cabinet/unknown", "https://example.com/api/unknown", "https://example.com/admin/unknown"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, target, nil))
		if res.Code != http.StatusNotFound {
			t.Fatalf("unknown route %s: %d", target, res.Code)
		}
	}
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "https://my.example.com/pages/help-guide", nil))
	if res.Code != http.StatusOK {
		t.Fatal("custom content deep link unavailable")
	}
}

func TestCleanAdminShortcutDoesNotForwardCredentials(t *testing.T) {
	h := adminHostTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "https://example.com/admin/content?token=private&redirect=https://attacker.example", nil))
	if res.Code != http.StatusFound || res.Header().Get("Location") != "https://admin.example.com/content" {
		t.Fatalf("redirect = %d %s", res.Code, res.Header().Get("Location"))
	}
}

func TestAdminSignOutAndSignInUseCabinetOrigin(t *testing.T) {
	h := adminHostTestHandler(t)
	h.cabinetBaseURL = "https://my.example.com"
	mux := http.NewServeMux()
	h.Register(mux)
	for _, page := range []string{"login", "logout"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "https://admin.example.com/"+page+"?token=private", nil))
		if res.Code != http.StatusFound || res.Header().Get("Location") != "https://my.example.com/"+page {
			t.Fatalf("%s redirects to %s", page, res.Header().Get("Location"))
		}
	}
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodHead, "https://my.example.com/dashboard", nil))
	if res.Code != http.StatusOK || res.Body.Len() != 0 {
		t.Fatal("HEAD route returned a body")
	}
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "https://my.example.com/dashboard", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatal("application route accepts writes")
	}
}
