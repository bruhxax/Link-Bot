export function defaultInteractive() {
  return {
    enabled: false,
    addresses: [],
    protectedText: "Вы под защитой {brand} — спасибо, что вы с нами!",
    unprotectedText: "Вы не под защитой",
    durationSeconds: 8,
  };
}
export function interactiveText(settings, protectedState, brand) {
  const defaults = defaultInteractive();
  return String(
    (protectedState ? settings.protectedText : settings.unprotectedText) ||
      (protectedState ? defaults.protectedText : defaults.unprotectedText),
  ).replaceAll("{brand}", brand || "VPN");
}
export function renderInteractiveEditor(
  settings,
  { escapeHtml: e, escapeAttribute: q, icon },
  options = {},
) {
  const cfg = { ...defaultInteractive(), ...settings },
    protectedState = options.preview !== "unprotected";
  return `<div class="interactive-editor">
 <section class="interactive-card interactive-intro"><span class="interactive-card__icon">${icon("shield")}</span><div><h3>Статус защиты на лендинге</h3><p>Плашка под меню показывает, подключён ли посетитель через вашу ноду.</p></div><label class="interactive-switch"><input type="checkbox" role="switch" aria-label="Показывать статус защиты" data-interactive-field="enabled" ${cfg.enabled ? "checked" : ""}><span></span></label></section>
 <div class="interactive-columns"><div class="interactive-settings">
 <section class="interactive-card"><div class="interactive-heading"><div><h3>Ваши IP-адреса</h3><p>По одному на строку. IPv4, IPv6 или подсеть CIDR.</p></div><span class="interactive-count" data-interactive-count>${cfg.addresses?.length || 0}</span></div>
 <label class="interactive-field"><span>Адреса нод</span><textarea rows="7" spellcheck="false" autocomplete="off" data-interactive-field="addresses" placeholder="203.0.113.10&#10;2001:db8::10">${e((cfg.addresses || []).join("\n"))}</textarea></label>
 <button type="button" class="interactive-button" data-action="interactive-import" ${options.importing ? "disabled" : ""}>${icon("server")}<span>${options.importing ? "Получаем адреса…" : "Добавить из панели"}</span></button>
 <p class="interactive-note">Импорт добавит адреса к списку. Для доменов определятся текущие IP. Если выходной IP отличается от адреса ноды, добавьте его вручную.</p>
 ${options.error ? `<p class="interactive-error" role="alert">${e(options.error)}</p>` : ""}${options.notice ? `<p class="interactive-note" role="status">${e(options.notice)}</p>` : ""}</section>
 <section class="interactive-card"><div class="interactive-heading"><div><h3>Сообщения</h3><p><code>{brand}</code> подставит название вашего сервиса.</p></div></div>
 <label class="interactive-field"><span>Через вашу ноду</span><textarea rows="3" maxlength="300" data-interactive-field="protectedText">${e(cfg.protectedText)}</textarea></label>
 <label class="interactive-field"><span>Другой IP-адрес</span><textarea rows="2" maxlength="300" data-interactive-field="unprotectedText">${e(cfg.unprotectedText)}</textarea></label></section>
 <section class="interactive-card"><div class="interactive-heading"><div><h3>Время показа</h3><p>При каждом заходе или обновлении страницы.</p></div></div>
 <label class="interactive-toggle"><span>Показывать постоянно</span><input type="checkbox" role="switch" data-interactive-field="always" ${cfg.durationSeconds === 0 ? "checked" : ""}></label>
 <label class="interactive-field" data-interactive-duration ${cfg.durationSeconds === 0 ? "hidden" : ""}><span>Скрывать через, секунд</span><input type="number" min="1" max="3600" step="1" required value="${q(cfg.durationSeconds || 8)}" data-interactive-field="durationSeconds"></label>
 <p class="interactive-note">Плашка плавно появится и исчезнет, не сдвигая содержимое страницы.</p></section></div>
 <aside class="interactive-card interactive-preview"><div class="interactive-heading"><div><h3>Предпросмотр</h3><p>Вид плашки под меню лендинга.</p></div></div>
 <div class="interactive-preview-tabs" role="group" aria-label="Состояние защиты"><button type="button" data-action="interactive-preview" data-value="protected" aria-pressed="${protectedState}">Ваш IP</button><button type="button" data-action="interactive-preview" data-value="unprotected" aria-pressed="${!protectedState}">Другой IP</button></div>
 <div class="interactive-preview-stage"><div class="interactive-preview-menu"><strong>${e(options.brand || "VPN")}</strong><span>${icon("server")}${icon("cartShopping")}${icon("profile")}</span></div><div class="interactive-preview-banner" data-protected="${protectedState}"><span class="interactive-status-dot" aria-hidden="true"></span><span data-interactive-preview-text>${e(interactiveText(cfg, protectedState, options.brand))}</span></div><div class="interactive-preview-lines" aria-hidden="true"><i></i><i></i><i></i></div></div>
 <p class="interactive-note" data-interactive-preview-duration>${cfg.durationSeconds === 0 ? "Остаётся на странице" : `Исчезнет через ${cfg.durationSeconds} сек.`}</p></aside></div></div>`;
}
