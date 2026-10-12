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

// Every curve has the same SVG topology, so changing point counts can morph
// continuously. Sample the actual Bezier curve rather than adding fake dates.
export function overviewMorphPaths(graph, samples = 181) {
  const points = graph.coordinates;
  const baseline = graph.height - graph.bottom;
  const cubic = (a, b, c, d, t) =>
    (1 - t) ** 3 * a +
    3 * (1 - t) ** 2 * t * b +
    3 * (1 - t) * t ** 2 * c +
    t ** 3 * d;
  const line = Array.from({ length: samples }, (_, index) => {
    const fraction = index / (samples - 1);
    let x = graph.left + fraction * (graph.width - graph.left - graph.right);
    let y = points[0]?.y ?? baseline;
    if (points.length > 1) {
      const position = fraction * (points.length - 1);
      const segment = Math.min(points.length - 2, Math.floor(position));
      const a = points[segment],
        b = points[segment + 1],
        t = position - segment;
      const middle = (a.x + b.x) / 2;
      x = cubic(a.x, middle, middle, b.x, t);
      y = cubic(a.y, a.y, b.y, b.y, t);
    }
    return `${index ? "L" : "M"}${x.toFixed(3)},${y.toFixed(3)}`;
  }).join(" ");
  return {
    line,
    area: `${line} L${graph.width - graph.right},${baseline} L${graph.left},${baseline} Z`,
  };
}

export function selectOverviewPeriod(data, days) {
  if (!data) return data;
  const count = [7, 30, 90].includes(days) ? days : 30;
  const trim = (part) =>
    part ? { ...part, daily: part.daily == null ? part.daily : part.daily.slice(-count) } : part;
  return {
    ...data,
    days: count,
    users: trim(data.users),
    revenue: trim(data.revenue),
  };
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
