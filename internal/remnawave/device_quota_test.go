package remnawave

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestQuotaNativeRequestsAndRecoveryDoNotResetCounters(t *testing.T) {
	created := false
	registered := false
	revoked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/users/by-username/lbq_1_1":
			if !created {
				http.Error(w, "not found", 404)
				return
			}
			w.Write([]byte(`{"response":{"id":12,"username":"lbq_1_1","tag":"LB_DEVICE_QUOTA","userTraffic":{"usedTrafficBytes":1234}}}`))
		case r.URL.Path == "/api/users" && r.Method == "POST":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if created || body["tag"] != "LB_DEVICE_QUOTA" || body["telegramId"] != nil || body["username"] != "lbq_1_1" {
				t.Errorf("unsafe child creation: %+v", body)
			}
			created = true
			w.Write([]byte(`{"response":{"id":12,"username":"lbq_1_1"}}`))
		case r.URL.Path == "/api/hwid/devices/12" && r.Method == "GET":
			if registered {
				w.Write([]byte(`{"response":{"total":1,"devices":[{"hwid":"device-first-123"}]}}`))
			} else {
				w.Write([]byte(`{"response":{"total":0,"devices":[]}}`))
			}
		case r.URL.Path == "/api/hwid/devices" && r.Method == "POST":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if registered || body["userId"] != float64(12) || body["hwid"] != "device-first-123" {
				t.Errorf("wrong HWID registration: %+v", body)
			}
			registered = true
			w.Write([]byte(`{"response":{"total":1,"devices":[]}}`))
		case r.URL.Path == "/api/users/12/actions/revoke":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["revokeOnlyPasswords"] != false || len(body) != 1 {
				t.Errorf("revoke must rotate URLs too without touching usage: %+v", body)
			}
			revoked = true
			w.Write([]byte(`{"response":{"id":12}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, "secret", "remote")
	ctx := context.Background()
	user, err := client.EnsureQuotaUser(ctx, "lbq_1_1", map[string]any{"status": "DISABLED"})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := client.EnsureQuotaUser(ctx, "lbq_1_1", nil)
	if err != nil || recovered.ID != user.ID || recovered.UserTraffic.UsedTrafficBytes != 1234 {
		t.Fatalf("recovery changed usage: %+v %v", recovered, err)
	}
	for i := 0; i < 2; i++ {
		if err = client.RegisterQuotaHWID(ctx, user, UserDevice{Hwid: "device-first-123"}); err != nil {
			t.Fatal(err)
		}
	}
	if err = client.RevokeQuotaUser(ctx, user); err != nil || !revoked {
		t.Fatalf("revoke failed: %v", err)
	}
}

func TestQuotaRecoveryRefusesUnrelatedPanelAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatal("must not mutate an unrelated account")
		}
		json.NewEncoder(w).Encode(map[string]any{"response": PanelUser{ID: 7, UUID: uuid.New(), Username: "lbq_1_1"}})
	}))
	defer server.Close()
	_, err := NewClient(server.URL, "secret", "remote").EnsureQuotaUser(context.Background(), "lbq_1_1", nil)
	if err == nil {
		t.Fatal("unrelated account was adopted as technical profile")
	}
}
