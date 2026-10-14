import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { validDeviceTrafficRules, sortedDeviceTrafficRules, deviceTrafficLabels, deviceTrafficProgress } from "./static/device-traffic.mjs";

test("traffic annotations sort, localize, and reject ambiguous or invalid rows", () => {
  const rules = [{devices: 2, trafficGb: 40}, {devices: 1, trafficGb: 20}];
  assert.deepEqual(deviceTrafficLabels(rules), ["1 устройство — 20 GB", "2 устройства — 40 GB"]);
  assert.deepEqual(deviceTrafficLabels(rules, "en"), ["1 device — 20 GB", "2 devices — 40 GB"]);
  assert.match(deviceTrafficLabels([{devices: 11, trafficGb: 20}])[0], /11 устройств/);
  assert.equal(rules[0].devices, 2);
  for (const bad of [null, [], [{devices: 0, trafficGb: 20}], [{devices: 1.5, trafficGb: 20}], [{devices: 1, trafficGb: 0}], [{devices: 1, trafficGb: "<img>"}], [rules[0], rules[0]], [{devices: 1001, trafficGb: 20}], [{devices: 1, trafficGb: 1000001}]]) {
    assert.equal(validDeviceTrafficRules(bad), false);
    assert.deepEqual(deviceTrafficLabels(bad), []);
  }
});

test("real device quotas use ordinal labels and only confirmed traffic counters", () => {
  assert.deepEqual(deviceTrafficLabels([{devices:1,trafficGb:20},{devices:2,trafficGb:40}], "ru", true), ["Устройство №1 — 20 GB", "Устройство №2 — 40 GB"]);
  assert.equal(deviceTrafficProgress({}), null);
  assert.deepEqual(deviceTrafficProgress({slot:1}), {unknown:true});
  assert.deepEqual(deviceTrafficProgress({slot:1,trafficUsedBytes:null,trafficLimitBytes:20}), {unknown:true});
  assert.deepEqual(deviceTrafficProgress({slot:1,trafficUsedBytes:7,trafficLimitBytes:20}), {used:7,limit:20,percent:35,exceeded:false});
  assert.deepEqual(deviceTrafficProgress({trafficUsedBytes:21,trafficLimitBytes:20}), {used:21,limit:20,percent:100,exceeded:true});
  assert.deepEqual(deviceTrafficProgress({trafficUsedBytes:42,trafficLimitBytes:0}), {used:42,limit:0,percent:0,exceeded:false});
});

test("editor applies optional annotations without changing actual plan limits", () => {
  const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
  const draft = {id: "custom", months: 1, days: 0, priceRub: 100, trafficGb: 100, deviceLimit: 5,
    deviceTrafficEnabled: true, deviceTrafficRules: [{devices: 2, trafficGb: 40}, {devices: 1, trafficGb: 20}]};
  let toast = "";
  const context = vm.createContext({state: {locale: "ru", adminPlanDurationUnit: "months", adminPlanEditingID: "custom", adminPlanFormDraft: draft, adminSettingsDraft: {plans: [{id: "custom"}]}},
    validDeviceTrafficRules, sortedDeviceTrafficRules, showToast: (message) => {toast = message;}, getPlanDurationTitle: () => "1 месяц", ensureSelections() {}, haptic() {}, render() {}});
  const start = source.indexOf("function applyAdminPlanEdit()");
  vm.runInContext(source.slice(start, source.indexOf("function deleteAdminPlan(", start)), context);
  context.applyAdminPlanEdit();
  assert.equal(toast, "");
  const result = context.state.adminSettingsDraft.plans[0];
  assert.equal(result.trafficGb, 100);
  assert.equal(result.deviceLimit, 5);
  assert.deepEqual(result.deviceTrafficRules, [{devices: 1, trafficGb: 20}, {devices: 2, trafficGb: 40}]);
  context.state.adminPlanFormDraft = {...draft, deviceTrafficRules: [{devices: 1, trafficGb: ""}]};
  context.state.adminPlanEditingID = "custom";
  context.applyAdminPlanEdit();
  assert.match(toast, /каждой строке/);
  assert.equal(context.state.adminSettingsDraft.plans[0], result);
  context.state.adminPlanFormDraft.deviceTrafficEnabled = false;
  context.applyAdminPlanEdit();
  assert.equal(context.state.adminSettingsDraft.plans[0].deviceTrafficEnabled, false);
  assert.equal(context.state.adminSettingsDraft.plans[0].deviceTrafficRules.length, 0);
});
