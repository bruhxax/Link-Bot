package miniapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"link-bot/internal/database"
)

type activityMemory struct {
	created, finished []database.AdminActivity
	err               error
	query             database.AdminActivityQuery
	listCalls         int
}

func (m *activityMemory) CreateAdminActivity(_ context.Context, e database.AdminActivity) (int64, error) {
	m.created = append(m.created, e)
	return 41, m.err
}
func (m *activityMemory) FinishAdminActivity(_ context.Context, e database.AdminActivity) error {
	m.finished = append(m.finished, e)
	return nil
}
func (m *activityMemory) ListAdminActivity(_ context.Context, q database.AdminActivityQuery) ([]database.AdminActivity, bool, error) {
	m.listCalls++
	m.query = q
	return []database.AdminActivity{}, false, nil
}
func activitySession(owner bool) *session {
	return &session{User: telegramUser{ID: 8544649953, Username: "operator"}, AdminAccess: adminAccess{IsAdmin: true, IsOwner: owner, Role: "Поддержка"}}
}

func TestActivityRecordsTrustedActorTargetAndOutcome(t *testing.T) {
	for _, status := range []int{200, 400, 500} {
		repo := &activityMemory{}
		h := &Handler{adminActivityRepository: repo}
		r := httptest.NewRequest("POST", "/api/mini-app/admin/users/block", strings.NewReader(`{"customerId":9,"blocked":true,"reason":"Спам","actorTelegramId":123}`))
		w := httptest.NewRecorder()
		h.runAdminActivity(w, r, activitySession(false), nil, func(w http.ResponseWriter, r *http.Request, _ *session, _ *database.Customer) {
			var body struct {
				ActorTelegramID int64  `json:"actorTelegramId"`
				CustomerID      int64  `json:"customerId"`
				Blocked         bool   `json:"blocked"`
				Reason          string `json:"reason"`
			}
			if err := h.decodeJSONRequest(w, r, 4096, &body); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(status)
		})
		if len(repo.created) != 1 || repo.created[0].Status != "pending" || len(repo.finished) != 1 {
			t.Fatalf("missing lifecycle %+v", repo)
		}
		e := repo.finished[0]
		want := "failed"
		if status == 200 {
			want = "success"
		}
		if e.ActorTelegramID != 8544649953 || e.ActorName != "operator" || e.TargetCustomerID != 9 || e.Status != want || e.Title != "Заблокировал пользователя" {
			t.Fatalf("wrong audit entry %+v", e)
		}
	}
}

func TestActivityFailurePreventsUnauditedMutation(t *testing.T) {
	repo := &activityMemory{err: errors.New("offline")}
	h := &Handler{adminActivityRepository: repo}
	w := httptest.NewRecorder()
	called := false
	h.runAdminActivity(w, httptest.NewRequest("POST", "/api/mini-app/admin/users/balance", nil), activitySession(false), nil, func(http.ResponseWriter, *http.Request, *session, *database.Customer) { called = true })
	if called || w.Code != 503 || len(repo.finished) != 0 {
		t.Fatalf("mutation escaped audit: called=%v status=%d", called, w.Code)
	}
}

func TestActivityOneUserOpenSurvivesRepeatedBackgroundRefreshes(t *testing.T) {
	repo := &activityMemory{}
	h := &Handler{adminActivityRepository: repo}
	request := func(route, body string) {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/mini-app/admin/"+route, strings.NewReader(body))
		w := httptest.NewRecorder()
		h.runAdminActivity(w, r, activitySession(false), nil, func(w http.ResponseWriter, r *http.Request, _ *session, _ *database.Customer) {
			var payload adminUserActionRequest
			if err := h.decodeJSONRequest(w, r, 4096, &payload); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusOK)
		})
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d", route, w.Code)
		}
	}
	request("users/open", `{"customerId":9}`)
	for i := 0; i < 10; i++ {
		request("users/detail", `{"customerId":9}`)
	}
	request("users/subscription", `{"customerId":9,"days":7}`)
	for i := 0; i < 10; i++ {
		request("users/detail", `{"customerId":9}`)
	}
	if len(repo.finished) != 2 || repo.finished[0].Action != "users/open" || repo.finished[0].TargetCustomerID != 9 || repo.finished[1].Action != "users/subscription" {
		t.Fatalf("background reads polluted audit: %+v", repo.finished)
	}
	// Returning to the list and opening again is a separate real action.
	request("users/open", `{"customerId":9}`)
	if len(repo.finished) != 3 || repo.finished[2].Action != "users/open" {
		t.Fatal("genuine second visit was lost")
	}
	access := adminAccess{IsAdmin: true, Permissions: []string{"users"}}
	if !adminRouteAllowed(access, "/api/mini-app/admin/users/open") || adminRouteAllowed(adminAccess{IsAdmin: true, Permissions: []string{"status"}}, "/api/mini-app/admin/users/open") {
		t.Fatal("opening policy differs from user detail access")
	}
}

