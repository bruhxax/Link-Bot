import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
function section(start, end) {
  const first = source.indexOf(start);
  const last = source.indexOf(end, first);
  assert.ok(first >= 0 && last > first);
  return source.slice(first, last);
}

// A small DOM with controllable animation promises exercises cancellation races
// without timers or browser frame timing determining the result.
function harness() {
  const animations = [];
  class TextNode {
    nodeType = 3;
    constructor(value) { this.textContent = value; }
    replaceWith(...nodes) { replace(this, nodes); }
  }
  function replace(node, replacements) {
    const parent = node.parentElement;
    if (!parent) return;
    parent.childNodes.splice(parent.childNodes.indexOf(node), 1, ...replacements);
    replacements.forEach(child => { child.parentElement = parent; });
    node.parentElement = null;
  }
  class Element {
    nodeType = 1;
    id = "";
    className = "";
    dataset = {};
    style = { position: "" };
    childNodes = [];
    clientLeft = 0;
    clientTop = 0;
    constructor(tag) { this.tagName = tag.toUpperCase(); }
    get children() { return this.childNodes.filter(child => child.nodeType === 1); }
    get childElementCount() { return this.children.length; }
    get firstChild() { return this.childNodes[0]; }
    get textContent() { return this.childNodes.map(child => child.textContent).join(""); }
    set textContent(value) { this.childNodes = []; this.appendChild(new TextNode(value)); }
    get clientHeight() { return parseFloat(this.style.height) || 20; }
    get classList() {
      return {
        add: name => { this.className = [this.className, name].filter(Boolean).join(" "); },
        remove: name => { this.className = this.className.split(" ").filter(item => item !== name).join(" "); },
      };
    }
    appendChild(child) { child.parentElement = this; this.childNodes.push(child); return child; }
    setAttribute(name, value) { this[name] = value; }
    replaceWith(...nodes) { replace(this, nodes); }
    remove() { replace(this, []); }
    normalize() {
      const text = this.textContent;
      assert.equal(this.children.length, 0, "cleanup must remove masks and animated layers");
      this.textContent = text;
    }
    getBoundingClientRect() { return { left: 10, top: 10, width: 250, height: 20, bottom: 30 }; }
    animate(frames, options) {
      let resolve, reject;
      const finished = new Promise((done, fail) => { resolve = done; reject = fail; });
      const animation = { element: this, frames, options, finished, finish: resolve, cancel: () => reject(new Error("cancelled")) };
      animations.push(animation);
      return animation;
    }
  }
  const document = {
    hidden: false, body: new Element("body"), addEventListener() {},
    createElement: tag => new Element(tag), createTextNode: value => new TextNode(value),
    createRange: () => {
      let start, end;
      return {
        setStart(node, offset) { start = offset; }, setEnd(node, offset) { end = offset; },
        getBoundingClientRect: () => ({ left: 10 + start * 10, top: 10, width: (end - start) * 10, height: 20 }),
      };
    },
  };
  const reducedMotionMedia = { matches: false };
  const context = vm.createContext({
    document, Node: { TEXT_NODE: 3 }, HTMLElement: Element, Intl, reducedMotionMedia,
    window: {
      innerHeight: 600, addEventListener() {},
      getComputedStyle: node => ({ position: node.style.position || "static", visibility: "visible", opacity: "1", font: "14px sans-serif", color: "white", lineHeight: "20px" }),
    },
  });
  vm.runInContext(section("const activeTextTransitions =", "function textNodePath(") + section("function updateAnimatedText(", "function render("), context);
  const node = new Element("span");
  node.textContent = "Оплатить 199 ₽";
  document.body.appendChild(node);
  return { context, node, document, reducedMotionMedia,
    get animations() { return animations.filter(animation => animation.element.className === "text-roll__glyph"); },
    get widthAnimations() { return animations.filter(animation => animation.element.className === "text-transition-cell"); },
    update: value => context.updateAnimatedText(node, value),
    finish: async () => { animations.forEach(animation => animation.finish()); await new Promise(setImmediate); },
  };
}

