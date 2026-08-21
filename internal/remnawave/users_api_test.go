package remnawave

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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
		if got := r.URL.Query().Get("size"); got != "20" {
			t.Fatalf("unexpected page size: %s", got)
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

func TestDoAPIJSONRetriesTruncatedGETResponse(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"response":{"ok":`))
			return
		}
		_, _ = w.Write([]byte(`{"response":{"ok":true}}`))
	}))
	defer server.Close()

	var payload struct {
		Response struct {
			OK bool `json:"ok"`
		} `json:"response"`
	}
	if err := NewClient(server.URL, "token", "remote").doAPIJSON(context.Background(), http.MethodGet, "/api/test", nil, &payload); err != nil {
		t.Fatalf("doAPIJSON() error = %v", err)
	}
	if !payload.Response.OK || requests.Load() != 2 {
		t.Fatalf("payload = %#v, requests = %d", payload, requests.Load())
	}
}

func TestDoAPIJSONDoesNotRetryMutationResponse(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"id":`))
	}))
	defer server.Close()

	var payload map[string]any
	err := NewClient(server.URL, "token", "remote").doAPIJSON(context.Background(), http.MethodPatch, "/api/users", map[string]any{"id": 1}, &payload)
	if err == nil {
		t.Fatal("doAPIJSON() must reject a truncated mutation response")
	}
	if requests.Load() != 1 {
		t.Fatalf("mutation request count = %d, want 1", requests.Load())
	}
}

func TestPingUsesDedicatedSystemHealthEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/system/health" {
			t.Fatalf("unexpected healthcheck path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"isHealthy":true}}`))
	}))
	defer server.Close()

	if err := NewClient(server.URL, "token", "remote").Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}

func TestListSquadsCachesSuccessfulCatalog(t *testing.T) {
	var internalRequests atomic.Int32
	var externalRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/internal-squads":
			internalRequests.Add(1)
			_, _ = w.Write([]byte(`{"response":{"internalSquads":[{"uuid":"11111111-1111-1111-1111-111111111111","name":"Main"}]}}`))
		case "/api/external-squads":
			externalRequests.Add(1)
			_, _ = w.Write([]byte(`{"response":{"externalSquads":[{"uuid":"22222222-2222-2222-2222-222222222222","name":"External"}]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", "remote")
	for i := 0; i < 2; i++ {
		catalog, err := client.ListSquads(context.Background())
		if err != nil || len(catalog.Internal) != 1 || len(catalog.External) != 1 {
			t.Fatalf("ListSquads() = %#v, %v", catalog, err)
		}
	}
	if internalRequests.Load() != 1 || externalRequests.Load() != 1 {
		t.Fatalf("request counts = internal %d, external %d", internalRequests.Load(), externalRequests.Load())
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

func TestGetAndDeletePanelUserByNumericIdentity(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/users/1281" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":{"id":1281,"username":"15_6402520205_s2","status":"ACTIVE","expireAt":"2026-08-10T12:00:00Z","telegramId":6402520205}}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", "remote")
	user, err := client.getPanelUserByIdentity(context.Background(), 1281, uuid.Nil)
	if err != nil || user == nil || user.ID != 1281 {
		t.Fatalf("getPanelUserByIdentity() = %#v, %v", user, err)
	}
	if err := client.DeleteUser(context.Background(), 1281, uuid.Nil); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}
	if requests != 3 {
		t.Fatalf("request count = %d, want 3 (lookup plus delete lookup/delete)", requests)
	}
}

func TestPickPanelTelegramUserPrefersPrimaryUsernameSuffix(t *testing.T) {
	telegramID := int64(6402520205)
	users := []PanelUser{
		{ID: 2, Username: "15_6402520205_s2", TelegramID: &telegramID},
		{ID: 1, Username: "15_6402520205", TelegramID: &telegramID},
	}
	selected := pickPanelTelegramUser(users, telegramID)
	if selected == nil || selected.ID != 1 {
		t.Fatalf("pickPanelTelegramUser() = %#v, want primary user", selected)
	}
}

func TestPickPanelTelegramUserPrefersCustomPrimaryOverSecondary(t *testing.T) {
	telegramID := int64(6402520205)
	users := []PanelUser{
		{ID: 2, Username: "custom_name_s2", TelegramID: &telegramID},
		{ID: 1, Username: "custom_name", TelegramID: &telegramID},
	}
	selected := pickPanelTelegramUser(users, telegramID)
	if selected == nil || selected.ID != 1 {
		t.Fatalf("pickPanelTelegramUser() = %#v, want custom primary user", selected)
	}
}
