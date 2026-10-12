import React, { useId, useState } from "react";
import {
  PiChartBar,
  PiUsers,
  PiCreditCard,
  PiHeadphones,
  PiChartLine,
  PiReceipt,
  PiCurrencyCircleDollar,
} from "react-icons/pi";
import {
  overviewSegments,
  groupOverviewPoints,
  overviewChartGeometry,
} from "./overview-data.mjs";
import "./overview.css";

const number = (value) =>
  value == null ? "—" : Number(value).toLocaleString("ru-RU");
const rub = (value) =>
  value == null
    ? "—"
    : `${Number(value).toLocaleString("ru-RU", { maximumFractionDigits: 2 })} ₽`;

function Distribution({ title, items, loaded, caption, note }) {
  const segments = overviewSegments(items);
  return (
    <section className="rn-overview-section">
      <div className="rn-overview-section-head">
        <h2>{title}</h2>
        {note && <span>{note}</span>}
      </div>
      <div className="rn-distribution-card" aria-busy={!loaded}>
        <ul className="rn-distribution-legend">
          {segments.items.map((item) => (
            <li key={item.key}>
              <i style={{ background: item.color }} />
              <span>{item.label}</span>
              <strong>{loaded ? number(item.count) : "—"}</strong>
            </li>
          ))}
        </ul>
        <svg
          className="rn-distribution-ring"
          viewBox="0 0 200 200"
          role="img"
          aria-label={`${title}: ${loaded ? number(segments.total) : "загрузка"}`}
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
              .map((item) => (
                <circle
                  key={item.key}
                  cx="100"
                  cy="100"
                  r="62"
                  fill="none"
                  stroke={item.color}
                  strokeWidth="36"
                  pathLength="100"
                  strokeDasharray={`${Math.max(0.1, item.fraction * 100 - 0.6)} 100`}
                  strokeDashoffset={-item.offset * 100}
                  transform="rotate(-90 100 100)"
                />
              ))}
          {loaded &&
            segments.items
              .filter((item) => item.fraction >= 0.075)
              .map((item) => {
                const angle =
                  (item.offset + item.fraction / 2) * Math.PI * 2 - Math.PI / 2;
                return (
                  <text
                    key={item.key}
                    x={100 + 62 * Math.cos(angle)}
                    y={100 + 62 * Math.sin(angle)}
                    className="rn-ring-percentage"
                  >
                    {Math.round(item.fraction * 100)}%
                  </text>
                );
              })}
          <text x="100" y="101" className="rn-ring-total">
            {loaded ? number(segments.total) : "—"}
          </text>
          <text x="100" y="122" className="rn-ring-caption">
            {caption}
          </text>
        </svg>
      </div>
    </section>
  );
}

