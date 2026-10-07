import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const code = source.slice(source.indexOf("function captureBottomNavSelection("), source.indexOf("function clampIndex(")) + source.slice(source.indexOf("function clampIndex("), source.indexOf("function renderSidebarPageItem("));

function harness({ style = "notch", active = 3, previous = 0, reduced = false, count = 5 } = {}) {
  const calls = [];
  const properties = new Map([["--nav-selection-x", "42px"]]);
  const nav = {
    dataset: { navStyle: style, activeIndex: active, prevIndex: previous }, offsetWidth: 358,
    style: { setProperty: (name, value) => properties.set(name, value) },
    querySelector: () => ({}),
    querySelectorAll: () => Array.from({length: count}, (_, i) => ({offsetLeft: 8 + 68 * i, offsetWidth: 68})),
    getAnimations: () => [], animate: (frames, timing) => calls.push({frames, timing}),
  };
  const context = vm.createContext({
    app: {querySelector: () => nav, querySelectorAll: () => []}, pendingBottomNavAnimation: {shouldAnimate: active !== previous},
    getComputedStyle: () => ({getPropertyValue: name => properties.get(name)}),
    window: {matchMedia: () => ({matches: reduced})},
    CSS: {registerProperty() {}}, performance: {now: () => 0},
  });
  vm.runInContext(code, context);
  return {context, nav, calls, properties};
}

test("all designs use actual button centers and finish on the selected tab", () => {
  for (const style of ["classic", "contour", "notch", "capsule"]) {
    const h = harness({style});
    h.context.syncBottomNavIndicator();
    assert.equal(h.properties.get("--nav-selection-x"), "246px");
    assert.equal(h.calls[0].frames[0]["--nav-selection-x"], "42px");
    assert.equal(h.calls[0].frames[1]["--nav-selection-x"], "246px");
  }
});

test("rapid reversals capture the coordinate currently on screen", () => {
  const h = harness({active: 0, previous: 3});
  h.properties.set("--nav-selection-x", "164px");
  h.nav.getAnimations = () => [{cancel() {}, playState: "running", currentTime: 120, effect: {getTiming: () => ({duration: 380}), getKeyframes: () => [{"--nav-selection-x": "42px"}]}}];
  const before = h.context.captureBottomNavSelection();
  before.index = 3;
  h.context.syncBottomNavIndicator(before);
  assert.equal(h.calls[0].frames[0]["--nav-selection-x"], "164px");
  assert.equal(h.calls[0].frames[1]["--nav-selection-x"], "42px");
});

test("a polling render continues only the remaining animation", () => {
  const h = harness();
  h.context.syncBottomNavIndicator({style: "notch", width: 358, index: 3, x: 180, remaining: 95});
  assert.equal(h.calls[0].timing.duration, 95);
});

test("reduced motion and settled polling snap without starting an animation", () => {
  const reduced = harness({reduced: true});
  reduced.context.syncBottomNavIndicator();
  assert.equal(reduced.calls.length, 0);
  assert.equal(reduced.properties.get("--nav-selection-x"), "246px");
  const settled = harness();
  settled.context.syncBottomNavIndicator({style: "notch", width: 358, index: 3, x: 246, remaining: 0});
  assert.equal(settled.calls.length, 0);
});

test("removing navigation entries clamps a stale selected index", () => {
  const h = harness({active: 4, previous: 4, count: 3});
  h.context.syncBottomNavIndicator();
  assert.equal(h.properties.get("--nav-selection-x"), "178px");
});

test("older Telegram webviews animate the same coordinate without CSS registration", () => {
  const h = harness();
  h.context.CSS = {};
  h.nav.isConnected = true;
  const frames = [];
  h.context.requestAnimationFrame = callback => frames.push(callback);
  h.context.syncBottomNavIndicator();
  assert.equal(h.calls.length, 0);
  frames.shift()(190);
  assert.equal(h.properties.get("--nav-selection-x"), "220.5px");
  frames.shift()(380);
  assert.equal(h.properties.get("--nav-selection-x"), "246px");
  assert.equal(h.nav.dataset.selectionEnds, undefined);
});
