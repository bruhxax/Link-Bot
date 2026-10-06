import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const functions = source.slice(source.indexOf("function captureSwitchSelections("), source.indexOf("function render({"));

function harness({ value = "open", left = 4, width = 170, running = null, reduced = false } = {}) {
  const calls = [];
  const rect = { left, top: 4, width, height: 34 };
  const indicator = {
    dataset: { selected: value }, style: {},
    getBoundingClientRect: () => rect,
    getAnimations: () => running ? [running] : [],
    animate: (frames, options) => calls.push({ frames, options }),
  };
  const selected = { dataset: { value }, offsetLeft: left, offsetTop: 4, offsetWidth: width, offsetHeight: 34 };
  const control = {
    dataset: { animatedSwitch: "support" },
    querySelector: selector => selector === "[data-switch-indicator]" ? indicator : selected,
    getBoundingClientRect: () => ({ left: 0, top: 0 }),
  };
  const context = vm.createContext({
    app: { querySelectorAll: () => [control] },
    reducedMotionMedia: { matches: reduced }, document: { hidden: false },
  });
  vm.runInContext(functions, context);
  return { context, calls, rect, indicator, selected };
}

test("selection travels from its old visual position, including resized segments", () => {
  const { context, calls, rect, selected } = harness();
  const previous = context.captureSwitchSelections();
  selected.dataset.value = "history";
  selected.offsetLeft = rect.left = 180;
  selected.offsetWidth = rect.width = 160;
  context.mountSwitchSelections(previous);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].frames[0].transform, "translate3d(-176px, 0px, 0) scaleX(1.0625)");
  assert.equal(calls[0].options.duration, 240);
});

test("rapid reversal starts at the moving highlight rather than the previous button", () => {
  const running = { playState: "running", currentTime: 90, effect: { getTiming: () => ({ duration: 240 }) } };
  const { context, calls, rect, selected } = harness({ value: "history", left: 110, running });
  const previous = context.captureSwitchSelections();
  selected.dataset.value = "open";
  selected.offsetLeft = rect.left = 4;
  context.mountSwitchSelections(previous);
  assert.equal(calls[0].frames[0].transform, "translate3d(106px, 0px, 0) scaleX(1)");
  assert.equal(calls[0].options.duration, 240);
});

test("background rerenders finish only the remaining movement without restarting it", () => {
  const running = { playState: "running", currentTime: 60, effect: { getTiming: () => ({ duration: 150 }) } };
  const { context, calls, rect, selected } = harness({ value: "history", left: 140, running });
  const previous = context.captureSwitchSelections();
  selected.offsetLeft = rect.left = 180;
  context.mountSwitchSelections(previous);
  assert.equal(calls[0].options.duration, 90);
});

test("initial mounting, unchanged selection and reduced motion do not animate", () => {
  for (const reduced of [false, true]) {
    const { context, calls, indicator } = harness({ reduced });
    context.mountSwitchSelections();
    context.mountSwitchSelections(context.captureSwitchSelections());
    assert.equal(calls.length, 0);
    assert.equal(indicator.style.left, "4px");
    assert.equal(indicator.style.width, "170px");
  }
});
