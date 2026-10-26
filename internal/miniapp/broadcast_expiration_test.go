package miniapp

import (
	"net/http/httptest"
	"strings"
	"testing"

	"link-bot/internal/broadcast"
)

func TestBroadcastExpirationRejectsInvalidRequestsAndNonAdministrators(t *testing.T) {
	h := &Handler{broadcastService: broadcast.NewService(nil, nil, nil, nil)}
	admin := &session{User: telegramUser{ID: 42}, AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"broadcast"}}}
	for _, tc := range []struct {
		body    string
		session *session
		status  int
		code    string
	}{
		{`{"deleteAfterMinutes":720}`, nil, 403, "forbidden"},
		{`{"deleteAfterMinutes":-1}`, admin, 400, "broadcast_invalid_expiration"},
		{`{"deleteAfterMinutes":2821}`, admin, 400, "broadcast_invalid_expiration"},
		{`{"deleteAfterMinutes":1.5}`, admin, 400, "invalid_request"},
		{`{"deleteAfterMinutes":720}`, admin, 409, "broadcast_draft_changed"},
	} {
		w := httptest.NewRecorder()
		h.handleAdminBroadcastExpiration(w, httptest.NewRequest("POST", "/api/mini-app/admin/broadcast/expiration", strings.NewReader(tc.body)), tc.session, nil)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("%s: status=%d response=%s", tc.body, w.Code, w.Body.String())
		}
	}
}
