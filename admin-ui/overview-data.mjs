export const OVERVIEW_COLORS = [
  "#69b7a7",
  "#edbd70",
  "#6c8b9e",
  "#d47db5",
  "#b2b1ed",
  "#53868b",
  "#a09177",
];

export function groupOverviewPoints(points = [], unit = "days") {
  const groups = new Map();
  for (const point of points) {
    const date = new Date(`${point.date}T00:00:00Z`);
    if (!Number.isFinite(date.getTime())) continue;
    if (unit === "weeks")
      date.setUTCDate(date.getUTCDate() - ((date.getUTCDay() + 6) % 7));
    if (unit === "months") date.setUTCDate(1);
    const key = date.toISOString().slice(0, 10);
    groups.set(
      key,
      (groups.get(key) || 0) + Math.max(0, Number(point.value) || 0),
    );
  }
  return [...groups]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([date, value]) => ({ date, value }));
}

export function overviewSegments(items = []) {
  const total = items.reduce(
    (sum, item) => sum + Math.max(0, Number(item.count) || 0),
    0,
  );
  let offset = 0;
  return {
    total,
    items: items.map((item, index) => {
      const fraction = total ? Math.max(0, Number(item.count) || 0) / total : 0;
      const segment = {
        ...item,
        color: item.color || OVERVIEW_COLORS[index % OVERVIEW_COLORS.length],
        fraction,
        offset,
      };
      offset += fraction;
      return segment;
    }),
  };
}

export function overviewRingPath(segment, outer = 80, inner = 44) {
  const gap = Math.min(0.012, (segment.fraction * Math.PI) / 3);
  const start = segment.offset * 2 * Math.PI - Math.PI / 2 + gap;
  const end =
    (segment.offset + segment.fraction) * 2 * Math.PI - Math.PI / 2 - gap;
  if (end <= start) return "";
  const point = (radius, angle) =>
    `${(100 + radius * Math.cos(angle)).toFixed(3)},${(100 + radius * Math.sin(angle)).toFixed(3)}`;
  const large = end - start > Math.PI ? 1 : 0;
  return `M${point(outer, start)} A${outer},${outer} 0 ${large} 1 ${point(outer, end)} L${point(inner, end)} A${inner},${inner} 0 ${large} 0 ${point(inner, start)} Z`;
}

export function overviewChartGeometry(points = []) {
  const width = 600,
    height = 200,
    left = 44,
    right = 14,
    top = 14,
    bottom = 30;
  const max = Math.max(1, ...points.map((point) => Number(point.value) || 0));
  const magnitude = 10 ** Math.floor(Math.log10(max));
  const ceiling = Math.ceil(max / magnitude / 2) * magnitude * 2;
  const coordinates = points.map((point, index) => ({
    ...point,
    x:
      left +
      (points.length === 1 ? 0.5 : index / (points.length - 1)) *
        (width - left - right),
    y:
      height -
      bottom -
      (Math.max(0, Number(point.value) || 0) / ceiling) *
        (height - top - bottom),
  }));
  const line = coordinates
    .map((point, index) => {
      if (!index) return `M${point.x.toFixed(2)},${point.y.toFixed(2)}`;
      const previous = coordinates[index - 1],
        middle = (point.x + previous.x) / 2;
      return `C${middle.toFixed(2)},${previous.y.toFixed(2)} ${middle.toFixed(2)},${point.y.toFixed(2)} ${point.x.toFixed(2)},${point.y.toFixed(2)}`;
    })
    .join(" ");
  const area = coordinates.length
    ? `${line} L${coordinates.at(-1).x},${height - bottom} L${coordinates[0].x},${height - bottom} Z`
    : "";
  return {
    width,
    height,
    left,
    right,
    top,
    bottom,
    ceiling,
    coordinates,
    line,
    area,
  };
}
