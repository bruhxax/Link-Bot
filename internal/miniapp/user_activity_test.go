package miniapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

type userActivityMemory struct {
	items            []database.UserActivity
	customer, before int64
	calls            int
	err              error
}

func (m *userActivityMemory) RecordUserActivity(_ context.Context, e database.UserActivity) error {
	m.items = append(m.items, e)
	return m.err
}
func (m *userActivityMemory) ListUserActivity(_ context.Context, c, b int64, _ int) ([]database.UserActivity, bool, error) {
	m.calls++
	m.customer = c
	m.before = b
	return m.items, false, m.err
}

func TestUserActivityCapturesOnlyDeliberateTrustedActions(t *testing.T) {
	for _, status := range []int{200, 400, 500} {
		repo := &userActivityMemory{}
		h := &Handler{userActivityRepository: repo}
		r := httptest.NewRequest("POST", "/api/mini-app/purchase", strings.NewReader(`{"customerId":999,"planId":"year","paymentMethod":"smerch","receiptEmail":"private@example.com","initData":"secret-auth","password":"secret-password","promoCode":"private-code","name":"private-name"}`))
		w := httptest.NewRecorder()
		h.runUserActivity(w, r, &session{}, &database.Customer{ID: 42}, func(w http.ResponseWriter, r *http.Request, _ *session, _ *database.Customer) {
			var body map[string]any
			if err := h.decodeJSONRequest(w, r, 4096, &body); err != nil {
				t.Fatal(err)
			}
			if status >= 400 {
				h.writeError(w, status, "payment_failed", "https://secret-provider.example/secret-token")
			} else {
				w.WriteHeader(status)
			}
		})
		if w.Code != status || len(repo.items) != 1 {
			t.Fatalf("response %d history %+v", w.Code, repo.items)
		}
		entry := repo.items[0]
		if entry.CustomerID != 42 || entry.Source != "miniapp" || (entry.Status == "success") != (status == 200) {
			t.Fatalf("untrusted event %+v", entry)
		}
		raw, _ := json.Marshal(entry)
		for _, private := range []string{"999", "private", "secret", "password", "initData"} {
			if strings.Contains(string(raw), private) {
				t.Fatalf("copied %s: %s", private, raw)
			}
		}
		if !strings.Contains(string(raw), "smerch") || !strings.Contains(string(raw), "year") {
			t.Fatal("missing safe checkout details")
		}
		if status >= 400 && !strings.Contains(string(raw), "payment_failed") {
			t.Fatal("missing error code")
		}
	}
	repo := &userActivityMemory{err: errors.New("database offline")}
	h := &Handler{userActivityRepository: repo}
	for _, route := range []string{"bootstrap", "subscription/state", "realtime", "admin/users/activity", "support/thread"} {
		h.runUserActivity(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/mini-app/"+route, nil), &session{}, &database.Customer{ID: 42}, func(w http.ResponseWriter, _ *http.Request, _ *session, _ *database.Customer) { w.WriteHeader(200) })
	}
	if len(repo.items) != 0 {
		t.Fatal("background reads entered user history")
	}
	w := httptest.NewRecorder()
	h.runUserActivity(w, httptest.NewRequest("POST", "/api/mini-app/purchase", nil), &session{}, &database.Customer{ID: 42}, func(w http.ResponseWriter, _ *http.Request, _ *session, _ *database.Customer) { w.WriteHeader(200) })
	if w.Code != 200 {
		t.Fatal("logging failure blocked checkout")
	}
}

func TestUserActivityRequiresUsersPermissionAndValidCursor(t *testing.T) {
	repo := &userActivityMemory{}
	h := &Handler{userActivityRepository: repo}
	owner := activitySession(true)
	for _, tc := range []struct {
		sess   *session
		body   string
		status int
	}{
		{&session{}, `{"customerId":42}`, 403},
		{activitySession(false), `{"customerId":42}`, 403},
		{owner, `{"customerId":0}`, 400},
		{owner, `{"customerId":42,"beforeId":-1}`, 400},
		{owner, `bad`, 400},
		{owner, `{"customerId":42,"beforeId":27}`, 200},
	} {
		w := httptest.NewRecorder()
		h.handleAdminUserActivity(w, httptest.NewRequest("POST", "/", strings.NewReader(tc.body)), tc.sess, nil)
		if w.Code != tc.status {
			t.Fatalf("%s got %d: %s", tc.body, w.Code, w.Body.String())
		}
	}
	if repo.calls != 1 || repo.customer != 42 || repo.before != 27 {
		t.Fatalf("query %+v", repo)
	}
	if adminRouteAllowed(owner.access(), "/api/mini-app/admin/users/activity") == false {
		t.Fatal("owner cannot read history")
	}
	if _, record := activityRoute("/api/mini-app/admin/users/activity"); record {
		t.Fatal("history polling audited as mutation")
	}
	viewer := &session{AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"users"}}}
	if !adminRouteAllowed(viewer.access(), "/api/mini-app/admin/users/activity") {
		t.Fatal("viewer denied history")
	}
	w := httptest.NewRecorder()
	h.handleAdminUserActivity(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"customerId":42}`)), viewer, nil)
	if w.Code != 200 {
		t.Fatalf("viewer %d", w.Code)
	}
}

func TestUserActivityReadablePlanAndPaymentLabels(t *testing.T) {
	settings := runtimeconfig.Settings{Plans: []runtimeconfig.PlanSettings{{ID: "year", Name: "Годовой"}}, PaymentMethodText: map[string]runtimeconfig.PaymentMethodText{"custom": {Name: "СБП"}}}
	entry := database.UserActivity{Details: []database.UserActivityDetail{{Label: "Тариф", Value: "year"}, {Label: "Способ", Value: "custom"}, {Label: "Тип покупки", Value: "gift"}, {Label: "Причина", Value: "payment_failed"}, {Label: "Способ", Value: "smerch"}}}
	enrichUserActivity(&entry, settings)
	want := []string{"Годовой", "СБП", "Подарок", "Не удалось создать платёж (payment_failed)", "Smerch"}
	for i, value := range want {
		if entry.Details[i].Value != value {
			t.Fatalf("detail %d: %s", i, entry.Details[i].Value)
		}
	}
}
