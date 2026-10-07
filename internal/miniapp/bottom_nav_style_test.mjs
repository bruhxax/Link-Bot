import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const code = source.slice(source.indexOf("function captureBottomNavSelection("), source.indexOf("function renderSidebarPageItem("));

function harness({style = "notch", active = 3, previous = 0, reduced = false, count = 5} = {}) {
  let now = 0;
  const frames = [];
  const properties = new Map();
  const paths = new Map();
  const path = name => ({style: {}, setAttribute: (key, value) => paths.set(name, value)});
  const makeNav = () => {
    const items = Array.from({length: count}, (_, i) => ({
      offsetLeft: 14 + 54 * i, offsetWidth: 44, offsetTop: 6, offsetHeight: 38,
      querySelector: () => ({innerHTML: `<svg data-icon="${i}"></svg>`}),
      style: {setProperty(name, value) { this.values.set(name, value); }, values: new Map()},
    }));
    return {
      items, dataset: {navStyle: style, activeIndex: active, prevIndex: previous},
      clientWidth: 290, clientHeight: 50, isConnected: true,
      style: {setProperty: (name, value) => properties.set(name, value)},
      querySelector: name => name === ".bottom-nav__indicator" ? {querySelector: () => path("contour")} : path(name),
      querySelectorAll: () => items,
    };
  };
  let nav = makeNav();
  const context = vm.createContext({
    app: {querySelector: () => nav, querySelectorAll: () => []},
    bottomNavMotions: new WeakMap(), pendingBottomNavAnimation: {shouldAnimate: active !== previous},
    window: {matchMedia: () => ({matches: reduced})}, performance: {now: () => now},
    requestAnimationFrame: callback => frames.push(callback),
  });
  vm.runInContext(code, context);
  return {
    context, properties, paths, frames, get nav() {return nav;},
    tick(time) {now = time; const callback = frames.shift(); assert.ok(callback); callback(time);},
    replace() {nav.isConnected = false; nav = makeNav(); return nav;},
  };
}

test("every design finishes exactly at the selected button center", () => {
  for (const style of ["classic", "contour", "notch", "capsule"]) {
    const h = harness({style});
    h.context.syncBottomNavIndicator();
    assert.equal(h.properties.get("--nav-selection-x"), "36px");
    h.tick(520);
    assert.equal(h.properties.get("--nav-selection-x"), "198px");
    assert.equal(h.properties.get("--nav-selection-y"), style === "notch" ? "2px" : style === "classic" ? "43.5px" : "25px");
  }
});

test("the contour changes shape while travelling instead of sliding a rigid SVG", () => {
  const h = harness({style: "contour"});
  h.context.syncBottomNavIndicator();
  const original = h.paths.get("contour");
  assert.ok(original.includes(" 4 "));
  h.tick(260);
  assert.equal(h.properties.get("--nav-selection-x"), "117px");
  assert.ok(h.paths.get("contour").includes("17.12"));
  assert.notEqual(h.paths.get("contour"), original);
  h.tick(520);
  assert.ok(h.paths.get("contour").includes(" 4 "));
});

test("the notch, old icon descent and new icon ascent share intermediate motion", () => {
  const h = harness();
  h.context.syncBottomNavIndicator();
  h.tick(260);
  assert.equal(h.nav.items[0].style.values.get("--nav-icon-lift"), "11.5px");
  assert.equal(h.nav.items[3].style.values.get("--nav-icon-lift"), "11.5px");
  assert.equal(h.paths.get("[data-nav-cutout]"), h.paths.get("[data-nav-rim]") + " V50 H0 Z");
  h.tick(520);
  assert.equal(h.nav.items[0].style.values.get("--nav-icon-lift"), "0px");
  assert.equal(h.nav.items[3].style.values.get("--nav-icon-lift"), "23px");
});

test("rapid reversal resumes the visible curve and icon lifts without jumping", () => {
  const h = harness();
  h.context.syncBottomNavIndicator(); h.tick(260);
  const before = h.context.captureBottomNavSelection();
  h.replace().dataset.activeIndex = 0;
  h.context.pendingBottomNavAnimation = {shouldAnimate: true};
  h.context.syncBottomNavIndicator(before);
  assert.equal(h.properties.get("--nav-selection-x"), "117px");
  assert.equal(h.nav.items[3].style.values.get("--nav-icon-lift"), "11.5px");
  h.tick(300); // Disconnected old nav callback.
  h.tick(780);
  assert.equal(h.properties.get("--nav-selection-x"), "36px");
});

test("polling keeps the original animation timeline and deformation phase", () => {
  const h = harness();
  h.context.syncBottomNavIndicator(); h.tick(260);
  const before = h.context.captureBottomNavSelection();
  h.replace(); h.context.syncBottomNavIndicator(before);
  assert.equal(h.properties.get("--nav-selection-x"), "117px");
  h.tick(300); h.tick(390);
  const expected = 36 + 162 * .896484375;
  assert.equal(parseFloat(h.properties.get("--nav-selection-x")), expected);
  h.tick(520);
  assert.equal(h.properties.get("--nav-selection-x"), "198px");
});

test("selecting the tab currently under a travelling notch still eases its icon lift", () => {
  const h = harness({active: 4});
  h.context.syncBottomNavIndicator(); h.tick(260);
  const before = h.context.captureBottomNavSelection();
  h.replace().dataset.activeIndex = 2;
  h.context.pendingBottomNavAnimation = {shouldAnimate: true};
  h.context.syncBottomNavIndicator(before);
  assert.equal(h.properties.get("--nav-selection-x"), "144px");
  assert.equal(h.nav.items[2].style.values.get("--nav-icon-lift"), "0px");
  h.tick(300); h.tick(780);
  assert.equal(h.nav.items[2].style.values.get("--nav-icon-lift"), "23px");
});

test("reduced motion snaps all parts and starts no animation", () => {
  const h = harness({reduced: true}); h.context.syncBottomNavIndicator();
  assert.equal(h.frames.length, 0);
  assert.equal(h.properties.get("--nav-selection-x"), "198px");
  assert.equal(h.nav.items[3].style.values.get("--nav-icon-lift"), "23px");
});

test("resize cancels a pending animation before aligning with new geometry", () => {
  const h = harness(); h.context.syncBottomNavIndicator();
  h.nav.items[3].offsetLeft += 20;
  h.context.syncBottomNavIndicator();
  assert.equal(h.properties.get("--nav-selection-x"), "218px");
  h.tick(260);
  assert.equal(h.properties.get("--nav-selection-x"), "218px");
});

test("a removed tab clamps stale indices to the remaining navigation", () => {
  const h = harness({active: 4, previous: 4, count: 3}); h.context.syncBottomNavIndicator();
  assert.equal(h.properties.get("--nav-selection-x"), "144px");
  assert.equal(h.frames.length, 0);
});
