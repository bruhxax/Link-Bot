import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const extract = (start, end) => source.slice(source.indexOf(start), source.indexOf(end, source.indexOf(start)));
const liveCode = extract("async function refreshRealtimeData(", "async function startRealtimeSync(");
function setup(overrides = {}) {
  const requests = [];
  let renders = 0;
  const data = { user: { panelUsername: "old" }, runtime: {}, meta: {}, subscriptions: { activeId: 1 }, subscription: { userId: 42, trafficUsedBytes: 100, devices: [{hwid: "phone"}], deviceUsedCount: 1, devicesLoaded: true }, trial: {} };
  const context = vm.createContext({
    state: { data, currentPage: "dashboard", adminSection: "users", adminUsers: {items:[]}, adminUserSelectedSubscriptionID: "1" },
    document: { hidden: false }, window: { clearTimeout() {}, setTimeout() {} }, console: { warn() {} },
    dashboardDataVersion: 0, dashboardRefreshPromise: null, dashboardRetryAt: 0, dashboardHydrationTimer: null,
    adminUserDetailRequestID: 0, adminUsersSearchRequestID: 0,
    realtimeStarted: true, realtimeRefreshRunning: false, realtimeRefreshPending: false, realtimeLastRefresh: 0,
    realtimeBatching: false, realtimeBatchRenderRequested: false, previewMode: false, tg: null,
    hasAuth: () => true, isAdminUser: () => true, render() { renders++; }, renderRealtime() { renders++; },
    syncGiftReceiptState() {}, syncLocalizationFromSettings() {}, syncAdminSettingsDraft() {}, ensureSelections() {},
    queueRealtimeRefresh() {}, haptic() {}, showToast() {}, refreshAdminUsers: async () => {},
    post: (url, body) => new Promise((resolve, reject) => requests.push({url, body, resolve, reject})),
    ...overrides,
  });
  vm.runInContext(liveCode, context);
  return { context, requests, renders: () => renders };
}
const tick = () => new Promise(resolve => setImmediate(resolve));

test("admin card updates before a slow bootstrap and survives bootstrap failure", async () => {
  let rejectBootstrap;
  const {context, requests, renders} = setup({ refreshDashboard: () => new Promise((_, reject) => {rejectBootstrap = reject;}) });
  context.state.currentPage = "admin";
  context.state.adminUserDetail = {customerId:9, subscriptions:[]};
  const running = context.refreshRealtimeData();
  assert.equal(requests.length, 1);
  requests[0].resolve({data:{customerId:9, subscriptions:[{id:1, usedTrafficBytes:123}]}});
  await tick();
  assert.equal(context.state.adminUserDetail.subscriptions[0].usedTrafficBytes, 123);
  assert.equal(renders(), 1);
  rejectBootstrap(new Error("offline"));
  await running;
  assert.equal(context.realtimeRefreshRunning, false);
});

test("late admin poll cannot undo a saved subscription change", async () => {
  const {context, requests} = setup({refreshDashboard:async()=>{}});
  context.state.currentPage = "admin";
  context.state.adminUserDetail = {customerId:9, subscriptions:[{id:1, usedTrafficBytes:10}]};
  vm.runInContext(extract("async function runAdminUserAction(", "async function creditAdminUserBalance("), context);
  const polling = context.refreshRealtimeData();
  const mutation = context.runAdminUserAction("/save", {}, "subscription");
  requests[1].resolve({data:{customerId:9, subscriptions:[{id:1, usedTrafficBytes:99}]}});
  await mutation;
  requests[0].resolve({data:{customerId:9, subscriptions:[{id:1, usedTrafficBytes:10}]}});
  await polling;
  assert.equal(context.state.adminUserDetail.subscriptions[0].usedTrafficBytes, 99);
});

test("older bootstrap cannot replace selected subscription or new purchase data", async () => {
  const {context, requests} = setup();
  vm.runInContext(extract("async function refreshDashboard(", "function browserDeviceSeed("), context);
  vm.runInContext(extract("function applySubscriptionBootstrap(", "async function selectSubscription("), context);
  const old = context.refreshDashboard({silent:true});
  context.applySubscriptionBootstrap({runtime:{}, user:{}, subscription:{userId:84, trafficLimitBytes:2000}, subscriptions:{activeId:2}});
  requests[0].resolve({data:{runtime:{}, user:{}, subscription:{userId:42, trafficLimitBytes:100}, subscriptions:{activeId:1}}});
  await old;
  assert.equal(context.state.data.subscriptions.activeId, 2);
  assert.equal(context.state.data.subscription.trafficLimitBytes, 2000);
});

