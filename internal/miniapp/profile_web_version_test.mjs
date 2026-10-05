import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const profileSource = source.slice(source.indexOf("function getProfileItems()"), source.indexOf("function renderProfileItem("));
function profile(surface, editing = false, overrides = {}) {
  const context = {
    state: { data: {}, locale: "ru", adminLayoutEditing: editing }, clientSurface: surface,
    t: () => ({}), featureEnabled: () => true, localizedText: ru => ru,
    getRuntimeSettings: () => ({ layout: { elements: [{ area: "profile", id: "web_version", visible: true }] }, content: { profileButtons: overrides } }),
    ADMIN_LAYOUT_DEFAULTS: [], deepClone: value => value,
    resolveWebVersionOverrideURL: value => value,
  };
  for (const name of ["formatServerStatusHint", "mediaLabel", "mediaHint", "linkHint", "reviewsSummaryHint", "loginMethodsLabel", "loginMethodsHint", "webVersionLabel", "webVersionHint", "addToHomeLabel", "addToHomeHint", "replaceRuntimeBrandTokens"]) context[name] = () => "label";
  vm.createContext(context);
  vm.runInContext(profileSource, context);
  return context.getProfileItems();
}
test("web account menus hide the Web version button even with a configured URL", () => {
  assert.equal(profile("browser").some(item => item.id === "web_version"), false);
  assert.equal(profile("browser", false, { web_version: { url: "https://example.com/mini-app/" } }).some(item => item.id === "web_version"), false);
});
test("Telegram and UI builder retain the Web version button", () => {
  assert.equal(profile("telegram").some(item => item.id === "web_version"), true);
  assert.equal(profile("browser", true).some(item => item.id === "web_version"), true);
});