function OverviewChart({ title, points, unit, loaded, currency = false }) {
  const id = useId().replace(/:/g, ""),
    data = groupOverviewPoints(points, unit);
  const graph = overviewChartGeometry(data);
  const [hover, setHover] = useState(null);
  const value = hover == null ? null : graph.coordinates[hover];
  const date = (input) =>
    new Date(`${input}T12:00:00Z`).toLocaleDateString("ru-RU", {
      day: "2-digit",
      month: "2-digit",
    });
  const labelIndices = [
    ...new Set([
      0,
      Math.floor((data.length - 1) / 4),
      Math.floor((data.length - 1) / 2),
      Math.floor((3 * (data.length - 1)) / 4),
      data.length - 1,
    ]),
  ].filter((i) => i >= 0);
  return (
    <section className="rn-overview-chart" aria-busy={!loaded}>
      <div className="rn-overview-chart-head">
        <h3>{title}</h3>
        <span>
          {value
            ? `${date(value.date)} · ${currency ? rub(value.value) : number(value.value)}`
            : currency
              ? "₽"
              : ""}
        </span>
      </div>
      <svg
        viewBox={`0 0 ${graph.width} ${graph.height}`}
        role="img"
        aria-label={`${title}, ${loaded ? data.length + " периодов" : "загрузка"}`}
        onMouseLeave={() => setHover(null)}
      >
        <defs>
          <linearGradient
            id={`overview-fill-${id}`}
            x1="0"
            y1="0"
            x2="0"
            y2="1"
          >
            <stop offset="0" stopColor="#7895a7" stopOpacity=".24" />
            <stop offset="1" stopColor="#7895a7" stopOpacity=".015" />
          </linearGradient>
        </defs>
        {[0, 1, 2, 3, 4].map((index) => {
          const y =
            graph.height -
            graph.bottom -
            (index / 4) * (graph.height - graph.top - graph.bottom);
          return (
            <g key={index}>
              <line
                x1={graph.left}
                x2={graph.width - graph.right}
                y1={y}
                y2={y}
                className="rn-chart-grid-line"
              />
              <text x={graph.left - 14} y={y + 4} textAnchor="end">
                {number((graph.ceiling * index) / 4)}
              </text>
            </g>
          );
        })}
        {loaded && (
          <>
            <path d={graph.area} fill={`url(#overview-fill-${id})`} />
            <path
              d={graph.line}
              fill="none"
              stroke="#7691a2"
              strokeWidth="2"
              strokeLinejoin="round"
              vectorEffect="non-scaling-stroke"
            />
          </>
        )}
        {labelIndices.map((index) => (
          <text
            key={index}
            x={graph.coordinates[index]?.x}
            y={graph.height - 5}
            textAnchor={
              index === 0
                ? "start"
                : index === data.length - 1
                  ? "end"
                  : "middle"
            }
          >
            {data[index] ? date(data[index].date) : ""}
          </text>
        ))}
        {graph.coordinates.map((point, index) => (
          <rect
            key={point.date}
            x={
              point.x -
              (graph.width - graph.left - graph.right) /
                Math.max(1, data.length - 1) /
                2
            }
            y={graph.top}
            width={
              (graph.width - graph.left - graph.right) /
              Math.max(1, data.length - 1)
            }
            height={graph.height - graph.top - graph.bottom}
            fill="transparent"
            onMouseEnter={() => setHover(index)}
          >
            <title>
              {date(point.date)}:{" "}
              {currency ? rub(point.value) : number(point.value)}
            </title>
          </rect>
        ))}
        {value && <circle cx={value.x} cy={value.y} r="4" fill="#adc3d0" />}
      </svg>
      {!loaded && <span className="rn-chart-status">Загрузка статистики…</span>}
      {loaded && data.length === 0 && (
        <span className="rn-chart-status">Данных за этот период пока нет</span>
      )}
    </section>
  );
}

