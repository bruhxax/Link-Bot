export function defaultInteractive() {
  return {
    enabled: false,
    addresses: [],
    protectedText: "Вы под защитой {brand} — спасибо, что вы с нами!",
    unprotectedText: "Вы не под защитой",
    durationSeconds: 8,
  };
}
export function renderInteractiveEditor(
  settings,
  { escapeHtml: e, escapeAttribute: q, icon },
  options = {},
) {
  const cfg = { ...defaultInteractive(), ...settings },
    open = Boolean(options.open);
  return `<div class="interactive-editor"><section class="interactive-panel" data-open="${open}">
 <button type="button" class="interactive-trigger" data-action="interactive-toggle" aria-expanded="${open}" aria-controls="interactive-settings"><span>Статус защиты</span><span class="interactive-chevron" aria-hidden="true"></span></button>
 <div class="interactive-collapse" id="interactive-settings" data-interactive-body aria-hidden="${!open}"><div class="interactive-collapse__clip"><fieldset class="interactive-fields" data-interactive-fields ${open ? "" : "disabled"}>
 <label class="interactive-toggle"><span>Включить</span><input type="checkbox" role="switch" aria-label="Включить статус защиты" data-interactive-field="enabled" ${cfg.enabled ? "checked" : ""}></label>
 <label class="interactive-field"><span>IP нод</span><textarea rows="4" spellcheck="false" autocomplete="off" data-interactive-field="addresses" placeholder="203.0.113.10&#10;2001:db8::10">${e((cfg.addresses || []).join("\n"))}</textarea></label>
 <button type="button" class="interactive-import" data-action="interactive-import" ${options.importing ? "disabled" : ""}>${icon("server")}<span>${options.importing ? "Загрузка…" : "Из панели"}</span></button>
 ${options.error ? `<p class="interactive-feedback interactive-feedback--error" role="alert">${e(options.error)}</p>` : ""}${options.notice ? `<p class="interactive-feedback" role="status">${e(options.notice)}</p>` : ""}
 <div class="interactive-divider"></div>
 <label class="interactive-field"><span>С VPN</span><textarea rows="2" maxlength="300" data-interactive-field="protectedText">${e(cfg.protectedText)}</textarea></label>
 <label class="interactive-field"><span>Без VPN</span><textarea rows="2" maxlength="300" data-interactive-field="unprotectedText">${e(cfg.unprotectedText)}</textarea></label>
 <div class="interactive-divider"></div>
 <label class="interactive-toggle"><span>Показывать всегда</span><input type="checkbox" role="switch" data-interactive-field="always" ${cfg.durationSeconds === 0 ? "checked" : ""}></label>
 <label class="interactive-field interactive-duration" data-interactive-duration ${cfg.durationSeconds === 0 ? "hidden" : ""}><span>Скрыть через, сек.</span><input type="number" min="1" max="3600" step="1" required value="${q(cfg.durationSeconds || 8)}" data-interactive-field="durationSeconds"></label>
 </fieldset></div></div></section></div>`;
}
