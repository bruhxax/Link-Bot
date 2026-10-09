import test from "node:test";
import assert from "node:assert/strict";
import { createFinanceChartLoader } from "./static/finance-chart-loader.mjs";

test("a delayed chart download mounts only the latest page after navigation", async () => {
  let finish;
  let downloads = 0;
  const mounted = [];
  const loader = createFinanceChartLoader(() => {
    downloads++;
    return new Promise((resolve) => {
      finish = resolve;
    });
  });
  const host = (name) => ({
    name,
    isConnected: true,
    querySelector: () => ({}),
  });
  const first = loader.mountFinanceCharts(host("old"));
  loader.unmountFinanceCharts();
  const second = loader.mountFinanceCharts(host("new"));
  finish({
    mountFinanceCharts: (container) => mounted.push(container.name),
    unmountFinanceCharts: () => {},
  });
  await Promise.all([first, second]);
  assert.deepEqual(mounted, ["new"]);
  assert.equal(downloads, 1);
  loader.unmountFinanceCharts();
  await loader.mountFinanceCharts(host("next"));
  assert.deepEqual(mounted, ["new", "next"]);
  assert.equal(downloads, 1);
});

test("non-finance pages do not load charts, and a failed download can be retried", async () => {
  let downloads = 0,
    mounts = 0;
  const loader = createFinanceChartLoader(async () => {
    if (++downloads === 1) throw new Error("network");
    return {
      mountFinanceCharts: () => mounts++,
      unmountFinanceCharts: () => {},
    };
  });
  await loader.mountFinanceCharts({ querySelector: () => null });
  assert.equal(downloads, 0);
  const status = { textContent: "" };
  const page = { isConnected: true, querySelector: () => status };
  await loader.mountFinanceCharts(page);
  assert.ok(status.textContent.includes("Не удалось загрузить график"));
  await loader.mountFinanceCharts(page);
  assert.equal(mounts, 1);
  assert.equal(downloads, 2);
});
