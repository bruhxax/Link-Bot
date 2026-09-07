package miniapp

import (
	"strings"
	"testing"

	"link-bot/internal/database"
)

func TestSubPageAdminAndDynamicCatalogAreWired(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	setupRaw, err := embeddedStatic.ReadFile("static/setup-apps.js")
	if err != nil {
		t.Fatalf("read setup-apps.js: %v", err)
	}
	openerRaw, err := embeddedStatic.ReadFile("static/open-app.js")
	if err != nil {
		t.Fatalf("read open-app.js: %v", err)
	}

	app := string(appRaw)
	for _, fragment := range []string{
		`["Sub page", "", "subpage", "adminSubscriptions"]`,
		`function renderAdminSubPagePage()`,
		`data-action="admin-add-subpage-client"`,
		`data-action="admin-remove-subpage-client"`,
		`data-input="admin-subpage-scope"`,
		`data-input="admin-subpage-platform"`,
		`["additional_subscriptions", "Доп. подписки"`,
		`getSetupPlatforms(getSubPageSettings())`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("app.js does not contain %q", fragment)
		}
	}

	setup := string(setupRaw)
	for _, fragment := range []string{`export function getSetupPlatforms(settings = null)`, `client.allPlatforms`, `client.platforms.includes(platform.id)`, `apps.unshift(app)`} {
		if !strings.Contains(setup, fragment) {
			t.Fatalf("setup-apps.js does not contain %q", fragment)
		}
	}

	opener := string(openerRaw)
	for _, fragment := range []string{`fetch("/api/mini-app/public-config"`, `payload?.data?.subPage`, `buildSetupClientURL(platformID, appID, subscription, subPageSettings)`} {
		if !strings.Contains(opener, fragment) {
			t.Fatalf("open-app.js does not contain %q", fragment)
		}
	}
	clearFragment := strings.Index(opener, `window.history.replaceState(null, "", window.location.pathname);`)
	fetchConfig := strings.Index(opener, `const subPageSettings = await loadSubPageSettings();`)
	if clearFragment < 0 || fetchConfig < 0 || clearFragment > fetchConfig {
		t.Fatal("open-app.js must clear the subscription fragment before loading public settings")
	}
}

func TestSubscriptionSwitcherIsAResizableDashboardElement(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}

	app := string(appRaw)
	for _, fragment := range []string{
		`["dashboard", "subscription_switcher", 0, 36, 38, false, "center"]`,
		`{ subscription_switcher: subscriptionSwitcher }`,
		`"width", item.width, 24, 100, "%"`,
		`"height", item.height, 32, 72, " px"`,
		`if (!featureEnabled("additional_subscriptions") && !state.adminLayoutEditing) return "";`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("app.js does not contain %q", fragment)
		}
	}

	styles := string(stylesRaw)
	for _, fragment := range []string{
		`.runtime-layout-item[data-runtime-id="subscription_switcher"] {`,
		`container-type: size;`,
		`border-radius: var(--runtime-corner-radius, 15px);`,
		`text-overflow: ellipsis;`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("styles.css does not contain %q", fragment)
		}
	}
}

func TestAdditionalSubscriptionsFeatureUsesOnlyPrimarySubscription(t *testing.T) {
	active := database.CustomerSubscription{ID: 2, DisplayName: "Extra"}
	subscriptions := []database.CustomerSubscription{
		{ID: 2, DisplayName: "Extra"},
		{ID: 1, DisplayName: "Primary", IsPrimary: true},
	}

	selected, visible := primarySubscriptionOnly(&active, subscriptions)
	if selected == nil || selected.ID != 1 || len(visible) != 1 || visible[0].ID != 1 {
		t.Fatalf("primarySubscriptionOnly() = %+v, %+v", selected, visible)
	}
	if feature := runtimeFeatureForPath("/api/mini-app/subscriptions/create"); feature != "additional_subscriptions" {
		t.Fatalf("runtimeFeatureForPath() = %q", feature)
	}
}
