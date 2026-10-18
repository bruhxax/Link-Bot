package miniapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPinnedMessagePermissions(t *testing.T) {
	content := adminAccess{IsAdmin: true, Permissions: []string{"content"}}
	broadcaster := adminAccess{IsAdmin: true, Permissions: []string{"broadcast"}}
	for _, action := range []string{"state", "capture", "cancel", "toggle", "preview", "retry"} {
		path := "/api/mini-app/admin/content/pinned/" + action
		if !adminRouteAllowed(content, path) || adminRouteAllowed(broadcaster, path) || adminRouteAllowed(adminAccess{}, path) {
			t.Fatalf("wrong permissions: %s", path)
		}
	}
	if adminRouteAllowed(content, "/api/mini-app/admin/content/pinned/unknown") {
		t.Fatal("unknown action allowed")
	}
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/content/pinned/toggle", strings.NewReader(`{"enabled":true,"revision":1}`)).WithContext(context.Background())
	w := httptest.NewRecorder()
	h.handleAdminPinnedMessage(w, req, &session{AdminAccess: broadcaster}, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("unauthorized mutation: %d", w.Code)
	}
}
