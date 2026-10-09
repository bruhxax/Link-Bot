package miniapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

func TestAdminOverviewUsesSelectedSubscriptionIdentityAndTraffic(t *testing.T) {
	id := int64(222)
	expires := time.Date(2027, 2, 3, 12, 0, 0, 0, time.UTC)
	created := expires.Add(-time.Hour * 24)
	online := created.Add(time.Hour)
	description, tag := "Subscription granted manually", "PREMIUM"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/222" {
			t.Errorf("unexpected lookup: %s", r.URL.Path)
			w.WriteHeader(400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"response": remnawave.PanelUser{ID: id, Username: "secondary", Status: "LIMITED", ExpireAt: expires, CreatedAt: created, Description: &description, Tag: &tag, TrafficLimitStrategy: "MONTH", SubscriptionURL: "https://example.com/sub/secondary", TrafficLimitBytes: 100, UserTraffic: remnawave.PanelUserTraffic{UsedTrafficBytes: 100, LifetimeUsedTrafficBytes: 700, OnlineAt: &online}}})
	}))
	defer server.Close()
	h := Handler{remnawaveClient: remnawave.NewClient(server.URL, "test", "remote")}
	rows := []adminUserSummaryPayload{{CustomerID: 12, TelegramID: 555, SubscriptionName: "Работа"}}
	h.enrichAdminUserOverview(context.Background(), []database.AdminUserSummary{{CustomerID: 12, TelegramID: 555, PanelUserID: &id, SubscriptionIsPrimary: false}}, rows)
	got := rows[0]
	if !got.TrafficLoaded || got.UsedTrafficBytes != 100 || got.TrafficLimitBytes != 100 || got.PanelUsername != "secondary" || got.SubscriptionStatus != "limited" || got.SubscriptionName != "Работа" || got.ExpiresAt != expires.Format(time.RFC3339) || got.SubscriptionLink != "https://example.com/sub/secondary" {
		t.Fatalf("wrong selected subscription: %+v", got)
	}
	if got.PanelID != id || got.Description != description || got.Tag != tag || got.PanelCreatedAt != created.Format(time.RFC3339) || got.OnlineAt != online.Format(time.RFC3339) || got.LifetimeUsedTrafficBytes != 700 || got.TrafficLimitStrategy != "MONTH" {
		t.Fatalf("panel overview metadata lost: %+v", got)
	}
}

func TestAdminOverviewFailureDoesNotInventZeroTraffic(t *testing.T) {
	h := Handler{remnawaveClient: remnawave.NewClient("http://127.0.0.1:1", "test", "remote")}
	rows := []adminUserSummaryPayload{{SubscriptionStatus: "expired"}}
	h.enrichAdminUserOverview(context.Background(), []database.AdminUserSummary{{SubscriptionIsPrimary: false}}, rows)
	if rows[0].TrafficLoaded || rows[0].SubscriptionStatus != "expired" {
		t.Fatalf("unavailable traffic must remain unknown: %+v", rows[0])
	}
}

func TestAdminOverviewPreservesBotBlockAndUnlimitedTraffic(t *testing.T) {
	row := adminUserSummaryPayload{IsBlocked: true}
	applyAdminUserOverview(&row, &remnawave.PanelUser{Status: "ACTIVE", UUID: uuid.New(), UserTraffic: remnawave.PanelUserTraffic{UsedTrafficBytes: 1024}})
	if row.SubscriptionStatus != "blocked" || !row.TrafficLoaded || row.TrafficLimitBytes != 0 || row.UsedTrafficBytes != 1024 {
		t.Fatalf("blocked unlimited account: %+v", row)
	}
}
