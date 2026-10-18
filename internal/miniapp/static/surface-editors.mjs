import { CURSOR_PRESETS, presetCursor } from "./custom-cursor.mjs";
export function previewButtonColor(value) {
  const hex = /^#[0-9a-f]{6}$/i.test(value || "") ? value : "#ba173d";
  const channels = [1, 3, 5]
    .map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
    .map((v) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722 >
    0.179
    ? "#080b10"
    : "#ffffff";
}
export const LANDING_TEXT_FIELDS = [
  ["title", "Название страницы"],
  ["heroTitle", "Заголовок главной"],
  ["heroAccent", "Вторая строка заголовка"],
  ["heroLead", "Описание на главной"],
  ["trialButton", "Кнопка пробного периода"],
  ["cabinetButton", "Кнопка кабинета"],
  ["serversButton", "Пункт «Сервера»"],
  ["serversTitle", "Заголовок серверов"],
  ["plansButton", "Пункт «Тарифы»"],
  ["plansTitle", "Заголовок тарифов"],
  ["buyButton", "Кнопка покупки"],
  ["supportButton", "Пункт «Поддержка»"],
  ["supportTitle", "Заголовок поддержки"],
  ["contactButton", "Кнопка обращения"],
];
export function ensureSurfaceSettings(appearance) {
  appearance.cursors ||= {};
  for (const key of ["landing", "cabinet", "admin"])
    appearance.cursors[key] ||= {
      mode: "system",
      url: "",
      pointerUrl: "",
      size: 32,
      hotspotX: 0,
      hotspotY: 0,
    };
  appearance.landing ||= {};
  const l = appearance.landing;
  l.colors ||= { ...appearance.colors };
  l.background ||= { mode: "solid", backgroundId: "" };
  l.pages ||= {};
  l.text ||= {};
  l.icons ||= {};
  l.buttonRadius ??= 8;
  for (const key of ["home", "servers", "plans", "support"])
    l.pages[key] ||= { mode: "inherit", backgroundId: "" };
  return l;
}
export function renderCursorEditor(
  a,
  surface,
  { escapeHtml: e, escapeAttribute: q },
) {
  ensureSurfaceSettings(a);
  const c = a.cursors[surface],
    path = `appearance.cursors.${surface}`;
  const field = (label, key, type = "text", extra = "") =>
    `<label class="admin-field surface-field"><span>${label}</span><input class="admin-field__control" type="${type}" data-setting-path="${path}.${key}" data-setting-type="${type === "number" ? "number" : "text"}" value="${q(c[key] ?? "")}" ${extra}></label>`;
  return `<div class="surface-settings"><div class="surface-tabs" role="tablist" aria-label="Где настроить курсор">${[
    ["landing", "Лендинг"],
    ["cabinet", "Кабинет"],
    ["admin", "Админка"],
  ]
    .map(
      ([id, label]) =>
        `<button type="button" role="tab" aria-selected="${surface === id}" data-action="admin-cursor-surface" data-value="${id}">${label}</button>`,
    )
    .join("")}</div>
 <div class="surface-editor-grid"><section class="admin-editor__section surface-card"><h3>Вид курсора</h3><p>Настройки сохраняются отдельно для каждой поверхности.</p>
 <div class="cursor-presets">${CURSOR_PRESETS.map(([id, label]) => `<button type="button" class="cursor-preset ${c.mode === id ? "is-active" : ""}" aria-pressed="${c.mode === id}" data-action="admin-cursor-preset" data-value="${id}">${presetCursor(id) ? `<img src="${q(presetCursor(id))}" alt="" width="32" height="32">` : '<span class="cursor-preset__symbol" aria-hidden="true">↖</span>'}<span>${label}</span></button>`).join("")}</div>
 <div class="surface-number-grid">${field("Размер, px", "size", "number", 'min="16" max="64"')}${field("Точка X", "hotspotX", "number", `min="0" max="${c.size - 1}"`)}${field("Точка Y", "hotspotY", "number", `min="0" max="${c.size - 1}"`)}</div><p>Активная точка — место клика внутри изображения. У лапки, кольца и прицела она находится в центре.</p></section>
 <section class="admin-editor__section surface-card"><h3>Свои курсоры</h3><p>PNG, SVG, JPG, WebP, GIF или CUR · до 2 МБ. Изображение будет уменьшено до выбранного размера.</p>
 ${field("Основной курсор — ссылка", "url")}
 <label class="surface-upload"><span>Загрузить основной курсор</span><input class="surface-upload__input" type="file" accept="image/*,.cur" data-input="admin-cursor-file" data-surface="${surface}" data-cursor-kind="url"></label>
 ${field("Курсор над кнопками — ссылка (необязательно)", "pointerUrl")}
 <label class="surface-upload"><span>Загрузить курсор для кнопок</span><input class="surface-upload__input" type="file" accept="image/*,.cur" data-input="admin-cursor-file" data-surface="${surface}" data-cursor-kind="pointerUrl"></label>
 <p>На сенсорных экранах курсор не отображается. Поля ввода и недоступные кнопки сохраняют понятные системные указатели.</p></section></div>
 <section class="admin-editor__section surface-card cursor-demo"><h3>Проверка курсора</h3><div data-cursor-demo="${surface}"><span>Наведите курсор сюда</span><button class="surface-button" type="button" data-action="admin-cursor-test">Обычная кнопка</button><input class="admin-field__control" placeholder="Поле ввода" aria-label="Проверка текстового курсора"></div><p>Настройки начнут действовать после сохранения.</p></section></div>`;
}
export function renderLandingEditor(
  a,
  tab,
  { escapeHtml: e, escapeAttribute: q, renderAdminColorField: color },
) {
  const l = ensureSurfaceSettings(a);
  const path = "appearance.landing";
  const modes = [
    ["solid", "Сплошной цвет"],
    ["animated", "Волны"],
    ["grid", "Движущаяся сетка"],
    ["grid2", "Сетка со свечением"],
    ["blocks", "Кубики"],
    ["morphic", "Морфинг"],
    ["twinkle", "Звёзды"],
    ["backtyan", "BackTyan"],
    ["custom", "Свой фон"],
  ];
  const select = (label, key, options, value) =>
    `<label class="admin-field surface-field"><span>${label}</span><select class="admin-field__control" data-setting-path="${path}.${key}">${options.map(([id, text]) => `<option value="${q(id)}" ${id === value ? "selected" : ""}>${e(text)}</option>`).join("")}</select></label>`;
  const bg = (title, key, value, inherit) =>
    `<section class="admin-editor__section surface-card"><h3>${title}</h3>${select("Стиль фона", `${key}.mode`, inherit ? [["inherit", "Общий фон"], ...modes] : modes, value.mode)}${value.mode === "custom" ? select("Сохранённый фон", `${key}.backgroundId`, [["", "Выберите фон"], ...(a.customBackgrounds || []).map((item) => [item.id, item.name])], value.backgroundId) : ""}</section>`;
  const tabs = [
    ["colors", "Цвета"],
    ["backgrounds", "Фоны"],
    ["text", "Тексты"],
    ["icons", "Иконки"],
  ];
  let body = "";
  if (tab === "colors")
    body = `<div class="surface-editor-grid"><section class="admin-editor__section surface-card"><h3>Палитра лендинга</h3><p>Цвета кабинета и админки настраиваются отдельно.</p><div class="admin-color-grid">${[
      ["background", "Основной фон"],
      ["surface", "Карточки"],
      ["surfaceStrong", "Плотные поверхности"],
      ["text", "Текст"],
      ["muted", "Подписи"],
      ["border", "Границы"],
      ["accent", "Кнопки и акцент"],
      ["success", "Сервер онлайн"],
      ["danger", "Сервер недоступен"],
    ]
      .map(([key, label]) => color(label, `${path}.colors.${key}`))
      .join(
        "",
      )}</div><label class="admin-field surface-field"><span>Скругление кнопок, px</span><input class="admin-field__control" type="number" min="0" max="24" data-setting-type="number" data-setting-path="${path}.buttonRadius" value="${l.buttonRadius}"></label></section><section class="admin-editor__section surface-card"><h3>Предпросмотр палитры</h3><div class="landing-editor-preview" data-landing-preview style="--preview-bg:${q(l.colors.background)};--preview-surface:${q(l.colors.surface)};--preview-text:${q(l.colors.text)};--preview-muted:${q(l.colors.muted)};--preview-accent:${q(l.colors.accent)};--preview-button-text:${previewButtonColor(l.colors.accent)};--preview-radius:${l.buttonRadius}px"><small>ГЛАВНАЯ</small><strong>${e(l.text.heroTitle || "Интернет без ограничений")}</strong><p>${e(l.text.heroLead || "Стабильное соединение для работы и общения.")}</p><button class="surface-button" type="button" data-action="admin-cursor-test">${e(l.text.trialButton || "Попробовать бесплатно")}</button><div><b>Тариф</b><span>Сервера · Поддержка</span></div></div></section></div>`;
  if (tab === "backgrounds")
    body = `<section class="admin-editor__section surface-card"><h3>Фон лендинга</h3><label class="admin-toggle"><span>Отдельный фон для каждого раздела</span><input type="checkbox" data-setting-type="boolean" data-setting-path="${path}.perPage" ${l.perPage ? "checked" : ""}><i aria-hidden="true"></i></label><p>Лендинг состоит из четырёх разделов: главная, сервера, тарифы и поддержка. Можно задать один фон на всю страницу или отдельный фон каждому разделу.</p></section><div class="surface-editor-grid">${bg("Общий фон", "background", l.background, false)}${
      l.perPage
        ? [
            ["home", "Главная"],
            ["servers", "Сервера"],
            ["plans", "Тарифы"],
            ["support", "Поддержка"],
          ]
            .map(([id, label]) => bg(label, `pages.${id}`, l.pages[id], true))
            .join("")
        : ""
    }</div><section class="admin-editor__section surface-card"><h3>Добавить свой фон</h3><p>Используйте готовые фоны из «Оформления» или загрузите изображение, GIF либо видео до 50 МБ.</p><label class="surface-upload"><span>Загрузить фон лендинга</span><input class="surface-upload__input" type="file" accept="image/*,video/*" data-input="admin-landing-background-file"></label><div class="surface-inline"><input class="admin-field__control" aria-label="Прямая ссылка на фон" placeholder="https://…" data-input="admin-landing-background-url"><button class="surface-button" type="button" data-action="admin-landing-background-import">Добавить по ссылке</button></div></section>`;
  if (tab === "text") {
    const groups = [
      [
        "Главная",
        [
          "title",
          "heroTitle",
          "heroAccent",
          "heroLead",
          "trialButton",
          "cabinetButton",
        ],
      ],
      ["Серверы", ["serversButton", "serversTitle"]],
      ["Тарифы", ["plansButton", "plansTitle", "buyButton"]],
      [
        "Поддержка",
        ["supportButton", "supportTitle", "contactButton"],
      ],
    ];
    const labels = Object.fromEntries(LANDING_TEXT_FIELDS);
    body = `<div class="surface-editor-grid">${groups.map(([title, keys]) => `<section class="admin-editor__section surface-card"><h3>${title}</h3>${keys.map((key) => `<label class="admin-field surface-field"><span>${labels[key]}</span>${key.endsWith("Lead") ? `<textarea class="admin-field__control admin-field__textarea" rows="3" maxlength="1000" data-setting-path="${path}.text.${key}">${e(l.text[key] || "")}</textarea>` : `<input class="admin-field__control" maxlength="1000" data-setting-path="${path}.text.${key}" value="${q(l.text[key] || "")}">`}</label>`).join("")}</section>`).join("")}</div>`;
  }
  if (tab === "icons")
    body = `<div class="surface-editor-grid">${[
      ["servers", "Сервера"],
      ["plans", "Тарифы"],
      ["support", "Поддержка"],
      ["cabinet", "Кабинет"],
    ]
      .map(
        ([key, label]) =>
          `<section class="admin-editor__section surface-card"><h3>${label}</h3>${l.icons[key] ? `<img class="landing-icon-preview" src="${q(l.icons[key])}" alt="${label}" width="32" height="32">` : ""}<label class="admin-field surface-field"><span>Ссылка на иконку</span><input class="admin-field__control" data-setting-path="${path}.icons.${key}" value="${q(l.icons[key] || "")}" placeholder="Пусто — стандартная иконка"></label><label class="surface-upload"><span>Загрузить иконку</span><input class="surface-upload__input" type="file" accept="image/*" data-input="admin-landing-icon-file" data-icon-key="${key}"></label></section>`,
      )
      .join("")}</div>`;
  return `<div class="surface-settings"><div class="surface-tabs" role="tablist" aria-label="Настройки лендинга">${tabs.map(([id, label]) => `<button type="button" role="tab" aria-selected="${id === tab}" data-action="admin-landing-tab" data-value="${id}">${label}</button>`).join("")}</div>${body}</div>`;
}

export function renderAudienceEditor(
  a,
  options,
  count,
  running,
  channel,
  { escapeHtml: e, escapeAttribute: q },
) {
  const categories = [
    ["all", "Все пользователи"],
    ["active", "Есть активная подписка"],
    ["none", "Нет активной подписки"],
    ["expired", "Подписка истекла"],
    ["trial", "Активный триал"],
    ["trial_used", "Использовали триал"],
    ["paid", "Покупали подписку"],
  ];
  const select = (key, label, values) =>
    `<label class="admin-field surface-field"><span>${label}</span><select class="admin-field__control" data-audience-field="${key}" data-channel="${channel}" ${running ? "disabled" : ""}>${values.map(([id, text]) => `<option value="${q(id)}" ${a[key] === id ? "selected" : ""}>${e(text)}</option>`).join("")}</select></label>`;
  const squads = (key, title, items) =>
    `<div class="audience-squads"><h4>${title}</h4>${items?.length ? items.map((s) => `<label><input type="checkbox" data-audience-field="${key}" data-channel="${channel}" value="${q(s.uuid)}" ${(a[key] || []).includes(s.uuid) ? "checked" : ""} ${running ? "disabled" : ""}><span>${e(s.name)}</span></label>`).join("") : "<p>Сквады не загружены. Нажмите «Обновить сквады».</p>"}</div>`;
  return `<section class="admin-editor__section surface-card surface-settings audience-editor"><div class="surface-inline"><div><h3>Кому отправлять</h3></div><strong class="audience-count" role="status">${count === null || count === undefined ? "Не подсчитано" : `${count} получателей`}</strong></div><div class="admin-editor__grid">${select("category", "Категория", categories)}${select(
    "language",
    "Язык",
    [
      ["", "Любой"],
      ["ru", "Русский"],
      ["en", "English"],
      ["fa", "فارسی"],
    ],
  )}<label class="admin-field surface-field"><span>Тег в панели</span><input class="admin-field__control" data-audience-field="tag" data-channel="${channel}" value="${q(a.tag || "")}" placeholder="Любой тег" ${running ? "disabled" : ""}></label></div><details><summary>Отбор по сквадам <small>${(a.internalSquads?.length || 0) + (a.externalSquads?.length || 0)} выбрано</small></summary><div class="surface-editor-grid">${squads("internalSquads", "Внутренние сквады", options?.internal)}${squads("externalSquads", "Внешние сквады", options?.external)}</div><button class="surface-button" type="button" data-action="admin-audience-options" ${running ? "disabled" : ""}>Обновить сквады</button></details><label class="admin-toggle admin-feature-toggle"><span class="admin-feature-toggle__copy"><strong>Игнорировать отписку от рассылки</strong><small>Отправить эту рассылку также отписавшимся пользователям. Их настройки останутся прежними.</small></span><input type="checkbox" role="switch" data-audience-field="ignoreUnsubscribe" data-channel="${channel}" ${a.ignoreUnsubscribe ? "checked" : ""} ${running ? "disabled" : ""}><i aria-hidden="true"></i></label><div class="surface-inline"><button class="surface-button" type="button" data-action="admin-audience-check" data-value="${channel}" ${running ? "disabled" : ""}>Сохранить аудиторию и посчитать</button></div></section>`;
}
