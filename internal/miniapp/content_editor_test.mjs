import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const giftInput = source.slice(source.indexOf("\t\tconst giftKind = target?.dataset?.giftKind;"), source.indexOf('\t\tif (target?.dataset?.input === "admin-subpage-scope")'));

const testContentFunction = source.slice(source.indexOf("async function testAdminContent("), source.indexOf("async function testAdminSubscriptionSuccess("));

test("content test sends the unsaved draft and releases busy state on failure", async () => {
  const content = { startTextRu: "unsaved", commerce: { paymentMethodsText: "draft methods" } };
  const state = { adminSettingsDraft: { content }, adminBusy: "", adminSettingsDirty: true };
  const requests = [];
  let fail = false;
  const context = vm.createContext({
    state, render() {}, showToast() {},
    async post(path, payload) { requests.push({ path, payload }); if (fail) throw new Error("Telegram error"); },
  });
  vm.runInContext(testContentFunction, context);
  await context.testAdminContent("start");
  assert.equal(requests[0].path, "/api/mini-app/admin/content/test");
  assert.equal(requests[0].payload.content, content);
  assert.equal(state.adminBusy, "");
  assert.equal(state.adminSettingsDirty, true);
  fail = true;
  await assert.rejects(context.testAdminContent("payment-methods"), /Telegram error/);
  assert.equal(state.adminBusy, "");
  assert.equal(state.adminSettingsDraft.content, content);
  state.adminBusy = "saving";
  await context.testAdminContent("verification");
  assert.equal(requests.length, 2);
});

test("the first gift button type change survives the editor refresh and enables saving", () => {
  const baseline = { content: { gift: { sender: { buttons: [{ type: "gift", text: "Подарок", url: "" }] } } } };
  const state = { adminSettingsDraft: structuredClone(baseline), adminSettingsDirty: false };
  let refreshes = 0, saveEnabled = false;
  const context = vm.createContext({
    state,
    render() {
      refreshes++;
      // An unmodified draft is seeded from saved settings on every render.
      if (!state.adminSettingsDirty) state.adminSettingsDraft = structuredClone(baseline);
    },
    syncAdminSaveBarDOM() { saveEnabled = state.adminSettingsDirty; },
  });
  vm.runInContext(`function input(target) { ${giftInput} }`, context);
  const dataset = { giftKind: "sender", giftIndex: "0", giftField: "type" };
  context.input({ dataset, value: "url" });
  assert.equal(refreshes, 1);
  assert.equal(state.adminSettingsDraft.content.gift.sender.buttons[0].type, "url");
  assert.equal(saveEnabled, true);
  state.adminSettingsDraft.content.gift.sender.buttons[0].url = "https://example.com";
  context.input({ dataset, value: "main" });
  assert.equal(state.adminSettingsDraft.content.gift.sender.buttons[0].type, "main");
  assert.equal(state.adminSettingsDraft.content.gift.sender.buttons[0].url, "");
  assert.equal(state.adminSettingsDraft.content.gift.sender.buttons[0].text, "Подарок");
});
