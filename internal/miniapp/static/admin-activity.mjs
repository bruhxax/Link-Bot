export const ACTIVITY_CATEGORIES = [
  ["", "Все действия"], ["users", "Пользователи"], ["settings", "Настройки"],
  ["support", "Обращения"], ["communication", "Рассылки и сообщения"],
  ["servers", "Ноды"], ["access", "Права доступа"], ["system", "Система"],
];

function dateText(value, options) {
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? new Intl.DateTimeFormat("ru-RU", options).format(date) : "—";
}

function recordCount(count) {
  const last = count % 10, hundred = count % 100;
  const word = hundred >= 11 && hundred <= 14 ? "записей" : last === 1 ? "запись" : last >= 2 && last <= 4 ? "записи" : "записей";
  return `${count} ${word}`;
}

export function renderAdminActivity(activity = {}, { escapeHtml: html, escapeAttribute: attr, icon }) {
  const items = activity.items || [];
  const symbols = { users: "userAlt", settings: "adminFeatures", support: "chat", communication: "send", servers: "server", access: "users", system: "adminDiagnostics" };
  const categoryNames = Object.fromEntries(ACTIVITY_CATEGORIES);
  let lastDay = "";
  const rows = items.map((entry) => {
    const day = dateText(entry.createdAt, { day: "numeric", month: "long", year: "numeric" });
    const group = day !== lastDay ? `<h3 class="admin-activity__day">${html(day)}</h3>` : "";
    lastDay = day;
    const status = ["success", "failed", "pending"].includes(entry.status) ? entry.status : "pending";
    const stale = status === "pending" && Date.now() - new Date(entry.createdAt).getTime() > 600000;
    const statusCopy = stale ? "Не завершено" : ({ success: "Выполнено", failed: "Не выполнено", pending: "В процессе" })[status];
    const target = entry.targetName ? `@${entry.targetName.replace(/^@+/, "")}` : entry.targetTelegramId ? `Telegram ID: ${entry.targetTelegramId}` : entry.targetCustomerId ? `Пользователь №${entry.targetCustomerId}` : "";
    const details = (entry.details || []).map((detail) => `<div class="admin-activity__detail"><dt>${html(detail.label)}</dt><dd>${detail.before !== undefined || detail.after !== undefined ? `<span class="admin-activity__before">${html(detail.before || "Не задано")}</span><span class="admin-activity__arrow" aria-label="изменено на">→</span><span>${html(detail.after || "Не задано")}</span>` : html(detail.value || "—")}</dd></div>`).join("");
    return `${group}<details class="admin-activity__entry" data-activity-id="${Number(entry.id)}"><summary><span class="admin-activity__icon">${icon(symbols[entry.category] || "clock")}</span><span class="admin-activity__identity"><strong>${html(entry.title || "Действие администратора")}</strong><small>${html(target || categoryNames[entry.category] || "Система")}${status !== "success" ? `<span class="admin-activity__inline-status admin-activity__status--${status}"> · ${statusCopy}</span>` : ""}</small></span><span class="admin-activity__time">${html(dateText(entry.createdAt, { hour: "2-digit", minute: "2-digit" }))}${icon("chevronRight")}</span></summary><div class="admin-activity__body"><div class="admin-activity__meta"><span class="admin-activity__status admin-activity__status--${status}">${statusCopy}</span><span>${html(categoryNames[entry.category] || "Система")}</span></div>${entry.targetTelegramId && entry.targetName ? `<p class="admin-activity__target">Telegram ID: ${html(String(entry.targetTelegramId))}</p>` : ""}${details ? `<dl>${details}</dl>` : `<p class="admin-activity__note">Дополнительных параметров нет.</p>`}<footer>${html(entry.actorName ? `@${entry.actorName.replace(/^@+/, "")}` : `Telegram ID: ${entry.actorTelegramId}`)}${entry.actorRole ? ` · ${html(entry.actorRole)}` : ""}</footer></div></details>`;
  }).join("");
  return `<section class="admin-activity" aria-label="Активность администратора"><div class="admin-activity__toolbar"><label class="admin-users__search">${icon("search")}<input type="search" data-input="administrators-activity-search" value="${attr(activity.query || "")}" placeholder="Действие, пользователь или настройка" aria-label="Поиск по активности" maxlength="100" autocomplete="off"></label><button type="button" data-action="administrators-activity-refresh" aria-label="Обновить журнал" ${activity.loading ? "disabled" : ""}>${icon("refresh")}</button></div><label class="admin-activity__filter"><span class="sr-only">Тип действий</span><select data-input="administrators-activity-category" aria-label="Тип действий">${ACTIVITY_CATEGORIES.map(([value, label]) => `<option value="${value}" ${activity.category === value || (!activity.category && !value) ? "selected" : ""}>${label}</option>`).join("")}</select><small>${activity.loading ? "Загружаем…" : activity.hasMore ? `Показано: ${items.length}` : recordCount(items.length)}</small></label>${activity.error ? `<div class="admin-access__error" role="alert">${html(activity.error)}<button type="button" data-action="administrators-activity-refresh">Повторить</button></div>` : ""}<div class="admin-activity__list" aria-busy="${Boolean(activity.loading)}">${rows || `<div class="admin-users__empty">${icon(activity.loading ? "refresh" : "clock")}<strong>${activity.loading ? "Загружаем журнал" : activity.query || activity.category ? "Ничего не найдено" : "Действий пока нет"}</strong><span>${activity.loading ? "" : activity.query || activity.category ? "Измените поиск или фильтр" : "Здесь появятся действия после установки обновления"}</span></div>`}</div>${activity.hasMore ? `<button class="admin-users__more" type="button" data-action="administrators-activity-more" ${activity.loading ? "disabled" : ""}>${activity.loading ? "Загружаем…" : "Показать ещё"}</button>` : ""}</section>`;
}
