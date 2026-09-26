package remnawave

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetDeviceLimitOnlyChangesLimitAndSkipsDuplicatePatch(t *testing.T) {
	limit := 6
	patches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch && r.URL.Path == "/api/users" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["expireAt"] != nil || body["status"] != nil || body["trafficLimitBytes"] != nil {
				t.Fatalf("changed subscription access: %#v", body)
			}
			limit = int(body["hwidDeviceLimit"].(float64))
			patches++
		} else if r.Method != http.MethodGet || r.URL.Path != "/api/users/1281" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"id": 1281, "uuid": "00000000-0000-4000-8000-000000000001", "hwidDeviceLimit": limit, "expireAt": "2027-10-10T12:00:00Z", "status": "ACTIVE"}})
	}))
	defer server.Close()
	client := NewClient(server.URL, "token", "remote")
	for i := 0; i < 2; i++ {
		u, err := client.SetDeviceLimit(context.Background(), 10, 1281, uuid.Nil, 5)
		if err != nil || u == nil || u.HwidDeviceLimit == nil || *u.HwidDeviceLimit != 5 {
			t.Fatalf("limit update: user=%+v err=%v", u, err)
		}
	}
	if patches != 1 {
		t.Fatalf("patches=%d, want one idempotent update", patches)
	}
}
