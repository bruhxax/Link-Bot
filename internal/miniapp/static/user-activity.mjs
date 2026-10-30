export const activitySources = { miniapp: "Кабинет", telegram: "Telegram", payment: "Платёж", remnawave: "Remnawave" };
export const activityStatuses = { info: "Действие", pending: "Ожидание", success: "Успешно", failed: "Ошибка" };

export function mergeUserActivity(current, incoming, append = false) {
  if (!incoming || !Array.isArray(incoming.items)) return current;
  const same = Number(current?.customerId) === Number(incoming.customerId);
  const existing = same && Array.isArray(current.items) ? current.items : [];
  // If an entire page arrived between refreshes, show a contiguous new page;
  // subsequent pagination must not silently skip the intervening events.
  const gap = !append && existing.length > 0 && incoming.items.length > 0
    && incoming.items.at(-1).id > existing[0].id;
  const all = [...(gap ? [] : existing), ...incoming.items];
  const items = [...new Map(all.map(item => [item.id, item])).values()].sort((a,b) => b.id-a.id);
  const hasMore = append || !same || gap || !existing.length ? incoming.hasMore : current.hasMore;
  return { customerId: incoming.customerId, items, hasMore: Boolean(hasMore) };
}

export function escapeActivityHTML(value) {
  return String(value ?? "").replace(/[&<>"']/g, character => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[character]));
}

const svg = path => `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${path}</svg>`;
const glyphs = {
  history: svg('<path d="M3 11a9 9 0 1 1 2.6 7M3 4v7h7M12 7v5l3 2"/>'),
  refresh: svg('<path d="M20 7v5h-5M4 17v-5h5M5.5 7a8 8 0 0 1 13-2L20 7M4 17l1.5 2a8 8 0 0 0 13-2"/>'),
  check: svg('<path d="m5 12 4 4L19 6"/>'),
  clock: svg('<circle cx="12" cy="12" r="8"/><path d="M12 7v5l3 2"/>'),
  close: svg('<path d="m7 7 10 10M17 7 7 17"/>'),
  arrow: svg('<path d="m9 5 7 7-7 7"/>'),
  action: svg('<path d="m5 4 5 16 3-7 7-3Z"/>'),
  more: svg('<path d="m6 9 6 6 6-6"/>'),
  torrent: svg('<path d="M8 3v9a4 4 0 0 0 8 0V3M5 3h6M13 3h6M5 7h6M13 7h6"/>'),
};

export function formatActivityAmount(value) {
  const match = String(value || "").trim().match(/^(-?\d+(?:\.\d+)?(?:e[+-]?\d+)?)\s+([A-Z]{2,10})$/i);
  if (!match || !Number.isFinite(Number(match[1]))) return String(value || "");
  const currency = match[2].toUpperCase();
  const amount = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: ["RUB","USD","EUR","XTR"].includes(currency) ? 2 : 8 }).format(Number(match[1]));
  return `${amount} ${{RUB:"₽",USD:"$",EUR:"€",XTR:"Stars"}[currency] || currency}`;
}

export function activityTitle(item) {
  return ({"payment.new":"Счёт создан","payment.pending":"Ожидает оплаты","payment.paid":"Оплата получена","payment.cancel":"Платёж отменён",
    purchase:"Запрос оплаты","gifts/purchase":"Покупка подарка","purchase/cancel":"Отмена оплаты","trial/activate":"Пробная подписка",
    "promocode/apply":"Применение промокода","promocode/redeem":"Активация промокода","wallet/withdraw":"Вывод средств",
  })[item.action] || item.title;
}

export function activityDetails(item) {
  const details = item.details || [];
  return details.filter(detail => detail.value && !(detail.label === "Автоплатёж" && detail.value === "Нет")
    && !(detail.label === "Тип покупки" && detail.value === "Подписка")
    && !(detail.label === "Месяцев" && (Number(detail.value) === 0 || details.some(entry => entry.label === "Тариф" && entry.value))))
    .map(detail => ({...detail,value:detail.label === "Сумма" ? formatActivityAmount(detail.value) : detail.value}));
}

