import React, { useState } from "react";
import { overviewSegments, overviewRingPath } from "./overview-data.mjs";

const number = (value) => Number(value).toLocaleString("ru-RU");

export function Distribution({
  title,
  items,
  loaded,
  caption,
  note,
  hidden = false,
}) {
  const [hovered, setHovered] = useState(null);
  const segments = overviewSegments(items);
  const selected = loaded
    ? segments.items.find((item) => item.key === hovered)
    : null;
  const percentage = selected ? Math.round(selected.fraction * 100) : null;
  const clear = (event) => {
    if (event.pointerType !== "touch") setHovered(null);
  };
  return (
    <section className="rn-overview-section">
      <div className="rn-overview-section-head">
        <h2>{title}</h2>
        {note && <span>{note}</span>}
      </div>
      <div
        className={`rn-distribution-card ${selected ? "has-highlight" : ""}`}
        aria-busy={!loaded}
      >
        <ul className="rn-distribution-legend">
          {segments.items.map((item) => (
            <li
              key={item.key}
              data-highlighted={selected?.key === item.key || undefined}
              data-dimmed={(selected && selected.key !== item.key) || undefined}
            >
              <button
                className="rn-distribution-legend-button"
                onPointerEnter={() => setHovered(item.key)}
                onPointerLeave={clear}
                onFocus={() => setHovered(item.key)}
                onBlur={() => setHovered(null)}
                onClick={() => setHovered(item.key)}
                aria-label={`${item.label}: ${hidden ? "значение скрыто" : loaded ? number(item.count) : "загрузка"}`}
              >
                <i style={{ background: item.color }} />
                <span>{item.label}</span>
                <strong className="rn-private-value" aria-hidden={hidden}>
                  {loaded ? number(item.count) : "—"}
                </strong>
              </button>
            </li>
          ))}
        </ul>
        <div className="rn-distribution-ring-wrap">
          <svg
            className="rn-distribution-ring"
            viewBox="0 0 200 200"
            role="group"
            aria-label={`${title}: ${hidden ? "данные скрыты" : loaded ? number(segments.total) : "загрузка"}`}
            onPointerLeave={clear}
            onPointerMove={(event) => {
              const box = event.currentTarget.getBoundingClientRect(),
                radius = Math.hypot(
                  ((event.clientX - box.left) / box.width) * 200 - 100,
                  ((event.clientY - box.top) / box.height) * 200 - 100,
                );
              if (radius < 35 || radius > 98) setHovered(null);
            }}
          >
            <circle
              cx="100"
              cy="100"
              r="62"
              fill="none"
              stroke="#2b2e34"
              strokeWidth="36"
            />
            {loaded &&
              segments.items
                .filter((item) => item.fraction > 0)
                .map((item) => {
                  const angle =
                    (item.offset + item.fraction / 2) * Math.PI * 2 -
                    Math.PI / 2;
                  const active = selected?.key === item.key;
                  return (
                    <g
                      key={item.key}
                      onPointerEnter={() => setHovered(item.key)}
                      onFocus={() => setHovered(item.key)}
                      onBlur={() => setHovered(null)}
                      onClick={() => setHovered(item.key)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault();
                          setHovered(item.key);
                        }
                        if (event.key === "Escape") setHovered(null);
                      }}
                    >
                      <g
                        className="rn-ring-segment"
                        data-segment={item.key}
                        data-highlighted={active || undefined}
                        data-dimmed={(selected && !active) || undefined}
                        style={{
                          transform: `translate(${active ? Math.cos(angle) * 2.5 : 0}px,${active ? Math.sin(angle) * 2.5 : 0}px)`,
                        }}
                      >
                        <path d={overviewRingPath(item)} fill={item.color} />
                        {item.fraction >= 0.075 && (
                          <text
                            x={100 + 62 * Math.cos(angle)}
                            y={100 + 62 * Math.sin(angle)}
                            className="rn-ring-percentage rn-private-value"
                            aria-hidden={hidden}
                            pointerEvents="none"
                          >
                            {Math.round(item.fraction * 100)}%
                          </text>
                        )}
                      </g>
                      <path
                        className="rn-ring-hit"
                        d={overviewRingPath(item)}
                        fill="transparent"
                        tabIndex={0}
                        role="button"
                        aria-label={
                          hidden
                            ? `${item.label}: значение скрыто`
                            : `${item.label}: ${number(item.count)}, ${Math.round(item.fraction * 100)}%`
                        }
                      />
                    </g>
                  );
                })}
            <text
              x="100"
              y="101"
              className="rn-ring-total rn-private-value"
              aria-hidden={hidden}
              pointerEvents="none"
            >
              {loaded
                ? number(selected ? selected.count : segments.total)
                : "—"}
            </text>
            <text
              x="100"
              y="122"
              className={`rn-ring-caption ${selected ? "rn-private-value" : ""}`}
              aria-hidden={hidden && Boolean(selected)}
              pointerEvents="none"
            >
              {selected ? `${percentage}%` : caption}
            </text>
          </svg>
          <div
            className="rn-ring-tooltip"
            role="status"
            aria-live="polite"
            data-visible={Boolean(selected)}
          >
            {selected && (
              <>
                <strong>{selected.label}</strong>
                <span className="rn-private-value" aria-hidden={hidden}>
                  {number(selected.count)}{" "}
                  {caption === "активных" ? "подписок" : "пользователей"} ·{" "}
                  {percentage}%
                </span>
              </>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