export function Overview({ model }) {
  const data = model.overview,
    users = data?.users,
    revenue = data?.revenue;
  const [unit, setUnit] = useState("weeks");
  const routes = model.groups.flatMap((group) => group[2]);
  const tabs = [
    ["users", "Пользователи", PiUsers],
    ["plans", "Тарифы", PiReceipt],
    ["finance", "Платежи", PiCreditCard],
    ["support", "Поддержка", PiHeadphones],
    ["analytics", "Аналитика", PiChartLine],
  ].filter(([key]) => routes.some((route) => route[0] === key));
  const userItems = [
    {
      key: "active",
      label: "Активные",
      count: users?.active,
      color: "#69b7a7",
    },
    {
      key: "expired",
      label: "Истекла подписка",
      count: users?.expired,
      color: "#edbd70",
    },
    {
      key: "without",
      label: "Без подписки",
      count: users?.without,
      color: "#6c8b9e",
    },
    {
      key: "blocked",
      label: "Заблокированы",
      count: users?.blocked,
      color: "#b2b1ed",
    },
  ];
  const planItems = (data?.subscriptions || []).map((item) => ({
    ...item,
    color: { free: "#edbd70", manual: "#d47db5", paid: "#53868b" }[item.key],
    label:
      {
        free: "Бесплатный тариф",
        manual: "Без покупки",
        paid: "Платный тариф",
      }[item.key] ||
      model.plans.find((plan) => plan.id === item.key)?.titleRu ||
      item.key,
  }));
  return (
    <div className="rn-statistics" aria-busy={model.overviewBusy}>
      <nav className="rn-statistics-tabs" aria-label="Разделы статистики">
        <button className="is-active" aria-current="page">
          <PiChartBar />
          Обзор
        </button>
        {tabs.map(([key, label, Icon]) => {
          const route = routes.find((route) => route[0] === key);
          return (
            <button
              key={key}
              data-action={
                route[5] === "page"
                  ? "admin-console-page"
                  : "open-admin-section"
              }
              data-value={key}
            >
              <Icon />
              {label}
            </button>
          );
        })}
      </nav>
      {model.overviewError && (
        <div className="rn-overview-error" role="alert">
          {model.overviewError}
          <button onClick={() => model.refreshOverview()}>Повторить</button>
        </div>
      )}
      {model.canOverviewUsers && (
        <div className="rn-overview-distributions">
          <Distribution
            title="Пользователи"
            caption="всего"
            loaded={Boolean(users)}
            items={userItems}
            note={`сегодня: ${number(users?.today)} · неделя: ${number(users?.week)}`}
          />
          <Distribution
            title="Распределение подписок"
            caption="активных"
            loaded={Boolean(data)}
            items={
              planItems.length
                ? planItems
                : [
                    {
                      key: "empty",
                      label: "Активные подписки",
                      count: 0,
                      color: "#edbd70",
                    },
                  ]
            }
            note="По последней покупке"
          />
        </div>
      )}
      {model.canOverviewFinance && (
        <section className="rn-overview-section rn-overview-income">
          <h2>Доходы</h2>
          <div className="rn-overview-income-card">
            {[
              ["Сегодня", revenue?.todayRub, PiCurrencyCircleDollar],
              ["За неделю", revenue?.weekRub, PiCurrencyCircleDollar],
              ["За месяц", revenue?.monthRub, PiCurrencyCircleDollar],
              ["Средний чек", revenue?.averageRub, PiReceipt],
              ["За всё время", revenue?.allRub, PiCurrencyCircleDollar],
            ].map(([label, value, Icon]) => (
              <div key={label}>
                <span>
                  <Icon />
                  {label}
                </span>
                <strong>{rub(value)}</strong>
              </div>
            ))}
            <div>
              <span>
                <PiCurrencyCircleDollar />
                За всё время (Stars)
              </span>
              <strong>
                {number(revenue?.allStars)}{" "}
                <span className="rn-stars-unit">★</span>
              </strong>
            </div>
          </div>
        </section>
      )}
      {model.canOverviewUsers || model.canOverviewFinance ? (
        <section className="rn-overview-section rn-overview-graphs">
          <div className="rn-overview-section-head">
            <h2>Графики</h2>
            <div className="rn-overview-chart-controls">
              <div role="group" aria-label="Период графиков">
                {[7, 30, 90].map((days) => (
                  <button
                    key={days}
                    aria-pressed={model.overviewDays === days}
                    onClick={() => model.setOverviewDays(days)}
                  >
                    {days} дней
                  </button>
                ))}
              </div>
              <div role="group" aria-label="Группировка графиков">
                {[
                  ["days", "Дни"],
                  ["weeks", "Недели"],
                  ["months", "Месяцы"],
                ].map(([value, label]) => (
                  <button
                    key={value}
                    aria-pressed={unit === value}
                    onClick={() => setUnit(value)}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>
          </div>
          <div className="rn-overview-charts">
            {model.canOverviewUsers && (
              <OverviewChart
                title="Новые пользователи"
                points={users?.daily}
                unit={unit}
                loaded={Boolean(users)}
              />
            )}{" "}
            {model.canOverviewFinance && (
              <OverviewChart
                title="Доходы"
                points={revenue?.daily}
                unit={unit}
                loaded={Boolean(revenue)}
                currency
              />
            )}
          </div>
        </section>
      ) : (
        <div className="rn-overview-no-access">
          Статистика доступна администраторам с правами просмотра пользователей
          или финансов. Используйте меню слева для доступных разделов.
        </div>
      )}
    </div>
  );
}
