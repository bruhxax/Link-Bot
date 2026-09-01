package miniapp

import (
	"strings"
	"testing"
)

func TestAdminUsersSearchKeepsFocusAndDetailOpensWithoutListRefresh(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}

	app := string(appRaw)
	for _, expected := range []string{
		`adminUsersSearchRequestID += 1`,
		`captureAdminUsersSearchFocus()`,
		`restoreAdminUsersSearchFocus(searchFocus)`,
		`state.adminUserPending = summary`,
		`renderAdminUserDetailLoading(state.adminUserPending)`,
		`state.adminUserDetailSettled = true`,
	} {
		if !strings.Contains(app, expected) {
			t.Fatalf("admin users interaction fragment is missing: %q", expected)
		}
	}

	styles := string(stylesRaw)
	for _, expected := range []string{
		`.admin-users__search input:focus-visible { outline: 0 !important; box-shadow: none; }`,
		`.admin-user-detail--settled { animation: none; }`,
		`.admin-user-detail__loading {`,
	} {
		if !strings.Contains(styles, expected) {
			t.Fatalf("admin users interaction style is missing: %q", expected)
		}
	}
}
