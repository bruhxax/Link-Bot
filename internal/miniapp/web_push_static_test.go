package miniapp

import (
	"strings"
	"testing"

	"link-bot/internal/runtimeconfig"
)

func TestAdminWebPushUIRequiresDirectPermissionGestureAndHomeScreen(t *testing.T) {
	appJS, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	source := string(appJS)
	for _, required := range []string{
		`Push-уведомления`,
		`Notification.requestPermission()`,
		`navigator.serviceWorker.ready`,
		`withWebPushTimeout(`,
		`push_subscribe_timeout`,
		`registration.pushManager.subscribe({`,
		`userVisibleOnly: true`,
		`applicationServerKey: urlBase64ToUint8Array`,
		`updateViaCache: "none"`,
		`renewAdminPushSubscription()`,
		`push_subscription_stale`,
		`if (subscription) remoteState = await syncAdminPushSubscription(subscription);`,
		`/api/mini-app/admin/push/subscribe`,
		`/api/mini-app/admin/push/unsubscribe`,
		`/api/mini-app/admin/push/test`,
		`Добавьте сайт на экран «Домой»`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("app.js is missing Web Push fragment %q", required)
		}
	}
}

func TestServiceWorkerAlwaysShowsPushAndOpensTarget(t *testing.T) {
	worker, err := embeddedStatic.ReadFile("static/sw.js")
	if err != nil {
		t.Fatalf("read sw.js: %v", err)
	}
	source := string(worker)
	for _, required := range []string{
		`self.addEventListener("push"`,
		`self.registration.showNotification(title, options)`,
		`self.addEventListener("notificationclick"`,
		`self.clients.openWindow(target)`,
		`self.navigator.setAppBadge`,
		`notification delivery must not depend on the icon badge`,
		`renotify: true`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("sw.js is missing Web Push fragment %q", required)
		}
	}
}

func TestManifestIsInstallableHomeScreenApp(t *testing.T) {
	settings := runtimeconfig.DefaultSettings()
	settings.Content.BrandName = "Bruh VPN"
	settings.Content.LogoURL = "/mini-app/uploads/logo-0123456789abcdef.png"
	manifest := buildPWAManifest(settings, "test-version")
	if manifest.ID != "/mini-app/" || manifest.Display != "standalone" {
		t.Fatalf("manifest is missing installable app identity: %+v", manifest)
	}
	if manifest.Name != "Bruh VPN" || manifest.ShortName != "Bruh VPN" {
		t.Fatalf("manifest brand = %q / %q", manifest.Name, manifest.ShortName)
	}
	if len(manifest.Icons) != 2 || manifest.Icons[0].Source != settings.Content.LogoURL || manifest.Icons[1].Source != settings.Content.LogoURL {
		t.Fatalf("manifest icons do not use the uploaded logo: %+v", manifest.Icons)
	}
}
