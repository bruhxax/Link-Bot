import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const app = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const save = app.slice(app.indexOf("async function saveAdminIntegration(provider)"), app.indexOf("async function refreshAdminMoyNalog("));

function harness({ enabled = true, methods = [{ id: "card" }, { id: "payhot" }], failure = false } = {}) {
  const requests = [], messages = [], renders = [];
  const state = {
    adminIntegrationBusy: "",
    adminIntegrationDrafts: { payhot: { enabled, fields: { apiKey: "", webhookSecret: "", paymentMethod: "sbp" } } },
    data: { paymentMethods: [{ id: "card" }], admin: { integrations: [{ id: "payhot", enabled: false, configured: false }] } },
  };
  const context = vm.createContext({
    state, render: options => renders.push(options), showToast: message => messages.push(message),
    async post(url, body) {
      requests.push({ url, body });
      if (failure) throw new Error("Save failed");
      return { data: { id: "payhot", kind: "payment", enabled, configured: true, fields: [{ key: "apiKey", secret: true, configured: true }, { key: "webhookSecret", secret: true, configured: true }, { key: "paymentMethod", value: "sbp" }] }, paymentMethods: methods };
    },
  });
  vm.runInContext(save, context);
  return { context, state, requests, messages, renders };
}

test("enabling PayHot updates checkout in the same open session", async () => {
  const h = harness();
  await h.context.saveAdminIntegration("payhot");
  assert.deepEqual(h.state.data.paymentMethods.map(method => method.id), ["card", "payhot"]);
  assert.equal(h.state.data.admin.integrations[0].enabled, true);
  assert.equal(h.requests[0].body.enabled, true);
  assert.equal(h.state.adminIntegrationDrafts.payhot.fields.apiKey, "");
  assert.equal(h.state.adminIntegrationDrafts.payhot.fields.webhookSecret, "");
  assert.match(h.messages[0], /включён/);
  assert.ok(h.renders.every(options => options.preserveScroll));
});

test("disabling a provider removes it from checkout and reports it as disabled", async () => {
  const h = harness({ enabled: false, methods: [{ id: "card" }] });
  h.state.data.paymentMethods.push({ id: "payhot" });
  await h.context.saveAdminIntegration("payhot");
  assert.deepEqual(h.state.data.paymentMethods.map(method => method.id), ["card"]);
  assert.match(h.messages[0], /выключен/);
});

test("an empty server list replaces old checkout methods", async () => {
  const h = harness({ enabled: false, methods: [] });
  await h.context.saveAdminIntegration("payhot");
  assert.deepEqual(h.state.data.paymentMethods, []);
});

test("failed saves keep the previous checkout methods and draft", async () => {
  const h = harness({ failure: true });
  await assert.rejects(h.context.saveAdminIntegration("payhot"), /Save failed/);
  assert.deepEqual(h.state.data.paymentMethods.map(method => method.id), ["card"]);
  assert.equal(h.state.adminIntegrationBusy, "");
  assert.equal(h.state.adminIntegrationDrafts.payhot.enabled, true);
  assert.deepEqual(h.messages, []);
});
