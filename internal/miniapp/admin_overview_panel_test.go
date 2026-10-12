package miniapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
)

func TestOverviewUsesEveryPanelAccountAndPanelRegistrationDates(t *testing.T) {
	now := time.Date(2026, 10, 12, 1, 0, 0, 0, time.FixedZone("MSK", 10800))
	users := []remnawave.PanelUser{
		{ID: 1, Status: "ACTIVE", ExpireAt: now.Add(time.Hour), CreatedAt: now.Add(-30 * time.Minute), TrafficLimitBytes: 100},
		{ID: 2, Status: "EXPIRED", ExpireAt: now.Add(-time.Hour), CreatedAt: now.Add(-2 * time.Hour)},
		{ID: 3, Status: "LIMITED", ExpireAt: now.Add(time.Hour)},
		{ID: 4, Status: "DISABLED", ExpireAt: now.Add(time.Hour)},
		{ID: 5, Status: "UNKNOWN", ExpireAt: now.Add(time.Hour)},
	}
	stats, _ := panelOverview(users, nil, runtimeconfig.Settings{}, 7, now)
	if stats.Source != "panel" || stats.Total != 5 || stats.Active != 1 || stats.Expired != 1 || stats.Limited != 1 || stats.Disabled != 1 || stats.Other != 1 {
		t.Fatalf("wrong panel counters %+v", stats)
	}
	if stats.Today != 1 || stats.Week != 2 || stats.Daily[6].Date != "2026-10-12" || stats.Daily[6].Value != 1 || stats.Daily[5].Value != 1 {
		t.Fatalf("wrong panel/Moscow dates %+v", stats)
	}
}

func TestOverviewDoesNotGuessAmbiguousTermsOrReuseAnotherPanelIdentity(t *testing.T) {
	now := time.Now()
	id := int64(1)
	oldUUID, currentUUID := uuid.New(), uuid.New()
	users := []remnawave.PanelUser{{ID: id, UUID: currentUUID, Status: "ACTIVE", ExpireAt: now.Add(time.Hour)}}
	settings := runtimeconfig.Settings{Plans: []runtimeconfig.PlanSettings{{Months: 1, UnlimitedTraffic: true}, {Months: 3, UnlimitedTraffic: true}}}
	observations := []database.PanelPlanObservation{{PanelID: &id, PanelUUID: &oldUUID, Months: 12}}
	_, plans := panelOverview(users, observations, settings, 7, now)
	if len(plans) != 1 || plans[0].Months != 0 || plans[0].Label != "Индивидуальный тариф · Безлимит" {
		t.Fatalf("guessed a term or reused stale identity: %+v", plans)
	}
}

func TestOverviewMergesTariffAliasesAndOrdersDurationAndUnlimited(t *testing.T) {
	now := time.Now()
	var users []remnawave.PanelUser
	var observations []database.PanelPlanObservation
	for i, months := range []int{12, 1, 3, 1, 1, 6} {
		id := int64(i + 1)
		traffic := int64(100)
		if i == 3 || i == 5 {
			traffic = 0
		}
		users = append(users, remnawave.PanelUser{ID: id, Status: "ACTIVE", ExpireAt: now.Add(time.Hour), TrafficLimitBytes: traffic})
		observations = append(observations, database.PanelPlanObservation{PanelID: &id, Months: months, PlanID: []string{"annual", "month", "quarter", "unlimited_month", "legacy_month", "unlimited_half"}[i]})
	}
	_, plans := panelOverview(users, observations, runtimeconfig.Settings{}, 30, now)
	if len(plans) != 5 || plans[0].Months != 1 || plans[0].Unlimited || plans[0].Count != 2 || plans[1].Months != 1 || !plans[1].Unlimited || plans[2].Months != 3 || plans[3].Months != 6 || plans[4].Months != 12 {
		t.Fatalf("aliases/order not merged %+v", plans)
	}
	if plans[1].Label != "1 месяц · Безлимит" || plans[2].Label != "3 месяца · Лимитный" {
		t.Fatalf("ambiguous labels %+v", plans)
	}
}

func TestOverviewEndpointNeverFallsBackToBotCountsAndCachesPanelReads(t *testing.T) {
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("overview mutated panel")
		}
		if r.URL.Path == "/api/system/stats" {
			json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"users": map[string]any{"totalUsers": 2, "statusCounts": map[string]int{"ACTIVE": 1, "DISABLED": 1}}}})
			return
		}
		reads++
		_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"users": []map[string]any{{"id": 1, "status": "ACTIVE", "expireAt": "2028-01-01T00:00:00Z"}, {"id": 2, "status": "DISABLED", "expireAt": "2028-01-01T00:00:00Z"}}, "hasMore": false}})
	}))
	defer server.Close()
	h := &Handler{remnawaveClient: remnawave.NewClient(server.URL, "token", "remote")}
	sess := &session{AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"users"}}}
	for _, body := range []string{`{"days":30}`, `{"days":7}`, `{"days":30,"force":true}`} {
		w := httptest.NewRecorder()
		h.handleAdminOverview(w, httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/overview", strings.NewReader(body)), sess, nil)
		if w.Code != 200 {
			t.Fatalf("endpoint failed %s", w.Body.String())
		}
		var payload struct {
			Data database.AdminOverview `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Data.Users == nil || payload.Data.Users.Total != 2 || payload.Data.Users.Disabled != 1 || payload.Data.Users.Source != "panel" {
			t.Fatalf("wrong source %s", w.Body.String())
		}
	}
	if reads != 2 {
		t.Fatalf("cache/forced refresh made %d reads", reads)
	}
	h.remnawaveClient = nil
	w := httptest.NewRecorder()
	h.handleAdminOverview(w, httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/overview", strings.NewReader(`{"days":30}`)), sess, nil)
	if !strings.Contains(w.Body.String(), `"users":null`) || !strings.Contains(w.Body.String(), `"panelError"`) {
		t.Fatalf("panel outage became bot statistics: %s", w.Body.String())
	}
	if _, err := h.overviewPanelUsers(context.Background(), false); err == nil {
		t.Fatal("missing panel silently succeeded")
	}
}

func TestOverviewCatalogBudgetCoversMultipleSlowPages(t *testing.T) {
	if testing.Short() {
		t.Skip("slow catalog timeout regression")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(6200 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		cursor := r.URL.Query().Get("cursor")
		id := 1
		if cursor != "" {
			id = 2
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{
			"users":      []map[string]any{{"id": id, "status": "ACTIVE", "expireAt": "2028-01-01T00:00:00Z"}},
			"nextCursor": "second", "hasMore": cursor == "",
		}})
	}))
	defer server.Close()
	h := &Handler{remnawaveClient: remnawave.NewClient(server.URL, "token", "remote")}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	users, err := h.overviewPanelUsers(ctx, true)
	if err != nil || len(users) != 2 {
		t.Fatalf("complete catalog cut off by one-page budget: %d users, %v", len(users), err)
	}
}
