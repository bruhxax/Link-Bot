package sync

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

type stateRepositoryFake struct {
	active          *database.CustomerSubscription
	expires         map[int64]*time.Time
	candidates      []database.PanelSyncCandidate
	readBeforePanel bool
	writes          map[int64]int64
	staleID         int64
}

func (r *stateRepositoryFake) ActiveSubscriptionForCustomer(context.Context, *database.Customer, bool) (*database.CustomerSubscription, error) {
	return r.active, nil
}

func (r *stateRepositoryFake) PanelSyncCandidates(context.Context) ([]database.PanelSyncCandidate, error) {
	r.readBeforePanel = true
	return r.candidates, nil
}
func (r *stateRepositoryFake) SyncPanelObservation(_ context.Context, c database.PanelSyncCandidate, id int64, _ uuid.UUID, _ *string, expire *time.Time) (bool, error) {
	if c.Subscription.ID == r.staleID {
		return false, nil
	}
	r.writes[c.Subscription.ID] = id
	if r.expires != nil {
		r.expires[c.Subscription.ID] = expire
	}
	return true, nil
}

func TestBotRefreshUsesSelectedIdentityAndKeepsDisabledExpiryInStorage(t *testing.T) {
	for _, status := range []string{"ACTIVE", "DISABLED", "EXPIRED"} {
		t.Run(status, func(t *testing.T) {
			id := int64(42)
			repo := &stateRepositoryFake{active: &database.CustomerSubscription{ID: 2, PanelUserID: &id}, writes: map[int64]int64{}, expires: map[int64]*time.Time{}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Error("bot refresh modified panel")
				}
				switch r.URL.Path {
				case "/api/users/42":
					_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"id": 42, "status": status, "expireAt": "2027-12-01T00:00:00Z", "subscriptionUrl": "https://example.com/selected", "hwidDeviceLimit": 2}})
				case "/api/hwid/devices/42":
					_, _ = w.Write([]byte(`{"response":{"total":0,"devices":[]}}`))
				default:
					t.Errorf("selected subscription fell back to %s", r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			svc := SyncService{client: remnawave.NewClient(server.URL, "token", "remote"), stateRepository: repo}
			customer := &database.Customer{ID: 1, TelegramID: 99}
			view, err := svc.RefreshCustomerSubscription(context.Background(), customer, false)
			if err != nil {
				t.Fatal(err)
			}
			if view.SubscriptionLink == nil || *view.SubscriptionLink != "https://example.com/selected" || repo.writes[2] != 42 || repo.expires[2] == nil {
				t.Fatalf("bad bot state %+v, expiry %+v", view, repo.expires[2])
			}
			if (view.ExpireAt != nil) != (status == "ACTIVE") {
				t.Fatalf("disabled access advertised: %+v", view)
			}
			if customer.ExpireAt != nil || customer.SubscriptionLink != nil {
				t.Fatal("selected subscription replaced primary cache in memory")
			}
			repo.staleID = 2
			if _, err := svc.RefreshCustomerSubscription(context.Background(), customer, false); err == nil {
				t.Fatal("bot accepted stale snapshot")
			}
		})
	}
}

func TestBackgroundRefreshKeepsBoundPrimaryIdentityAndSkipsStaleSnapshots(t *testing.T) {
	const telegramID int64 = 99
	boundID, missingID := int64(3), int64(404)
	identity := uuid.MustParse("10000000-0000-4000-8000-000000000004")
	linked := "https://example.com/3"
	repo := &stateRepositoryFake{writes: map[int64]int64{}, staleID: 6}
	for n := int64(1); n <= 8; n++ {
		repo.candidates = append(repo.candidates, database.PanelSyncCandidate{Customer: database.Customer{ID: n, TelegramID: telegramID}, Subscription: database.CustomerSubscription{ID: n, IsPrimary: true}})
	}
	repo.candidates[0].Subscription.PanelUserID = &boundID
	repo.candidates[1].Subscription.PanelUserID = &missingID
	repo.candidates[2].Subscription.PanelUserUUID = &identity
	repo.candidates[3].Subscription.SubscriptionLink = &linked
	repo.candidates[6].Subscription.IsPrimary = false
	repo.candidates[6].Subscription.PanelUserID = &boundID
	repo.candidates[7].Subscription.IsPrimary = false
	users := []remnawave.PanelUser{
		{ID: 2, TelegramID: ptr(telegramID), Username: "1_99_s2", ExpireAt: time.Now().Add(time.Hour)},
		{ID: 1, TelegramID: ptr(telegramID), Username: "1_99", ExpireAt: time.Now().Add(time.Hour)},
		{ID: 3, TelegramID: ptr(telegramID), Username: "custom", SubscriptionURL: linked, ExpireAt: time.Now().Add(time.Hour)},
		{ID: 4, UUID: identity, TelegramID: ptr(int64(500)), Username: "metadata_changed", ExpireAt: time.Now().Add(time.Hour)},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !repo.readBeforePanel {
			t.Error("local snapshot loaded after remote catalog")
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/users/stream" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"users": users, "hasMore": false}})
	}))
	defer server.Close()
	svc := SyncService{client: remnawave.NewClient(server.URL, "token", "remote"), stateRepository: repo}
	if err := svc.RefreshSubscriptionState(); err != nil {
		t.Fatal(err)
	}
	for slot, want := range map[int64]int64{1: 3, 3: 4, 4: 3, 5: 1, 7: 3} {
		if repo.writes[slot] != want {
			t.Fatalf("slot %d synced to %d, want %d", slot, repo.writes[slot], want)
		}
	}
	if _, ok := repo.writes[2]; ok {
		t.Fatal("missing bound subscription became another Telegram subscription")
	}
	if _, ok := repo.writes[6]; ok {
		t.Fatal("stale snapshot overwrote a concurrent purchase")
	}
	if _, ok := repo.writes[8]; ok {
		t.Fatal("unbound secondary subscription became the primary subscription")
	}
}
func ptr[T any](value T) *T { return &value }

func TestUnchangedPanelStateAvoidsDatabaseWrites(t *testing.T) {
	expiry := time.Now().UTC()
	link := "https://example.com/sub"
	id := int64(42)
	candidate := database.PanelSyncCandidate{Customer: database.Customer{ExpireAt: &expiry, SubscriptionLink: &link}, Subscription: database.CustomerSubscription{PanelUserID: &id, ExpireAt: &expiry, SubscriptionLink: &link}}
	user := &remnawave.PanelUser{ID: id, ExpireAt: expiry, SubscriptionURL: link}
	if !panelObservationUnchanged(candidate, user) {
		t.Fatal("unchanged state unnecessarily updated")
	}
	user.ExpireAt = expiry.Add(time.Hour)
	if panelObservationUnchanged(candidate, user) {
		t.Fatal("renewal ignored")
	}
}
