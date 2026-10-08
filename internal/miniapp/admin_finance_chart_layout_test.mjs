import test from "node:test";
import assert from "node:assert/strict";
import { financeChartLayout } from "../../admin-ui/finance-chart.mjs";

const series = Array.from({ length: 31 }, (_, index) => ({ date: `2026-10-${index + 1}`, revenueRub: (index % 7) * 350 }));

test("a wider finance card changes the horizontal scale without enlarging the chart", () => {
  const normal = financeChartLayout(series, 740, 260);
  const wide = financeChartLayout(series, 1700, 260);
  assert.deepEqual(normal.points.map(point => point.y), wide.points.map(point => point.y));
  assert.equal(wide.plot.bottom, 226);
  assert.ok(wide.points.at(-1).x > normal.points.at(-1).x);
});

test("mobile and annual charts keep ticks apart and every point inside the plot", () => {
  const annual = Array.from({ length: 365 }, (_, i) => ({ date: String(i), revenueRub: i * 19 }));
  for (const width of [280, 390, 740, 1700]) {
    const chart = financeChartLayout(annual, width, 220);
    assert.equal(chart.labels.at(-1), chart.points.at(-1));
    chart.labels.slice(1).forEach((point, i) => assert.ok(point.x - chart.labels[i].x >= 100));
    chart.points.forEach(point => {
      assert.ok(point.x >= chart.plot.left && point.x <= chart.plot.right);
      assert.ok(point.y >= chart.plot.top && point.y <= chart.plot.bottom);
    });
  }
});

test("zero revenue and a single day produce finite coordinates", () => {
  const chart = financeChartLayout([{ date: "2026-10-08", revenueRub: 0 }], 390, 220);
  assert.equal(chart.labels.length, 1);
  assert.equal(chart.points[0].x, (chart.plot.left + chart.plot.right) / 2);
  assert.equal(chart.points[0].y, chart.plot.bottom);
  assert.ok(!chart.line.includes("NaN"));
});
