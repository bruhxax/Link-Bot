package remnawave

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestReissueUserPreservesRemainingAccess(t *testing.T) {
	expireAt := time.Now().UTC().Add(15 * 24 * time.Hour).Truncate(time.Second)
	squadID := uuid.New()
	var created map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/users/17":
			_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{
				"id": 17, "username": "person", "status": "ACTIVE", "expireAt": expireAt,
				"telegramId": 42, "subscriptionUrl": "https://example.test/old",
				"trafficLimitBytes": 1000, "trafficLimitStrategy": "NO_RESET", "hwidDeviceLimit": 5,
				"activeInternalSquads": []map[string]any{{"uuid": squadID, "name": "Node"}},
				"userTraffic":          map[string]any{"usedTrafficBytes": 400},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/users":
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Errorf("decode create: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{
				"id": 18, "username": created["username"], "status": "ACTIVE", "expireAt": expireAt,
				"subscriptionUrl": "https://example.test/new",
			}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()
	old, next, err := NewClient(server.URL, "token", "remote").ReissueUser(context.Background(), 17, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if old.ID != 17 || next.ID != 18 || next.SubscriptionURL != "https://example.test/new" {
		t.Fatalf("identities: old=%+v new=%+v", old, next)
	}
	if created["trafficLimitBytes"] != float64(600) || created["hwidDeviceLimit"] != float64(5) || created["telegramId"] != float64(42) {
		t.Fatalf("access was not preserved: %+v", created)
	}
	if created["expireAt"] != expireAt.Format(time.RFC3339) {
		t.Fatalf("expiry changed: %v", created["expireAt"])
	}
	if created["trafficLimitStrategy"] != "NO_RESET" {
		t.Fatalf("strategy changed: %v", created["trafficLimitStrategy"])
	}
	if squads, ok := created["activeInternalSquads"].([]any); !ok || len(squads) != 1 || squads[0] != squadID.String() {
		t.Fatalf("squads changed: %v", created["activeInternalSquads"])
	}
}
