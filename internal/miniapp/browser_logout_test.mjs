import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
function section(start, end) {
  const first = source.indexOf(start), last = source.indexOf(end, first);
  assert.ok(first >= 0 && last > first);
  return source.slice(first, last);
}
const credentials = ["telegram-token", "browser-session", "google-token"];
function store() {
  const values = new Map();
  return {values, getItem: key => values.get(key) || null,
    setItem: (key, value) => values.set(key, value), removeItem: key => values.delete(key)};
}
function harness({telegram = false, localStorage = store(), sessionStorage = store()} = {}) {
  const listeners = {};
  const redirects = [];
  let aborted = false;
  const context = vm.createContext({
    clientSurface: telegram ? "telegram" : "browser", tg: telegram ? {initData: "telegram-init"} : null,
    BROWSER_LOGOUT_KEY: "logout-stamp", browserLogoutStamp: localStorage.getItem("logout-stamp") || "", browserLoggingOut: false,
    STORAGE_KEYS: {telegramIDToken: credentials[0], telegramLogin: credentials[1], googleLogin: credentials[2], page: "page"},
    state: {data: {user: {id: 123}}, adminLayoutEditing: false},
    window: {localStorage, sessionStorage, location: {replace: url => redirects.push(url)},
      addEventListener: (name, fn) => { listeners[name] = fn; }},
    realtimeAbortController: {abort() { aborted = true; }},
    stopTelegramQRLogin() {}, closeSupportThreadState() {}, clearGoogleAuth() {},
    escapeHtml: value => value, localizedText: ru => ru,
    setTimeout, clearTimeout, AbortController, requestTimeoutForURL: () => 1000,
    getBrowserAuthHeaders: async () => ({"X-Telegram-Login-Data": "saved-session"}),
    t: () => ({timeout: "timeout"}), mapApiErrorMessage: (_, message) => message,
  });
  vm.runInContext(
    section("function isBrowserSessionCurrent()", "function isInstallGuideMode()") +
    section("function readSetting(", "let lastPersistedNavigation") +
    section("function readSessionSetting(", "function normalizePage(") +
    section("function renderBrowserLogoutButton(", "function renderPages()") +
    section("async function post(", "async function getJSON("), context);
  return {context, localStorage, sessionStorage, listeners, redirects, get aborted() { return aborted; }};
}

test("logout clears every login method in both stores and opens a clean sign-in URL", () => {
  const page = harness();
  for (const key of credentials) {
    page.localStorage.setItem(key, "persistent");
    page.sessionStorage.setItem(key, "legacy");
  }
  page.localStorage.setItem("appearance", "keep");
  page.context.logoutBrowser();
  for (const key of credentials) {
    assert.equal(page.localStorage.getItem(key), null);
    assert.equal(page.sessionStorage.getItem(key), null);
  }
  assert.equal(page.aborted, true);
  assert.equal(page.context.state.data, null);
  assert.equal(page.localStorage.getItem("appearance"), "keep");
  assert.deepEqual(page.redirects, ["/mini-app/?cabinet=1"]);
  page.context.logoutBrowser();
  assert.equal(page.redirects.length, 1);
  // A new page cannot restore any of the old credentials.
  const reopened = harness({localStorage: page.localStorage, sessionStorage: page.sessionStorage});
  assert.equal(reopened.context.readSessionSetting(credentials[1], ""), "");
  assert.equal(reopened.context.isBrowserSessionCurrent(), true);
});

test("late server and identity callbacks cannot persist a session after logout", () => {
  const page = harness();
  page.context.logoutBrowser();
  page.context.persistBrowserSessionFromResponse({headers: {get: () => "late-session"}});
  for (const key of credentials) page.context.writeSessionSetting(key, "late-identity");
  for (const key of credentials) assert.equal(page.localStorage.getItem(key), null);
});

test("other tabs reject stale credentials before the storage event and then sign out", () => {
  const shared = store();
  const first = harness({localStorage: shared});
  const second = harness({localStorage: shared});
  second.sessionStorage.setItem(credentials[1], "legacy-session");
  first.context.logoutBrowser();
  second.context.writeSessionSetting(credentials[1], "late-response");
  assert.equal(second.context.readSessionSetting(credentials[1], ""), "");
  assert.equal(shared.getItem(credentials[1]), null);
  const stamp = shared.getItem("logout-stamp");
  second.listeners.storage({key: "logout-stamp", newValue: stamp});
  assert.equal(shared.getItem("logout-stamp"), stamp); // No event broadcast loop.
  assert.equal(second.sessionStorage.getItem(credentials[1]), null);
  assert.deepEqual(second.redirects, ["/mini-app/?cabinet=1"]);
});

test("clearing legacy session credentials still works if persistent storage is unavailable", () => {
  const inaccessible = {getItem: () => null, setItem() { throw Error("denied"); }, removeItem() { throw Error("denied"); }};
  const page = harness({localStorage: inaccessible});
  credentials.forEach(key => page.sessionStorage.setItem(key, "legacy"));
  page.context.logoutBrowser();
  credentials.forEach(key => assert.equal(page.sessionStorage.getItem(key), null));
  assert.equal(page.redirects.length, 1);
});

test("the button and logout behavior are restricted to the web cabinet", () => {
  const web = harness();
  assert.match(web.context.renderBrowserLogoutButton(), /data-action="browser-logout"/);
  assert.match(web.context.renderBrowserLogoutButton(), />Выйти</);
  const telegram = harness({telegram: true});
  assert.equal(telegram.context.renderBrowserLogoutButton(), "");
  telegram.context.logoutBrowser();
  assert.equal(telegram.redirects.length, 0);
  assert.equal(telegram.aborted, false);
});

test("logout during response decoding rejects old account payloads for JSON and uploads", async () => {
  for (const method of ["post", "postForm"]) {
    const page = harness();
    let complete;
    const payload = new Promise(resolve => { complete = resolve; });
    page.context.fetch = async () => ({ok: true, headers: {get: () => ""}, json: () => payload});
    const pending = page.context[method]("/api/mini-app/bootstrap", {});
    await new Promise(resolve => setImmediate(resolve));
    page.context.logoutBrowser();
    complete({ok: true, data: {user: {id: 123}}});
    await assert.rejects(pending, /Browser session ended/);
    assert.equal(page.context.state.data, null);
  }
});

test("logout while auth headers are preparing prevents sending the request", async () => {
  const page = harness();
  let complete;
  page.context.getBrowserAuthHeaders = () => new Promise(resolve => { complete = resolve; });
  page.context.fetch = () => { throw Error("should not send"); };
  const pending = page.context.post("/api/mini-app/bootstrap", {});
  page.context.logoutBrowser();
  complete({});
  await assert.rejects(pending, /Browser session ended/);
});
