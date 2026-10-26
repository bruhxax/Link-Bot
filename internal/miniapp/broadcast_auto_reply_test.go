package miniapp

import (
	"net/http/httptest"
	"strings"
	"testing"

	"link-bot/internal/broadcast"
)

func TestBroadcastAutoReplyRejectsInvalidSettingsAndUnauthorizedRequests(t *testing.T) {
	h := &Handler{broadcastService: broadcast.NewService(nil, nil, nil, nil)}
	admin := &session{User: telegramUser{ID: 42}, AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"broadcast"}}}
	for _, tc := range []struct {
		body   string
		who    *session
		status int
		code   string
	}{
		{`{}`, nil, 403, "forbidden"},
		{`{"text":"Message","windowHours":169}`, admin, 400, "broadcast_invalid_auto_reply"},
		{`{"text":"","windowHours":24}`, admin, 400, "broadcast_invalid_auto_reply"},
		{`{"text":"<b>Unclosed","windowHours":24}`, admin, 400, "broadcast_invalid_auto_reply"},
		{`{"text":"Message","windowHours":24}`, admin, 409, "broadcast_draft_changed"},
		{`{"windowHours":true}`, admin, 400, "invalid_request"},
	} {
		w := httptest.NewRecorder()
		h.handleAdminBroadcastAutoReply(w, httptest.NewRequest("POST", "/api/mini-app/admin/broadcast/auto-reply", strings.NewReader(tc.body)), tc.who, nil)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("request=%s code=%d response=%s", tc.body, w.Code, w.Body.String())
		}
	}
}
