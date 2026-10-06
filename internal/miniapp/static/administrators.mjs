import { renderAdminActivity } from "./admin-activity.mjs";

export function canAdmin(access, permission) {
  if (!access?.isAdmin) return false;
  if (access.isOwner) return true;
  if (permission === "administrators") return false;
  return Array.isArray(access.permissions) && access.permissions.includes(permission);
}

export function changePermission(selected, id, enabled, catalog) {
  const result = new Set(selected);
  const add = (key) => {
    const entry = catalog.find((item) => item.id === key);
    if (!entry || result.has(key)) return;
    result.add(key);
    (entry.requires || []).forEach(add);
  };
  if (enabled) add(id);
  else {
    result.delete(id);
    let changed;
    do {
      changed = false;
      for (const entry of catalog) {
        if (result.has(entry.id) && (entry.requires || []).some((key) => !result.has(key))) {
          result.delete(entry.id);
          changed = true;
        }
      }
    } while (changed);
  }
  return catalog.filter((item) => result.has(item.id)).map((item) => item.id);
}

export const ROLE_PRESETS = [
  { id: "admin", name: "Администратор", color: "#69a8d4", permissions: null },
  { id: "support", name: "Поддержка", color: "#55c58a", permissions: ["users", "support.view", "support.reply", "support.close", "servers.view"] },
  { id: "observer", name: "Наблюдатель", color: "#aa8bd4", permissions: ["status", "users", "finance", "analytics", "servers.view"] },
];

export function presetPermissions(id, catalog) {
  const preset = ROLE_PRESETS.find((item) => item.id === id);
  if (!preset) return [];
  return catalog.filter((item) => preset.permissions === null || preset.permissions.includes(item.id)).map((item) => item.id);
}

function permissionCountLabel(count) {
  const last = count % 10, hundred = count % 100;
  const word = hundred >= 11 && hundred <= 14 ? "прав" : last === 1 ? "право" : last >= 2 && last <= 4 ? "права" : "прав";
  return `${count} ${word}`;
}

export function roleDot(color, role, { escapeAttribute: attr }) {
  return /^#[\da-f]{6}$/i.test(color || "") ? `<i class="admin-role-dot" style="--role-color:${color}" title="${attr(role || "Администратор")}" aria-label="${attr(role || "Администратор")}"></i>` : "";
}

