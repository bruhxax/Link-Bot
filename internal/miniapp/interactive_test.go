package miniapp

import (
	"encoding/json"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLandingVisitorIPUsesTrustedProxyChain(t *testing.T) {
	for _, tc := range []struct{ peer, forwarded, want string }{
		{"203.0.113.50:2345", "198.51.100.5", "203.0.113.50"},
		{"172.18.0.2:1234", "198.51.100.5, 203.0.113.50", "203.0.113.50"},
		{"127.0.0.1:1234", "198.51.100.5", "198.51.100.5"},
		{"172.18.0.2:1234", "2001:db8::4, 10.0.0.2", "2001:db8::4"},
		{"[::ffff:203.0.113.5]:1234", "", "203.0.113.5"},
		{"172.18.0.2:1234", "invalid", ""},
	} {
		r := httptest.NewRequest("GET", "/api/site/protection", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Forwarded-For", tc.forwarded)
		if got := landingVisitorIP(r); got != tc.want {
			t.Fatalf("%+v got=%s", tc, got)
		}
	}
}
func TestLandingProtectionMatchesWithoutPublishingAddresses(t *testing.T) {
	cfg := runtimeconfig.DefaultSettings()
	cfg.Content.BrandName = "BruhVPN"
	cfg.Interactive.Enabled = true
	cfg.Interactive.Addresses = []string{"203.0.113.8", "2001:db8::/32"}
	for _, ip := range []string{"203.0.113.8", "2001:db8::4", "203.0.113.80"} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = ip
		out := protectionForRequest(cfg, r)
		want := ip != "203.0.113.80"
		if !out.Enabled || (out.State == "protected") != want {
			t.Fatalf("%s: %+v", ip, out)
		}
		if want && !strings.Contains(out.Text, "BruhVPN") {
			t.Fatal("brand not substituted")
		}
		raw, _ := json.Marshal(out)
		if strings.Contains(string(raw), ip) || strings.Contains(string(raw), "addresses") {
			t.Fatal("address leaked")
		}
	}
	cfg.Interactive.Enabled = false
	if protectionForRequest(cfg, httptest.NewRequest("GET", "/", nil)).Enabled {
		t.Fatal("disabled shown")
	}
	w := httptest.NewRecorder()
	(&Handler{}).handleLandingProtection(w, httptest.NewRequest("GET", "/api/site/protection", nil))
	if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("personal state cacheable")
	}
}
func TestInteractiveAdminPermissionsAndPanelImport(t *testing.T) {
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/nodes" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":[{"name":"A","address":"203.0.113.8"},{"name":"B","address":"203.0.113.8:443"},{"name":"C","address":"[2001:db8::4]:443"}]}`))
	}))
	defer panel.Close()
	h := &Handler{remnawaveClient: remnawave.NewClient(panel.URL, "test", "")}
	admin := &session{User: telegramUser{ID: 42}, AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"interactive"}}}
	for _, permission := range []string{"appearance", "interactive"} {
		sess := *admin
		sess.AdminAccess = adminAccess{IsAdmin: true, Permissions: []string{permission}}
		w := httptest.NewRecorder()
		h.handleInteractiveNodes(w, httptest.NewRequest("POST", "/api/mini-app/admin/interactive/nodes", nil), &sess, nil)
		if permission != "interactive" {
			if w.Code != 403 {
				t.Fatal("permission bypass")
			}
			continue
		}
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var payload struct {
			Data struct {
				Addresses []string `json:"addresses"`
			} `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &payload)
		if len(payload.Data.Addresses) != 2 {
			t.Fatal(w.Body.String())
		}
	}
	if adminRouteAllowed(adminAccess{IsAdmin: true, Permissions: []string{"appearance"}}, "/api/mini-app/admin/interactive/nodes") {
		t.Fatal("route bypass")
	}
	cfg := runtimeconfig.DefaultSettings()
	cfg.Interactive.Addresses = []string{"203.0.113.8"}
	if len(adminSettingsForAccess(cfg, adminAccess{IsAdmin: true, Permissions: []string{"appearance"}}).Interactive.Addresses) != 0 {
		t.Fatal("private addresses leaked to another role")
	}
}
