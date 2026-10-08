package remnawave

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUserSettingsValidation(t *testing.T) {
	valid := UserSettings{ExpireAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), TrafficLimitStrategy: "MONTH"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*UserSettings){
		"negative traffic": func(s *UserSettings) { s.TrafficLimitBytes = -1 },
		"overflow traffic": func(s *UserSettings) { s.TrafficLimitBytes = 1 << 62 },
		"invalid email":    func(s *UserSettings) { s.Email = "not-an-email" },
		"invalid tag":      func(s *UserSettings) { s.Tag = "invalid tag" },
		"long tag":         func(s *UserSettings) { s.Tag = "TAG_TOO_LONG_123456" },
		"invalid telegram": func(s *UserSettings) { n := int64(-1); s.TelegramID = &n },
		"strategy":         func(s *UserSettings) { s.TrafficLimitStrategy = "arbitrary" },
		"expiry":           func(s *UserSettings) { s.ExpireAt = time.Time{} },
		"devices":          func(s *UserSettings) { n := -1; s.HwidDeviceLimit = &n },
		"empty squad":      func(s *UserSettings) { s.ActiveInternalSquads = []uuid.UUID{uuid.Nil} },
		"duplicate squad":  func(s *UserSettings) { id := uuid.New(); s.ActiveInternalSquads = []uuid.UUID{id, id} },
	} {
		t.Run(name, func(t *testing.T) {
			s := valid
			change(&s)
			if s.Validate() == nil {
				t.Fatal("accepted invalid settings")
			}
		})
	}
}

func TestUpdateAdminUserSettingsPreservesIdentityAndStatus(t *testing.T) {
	patches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/users/42":
			_, _ = w.Write([]byte(`{"response":{"id":42,"status":"DISABLED","username":"original","expireAt":"2027-01-01T00:00:00Z"}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/users":
			patches++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["id"] != float64(42) {
				t.Fatal("identity not selected server-side")
			}
			for _, key := range []string{"status", "username"} {
				if _, ok := body[key]; ok {
					t.Fatalf("unexpected security-sensitive field %s", key)
				}
			}
			for _, key := range []string{"telegramId", "email", "description", "tag", "externalSquadUuid", "hwidDeviceLimit"} {
				if value, ok := body[key]; !ok || value != nil {
					t.Fatalf("empty %s must be explicit null", key)
				}
			}
			if squads, ok := body["activeInternalSquads"].([]any); !ok || len(squads) != 0 {
				t.Fatal("empty squads must be an array")
			}
			_, _ = w.Write([]byte(`{"response":{"id":42,"status":"DISABLED","expireAt":"2027-01-01T00:00:00Z"}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, "token", "remote")
	_, err := client.UpdateAdminUserSettings(context.Background(), 42, uuid.Nil, UserSettings{ExpireAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), TrafficLimitStrategy: "NO_RESET"})
	if err != nil || patches != 1 {
		t.Fatalf("patches=%d error=%v", patches, err)
	}
}
