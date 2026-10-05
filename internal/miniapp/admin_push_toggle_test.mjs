import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const pushSource = source.slice(source.indexOf("function adminPushEnvironment()"), source.indexOf("function adminUserDisplayName("));
function deferred() { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; }
function harness({ permission = "default", response = "granted", current = null, surface = "browser", ios = false } = {}) {
  const calls = [], errors = [];
  const input = { checked: false, disabled: false };
  const subscription = { endpoint: "https://push.example/current-device", toJSON() { return { endpoint: this.endpoint, keys: { p256dh: "key", auth: "auth" } }; }, async unsubscribe() { calls.push("unsubscribe-local"); return true; } };
  const manager = {
    async getSubscription() { calls.push("get-subscription"); return current; },
    async subscribe(options) { calls.push("subscribe-local"); assert.equal(options.userVisibleOnly, true); return subscription; },
  };
  const notification = { permission, requestPermission() { calls.push("permission"); return Promise.resolve(response).then(value => { notification.permission = value; return value; }); } };
  const context = vm.createContext({
    state: { currentPage: "admin", adminSection: "home", adminPush: null, adminPushBusy: "" },
    app: { querySelector: () => input },
    navigator: { userAgent: ios ? "iPhone" : "Browser", serviceWorker: {} },
    window: { isSecureContext: true, PushManager: {}, Notification: notification, atob: () => "\0" },
    Notification: notification, clientSurface: surface, standaloneWebApp: false, previewMode: false,
    WEB_PUSH_WORKER_TIMEOUT_MS: 100, WEB_PUSH_SUBSCRIPTION_TIMEOUT_MS: 100,
    isAdminUser: () => true, localizedText: ru => ru, escapeAttribute: String, escapeHtml: String, icon: () => "",
    showToast: message => errors.push(message),
    render: () => { throw new Error("push must not remount the page"); },
    renderRealtime: () => { throw new Error("push must not remount the page"); },
    getPWAServiceWorkerRegistration: async () => { calls.push("worker"); return { pushManager: manager }; },
    withWebPushTimeout: promise => promise,
    async post(url, body) { calls.push(url); if (url.endsWith("unsubscribe")) assert.equal(body.endpoint, subscription.endpoint); return { data: { available: true, publicKey: "AA", subscriptionCount: 4 } }; },
  });
  vm.runInContext(pushSource, context);
  return { context, calls, errors, input, notification, manager, subscription };
}

test("the admin menu has a switch without opening a push page", () => {
  const h = harness();
  const html = h.context.renderAdminPushToggle("Push-уведомления");
  assert.match(html, /role="switch"/);
  assert.doesNotMatch(html, /open-admin-section|admin-push__|checked/);
  h.context.state.adminPush = { subscribed: true, permission: "granted" };
  assert.match(h.context.renderAdminPushToggle("Push"), /checked/);
  assert.doesNotMatch(source, /function renderAdminPushPage\(|\["push", "Push-уведомления"/);
});

test("permission is requested immediately before network or worker awaits; only then enable", async () => {
  const h = harness();
  const operation = h.context.setAdminPushEnabled(true);
  assert.deepEqual(h.calls, ["permission"]);
  assert.equal(h.input.checked, false);
  assert.equal(h.input.disabled, true);
  await operation;
  assert.equal(h.context.state.adminPush.subscribed, true);
  assert.equal(h.input.checked, true);
  assert.equal(h.input.disabled, false);
  assert.deepEqual(h.calls, ["permission", "/api/mini-app/admin/push/state", "worker", "get-subscription", "subscribe-local", "/api/mini-app/admin/push/subscribe"]);
});

for (const permission of ["denied", "default"]) {
  test(`declining or dismissing permission keeps switch off: ${permission}`, async () => {
    const h = harness({ response: permission });
    await h.context.setAdminPushEnabled(true);
    assert.deepEqual(h.calls, ["permission"]);
    assert.equal(h.input.checked, false);
    assert.equal(h.input.disabled, false);
    assert.equal(h.context.state.adminPushBusy, "");
  });
}

test("repeated toggles while the permission prompt is open do not overlap", async () => {
  const h = harness();
  const pending = deferred();
  h.notification.requestPermission = () => { h.calls.push("permission"); return pending.promise; };
  const first = h.context.setAdminPushEnabled(true);
  await h.context.setAdminPushEnabled(true);
  await h.context.setAdminPushEnabled(false);
  assert.deepEqual(h.calls, ["permission"]);
  pending.resolve("granted");
  await first;
  assert.equal(h.calls.filter(value => value === "subscribe-local").length, 1);
});

test("subscription errors leave the switch off and usable for retry", async () => {
  const h = harness();
  h.manager.subscribe = async () => { throw new Error("Connection failed"); };
  await h.context.setAdminPushEnabled(true);
  assert.equal(h.input.checked, false);
  assert.equal(h.input.disabled, false);
  assert.equal(h.context.state.adminPushBusy, "");
  assert.equal(h.errors.at(-1), "Connection failed");
  assert.equal(h.calls.includes("/api/mini-app/admin/push/subscribe"), false);
});

test("disable unsubscribes only the current device without requesting permission", async () => {
  const h = harness({ permission: "granted" });
  h.manager.getSubscription = async () => h.subscription;
  h.context.state.adminPush = { subscribed: true, permission: "granted" };
  await h.context.setAdminPushEnabled(false);
  assert.equal(h.input.checked, false);
  assert.deepEqual(h.calls, ["worker", "unsubscribe-local", "/api/mini-app/admin/push/unsubscribe"]);
});

test("background push check never remounts the builder and releases busy state after failure", async () => {
  const h = harness();
  const pending = deferred();
  h.context.post = () => pending.promise;
  const operation = h.context.refreshAdminPush();
  assert.equal(h.input.disabled, true);
  h.context.state.currentPage = "settings";
  h.context.state.adminLayoutEditing = true;
  pending.resolve({ data: { available: true, publicKey: "AA" } });
  await operation;
  assert.equal(h.context.state.currentPage, "settings");
  assert.equal(h.context.state.adminLayoutEditing, true);
  assert.equal(h.input.disabled, false);
  assert.equal(h.calls.includes("permission"), false);
  h.context.post = async () => { throw new Error("Offline"); };
  await h.context.refreshAdminPush();
  assert.equal(h.input.disabled, false);
});

for (const options of [{ surface: "telegram" }, { ios: true }]) {
  test(`unsupported surface gives a short explanation without enabling: ${JSON.stringify(options)}`, async () => {
    const h = harness(options);
    await h.context.setAdminPushEnabled(true);
    assert.deepEqual(h.calls, []);
    assert.equal(h.input.checked, false);
    assert.equal(h.errors.length, 1);
  });
}