test("rapid changes keep the newest animation alive when cancelled promises settle", async () => {
  const page = harness();
  for (const value of ["Оплатить 450 ₽", "Оплатить 800 ₽", "Оплатить 199 ₽", "Оплатить 6000 ₽"]) {
    const previousCount = page.animations.length;
    page.update(value);
    assert.ok(page.animations.length > previousCount, "every change starts a new roll");
  }
  await new Promise(setImmediate);
  assert.equal(page.node.textContent, "Оплатить 6000 ₽");
  assert.ok(page.node.children.some(child => child.className === "text-transition-cell"));
  assert.equal(page.document.body.children.length, 1, "glyphs stay inside the text, below surrounding navigation");
  await page.finish();
  assert.equal(page.node.textContent, "Оплатить 6000 ₽");
  assert.equal(page.node.children.length, 0);
  assert.equal(page.node.style.position, "");
});

test("letters and digits roll in adjacent opposite directions without fades or delays", async () => {
  const page = harness();
  page.update("Pay 800 ₽");
  const moving = page.animations;
  assert.ok(moving.length > 6, "language changes animate letters as well as the price");
  for (const animation of moving) {
    assert.equal(animation.options.delay, 0);
    assert.ok(animation.frames.every(frame => Object.keys(frame).every(key => key === "transform")));
  }
  assert.equal(moving[0].frames[1].transform, "translateY(-100%)");
  assert.equal(moving[1].frames[0].transform, "translateY(100%)");
  assert.equal(moving[2].frames[1].transform, "translateY(100%)");
  assert.equal(moving[3].frames[0].transform, "translateY(-100%)");
  await page.finish();
  assert.equal(page.node.textContent, "Pay 800 ₽");
});

test("price changes roll only the changed digit positions, including separated changes", async () => {
  for (const [before, after, expected] of [
    ["Оплатить 3000 ₽", "Оплатить 4000 ₽", ["3", "4"]],
    ["Оплатить 3010 ₽", "Оплатить 4020 ₽", ["3", "4", "1", "2"]],
    ["Оплатить 1234 ₽", "Оплатить 2345 ₽", ["1", "2", "2", "3", "3", "4", "4", "5"]],
  ]) {
    const page = harness();
    page.node.textContent = before;
    page.update(after);
    assert.deepEqual(page.animations.map(animation => animation.element.dataset.character), expected);
    assert.ok(page.animations.every(animation => animation.options.duration >= 450));
    assert.equal(page.node.textContent, after);
    await page.finish();
    assert.equal(page.node.textContent, after);
  }
});

test("changing a plan word length leaves its unchanged letters and label unanimated", async () => {
  const page = harness();
  page.node.textContent = "6 месяцев · Безлимит";
  page.update("3 месяца · Безлимит");
  const movingCharacters = page.animations.map(animation => animation.element.dataset.character);
  assert.ok(movingCharacters.includes("6") && movingCharacters.includes("3"));
  assert.ok(movingCharacters.every(character => "63ева".includes(character)));
  assert.equal(page.node.textContent, "3 месяца · Безлимит");
  await page.finish();
  assert.equal(page.node.textContent, "3 месяца · Безлимит");
});

test("inserted and removed symbols keep their reels inside cells that resize with the text", async () => {
  for (const [before, after] of [
    ["1 месяц · Безлимит", "12 месяцев · Безлимит"],
    ["12 месяцев · Безлимит", "1 месяц · Безлимит"],
    ["Оплатить 700 ₽", "Оплатить 6000 ₽"],
    ["Оплатить 6000 ₽", "Оплатить 700 ₽"],
  ]) {
    const page = harness();
    page.node.textContent = before;
    page.update(after);
    assert.equal(page.node.textContent, after);
    assert.ok(page.widthAnimations.length > 0, "insertion/removal resizes in-flow cells");
    for (const animation of page.animations) {
      const slot = animation.element.parentElement;
      const overlay = slot.parentElement;
      const cell = overlay.parentElement;
      assert.equal(cell.className, "text-transition-cell");
      assert.equal(cell.parentElement, page.node);
      assert.equal(slot.style.width, "100%", "each reel is clipped to its own cell, never its neighbour");
      assert.equal(slot.style.left, "0");
    }
    assert.ok(page.widthAnimations.some(animation => animation.frames.some(frame => frame.width === "0px")));
    await page.finish();
    assert.equal(page.node.textContent, after);
    assert.equal(page.node.children.length, 0);
  }
});

