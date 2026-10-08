package remnawave

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMissingIdentityDoesNotDownloadCatalog(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/users/42" {
			t.Errorf("missing user triggered catalog request %s", r.URL.Path)
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	state, err := NewClient(server.URL, "token", "remote").GetUserStateByIdentity(context.Background(), 42, uuid.Nil)
	if err != nil || state != nil || requests != 1 {
		t.Fatalf("state=%+v err=%v requests=%d", state, err, requests)
	}
}

func TestIdentityReadUsesDirectLegacyUUIDFallback(t *testing.T) {
	identity := uuid.MustParse("10000000-0000-4000-8000-000000000042")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/users/42" {
			w.WriteHeader(400)
			return
		}
		if r.URL.Path != "/api/users/"+identity.String() {
			t.Errorf("unexpected fallback %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"response":{"uuid":"10000000-0000-4000-8000-000000000042","username":"legacy","expireAt":"2027-01-01T00:00:00Z"}}`))
	}))
	defer server.Close()
	user, err := NewClient(server.URL, "token", "remote").getPanelUserByIdentity(context.Background(), 42, identity)
	if err != nil || user == nil || user.UUID != identity {
		t.Fatalf("user=%+v err=%v", user, err)
	}
}

func TestMalformedUserAndTransientNotFoundMessageRemainErrors(t *testing.T) {
	for _, body := range []string{`{}`, `{"response":null}`, `{"response":{"id":84}}`, `{"message":"upstream user not found"}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if body == `{"message":"upstream user not found"}` {
					w.WriteHeader(503)
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			_, err := NewClient(server.URL, "token", "remote").GetUserStateByIdentity(context.Background(), 42, uuid.Nil)
			if err == nil || errors.Is(err, ErrAdminSubscriptionNotFound) {
				t.Fatalf("bad response became confirmed missing user: %v", err)
			}
		})
	}
}

func TestTelegramSelectionNeverReturnsUnrelatedUser(t *testing.T) {
	id := int64(999)
	if user := pickPanelTelegramUser([]PanelUser{{ID: 1, TelegramID: &id}}, 42); user != nil {
		t.Fatalf("unrelated user selected: %+v", user)
	}
}

func TestUUIDObservationWithoutUUIDRequiresMatchingStoredID(t *testing.T) {
	identity := uuid.MustParse("10000000-0000-4000-8000-000000000042")
	for _, storedID := range []int64{0, 42, 84} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"response":{"id":84}}`))
		}))
		user, err := NewClient(server.URL, "token", "remote").getPanelUserByIdentity(context.Background(), storedID, identity)
		server.Close()
		if storedID == 84 {
			if err != nil || user == nil || user.ID != 84 {
				t.Fatalf("matching legacy identity rejected: user=%+v err=%v", user, err)
			}
		} else if err == nil || errors.Is(err, ErrAdminSubscriptionNotFound) {
			t.Fatalf("unverified UUID observation accepted for ID %d: user=%+v err=%v", storedID, user, err)
		}
	}
}

func TestSlowDeviceEndpointDoesNotBlockSubscriptionState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client := NewClient(server.URL, "token", "remote")
	started := time.Now()
	state, err := client.userStateFromPanelUser(context.Background(), &PanelUser{ID: 42, Status: "ACTIVE", ExpireAt: time.Now().Add(time.Hour), TrafficLimitBytes: 777}, "userId", 42)
	if err != nil || state == nil || !state.Active || state.DevicesLoaded || state.TrafficLimitBytes != 777 {
		t.Fatalf("subscription lost behind HWID endpoint: %+v err=%v", state, err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("optional device request held up subscription state")
	}
}

func TestNewActivationReplacesDeletedUserWithoutUpdatingOtherSubscriptions(t *testing.T) {
	for _, primary := range []bool{true, false} {
		t.Run(map[bool]string{true: "primary", false: "secondary"}[primary], func(t *testing.T) {
			created := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/users/42":
					w.WriteHeader(404)
				case r.Method == http.MethodPost && r.URL.Path == "/api/users":
					created++
					var fields map[string]any
					if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
						t.Error(err)
					}
					want := "1_99"
					if !primary {
						want += "_s7"
					}
					if fields["username"] != want || fields["trafficLimitBytes"] != float64(777) {
						t.Errorf("wrong activation fields: %+v", fields)
					}
					_, _ = w.Write([]byte(`{"response":{"id":84,"username":"replacement","expireAt":"2027-01-01T00:00:00Z"}}`))
				default:
					t.Errorf("activation targeted another user: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			client := NewClient(server.URL, "token", "remote")
			options := ProvisioningOptions{UsernameTemplate: "{{customer_id}}_{{telegram_id}}", InternalSquadsConfigured: true, ApplySquads: true, TrafficResetStrategy: "NO_RESET"}
			user, err := client.CreateOrUpdateUserForSubscription(context.Background(), 1, 99, 7, 42, uuid.Nil, primary, 777, 2, 30, options)
			if err != nil || user == nil || user.ID != 84 || created != 1 {
				t.Fatalf("replacement user=%+v err=%v created=%d", user, err, created)
			}
			_, err = client.CreateOrUpdateUserForSubscription(context.Background(), 1, 99, 7, 42, uuid.Nil, primary, 777, 2, 0, options)
			if !errors.Is(err, ErrAdminSubscriptionNotFound) || created != 1 {
				t.Fatal("an add-on silently recreated subscription access")
			}
		})
	}
}
