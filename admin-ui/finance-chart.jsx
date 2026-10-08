import React, { useLayoutEffect, useRef, useState } from "react";
import { financeChartLayout } from "./finance-chart.mjs";

const dateLabel = (date) =>
  new Date(date).toLocaleDateString("ru", { day: "2-digit", month: "short" });
const rubles = (value) =>
  new Intl.NumberFormat("ru", {
    style: "currency",
    currency: "RUB",
    maximumFractionDigits: 2,
  }).format(value || 0);

export function FinanceChart({ series }) {
  const container = useRef(null);
  const [size, setSize] = useState({ width: 740, height: 260 });
  const [active, setActive] = useState(null);
  useLayoutEffect(() => {
    const measure = () => {
      const { width, height } = container.current.getBoundingClientRect();
      setSize((previous) =>
        previous.width === width && previous.height === height
          ? previous
          : { width, height },
      );
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(container.current);
    return () => observer.disconnect();
  }, []);
  const { plot, ceiling, points, labels, line } = financeChartLayout(
    series,
    size.width,
    size.height,
  );
  const selected = active == null ? null : points[active];
  return (
    <div className="rn-finance-chart" ref={container}>
      <svg
        width="100%"
        height="100%"
        viewBox={`0 0 ${size.width} ${size.height}`}
        role="img"
        aria-label="График выручки по дням"
      >
        {[0, 0.25, 0.5, 0.75, 1].map((ratio) => {
          const y = plot.bottom - ratio * (plot.bottom - plot.top);
          return (
            <g key={ratio} className="rn-chart-axis">
              <line x1={plot.left} y1={y} x2={plot.right} y2={y} />
              <text x={plot.left - 12} y={y + 4} textAnchor="end">
                {new Intl.NumberFormat("ru", {
                  notation: "compact",
                  maximumFractionDigits: 1,
                }).format(ceiling * ratio)}
              </text>
            </g>
          );
        })}
        <path
          className="rn-chart-area"
          d={`${line} L ${points.at(-1).x} ${plot.bottom} L ${points[0].x} ${plot.bottom} Z`}
        />
        <path className="rn-chart-line" d={line} />
        {labels.map((point) => (
          <text
            key={point.date}
            className="rn-chart-label"
            x={point.x}
            y={size.height - 10}
            textAnchor={
              point === points[0]
                ? "start"
                : point === points.at(-1)
                  ? "end"
                  : "middle"
            }
          >
            {dateLabel(point.date)}
          </text>
        ))}
        {selected && (
          <line
            className="rn-chart-cursor"
            x1={selected.x}
            x2={selected.x}
            y1={plot.top}
            y2={plot.bottom}
          />
        )}
        {points.map((point, index) => (
          <g
            key={point.date}
            tabIndex={0}
            role="img"
            aria-label={`${dateLabel(point.date)}: ${rubles(point.revenueRub)}, платежей ${point.paymentCount || 0}`}
            onMouseEnter={() => setActive(index)}
            onMouseLeave={() => setActive(null)}
            onFocus={() => setActive(index)}
            onBlur={() => setActive(null)}
          >
            <circle cx={point.x} cy={point.y} r={10} fill="transparent" />
            <circle
              className="rn-chart-dot"
              cx={point.x}
              cy={point.y}
              r={selected === point ? 4 : 3}
            />
          </g>
        ))}
      </svg>
      {selected && (
        <div
          role="tooltip"
          className="rn-chart-tooltip"
          style={{
            left: Math.max(8, Math.min(size.width - 180, selected.x - 86)),
            top: selected.y < 96 ? selected.y + 16 : selected.y - 82,
          }}
        >
          <span>{dateLabel(selected.date)}</span>
          <strong>{rubles(selected.revenueRub)}</strong>
          {Number(selected.revenueStars) > 0 && (
            <span>{selected.revenueStars} Stars</span>
          )}
          <span>Платежей: {selected.paymentCount || 0}</span>
        </div>
      )}
    </div>
  );
}
