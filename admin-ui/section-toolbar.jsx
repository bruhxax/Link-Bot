import React from "react";
import {
  PiChartBar,
  PiUsers,
  PiReceipt,
  PiCreditCard,
  PiHeadphones,
  PiChartLine,
  PiArrowsClockwise,
} from "react-icons/pi";

export function SectionToolbar({
  model,
  active,
  children,
  userTab,
  setUserTab,
}) {
  const routes = model.groups.flatMap((group) => group[2]);
  const tabs = (
    active === "users"
      ? [
          ["users-overview", "Обзор", PiChartBar],
          ["users-list", "Пользователи", PiUsers],
          ["plans", "Тарифы", PiReceipt],
          ["subscriptions", "Подписки", PiCreditCard],
        ]
      : [
          ["home", "Обзор", PiChartBar],
          ["users", "Пользователи", PiUsers],
          ["plans", "Тарифы", PiReceipt],
          ["finance", "Платежи", PiCreditCard],
          ["support", "Поддержка", PiHeadphones],
          ["analytics", "Аналитика", PiChartLine],
        ]
  ).filter(
    ([key]) =>
      ["home", "users-overview", "users-list"].includes(key) ||
      routes.some((route) => route[0] === key),
  );
  const busy = active === "home" ? model.overviewBusy : model.userBusy;
  return (
    <div className="rn-statistics-toolbar rn-section-toolbar">
      <nav className="rn-statistics-tabs" aria-label="Разделы админки">
        {tabs.map(([key, label, Icon]) => {
          const route = routes.find((item) => item[0] === key);
          const internal = key === "users-overview" || key === "users-list";
          const selected = internal ? userTab === key : active === key;
          return (
            <button
              key={key}
              className={selected ? "is-active" : undefined}
              aria-current={selected ? "page" : undefined}
              onClick={internal ? () => setUserTab(key) : undefined}
              data-action={
                internal
                  ? undefined
                  : route?.[5] === "page"
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
      {children || (
        <button
          className="rn-statistics-refresh"
          disabled={busy}
          aria-label={
            active === "home" ? "Обновить статистику" : "Обновить пользователей"
          }
          title={
            active === "home" ? "Обновить статистику" : "Обновить пользователей"
          }
          {...(active === "home"
            ? { onClick: () => model.refreshOverview() }
            : { "data-action": "refresh" })}
        >
          <PiArrowsClockwise />
        </button>
      )}
    </div>
  );
}