func TestActivitySubscriptionRefreshesNeverReplaceManualSearchLogs(t *testing.T) {
	repo := &activityMemory{}
	h := &Handler{adminActivityRepository: repo}
	for i := 0; i < 10; i++ {
		for _, route := range []string{"subscriptions/find/refresh", "subscriptions/target/refresh"} {
			called := false
			h.runAdminActivity(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/mini-app/admin/"+route, nil), activitySession(false), nil, func(w http.ResponseWriter, _ *http.Request, _ *session, _ *database.Customer) {
				called = true
				w.WriteHeader(200)
			})
			if !called {
				t.Fatal("refresh stopped working")
			}
		}
	}
	if len(repo.created) != 0 || len(repo.finished) != 0 {
		t.Fatal("subscription polling is recorded")
	}
	for _, route := range []string{"subscriptions/find", "subscriptions/target", "subscriptions/rebind"} {
		if _, record := activityRoute("/api/mini-app/admin/" + route); !record {
			t.Fatalf("manual %s lost audit", route)
		}
	}
}

func TestActivityPrivacyAndReadableDiff(t *testing.T) {
	h := &Handler{}
	capture := &activityCapture{entry: database.AdminActivity{Action: "smtp/update"}}
	r := httptest.NewRequest("POST", "/", nil).WithContext(context.WithValue(context.Background(), activityContextKey{}, capture))
	h.captureAdminActivityPayload(r, map[string]any{"password": "secret-password", "proxyUrl": "https://name:secret-proxy@example.com", "host": "smtp.example.com", "initData": "secret-auth", "unknown": "secret-unknown"})
	activityDiff(&capture.entry.Details, "appearance", map[string]any{"showFrames": false, "apiKey": "old"}, map[string]any{"showFrames": true, "apiKey": "secret-key", "logoUrl": "https://name:secret-url@example.com"})
	data, _ := json.Marshal(capture.entry)
	if strings.Contains(string(data), "secret-") {
		t.Fatalf("secret leaked: %s", data)
	}
	if !strings.Contains(string(data), "Рамки") || !strings.Contains(string(data), "Выключено") || !strings.Contains(string(data), "Включено") {
		t.Fatalf("missing before/after %s", data)
	}
	if activityInt(float64(8544649953)) != 8544649953 {
		t.Fatal("Telegram ID precision lost")
	}
}

func TestActivityOwnerOnlyAndBoundedQuery(t *testing.T) {
	repo := &activityMemory{}
	h := &Handler{adminActivityRepository: repo}
	for _, owner := range []bool{false, true} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/mini-app/admin/administrators/logs", strings.NewReader(`{"telegramId":22,"limit":100000,"query":"  пользователь  "}`))
		h.handleAdminActivity(w, r, activitySession(owner), nil)
		if !owner {
			if w.Code != 403 || repo.listCalls != 0 {
				t.Fatal("delegate saw logs")
			}
			if adminRouteAllowed(activitySession(false).access(), r.URL.Path) {
				t.Fatal("route exposed")
			}
		} else if w.Code != 200 || repo.query.Limit != 30 || repo.query.Query != "пользователь" {
			t.Fatalf("query %+v, status %d", repo.query, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.handleAdminActivity(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"telegramId":22,"category":"settings|users"}`)), activitySession(true), nil)
	if w.Code != 400 {
		t.Fatal("invalid category accepted")
	}
}

func TestEveryAdminActionIsAuditedAndReadsAreExcluded(t *testing.T) {
	raw, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range regexp.MustCompile(`mux.HandleFunc\("(/api/mini-app/admin/[^"\s]+)"`).FindAllStringSubmatch(string(raw), -1) {
		route, record := activityRoute(match[1])
		if route == "administrators/" {
			continue
		} // Routes registered by the list/save/remove loop.
		if record {
			if _, ok := activityActions[route]; !ok {
				t.Errorf("missing readable action %s", route)
			}
		}
	}
	if _, record := activityRoute("/api/mini-app/admin/new/future-mutation"); !record {
		t.Fatal("future mutation not audited")
	}
	for _, route := range []string{"status", "users/search", "users/detail", "subscriptions/find/refresh", "subscriptions/target/refresh", "administrators/logs", "broadcast/state"} {
		if _, record := activityRoute("/api/mini-app/admin/" + route); record {
			t.Errorf("polling recorded %s", route)
		}
	}
	for _, route := range []string{"support/send", "support/send-media", "support/operator", "support/close"} {
		if _, record := activityRoute("/api/mini-app/" + route); !record {
			t.Errorf("support escaped audit %s", route)
		}
	}
}