test("repeating the current value and hiding the page leave no stale text or layers", async () => {
  const page = harness();
  page.update("Оплатить 800 ₽");
  page.update("Оплатить 800 ₽");
  await new Promise(setImmediate);
  assert.equal(page.node.textContent, "Оплатить 800 ₽");
  assert.equal(page.node.children.length, 0);
  page.update("Оплатить 450 ₽");
  page.context.cancelAllTextTransitions();
  await page.finish();
  assert.equal(page.node.textContent, "Оплатить 450 ₽");
  assert.equal(page.node.children.length, 0);
});

test("Unicode graphemes survive rolling and reduced motion replaces text immediately", async () => {
  const page = harness();
  assert.deepEqual(Array.from(page.context.splitTransitionText("👨‍👩‍👧‍👦е́")), ["👨‍👩‍👧‍👦", "е́"]);
  page.update("Тариф 👨‍👩‍👧‍👦 · Безлимит");
  await page.finish();
  assert.equal(page.node.textContent, "Тариф 👨‍👩‍👧‍👦 · Безлимит");
  const count = page.animations.length;
  page.reducedMotionMedia.matches = true;
  page.update("Family · Unlimited");
  assert.equal(page.animations.length, count);
  assert.equal(page.node.textContent, "Family · Unlimited");
  assert.equal(page.node.children.length, 0);
});

test("fast plan selection animates both price and title while preserving the payment button", () => {
  const titleSelector = '[data-text-transition="checkout-plan"]';
  const calls = [];
  const state = { selectedPlanId: "1" };
  let checkout;
  function makeCheckout() {
    const parts = new Map();
    const title = { textContent: `Plan ${state.selectedPlanId}`, replaceWith: node => parts.set(titleSelector, node) };
    const label = { textContent: `Pay ${state.selectedPlanId}00` };
    const action = {
      disabled: false, setAttribute() {},
      querySelector: () => label, replaceWith: node => parts.set(".buy-action", node),
    };
    parts.set(titleSelector, title);
    parts.set(".buy-action", action);
    const current = { querySelector: selector => parts.get(selector), replaceWith: next => { checkout = next; } };
    return current;
  }
  checkout = makeCheckout();
  const originalTitle = checkout.querySelector(titleSelector);
  const originalAction = checkout.querySelector(".buy-action");
  const page = { querySelector: () => checkout, querySelectorAll: () => [] };
  const context = vm.createContext({
    state, app: { querySelector: () => page }, performance: { now: () => 1000 },
    document: { createElement: () => ({ content: { querySelector: () => makeCheckout() } }) },
    renderBuyPage: () => "",
    updateAnimatedText(node, value) { calls.push([node, value]); node.textContent = value; },
  });
  vm.runInContext(section("function syncSelectedPlanUI(", "function getDevicePacks("), context);
  for (const id of ["3", "6", "1", "12", "6"]) {
    state.selectedPlanId = id;
    assert.equal(context.syncSelectedPlanUI(), true);
    assert.equal(checkout.querySelector(".buy-action"), originalAction);
    assert.equal(checkout.querySelector(titleSelector), originalTitle);
    assert.equal(originalTitle.textContent, `Plan ${id}`);
  }
  assert.equal(calls.length, 10, "each selection animates both labels even within a single frame");
  assert.equal(originalAction.querySelector().textContent, "Pay 600");
});

test("language changes render immediately and persist the latest selection without timers", () => {
  const stored = new Map();
  const state = { data: { user: { id: 7 } }, locale: "ru" };
  const renders = [];
  const context = vm.createContext({
    state, STORAGE_KEYS: { languageOverride: "locale" },
    getRuntimeSettings: () => ({ localization: { language: "ru" } }),
    pickLocale: value => value, haptic() {}, syncLocalizationFromSettings() {},
    render: () => renders.push(state.locale),
    window: { localStorage: {
      setItem: (key, value) => stored.set(key, value), removeItem: key => stored.delete(key),
    } },
  });
  vm.runInContext(section("function setProfileLanguage(", "function getProfileItems("), context);
  context.setProfileLanguage("en");
  assert.deepEqual(renders, ["en"]);
  assert.equal(stored.get("locale:7"), "en");
  context.setProfileLanguage("ru");
  assert.deepEqual(renders, ["en", "ru"]);
  assert.equal(stored.has("locale:7"), false);
  context.setProfileLanguage("invalid");
  assert.deepEqual(renders, ["en", "ru"]);
});
