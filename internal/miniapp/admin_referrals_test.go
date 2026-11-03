package miniapp

import "testing"

func TestReferralReportRequiresReferralPermission(t *testing.T) {
	path := "/api/mini-app/admin/referrals"
	if adminRouteAllowed(adminAccess{IsAdmin: true, Permissions: []string{"users"}}, path) {
		t.Fatal("users permission must not expose referral report")
	}
	if !adminRouteAllowed(adminAccess{IsAdmin: true, Permissions: []string{"referrals"}}, path) {
		t.Fatal("referrals permission must expose referral report")
	}
	if adminRouteAllowed(adminAccess{}, path) {
		t.Fatal("non-administrator can access referral report")
	}
	if _, audited := activityRoute(path); audited {
		t.Fatal("read-only referral report should not create activity log entries")
	}
}
