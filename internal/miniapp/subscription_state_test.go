package miniapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

func TestAdminSubscriptionDetailsLoadInParallelFromOneSnapshot(t *testing.T) {
	initMiniAppTestConfig()
	gate := make(chan struct{})
	var release sync.Once
	defer release.Do(func() { close(gate) })
	started := make(chan int64, 10)
	var mu sync.Mutex
	counts := map[int64]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("refresh mutated panel: %s", r.Method)
		}
		if strings.HasPrefix(r.URL.Path, "/api/hwid/devices/") {
			_, _ = w.Write([]byte(`{"response":{"total":0,"devices":[]}}`))
			return
		}
		id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/users/"), 10, 64)
		if id <= 0 {
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		mu.Lock()
		counts[id]++
		mu.Unlock()
		started <- id
		select {
		case <-gate:
		case <-r.Context().Done():
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"id": id, "username": "slot-" + strconv.FormatInt(id, 10), "status": "ACTIVE", "expireAt": "2027-12-01T00:00:00Z", "trafficLimitBytes": id * 100, "hwidDeviceLimit": 5, "userTraffic": map[string]any{"usedTrafficBytes": id * 10}}})
	}))
	defer server.Close()
	h := &Handler{remnawaveClient: remnawave.NewClient(server.URL, "token", "remote")}
	subscriptions := []database.CustomerSubscription{}
	for id := int64(1); id <= 10; id++ {
		panelID := id
		subscriptions = append(subscriptions, database.CustomerSubscription{ID: id, PanelUserID: &panelID, DisplayName: "slot", IsPrimary: id == 1})
	}
	done := make(chan []adminUserSubscriptionPayload, 1)
	go func() {
		done <- h.loadAdminSubscriptionDetails(context.Background(), &database.Customer{TelegramID: 99}, &subscriptions[3], subscriptions, false)
	}()
	for n := 0; n < 6; n++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			release.Do(func() { close(gate) })
			t.Fatal("card waits for subscriptions sequentially")
		}
	}
	release.Do(func() { close(gate) })
	var items []adminUserSubscriptionPayload
	select {
	case items = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("card did not finish")
	}
	for i, item := range items {
		id := int64(i + 1)
		if item.ID != id || item.PanelID != id || item.UsedTrafficBytes != id*10 || item.Settings == nil || item.Settings.TrafficLimitBytes != item.TrafficLimitBytes || item.IsSelected != (id == 4) {
			t.Fatalf("mixed subscription snapshot: %+v", item)
		}
		mu.Lock()
		n := counts[id]
		mu.Unlock()
		if n != 1 {
			t.Fatalf("user %d fetched %d times", id, n)
		}
	}
}

func TestLiveSubscriptionStateKeepsManualChangesAndInactiveExpiry(t *testing.T) {
	initMiniAppTestConfig()
	for _, status := range []string{"ACTIVE", "DISABLED", "EXPIRED"} {
		t.Run(status, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("read changed panel with %s", r.Method)
				}
				switch r.URL.Path {
				case "/api/users/stream":
					_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"users": []any{map[string]any{"id": 42, "telegramId": 99, "username": "manually_granted", "status": status, "expireAt": "2027-12-01T00:00:00Z", "subscriptionUrl": "https://example.com/new", "trafficLimitBytes": 3145728, "hwidDeviceLimit": 2, "userTraffic": map[string]any{"usedTrafficBytes": 1048576}}}, "hasMore": false}})
				case "/api/hwid/devices/42":
					_, _ = w.Write([]byte(`{"response":{"total":0,"devices":[]}}`))
				default:
					t.Errorf("extra request %s", r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			h := &Handler{remnawaveClient: remnawave.NewClient(server.URL, "token", "remote")}
			oldExpiry := time.Now().Add(time.Hour)
			oldLink := "https://example.com/old"
			customer := &database.Customer{ID: 1, TelegramID: 99, TrialUsed: true, ExpireAt: &oldExpiry, SubscriptionLink: &oldLink}
			payload, err := h.loadLiveSubscriptionState(context.Background(), customer)
			if err != nil {
				t.Fatal(err)
			}
			if payload.PanelUsername != "manually_granted" || payload.Subscription.TrafficLimitBytes != 3145728 || payload.Subscription.TrafficUsedBytes != 1048576 || payload.Subscription.IsTrial || !payload.Subscription.StateLoaded {
				t.Fatalf("manual panel state lost: %+v", payload)
			}
			if customer.ExpireAt == nil || customer.ExpireAt.Year() != 2027 || customer.SubscriptionLink == nil || *customer.SubscriptionLink != "https://example.com/new" {
				t.Fatalf("inactive subscription metadata erased: %+v", customer)
			}
			if (payload.Subscription.Status == "active") != (status == "ACTIVE") || (payload.Subscription.HasAccessLink) != (status == "ACTIVE") {
				t.Fatalf("inactive access leak: %+v", payload.Subscription)
			}
		})
	}
}

