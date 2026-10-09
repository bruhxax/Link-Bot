import React, {
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { PiChartLine } from "react-icons/pi";
import {
  financeChartLayout,
  financeChartPointIndex,
  financeTooltipPosition,
} from "./finance-chart.mjs";

const dateLabel = (date, full = false) =>
  new Date(date).toLocaleDateString("ru", {
    day: "numeric",
    month: full ? "long" : "short",
    ...(full ? { year: "numeric" } : {}),
    timeZone: "UTC",
  });
const compact = new Intl.NumberFormat("ru", {
  notation: "compact",
  maximumFractionDigits: 2,
});
const number = new Intl.NumberFormat("ru", { maximumFractionDigits: 2 });
const money = (value, currency) =>
  `${number.format(Number(value) || 0)} ${currency === "STARS" ? "Stars" : "₽"}`;

export function FinanceChart({ series = [] }) {
  const canvas = useRef(null);
  const tooltip = useRef(null);
  const gradient = `finance-${useId().replace(/:/g, "")}`;
  const [size, setSize] = useState({ width: 740, height: 252 });
  const [tooltipSize, setTooltipSize] = useState({ width: 216, height: 144 });
  const [active, setActive] = useState(null);
  const [requestedCurrency, setCurrency] = useState("RUB");
  const hasStars = series.some(
    (day) => Number(day.revenueStars) > 0 || Number(day.refundsStars) > 0,
  );
  const currency = hasStars ? requestedCurrency : "RUB";
  const { plot, ceiling, ticks, points, labels, line, refundLine, hasRefunds } =
    useMemo(
      () => financeChartLayout(series, size.width, size.height, currency),
      [series, size.width, size.height, currency],
    );
  const selected = active == null ? null : points[active];

  useLayoutEffect(() => {
    const measure = () => {
      const { width, height } = canvas.current.getBoundingClientRect();
      setSize((previous) =>
        previous.width === width && previous.height === height
          ? previous
          : { width, height },
      );
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(canvas.current);
    return () => observer.disconnect();
  }, []);
  useLayoutEffect(() => {
    setActive(null);
  }, [series, currency]);
  useLayoutEffect(() => {
    if (tooltip.current) {
      const { width, height } = tooltip.current.getBoundingClientRect();
      setTooltipSize((previous) =>
        previous.width === width && previous.height === height
          ? previous
          : { width, height },
      );
    }
  }, [selected, currency]);

  const selectPoint = (event) => {
    const bounds = canvas.current.getBoundingClientRect();
    setActive(
      financeChartPointIndex(event.clientX - bounds.left, plot, points.length),
    );
  };
  const navigate = (event) => {
    const previous = active ?? points.length - 1;
    const index = {
      ArrowLeft: Math.max(0, previous - 1),
      ArrowRight: Math.min(points.length - 1, previous + 1),
      Home: 0,
      End: points.length - 1,
      Escape: null,
    }[event.key];
    if (
      ["ArrowLeft", "ArrowRight", "Home", "End", "Escape"].includes(event.key)
    ) {
      event.preventDefault();
      setActive(index);
    }
  };
  const position = selected
    ? financeTooltipPosition(
        selected,
        size.width,
        size.height,
        tooltipSize.width,
        tooltipSize.height,
      )
    : {};
  return (
    <div className="rn-finance-chart">
      <div className="rn-chart-canvas" ref={canvas}>
        {!points.length ? (
          <div className="rn-chart-empty">
            <PiChartLine size={28} />
            <strong>Нет данных за этот период</strong>
            <span>График появится после первых платежей</span>
          </div>
        ) : (
          <svg
            width="100%"
            height="100%"
            viewBox={`0 0 ${size.width} ${size.height}`}
            role="group"
            tabIndex={0}
            aria-label={`Выручка по дням, ${currency === "STARS" ? "Stars" : "рубли"}. Для просмотра значений используйте стрелки.`}
            aria-describedby={selected ? `${gradient}-tooltip` : undefined}
            onPointerMove={selectPoint}
            onPointerDown={selectPoint}
            onPointerLeave={(event) => {
              if (event.pointerType !== "touch") setActive(null);
            }}
            onFocus={(event) => {
              if (event.currentTarget.matches(":focus-visible"))
                setActive(points.length - 1);
            }}
            onBlur={() => setActive(null)}
            onKeyDown={navigate}
          >
            <defs>
              <linearGradient id={gradient} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#38d9a9" stopOpacity="0.18" />
                <stop offset="100%" stopColor="#38d9a9" stopOpacity="0" />
              </linearGradient>
            </defs>
            <text
              className="rn-chart-unit"
              x={plot.left - 12}
              y={12}
              textAnchor="end"
            >
              {currency === "STARS" ? "Stars" : "₽"}
            </text>
            {ticks.map((value) => {
              const y =
                plot.bottom - (value / ceiling) * (plot.bottom - plot.top);
              return (
                <g key={value} className="rn-chart-axis">
                  <line x1={plot.left} y1={y} x2={plot.right} y2={y} />
                  <text x={plot.left - 12} y={y + 4} textAnchor="end">
                    {compact.format(value)}
                  </text>
                </g>
              );
            })}
            <path
              className="rn-chart-area"
              fill={`url(#${gradient})`}
              d={`${line} L ${points.at(-1).x} ${plot.bottom} L ${points[0].x} ${plot.bottom} Z`}
            />
            <path className="rn-chart-line" d={line} />
            {hasRefunds && <path className="rn-chart-refunds" d={refundLine} />}
            {labels.map((point) => (
              <text
                key={point.date}
                className="rn-chart-label"
                x={point.x}
                y={size.height - 8}
                textAnchor={
                  points.length === 1
                    ? "middle"
                    : point === points[0]
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
            {(selected || points.length === 1) && (
              <g className="rn-chart-highlight">
                <circle
                  cx={(selected || points[0]).x}
                  cy={(selected || points[0]).y}
                  r={9}
                  className="rn-chart-halo"
                />
                <circle
                  cx={(selected || points[0]).x}
                  cy={(selected || points[0]).y}
                  r={3.5}
                  className="rn-chart-dot"
                />
              </g>
            )}
          </svg>
        )}
        {selected && (
          <div
            ref={tooltip}
            id={`${gradient}-tooltip`}
            role="tooltip"
            className="rn-chart-tooltip"
            style={position}
          >
            <span className="rn-chart-tooltip-date">
              {dateLabel(selected.date, true)}
            </span>
            <div className="rn-chart-tooltip-row">
              <span>
                <i />
                Выручка
              </span>
              <strong>{money(selected.value, currency)}</strong>
            </div>
            {selected.refund > 0 && (
              <div className="rn-chart-tooltip-row is-refund">
                <span>
                  <i />
                  Возвраты
                </span>
                <b>{money(selected.refund, currency)}</b>
              </div>
            )}
            {currency === "RUB" && Number(selected.revenueStars) > 0 && (
              <div className="rn-chart-tooltip-row is-secondary">
                <span>Stars отдельно</span>
                <b>{money(selected.revenueStars, "STARS")}</b>
              </div>
            )}
            {currency === "STARS" && Number(selected.revenueRub) > 0 && (
              <div className="rn-chart-tooltip-row is-secondary">
                <span>Рубли отдельно</span>
                <b>{money(selected.revenueRub, "RUB")}</b>
              </div>
            )}
            <div className="rn-chart-tooltip-row is-payments">
              <span>Платежи</span>
              <b>{number.format(selected.paymentCount || 0)}</b>
            </div>
          </div>
        )}
      </div>
      {points.length > 0 && (
        <div className="rn-chart-footer">
          <div className="rn-chart-legend">
            <span>
              <i />
              Выручка
            </span>
            {hasRefunds && (
              <span className="is-refund">
                <i />
                Возвраты
              </span>
            )}
          </div>
          {hasStars && (
            <div
              className="rn-chart-currencies"
              role="group"
              aria-label="Валюта графика"
            >
              {[
                ["RUB", "₽ Рубли"],
                ["STARS", "★ Stars"],
              ].map(([value, label]) => (
                <button
                  key={value}
                  type="button"
                  aria-pressed={currency === value}
                  onClick={() => {
                    setActive(null);
                    setCurrency(value);
                  }}
                >
                  {label}
                </button>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
