import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { renderAdminActivity } from "./static/admin-activity.mjs";
import { renderAdministrators } from "./static/administrators.mjs";

const escape = (value) => String(value).replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll('"', "&quot;");
const helpers = { escapeHtml: escape, escapeAttribute: escape, icon: name => `<svg data-icon="${name}"></svg>`, avatar: () => "<i></i>", displayName: user => user.username };
const entry = { id: 7, title: "Изменил баланс", category: "users", status: "success", actorName: "operator", actorRole: "Поддержка", targetName: "tester", targetTelegramId: 22, createdAt: "2026-10-06T08:00:00Z", details: [{ label: "Баланс", before: "0", after: "100" }] };

test("activity shows dated readable target, outcome and expandable changes with escaped text", () => {
  const result = renderAdminActivity({ items: [{ ...entry, title: '<img src=x onerror="bad()">', details: [{ label: "Баланс", before: "0", after: "<script>bad()</script>" }] }], hasMore: true }, helpers);
  assert.match(result, /<details/); assert.match(result, /6 октября 2026/); assert.match(result, /@tester/); assert.match(result, /Выполнено/); assert.match(result, /admin-activity__before">0/); assert.match(result, /Показать ещё/);
  assert.doesNotMatch(result, /<img|<script>/); assert.match(result, /&lt;script>/);
});

test("new administrators have no history tab; existing and owner cards do", () => {
  const state = { editor: { username: "operator", role: "Поддержка", permissions: ["users"], color: "#55c58a", isNew: false }, catalog: [{ id: "users", label: "Пользователи", group: "Операции" }], activity: { items: [entry] }, editorTab: "activity" };
  assert.match(renderAdministrators(state, helpers), /Изменил баланс/);
  assert.doesNotMatch(renderAdministrators(state, helpers), /administrators-save/);
  state.editor.isNew = true;
  assert.doesNotMatch(renderAdministrators(state, helpers), /data-action="administrators-tab"/);
});

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const code = source.slice(source.indexOf("async function refreshAdministratorActivity("), source.indexOf("function editAdministrator("));
function setup() {
  const pending = []; const activity = { requestID: 0, items: [], query: "", category: "", hasMore: false };
  const administrators = { activity, editor: { telegramId: 22 }, editorTab: "activity" };
  const context = vm.createContext({ administrators, state: { currentPage: "admin", adminSection: "administrators" }, canAdmin: () => true, renderAdministratorsPreservingFocus() {}, post: (url, body) => new Promise(resolve => pending.push({ url, body, resolve })) });
  vm.runInContext(code, context); return { context, pending, administrators, activity };
}

test("late search response cannot replace newer history or another administrator", async () => {
  const { context, pending, administrators, activity } = setup();
  const first = context.refreshAdministratorActivity(); activity.query = "баланс"; const second = context.refreshAdministratorActivity();
  pending[1].resolve({ data: { items: [entry], hasMore: false } }); await second;
  pending[0].resolve({ data: { items: [{ id: 3 }], hasMore: true } }); await first;
  assert.equal(activity.items[0].id, 7); assert.equal(activity.loading, false);
  const third = context.refreshAdministratorActivity(); administrators.editor = { telegramId: 33 };
  pending[2].resolve({ data: { items: [{ id: 9 }] } }); await third;
  assert.equal(activity.items[0].id, 7);
});

test("pagination sends stable cursor, prevents concurrent append and deduplicates", async () => {
  const { context, pending, activity } = setup(); activity.items = [entry]; activity.hasMore = true;
  const first = context.refreshAdministratorActivity({ append: true }); await context.refreshAdministratorActivity({ append: true });
  assert.equal(pending.length, 1); assert.equal(pending[0].body.beforeId, 7); assert.equal(pending[0].body.telegramId, 22);
  pending[0].resolve({ data: { items: [entry, { ...entry, id: 6 }], hasMore: false } }); await first;
  assert.deepEqual(Array.from(activity.items, item => item.id), [7, 6]); assert.equal(activity.hasMore, false);
});

test("history fetch is forbidden when owner access is lost", async () => {
  const { context, pending } = setup(); context.canAdmin = () => false;
  await context.refreshAdministratorActivity(); assert.equal(pending.length, 0);
});

test("one card visit and ten realtime refreshes request exactly one audited open", async () => {
  const calls = [];
  const detail = { customerId: 9, subscriptions: [] };
  const context = vm.createContext({
    state: { data: {}, currentPage: "admin", adminSection: "users", adminUsers: { items: [] } },
    window: { clearTimeout() {} }, document: { hidden: false }, previewMode: false,
    adminUsersSearchTimer: 0, adminUsersSearchRequestID: 0, adminUserDetailRequestID: 0,
    realtimeRefreshRunning: false, realtimeRefreshPending: false, realtimeBatching: false, realtimeBatchRenderRequested: false, realtimeLastRefresh: 0,
    hasAuth: () => true, isAdminUser: () => true, haptic() {}, render() {}, renderRealtime() {}, refreshDashboard: async () => {}, refreshAdminDirectMessage() {},
    post: async (url, body) => { calls.push({ url, body }); return { data: detail }; },
  });
  vm.runInContext(source.slice(source.indexOf("async function openAdminUser("), source.indexOf("function closeAdminUserDetail(")), context);
  vm.runInContext(source.slice(source.indexOf("async function refreshRealtimeData("), source.indexOf("async function startRealtimeSync(")), context);
  await context.openAdminUser(9);
  for (let i = 0; i < 10; i++) await context.refreshRealtimeData();
  assert.equal(calls.filter(call => call.url.endsWith("/users/open")).length, 1);
  assert.equal(calls.filter(call => call.url.endsWith("/users/detail")).length, 10);
  assert.ok(calls.every(call => call.body.customerId === 9));
  assert.equal(context.state.adminUserDetail.customerId, 9);
  await context.openAdminUser(9);
  assert.equal(calls.filter(call => call.url.endsWith("/users/open")).length, 2);
});

test("live subscription reconciliation uses reads while user searches remain audited", async () => {
  const calls = [];
  const context = vm.createContext({
    state: { data: {}, currentPage: "admin", adminSection: "subscriptions", adminSubscriptionQuery: "tester", adminSubscriptionResult: { id: 9 }, adminSubscriptionTargetResult: {}, adminSubscriptionTargetTelegramID: 22 },
    document: { hidden: false }, previewMode: false, hasAuth: () => true, isAdminUser: () => true,
    realtimeRefreshRunning: false, realtimeRefreshPending: false, realtimeBatching: false, realtimeBatchRenderRequested: false, realtimeLastRefresh: 0,
    refreshDashboard: async () => {}, renderRealtime() {}, render() {},
    post: async (url) => { calls.push(url); return { data: { id: 9 } }; },
  });
  vm.runInContext(source.slice(source.indexOf("async function refreshRealtimeData("), source.indexOf("async function startRealtimeSync(")), context);
  for (let i = 0; i < 5; i++) await context.refreshRealtimeData();
  assert.equal(calls.length, 10);
  assert.ok(calls.every(url => url.endsWith("/refresh")));
  context.showToast = () => {}; context.mapApiErrorMessage = value => value;
  vm.runInContext(source.slice(source.indexOf("async function findAdminSubscription("), source.indexOf("async function loadAdminSubscriptionTarget(")), context);
  await context.findAdminSubscription();
  assert.equal(calls.at(-1), "/api/mini-app/admin/subscriptions/find");
});
