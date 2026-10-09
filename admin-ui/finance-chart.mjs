// Coordinates are in CSS pixels: a wider card must not enlarge labels or dots.
export function financeChartLayout(series, width, height, currency = "RUB") {
  const plot = {
    left: width < 480 ? 46 : 60,
    right: Math.max(60, width - 16),
    top: 24,
    bottom: height - 34,
  };
  const revenueKey = currency === "STARS" ? "revenueStars" : "revenueRub";
  const refundKey = currency === "STARS" ? "refundsStars" : "refundsRub";
  const amount = (value) =>
    Number.isFinite(Number(value)) ? Math.max(0, Number(value)) : 0;
  const amounts = series.map((item) => amount(item[revenueKey]));
  const refunds = series.map((item) => amount(item[refundKey]));
  const maximum = Math.max(0, ...amounts, ...refunds) || 1;
  const magnitude = 10 ** Math.floor(Math.log10(maximum / 4));
  const stepValue = Math.max(
    currency === "STARS" ? 1 : 0.01,
    [1, 2, 2.5, 5, 10].find((value) => value * magnitude >= maximum / 4) *
      magnitude,
  );
  const ceiling = Math.ceil(maximum / stepValue) * stepValue;
  const ticks = Array.from(
    { length: Math.round(ceiling / stepValue) + 1 },
    (_, index) => index * stepValue,
  );
  const tickLabel = new Intl.NumberFormat("ru", {
    notation: "compact",
    maximumFractionDigits: 2,
  });
  plot.left = Math.max(
    plot.left,
    ...ticks.map(
      (value) => Math.ceil(tickLabel.format(value).length * 7.2) + 14,
    ),
  );
  const points = series.map((item, index) => ({
    ...item,
    value: amounts[index],
    refund: refunds[index],
    x:
      plot.left +
      (series.length === 1 ? 0.5 : index / (series.length - 1)) *
        (plot.right - plot.left),
    y: plot.bottom - (amounts[index] / ceiling) * (plot.bottom - plot.top),
    refundY:
      plot.bottom - (refunds[index] / ceiling) * (plot.bottom - plot.top),
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
  const path = (key) =>
    points
      .map((point, index) => {
        if (!index) return `M ${point.x} ${point[key]}`;
        const previous = points[index - 1];
        const middle = (previous.x + point.x) / 2;
        return `C ${middle} ${previous[key]} ${middle} ${point[key]} ${point.x} ${point[key]}`;
      })
      .join(" ");
  return {
    plot,
    ceiling,
    ticks,
    points,
    labels,
    line: path("y"),
    refundLine: path("refundY"),
    hasRefunds: refunds.some((value) => value > 0),
  };
}

export function financeChartPointIndex(x, plot, count) {
  if (!count) return null;
  if (count === 1) return 0;
  return Math.max(
    0,
    Math.min(
      count - 1,
      Math.round(((x - plot.left) / (plot.right - plot.left)) * (count - 1)),
    ),
  );
}

export function financeTooltipPosition(
  point,
  width,
  height,
  tooltipWidth,
  tooltipHeight,
) {
  const left =
    point.x + tooltipWidth + 24 < width
      ? point.x + 16
      : point.x - tooltipWidth - 16;
  const top =
    point.y > tooltipHeight + 32 ? point.y - tooltipHeight - 16 : point.y + 16;
  return {
    left: Math.max(8, Math.min(width - tooltipWidth - 8, left)),
    top: Math.max(8, Math.min(height - tooltipHeight - 8, top)),
  };
}