export function renderAdministrators(s, helpers) {
  const { escapeHtml: html, escapeAttribute: attr, icon, avatar, displayName, loading } = helpers;
  const busy = s.busy ? "disabled" : "";
  const header = (title, action = "administrators-back") => `<header class="admin-users__header admin-access__header"><button class="admin-access__back" type="button" data-action="${action}" aria-label="Назад">${icon("back")}</button></header>`;
  const row = (a, picking = false, index = 0) => `<button class="admin-user-row admin-access__row" type="button" data-action="${picking ? "administrators-pick" : "administrators-edit"}" data-value="${Number(a.customerId)}" style="--admin-user-index:${Math.min(index, 10)}">${avatar(a)}<span class="admin-user-row__identity"><strong>${roleDot(a.color || a.adminColor, a.role || a.adminRole, helpers)}${html(displayName(a))}</strong><small>Telegram ID: ${html(String(a.telegramId))}</small></span><span class="admin-user-row__subscription"><strong>${html(picking ? (a.adminRole || a.subscriptionName || "Без подписки") : a.role)}</strong><small>${picking ? (a.isBlocked ? "Заблокирован" : a.adminRole ? "Уже администратор" : "Выбрать") : a.isOwner ? "Все права" : permissionCountLabel(a.permissions.length)}</small></span>${icon("chevronRight")}</button>`;
  const search = (key, query, placeholder) => `<label class="admin-users__search">${icon("search")}<input type="search" data-input="${key}" value="${attr(query)}" placeholder="${placeholder}" aria-label="${placeholder}" autocomplete="off" enterkeyhint="search"><i class="${s.busy === "search" ? "is-visible" : ""}" aria-hidden="true"></i></label>`;
  const error = s.error ? `<div class="admin-access__error" role="alert">${html(s.error)}<button type="button" data-action="${s.editor ? s.confirmRemove ? "administrators-remove" : "administrators-save" : "administrators-refresh"}" ${busy}>Повторить</button></div>` : "";
  if (s.editor) {
    const a = s.editor;
    const owner = a.isOwner;
    const disabled = owner || s.busy ? "disabled" : "";
    const groups = [...new Set(s.catalog.map((item) => item.group))];
    const selected = new Set(a.permissions);
    const count = owner ? s.catalog.length : selected.size;
    const introduction = `${header(owner ? "Главный администратор" : "Настройка роли")}<div class="admin-access-card admin-access__person">${avatar(a, "large")}<div><strong>${html(displayName(a))}</strong><small>Telegram ID: ${html(String(a.telegramId))}</small></div>${roleDot(a.color, a.role, helpers)}</div>${a.isNew ? "" : `<div class="tabs admin-access__tabs" data-animated-switch="administrator-editor"><span class="switch-selection" data-switch-indicator aria-hidden="true"></span>${[["role", "Права доступа"], ["activity", "Активность"]].map(([value, label]) => `<button class="tab ${(s.editorTab || "role") === value ? "active" : ""}" type="button" data-action="administrators-tab" data-value="${value}" aria-pressed="${(s.editorTab || "role") === value}">${label}</button>`).join("")}</div>`}`;
    if (!a.isNew && s.editorTab === "activity") return `<div class="admin-users admin-access">${introduction}${renderAdminActivity(s.activity, helpers)}</div>`;
    return `<div class="admin-users admin-access">${introduction}
      ${owner ? `<p class="admin-access__hint">Главный администратор имеет полный доступ. Его аккаунт задаётся в настройках сервера.</p>` : `<section class="admin-access-card"><span class="admin-access__label">РОЛЬ</span><div class="admin-access__presets">${ROLE_PRESETS.map((p) => `<button type="button" class="${a.preset === p.id ? "is-selected" : ""}" data-action="administrators-preset" data-value="${p.id}" ${busy}>${html(p.name)}</button>`).join("")}</div><label class="admin-field"><span>Название роли</span><input class="admin-field__control" type="text" data-input="administrators-role" maxlength="60" value="${attr(a.role)}" placeholder="Своя роль" ${busy}></label><div class="admin-access__color"><span>Цвет в списке пользователей</span><div>${["#69a8d4", "#55c58a", "#aa8bd4", "#e4b567", "#ef8796", "#71c8bf"].map((color) => `<button type="button" data-action="administrators-color" data-value="${color}" class="${a.color === color ? "is-selected" : ""}" style="--role-color:${color}" aria-label="Цвет ${color}" ${busy}></button>`).join("")}<label title="Свой цвет"><span class="sr-only">Свой цвет роли</span><input type="color" aria-label="Свой цвет роли" data-input="administrators-color" value="${attr(a.color)}" ${busy}></label></div></div></section>`}
      <div class="admin-access__permissions-head"><div><strong>Права доступа</strong><small>${count} из ${s.catalog.length}</small></div>${owner ? "" : `<button type="button" data-action="administrators-all" ${busy}>${count === s.catalog.length ? "Снять все" : "Выбрать все"}</button>`}</div>
      ${groups.map((group) => `<section class="admin-access-card admin-access__permissions"><header><h3>${html(group)}</h3>${owner ? "" : `<button type="button" data-action="administrators-group" data-value="${attr(group)}" ${busy}>${s.catalog.filter((p) => p.group === group).every((p) => selected.has(p.id)) ? "Снять" : "Выбрать"}</button>`}</header>${s.catalog.filter((p) => p.group === group).map((p) => `<label class="admin-access__permission"><span>${html(p.label)}</span><input type="checkbox" data-input="administrators-permission" data-value="${attr(p.id)}" ${owner || selected.has(p.id) ? "checked" : ""} ${disabled}><i aria-hidden="true">${icon("check")}</i></label>`).join("")}</section>`).join("")}
      ${error}${owner ? "" : `<button class="admin-access__save btn" type="button" data-action="administrators-save" ${s.busy || !a.role.trim() || !count ? "disabled" : ""}>${icon("check")}<span>${s.busy === "save" ? "Сохраняем…" : a.isNew ? "Добавить администратора" : "Сохранить роль"}</span></button>${!a.isNew ? `<div class="admin-access__remove">${s.confirmRemove ? `<strong>Снять права администратора?</strong><p>Пользователь останется в сервисе. Доступ к админке будет закрыт.</p><div><button type="button" data-action="administrators-cancel-remove" ${busy}>Отмена</button><button type="button" data-action="administrators-remove" ${busy}>${s.busy === "remove" ? "Удаляем…" : "Снять права"}</button></div>` : `<button type="button" data-action="administrators-confirm-remove" ${busy}>${icon("trash")}Удалить из администраторов</button>`}</div>` : ""}`}
    </div>`;
  }
  if (s.picking) return `<div class="admin-users admin-access">${header("Добавить администратора")}${search("administrators-pick-search", s.pickQuery, "@username или Telegram ID")}${error}<div class="admin-users__list">${s.busy === "search" && !s.candidates.length ? loading() : s.candidates.map((a, i) => row(a, true, i)).join("") || `<div class="admin-users__empty"><strong>Никого не нашли</strong><span>Пользователь должен сначала войти в сервис</span></div>`}</div>${s.candidates.length < s.candidateTotal ? `<button class="admin-users__more" type="button" data-action="administrators-more" ${busy}>Показать ещё</button>` : ""}</div>`;
  return `<div class="admin-users admin-access"><div class="admin-access__add-row"><button class="admin-access__add btn" type="button" data-action="administrators-add" ${busy}>${icon("plus")}Добавить администратора</button><span class="admin-list-count" aria-label="Всего администраторов">${s.total}</span></div>${search("administrators-search", s.query, "Имя, Telegram ID или роль")}${error}<div class="admin-users__result" aria-live="polite">${s.busy === "search" && !s.items.length ? loading() : s.items.length ? `<div class="admin-users__list">${s.items.map((a, i) => row(a, false, i)).join("")}</div>` : `<div class="admin-users__empty">${icon("users")}<strong>Администраторы не найдены</strong><span>Добавьте пользователя или измените поиск</span></div>`}</div>${s.items.length < s.total ? `<button class="admin-users__more" type="button" data-action="administrators-more" ${busy}>Показать ещё</button>` : ""}</div>`;
}