export function activityRows(items, filter = "all") {
  const rows = new Map();
  for (const item of [...items].sort((a,b) => b.id-a.id)) {
    const order = item.source === "payment" ? item.details?.find(detail => detail.label === "Платёж")?.value?.match(/^#?(\d+)$/)?.[1] : null;
    const key = order ? `payment:${order}` : `event:${item.id}`;
    if (!rows.has(key)) rows.set(key, {key,item,events:[]});
    rows.get(key).events.push(item);
  }
  return [...rows.values()].filter(row => filter === "torrents" ? row.item.action === "torrent_blocker.report" : filter === "errors" ? row.events.some(item => item.status === "failed")
    : filter === "payments" ? row.item.source === "payment" || ["purchase","gifts/purchase","purchase/cancel","payment"].includes(row.item.action) : true);
}

export function renderUserActivity(model = {}, { escapeHtml = escapeActivityHTML, escapeAttribute = escapeActivityHTML, refreshIcon = glyphs.refresh } = {}) {
  const items = model.data?.items || [];
  const filter = ["all","payments","errors","torrents"].includes(model.filter) ? model.filter : "all";
  const rows = activityRows(items, filter);
  const expanded = new Set(model.expanded || []);
  const disabled = model.busy ? "disabled" : "";
  const prefix = escapeAttribute(model.idPrefix || "user-activity");
  return `<section class="admin-user-activity" aria-labelledby="${prefix}-title"><header class="admin-user-activity__head"><h3 id="${prefix}-title">${glyphs.history}<span>История действий</span></h3><button class="user-activity-icon" type="button" data-action="admin-user-activity-refresh" aria-label="Обновить историю" title="Обновить историю" ${disabled}>${refreshIcon}</button></header>
    ${items.length ? `<nav class="user-activity-filters" aria-label="Фильтр истории">${[["all","Все"],["payments","Оплаты"],["errors","Ошибки"],["torrents","Торренты"]].map(([value,label]) => `<button type="button" data-action="admin-user-activity-filter" data-value="${value}" aria-pressed="${filter === value}">${label}</button>`).join("")}</nav>` : ""}
    ${model.error ? `<p class="user-activity-error" role="alert">${escapeHtml(model.error)}</p>` : ""}
    <ol class="user-activity-list">${rows.map(row => {
      const {item,key,events} = row;
      const details = activityDetails(item);
      const amount = details.find(detail => detail.label === "Сумма")?.value || "";
      const info = details.filter(detail => ["Тариф","Способ","Платёж"].includes(detail.label)).map(detail => detail.value).join(" · ") || activitySources[item.source] || item.source;
      const reason = item.status === "failed" ? details.find(detail => detail.label === "Причина")?.value : "";
      const date = new Date(item.createdAt);
      const stamp = date.toLocaleString("ru-RU",{day:"2-digit",month:"2-digit",hour:"2-digit",minute:"2-digit"});
      const kind = item.action === "torrent_blocker.report" ? "torrent" : item.action === "payment.cancel" ? "close" : {success:"check",failed:"close",pending:"clock"}[item.status] || "action";
      const open = expanded.has(key);
      return `<li class="user-activity-entry" data-status="${escapeAttribute(item.status)}"><button class="user-activity-row" type="button" data-action="admin-user-activity-toggle" data-value="${escapeAttribute(key)}" aria-expanded="${open}" aria-controls="${prefix}-${escapeAttribute(key.replace(":","-"))}"><span class="user-activity-glyph" title="${escapeAttribute(activityStatuses[item.status] || item.status)}">${glyphs[kind]}</span><span class="user-activity-copy"><strong>${escapeHtml(activityTitle(item))}</strong><small>${escapeHtml(reason || info)}</small></span><span class="user-activity-value">${amount ? `<b>${escapeHtml(amount)}</b>` : ""}<time datetime="${escapeAttribute(item.createdAt)}" title="${escapeAttribute(date.toLocaleString("ru-RU"))}">${escapeHtml(stamp)}</time></span><span class="user-activity-chevron">${glyphs.arrow}</span></button>
      ${open ? `<div class="user-activity-details" id="${prefix}-${escapeAttribute(key.replace(":","-"))}"><dl>${details.map(detail => `<div><dt>${escapeHtml(detail.label)}</dt><dd>${escapeHtml(detail.value)}</dd></div>`).join("")}<div><dt>Источник</dt><dd>${escapeHtml(activitySources[item.source] || item.source)}</dd></div><div><dt>Время</dt><dd>${escapeHtml(date.toLocaleString("ru-RU"))}</dd></div></dl>${events.length > 1 ? `<div class="user-activity-states">${events.map(event => `<span><i data-status="${escapeAttribute(event.status)}"></i>${escapeHtml(activityTitle(event))}<time>${escapeHtml(new Date(event.createdAt).toLocaleTimeString("ru-RU"))}</time></span>`).join("")}</div>` : ""}</div>` : ""}</li>`;
    }).join("")}</ol>
    ${!rows.length ? `<p class="user-activity-empty">${model.busy ? "Загрузка…" : items.length ? "Нет событий по этому фильтру" : "Действий пока нет"}</p>` : ""}
    ${model.data?.hasMore ? `<button class="user-activity-more" type="button" data-action="admin-user-activity-more" ${disabled}>${glyphs.more}<span>Показать ещё</span></button>` : ""}</section>`;
}
