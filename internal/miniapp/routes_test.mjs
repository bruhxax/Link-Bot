import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";
import { CUSTOMER_ROUTES, ADMIN_SECTIONS, readAppRoute, appRoutePath, cabinetNavigationURL, browserLogoutURL } from "./static/routes.mjs";

test("all cabinet sections have clean reloadable routes, with or without a subdomain", () => {
  for (const [page, route] of Object.entries(CUSTOMER_ROUTES)) {
    for (const cabinetDedicated of [false, true]) {
      const path = appRoutePath({page, cabinetDedicated});
      assert.equal(path, (cabinetDedicated ? "" : "/cabinet") + "/" + route);
      assert.equal(readAppRoute({pathname: path}).page, page);
      assert.ok(!path.includes("?") && !path.includes("mini-app"));
    }
  }
  const custom = appRoutePath({page: "custom-page", customPageID: "help-guide", cabinetDedicated: true});
  assert.equal(custom, "/pages/help-guide");
  assert.equal(readAppRoute({pathname: custom}).customPageID, "help-guide");
});

test("admin sections and shared pages retain their context on reload", () => {
  for (const section of ADMIN_SECTIONS) {
    for (const adminDedicated of [false, true]) {
      const pathname = appRoutePath({page: "admin", section, adminDedicated});
      const route = readAppRoute({pathname, adminDedicated});
      assert.equal(route.page, "admin");
      assert.equal(route.section || "home", section);
    }
  }
  assert.equal(appRoutePath({page: "support", adminWorkspace: true, adminDedicated: true}), "/support");
  assert.equal(readAppRoute({pathname: "/support", adminDedicated: true}).page, "admin");
  assert.equal(readAppRoute({pathname: "/support"}).page, "support");
  assert.equal(readAppRoute({pathname: "/", adminDedicated: true}).section, "home");
  assert.equal(readAppRoute({pathname: "/admin/"}).section, "home");
  assert.equal(readAppRoute({pathname: "/does-not-exist"}), null);
});

test("every section offered by the settings editor has a reloadable route", () => {
  const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
  const catalog = source.slice(source.indexOf("const ADMIN_SEARCH_SECTIONS ="), source.indexOf("const ADMIN_SEARCH_CONTENT_SECTIONS ="));
  for (const match of catalog.matchAll(/\["([a-z-]+)"\s*,/g)) {
    assert.ok(ADMIN_SECTIONS.includes(match[1]), "Missing route for " + match[1]);
    assert.equal(readAppRoute({pathname: "/" + match[1], adminDedicated: true}).section, match[1]);
  }
});

test("legacy links still resolve and the cabinet auth URL never uses the admin host", () => {
  assert.deepEqual(readAppRoute({pathname: "/mini-app/", search: "?cabinet=1&page=admin&section=content"}), {page: "admin", section: "content"});
  assert.equal(readAppRoute({pathname: "/mini-app/", search: "?page=buy"}).page, "buy");
  const config = {cabinetBaseURL: "https://my.example.com", publicBaseURL: "https://example.com", origin: "https://admin.example.com"};
  assert.equal(cabinetNavigationURL(config), "https://my.example.com/dashboard");
  assert.equal(browserLogoutURL(config), "https://my.example.com/logout");
  assert.equal(browserLogoutURL({...config, origin: "https://my.example.com"}), "https://my.example.com/login");
  assert.equal(browserLogoutURL({...config, cabinetBaseURL: ""}), "https://example.com/cabinet/logout");
  assert.ok(readAppRoute({pathname: "/logout"}).logout);
  assert.ok(readAppRoute({pathname: "/cabinet/login"}).login);
});

test("navigation replaces the legacy URL once and records real section changes for Back", () => {
  const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
  const code = source.slice(source.indexOf("let lastPersistedNavigation ="), source.indexOf("function readSessionSetting("));
  let current = new URL("https://my.example.com/mini-app/?cabinet=1&page=buy&planId=3m#tgWebAppData=signed");
  const updates = [];
  const update = method => (_, __, path) => { updates.push({method, path}); current = new URL(path, current); };
  const location = { get href() {return current.href;}, get search() {return current.search;} };
  const state = {currentPage: "buy", adminSection: "home", adminWorkspace: false};
  const context = vm.createContext({URL, state, appRoutePath, adminDedicated: false, cabinetDedicated: true, writeSetting() {}, STORAGE_KEYS: {}, window: {location, history: {replaceState: update("replace"), pushState: update("push")}}});
  vm.runInContext(code, context);
  context.persistNavigationState();
  assert.deepEqual(updates[0], {method: "replace", path: "/plans?planId=3m#tgWebAppData=signed"});
  state.currentPage = "payments";
  context.persistNavigationState();
  assert.equal(updates[1].method, "push");
  assert.equal(current.pathname, "/payments");
  context.persistNavigationState();
  assert.equal(updates.length, 2);
});
