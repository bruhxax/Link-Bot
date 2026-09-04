package miniapp

import (
	"strings"
	"testing"
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
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("sw.js is missing Web Push fragment %q", required)
		}
	}
}

func TestManifestIsInstallableHomeScreenApp(t *testing.T) {
	manifest, err := embeddedStatic.ReadFile("static/manifest.webmanifest")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	source := string(manifest)
	if !strings.Contains(source, `"id": "/mini-app/"`) || !strings.Contains(source, `"display": "standalone"`) {
		t.Fatalf("manifest is missing installable app identity: %s", source)
	}
}
