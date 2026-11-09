import test from "node:test";
import assert from "node:assert/strict";
import { createProtectionBanner } from "./static/landing-protection.mjs";
function setup(t) {
  const previous = {};
  for (const name of ["window", "document", "matchMedia", "ResizeObserver"]) {
    previous[name] = globalThis[name];
    t.after(() => {
      if (previous[name] === undefined) delete globalThis[name];
      else globalThis[name] = previous[name];
    });
  }
  globalThis.window = { addEventListener() {}, removeEventListener() {} };
  globalThis.document = { documentElement: { style: { setProperty() {} } } };
  globalThis.matchMedia = () => ({ matches: true });
  globalThis.ResizeObserver = class {
    observe() {}
    disconnect() {}
  };
  const timers = new Map(),
    text = { textContent: "" },
    element = {
      hidden: true,
      style: {},
      dataset: {},
      querySelector: () => text,
      getBoundingClientRect: () => ({ bottom: 100 }),
    },
    nav = { getBoundingClientRect: () => ({ bottom: 70, width: 350 }) };
  let id = 0;
  return {
    element,
    text,
    nav,
    timers,
    options: {
      setTimer: (fn, ms) => {
        timers.set(++id, { fn, ms });
        return id;
      },
      clearTimer: (key) => timers.delete(key),
    },
  };
}
test("landing banner shows safe text and hides after the configured period; zero stays visible", async (t) => {
  const h = setup(t);
  let seconds = 3,
    requests = 0;
  const banner = createProtectionBanner(h.element, h.nav, {
    ...h.options,
    fetcher: async () => {
      requests++;
      return {
        ok: true,
        json: async () => ({
          ok: true,
          data: {
            enabled: true,
            state: "protected",
            text: "<img src=x> VPN",
            durationSeconds: seconds,
          },
        }),
      };
    },
  });
  await banner.show();
  assert.equal(h.element.hidden, false);
  assert.equal(h.text.textContent, "<img src=x> VPN");
  assert.equal(h.element.dataset.protected, "true");
  assert.equal([...h.timers.values()][0].ms, 3000);
  [...h.timers.values()][0].fn();
  assert.equal(h.element.hidden, true);
  assert.equal(requests, 1);
  seconds = 0;
  await banner.show();
  assert.equal(h.element.hidden, false);
  assert.equal(h.timers.size, 0);
  banner.destroy();
  assert.equal(h.element.hidden, true);
});
test("a stale personal status cannot override a newer visit and failed lookup stays hidden", async (t) => {
  const h = setup(t);
  let resolveFirst,
    calls = 0;
  const banner = createProtectionBanner(h.element, h.nav, {
    ...h.options,
    fetcher: async () => {
      if (++calls === 1)
        await new Promise((done) => {
          resolveFirst = done;
        });
      return {
        ok: true,
        json: async () => ({
          ok: true,
          data: {
            enabled: true,
            state: "unprotected",
            text: calls === 1 ? "old" : "current",
            durationSeconds: 0,
          },
        }),
      };
    },
  });
  const old = banner.show();
  await banner.show();
  resolveFirst();
  await old;
  assert.equal(h.text.textContent, "current");
  banner.destroy();
  const failed = createProtectionBanner(h.element, h.nav, {
    ...h.options,
    fetcher: async () => {
      throw Error("offline");
    },
  });
  await failed.show();
  assert.equal(h.element.hidden, true);
  failed.destroy();
});
