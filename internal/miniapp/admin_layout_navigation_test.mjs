import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const navigation = source.slice(source.indexOf("function setPage(page)"), source.indexOf("function getCurrentScrollTop()"));
test("builder switches dashboard and profile synchronously, with no temporal dead zone or timer", () => {
  const renders = [], events = [];
  const context = vm.createContext({
    state: { adminLayoutEditing: true, currentPage: "dashboard", adminLayoutAddMenuOpen: true, adminLayoutSelection: "dashboard:logo" },
    previousBottomNavIndex: 0,
    normalizePage: value => value,
    window: { dispatchEvent: event => events.push(event), setTimeout: () => { throw new Error("navigation must be immediate"); } },
    CustomEvent: class { constructor(type, options) { this.type = type; this.detail = options.detail; } },
    haptic: () => {}, render: options => renders.push(options),
  });
  vm.runInContext(navigation, context);
  for (const page of ["settings", "dashboard", "settings"]) {
    context.setPage(page);
    assert.equal(context.state.currentPage, page);
    assert.equal(context.state.adminLayoutCategory, page === "settings" ? "profile" : "dashboard");
    assert.equal(context.state.adminLayoutSelection, "");
    assert.equal(renders.at(-1).scrollTop, 0);
    assert.equal(context.state.adminLayoutEditing, true);
  }
  assert.equal(renders.length, 3);
  assert.equal(events.length, 3);
  context.setPage("settings");
  assert.equal(events.length, 3, "selecting the current page adds no duplicate pageview");
  context.setPage("buy");
  assert.equal(context.state.currentPage, "settings", "builder accepts only its editable screens");
});