test("purchase reconciliation waits out an old refresh then fetches fresh data while checkout is busy", async () => {
  const {context, requests} = setup();
  vm.runInContext(extract("async function refreshDashboard(", "function browserDeviceSeed("), context);
  vm.runInContext(extract("async function safeRefresh(", "function moveToDashboard("), context);
  const old = context.refreshDashboard({silent:true});
  context.state.busyMethod = "wallet";
  const purchase = context.safeRefresh();
  requests[0].resolve({data:{runtime:{}, subscription:{trafficLimitBytes:100}, subscriptions:{activeId:1}}});
  await old; await tick();
  assert.equal(requests.length, 2);
  requests[1].resolve({data:{runtime:{}, subscription:{trafficLimitBytes:9000}, subscriptions:{activeId:1}}});
  await purchase;
  assert.equal(context.state.data.subscription.trafficLimitBytes, 9000);
});

test("panel polling is lightweight and rejects a response after a subscription switch", async () => {
  const {context, requests} = setup();
  const polling = context.refreshRealtimeData({panelOnly:true});
  assert.equal(requests[0].url,"/api/mini-app/subscription/state");
  context.dashboardDataVersion++;
  context.state.data.subscriptions.activeId = 2;
  requests[0].resolve({data:{panelUsername:"obsolete", subscription:{userId:42,trafficUsedBytes:200},subscriptions:{activeId:1}}});
  await polling;
  assert.equal(context.state.data.subscriptions.activeId,2);
  assert.equal(context.state.data.user.panelUsername,"old");
});

test("panel/HWID outages retain observed traffic and devices, but confirmed empty lists clear devices", () => {
  const {context} = setup();
  const previous = context.state.data.subscription;
  const outage = context.mergeSubscriptionObservation(previous, {userId:42,stateLoaded:false,trafficUsedBytes:0});
  assert.equal(outage.trafficUsedBytes,100);
  const hwidOutage = context.mergeSubscriptionObservation(previous, {userId:42,stateLoaded:true,devicesLoaded:false,trafficUsedBytes:300,deviceUsedCount:0,devices:[]});
  assert.equal(hwidOutage.trafficUsedBytes,300);
  assert.equal(hwidOutage.deviceUsedCount,1);
  const empty = context.mergeSubscriptionObservation(previous, {userId:42,stateLoaded:true,devicesLoaded:true,deviceUsedCount:0,devices:[]});
  assert.equal(empty.deviceUsedCount,0);
  assert.equal(empty.devices.length,0);
  const different = context.mergeSubscriptionObservation(previous, {userId:84,stateLoaded:true,devicesLoaded:false,deviceUsedCount:0,devices:[]});
  assert.equal(different.devices.length,0);
  const detail = context.mergeAdminUserObservation({customerId:9,subscriptions:[{id:1,usedTrafficBytes:777,devices:[{Hwid:"phone"}]}]}, {customerId:9,subscriptions:[{id:1,status:"unavailable",usedTrafficBytes:0,isSelected:true}]});
  assert.equal(detail.subscriptions[0].usedTrafficBytes,777);
});

test("customer snapshots update without downloading dashboard sections and invalidate a slower bootstrap", async () => {
  const {context, requests, renders} = setup();
  const polling = context.refreshRealtimeData({panelOnly:true});
  requests[0].resolve({data:{panelUsername:"current",subscription:{userId:42,trafficUsedBytes:456,devicesLoaded:true,deviceUsedCount:0,devices:[]},subscriptions:{activeId:1},trial:{eligible:false}}});
  await polling;
  assert.equal(context.state.data.subscription.trafficUsedBytes,456);
  assert.equal(context.state.data.user.panelUsername,"current");
  assert.equal(context.dashboardDataVersion,1);
  assert.equal(requests.length,1);
  assert.equal(renders(),1);
});

test("hidden tabs skip remote polling", async () => {
  const {context, requests} = setup();
  context.document.hidden = true;
  await context.refreshRealtimeData({panelOnly:true});
  assert.equal(requests.length,0);
});

test("mutations invalidate both earlier reads and reads started before commit; polls do not", async () => {
  for (const path of ["/api/mini-app/purchase", "/api/mini-app/trial/activate", "/api/mini-app/promocode/redeem", "/api/mini-app/subscriptions/select", "/api/mini-app/admin/users/subscription/settings", "/api/mini-app/admin/subscriptions/rebind", "/api/mini-app/subscription/state"]) {
    let complete;
    const context = vm.createContext({
      dashboardDataVersion:0, AbortController, setTimeout, clearTimeout,
      requestTimeoutForURL:()=>1000, tg:{initData:"test"}, isBrowserSessionCurrent:()=>true,
      persistBrowserSessionFromResponse(){},
      fetch:async()=>({ok:true,headers:{get:()=>null},json:()=>new Promise(resolve=>{complete=resolve;})}),
    });
    vm.runInContext(extract("function changesSubscriptionState(","async function postForm("), context);
    const pending = context.post(path,{});
    const changes = !path.endsWith("/subscription/state");
    assert.equal(context.dashboardDataVersion,changes?1:0);
    await tick();complete({ok:true,data:{}});await pending;
    assert.equal(context.dashboardDataVersion,changes?2:0);
  }
});
