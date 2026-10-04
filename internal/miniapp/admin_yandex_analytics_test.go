package miniapp

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestYandexTrackingRenderedOnLandingAndMiniAppWithoutOAuthSecret(t *testing.T) {
	t.Setenv("YANDEX_METRIKA_COUNTER_ID", "12345678")
	t.Setenv("YANDEX_METRIKA_OAUTH_TOKEN", "DO-NOT-EXPOSE-OAUTH-TOKEN")
	staticFS, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{staticFS: staticFS, assetVersion: "test-version"}
	for _, path := range []string{"/", "/mini-app/"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if path == "/" {
			h.serveRoot(response, request)
		} else {
			h.serveIndex(response, request)
		}
		body := response.Body.String()
		if response.Code != http.StatusOK || !strings.Contains(body, `name="yandex-metrika-counter-id" content="12345678"`) || !strings.Contains(body, "/mini-app/yandex-metrika.js?v=test-version") {
			t.Fatalf("tracking not wired into %s", path)
		}
		if strings.Contains(body, "DO-NOT-EXPOSE-OAUTH-TOKEN") || strings.Contains(body, "__YANDEX_METRIKA_COUNTER_ID__") {
			t.Fatalf("unsafe analytics bootstrap on %s", path)
		}
		csp := response.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "https://mc.yandex.ru") || !strings.Contains(csp, "https://yastatic.net") {
			t.Fatal("CSP would block Metrika")
		}
	}
}

func TestFetchYandexReportMapsSummaryDailyAndSources(t *testing.T) {
	client := &http.Client{Transport: ga4RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Host != "api-metrika.yandex.net" || req.URL.Path != "/stat/v1/data" {
			t.Errorf("wrong endpoint %s", req.URL)
		}
		q := req.URL.Query()
		if q.Get("ids") != "12345678" || q.Get("date1") != "2026-10-01" || q.Get("date2") != "2026-10-07" || q.Get("timezone") != "+03:00" || q.Get("lang") != "ru" || q.Has("oauth_token") {
			t.Errorf("wrong query %s", req.URL)
		}
		if req.Header.Get("Authorization") != "OAuth secret-test-token" {
			t.Error("OAuth authorization header missing")
		}
		body := ""
		switch q.Get("dimensions") {
		case "":
			if q.Get("metrics") != "ym:s:users,ym:s:newUsers,ym:s:visits,ym:s:pageviews" {
				t.Error("wrong summary metrics")
			}
			body = `{"totals":[100,25,200,500],"data":[{"metrics":[999,999,999,999]}]}`
		case "ym:s:date":
			if q.Get("sort") != "ym:s:date" || q.Get("limit") != "370" {
				t.Error("daily report must cover the whole period in date order")
			}
			body = `{"totals":[100,200],"data":[{"dimensions":[{"name":"2026-10-02"}],"metrics":[10,20]},{"dimensions":[{"name":"bad-date"}],"metrics":[10,20]}]}`
		case "ym:s:lastTrafficSource":
			if q.Get("sort") != "-ym:s:visits" {
				t.Error("sources must be sorted by visits")
			}
			body = `{"sampled":true,"totals":[200],"data":[{"dimensions":[{"name":"Прямые заходы"}],"metrics":[55.6]}]}`
		default:
			t.Errorf("unexpected dimensions %s", q.Get("dimensions"))
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, adminFinanceLocation)
	report, err := fetchYandexReport(context.Background(), client, "12345678", "secret-test-token", from, from.AddDate(0, 0, 7))
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "ready" || report.ActiveUsers != 100 || report.NewUsers != 25 || report.Sessions != 200 || report.PageViews != 500 {
		t.Fatalf("wrong totals: %+v", report)
	}
	if len(report.Daily) != 1 || report.Daily[0].Date != "2026-10-02" || report.Daily[0].Sessions != 20 {
		t.Fatalf("wrong daily: %+v", report.Daily)
	}
	if len(report.Channels) != 1 || report.Channels[0].Name != "Прямые заходы" || report.Channels[0].Sessions != 56 || !report.Sampled {
		t.Fatalf("wrong sources: %+v", report)
	}
}

func TestYandexAPIErrorDoesNotExposeResponseSecrets(t *testing.T) {
	client := &http.Client{Transport: ga4RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(`{"message":"secret-test-token"}`))}, nil
	})}
	_, err := runYandexReport(context.Background(), client, "123", "secret-test-token", "2026-10-01", "2026-10-07", "", "ym:s:visits", "", 1)
	if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "secret-test-token") {
		t.Fatalf("unsafe or missing API error: %v", err)
	}
}

func TestYandexConfigurationStates(t *testing.T) {
	h := &Handler{}
	now := time.Now()
	t.Setenv("YANDEX_METRIKA_COUNTER_ID", "")
	t.Setenv("YANDEX_METRIKA_OAUTH_TOKEN", "")
	if r := h.loadAdminYandex(context.Background(), now, now); r.State != "unconfigured" {
		t.Fatalf("unconfigured: %+v", r)
	}
	for _, id := range []string{"0", "-1", "123<script>", "1234567890123"} {
		t.Setenv("YANDEX_METRIKA_COUNTER_ID", id)
		if yandexCounterID() != "" {
			t.Fatal("invalid ID can reach HTML")
		}
		if r := h.loadAdminYandex(context.Background(), now, now); r.State != "error" {
			t.Fatalf("invalid config: %+v", r)
		}
	}
	t.Setenv("YANDEX_METRIKA_COUNTER_ID", "12345678")
	if yandexCounterID() != "12345678" {
		t.Fatal("valid ID not exposed for collection")
	}
	if r := h.loadAdminYandex(context.Background(), now, now); r.State != "credentials_required" {
		t.Fatalf("missing token: %+v", r)
	}
}
