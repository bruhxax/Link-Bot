package miniapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

type testAdministratorLookup struct {
	record *database.Administrator
	err    error
	calls  int
}

func (f *testAdministratorLookup) AdministratorForTelegramID(context.Context, int64) (*database.Administrator, error) {
	f.calls++
	return f.record, f.err
}

func TestAdministratorStatusOnlyAccessAndDefaultDeny(t *testing.T) {
	a := adminAccess{IsAdmin: true, Permissions: []string{"status"}}
	if !adminRouteAllowed(a, "/api/mini-app/admin/status") {
		t.Fatal("status denied")
	}
	for _, route := range []string{"users/search", "users/balance", "finance", "settings/update", "ai/settings", "reviews/rewards", "servers/visibility", "administrators/list", "administrators/save", "administrators/remove", "unknown"} {
		if adminRouteAllowed(a, "/api/mini-app/admin/"+route) {
			t.Errorf("status role gained %s", route)
		}
	}
}

func TestEveryRegisteredAdminEndpointHasAnExplicitPolicy(t *testing.T) {
	raw, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatal(err)
	}
	paths := regexp.MustCompile(`mux.HandleFunc\("(/api/mini-app/admin/[^"\s]+)"`).FindAllStringSubmatch(string(raw), -1)
	all := adminAccess{IsAdmin: true}
	for _, p := range adminPermissions {
		all.Permissions = append(all.Permissions, p.ID)
	}
	for _, path := range paths {
		if strings.HasPrefix(path[1], "/api/mini-app/admin/administrators/") {
			continue
		}
		if !adminRouteAllowed(all, path[1]) {
			t.Errorf("missing policy for %s", path[1])
		}
		if adminRouteAllowed(adminAccess{}, path[1]) {
			t.Errorf("ordinary user can access %s", path[1])
		}
	}
	if all.can("administrators") {
		t.Fatal("delegate can manage roles")
	}
}

func TestAdministratorDependenciesAndSeparateActions(t *testing.T) {
	a := adminAccess{IsAdmin: true, Permissions: []string{"users", "support.view", "servers.view"}}
	for _, p := range []string{"users.balance", "users.subscription", "users.block", "users.message", "support.reply", "support.close", "servers.manage", "reviews.rewards"} {
		if a.can(p) {
			t.Errorf("view permissions granted %s", p)
		}
	}
	a.Permissions = []string{"users.balance", "support.reply", "servers.manage"}
	for _, p := range a.Permissions {
		if a.can(p) {
			t.Errorf("missing dependency accepted for %s", p)
		}
	}
	a.Permissions = []string{"users", "users.balance"}
	if !adminRouteAllowed(a, "/api/mini-app/admin/users/balance") {
		t.Fatal("authorized balance change denied")
	}
}

