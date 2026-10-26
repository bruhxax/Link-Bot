package miniapp

import (
	"link-bot/internal/broadcast"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSurveyEndpointsValidateBeforeStorageAndRequireBroadcastPermission(t *testing.T) {
	h := &Handler{broadcastService: broadcast.NewService(nil, nil, nil, nil)}
	admin := &session{User: telegramUser{ID: 42}, AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"broadcast"}}}
	limited := &session{User: telegramUser{ID: 42}, AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"integrations"}}}
	for _, tc := range []struct {
		body string
		who  *session
		code int
	}{
		{`{}`, nil, 403}, {`{}`, limited, 403}, {`{"survey":{"enabled":true}}`, admin, 400}, {`{"survey":{"enabled":false}}`, admin, 409}, {`{"survey":true}`, admin, 400},
	} {
		w := httptest.NewRecorder()
		h.handleAdminBroadcastSurvey(w, httptest.NewRequest("POST", "/survey", strings.NewReader(tc.body)), tc.who, nil)
		if w.Code != tc.code {
			t.Fatalf("code=%d %s", w.Code, w.Body.String())
		}
	}
	for _, body := range []string{`{"id":"bad"}`, `{"before":-1}`, `{"before":true}`} {
		w := httptest.NewRecorder()
		h.handleAdminBroadcastSurveyResults(w, httptest.NewRequest("POST", "/results", strings.NewReader(body)), admin, nil)
		if w.Code != 400 {
			t.Fatal(w.Body.String())
		}
	}
}