func TestLivePanelFailureNeverClearsSubscription(t *testing.T) {
	initMiniAppTestConfig()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"message":"upstream user not found temporarily"}`))
	}))
	defer server.Close()
	h := &Handler{remnawaveClient: remnawave.NewClient(server.URL, "token", "remote")}
	expiry := time.Now().Add(time.Hour)
	link := "https://example.com/sub"
	customer := &database.Customer{ID: 1, TelegramID: 99, ExpireAt: &expiry, SubscriptionLink: &link}
	if _, err := h.loadLiveSubscriptionState(context.Background(), customer); err == nil {
		t.Fatal("panel failure became empty subscription")
	}
	if customer.ExpireAt != &expiry || customer.SubscriptionLink != &link {
		t.Fatal("transient error cleared existing subscription")
	}
}

func TestChangedPanelTariffDoesNotUseOldPurchaseLabel(t *testing.T) {
	initMiniAppTestConfig()
	expiry := time.Now().Add(24 * time.Hour)
	state := &remnawave.UserState{Exists: true, Active: true, TrafficLimitBytes: 1000 * 1024 * 1024 * 1024, DeviceLimit: 10}
	payload := buildSubscriptionPayload(&database.Customer{ExpireAt: &expiry, TrialUsed: true}, &database.Purchase{Month: 1}, state)
	if payload.PlanMonths != 6 || payload.PlanLabel != "6 Месяцев" || payload.IsTrial {
		t.Fatalf("stale purchase label: %+v", payload)
	}
	state.TrafficLimitBytes = 123456
	state.DeviceLimit = 2
	payload = buildSubscriptionPayload(&database.Customer{ExpireAt: &expiry, TrialUsed: true}, &database.Purchase{Month: 1}, state)
	if payload.PlanMonths != 0 || payload.IsTrial {
		t.Fatalf("manual custom subscription became purchased plan/trial: %+v", payload)
	}
}

type entitlementReaderFake struct {
	deviceBase, deviceExtra   int
	trafficBase, trafficExtra int64
	unlimited                 bool
}

func (s entitlementReaderFake) DeviceLimitState(context.Context, int64, int64, time.Time) (int, int, error) {
	return s.deviceBase, s.deviceExtra, nil
}
func (s entitlementReaderFake) TrafficLimitState(context.Context, int64, int64) (int64, int64, bool, error) {
	return s.trafficBase, s.trafficExtra, s.unlimited, nil
}

func TestPurchasedAddonsRetainPlanWhileManualChangesStillUsePanel(t *testing.T) {
	initMiniAppTestConfig()
	expiry := time.Now().Add(time.Hour)
	base := int64(150 * 1024 * 1024 * 1024)
	extra := int64(20 * 1024 * 1024 * 1024)
	purchase := &database.Purchase{Month: 1, TrafficLimitBytes: &base}
	paid := purchaseWithEntitlements(context.Background(), entitlementReaderFake{deviceBase: 5, deviceExtra: 2, trafficBase: base, trafficExtra: extra}, 1, purchase)
	state := &remnawave.UserState{Exists: true, Active: true, TrafficLimitBytes: base + extra, DeviceLimit: 7}
	payload := buildSubscriptionPayload(&database.Customer{ExpireAt: &expiry}, paid, state)
	if payload.PlanMonths != 1 || payload.TrafficLimitBytes != base+extra || payload.DeviceLimitCount != 7 {
		t.Fatalf("add-ons erased plan: %+v", payload)
	}
	if purchase.DeviceLimitCount != nil || *purchase.TrafficLimitBytes != base {
		t.Fatal("display changed original purchase snapshot")
	}
	state.TrafficLimitBytes = 1000 * 1024 * 1024 * 1024
	state.DeviceLimit = 10
	payload = buildSubscriptionPayload(&database.Customer{ExpireAt: &expiry}, paid, state)
	if payload.PlanMonths != 6 {
		t.Fatalf("manual upgrade still shows old plan: %+v", payload)
	}
	unlimited := purchaseWithEntitlements(context.Background(), entitlementReaderFake{deviceBase: 0, deviceExtra: 2, trafficBase: base, unlimited: true}, 1, purchase)
	if *unlimited.DeviceLimitCount != 0 || *unlimited.TrafficLimitBytes != 0 {
		t.Fatal("add-ons changed unlimited semantics")
	}
}
