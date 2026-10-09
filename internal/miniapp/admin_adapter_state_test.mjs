import test from "node:test";
import assert from "node:assert/strict";
import {
  snapshot,
  controlKey,
  selectedTab,
} from "../../admin-ui/adapter-state.mjs";
import { createAdminLoader } from "./static/admin-loader.mjs";

test("background refreshes reuse equal rows and replace data when panel values change", () => {
  const rows = [
    { customerId: 42, expiresAt: "2027-01-01", usedTrafficBytes: 100 },
  ];
  const first = snapshot(rows);
  assert.equal(snapshot(structuredClone(rows), first), first);
  const updated = snapshot([{ ...rows[0], usedTrafficBytes: 200 }], first);
  assert.notEqual(updated.value, first.value);
  assert.equal(updated.value[0].usedTrafficBytes, 200);
});

test("editing a value preserves its field while changing a settings tab replaces different fields", () => {
  const field = {
    dataset: { input: "admin-ai-apiUrl" },
    type: "text",
    value: "one",
  };
  assert.equal(controlKey(field, 0), controlKey({ ...field, value: "two" }, 0));
  assert.notEqual(
    controlKey({ dataset: { settingPath: "trial.days" } }, 0),
    controlKey({ dataset: { settingPath: "grace.days" } }, 0),
  );
  assert.notEqual(
    controlKey(
      { dataset: { input: "squad" }, type: "checkbox", value: "a" },
      0,
    ),
    controlKey(
      { dataset: { input: "squad" }, type: "checkbox", value: "b" },
      0,
    ),
  );
});

test("native tabs respect the selected group and active legacy tab", () => {
  const tab = (attrs = {}, classes = []) => ({
    getAttribute: (name) => attrs[name],
    classList: { contains: (name) => classes.includes(name) },
  });
  const first = tab(),
    current = tab({ "aria-selected": "true" });
  assert.equal(selectedTab([first, current]), current);
  const group = tab({ "aria-pressed": "true" });
  assert.equal(selectedTab([first, group]), group);
  assert.equal(
    selectedTab([first, tab({}, ["is-active"])]).classList.contains(
      "is-active",
    ),
    true,
  );
});

function host(name) {
  const status = { textContent: "", removeAttribute() {} };
  return {
    name,
    isConnected: true,
    innerHTML: "",
    querySelector: () => status,
  };
}

test("a slow admin download uses the latest section and ignores an abandoned page", async () => {
  let finish,
    downloads = 0;
  const callbacks = [],
    mounts = [];
  const loader = createAdminLoader(() => {
    downloads++;
    return new Promise((resolve) => {
      finish = resolve;
    });
  });
  const pending = loader.mountRemnaAdmin(host("old"), {}, {}, () =>
    callbacks.push("old"),
  );
  loader.unmountRemnaAdmin();
  loader.mountRemnaAdmin(host("new"), {}, {}, () => callbacks.push("new"));
  finish({
    mountRemnaAdmin: (element) => mounts.push(element.name),
    unmountRemnaAdmin() {},
  });
  await pending;
  assert.equal(downloads, 1);
  assert.deepEqual(callbacks, ["new"]);
  loader.mountRemnaAdmin(host("cached"), {}, {});
  assert.deepEqual(mounts, ["cached"]);
});

test("leaving admin cancels its pending callback and a failed download can retry", async () => {
  let finish;
  const loader = createAdminLoader(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const pending = loader.mountRemnaAdmin(host("old"), {}, {}, () =>
    assert.fail("abandoned callback"),
  );
  loader.unmountRemnaAdmin();
  finish({ unmountRemnaAdmin() {} });
  await pending;
  let attempts = 0;
  const retry = createAdminLoader(async () => {
    if (++attempts === 1) throw Error("network");
    return { mountRemnaAdmin() {} };
  });
  const element = host("retry");
  await retry.mountRemnaAdmin(element, {}, {});
  assert.match(element.querySelector().textContent, /Не удалось/);
  await retry.mountRemnaAdmin(element, {}, {});
  assert.equal(attempts, 2);
});
