package remnawave

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUserStateDistinguishesUnknownDevicesFromEmptyList(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		loaded bool
		count  int
	}{
		{name: "existing devices", status: 200, body: `{"response":{"total":2,"devices":[{"hwid":"phone"},{"hwid":"laptop"}]}}`, loaded: true, count: 2},
		{name: "confirmed empty", status: 200, body: `{"response":{"total":0,"devices":[]}}`, loaded: true},
		{name: "panel failure", status: 503, body: `{"message":"unavailable"}`},
		{name: "endpoint missing", status: 404, body: `{"message":"not found"}`},
		{name: "malformed JSON", status: 200, body: `{"response":`},
		{name: "missing response", status: 200, body: `{}`},
		{name: "null devices", status: 200, body: `{"response":{"total":0,"devices":null}}`},
		{name: "missing devices", status: 200, body: `{"response":{"total":2}}`},
		{name: "partial list", status: 200, body: `{"response":{"total":2,"devices":[{"hwid":"phone"}]}}`},
		{name: "missing HWID", status: 200, body: `{"response":{"total":1,"devices":[{"platform":"Windows"}]}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/hwid/devices/1281" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client := NewClient(server.URL, "token", "remote")
			for _, active := range []bool{true, false} {
				status := "ACTIVE"
				if !active {
					status = "DISABLED"
				}
				state, err := client.userStateFromPanelUser(context.Background(), &PanelUser{
					ID: 1281, Status: status, ExpireAt: time.Now().Add(time.Hour),
					SubscriptionURL: "https://example.com/sub", TrafficLimitBytes: 1000,
				}, "userId", 1281)
				if err != nil || state == nil || !state.Exists || state.Active != active {
					t.Fatalf("subscription lost on device read: state=%+v err=%v", state, err)
				}
				if state.DevicesLoaded != tt.loaded || state.UsedDevices != tt.count {
					t.Fatalf("DevicesLoaded=%t UsedDevices=%d, want %t %d", state.DevicesLoaded, state.UsedDevices, tt.loaded, tt.count)
				}
				if state.DevicesCheckedAt.IsZero() {
					t.Fatal("missing observation time")
				}
			}
		})
	}
}

func TestDeviceReadKeepsLegacyUUIDFallback(t *testing.T) {
	id := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/hwid/devices/1281" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path != "/api/hwid/devices/"+id.String() {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"response":{"total":1,"devices":[{"hwid":"phone"}]}}`)
	}))
	defer server.Close()
	state, err := NewClient(server.URL, "token", "remote").userStateFromPanelUser(context.Background(), &PanelUser{
		ID: 1281, UUID: id, Status: "ACTIVE", ExpireAt: time.Now().Add(time.Hour),
	}, "userId", 1281)
	if err != nil || state == nil || !state.DevicesLoaded || state.UsedDevices != 1 {
		t.Fatalf("legacy device state=%+v err=%v", state, err)
	}
}

func TestDeviceReadTimeoutDoesNotConfirmZeroDevices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	state, err := NewClient(server.URL, "token", "remote").userStateFromPanelUser(ctx, &PanelUser{
		ID: 1281, Status: "ACTIVE", ExpireAt: time.Now().Add(time.Hour),
	}, "userId", 1281)
	if err != nil || state == nil || !state.Active || state.DevicesLoaded {
		t.Fatalf("timeout state=%+v err=%v", state, err)
	}
}
