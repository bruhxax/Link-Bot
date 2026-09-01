package miniapp

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestAdminUserAvatarURL(t *testing.T) {
	if got := adminUserAvatarURL(" @Example_User "); got != "https://t.me/i/userpic/320/example_user.jpg" {
		t.Fatalf("adminUserAvatarURL() = %q", got)
	}
	if got := adminUserAvatarURL(" "); got != "" {
		t.Fatalf("empty adminUserAvatarURL() = %q", got)
	}
}

func TestAdminSubscriptionStatus(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	past := time.Now().UTC().Add(-time.Hour)
	if got := adminSubscriptionStatus(&future, false); got != "active" {
		t.Fatalf("future status = %q", got)
	}
	if got := adminSubscriptionStatus(&past, false); got != "expired" {
		t.Fatalf("past status = %q", got)
	}
	if got := adminSubscriptionStatus(&future, true); got != "blocked" {
		t.Fatalf("blocked status = %q", got)
	}
}

func TestAdminUsersStaticSurface(t *testing.T) {
	appRaw, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := os.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	app := string(appRaw)
	styles := string(stylesRaw)
	for _, fragment := range []string{
		`"Пользователи"`,
		`renderAdminUsersPage`,
		`data-input="admin-users-search"`,
		`/api/mini-app/admin/users/search`,
		`/api/mini-app/admin/users/detail`,
		`/api/mini-app/admin/users/balance`,
		`/api/mini-app/admin/users/subscription`,
		`/api/mini-app/admin/users/block`,
		`Telegram ID:`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("app.js does not contain %q", fragment)
		}
	}
	for _, fragment := range []string{
		".admin-users__search",
		".admin-user-row",
		".admin-user-metrics",
		".admin-user-controls",
		"@media (prefers-reduced-motion: reduce)",
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("styles.css does not contain %q", fragment)
		}
	}
}
