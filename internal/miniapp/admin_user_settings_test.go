package miniapp

import "testing"

func TestPanelUserSettingsRequireSubscriptionPermission(t *testing.T) {
	const route = "/api/mini-app/admin/users/subscription/settings"
	for _, a := range []adminAccess{{}, {IsAdmin: true, Permissions: []string{"users"}}, {IsAdmin: true, Permissions: []string{"users.balance"}}} {
		if adminRouteAllowed(a, route) {
			t.Fatal("read-only or balance access allowed panel changes")
		}
	}
	if !adminRouteAllowed(adminAccess{IsAdmin: true, Permissions: []string{"users", "users.subscription"}}, route) {
		t.Fatal("subscription editor denied")
	}
}
