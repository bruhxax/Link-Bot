import test from "node:test";
import assert from "node:assert/strict";
import {
  financeChartLayout,
  financeChartPointIndex,
  financeTooltipPosition,
} from "../../admin-ui/finance-chart.mjs";

const series = Array.from({ length: 31 }, (_, index) => ({
  date: `2026-10-${index + 1}`,
  revenueRub: (index % 7) * 350,
}));

test("a wider finance card changes the horizontal scale without enlarging the chart", () => {
  const normal = financeChartLayout(series, 740, 260);
  const wide = financeChartLayout(series, 1700, 260);
  assert.deepEqual(
    normal.points.map((point) => point.y),
    wide.points.map((point) => point.y),
  );
  assert.equal(wide.plot.bottom, 226);
  assert.ok(wide.points.at(-1).x > normal.points.at(-1).x);
});

test("mobile and annual charts keep ticks apart and every point inside the plot", () => {
  const annual = Array.from({ length: 365 }, (_, i) => ({
    date: String(i),
    revenueRub: i * 19,
  }));
  for (const width of [280, 390, 740, 1700]) {
    const chart = financeChartLayout(annual, width, 220);
    assert.equal(chart.labels.at(-1), chart.points.at(-1));
    chart.labels
      .slice(1)
      .forEach((point, i) => assert.ok(point.x - chart.labels[i].x >= 100));
    chart.points.forEach((point) => {
      assert.ok(point.x >= chart.plot.left && point.x <= chart.plot.right);
      assert.ok(point.y >= chart.plot.top && point.y <= chart.plot.bottom);
    });
  }
});

test("zero revenue and a single day produce finite coordinates", () => {
  const chart = financeChartLayout(
    [{ date: "2026-10-08", revenueRub: 0 }],
    390,
    220,
  );
  assert.equal(chart.labels.length, 1);
  assert.equal(chart.points[0].x, (chart.plot.left + chart.plot.right) / 2);
  assert.equal(chart.points[0].y, chart.plot.bottom);
  assert.ok(!chart.line.includes("NaN"));
});

test("rubles and Stars keep separate scales, including larger refunds", () => {
  const days = [
    {
      date: "2026-10-08",
      revenueRub: 100,
      refundsRub: 2500,
      revenueStars: 35,
      refundsStars: 100000,
    },
  ];
  const rubles = financeChartLayout(days, 740, 260);
  const stars = financeChartLayout(days, 740, 260, "STARS");
  assert.equal(rubles.points[0].value, 100);
  assert.equal(rubles.points[0].refund, 2500);
  assert.ok(rubles.ceiling >= 2500 && rubles.ceiling < 100000);
  assert.equal(stars.points[0].value, 35);
  assert.ok(stars.ceiling >= 100000);
  for (const chart of [rubles, stars]) {
    assert.ok(chart.hasRefunds);
    assert.ok(chart.points[0].refundY >= chart.plot.top);
    assert.ok(chart.ticks.length <= 5);
  }
});

test("hover chooses the nearest day across the plot and clamps at the edges", () => {
  const chart = financeChartLayout(series, 740, 260);
  assert.equal(financeChartPointIndex(-50, chart.plot, series.length), 0);
  assert.equal(
    financeChartPointIndex(800, chart.plot, series.length),
    series.length - 1,
  );
  assert.equal(
    financeChartPointIndex(chart.points[15].x + 2, chart.plot, series.length),
    15,
  );
  assert.equal(financeChartPointIndex(400, chart.plot, 1), 0);
  assert.equal(financeChartPointIndex(400, chart.plot, 0), null);
});

test("tooltips stay inside mobile and desktop charts at every corner", () => {
  for (const width of [280, 740, 1700]) {
    for (const point of [
      { x: 0, y: 0 },
      { x: width, y: 0 },
      { x: 0, y: 212 },
      { x: width, y: 212 },
    ]) {
      const position = financeTooltipPosition(point, width, 212, 216, 160);
      assert.ok(position.left >= 8 && position.left + 216 <= width - 8);
      assert.ok(position.top >= 8 && position.top + 160 <= 204);
    }
  }
});
