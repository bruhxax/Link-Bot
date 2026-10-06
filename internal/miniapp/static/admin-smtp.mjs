export function renderSMTPSettings(s, { escapeHtml: html, escapeAttribute: attr, icon }) {
  const disabled = s.busy ? "disabled" : "";
  if (!s.draft) return `<div class="admin-ai__loading">${s.error ? `<p class="admin-ai__error" role="alert">${html(s.error)}</p><button class="admin-ai__button" type="button" data-action="admin-smtp-load">Повторить</button>` : "Загрузка настроек почты…"}</div>`;
  const d = s.draft;
  const field = (key,label,type,placeholder) => `<label class="admin-field"><span>${label}</span><input class="admin-field__control" type="${type}" data-input="admin-smtp-${key}" value="${attr(d[key] || "")}" placeholder="${placeholder}" autocomplete="${type === "password" ? "new-password" : "off"}" ${disabled}></label>`;
  return `<div class="admin-ai admin-smtp">
    <label class="admin-ai__power admin-toggle"><span>Отправка писем</span><input type="checkbox" role="switch" data-input="admin-smtp-enabled" ${d.enabled ? "checked" : ""} ${disabled}><i aria-hidden="true"></i></label>
    <section class="admin-ai__card"><div class="admin-ai__card-title"><span>01</span><h3>Подключение</h3><small>${s.checked ? "Проверено" : d.source === "env" ? "Из .env" : "SMTP"}</small></div>
      <div class="admin-smtp__server">${field("host","SMTP-сервер","text","smtp.example.com")}${field("port","Порт","number","587")}</div>
      ${field("user","Логин","text","mail@example.com")}${field("password","Пароль приложения","password",d.passwordConfigured ? "Сохранён · введите новый для замены" : "Пароль SMTP")}${field("from","Адрес отправителя","text","Link-Bot <mail@example.com>")}
      <p class="admin-ai__hint">465 — TLS, остальные порты — STARTTLS. Эти настройки используются для входа, привязки почты и email-рассылок.</p>
      <details><summary>Прокси</summary>${field("proxyUrl","HTTP(S)-прокси","password",d.proxyConfigured ? "Сохранён · введите новый для замены" : "https://user:password@proxy:port")}<label class="admin-toggle"><span>Удалить сохранённый прокси</span><input type="checkbox" data-input="admin-smtp-clearProxy" ${d.clearProxy ? "checked" : ""} ${disabled}><i aria-hidden="true"></i></label></details>
      <button class="admin-ai__button admin-ai__button--secondary" type="button" data-action="admin-smtp-check" ${disabled}>${icon("check")}<span>${s.busy === "check" ? "Проверяем…" : "Проверить подключение"}</span></button>
    </section>
    <section class="admin-ai__card"><div class="admin-ai__card-title"><span>02</span><h3>Тестовое письмо</h3></div><label class="admin-field"><span>Получатель</span><input class="admin-field__control" type="email" data-input="admin-smtp-testEmail" value="${attr(s.testEmail || "")}" placeholder="you@example.com" ${disabled}></label><button class="admin-ai__button admin-ai__button--secondary" type="button" data-action="admin-smtp-test" ${disabled}>${icon("profileLetter")}<span>${s.busy === "test" ? "Отправляем…" : "Отправить тестовое письмо"}</span></button></section>
    ${s.error ? `<p class="admin-ai__error" role="alert">${html(s.error)}</p>` : ""}
    <button class="admin-ai__button" type="button" data-action="admin-smtp-save" ${disabled}>${icon("check")}<span>${s.busy === "save" ? "Сохраняем…" : "Сохранить настройки"}</span></button>
  </div>`;
}