func TestAdministratorAccessIsReloadedAndRevocationIsImmediate(t *testing.T) {
	initMiniAppTestConfig()
	lookup := &testAdministratorLookup{record: &database.Administrator{Role: "Наблюдатель", Color: "#aa8bd4", Permissions: []string{"status"}}}
	h := &Handler{administratorRepository: lookup}
	sess := &session{User: telegramUser{ID: 22}}
	if err := h.resolveAdminAccess(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if !sess.canAdmin("status") || sess.access().IsOwner {
		t.Fatal("delegated access not resolved")
	}
	lookup.record = nil
	if err := h.resolveAdminAccess(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if sess.isAdministrator() || sess.canAdmin("status") {
		t.Fatal("revoked access survived")
	}
	if lookup.calls != 2 {
		t.Fatal("access was cached")
	}
	lookup.err = errors.New("database unavailable")
	if h.resolveAdminAccess(context.Background(), sess) == nil || sess.isAdministrator() {
		t.Fatal("lookup failure did not fail closed")
	}
	owner := &session{User: telegramUser{ID: config.GetAdminTelegramId()}}
	if err := h.resolveAdminAccess(context.Background(), owner); err != nil || !owner.canAdmin("administrators") {
		t.Fatal("owner depends on delegated role records")
	}
}

func TestRestrictedAdminRawAPIRequestsAreRejectedBeforeHandlers(t *testing.T) {
	initMiniAppTestConfig()
	login, err := createTelegramBrowserSessionData(telegramUser{ID: 22}, config.TelegramToken(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"users/balance", "settings/update", "reviews/rewards", "administrators/save", "administrators/remove"} {
		t.Run(route, func(t *testing.T) {
			h := &Handler{administratorRepository: &testAdministratorLookup{record: &database.Administrator{Permissions: []string{"status"}}}}
			req := httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/"+route, strings.NewReader(`{"settings":{"features":{"support":false}},"permissions":["administrators"]}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Telegram-Login-Data", login)
			recorder := httptest.NewRecorder()
			h.withSession(func(http.ResponseWriter, *http.Request, *session, *database.Customer) {
				t.Fatal("forbidden handler was invoked")
			}).ServeHTTP(recorder, req)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestScopedSettingsCannotOverwriteOtherSections(t *testing.T) {
	current := runtimeconfig.DefaultSettings()
	current.HiddenServerNodes = []string{"private-node"}
	next := runtimeconfig.DefaultSettings()
	next.Appearance.Glow = true
	next.Maintenance.Enabled = true
	next.Features["support"] = false
	next.HiddenServerNodes = []string{"injected"}
	merged, err := mergeAdminSettings(current, next, "appearance")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(merged.Appearance, next.Appearance) {
		t.Fatal("appearance edit was lost")
	}
	if merged.Maintenance.Enabled || !merged.Features["support"] || !reflect.DeepEqual(merged.HiddenServerNodes, current.HiddenServerNodes) {
		t.Fatal("settings outside permitted section were changed")
	}
	if _, err := mergeAdminSettings(current, next, "administrators"); err == nil {
		t.Fatal("unknown section accepted")
	}
}

func TestAdministratorValidationRejectsPrivilegeEscalation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		permissions []string
		color       string
	}{
		{"unknown permission", []string{"administrators"}, "#69a8d4"},
		{"missing support view", []string{"support.reply"}, "#69a8d4"},
		{"missing users view", []string{"users.balance"}, "#69a8d4"},
		{"invalid color", []string{"status"}, "red;display:none"},
		{"empty permissions", nil, "#69a8d4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := database.Administrator{CustomerID: 2, Role: "Роль", Color: tc.color, Permissions: tc.permissions}
			if validateAdministrator(&a) == nil {
				t.Fatal("invalid role accepted")
			}
		})
	}
	a := database.Administrator{CustomerID: 2, Role: " Поддержка ", Color: "#55C58A", Permissions: []string{"support.reply", "support.view", "support.reply"}}
	if err := validateAdministrator(&a); err != nil {
		t.Fatal(err)
	}
	if a.Role != "Поддержка" || a.Color != "#55c58a" || !reflect.DeepEqual(a.Permissions, []string{"support.view", "support.reply"}) {
		t.Fatalf("not normalized: %+v", a)
	}
}

func TestAdminBootstrapDoesNotLoadPrivateCollectionsWithoutPermission(t *testing.T) {
	h := &Handler{walletRepository: &database.WalletRepository{}, promoCodeRepository: &database.PromoCodeRepository{}}
	a := adminAccess{IsAdmin: true, Permissions: []string{"status"}}
	payload, err := h.buildAdminPayload(context.WithValue(context.Background(), adminAccessContextKey{}, a))
	if err != nil {
		t.Fatal(err)
	}
	if payload.Access.IsOwner || len(payload.Integrations) != 0 || len(payload.Events) != 0 || len(payload.Withdrawals) != 0 || len(payload.PromoCodes) != 0 {
		t.Fatalf("private collections in limited bootstrap: %+v", payload)
	}
}

func TestAdministratorBotPermissionsFollowRoleAndRevocation(t *testing.T) {
	initMiniAppTestConfig()
	lookup := &testAdministratorLookup{record: &database.Administrator{IsOwner: true, Permissions: []string{"users", "users.message"}}}
	h := &Handler{administratorRepository: lookup}
	if !h.HasAdminPermission(context.Background(), 22, "users.message") || h.HasAdminPermission(context.Background(), 22, "broadcast", "administrators") {
		t.Fatal("bot does not enforce current delegated permissions")
	}
	lookup.record = nil
	if h.HasAdminPermission(context.Background(), 22, "users.message") {
		t.Fatal("revoked admin can still capture bot messages")
	}
}

func TestBlockingCannotDeleteSubscriptionsWithoutSeparatePermission(t *testing.T) {
	initMiniAppTestConfig()
	sess := &session{User: telegramUser{ID: 22}, AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"users", "users.block"}}}
	req := httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/users/block", strings.NewReader(`{"customerId":33,"blocked":true,"deleteSubscription":true}`))
	recorder := httptest.NewRecorder()
	(&Handler{}).handleAdminUserBlock(recorder, req, sess, nil)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("delete through blocking status=%d", recorder.Code)
	}
}

func TestMissingAccessContextDoesNotBecomeOwner(t *testing.T) {
	if accessFromContext(context.Background()).IsOwner {
		t.Fatal("missing context grants owner access")
	}
}

func TestSettingsResponsesDoNotRevealHiddenNodesWithoutPermission(t *testing.T) {
	settings := runtimeconfig.DefaultSettings()
	settings.HiddenServerNodes = []string{"private-node"}
	if len(adminSettingsForAccess(settings, adminAccess{IsAdmin: true, Permissions: []string{"appearance"}}).HiddenServerNodes) != 0 {
		t.Fatal("appearance admin receives hidden nodes")
	}
	if len(adminSettingsForAccess(settings, adminAccess{IsAdmin: true, Permissions: []string{"servers.view"}}).HiddenServerNodes) != 1 {
		t.Fatal("node viewer loses visibility settings")
	}
}
