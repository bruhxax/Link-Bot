export function renderSMTPSettings(s, { escapeHtml: html, escapeAttribute: attr, icon }) {
  const disabled = s.busy ? "disabled" : "";
  if (!s.draft) return `<div class="admin-mail__hint">${s.error ? `<p class="admin-ai__error" role="alert">${html(s.error)}</p><button class="admin-mail__button" type="button" data-action="admin-smtp-load">Повторить</button>` : "Загрузка подключения…"}</div>`;
  const d = s.draft;
  const field = (key,label,type,placeholder) => `<label class="admin-email-broadcast__field"><span>${label}</span><input type="${type}" data-input="admin-smtp-${key}" value="${attr(d[key] || "")}" placeholder="${placeholder}" autocomplete="${type === "password" ? "new-password" : "off"}" ${disabled}></label>`;
  return `<div class="admin-smtp admin-mail__form">
    <label class="admin-mail__power admin-toggle"><span>Отправка писем</span><input type="checkbox" role="switch" data-input="admin-smtp-enabled" ${d.enabled ? "checked" : ""} ${disabled}><i aria-hidden="true"></i></label>
      <div class="admin-smtp__server">${field("host","SMTP-сервер","text","smtp.example.com")}${field("port","Порт","number","587")}</div>
      ${field("user","Логин","text","mail@example.com")}${field("password","Пароль приложения","password",d.passwordConfigured ? "Сохранён · введите новый для замены" : "Пароль SMTP")}${field("from","Адрес отправителя","text","Link-Bot <mail@example.com>")}
      <details><summary>Прокси</summary>${field("proxyUrl","HTTP(S)-прокси","password",d.proxyConfigured ? "Сохранён · введите новый для замены" : "https://user:password@proxy:port")}<label class="admin-toggle"><span>Удалить сохранённый прокси</span><input type="checkbox" data-input="admin-smtp-clearProxy" ${d.clearProxy ? "checked" : ""} ${disabled}><i aria-hidden="true"></i></label></details>
    ${s.checked ? `<p class="admin-mail__hint">Подключение проверено</p>` : ""}
    ${s.error ? `<p class="admin-ai__error" role="alert">${html(s.error)}</p>` : ""}
    <div class="admin-mail__actions"><button class="admin-mail__button" type="button" data-action="admin-smtp-check" ${disabled}>${icon("check")}<span>${s.busy === "check" ? "Проверяем…" : "Проверить"}</span></button><button class="admin-mail__button" type="button" data-action="admin-smtp-save" ${disabled}>${icon("check")}<span>${s.busy === "save" ? "Сохраняем…" : "Сохранить"}</span></button></div>
  </div>`;
}
