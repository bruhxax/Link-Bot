package miniapp

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
)

func TestLandingServesPublicPage(t *testing.T) {
	staticFS, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{staticFS: staticFS, assetVersion: "test-version"}
	response := httptest.NewRecorder()
	handler.serveRoot(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("root status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	for _, want := range []string{"Интернет без", "Сервера", "Тарифы", "Поддержка", "/mini-app/", "landing.css?v=test-version"} {
		if !strings.Contains(body, want) {
			t.Errorf("landing page missing %q", want)
		}
	}
	if strings.Contains(body, "__ASSET_VERSION__") || strings.Contains(body, "__BRAND_NAME__") || strings.Contains(body, "__INITIAL_THEME__") || strings.Contains(body, "__THEME_COLOR__") || strings.Contains(body, "__GLASS_MODE__") {
		t.Error("landing page contains unresolved placeholders")
	}
	if !strings.Contains(body, `data-glass="off"`) {
		t.Error("landing page must start with the default glass setting")
	}
	if !strings.Contains(body, `<style id="landing-initial-theme">:root{`) || !strings.Contains(body, `--accent:#ba173d;`) {
		t.Error("landing page must render its theme before JavaScript loads")
	}
	if strings.Contains(body, "Интернет на вашей стороне.") || strings.Contains(body, ">В кабинет</a>") {
		t.Error("landing page contains the removed footer content")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Error("landing page must not cache dynamic branding")
	}
}

func TestLandingInitialThemeUsesSavedColorsSafely(t *testing.T) {
	css, background := landingInitialTheme(map[string]string{
		"background": "#010203",
		"accent":     "#B8FF48",
		"text":       `#ffffff;}</style><script>`,
	})
	if background != "#010203" || !strings.Contains(css, "--bg:#010203;") || !strings.Contains(css, "--accent:#B8FF48;") || !strings.Contains(css, "--accent-ink:#111217;") {
		t.Fatalf("landing initial theme does not match saved colors: %q / %q", css, background)
	}
	if strings.Contains(css, "</style>") || strings.Contains(css, "--text:") {
		t.Fatalf("unsafe theme color was included in HTML: %q", css)
	}
}

func TestLandingLinksToOptionalCabinetHost(t *testing.T) {
	staticFS, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{staticFS: staticFS, cabinetBaseURL: "https://my.example.com"}
	landing := httptest.NewRecorder()
	handler.serveRoot(landing, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	if landing.Code != http.StatusOK || !strings.Contains(landing.Body.String(), `href="https://my.example.com/mini-app/?cabinet=1"`) || !strings.Contains(landing.Body.String(), `data-cabinet-base="https://my.example.com"`) {
		t.Fatalf("landing does not link to cabinet host: status=%d", landing.Code)
	}
	cabinet := httptest.NewRecorder()
	handler.serveRoot(cabinet, httptest.NewRequest(http.MethodGet, "https://my.example.com/", nil))
	if cabinet.Code != http.StatusFound || cabinet.Header().Get("Location") != "/mini-app/?cabinet=1" {
		t.Fatalf("cabinet root redirect = %d %q", cabinet.Code, cabinet.Header().Get("Location"))
	}
}

func TestLandingAPIHidesNodeAddressesAndSortsPlans(t *testing.T) {
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/nodes" {
			t.Errorf("unexpected panel path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"response":[{"name":"","address":"203.0.113.9","countryCode":"DE","isConnected":true},{"name":"Sweden","address":"198.51.100.4","countryCode":"SE","isConnected":false}]}`))
	}))
	defer panel.Close()

	handler := &Handler{remnawaveClient: remnawave.NewClient(panel.URL, "test-token", "")}
	response := httptest.NewRecorder()
	handler.handleLandingData(response, httptest.NewRequest(http.MethodGet, "/api/site/landing", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("landing API status = %d, want 200", response.Code)
	}
	if strings.Contains(response.Body.String(), "203.0.113.9") || strings.Contains(response.Body.String(), "198.51.100.4") {
		t.Fatal("public landing response leaked a node address")
	}
	var result struct {
		Data landingPayload `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Glass {
		t.Fatal("landing API must leave glass disabled by default")
	}
	if !result.Data.NodesAvailable || len(result.Data.Nodes) != 2 || result.Data.Nodes[1].Name != "Сервер 1" {
		t.Fatalf("unexpected sanitized nodes: %+v", result.Data.Nodes)
	}
	for index := 1; index < len(result.Data.Plans); index++ {
		if landingPlanDays(result.Data.Plans[index-1]) > landingPlanDays(result.Data.Plans[index]) {
			t.Fatalf("plans are not ordered by duration at %d", index)
		}
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Error("public landing API must not be cached by the browser")
	}
}

func TestLandingContactsFromMiniAppSettings(t *testing.T) {
	settings := runtimeconfig.DefaultSettings()
	settings.Content.AdminContact = "@example_admin"
	contacts := landingContacts(settings, "https://t.me/other_support")
	if len(contacts) != 2 || contacts[0].URL != "https://t.me/example_admin" || contacts[1].URL != "https://t.me/other_support" {
		t.Fatalf("unexpected contacts: %+v", contacts)
	}
	if got := landingContacts(settings, "javascript:alert(1)"); len(got) != 1 {
		t.Fatalf("unsafe contact URL should be excluded: %+v", got)
	}
}
