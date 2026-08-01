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

func TestGetPanelUserByTelegramIDUsesV3Stream(t *testing.T) {
	const telegramID int64 = 6402520205
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/stream" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("telegramId"); got != "6402520205" {
			t.Fatalf("unexpected telegramId: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"users":[{"id":1281,"shortUuid":"abc123","username":"link_user","status":"ACTIVE","expireAt":"2026-08-10T12:00:00Z","telegramId":6402520205,"subscriptionUrl":"https://example.com/sub"}],"nextCursor":null,"hasMore":false}}`))
	}))
	defer server.Close()

	user, err := NewClient(server.URL, "token", "remote").getPanelUserByTelegramID(context.Background(), telegramID)
	if err != nil {
		t.Fatalf("getPanelUserByTelegramID() error = %v", err)
	}
	if user == nil || user.ID != 1281 || user.UUID != uuid.Nil || user.TelegramID == nil || *user.TelegramID != telegramID {
		t.Fatalf("unexpected user: %#v", user)
	}
}

func TestPatchPanelUserUsesNumericID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/users" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["id"] != float64(1281) {
			t.Fatalf("expected numeric id, got %#v", body["id"])
		}
		if _, ok := body["uuid"]; ok {
			t.Fatalf("v3 request must not contain uuid")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"id":1281,"username":"link_user","status":"ACTIVE","expireAt":"2026-08-10T12:00:00Z","subscriptionUrl":"https://example.com/sub"}}`))
	}))
	defer server.Close()

	updated, err := NewClient(server.URL, "token", "remote").patchPanelUser(context.Background(), &PanelUser{ID: 1281}, map[string]any{"description": "Link | @link"})
	if err != nil {
		t.Fatalf("patchPanelUser() error = %v", err)
	}
	if updated.ID != 1281 {
		t.Fatalf("unexpected updated user: %#v", updated)
	}
}

func TestDeleteHWIDDeviceUsesUserID(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/hwid/devices/delete" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["userId"] != float64(1281) || body["hwid"] != "device-1" {
			t.Fatalf("unexpected request body: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"total": 1, "devices": []map[string]any{{"hwid": "device-2", "userId": 1281, "createdAt": now, "updatedAt": now}}}})
	}))
	defer server.Close()

	devices, err := NewClient(server.URL, "token", "remote").DeleteUserHWIDDevice(context.Background(), 1281, uuid.Nil, "device-1")
	if err != nil {
		t.Fatalf("DeleteUserHWIDDevice() error = %v", err)
	}
	if len(devices) != 1 || devices[0].UserID != 1281 || devices[0].Hwid != "device-2" {
		t.Fatalf("unexpected devices: %#v", devices)
	}
}

func TestDoAPIJSONAcceptsEmptyNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := NewClient(server.URL, "token", "remote").doAPIJSON(context.Background(), http.MethodDelete, "/api/users/1281", nil, nil); err != nil {
		t.Fatalf("doAPIJSON() error = %v", err)
	}
}
