self.addEventListener("install", () => {
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data?.json() || {};
  } catch (_) {
    data = { body: event.data?.text() || "" };
  }

  const title = String(data.title || "Новое событие");
  const options = {
    body: String(data.body || "Откройте Link-Bot, чтобы посмотреть подробности"),
    tag: String(data.tag || "admin-event"),
    renotify: true,
    data: {
      url: String(data.url || "/mini-app/"),
    },
  };

  event.waitUntil((async () => {
    await self.registration.showNotification(title, options);
    if (Number(data.badge || 0) > 0 && "setAppBadge" in self.navigator) {
      try {
        await self.navigator.setAppBadge(Number(data.badge));
      } catch (_) { /* notification delivery must not depend on the icon badge */ }
    }
  })());
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const target = new URL(event.notification.data?.url || "/mini-app/", self.location.origin).href;
  event.waitUntil((async () => {
    if ("clearAppBadge" in self.navigator) {
      try { await self.navigator.clearAppBadge(); } catch (_) { /* continue opening the app */ }
    }
    const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
    const current = windows.find((client) => new URL(client.url).origin === self.location.origin);
    if (current) {
      await current.navigate(target);
      return current.focus();
    }
    return self.clients.openWindow(target);
  })());
});

self.addEventListener("message", (event) => {
  if (event.data?.type !== "CLEAR_APP_BADGE" || !("clearAppBadge" in self.navigator)) return;
  event.waitUntil(self.navigator.clearAppBadge().catch(() => {}));
});
