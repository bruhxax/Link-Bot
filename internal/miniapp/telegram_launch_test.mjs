import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";
import { configureTelegramLaunch, telegramDevice, syncTelegramLayout } from "./static/telegram-layout.mjs";

function client(platform, overrides = {}) {
  const calls = [], events = new Map();
  const webApp = {
    platform, initData: "signed-launch", isFullscreen: false, isExpanded: false,
    isVersionAtLeast: () => true,
    requestFullscreen() { calls.push("fullscreen"); },
    exitFullscreen() { calls.push("window"); },
    expand() { calls.push("expand"); },
    onEvent(name, handler) { events.set(name, [...(events.get(name) || []), handler]); },
    ...overrides,
  };
  return {webApp, calls, emit(name, event) { for (const handler of events.get(name) || []) handler(event); }};
}

test("Android and iOS request fullscreen for both chat and main-app launch sizes", () => {
  for (const platform of ["android", "android_x", "ios"]) {
    for (const isExpanded of [false, true]) {
      const c = client(platform, {isExpanded});
      configureTelegramLaunch(c.webApp);
      assert.deepEqual(c.calls, ["fullscreen"]);
      c.webApp.isFullscreen = true;
      c.emit("fullscreenChanged");
      c.emit("viewportChanged");
      assert.deepEqual(c.calls, ["fullscreen"]);
    }
  }
});

test("phones already launched in fullscreen do not send another request", () => {
  const c = client("ios", {isFullscreen: true});
  configureTelegramLaunch(c.webApp);
  assert.deepEqual(c.calls, []);
});

test("desktop exits fullscreen for both chat and the BotFather launch mode", () => {
  for (const platform of ["tdesktop", "macos", "unigram", "windows", "linux"]) {
    for (const isFullscreen of [false, true]) {
      const c = client(platform, {isFullscreen});
      configureTelegramLaunch(c.webApp);
      assert.deepEqual(c.calls, ["window"]);
      c.webApp.isFullscreen = false;
      c.emit("fullscreenChanged");
      assert.deepEqual(c.calls, ["window"]);
    }
  }
});

test("late desktop fullscreen startup is corrected without event loops", () => {
  const c = client("tdesktop");
  configureTelegramLaunch(c.webApp);
  c.webApp.isFullscreen = true;
  for (let i = 0; i < 20; i++) c.emit("fullscreenChanged");
  assert.deepEqual(c.calls, ["window", "window"]);
});

test("unsupported fullscreen keeps mobile usable and never expands desktop", () => {
  for (const platform of ["ios", "tdesktop"]) {
    const c = client(platform, {isVersionAtLeast: () => false});
    configureTelegramLaunch(c.webApp);
    assert.deepEqual(c.calls, platform === "ios" ? ["expand"] : []);
  }
});

test("native errors fall back once and never block application startup", () => {
  const mobile = client("android");
  configureTelegramLaunch(mobile.webApp);
  mobile.emit("fullscreenFailed", {error: "UNSUPPORTED"});
  mobile.emit("fullscreenFailed", {error: "UNSUPPORTED"});
  assert.deepEqual(mobile.calls, ["fullscreen", "expand"]);
  const desktop = client("tdesktop", {exitFullscreen() { throw Error("unsupported"); }});
  assert.doesNotThrow(() => configureTelegramLaunch(desktop.webApp));
  desktop.webApp.isFullscreen = true;
  assert.doesNotThrow(() => desktop.emit("fullscreenChanged"));
  const already = client("ios");
  configureTelegramLaunch(already.webApp);
  already.emit("fullscreenFailed", {error: "ALREADY_FULLSCREEN"});
  assert.deepEqual(already.calls, ["fullscreen"]);
});

test("browsers and unknown clients never receive native presentation commands", () => {
  const browser = client("ios", {initData: ""});
  configureTelegramLaunch(browser.webApp);
  assert.deepEqual(browser.calls, []);
  const unknown = client("unknown");
  configureTelegramLaunch(unknown.webApp);
  assert.deepEqual(unknown.calls, []);
  assert.doesNotThrow(() => configureTelegramLaunch(null));
});

test("device detection trusts native platforms and handles web clients and iPads", () => {
  assert.equal(telegramDevice({platform: "tdesktop"}, {userAgent: "Mobile"}), "desktop");
  assert.equal(telegramDevice({platform: "ios"}, {userAgent: "Macintosh"}), "mobile");
  assert.equal(telegramDevice({platform: "webk"}, {userAgent: "Android Mobile"}), "mobile");
  assert.equal(telegramDevice({platform: "web"}, {userAgent: "Windows NT 10.0"}), "desktop");
  assert.equal(telegramDevice({platform: "unknown"}, {platform: "MacIntel", maxTouchPoints: 5}), "mobile");
  assert.equal(telegramDevice({platform: "unknown"}, {userAgent: "Macintosh"}), "desktop");
});

test("app startup binds the policy once and updates geometry after desktop exit", () => {
  const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
  const code = source.slice(source.indexOf("function initTelegram()"), source.indexOf("async function handlePostBootstrapFlow()"));
  const c = client("tdesktop", {isFullscreen: true, ready() {}, viewportStableHeight: 1080, safeAreaInset: {top: 24}, contentSafeAreaInset: {top: 56}});
  const values = new Map();
  const root = {dataset: {}, style: {setProperty: (key, value) => values.set(key, value)}};
  const context = vm.createContext({tg: c.webApp, navigator: {}, clientSurface: "telegram", syncTelegramLayout, configureTelegramLaunch,
    document: {documentElement: root}, window: {innerHeight: 720, addEventListener() {}}, cabinetWideMedia: {addEventListener() {}}, syncCabinetLayout() {}});
  vm.runInContext(code, context);
  context.initTelegram(); context.initTelegram();
  assert.deepEqual(c.calls, ["window"]);
  c.webApp.isFullscreen = false;
  c.webApp.isExpanded = true;
  c.webApp.viewportStableHeight = 640;
  c.webApp.safeAreaInset = {}; c.webApp.contentSafeAreaInset = {};
  c.emit("fullscreenChanged");
  assert.equal(root.dataset.launchMode, "fullsize");
  assert.equal(values.get("--app-viewport-height"), "640px");
  assert.equal(values.get("--telegram-safe-top"), "0px");
});
