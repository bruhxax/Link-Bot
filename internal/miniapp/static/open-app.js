import {
  SETUP_PLATFORMS,
  buildSetupClientURL,
  getLocalizedSetupValue,
  getSetupApp,
  getSetupPlatform,
} from "./setup-apps.js";

const root = document.getElementById("opener");
const toast = document.getElementById("opener-toast");

const copybook = {
  ru: {
    eyebrow: "ПОДПИСКА ГОТОВА",
    title: (name) => `Открыть в ${name}`,
    hint: (name) => `Нажмите кнопку ниже — ${name} откроется и добавит подписку автоматически.`,
    open: (name) => `Открыть в ${name}`,
    fallback: "Если приложение не открылось",
    fallbackHint: "Установите его, вернитесь на эту страницу и нажмите кнопку ещё раз.",
    copy: "Скопировать ссылку",
    copied: "Ссылка скопирована",
    back: "Вернуться в Mini App",
    invalidTitle: "Ссылка недействительна",
    invalidHint: "Вернитесь в Mini App и откройте подключение ещё раз.",
  },
  en: {
    eyebrow: "SUBSCRIPTION IS READY",
    title: (name) => `Open in ${name}`,
    hint: (name) => `Tap the button below — ${name} will open and add the subscription automatically.`,
    open: (name) => `Open in ${name}`,
    fallback: "If the app did not open",
    fallbackHint: "Install it, return to this page and tap the button again.",
    copy: "Copy subscription link",
    copied: "Link copied",
    back: "Back to Mini App",
    invalidTitle: "Invalid link",
    invalidHint: "Return to Mini App and open the setup flow again.",
  },
  fa: {
    eyebrow: "اشتراک آماده است",
    title: (name) => `باز کردن در ${name}`,
    hint: (name) => `دکمه زیر را بزنید؛ ${name} باز می‌شود و اشتراک را خودکار اضافه می‌کند.`,
    open: (name) => `باز کردن در ${name}`,
    fallback: "اگر برنامه باز نشد",
    fallbackHint: "آن را نصب کنید، به این صفحه برگردید و دوباره دکمه را بزنید.",
    copy: "کپی لینک اشتراک",
    copied: "لینک کپی شد",
    back: "بازگشت به Mini App",
    invalidTitle: "لینک نامعتبر است",
    invalidHint: "به Mini App برگردید و اتصال را دوباره باز کنید.",
  },
};

function escapeHTML(value) {
  return String(value || "").replace(/[&<>'"]/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;" })[char]);
}

function validColor(value, fallback) {
  return /^#[0-9a-f]{3,8}$/i.test(String(value || "")) ? value : fallback;
}

function applyTheme(params) {
  const values = {
    "--accent": validColor(params.get("accent"), "#ba173d"),
		"--accent-contrast": validColor(params.get("contrast"), "#ffffff"),
    "--icon": validColor(params.get("icon"), "#f3f3f3"),
    "--bg": validColor(params.get("background"), "#000000"),
    "--surface": validColor(params.get("surface"), "#08090c"),
    "--text": validColor(params.get("text"), "#f3f3f3"),
  };
  Object.entries(values).forEach(([property, value]) => document.documentElement.style.setProperty(property, value));
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", values["--bg"]);
}

function showToast(message) {
  toast.textContent = message;
  toast.classList.add("is-visible");
  window.setTimeout(() => toast.classList.remove("is-visible"), 1800);
}

async function copyText(value, message) {
  try {
    await navigator.clipboard.writeText(value);
  } catch {
    const field = document.createElement("textarea");
    field.value = value;
    field.setAttribute("readonly", "");
    field.style.position = "fixed";
    field.style.opacity = "0";
    document.body.appendChild(field);
    field.select();
    document.execCommand("copy");
    field.remove();
  }
  showToast(message);
}

function renderInvalid(copy) {
  root.innerHTML = `<section class="opener-card opener-card--error"><span class="opener-mark" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none"><path d="M12 8v5m0 4h.01M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg></span><p class="opener-eyebrow">${escapeHTML(copy.eyebrow)}</p><h1>${escapeHTML(copy.invalidTitle)}</h1><p class="opener-hint">${escapeHTML(copy.invalidHint)}</p><a class="opener-secondary" href="/mini-app/">${escapeHTML(copy.back)}</a></section>`;
}

function boot() {
  const params = new URLSearchParams(window.location.hash.replace(/^#/, ""));
  const locale = ["ru", "en", "fa"].includes(params.get("lang")) ? params.get("lang") : "ru";
  const copy = copybook[locale];
  document.documentElement.lang = locale;
  document.documentElement.dir = locale === "fa" ? "rtl" : "ltr";
  applyTheme(params);

  const platformID = params.get("platform") || "";
  const appID = params.get("app") || "";
  const subscription = params.get("subscription") || "";
  const platformExists = SETUP_PLATFORMS.some((platform) => platform.id === platformID);
  const appExists = platformExists && getSetupPlatform(platformID).apps.some((app) => app.id === appID);
  let clientURL = "";
  try {
    if (!appExists) throw new Error("Unknown app");
    clientURL = buildSetupClientURL(platformID, appID, subscription);
  } catch {
    window.history.replaceState(null, "", window.location.pathname);
    renderInvalid(copy);
    return;
  }

  const app = getSetupApp(platformID, appID);
  const platform = getSetupPlatform(platformID);
  window.history.replaceState(null, "", window.location.pathname);
  document.title = copy.title(app.name);
  const installLinks = (app.links || []).slice(0, 4).map((item) => `<a href="${escapeHTML(item.url)}" target="_blank" rel="noopener noreferrer"><svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M14 5h5v5M10 14 19 5M19 13v5a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>${escapeHTML(getLocalizedSetupValue(item.label, locale))}</a>`).join("");
  root.innerHTML = `<section class="opener-card"><span class="opener-mark" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none"><path d="M7 17 17 7M9 7h8v8" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/></svg></span><p class="opener-eyebrow">${escapeHTML(copy.eyebrow)}</p><h1>${escapeHTML(copy.title(app.name))}</h1><p class="opener-platform">${escapeHTML(getLocalizedSetupValue(platform.name, locale))}</p><p class="opener-hint">${escapeHTML(copy.hint(app.name))}</p><button class="opener-primary" id="open-client" type="button"><svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M12 5v14M5 12h14" stroke="currentColor" stroke-width="1.9" stroke-linecap="round"/></svg>${escapeHTML(copy.open(app.name))}</button><div class="opener-fallback"><h2>${escapeHTML(copy.fallback)}</h2><p>${escapeHTML(copy.fallbackHint)}</p><div class="opener-install-links">${installLinks}</div></div><button class="opener-copy" id="copy-subscription" type="button"><svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><rect x="8" y="8" width="11" height="11" rx="2.5" stroke="currentColor" stroke-width="1.8"/><path d="M6 15H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v1" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>${escapeHTML(copy.copy)}</button><a class="opener-back" href="/mini-app/">${escapeHTML(copy.back)}</a></section>`;
  document.getElementById("open-client")?.addEventListener("click", () => { window.location.href = clientURL; });
  document.getElementById("copy-subscription")?.addEventListener("click", () => copyText(subscription, copy.copied));
}

boot();
