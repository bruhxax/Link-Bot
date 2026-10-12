package miniapp

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPanelUserRowsExcludeBotOnlyAndKeepExpiredOrUnlinkedSubscriptions(t *testing.T) {
	now := time.Now()
	panelUUID := uuid.New()
	id := int64(2)
	customerID := int64(901)
	catalog := panelCatalogSnapshot{Users: []remnawave.PanelUser{
		{ID: 1, Username: "panel-only", Status: "EXPIRED", ExpireAt: now.Add(-time.Hour), CreatedAt: now},
		{ID: 2, UUID: panelUUID, Username: "linked", Status: "DISABLED", SubscriptionURL: "https://example.com/2", CreatedAt: now.Add(-time.Hour)},
		{ID: 3, Username: "no-subscription", Status: "ACTIVE"},
	}, Observations: []database.PanelPlanObservation{{CustomerID: customerID, SubscriptionID: 91, PanelID: &id, PanelUUID: &panelUUID, Months: 3}, {CustomerID: 99999, SubscriptionID: 99999, Link: "https://example.com/bot-only"}}}
	rows, counts := panelUserRows(catalog, "")
	if len(rows) != 2 || counts.TotalUsers != 2 || counts.StatusCounts["EXPIRED"] != 1 || counts.StatusCounts["DISABLED"] != 1 {
		t.Fatalf("wrong source/subscription filter: %+v %+v", rows, counts)
	}
	if rows[0].CustomerID != 0 || rows[0].PanelID != 1 || rows[1].CustomerID != 901 || rows[1].SubscriptionID != 91 || rows[0].Key == rows[1].Key {
		t.Fatalf("lost unlinked identity or wrong linked subscription %+v", rows)
	}
	rows, _ = panelUserRows(catalog, "linked")
	if len(rows) != 1 || rows[0].PanelID != 2 {
		t.Fatalf("panel search wrong: %+v", rows)
	}
	stale := uuid.New()
	catalog.Observations[0].PanelUUID = &stale
	rows, _ = panelUserRows(catalog, "linked")
	if rows[0].CustomerID != 0 {
		t.Fatal("reused panel ID bound to a different UUID")
	}
}

func TestCatalogColdResponseIsBoundedAndCancelledTabDoesNotCancelSharedLoad(t *testing.T) {
	ready := make(chan struct{})
	release := make(chan struct{})
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/system/stats" {
			json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"users": map[string]any{"totalUsers": 1, "statusCounts": map[string]int{"EXPIRED": 1}}}})
			return
		}
		if reads.Add(1) == 1 {
			close(ready)
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"users": []map[string]any{{"id": 1, "username": "panel", "status": "EXPIRED", "expireAt": "2020-01-01T00:00:00Z"}}, "hasMore": false}})
	}))
	defer server.Close()
	h := &Handler{remnawaveClient: remnawave.NewClient(server.URL, "token", "remote")}
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	snapshot, pending, err := h.requestPanelCatalog(ctx, false, "")
	if err != nil || !pending || snapshot.Users != nil || time.Since(start) > 300*time.Millisecond {
		t.Fatalf("request blocked on catalog: %v %v %v", pending, err, time.Since(start))
	}
	<-ready
	cancel()
	w := httptest.NewRecorder()
	h.handleAdminOverview(w, httptest.NewRequest("POST", "/api/mini-app/admin/overview", strings.NewReader(`{"days":90}`)), &session{AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"users"}}}, nil)
	var overview struct {
		Data database.AdminOverview `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &overview)
	if !overview.Data.Pending || overview.Data.Users == nil || overview.Data.Users.Total != 1 || overview.Data.Users.Daily != nil {
		t.Fatalf("fast counts wait for the full catalog or invent daily data: %s", w.Body.String())
	}
	for range 4 {
		h.requestPanelCatalog(context.Background(), false, "")
	}
	if reads.Load() != 1 {
		t.Fatalf("duplicate catalog fetches: %d", reads.Load())
	}
	close(release)
	users, err := h.overviewPanelUsers(context.Background(), false)
	if err != nil || len(users) != 1 {
		t.Fatalf("tab cancellation destroyed shared result: %+v %v", users, err)
	}
	old, _, _ := h.panelCatalog(false, "")
	// Immutable revision survives a new panel population, keeping offset pagination stable.
	h.overviewPanel.mu.Lock()
	h.overviewPanel.history = append(h.overviewPanel.history, panelCatalogSnapshot{Revision: "new", Users: []remnawave.PanelUser{{ID: 99}}})
	h.overviewPanel.mu.Unlock()
	same, _, err := h.panelCatalog(false, old.Revision)
	if err != nil || same.Users[0].ID != 1 {
		t.Fatalf("append snapshot changed: %+v %v", same, err)
	}
}

func TestCatalogOutageKeepsLastSnapshotWithoutInventingBotCounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "offline", 502) }))
	defer server.Close()
	h := &Handler{remnawaveClient: remnawave.NewClient(server.URL, "token", "remote")}
	old := []remnawave.PanelUser{{ID: 21, Status: "DISABLED", ExpireAt: time.Now().Add(time.Hour)}}
	h.overviewPanel.users = old
	h.overviewPanel.at = time.Now()
	h.overviewPanel.history = []panelCatalogSnapshot{{Users: old, Revision: "last"}}
	h.overviewPanel.at = time.Time{}
	h.panelCatalog(true, "")
	_, _ = h.overviewPanelUsers(context.Background(), false)
	s, pending, err := h.panelCatalog(false, "")
	if err == nil || pending || len(s.Users) != 1 || s.Users[0].ID != 21 {
		t.Fatalf("failed refresh discarded last snapshot %+v %v %v", s, pending, err)
	}
}

func TestPanelOnlyAccountOpensItsActualSettingsWithoutCreatingBotCustomer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("opening a card mutated panel")
		}
		json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"id": 18, "username": "manual-panel-user", "status": "ACTIVE", "expireAt": "2028-01-01T00:00:00Z", "trafficLimitBytes": 1234, "subscriptionUrl": "https://example.com/manual"}})
	}))
	defer server.Close()
	h := &Handler{remnawaveClient: remnawave.NewClient(server.URL, "token", "remote")}
	sess := &session{AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"users"}}}
	w := httptest.NewRecorder()
	h.handleAdminUserDetail(w, httptest.NewRequest("POST", "/users/open", strings.NewReader(`{"panelId":18}`)), sess, nil)
	var response struct {
		Data adminUserDetailPayload `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &response)
	if w.Code != 200 || !response.Data.PanelOnly || response.Data.CustomerID != 0 || response.Data.PanelID != 18 || len(response.Data.Subscriptions) != 1 || response.Data.Subscriptions[0].Settings.TrafficLimitBytes != 1234 {
		t.Fatalf("wrong panel-only detail %s", w.Body.String())
	}
	if adminRouteAllowed(sess.AdminAccess, "/api/mini-app/admin/users/panel/settings") {
		t.Fatal("read-only user can edit panel")
	}
}
