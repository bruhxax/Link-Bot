export const activitySources = { miniapp: "Кабинет", telegram: "Telegram", payment: "Платёж" };
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

export function renderUserActivity(model, { escapeHtml, escapeAttribute }) {
  const items = model?.data?.items || [];
  return `<section class="admin-user-activity" aria-labelledby="user-activity-title"><div class="admin-user-activity__head"><div><h3 id="user-activity-title">История действий</h3><span>Обновляется автоматически</span></div><button type="button" data-action="admin-user-activity-refresh" ${model.busy ? "disabled" : ""}>Обновить</button></div>
    ${model.error ? `<p role="alert">${escapeHtml(model.error)}</p>` : ""}
    ${items.length ? `<ol>${items.map(item => `<li data-status="${escapeAttribute(item.status)}"><div class="admin-user-activity__title"><strong>${escapeHtml(item.title)}</strong><small>${escapeHtml(activityStatuses[item.status] || item.status)}</small></div><div class="admin-user-activity__meta">${escapeHtml(activitySources[item.source] || item.source)} · <time datetime="${escapeAttribute(item.createdAt)}">${escapeHtml(new Date(item.createdAt).toLocaleString("ru-RU"))}</time></div>${(item.details || []).filter(detail => detail.value).map(detail=>`<div class="admin-user-activity__detail"><span>${escapeHtml(detail.label)}:</span> ${escapeHtml(detail.value)}</div>`).join("")}</li>`).join("")}</ol>` : `<p>${model.busy ? "Загрузка истории…" : "Действий пока нет. История записывается после обновления."}</p>`}
    ${model?.data?.hasMore ? `<button type="button" data-action="admin-user-activity-more" ${model.busy ? "disabled" : ""}>Показать ещё</button>` : ""}</section>`;
}
