package remnawave

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestAdjustUserAccessAddsTrafficAndActivates(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/users/1281":
			_, _ = w.Write([]byte(`{"response":{"id":1281,"username":"link_user","status":"ACTIVE","expireAt":"2027-08-10T12:00:00Z","trafficLimitBytes":10737418240}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/users":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode patch: %v", err)
			}
			if body["status"] != "ACTIVE" || body["trafficLimitBytes"] != float64(21474836480) {
				t.Fatalf("unexpected patch: %#v", body)
			}
			_, _ = w.Write([]byte(`{"response":{"id":1281,"username":"link_user","status":"ACTIVE","expireAt":"2027-08-10T12:00:00Z","trafficLimitBytes":21474836480}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	updated, err := NewClient(server.URL, "token", "remote").AdjustUserAccess(context.Background(), 1281, uuid.Nil, 0, 10*1024*1024*1024)
	if err != nil {
		t.Fatalf("AdjustUserAccess() error = %v", err)
	}
	if updated.TrafficLimitBytes != 20*1024*1024*1024 || requests != 2 {
		t.Fatalf("unexpected result: %#v, requests=%d", updated, requests)
	}
}

func TestSetUserBlockedDisablesPanelUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"response":{"id":1281,"username":"link_user","status":"ACTIVE","expireAt":"2027-08-10T12:00:00Z"}}`))
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode patch: %v", err)
		}
		if body["status"] != "DISABLED" {
			t.Fatalf("status = %#v", body["status"])
		}
		_, _ = w.Write([]byte(`{"response":{"id":1281,"username":"link_user","status":"DISABLED","expireAt":"2027-08-10T12:00:00Z"}}`))
	}))
	defer server.Close()

	updated, err := NewClient(server.URL, "token", "remote").SetUserBlocked(context.Background(), 1281, uuid.Nil, true)
	if err != nil || updated.Status != "DISABLED" {
		t.Fatalf("SetUserBlocked() = %#v, %v", updated, err)
	}
}
