package miniapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBotUpdateRequiresOwnerAndExplicitConfirmation(t *testing.T) {
	h := &Handler{}
	for _, tc := range []struct {
		name, body string
		access     adminAccess
		want       int
	}{
		{"customer", `{"confirm":true}`, adminAccess{}, 403},
		{"delegated admin", `{"confirm":true}`, adminAccess{IsAdmin: true, Permissions: []string{"status", "features", "maintenance"}}, 403},
		{"owner missing confirmation", `{}`, ownerAccess(), 400},
		{"owner false confirmation", `{"confirm":false}`, ownerAccess(), 400},
		{"invalid request", `{`, ownerAccess(), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/bot/update", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			h.handleAdminBotUpdate(w, r, &session{AdminAccess: tc.access}, nil)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestBotUpdateProgressRequiresOwner(t *testing.T) {
	h := &Handler{}
	w := httptest.NewRecorder()
	h.handleAdminBotUpdateStatus(w, httptest.NewRequest("POST", "/api/mini-app/admin/bot/update/status", nil), &session{AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"status"}}}, nil)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	if !activityReadRoutes["bot/update/status"] || activityActions["bot/update"].title == "" {
		t.Fatal("polling must not create audit noise; starts must be audited")
	}
}
