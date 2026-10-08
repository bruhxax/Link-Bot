// Coordinates are in CSS pixels: a wider card must not enlarge labels or dots.
export function financeChartLayout(series, width, height) {
  const plot = {
    left: 60,
    right: Math.max(60, width - 20),
    top: 18,
    bottom: height - 34,
  };
  const amounts = series.map((item) =>
    Math.max(0, Number(item.revenueRub) || 0),
  );
  const maximum = Math.max(1, ...amounts);
  const magnitude = 10 ** Math.floor(Math.log10(maximum));
  const ceiling = Math.ceil(maximum / magnitude / 0.5) * magnitude * 0.5;
  const points = series.map((item, index) => ({
    ...item,
    x:
      plot.left +
      (series.length === 1 ? 0.5 : index / (series.length - 1)) *
        (plot.right - plot.left),
    y: plot.bottom - (amounts[index] / ceiling) * (plot.bottom - plot.top),
  }));
  const step = Math.max(
    1,
    Math.ceil(
      (points.length - 1) /
        Math.max(1, Math.floor((plot.right - plot.left) / 110)),
    ),
  );
  const labels = points.filter(
    (_, index) =>
      index % step === 0 &&
      (points.length === 1 || index <= points.length - 1 - step),
  );
  if (points.length > 1) labels.push(points.at(-1));
  const line = points
    .map((point, index) => {
      if (!index) return `M ${point.x} ${point.y}`;
      const previous = points[index - 1];
      const middle = (previous.x + point.x) / 2;
      return `C ${middle} ${previous.y} ${middle} ${point.y} ${point.x} ${point.y}`;
    })
    .join(" ");
  return { plot, ceiling, points, labels, line };
}
