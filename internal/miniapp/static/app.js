const app = document.getElementById("app");
const toast = document.getElementById("toast");
const tg = window.Telegram?.WebApp;
const telegramBotUsername = document.querySelector('meta[name="telegram-bot-username"]')?.content?.trim() || "";
const telegramBotID = document.querySelector('meta[name="telegram-bot-id"]')?.content?.trim() || "";
const googleMetaClientID = document.querySelector('meta[name="google-client-id"]')?.content?.trim() || "";
const urlParams = new URLSearchParams(window.location.search);
const previewMode = (() => {
  const enabled = urlParams.get("preview") === "1";
  if (!enabled) return false;
  const host = String(window.location.hostname || "").toLowerCase();
  return host === "localhost" || host === "127.0.0.1" || host === "::1" || window.location.protocol === "file:";
})();
const previewAdminMode = previewMode && urlParams.get("admin") === "1";
const installGuideMode = urlParams.get("install") === "desktop";
const themeMeta = document.querySelector('meta[name="theme-color"]');
const reducedMotionMedia = window.matchMedia?.("(prefers-reduced-motion: reduce)");

configureBackgroundPerformance();
preventMiniAppZoom();
registerPWAServiceWorker();

const STORAGE_KEYS = {
  page: "link-bot-page",
  theme: "link-bot-theme",
  payMethod: "link-bot-pay-method",
  telegramLogin: "link-bot-telegram-login",
  googleLogin: "link-bot-google-login",
};

consumeTelegramLoginRedirect();

let telegramLoginScriptPromise = null;
let googleLoginScriptPromise = null;
let googleLoginInitializedClientID = "";
let googleAuthMode = "login";
let googleLoginPendingMode = "";
let googleLinkRefreshTimer = null;
let dashboardHydrationTimer = null;

function preventMiniAppZoom() {
  let lastTouchEndAt = 0;

  const stopGesture = (event) => {
    event.preventDefault();
  };

  document.addEventListener("gesturestart", stopGesture, { passive: false });
  document.addEventListener("gesturechange", stopGesture, { passive: false });
  document.addEventListener("gestureend", stopGesture, { passive: false });

  document.addEventListener("touchmove", (event) => {
    if ((event.touches && event.touches.length > 1) || Number(event.scale || 1) !== 1) {
      event.preventDefault();
    }
  }, { passive: false });

  document.addEventListener("touchend", (event) => {
    const now = Date.now();
    if (now - lastTouchEndAt < 320) {
      event.preventDefault();
    }
    lastTouchEndAt = now;
  }, { passive: false });
}

function registerPWAServiceWorker() {
  if (!("serviceWorker" in navigator) || window.location.protocol !== "https:") return;
  navigator.serviceWorker.register("/mini-app/sw.js", { scope: "/mini-app/" }).catch(() => {});
}

function configureBackgroundPerformance() {
  const connection = navigator.connection || navigator.mozConnection || navigator.webkitConnection;
  const reduced = Boolean(reducedMotionMedia?.matches || connection?.saveData);
  const lowPower = (Number(navigator.deviceMemory) > 0 && Number(navigator.deviceMemory) <= 4)
    || (Number(navigator.hardwareConcurrency) > 0 && Number(navigator.hardwareConcurrency) <= 4);
  document.documentElement.dataset.performance = reduced ? "reduced" : (lowPower ? "low" : "normal");
  document.documentElement.dataset.pageActive = document.hidden ? "false" : "true";
}

function scheduleDashboardHydration() {
  if (!hasAuth()) return;
  if (dashboardHydrationTimer) window.clearTimeout(dashboardHydrationTimer);
  dashboardHydrationTimer = window.setTimeout(() => {
    dashboardHydrationTimer = null;
    refreshDashboard({ silent: true }).catch(() => {});
  }, 150);
}

window.onTelegramAuth = (user) => {
  const loginData = serializeTelegramLoginAuth(user);
  if (!loginData) {
    showToast(browserAuthCopy().loginFailed, "danger");
    return;
  }

  writeSessionSetting(STORAGE_KEYS.telegramLogin, loginData);
  clearGoogleAuth();
  state.loading = true;
  state.error = "";
  state.subscriptionGate = null;
  render();
  refreshDashboard({ initial: true }).catch((error) => {
    clearBrowserTelegramAuth();
    showToast(error?.message || browserAuthCopy().loginFailed, "danger");
  });
};

function serializeTelegramLoginAuth(user) {
  const required = ["id", "auth_date", "hash"];
  if (!user || required.some((key) => user[key] === undefined || user[key] === null || user[key] === "")) {
    return "";
  }

  const params = new URLSearchParams();
  ["id", "first_name", "last_name", "username", "photo_url", "auth_date", "hash"].forEach((key) => {
    const value = user[key];
    if (value !== undefined && value !== null && String(value) !== "") params.set(key, String(value));
  });
  return params.toString();
}

function readBrowserTelegramAuth() {
  return readSessionSetting(STORAGE_KEYS.telegramLogin, "");
}

function clearBrowserTelegramAuth() {
  writeSessionSetting(STORAGE_KEYS.telegramLogin, "");
}

function hasTelegramAuth() {
  return Boolean(tg?.initData || readBrowserTelegramAuth());
}

function readGoogleAuth() {
  return readSessionSetting(STORAGE_KEYS.googleLogin, "");
}

function clearGoogleAuth() {
  writeSessionSetting(STORAGE_KEYS.googleLogin, "");
}

function hasAuth() {
  return Boolean(hasTelegramAuth() || readGoogleAuth());
}

function isInstallGuideMode() {
  return installGuideMode;
}

function getWebVersionURL() {
  if (/^https?:\/\//i.test(window.location.origin || "")) {
    return `${window.location.origin}/mini-app/`;
  }

  const configured = String(state.data?.meta?.miniAppUrl || "").trim();
  if (/^https?:\/\//i.test(configured)) {
    try {
      const url = new URL(configured);
      url.pathname = "/mini-app/";
      url.search = "";
      url.hash = "";
      return url.toString();
    } catch {
      return configured;
    }
  }
  return `${window.location.origin}/mini-app/`;
}

function getInstallGuideURL() {
  const url = new URL(getWebVersionURL());
  url.searchParams.set("install", "desktop");
  return url.toString();
}

function getDefaultInstallPlatform() {
  const ua = String(navigator.userAgent || "").toLowerCase();
  if (/iphone|ipad|ipod/.test(ua)) return "ios";
  return "android";
}

function webVersionLabel() {
  return state.locale === "en" ? "Open web version" : "\u041e\u0442\u043a\u0440\u044b\u0442\u044c \u0432\u0435\u0431-\u0432\u0435\u0440\u0441\u0438\u044e";
}

function webVersionHint() {
  return state.locale === "en" ? "Browser dashboard" : "\u0411\u0440\u0430\u0443\u0437\u0435\u0440\u043d\u0430\u044f \u0432\u0435\u0440\u0441\u0438\u044f \u043a\u0430\u0431\u0438\u043d\u0435\u0442\u0430";
}

function addToHomeLabel() {
  return state.locale === "en" ? "Add to home screen" : "\u0414\u043e\u0431\u0430\u0432\u0438\u0442\u044c \u043d\u0430 \u0440\u0430\u0431\u043e\u0447\u0438\u0439 \u0441\u0442\u043e\u043b";
}

function addToHomeHint() {
  return state.locale === "en" ? "iOS and Android install guide" : "\u0418\u043d\u0441\u0442\u0440\u0443\u043a\u0446\u0438\u044f \u0434\u043b\u044f iOS \u0438 Android";
}

function mediaLabel() {
  return state.locale === "en" ? "Media" : "\u041c\u0435\u0434\u0438\u0430";
}

function mediaHint() {
  return state.locale === "en" ? "Videos and materials" : "\u0412\u0438\u0434\u0435\u043e \u0438 \u043c\u0430\u0442\u0435\u0440\u0438\u0430\u043b\u044b";
}

function mediaComingSoonLabel() {
  return state.locale === "en" ? "Coming soon" : "\u0411\u0443\u0434\u0435\u0442 \u0434\u043e\u0441\u0442\u0443\u043f\u043d\u043e \u0432 \u0441\u043a\u043e\u0440\u043e\u043c \u0432\u0440\u0435\u043c\u0435\u043d\u0438";
}

function loginMethodsLabel() {
  return state.locale === "en" ? "Sign-in method" : "\u0421\u043f\u043e\u0441\u043e\u0431 \u0432\u0445\u043e\u0434\u0430";
}

function loginMethodsHint() {
  return state.locale === "en" ? "Telegram and Gmail" : "Telegram \u0438 Gmail";
}

function gmailLabel() {
  return state.locale === "en" ? "Gmail" : "Gmail";
}

function telegramLabel() {
  return "Telegram";
}

function authLinkedLabel() {
  return state.locale === "en" ? "Linked" : "\u041f\u0440\u0438\u0432\u044f\u0437\u0430\u043d";
}

function authNotLinkedLabel() {
  return state.locale === "en" ? "Not linked" : "\u041d\u0435 \u043f\u0440\u0438\u0432\u044f\u0437\u0430\u043d";
}

function gmailLinkHint() {
  return state.locale === "en" ? "Use the same account from the browser" : "\u0412\u0445\u043e\u0434 \u0432 \u0442\u043e\u0442 \u0436\u0435 \u0430\u043a\u043a\u0430\u0443\u043d\u0442 \u0447\u0435\u0440\u0435\u0437 \u0431\u0440\u0430\u0443\u0437\u0435\u0440";
}

function openWebVersion() {
  openExternal(getWebVersionURL());
}

function openInstallGuide() {
  openExternal(getInstallGuideURL());
}

function consumeTelegramLoginRedirect() {
  if (tg?.initData) return;

  const loginData = readTelegramLoginRedirectData();
  if (!loginData) return;

  writeSessionSetting(STORAGE_KEYS.telegramLogin, loginData);
  clearGoogleAuth();

  try {
    const cleanURL = new URL(window.location.href);
    ["telegram_login", "id", "first_name", "last_name", "username", "photo_url", "auth_date", "hash"].forEach((key) => {
      cleanURL.searchParams.delete(key);
    });
    window.history.replaceState(null, document.title, `${cleanURL.pathname}${cleanURL.search}${cleanURL.hash}`);
  } catch {
    // Session storage already has the signed Telegram login payload.
  }
}

function readTelegramLoginRedirectData() {
  const params = new URLSearchParams(window.location.search);
  if (!params.has("id") || !params.has("auth_date") || !params.has("hash")) return "";

  const user = {};
  ["id", "first_name", "last_name", "username", "photo_url", "auth_date", "hash"].forEach((key) => {
    const value = params.get(key);
    if (value !== null && value !== "") user[key] = value;
  });
  return serializeTelegramLoginAuth(user);
}

function telegramLoginButtonLabel() {
  return /^en\b/i.test(navigator.language || "") ? "Sign in with Telegram" : "\u0412\u043e\u0439\u0442\u0438 \u0447\u0435\u0440\u0435\u0437 Telegram";
}

function gmailLoginButtonLabel() {
  return /^en\b/i.test(navigator.language || "") ? "Sign in with Gmail" : "\u0412\u043e\u0439\u0442\u0438 \u0447\u0435\u0440\u0435\u0437 Gmail";
}

function gmailLinkButtonLabel() {
  return state.locale === "en" ? "Link Gmail" : "\u041f\u0440\u0438\u0432\u044f\u0437\u0430\u0442\u044c Gmail";
}

function loadTelegramLoginScript() {
  if (typeof window.Telegram?.Login?.auth === "function") return Promise.resolve();
  if (telegramLoginScriptPromise) return telegramLoginScriptPromise;

  telegramLoginScriptPromise = new Promise((resolve, reject) => {
    const existing = document.querySelector('script[data-link-bot-telegram-login="1"]');
    if (existing) {
      existing.addEventListener("load", resolve, { once: true });
      existing.addEventListener("error", reject, { once: true });
      return;
    }

    const script = document.createElement("script");
    script.async = true;
    script.src = "https://telegram.org/js/telegram-widget.js?22";
    script.dataset.linkBotTelegramLogin = "1";
    script.onload = () => resolve();
    script.onerror = () => reject(new Error("telegram login script failed"));
    document.head.appendChild(script);
  });

  return telegramLoginScriptPromise;
}

async function startTelegramBrowserLogin(trigger) {
  const copy = browserAuthCopy();
  if (!telegramBotID) {
    showToast(copy.loginUnavailable, "danger");
    return;
  }

  if (trigger) {
    trigger.disabled = true;
    trigger.classList.add("is-loading");
  }

  try {
    await loadTelegramLoginScript();
    const auth = window.Telegram?.Login?.auth;
    if (typeof auth !== "function") throw new Error("telegram login auth unavailable");

    auth({
      bot_id: telegramBotID,
      request_access: "write",
      lang: /^ru\b/i.test(navigator.language || "") ? "ru" : "en",
    }, (user) => {
      if (user) {
        window.onTelegramAuth(user);
        return;
      }
      showToast(copy.loginFailed, "danger");
    });
  } catch {
    showToast(copy.loginUnavailable, "danger");
  } finally {
    window.setTimeout(() => {
      if (!trigger) return;
      trigger.disabled = false;
      trigger.classList.remove("is-loading");
    }, 800);
  }
}

function browserAuthCopy() {
  const isEnglish = /^en\b/i.test(navigator.language || "");
  if (isEnglish) {
    return {
      title: "Sign in to Link-Bot",
      text: "Use Telegram or Gmail to open your browser dashboard with your subscription, payments and support history.",
      openBot: "Open Telegram bot",
      loginFailed: "Telegram authorization failed",
      loginUnavailable: "Telegram login is temporarily unavailable",
    };
  }

  return {
    title: "Войти в Link-Bot",
    text: "Войдите через Telegram или Gmail, чтобы открыть браузерную версию кабинета с подпиской, оплатами и поддержкой.",
    openBot: "Открыть Telegram-бота",
    loginFailed: "Не удалось авторизоваться через Telegram",
    loginUnavailable: "Вход через Telegram временно недоступен",
  };
}

function mountTelegramLoginWidget() {
  const container = document.getElementById("telegram-login-widget");
  if (!container || container.dataset.mounted === "1") return;

  const copy = browserAuthCopy();
  if (!telegramBotUsername || !telegramBotID) {
    container.textContent = copy.loginUnavailable;
    return;
  }

  container.dataset.mounted = "1";
  container.textContent = "";
  loadTelegramLoginScript().catch(() => {
    container.textContent = copy.loginUnavailable;
  });
}

function getGoogleClientID() {
	if (!featureEnabled("google")) return "";
  return String(state.data?.meta?.googleClientId || googleMetaClientID || "").trim();
}

function googleAuthCopy() {
  if (state.locale === "en" || /^en\b/i.test(navigator.language || "")) {
    return {
      loginUnavailable: "Gmail login is not configured yet",
      loginFailed: "Gmail authorization failed",
      linked: "Gmail linked",
      linkFailed: "Could not link Gmail",
    };
  }
  return {
    loginUnavailable: "\u0412\u0445\u043e\u0434 \u0447\u0435\u0440\u0435\u0437 Gmail \u043f\u043e\u043a\u0430 \u043d\u0435 \u043d\u0430\u0441\u0442\u0440\u043e\u0435\u043d",
    loginFailed: "\u041d\u0435 \u0443\u0434\u0430\u043b\u043e\u0441\u044c \u0432\u043e\u0439\u0442\u0438 \u0447\u0435\u0440\u0435\u0437 Gmail",
    linked: "Gmail \u043f\u0440\u0438\u0432\u044f\u0437\u0430\u043d",
    linkFailed: "\u041d\u0435 \u0443\u0434\u0430\u043b\u043e\u0441\u044c \u043f\u0440\u0438\u0432\u044f\u0437\u0430\u0442\u044c Gmail",
  };
}

function loadGoogleLoginScript() {
  if (window.google?.accounts?.id) return Promise.resolve();
  if (googleLoginScriptPromise) return googleLoginScriptPromise;

  googleLoginScriptPromise = new Promise((resolve, reject) => {
    const existing = document.querySelector('script[data-link-bot-google-login="1"]');
    if (existing) {
      existing.addEventListener("load", resolve, { once: true });
      existing.addEventListener("error", reject, { once: true });
      return;
    }

    const script = document.createElement("script");
    script.async = true;
    script.src = "https://accounts.google.com/gsi/client";
    script.dataset.linkBotGoogleLogin = "1";
    script.onload = () => resolve();
    script.onerror = () => reject(new Error("google login script failed"));
    document.head.appendChild(script);
  });

  return googleLoginScriptPromise;
}

async function ensureGoogleLoginReady() {
  const clientID = getGoogleClientID();
  if (!clientID) throw new Error(googleAuthCopy().loginUnavailable);
  await loadGoogleLoginScript();
  if (!window.google?.accounts?.id) throw new Error(googleAuthCopy().loginUnavailable);
  if (googleLoginInitializedClientID === clientID) return;

  window.google.accounts.id.initialize({
    client_id: clientID,
    callback: handleGoogleCredential,
    ux_mode: "popup",
    context: "signin",
  });
  googleLoginInitializedClientID = clientID;
}

function isElementVisible(element) {
  if (!element) return false;
  const rect = element.getBoundingClientRect();
  return rect.width > 0 && rect.height > 0;
}

function resolveGoogleAuthMode(mode) {
  return mode === "link" ? "link" : "login";
}

function googleWidgetWidth(container) {
  const rect = container.getBoundingClientRect();
  return Math.max(220, Math.min(400, Math.round(rect.width || 320)));
}

function prepareGoogleWidgetShell(container, mode) {
  const shell = container.closest(".google-button-shell");
  const setMode = () => {
    googleAuthMode = mode;
  };
  container.dataset.googleMode = mode;
  shell?.setAttribute("data-google-mode", mode);
  shell?.addEventListener("pointerenter", setMode, { passive: true });
  shell?.addEventListener("pointerdown", setMode, { capture: true, passive: true });
  shell?.addEventListener("touchstart", setMode, { capture: true, passive: true });
  shell?.addEventListener("focusin", setMode);
}

function mountGoogleLoginWidgets() {
  const clientID = getGoogleClientID();
  document.querySelectorAll(".google-login-widget[data-google-mode]").forEach((container) => {
    const shell = container.closest(".google-button-shell");
    shell?.removeAttribute("data-google-ready");
    if (!clientID || !isElementVisible(container)) return;

    const mode = resolveGoogleAuthMode(container.dataset.googleMode);
    const mountedKey = `${clientID}:${mode}:${googleWidgetWidth(container)}`;
    if (container.dataset.mounted === mountedKey) {
      shell?.setAttribute("data-google-ready", "1");
      return;
    }

    container.dataset.mounted = mountedKey;
    container.textContent = "";
    prepareGoogleWidgetShell(container, mode);

    ensureGoogleLoginReady().then(() => {
      if (!container.isConnected || container.dataset.mounted !== mountedKey) return;
      googleAuthMode = mode;
      window.google.accounts.id.renderButton(container, {
        type: "standard",
        theme: "outline",
        size: "large",
        text: "continue_with",
        shape: "rectangular",
        logo_alignment: "center",
        width: googleWidgetWidth(container),
      });
      shell?.setAttribute("data-google-ready", "1");
    }).catch(() => {
      delete container.dataset.mounted;
      shell?.removeAttribute("data-google-ready");
    });
  });
}

async function startGoogleLogin(mode = "login", trigger = null) {
  googleAuthMode = mode === "link" ? "link" : "login";
  if (googleAuthMode === "link") return startGoogleLinkRedirect(trigger);
  googleLoginPendingMode = googleAuthMode;
  if (trigger) {
    trigger.disabled = true;
    trigger.classList.add("is-loading");
  }

  try {
    await ensureGoogleLoginReady();
    mountGoogleLoginWidgets();
    showToast(state.locale === "en" ? "Tap the Gmail button again" : "\u041d\u0430\u0436\u043c\u0438\u0442\u0435 \u043a\u043d\u043e\u043f\u043a\u0443 Gmail \u0435\u0449\u0451 \u0440\u0430\u0437");
  } catch (error) {
    showToast(error?.message || googleAuthCopy().loginUnavailable, "danger");
  } finally {
    window.setTimeout(() => {
      if (!trigger) return;
      trigger.disabled = false;
      trigger.classList.remove("is-loading");
    }, 900);
  }
}

async function startGoogleLinkRedirect(trigger = null) {
  if (state.loginMethodBusy) return;
  state.loginMethodBusy = "google";
  render();
  try {
    const response = await post("/api/mini-app/auth/google/link/start");
    const authUrl = String(response?.data?.authUrl || "").trim();
    if (!authUrl) throw new Error(googleAuthCopy().linkFailed);
    scheduleGoogleLinkRefresh();
    openExternal(authUrl);
    showToast(state.locale === "en" ? "Finish Gmail linking in the browser" : "\u0417\u0430\u0432\u0435\u0440\u0448\u0438\u0442\u0435 \u043f\u0440\u0438\u0432\u044f\u0437\u043a\u0443 Gmail \u0432 \u0431\u0440\u0430\u0443\u0437\u0435\u0440\u0435");
  } catch (error) {
    showToast(error?.message || googleAuthCopy().linkFailed, "danger");
  } finally {
    state.loginMethodBusy = "";
    render();
    if (trigger) {
      trigger.disabled = false;
      trigger.classList.remove("is-loading");
    }
  }
}

function handleGoogleCredential(response) {
  if (googleLoginPendingMode) {
    googleAuthMode = googleLoginPendingMode;
    googleLoginPendingMode = "";
  }
  const token = String(response?.credential || "").trim();
  if (!token) return showToast(googleAuthCopy().loginFailed, "danger");
  if (googleAuthMode === "link") return void linkGoogleAccount(token);

  writeSessionSetting(STORAGE_KEYS.googleLogin, token);
  clearBrowserTelegramAuth();
  state.loading = true;
  state.error = "";
  state.subscriptionGate = null;
  render();
  refreshDashboard({ initial: true }).catch((error) => {
    clearGoogleAuth();
    showToast(error?.message || googleAuthCopy().loginFailed, "danger");
  });
}

async function linkGoogleAccount(token) {
  if (state.loginMethodBusy) return;
  state.loginMethodBusy = "google";
  render();
  try {
    const response = await post("/api/mini-app/auth/google/link", { googleIdToken: token });
    state.data = response.data;
    state.locale = pickLocale(response.data?.user?.languageCode || state.locale);
    ensureSelections();
    state.loginMethodBusy = "";
    render();
    showToast(response.message || googleAuthCopy().linked, "success");
  } catch (error) {
    state.loginMethodBusy = "";
    render();
    throw error;
  }
}

function scheduleGoogleLinkRefresh() {
  const until = Date.now() + 2 * 60 * 1000;
  if (googleLinkRefreshTimer) window.clearInterval(googleLinkRefreshTimer);
  const tick = () => {
    if (Date.now() > until) {
      window.clearInterval(googleLinkRefreshTimer);
      googleLinkRefreshTimer = null;
      return;
    }
    if (!hasAuth() || !state.data || state.loading || state.refreshing) return;
    refreshDashboard({ silent: true }).catch(() => {});
  };
  googleLinkRefreshTimer = window.setInterval(tick, 5000);
}

function refreshAfterPossibleGoogleLink() {
  if (!googleLinkRefreshTimer || document.hidden || !hasAuth() || !state.data) return;
  refreshDashboard({ silent: true }).catch(() => {});
}

const PENDING_PAYMENT_KEY = "link-bot-pending-payment";
const STATIC_ASSET_REV = "20260724-v109";
const BRAND_MARK_PATH = "/mini-app/assets/brand-mark.png";
const BRAND_MARK_URL = `${BRAND_MARK_PATH}?v=${STATIC_ASSET_REV}`;

function resolveBrandMarkURL(value) {
  const url = String(value || "").trim();
  if (!url || url === BRAND_MARK_PATH || url.startsWith(`${BRAND_MARK_PATH}?`)) {
    return BRAND_MARK_URL;
  }
  return url;
}

const PAYMENT_LOGO_URLS = Object.freeze({
  sbp: "/mini-app/assets/payment-sbp.png",
  card: "/mini-app/assets/payment-card.png",
  stars: "/mini-app/assets/payment-stars.png",
  crypto: "/mini-app/assets/payment-crypto.png",
  lava: "/mini-app/assets/payment-lava.png",
  wata: "/mini-app/assets/payment-wata.png",
  platega: "/mini-app/assets/payment-platega.png",
  freekassa: "/mini-app/assets/payment-freekassa.png",
  heleket: "/mini-app/assets/payment-heleket.png",
  pally: "/mini-app/assets/payment-pally.png",
});

const PAGES = ["dashboard", "buy", "setup", "support", "faq", "reviews", "referrals", "servers", "settings", "media", "login-methods", "payments", "terms", "admin"];
const BOTTOM_NAV = ["dashboard", "buy", "support", "settings", "admin"];
const SUPPORT_TABS = ["open", "history"];
const PLATFORMS = ["windows", "android", "iphone", "mac"];

const PALETTE = {
  accent: {
    accent: "#cf244c",
    strong: "#f04b73",
    soft: "rgba(207,36,76,0.14)",
    border: "rgba(207,36,76,0.28)",
    particle: "rgba(207,36,76,0.34)",
  },
  themeColor: {
    dark: "#000000",
    light: "#faeaf6",
  },
};

const previewPayload = {
  brand: { name: "Link-Bot", logoUrl: BRAND_MARK_URL },
  user: { id: 777777, firstName: "Link", username: "linkbot", panelUsername: "", photoUrl: "", languageCode: "ru", authProvider: "telegram", googleEmail: "", googleLinked: false, telegramLinked: true },
  subscription: { status: "active", daysLeft: 26724, planMonths: 12, userUuid: "00000000-0000-0000-0000-000000000001", expiresAt: new Date(Date.now() + 26724 * 86400000).toISOString(), subscriptionLink: "https://example.com/sub/link-bot/secure-link", hasAccessLink: true, trafficUsedBytes: 0, trafficLimitBytes: 0, deviceUsedCount: 1, deviceLimitCount: 0, devices: [{ hwid: "demo-hwid-1", platform: "iOS", osVersion: "18.4", deviceModel: "iPhone 16", userAgent: "Happ/4.6.0/ios", createdAt: new Date(Date.now() - 86400000).toISOString(), updatedAt: new Date().toISOString() }] },
  trial: { enabled: true, eligible: false, days: 2 },
  referral: { enabled: true, count: 4, bonusDays: 7, bonusTrafficBytes: 53687091200, shareUrl: "https://t.me/share/url?url=https%3A%2F%2Ft.me%2Fyour_bot_username%3Fstart%3Dref_777777" },
  reviews: {
    count: 5,
    average: 4.6,
    canCreate: false,
    rewardDays: 2,
    rewardTrafficBytes: 21474836480,
    items: [
      { id: 1, username: "@alexvpn", rating: 5, comment: "Подключилось быстро, скорость отличная.", createdAt: new Date(Date.now() - 2 * 86400000).toISOString(), rewardGranted: true },
      { id: 2, username: "@exampleuser", rating: 5, comment: "Удобно, что всё теперь прямо внутри mini app.", createdAt: new Date(Date.now() - 86400000).toISOString(), rewardGranted: true, isMine: true },
      { id: 3, username: "@userfive", rating: 4, comment: "Всё ок, хотел бы ещё больше серверов.", createdAt: new Date(Date.now() - 6 * 3600000).toISOString(), rewardGranted: true },
    ],
    myReview: { id: 2, username: "@exampleuser", rating: 5, comment: "Удобно, что всё теперь прямо внутри mini app.", createdAt: new Date(Date.now() - 86400000).toISOString(), rewardGranted: true, isMine: true },
  },
  support: {
    isAdmin: false,
    openTickets: [
      { id: 101, subject: "Проблема с подключением", preview: "На iPhone не импортируется конфиг.", status: "open", updatedAt: new Date().toISOString(), createdAt: new Date().toISOString(), unreadCount: 1, subscriptionLabel: "Месяц" },
    ],
    historyTickets: [
      { id: 95, subject: "Не проходит оплата", preview: "Проблема решилась, спасибо.", status: "closed", updatedAt: new Date(Date.now() - 86400000).toISOString(), createdAt: new Date(Date.now() - 2 * 86400000).toISOString(), unreadCount: 0, subscriptionLabel: "Пробный" },
    ],
  },
  payments: {
    enabled: true,
    hasPaymentMethod: true,
    autoPaymentEnabled: false,
    autoPaymentPlanMonths: 1,
    method: { title: "Visa **** 4242", type: "bank_card", savedAt: new Date(Date.now() - 5 * 86400000).toISOString() },
    history: [
      { id: 301, months: 3, planLabel: "3 месяца", amount: 239, currency: "RUB", status: "paid", invoiceType: "yookassa", paymentMethodTitle: "Visa **** 4242", isAutoPayment: false, createdAt: new Date(Date.now() - 6 * 86400000).toISOString(), paidAt: new Date(Date.now() - 6 * 86400000).toISOString() },
      { id: 302, months: 1, planLabel: "Месяц", amount: 89, currency: "RUB", status: "paid", invoiceType: "yookassa", paymentMethodTitle: "Visa **** 4242", isAutoPayment: true, createdAt: new Date(Date.now() - 2 * 86400000).toISOString(), paidAt: new Date(Date.now() - 2 * 86400000).toISOString() },
    ],
  },
  plans: [],
  paymentMethods: [{ id: "sbp" }, { id: "card" }, { id: "stars" }, { id: "crypto" }],
  links: { support: "https://t.me/your_support_username", channel: "https://t.me/your_channel_username" },
  servers: { items: [
    { name: "Germany", address: "de.example.com", countryCode: "DE", online: true },
    { name: "USA", address: "us.example.com", countryCode: "US", online: true },
    { name: "Finland", address: "fi.example.com", countryCode: "FI", online: false },
  ] },
  meta: { now: new Date().toISOString(), botUrl: "https://t.me/your_bot_username", miniAppUrl: "https://example.com/mini-app/", googleClientId: "", starsNeedPriorPurchase: false },
};

const copybook = {
  ru: {
    appName: "Link-Bot", refresh: "Обновить", retry: "Повторить", pageDashboard: "Личный кабинет", pageBuy: "Тарифы", pageSetup: "Установить и настроить", pageSupport: "Поддержка", pageFaq: "FAQ", pageReferrals: "Реферальная система", pageServers: "Статус серверов", pageSettings: "Профиль", pageAdmin: "Админ панель",
    navDashboard: "Главная", navBuy: "Тарифы", navSupport: "Поддержка", navSettings: "Профиль", navAdmin: "Админ", dashboardLabel: "Основная", activeSubscription: "Подписка активна", inactiveSubscription: "Подписка не активна", trialAvailable: "Пробный доступ",
    buySubscription: "Купить подписку", extend: "Продлить", setup: "Подключиться", activateTrial: "Активировать пробный", support: "Поддержка", openAccess: "Открыть доступ", copyAccess: "Скопировать ссылку",
    loadingTitle: "Собираем ваш кабинет Link-Bot", loadingText: "Подтягиваем подписку, тарифы и быстрые действия.", openInTelegramTitle: "Откройте mini app из Telegram", openInTelegramText: "Telegram передаёт защищённые данные только внутри WebApp.", errorTitle: "Не удалось загрузить данные", subscriptionGateTitle: "Link-Bot Верификация", subscriptionGateLead: (channel) => `Чтобы открыть доступ к боту и mini app, подпишитесь на наш Telegram-канал ${channel}.`, subscriptionGateNews: "Там мы публикуем новости, обновления сервиса, важные изменения и полезные анонсы.", subscriptionGateHint: "После подписки нажмите кнопку ниже.", subscriptionGateOpen: "Link-Bot", subscriptionGateRetry: "✅ Я подписался",
    copied: "Ссылка скопирована", trialActivated: "Пробный период активирован", paymentOpened: "Окно оплаты открыто", invoiceOpened: "Инвойс открыт", paymentUnavailable: "Способ оплаты недоступен", noAccess: "Нет активной ссылки доступа", timeout: "Сервер не ответил вовремя", paymentCancelled: "Оплата отменена", paymentSuccess: "Успешная оплата", paymentPending: "Оплата ещё не завершена", resumePaymentTitle: "Оплата не завершена", resumePaymentText: "Продолжить оплату или вернуться в личный кабинет?", resumePaymentContinue: "Продолжить", resumePaymentReturn: "Вернуться в ЛК", paymentBrowserTitle: "Открыть оплату", paymentBrowserText: "Для СБП и банков на телефоне откроем YooKassa во внешнем браузере.", paymentBrowserOpen: "Открыть в браузере",
    invited: "Успешных приглашений", bonus: "Бонус", bonusDays: "Награда", expiresAt: "Истекает", quickAccess: "Быстрое подключение", shareReferral: "Поделиться", referralsHint: "Бонус начисляется только после того, как приглашённый пользователь оплатит любой тариф.", copyReferral: "Веб-ссылка", shareTelegram: "Telegram",
    selectTerm: "Выберите срок", selectedPlan: "Выбранный тариф", paymentMethod: "Способ оплаты", choosePaymentMethod: "Выберите способ оплаты", pay: "Оплатить", best: "Выгодно", perPeriod: "за период", savings: (v) => `-${v}%`,
    starsNeedPriorPurchase: "Telegram Stars откроются после первой оплаты картой или криптой.", serverStatus: "Статус серверов", feedback: "Отзывы", channel: "Новости", tos: "Пользовательское соглашение", tosHint: "Условия использования сервиса Link-Bot", webVersion: "Открыть web-версию",
    supportTitle: "Поддержка", supportHint: "Связь с поддержкой и полезные ссылки в одном месте.", newTicket: "Новое обращение", newTicketHint: "Связаться с поддержкой", supportTickets: "Открытые", supportLinks: "Полезное", noTickets: "Нет открытых обращений", noTicketsHint: "Когда появятся тикеты, они отобразятся здесь.",
    supportOpenTab: "Открытые", supportHistoryTab: "История", supportFaqTitle: "Часто задаваемые вопросы", supportFaqHint: "Быстрые ответы перед созданием обращения", supportAdminHint: "Новые обращения и ответы появляются здесь автоматически.", supportNoOpenTitle: "Нет открытых обращений", supportNoOpenHint: "Когда появятся тикеты, они отобразятся здесь.", supportNoHistoryTitle: "История обращений пуста", supportNoHistoryHint: "Закрытые обращения будут храниться здесь.", supportCreateTitle: "Создать обращение", supportSubjectLabel: "Тема", supportSubjectPlaceholder: "Кратко опишите проблему", supportMessageLabel: "Сообщение", supportMessagePlaceholder: "Опишите вашу проблему или вопрос подробно...", supportSendButton: "Отправить", supportCloseButton: "Закрыть обращение", supportClosedTitle: "Обращение закрыто", supportClosedHint: "История переписки сохранена в разделе истории.", supportReplyPlaceholder: "Напишите сообщение...", supportLoadingThread: "Загружаем переписку...",
    faqTitle: "Частые вопросы", faqHint: "Короткие ответы по подключению и оплате.", referralsTitle: "Реферальная программа", appearance: "Оформление", appearanceHint: "Переключайте светлую и тёмную тему внутри mini app.", theme: "Тема", darkTheme: "Тёмная", lightTheme: "Светлая", accentColor: "Акцент", settingsLinks: "Полезные ссылки", referralSystem: "Реферальная система",
    setupTitle: "Настройка подключения", setupHint: "Выберите устройство и получите доступ в пару касаний.", setupMissing: "Сначала нужен активный доступ", setupMissingHint: "Оформите тариф или активируйте пробный период, чтобы открыть ссылку подключения.", instructions: "Инструкция", accessLink: "Ссылка доступа", serverAll: "Все", serverOnline: "Онлайн", serverOffline: "Оффлайн", serverTotal: "Всего", serverStatusEmpty: "Ноды не найдены", serverStatusHint: "Живой статус серверов из панели",
    payMethodSbp: "СБП", payMethodSbpHint: "Система быстрых платежей", payMethodCard: "Карта", payMethodCardHint: "Оплата банковской картой", payMethodStars: "Telegram Stars", payMethodStarsHint: "Оплата звёздами Telegram", payMethodCrypto: "Crypto Pay", payMethodCryptoHint: "Оплата криптовалютой", subscriptionExpiringTemplate: "<tg-emoji emoji-id='5251391461244548685'>☺️</tg-emoji> <b>Подписка заканчивается</b>\n\nДата окончания:\n<b>{date}</b>\n\nПродлите доступ, чтобы сохранить соединение.", subscriptionExpiredTemplate: "<tg-emoji emoji-id='5251391461244548685'>☺️</tg-emoji> <b>Подписка закончилась</b>\n\nДата окончания:\n<b>{date}</b>\n\nВыберите тариф, чтобы восстановить доступ.", subscriptionRenewButton: "🔄 Продлить подписку", ready: "Ready", waiting: "После активации",
    monthLabel: (c) => `${c} ${pluralizeRu(c, ["месяц", "месяца", "месяцев"])}`, dayLabel: (c) => `${formatNumber(c, "ru")} ${pluralizeRu(c, ["день", "дня", "дней"])}`,
    paymentsTitle: "Платежи", paymentsHint: "Оплаты и история покупок", autopayTitle: "Автоплатёж", paymentHistory: "История покупок", paymentHistoryEmpty: "Покупок пока нет", agreementText: "", agreementLink: "Пользовательское соглашение", agreementRequired: "Подтвердите соглашение перед оплатой",
  },
  en: {
    appName: "Link-Bot", refresh: "Refresh", retry: "Retry", pageDashboard: "Dashboard", pageBuy: "Plans", pageSetup: "Setup", pageSupport: "Support", pageFaq: "FAQ", pageReferrals: "Referrals", pageServers: "Server status", pageSettings: "Profile", pageAdmin: "Admin panel",
    navDashboard: "Home", navBuy: "Plans", navSupport: "Support", navSettings: "Profile", navAdmin: "Admin", dashboardLabel: "Main", activeSubscription: "Subscription active", inactiveSubscription: "Subscription inactive", trialAvailable: "Trial access",
    buySubscription: "Buy subscription", extend: "Extend", setup: "Setup", activateTrial: "Activate trial", support: "Support", openAccess: "Open access", copyAccess: "Copy link",
    loadingTitle: "Building your Link-Bot dashboard", loadingText: "Loading subscription, plans and quick actions.", openInTelegramTitle: "Open this mini app from Telegram", openInTelegramText: "Telegram sends secure user data only inside the WebApp.", errorTitle: "Could not load data", subscriptionGateTitle: "Link-Bot Verification", subscriptionGateLead: (channel) => `To unlock the bot and mini app, subscribe to our Telegram channel ${channel}.`, subscriptionGateNews: "We post news, service updates, important changes and helpful announcements there.", subscriptionGateHint: "After subscribing, tap the button below.", subscriptionGateOpen: "Link-Bot", subscriptionGateRetry: "✅ I subscribed",
    copied: "Link copied", trialActivated: "Trial activated", paymentOpened: "Payment window opened", invoiceOpened: "Invoice opened", paymentUnavailable: "Payment method is unavailable", noAccess: "No active access link", timeout: "Server timeout", paymentCancelled: "Payment cancelled", paymentSuccess: "Payment successful", paymentPending: "Payment is not completed yet", resumePaymentTitle: "Payment not completed", resumePaymentText: "Continue payment or return to your dashboard?", resumePaymentContinue: "Continue", resumePaymentReturn: "Return to dashboard", paymentBrowserTitle: "Open payment", paymentBrowserText: "For SBP and mobile banking, we'll open YooKassa in your external browser.", paymentBrowserOpen: "Open in browser",
    invited: "Successful referrals", bonus: "Bonus", bonusDays: "Reward", expiresAt: "Expires", quickAccess: "Quick connection", shareReferral: "Share", referralsHint: "The reward is credited only after the invited user buys any subscription.", copyReferral: "Web link", shareTelegram: "Telegram",
    selectTerm: "Choose a term", selectedPlan: "Selected plan", paymentMethod: "Payment method", choosePaymentMethod: "Choose payment method", pay: "Pay", best: "Best", perPeriod: "per period", savings: (v) => `-${v}%`,
    starsNeedPriorPurchase: "Telegram Stars unlock after the first card or crypto payment.", serverStatus: "Server status", feedback: "Reviews", channel: "News", tos: "Terms of service", tosHint: "Rules and conditions for using Link-Bot", webVersion: "Open web version",
    supportTitle: "Support", supportHint: "Support and useful links in one place.", newTicket: "New ticket", newTicketHint: "Contact support", supportTickets: "Open", supportLinks: "Useful", noTickets: "No open tickets", noTicketsHint: "Your tickets will appear here.",
    faqTitle: "FAQ", faqHint: "Quick answers about setup and payments.", referralsTitle: "Referral program", appearance: "Appearance", appearanceHint: "Switch between light and dark themes inside the mini app.", theme: "Theme", darkTheme: "Dark", lightTheme: "Light", accentColor: "Accent", settingsLinks: "Useful links", referralSystem: "Referral system",
    setupTitle: "Connection setup", setupHint: "Choose a device and connect in a couple taps.", setupMissing: "You need an active access link first", setupMissingHint: "Buy a plan or activate a trial to open the connection link.", instructions: "Guide", accessLink: "Access link", serverAll: "All", serverOnline: "Online", serverOffline: "Offline", serverTotal: "Total", serverStatusEmpty: "No nodes found", serverStatusHint: "Live node status from panel",
    payMethodSbp: "SBP", payMethodSbpHint: "Faster Payments System", payMethodCard: "Card", payMethodCardHint: "Bank card payment", payMethodStars: "Telegram Stars", payMethodStarsHint: "Pay with Telegram Stars", payMethodCrypto: "Crypto Pay", payMethodCryptoHint: "Cryptocurrency payment", subscriptionExpiringTemplate: "⚠️ <b>Subscription expires soon</b>\n\nExpiration date:\n<b>{date}</b>\n\nRenew access to stay connected.", subscriptionExpiredTemplate: "⚠️ <b>Subscription expired</b>\n\nExpiration date:\n<b>{date}</b>\n\nChoose a plan to restore access.", subscriptionRenewButton: "🔄 Renew subscription", ready: "Ready", waiting: "After activation",
    monthLabel: (c) => `${c} month${c === 1 ? "" : "s"}`, dayLabel: (c) => `${formatNumber(c, "en")} day${c === 1 ? "" : "s"}`,
    paymentsTitle: "Payments", paymentsHint: "Payments and purchase history", autopayTitle: "Autopay", paymentHistory: "Purchase history", paymentHistoryEmpty: "No purchases yet", agreementText: "", agreementLink: "Terms of Service", agreementRequired: "Please confirm the terms before paying",
  },
};

Object.assign(copybook.ru, {
  promoCode: "Промокод",
  promoCodeHint: "Скидка применится к выбранному тарифу",
  promoCodePlaceholder: "Введите промокод",
  promoApply: "Применить",
  promoApplied: "Промокод применён",
  promoAppliedHint: (code, percent) => `${code} · скидка ${percent}%`,
  promoExpiresAt: "Действует до",
  promoCodeRequired: "Введите промокод",
  promoInvalid: "Такого промокода не существует",
  promoExpired: "Промокод истёк",
  promoInactive: "Промокод отключён",
  promoInvalidFormat: "Промокод может содержать только буквы, цифры, - и _",
  promoInvalidDiscount: "Укажите скидку от 1% до 99%",
  promoInvalidExpiry: "Укажите корректную дату окончания",
  promoCreateFailed: "Не удалось создать промокод",
  promoCreateSuccess: "Промокод создан",
  promoAlreadyExists: "Такой промокод уже существует",
  promoUnavailable: "Промокоды временно недоступны",
  promoFinalPrice: "Цена со скидкой",
  adminPromoTitle: "Промокоды",
  adminPromoHint: "Создавайте скидки для любой покупки прямо из mini app.",
  adminPromoCreate: "Создать промокод",
  adminPromoCodeLabel: "Код",
  adminPromoDiscountLabel: "Скидка, %",
  adminPromoExpiresLabel: "Действует до",
  adminPromoExpiresPlaceholder: "Необязательно",
  adminPromoListTitle: "Активные и созданные коды",
  adminPromoEmpty: "Промокодов пока нет",
  adminPromoStatusActive: "Активен",
  adminPromoStatusExpired: "Истёк",
  adminPromoStatusInactive: "Отключён",
});

Object.assign(copybook.en, {
  promoCode: "Promo code",
  promoCodeHint: "The discount will be applied to the selected plan",
  promoCodePlaceholder: "Enter promo code",
  promoApply: "Apply",
  promoApplied: "Promo code applied",
  promoAppliedHint: (code, percent) => `${code} · ${percent}% off`,
  promoExpiresAt: "Valid until",
  promoCodeRequired: "Enter a promo code",
  promoInvalid: "Promo code not found",
  promoExpired: "Promo code has expired",
  promoInactive: "Promo code is inactive",
  promoInvalidFormat: "Promo code may contain only letters, numbers, - and _",
  promoInvalidDiscount: "Enter a discount from 1% to 99%",
  promoInvalidExpiry: "Enter a valid expiry date",
  promoCreateFailed: "Failed to create promo code",
  promoCreateSuccess: "Promo code created",
  promoAlreadyExists: "This promo code already exists",
  promoUnavailable: "Promo codes are temporarily unavailable",
  promoFinalPrice: "Discounted price",
  adminPromoTitle: "Promo codes",
  adminPromoHint: "Create discounts for any purchase directly in the mini app.",
  adminPromoCreate: "Create promo code",
  adminPromoCodeLabel: "Code",
  adminPromoDiscountLabel: "Discount, %",
  adminPromoExpiresLabel: "Valid until",
  adminPromoExpiresPlaceholder: "Optional",
  adminPromoListTitle: "Created codes",
  adminPromoEmpty: "No promo codes yet",
  adminPromoStatusActive: "Active",
  adminPromoStatusExpired: "Expired",
  adminPromoStatusInactive: "Disabled",
});

Object.assign(copybook.ru, {
  promoAlreadyUsed: "Этот промокод уже был использован",
  promoPending: "У вас уже есть неоплаченная покупка с этим промокодом",
  promoLimitReached: "Лимит использований промокода исчерпан",
  promoInvalidLimit: "Укажите корректный лимит пользователей",
  promoDeleteFailed: "Не удалось удалить промокод",
  promoDeleteSuccess: "Промокод удалён",
  promoCompactHint: "Скидка применится после успешной оплаты",
  adminPanelHint: "Управление функциями mini app",
  adminPromoMenuHint: "Создание, лимиты и удаление кодов",
  adminPromoManage: "Промокоды",
  adminPromoLimitLabel: "Лимит пользователей",
  adminPromoLimitPlaceholder: "Необязательно",
  adminPromoDelete: "Удалить",
  adminPromoStatusExhausted: "Исчерпан",
  adminPromoUnlimited: "Без лимита",
  adminPromoNoExpiry: "Без срока",
  adminPromoUsage: (used, total) => total > 0 ? `${used}/${total} использ.` : `${used} использ.`,
  adminSubscriptionManage: "Привязка подписки",
  adminSubscriptionMenuHint: "Перенос доступа между Telegram-аккаунтами",
  adminSubscriptionTitle: "Привязка подписки",
  adminSubscriptionHint: "Найдите подписку по ID из панели или точному имени пользователя.",
  adminSubscriptionQueryLabel: "ID или имя подписки",
  adminSubscriptionQueryPlaceholder: "Например, 1281 или 10204_1404001393",
  adminSubscriptionFind: "Найти подписку",
  adminSubscriptionPanelID: "ID в панели",
  adminSubscriptionUsername: "Имя подписки",
  adminSubscriptionCurrentTelegram: "Текущий Telegram ID",
  adminSubscriptionStatus: "Статус",
  adminSubscriptionExpires: "Действует до",
  adminSubscriptionTargetLabel: "Новый Telegram ID",
  adminSubscriptionTargetPlaceholder: "Введите Telegram ID",
  adminSubscriptionTargetHint: "Новый аккаунт должен хотя бы один раз запустить бота командой /start.",
  adminSubscriptionRebind: "Перепривязать подписку",
  adminSubscriptionSuccess: "Подписка перепривязана",
  adminSubscriptionConfirm: (username, telegramID) => `Перепривязать ${username} к Telegram ID ${telegramID}? На прежнем аккаунте управление подпиской пропадёт. Если у нового аккаунта уже есть другая подписка, она будет отвязана.`,
});

Object.assign(copybook.en, {
  promoAlreadyUsed: "You have already used this promo code",
  promoPending: "You already have a pending purchase with this promo code",
  promoLimitReached: "Promo code limit has been reached",
  promoInvalidLimit: "Enter a valid user limit",
  promoDeleteFailed: "Failed to delete promo code",
  promoDeleteSuccess: "Promo code deleted",
  promoCompactHint: "The discount will be applied after successful payment",
  adminPanelHint: "Manage mini app tools",
  adminPromoMenuHint: "Create, limit and remove promo codes",
  adminPromoManage: "Promo codes",
  adminPromoLimitLabel: "User limit",
  adminPromoLimitPlaceholder: "Optional",
  adminPromoDelete: "Delete",
  adminPromoStatusExhausted: "Exhausted",
  adminPromoUnlimited: "Unlimited",
  adminPromoNoExpiry: "No expiry",
  adminPromoUsage: (used, total) => total > 0 ? `${used}/${total} used` : `${used} used`,
  adminSubscriptionManage: "Subscription binding",
  adminSubscriptionMenuHint: "Move access between Telegram accounts",
  adminSubscriptionTitle: "Subscription binding",
  adminSubscriptionHint: "Find a subscription by its panel ID or exact username.",
  adminSubscriptionQueryLabel: "Subscription ID or username",
  adminSubscriptionQueryPlaceholder: "For example, 1281 or 10204_1404001393",
  adminSubscriptionFind: "Find subscription",
  adminSubscriptionPanelID: "Panel ID",
  adminSubscriptionUsername: "Subscription username",
  adminSubscriptionCurrentTelegram: "Current Telegram ID",
  adminSubscriptionStatus: "Status",
  adminSubscriptionExpires: "Expires",
  adminSubscriptionTargetLabel: "New Telegram ID",
  adminSubscriptionTargetPlaceholder: "Enter Telegram ID",
  adminSubscriptionTargetHint: "The new account must start the bot with /start at least once.",
  adminSubscriptionRebind: "Rebind subscription",
  adminSubscriptionSuccess: "Subscription rebound",
  adminSubscriptionConfirm: (username, telegramID) => `Rebind ${username} to Telegram ID ${telegramID}? The previous account will lose subscription management. If the new account already has another subscription, it will be unlinked.`,
});

Object.assign(copybook.ru, {
  noPlansTitle: "Тарифы пока не настроены",
  noPlansHint: "Покупка станет доступна после добавления тарифов.",
  adminNoPlansHint: "Добавьте первый тариф кнопкой ниже.",
  noPaymentMethodsTitle: "Способы оплаты не настроены",
  noPaymentMethodsHint: "Подключите платёжную систему в разделе интеграций.",
});

Object.assign(copybook.en, {
  noPlansTitle: "Plans are not configured yet",
  noPlansHint: "Purchasing will become available after plans are added.",
  adminNoPlansHint: "Add the first plan using the button below.",
  noPaymentMethodsTitle: "Payment methods are not configured",
  noPaymentMethodsHint: "Connect a payment provider in Integrations.",
});

const FAQS = {
  ru: [
    ["Не получается подключиться к VPN", "Откройте страницу установки, выберите устройство и нажмите «Открыть доступ». Если проблема остаётся, отправьте ссылку доступа в поддержку."],
    ["Как быстро продлить подписку?", "Откройте вкладку «Тарифы», выберите срок и удобный способ оплаты. После успешной оплаты статус обновится автоматически."],
    ["Где взять ссылку подключения?", "Если подписка уже активна, ссылка доступна на главной странице и на странице настройки. Её можно открыть или скопировать одним нажатием."],
  ],
  en: [
    ["I cannot connect to the VPN", "Open the setup page, choose your device and tap “Open access”. If it still fails, send the access link to support."],
    ["How do I renew quickly?", "Open the Plans page, choose a term and pay with a convenient method. The dashboard will refresh automatically after payment."],
    ["Where is my connection link?", "If your subscription is active, the link is available on the dashboard and setup page. You can open or copy it in one tap."],
  ],
};

FAQS.ru = [["Почему все сервера показывают n/a?", "Сервера часто обновляются для лучшей производительности, обновите подписку в Happ и еще раз сделайте тест пинга."]];

FAQS.ru = [
  ["Не получается подключиться к VPN", "Откройте страницу установки, выберите устройство и нажмите \"Открыть доступ\". Если проблема остаётся, отправьте ссылку доступа в поддержку."],
  ["Как быстро продлить подписку?", "Откройте вкладку \"Тарифы\", выберите срок и удобный способ оплаты. После успешной оплаты статус обновится автоматически."],
  ["Где взять ссылку подключения?", "Если подписка уже активна, ссылка доступна на главной странице и на странице настройки. Её можно открыть или скопировать одним нажатием."],
  ["Почему все сервера показывают n/a?", "Сервера часто обновляются для лучшей производительности, обновите подписку в Happ и еще раз сделайте тест пинга."],
  ["WhatsApp не работает даже с ВПН", "Для решения данной проблемы, попробуйте переустановить приложение, либо скачать WhatsApp business. Обязательно сделайте резервную копию файлов и чатов в настройках приложения перед его удалением."],
  ["Что делать, если подписка заканчивается или уже закончилась?", "Нажмите кнопку в уведомлении от бота или откройте вкладку \"Тарифы\", выберите тариф и оплатите его. Доступ обновится автоматически после оплаты."],
];

const TERMS_ARTICLE = {
  ru: {
    title: "Пользовательское соглашение",
    effectiveLabel: "Дата вступления в силу",
    jurisdiction: "Юрисдикция: Российская Федерация",
    intro: [
      "Настоящее Пользовательское соглашение регулирует условия использования VPN-сервиса Link-Bot, права и обязанности пользователей, а также отношения между пользователем и администрацией сервиса.",
      "Используя Link-Bot, пользователь подтверждает, что ознакомился с условиями настоящего Соглашения, понял их и принимает полностью, без оговорок и исключений.",
    ],
    sections: [
      {
        title: "1. Общие положения",
        paragraphs: [
          "1.1. Настоящее Соглашение является публичной офертой в соответствии со статьёй 437 Гражданского кодекса Российской Федерации.",
          "1.2. Использование Link-Bot означает полное и безоговорочное принятие условий настоящего Соглашения.",
          "1.3. Администрация вправе изменять настоящее Соглашение без предварительного уведомления пользователя. Актуальная редакция размещается внутри официальных ресурсов Link-Bot.",
          "1.4. Продолжение использования сервиса после публикации новой редакции означает согласие пользователя с такими изменениями.",
          "1.5. Запуск Telegram mini app Link-Bot, открытие Telegram-бота или браузерной версии сервиса означает, что пользователь автоматически ознакомился с настоящим Соглашением и принимает его условия.",
        ],
      },
      {
        title: "2. Описание сервиса",
        paragraphs: [
          "2.1. Link-Bot предоставляет доступ к VPN-инфраструктуре для шифрования трафика, изменения маршрута соединения и упрощения доступа к интернет-ресурсам.",
          "2.2. Сервис предоставляется по модели «как есть» (as is). Администрация не гарантирует постоянную доступность, фиксированную скорость, отсутствие блокировок отдельных ресурсов и непрерывную работу всех серверов.",
          "2.3. Функциональность сервиса, перечень тарифов, лимиты трафика, количество устройств, бонусы, пробные периоды и иные условия могут изменяться по усмотрению Администрации.",
        ],
      },
      {
        title: "3. Учётная запись и доступ",
        paragraphs: [
          "3.1. Доступ к Link-Bot осуществляется через Telegram mini app, Telegram-бота и иные связанные с ними средства идентификации пользователя.",
          "3.2. Пользователь обязан самостоятельно обеспечивать сохранность своих ссылок доступа, конфигураций, подключённых устройств и иных данных, позволяющих использовать сервис.",
          "3.3. Передача доступа третьим лицам, перепродажа доступа, совместное использование одной подписки в обход лимитов либо любые попытки скрытого шаринга запрещены.",
        ],
      },
      {
        title: "4. Тарифы, оплата и бонусы",
        paragraphs: [
          "4.1. Доступ к платным функциям Link-Bot предоставляется по действующим тарифам, указанным в сервисе на момент оплаты.",
          "4.2. Оплата осуществляется через сторонние платёжные системы и сервисы. Администрация не несёт ответственности за задержки, ошибки, комиссии и технические ограничения на стороне платёжных провайдеров.",
          "4.3. Бонусные дни, реферальные начисления, пробные периоды, подарки за отзывы и иные промо-механики могут быть отменены, уменьшены либо аннулированы при выявлении злоупотреблений, накрутки или подозрительной активности.",
          "4.4. Поскольку Link-Bot предоставляет цифровой доступ к сервису, возвраты и компенсации осуществляются только в объёме и порядке, которые прямо предусмотрены законодательством или правилами используемой платёжной системы.",
          "4.5. При оплате банковской картой пользователь соглашается на привязку способа оплаты для автопродления. Списание средств происходит автоматически по окончании срока подписки. Автоплатежи можно отключить в разделе «Платежи».",
        ],
      },
      {
        title: "5. Пользователь обязуется",
        paragraphs: [
          "5.1. Не использовать Link-Bot для деятельности, нарушающей законодательство Российской Федерации либо применимое законодательство иных стран.",
          "5.2. Не распространять вредоносное программное обеспечение, спам, фишинг, запрещённый контент, а также не нарушать права третьих лиц.",
          "5.3. Не предпринимать попыток вмешательства в работу Link-Bot, обхода технических ограничений, подмены данных, эксплуатации уязвимостей либо перегрузки инфраструктуры сервиса.",
        ],
        items: [
          "не злоупотреблять пробными периодами, отзывами, реферальной системой и бонусными начислениями;",
          "не использовать Link-Bot как коммерческий resale-доступ без отдельного разрешения Администрации;",
          "не выдавать себя за представителя Link-Bot без прямого согласия Администрации.",
        ],
      },
      {
        title: "6. Права Администрации",
        paragraphs: [
          "6.1. Администрация вправе ограничить, приостановить, аннулировать или полностью прекратить доступ пользователя к сервису, подписке, бонусам или отдельным функциям в любое время, если сочтёт это необходимым.",
          "6.2. Администрация вправе аннулировать подписку пользователя полностью или частично, в том числе без раскрытия причин, если это требуется для защиты сервиса, инфраструктуры, иных пользователей либо по внутренним правилам Link-Bot.",
          "6.3. Администрация вправе проводить технические, аварийные, профилактические и иные работы без предварительного уведомления, а также менять состав серверов, нод, тарифов, лимитов и способов оплаты.",
          "6.4. Link-Bot может полностью прекратить своё существование, работу либо развитие в связи с техническими, финансовыми, юридическими, инфраструктурными или иными обстоятельствами. Пользователь принимает этот риск, начиная использование сервиса.",
        ],
      },
      {
        title: "7. Ограничение ответственности",
        paragraphs: [
          "7.1. Администрация не несёт ответственности за любые прямые или косвенные убытки, возникшие у пользователя в результате использования либо невозможности использования Link-Bot.",
          "7.2. Администрация не гарантирует доступность конкретных сайтов, приложений, игр, банковских сервисов, стриминговых платформ и иных ресурсов через Link-Bot.",
          "7.3. Пользователь использует Link-Bot на свой страх и риск и самостоятельно оценивает правовые последствия использования VPN-технологий на территории своей страны.",
        ],
      },
      {
        title: "8. Данные и конфиденциальность",
        paragraphs: [
          "8.1. Link-Bot обрабатывает только тот объём технических и учётных данных, который необходим для работы сервиса, оплаты, поддержки пользователей и противодействия злоупотреблениям.",
          "8.2. Администрация вправе хранить технические сведения, необходимые для защиты инфраструктуры, диагностики неисправностей и пресечения мошенничества.",
          "8.3. Link-Bot не гарантирует абсолютную анонимность пользователя от любых внешних факторов, включая блокировки, действия третьих лиц, ошибки приложений и ограничения платформ.",
        ],
      },
      {
        title: "9. Связь с Администрацией",
        paragraphs: [
          "9.1. Связь с Администрацией осуществляется исключительно через Telegram: @your_support_username либо через встроенную поддержку внутри mini app Link-Bot.",
          "9.2. Иные способы связи могут отсутствовать. Ответ поддержки предоставляется в разумный срок, но не гарантируется мгновенно.",
        ],
      },
      {
        title: "10. Разрешение споров",
        paragraphs: [
          "10.1. Все споры и разногласия стороны стремятся урегулировать путём переговоров и обращения в поддержку.",
          "10.2. При невозможности урегулировать спор мирным путём он подлежит рассмотрению в соответствии с законодательством Российской Федерации.",
        ],
      },
      {
        title: "11. Заключительные положения",
        paragraphs: [
          "11.1. Все элементы интерфейса Link-Bot, тексты, визуальные материалы, логотипы, код и иные объекты сервиса защищены законодательством об интеллектуальной собственности.",
          "11.2. Начав использование Link-Bot, пользователь подтверждает, что прочитал настоящее Соглашение и принимает его полностью.",
        ],
      },
    ],
    contactTitle: "Контакты",
    contacts: [
      "Telegram поддержки: @your_support_username",
      "Встроенная поддержка внутри mini app Link-Bot",
    ],
    footer: "© 2026 Link-Bot. Все права защищены.",
  },
  en: {
    title: "Terms of Service",
    effectiveLabel: "Effective date",
    jurisdiction: "Jurisdiction: Russian Federation",
    intro: [
      "These Terms of Service define the conditions for using the Link-Bot service, user rights and obligations, and the relationship between the user and the service administration.",
      "By using Link-Bot, the user confirms that they have read, understood and fully accepted these Terms.",
    ],
    sections: [
      {
        title: "1. General terms",
        paragraphs: [
          "1.1. These Terms are a public offer under the laws of the Russian Federation.",
          "1.2. Using Link-Bot means full acceptance of these Terms.",
          "1.3. The Administration may update these Terms without prior notice.",
          "1.4. Opening the Link-Bot Telegram mini app, Telegram bot or browser version means that the user has read and accepted these Terms.",
        ],
      },
      {
        title: "2. Service description",
        paragraphs: [
          "2.1. Link-Bot provides VPN access for encrypted traffic and improved network routing.",
          "2.2. The service is provided on an “as is” basis without guarantees of permanent availability, speed or uninterrupted operation.",
        ],
      },
      {
        title: "3. Access and account",
        paragraphs: [
          "3.1. Access is provided through the Telegram mini app, bot and related identifiers.",
          "3.2. Sharing access with third parties or bypassing tariff limits is prohibited.",
        ],
      },
      {
        title: "4. Payments and bonuses",
        paragraphs: [
          "4.1. Paid access is provided according to the current tariffs shown inside Link-Bot.",
          "4.2. Bonus days, trials, referral rewards and review rewards may be revoked in case of abuse or suspicious activity.",
          "4.3. When paying by bank card the payment method may be saved for auto-renewal. Charges are made automatically when the subscription ends. Autopay can be disabled in the Payments section.",
        ],
      },
      {
        title: "5. Administration rights",
        paragraphs: [
          "5.1. The Administration may suspend, restrict, cancel or revoke any subscription or access at its discretion.",
          "5.2. Link-Bot may be fully discontinued due to technical, legal, financial or other circumstances.",
        ],
      },
      {
        title: "6. Liability",
        paragraphs: [
          "6.1. The user uses Link-Bot at their own risk.",
          "6.2. The Administration is not responsible for direct or indirect losses resulting from the use or inability to use the service.",
        ],
      },
      {
        title: "7. Contact",
        paragraphs: [
          "7.1. Contact is available only through Telegram @your_support_username or through the in-app support inside Link-Bot.",
        ],
      },
    ],
    contactTitle: "Contact",
    contacts: [
      "Telegram: @your_support_username",
      "In-app support inside Link-Bot",
    ],
    footer: "© 2026 Link-Bot. All rights reserved.",
  },
};

const SETUP_STEPS = {
  ru: { windows: ["Откройте ссылку доступа", "Импортируйте конфиг в клиент", "Нажмите подключиться"], android: ["Откройте ссылку доступа", "Выберите Android-клиент", "Подтвердите импорт и подключение"], iphone: ["Откройте ссылку доступа", "Импортируйте конфиг в приложение", "Разрешите VPN-профиль"], mac: ["Откройте ссылку доступа", "Добавьте конфиг в клиент", "Запустите подключение"] },
  en: { windows: ["Open the access link", "Import the config into the client", "Start the connection"], android: ["Open the access link", "Choose the Android client", "Confirm import and connect"], iphone: ["Open the access link", "Import the config into the app", "Allow the VPN profile"], mac: ["Open the access link", "Add the config to the client", "Start the connection"] },
};

const INSTALL_GUIDES = {
  android: {
    icon: "android",
    title: "\u0418\u043d\u0441\u0442\u0440\u0443\u043a\u0446\u0438\u044f \u0434\u043b\u044f Android",
    browser: "Chrome",
    browserIcon: "chrome",
    steps: [
      { icon: "dotsVertical", title: "\u041e\u0442\u043a\u0440\u043e\u0439\u0442\u0435 \u043c\u0435\u043d\u044e", text: "\u041d\u0430\u0436\u043c\u0438\u0442\u0435 \u043d\u0430 \u0442\u0440\u0438 \u0442\u043e\u0447\u043a\u0438 (\u22ee) \u0432 \u043f\u0440\u0430\u0432\u043e\u043c \u0432\u0435\u0440\u0445\u043d\u0435\u043c \u0443\u0433\u043b\u0443 \u0431\u0440\u0430\u0443\u0437\u0435\u0440\u0430" },
      { icon: "download", title: "\u0423\u0441\u0442\u0430\u043d\u043e\u0432\u0438\u0442\u044c \u043f\u0440\u0438\u043b\u043e\u0436\u0435\u043d\u0438\u0435", text: "\u0412\u044b\u0431\u0435\u0440\u0438\u0442\u0435 \"\u0423\u0441\u0442\u0430\u043d\u043e\u0432\u0438\u0442\u044c \u043f\u0440\u0438\u043b\u043e\u0436\u0435\u043d\u0438\u0435\" \u0438\u043b\u0438 \"\u0414\u043e\u0431\u0430\u0432\u0438\u0442\u044c \u043d\u0430 \u0433\u043b\u0430\u0432\u043d\u044b\u0439 \u044d\u043a\u0440\u0430\u043d\"" },
      { icon: "plus", title: "\u0423\u0441\u0442\u0430\u043d\u043e\u0432\u0438\u0442\u044c", text: "\u041d\u0430\u0436\u043c\u0438\u0442\u0435 \"\u0423\u0441\u0442\u0430\u043d\u043e\u0432\u0438\u0442\u044c\" \u0434\u043b\u044f \u043f\u043e\u0434\u0442\u0432\u0435\u0440\u0436\u0434\u0435\u043d\u0438\u044f" },
    ],
    alternate: "\u041f\u043e\u043a\u0430\u0437\u0430\u0442\u044c \u0434\u043b\u044f iOS",
  },
  ios: {
    icon: "appleOutline",
    title: "\u0418\u043d\u0441\u0442\u0440\u0443\u043a\u0446\u0438\u044f \u0434\u043b\u044f iOS",
    browser: "Safari",
    browserIcon: "safari",
    steps: [
      { icon: "shareNodes", title: "\u041d\u0430\u0436\u043c\u0438\u0442\u0435 \"\u041f\u043e\u0434\u0435\u043b\u0438\u0442\u044c\u0441\u044f\"", text: "\u0412 Safari \u043d\u0430\u0436\u043c\u0438\u0442\u0435 \u0438\u043a\u043e\u043d\u043a\u0443 \"\u041f\u043e\u0434\u0435\u043b\u0438\u0442\u044c\u0441\u044f\" (\u043a\u0432\u0430\u0434\u0440\u0430\u0442 \u0441\u043e \u0441\u0442\u0440\u0435\u043b\u043a\u043e\u0439 \u0432\u0432\u0435\u0440\u0445) \u0432\u043d\u0438\u0437\u0443 \u044d\u043a\u0440\u0430\u043d\u0430" },
      { icon: "homeOutline", title: "\u041d\u0430 \u044d\u043a\u0440\u0430\u043d \"\u0414\u043e\u043c\u043e\u0439\"", text: "\u041f\u0440\u043e\u043a\u0440\u0443\u0442\u0438\u0442\u0435 \u0432\u043d\u0438\u0437 \u0438 \u0432\u044b\u0431\u0435\u0440\u0438\u0442\u0435 \"\u041d\u0430 \u044d\u043a\u0440\u0430\u043d \u0414\u043e\u043c\u043e\u0439\"" },
      { icon: "plus", title: "\u0414\u043e\u0431\u0430\u0432\u0438\u0442\u044c", text: "\u041d\u0430\u0436\u043c\u0438\u0442\u0435 \"\u0414\u043e\u0431\u0430\u0432\u0438\u0442\u044c\" \u0432 \u043f\u0440\u0430\u0432\u043e\u043c \u0432\u0435\u0440\u0445\u043d\u0435\u043c \u0443\u0433\u043b\u0443" },
    ],
    alternate: "\u041f\u043e\u043a\u0430\u0437\u0430\u0442\u044c \u0434\u043b\u044f Android",
  },
};

const ADMIN_LAYOUT_CATEGORIES = [
	{ id: "dashboard", labelRu: "\u0413\u043b\u0430\u0432\u043d\u0430\u044f", labelEn: "Home", icon: "home" },
	{ id: "profile", labelRu: "\u041f\u0440\u043e\u0444\u0438\u043b\u044c", labelEn: "Profile", icon: "profile" },
];

const PROFILE_GROUP_ORDER = ["main", "purchases", "programs", "help", "account"];
const PROFILE_DEFAULT_GROUPS = {
	server_status: "main", media: "main", news: "main",
	payments: "purchases",
	referrals: "programs", reviews: "programs",
	terms: "help",
	login_methods: "account", web_version: "account", pwa_install: "account",
};

const ADMIN_LAYOUT_META = {
	"buy:plans": ["\u0422\u0430\u0440\u0438\u0444\u044b", "cartShopping"],
	"buy:checkout": ["\u041e\u043f\u043b\u0430\u0442\u0430", "wallet"],
	"support:actions": ["\u0411\u044b\u0441\u0442\u0440\u044b\u0435 \u0434\u0435\u0439\u0441\u0442\u0432\u0438\u044f", "plus"],
	"support:tabs": ["\u0412\u043a\u043b\u0430\u0434\u043a\u0438", "menu"],
	"support:tickets": ["\u041e\u0431\u0440\u0430\u0449\u0435\u043d\u0438\u044f", "headphonesAlt"],
	"profile:server_status": ["\u0421\u0442\u0430\u0442\u0443\u0441 \u0441\u0435\u0440\u0432\u0435\u0440\u043e\u0432", "server"],
	"profile:referrals": ["\u0420\u0435\u0444\u0435\u0440\u0430\u043b\u044c\u043d\u0430\u044f \u0441\u0438\u0441\u0442\u0435\u043c\u0430", "users"],
	"profile:reviews": ["\u041e\u0442\u0437\u044b\u0432\u044b", "star"],
	"profile:payments": ["\u041f\u043b\u0430\u0442\u0435\u0436\u0438", "wallet"],
	"profile:media": ["\u041c\u0435\u0434\u0438\u0430", "youtube"],
	"profile:login_methods": ["\u0421\u043f\u043e\u0441\u043e\u0431 \u0432\u0445\u043e\u0434\u0430", "lockAlt"],
	"profile:news": ["\u041d\u043e\u0432\u043e\u0441\u0442\u0438", "broadcast"],
	"profile:web_version": ["\u041e\u0442\u043a\u0440\u044b\u0442\u044c \u0432\u0435\u0431-\u0432\u0435\u0440\u0441\u0438\u044e", "external"],
	"profile:pwa_install": ["\u0414\u043e\u0431\u0430\u0432\u0438\u0442\u044c \u043d\u0430 \u0440\u0430\u0431\u043e\u0447\u0438\u0439 \u0441\u0442\u043e\u043b", "download"],
	"profile:terms": ["\u041f\u043e\u043b\u044c\u0437\u043e\u0432\u0430\u0442\u0435\u043b\u044c\u0441\u043a\u043e\u0435 \u0441\u043e\u0433\u043b\u0430\u0448\u0435\u043d\u0438\u0435", "doc"],
	"navigation:dashboard": ["\u0413\u043b\u0430\u0432\u043d\u0430\u044f", "houseLine"],
	"navigation:buy": ["\u0422\u0430\u0440\u0438\u0444\u044b", "cartShopping"],
	"navigation:support": ["\u041f\u043e\u0434\u0434\u0435\u0440\u0436\u043a\u0430", "headphonesAlt"],
	"navigation:settings": ["\u041f\u0440\u043e\u0444\u0438\u043b\u044c", "userAlt"],
	"navigation:admin": ["\u0410\u0434\u043c\u0438\u043d", "grid"],
};

const ADMIN_LAYOUT_DEFAULTS = [
	["dashboard", "logo", 10, 60, 150, false, "center"],
	["dashboard", "username", 11, 100, 28, false, "center"],
	["dashboard", "plan_name", 12, 48, 32, false, "left"],
	["dashboard", "expires", 13, 48, 32, false, "right"],
	["dashboard", "traffic", 14, 48, 36, false, "left"],
	["dashboard", "devices", 15, 48, 36, false, "right"],
	["dashboard", "primary_action", 16, 100, 44, false, "center"],
	["dashboard", "secondary_action", 17, 100, 44, false, "center"],
	["buy", "plans", 0, 100, 330, false, "left"],
	["buy", "checkout", 1, 100, 250, false, "left"],
	...["1m", "1m_unlimited", "3m", "3m_unlimited", "6m", "6m_unlimited", "12m"].map((id, index) => ["buy", `plan_${id}`, 10 + index, 100, id === "12m" ? 92 : 112, false, "left"]),
	["buy", "summary", 20, 100, 72, false, "left"],
	["buy", "payment", 21, 100, 64, false, "left"],
	["buy", "promo", 22, 100, 72, false, "left"],
	["buy", "pay_button", 23, 100, 44, false, "left"],
	["support", "actions", 0, 100, 92, false, "left"],
	["support", "tabs", 1, 100, 44, false, "left"],
	["support", "tickets", 2, 100, 220, false, "left"],
	["support", "new_ticket", 10, 100, 64, false, "left"],
	["support", "faq", 11, 100, 64, false, "left"],
	["support", "tabs_detail", 12, 100, 44, false, "left"],
	["support", "tickets_detail", 13, 100, 220, false, "left"],
	...["server_status", "referrals", "reviews", "payments", "media", "login_methods", "news", "web_version", "pwa_install", "terms"].map((id, order) => ["profile", id, order, 100, 52, true, "left", PROFILE_DEFAULT_GROUPS[id]]),
	...["main", "purchases", "programs", "help", "account"].map((id, order) => ["profile", `group_${id}`, 20 + order, 100, 28, false, "left"]),
	...["dashboard", "buy", "support", "settings", "admin"].map((id, order) => ["navigation", id, order, 44, 38, true, "center"]),
].map(([area, id, order, width, height, framed, align, group]) => ({ area, id, order, visible: true, width, height, framed, align, offsetX: 0, offsetY: 0, ...(group ? { group } : {}) }));

function buildPreviewRuntimeSettings() {
	const features = Object.fromEntries(["stars", "trials", "google", "support", "reviews", "referrals", "promocodes", "media", "server_status", "web_version", "pwa_install"].map((name) => [name, true]));
	return {
		version: 7,
		maintenance: { enabled: false, titleRu: "\u0422\u0435\u0445\u043d\u0438\u0447\u0435\u0441\u043a\u0438\u0435 \u0440\u0430\u0431\u043e\u0442\u044b", textRu: "", reasonRu: "" },
		features,
		content: {
			brandName: "Link-Bot", adminContact: "", logoUrl: previewPayload.brand.logoUrl, startTextRu: "", startImage: "", copy: { ru: {} }, faq: { ru: [] }, links: deepClone(previewPayload.links), customLinks: [],
			verification: { text: "", banner: "", channelButton: { text: "Link-Bot", iconCustomEmojiId: "", style: "" }, confirmButton: { text: "Я подписался", iconCustomEmojiId: "", style: "" }, checkFailedText: "", notSubscribedText: "", verifiedText: "" },
			startMenu: { trialButton: { text: "Попробовать бесплатно", iconCustomEmojiId: "5276422526350681413", style: "" }, dashboardButton: { text: "Вход", iconCustomEmojiId: "5278413853577734640", style: "" }, plansButton: { text: "Тарифы", iconCustomEmojiId: "5206626000665868017", style: "" }, supportButton: { text: "Чат с поддержкой", iconCustomEmojiId: "5206222720416643915", style: "" } },
			commerce: { banner: "", tariffsText: "", paymentMethodsText: "", paymentReadyText: "", yookassaButton: { text: "СБП | Карта", iconCustomEmojiId: "5192678313415434135", style: "" }, cryptoButton: { text: "CryptoPay", iconCustomEmojiId: "5195058841988914267", style: "" }, starsButton: { text: "Telegram Stars", iconCustomEmojiId: "5242644275014951846", style: "" }, payButton: { text: "Оплатить", iconCustomEmojiId: "5206401524200145033", style: "" }, backButton: { text: "Назад", iconCustomEmojiId: "5877629862306385808", style: "" }, successText: "", successBanner: "", successButton: { text: "Личный кабинет", iconCustomEmojiId: "5278413853577734640", style: "" } },
		},
		appearance: { backgroundMode: "animated", compact: true, showFrames: true, colors: { background: "#000000", surface: "#08090c", surfaceStrong: "#0b0d12", text: "#f3f3f3", muted: "#a0a0a0", border: "#2a2d33", button: "#0b0d12", buttonText: "#f3f3f3", icon: "#f3f3f3", accent: "#ba173d", success: "#2da44e", danger: "#f85149", unlimitedBadge: "#949494", gridBackground: "#000000", gridLine: "#ffffff", gridGlowLeft: "#ffffff", gridGlowRight: "#ffffff", grid2Background: "#000000", grid2Line: "#ffffff", grid2Glow: "#ff0000", waveBackground: "#000000", waveDot: "#ebebeb" } },
		layout: { elements: deepClone(ADMIN_LAYOUT_DEFAULTS), planColumns: 2, logoWidth: 188 },
		plans: previewPayload.plans.map((plan) => ({ id: plan.id, enabled: true, months: plan.months, titleRu: `${plan.months} ${plan.months === 1 ? "\u043c\u0435\u0441\u044f\u0446" : plan.months < 5 ? "\u043c\u0435\u0441\u044f\u0446\u0430" : "\u043c\u0435\u0441\u044f\u0446\u0435\u0432"}`, titleEn: `${plan.months} month${plan.months === 1 ? "" : "s"}`, priceRub: plan.priceRub, priceStars: plan.priceStars, trafficGb: Math.round(Number(plan.trafficLimitBytes || 0) / (1024 ** 3)), unlimitedTraffic: Number(plan.trafficLimitBytes || 0) <= 0, deviceLimit: plan.deviceLimitCount, wide: Boolean(plan.wide), internalSquadUuids: [], externalSquadUuid: "" })),
		trial: { enabled: true, days: 3, trafficGb: 10, unlimitedTraffic: false, deviceLimit: 5, internalSquadUuids: [], externalSquadUuid: "", trafficResetStrategy: "MONTH", tag: "" },
	};
}

const state = {
  locale: "ru",
  data: null,
	publicSettings: null,
	maintenance: null,
  loading: true,
  refreshing: false,
  subscriptionGate: null,
  error: "",
  currentPage: readSetting(STORAGE_KEYS.page, "dashboard"),
  sidebarOpen: false,
  payModalOpen: false,
  paymentLaunchModalOpen: false,
  paymentLaunchURL: "",
  paymentLaunchPurchaseId: 0,
  supportTab: "open",
  supportComposeOpen: false,
  supportThreadOpen: false,
  devicesModalOpen: false,
  activeSupportTicketId: 0,
  activeSupportThread: null,
  supportBusy: "",
  deviceBusyHwid: "",
  supportDraftSubject: "",
  supportDraftMessage: "",
  supportReplyDraft: "",
  paymentAgreementAccepted: false,
  promoCodeDraft: "",
  appliedPromo: null,
  promoValidation: null,
  promoBusy: "",
  loginMethodBusy: "",
  reviewComposeOpen: false,
  reviewDetailOpen: false,
  activeReviewId: 0,
  reviewDraftRating: 0,
  reviewDraftComment: "",
  reviewBusy: "",
  adminSection: "home",
	adminLayoutEditing: false,
	adminPlanEditing: false,
	adminPlanEditorModalOpen: false,
	adminPlanEditingID: "",
	adminPlanFormDraft: null,
	adminPlanBaseline: null,
  adminPromoCodeDraft: "",
  adminPromoDiscountDraft: "",
  adminPromoLimitDraft: "",
  adminPromoExpiresDraft: "",
  adminSubscriptionQuery: "",
  adminSubscriptionTargetTelegramID: "",
  adminSubscriptionResult: null,
  adminBusy: "",
	adminBroadcast: null,
	adminBroadcastButtonsDraft: [],
	adminBroadcastButtonsDirty: false,
	adminBroadcastBusy: "",
	adminBroadcastConfirmOpen: false,
	adminIntegrationOpen: "",
	adminIntegrationDrafts: {},
	adminIntegrationBusy: "",
	adminSettingsDraft: null,
	adminSettingsDirty: false,
	adminJSONDrafts: {},
	adminContentSection: "start",
	adminContentTabsScrollLeft: 0,
	adminLayoutCategory: "dashboard",
	adminLayoutSelection: "dashboard:logo",
  selectedFaqIndex: -1,
  selectedPlatform: "windows",
  selectedPlanId: "",
  selectedPlanMonths: null,
  paymentMethod: readSetting(STORAGE_KEYS.payMethod, ""),
  serverFilter: "all",
  installGuidePlatform: getDefaultInstallPlatform(),
  busyMethod: "",
  theme: "dark",
  animatePageEntry: false,
  scrollTopByPage: {},
  supportThreadScrollTop: 0,
};

state.currentPage = normalizePage(state.currentPage);

let pageAnimationEnabled = false;
let supportListPollTimer = 0;
let supportThreadPollTimer = 0;
let previousActiveModalName = "";
let animatedModalName = "";
let closingModalName = "";
let closingModalTimer = 0;
let previousBottomNavIndex = -1;
let pendingBottomNavAnimation = null;
let promoApplyTimer = 0;
let promoApplySeq = 0;
let adminLayoutPointer = null;
let adminLayoutPointerFrame = 0;
let adminLayoutPendingPoint = null;
let suppressNextLayoutClick = false;
let adminProfilePointer = null;
let adminPlanPointer = null;
let adminBroadcastPollTimer = 0;
const ADMIN_LAYOUT_GRID_SIZE = 8;
const ADMIN_LAYOUT_SNAP_THRESHOLD = 6;
const MODAL_CLOSE_MS = 220;

const particleEngine = createParticleEngine();

function t() {
	const base = copybook[state.locale] || copybook.ru;
	const override = getRuntimeSettings()?.content?.copy?.[state.locale] || {};
	return { ...base, ...override };
}

function getRuntimeSettings() {
	if ((state.adminLayoutEditing || state.adminPlanEditing) && state.adminSettingsDraft) return state.adminSettingsDraft;
	return state.data?.runtime || state.publicSettings || state.adminSettingsDraft || null;
}

function featureEnabled(name) {
	const features = getRuntimeSettings()?.features;
	return !features || features[name] !== false;
}

function pageFeatureEnabled(page) {
	const featureByPage = {
		support: "support",
		reviews: "reviews",
		referrals: "referrals",
		servers: "server_status",
		media: "media",
		"login-methods": "google",
	};
	const feature = featureByPage[page];
	if (page === "buy") return true;
	return !feature || featureEnabled(feature);
}

function getLayoutElements(area) {
	const items = getRuntimeSettings()?.layout?.elements;
	if (!Array.isArray(items)) return [];
	return items
		.filter((item) => item?.area === area && item.visible !== false)
		.slice()
		.sort((left, right) => Number(left.order || 0) - Number(right.order || 0));
}

function getLayoutElement(area, id) {
	return (getRuntimeSettings()?.layout?.elements || []).find((item) => item?.area === area && item?.id === id) || null;
}

function hasStoredLayoutPosition(item) {
	return item?.positionX !== undefined
		&& item?.positionX !== null
		&& item?.positionY !== undefined
		&& item?.positionY !== null
		&& Number.isFinite(Number(item.positionX))
		&& Number.isFinite(Number(item.positionY));
}

function runtimeLayoutStyle(item, area = item?.area) {
	const isNavigation = area === "navigation";
	const width = isNavigation
		? Math.max(28, Math.min(100, Number(item?.width || 44)))
		: Math.max(10, Math.min(150, Number(item?.width || 100)));
	const height = isNavigation
		? Math.max(24, Math.min(96, Number(item?.height || 38)))
		: Math.max(20, Math.min(720, Number(item?.height || 52)));
	const positioned = hasStoredLayoutPosition(item);
	const offsetX = positioned ? 0 : Math.max(-1000, Math.min(1000, Number(item?.offsetX || 0)));
	const offsetY = positioned ? 0 : Math.max(-1000, Math.min(1000, Number(item?.offsetY || 0)));
	return `--runtime-width:${width}${isNavigation ? "px" : "%"};--runtime-height:${height}px;--runtime-x:${offsetX}px;--runtime-y:${offsetY}px`;
}

function getLayoutElementIndex(area, id) {
	return state.adminSettingsDraft?.layout?.elements?.findIndex((item) => item?.area === area && item?.id === id) ?? -1;
}

function renderLayoutEditHandles() {
	if (!state.adminLayoutEditing) return "";
	return `<span class="layout-editable__move" aria-hidden="true">${icon("move")}</span><span class="layout-editable__resize" data-ui-resize-handle aria-hidden="true">${icon("resize")}</span>`;
}

function renderLayoutDetail(area, id, content, className = "") {
	const item = getLayoutElement(area, id);
	if (item?.visible === false) return "";
	const fallback = ADMIN_LAYOUT_DEFAULTS.find((entry) => entry.area === area && entry.id === id) || { area, id, width: 100, height: 52, align: "left", offsetX: 0, offsetY: 0 };
	const layout = item || fallback;
	const index = getLayoutElementIndex(area, id);
	const editable = state.adminLayoutEditing && area === "dashboard" && index >= 0;
	const selected = editable && state.adminLayoutSelection === `${area}:${id}`;
	const key = escapeAttribute(`${area}:${id}`);
	return `<div class="runtime-detail-item ${className} ${editable ? "layout-editable" : ""} ${selected ? "is-selected" : ""}" data-runtime-align="${escapeAttribute(layout.align || "left")}" data-runtime-layout-key="${key}" style="${escapeAttribute(runtimeLayoutStyle(layout, area))}" ${editable ? `data-ui-layout-index="${index}" data-layout-edit-key="${key}" tabindex="0"` : ""}>${content}${renderLayoutEditHandles()}</div>`;
}

function layoutEditableData(area, id) {
	if (area === "dashboard" && ["brand", "subscription", "actions"].includes(id)) {
		return { className: "", attributes: "", handles: "" };
	}
	const runtimeItem = getLayoutElement(area, id);
	if (!runtimeItem) return { className: "", attributes: "", handles: "" };
	const key = escapeAttribute(`${area}:${id}`);
	const runtimeAttributes = `data-runtime-layout-key="${key}"`;
	if (!state.adminLayoutEditing || area !== "dashboard") return { className: "", attributes: runtimeAttributes, handles: "" };
	const index = getLayoutElementIndex(area, id);
	if (index < 0) return { className: "", attributes: runtimeAttributes, handles: "" };
	return {
		className: `layout-editable ${state.adminLayoutSelection === `${area}:${id}` ? "is-selected" : ""}`,
		attributes: `${runtimeAttributes} data-ui-layout-index="${index}" data-layout-edit-key="${key}" tabindex="0"`,
		handles: renderLayoutEditHandles(),
	};
}

function renderRuntimeLayoutArea(area, blocks, className = "") {
	let entries = getLayoutElements(area).filter((item) => Object.prototype.hasOwnProperty.call(blocks, item.id));
	if (!entries.length) {
		entries = Object.keys(blocks).map((id, order) => ({ id, area, order, visible: true, width: 100, height: 52, framed: false, align: "left", offsetX: 0, offsetY: 0 }));
	}
	return `<div class="runtime-layout-area runtime-layout-area--${escapeAttribute(area)} ${escapeAttribute(className)}">
		${entries.map((item) => {
			const editable = layoutEditableData(area, item.id);
			return `<div class="runtime-layout-item ${item.framed ? "runtime-layout-item--framed" : ""} ${editable.className}" ${editable.attributes} data-runtime-area="${escapeAttribute(area)}" data-runtime-id="${escapeAttribute(item.id)}" data-runtime-align="${escapeAttribute(item.align || "left")}" style="${escapeAttribute(runtimeLayoutStyle(item, area))}">${blocks[item.id]}${editable.handles}</div>`;
		}).join("")}
	</div>`;
}

function deepClone(value) {
	return value == null ? value : JSON.parse(JSON.stringify(value));
}

function syncAdminSettingsDraft(force = false) {
	const source = state.data?.admin?.settings || state.data?.runtime;
	if (!source || (state.adminSettingsDirty && !force)) return;
	state.adminSettingsDraft = deepClone(source);
	seedEditableCopy(state.adminSettingsDraft);
	state.adminSettingsDirty = false;
	state.adminJSONDrafts = {};
}

function seedEditableCopy(settings) {
	if (!settings?.content) return;
	const existing = settings.content.copy?.ru || {};
	const defaults = Object.fromEntries(Object.entries(copybook.ru || {}).filter(([, value]) => typeof value === "string"));
	settings.content.copy = { ru: { ...defaults, ...existing } };
	delete settings.content.startTextEn;
	if (!settings.content.faq) settings.content.faq = { ru: [] };
	delete settings.content.faq.en;
	if (!Array.isArray(settings.content.faq.ru) || !settings.content.faq.ru.length) {
		settings.content.faq.ru = FAQS.ru.map(([question, answer]) => ({ question, answer }));
	}
	if (!settings.content.subscriptionReminderButton) {
		settings.content.subscriptionReminderButton = { iconCustomEmojiId: "", style: "" };
	}
	if (!settings.content.support) {
		settings.content.support = {
			newTicketText: "🆕 <b>Новое обращение #{ticket_id}</b>\n\n👤 <b>Пользователь:</b> {name}\n🔗 <b>Username:</b> {username}\n💎 <b>Подписка:</b> {subscription}\n\n💬 <b>Сообщение:</b>\n{message}",
			customerReplyText: "📩 <b>Обращение #{ticket_id}</b>\nПолучен новый ответ от пользователя.\n\n👤 <b>Пользователь:</b> {name}\n🔗 <b>Username:</b> {username}\n💎 <b>Подписка:</b> {subscription}\n\n{message}",
			adminReplyText: "📬 <b>Обращение #{ticket_id}</b>\nПоддержка ответила на ваше сообщение.\n\n{message}",
			closedText: "💌 <b>Обращение #{ticket_id} закрыто.</b>\nИстория переписки доступна в Mini app.",
			openButton: { text: "Открыть Mini app", iconCustomEmojiId: "", style: "" },
		};
	}
	(settings.content.customLinks || []).forEach((item) => {
		delete item.labelEn;
		delete item.hintEn;
	});
	if (settings.maintenance) {
		delete settings.maintenance.titleEn;
		delete settings.maintenance.textEn;
		delete settings.maintenance.reasonEn;
	}
}

function setDeepValue(root, path, value) {
	const parts = String(path || "").split(".").filter(Boolean);
	if (!root || !parts.length) return;
	let cursor = root;
	for (let index = 0; index < parts.length - 1; index += 1) {
		const key = /^\d+$/.test(parts[index]) ? Number(parts[index]) : parts[index];
		const nextKey = parts[index + 1];
		if (cursor[key] == null) cursor[key] = /^\d+$/.test(nextKey) ? [] : {};
		cursor = cursor[key];
	}
	const finalKey = /^\d+$/.test(parts.at(-1)) ? Number(parts.at(-1)) : parts.at(-1);
	cursor[finalKey] = value;
}

function getDeepValue(root, path, fallback = "") {
	const parts = String(path || "").split(".").filter(Boolean);
	let cursor = root;
	for (const part of parts) {
		const key = /^\d+$/.test(part) ? Number(part) : part;
		if (cursor == null || !(key in cursor)) return fallback;
		cursor = cursor[key];
	}
	return cursor ?? fallback;
}

function supportText() {
  if (state.locale === "en") {
    return {
      open: "Open",
      history: "History",
      faq: "Frequently asked questions",
      faqHint: "Quick answers before creating a ticket",
      adminHint: "New tickets and replies appear here automatically.",
      noOpenTitle: "No open tickets",
      noOpenHint: "When a ticket appears, it will show up here.",
      noHistoryTitle: "No ticket history",
      noHistoryHint: "Closed tickets will stay here for future reference.",
      createTitle: "Create ticket",
      subject: "Subject",
      subjectPlaceholder: "Briefly describe the issue",
      message: "Message",
      messagePlaceholder: "Describe your problem or question in detail...",
      send: "Send",
      ticketFallback: (id) => `Ticket #${id}`,
      you: "You",
      admin: "Support",
      closeTicket: "Close ticket",
      closed: "Ticket closed",
      closedHint: "The conversation is available in history.",
      replyPlaceholder: "Write a reply...",
      ticketCreated: "Ticket sent",
      replySent: "Reply sent",
      ticketClosedToast: "Ticket closed",
      loadingThread: "Loading conversation...",
      customer: "Username",
      subscription: "Subscription",
      unread: "New",
    };
  }

  const overrides = getRuntimeSettings()?.content?.copy?.ru || {};
  const editable = (key, fallback) => String(overrides[key] || fallback);
  return {
    open: editable("supportOpenTab", "Открытые"),
    history: editable("supportHistoryTab", "История"),
    faq: editable("supportFaqTitle", "Часто задаваемые вопросы"),
    faqHint: editable("supportFaqHint", "Быстрые ответы перед созданием обращения"),
    adminHint: editable("supportAdminHint", "Новые обращения и ответы появляются здесь автоматически."),
    noOpenTitle: editable("supportNoOpenTitle", "Нет открытых обращений"),
    noOpenHint: editable("supportNoOpenHint", "Когда появятся тикеты, они отобразятся здесь."),
    noHistoryTitle: editable("supportNoHistoryTitle", "История обращений пуста"),
    noHistoryHint: editable("supportNoHistoryHint", "Закрытые обращения будут храниться здесь."),
    createTitle: editable("supportCreateTitle", "Создать обращение"),
    subject: editable("supportSubjectLabel", "Тема"),
    subjectPlaceholder: editable("supportSubjectPlaceholder", "Кратко опишите проблему"),
    message: editable("supportMessageLabel", "Сообщение"),
    messagePlaceholder: editable("supportMessagePlaceholder", "Опишите вашу проблему или вопрос подробно..."),
    send: editable("supportSendButton", "Отправить"),
    ticketFallback: (id) => `Обращение #${id}`,
    you: "Вы",
    admin: "Поддержка",
    closeTicket: editable("supportCloseButton", "Закрыть обращение"),
    closed: editable("supportClosedTitle", "Обращение закрыто"),
    closedHint: editable("supportClosedHint", "История переписки сохранена в разделе истории."),
    replyPlaceholder: editable("supportReplyPlaceholder", "Напишите сообщение..."),
    ticketCreated: editable("ticketCreated", "Обращение отправлено"),
    replySent: editable("replySent", "Ответ отправлен"),
    ticketClosedToast: editable("ticketClosedToast", "Обращение закрыто"),
    loadingThread: editable("supportLoadingThread", "Загружаем переписку..."),
      customer: "Имя пользователя",
      subscription: "Подписка",
      unread: "Новое",
  };
}

function deviceText() {
  if (state.locale === "en") {
    return {
      title: "Devices",
      connected: (used, limit) => `Connected: ${used}/${limit <= 0 ? "\u221E" : limit} devices`,
      added: "Added:",
      emptyTitle: "No connected devices",
      emptyHint: "Devices from your subscription will appear here.",
      deleted: "Device removed",
    };
  }

  return {
    title: "Устройства",
    connected: (used, limit) => `Подключено: ${used}/${limit <= 0 ? "\u221E" : limit} устройств`,
    added: "Добавлено:",
    emptyTitle: "Нет подключённых устройств",
    emptyHint: "Здесь появятся устройства из вашей подписки.",
    deleted: "Устройство удалено",
  };
}

function reviewsText() {
  if (state.locale === "en") {
    return {
      leaveReview: "Leave a review",
      thanksTitle: "Your review is published",
      thanksHint: "One account can leave only one review.",
      viewMine: "View my review",
      emptyTitle: "No reviews yet",
      emptyHint: "Be the first to rate Link-Bot.",
      ratingLabel: "Rating",
      commentLabel: "Comment",
      commentPlaceholder: "Tell others what you liked or what can be improved...",
      submit: "Submit review",
      ratingRequired: "Choose a rating from 1 to 5",
      alreadyReviewed: "You can leave only one review",
      rewardHint: "After submission you receive +2 days and +20 GB as a gift.",
      rewardToast: "Review reward received: +2 days and +20 GB",
      reviewsCount: (count) => `${formatNumber(count, "en")} review${count === 1 ? "" : "s"}`,
      mine: "Mine",
      delete: "Delete review",
      deleteSuccess: "Review deleted",
    };
  }

  return {
    leaveReview: "Оставить отзыв",
    thanksTitle: "Ваш отзыв уже опубликован",
    thanksHint: "Один пользователь может оставить только один отзыв.",
    viewMine: "Посмотреть мой отзыв",
    emptyTitle: "Пока нет отзывов",
    emptyHint: "Станьте первым и оцените Link-Bot.",
    ratingLabel: "Оценка",
    commentLabel: "Комментарий",
    commentPlaceholder: "Напишите, что вам понравилось или что можно улучшить...",
    submit: "Отправить отзыв",
    ratingRequired: "Выберите оценку от 1 до 5",
    alreadyReviewed: "Отзыв можно оставить только один раз",
    rewardHint: "После отправки вы получите подарок: +2 дня и +20 ГБ.",
    rewardToast: "Подарок за отзыв получен: +2 дня и +20 ГБ",
    reviewsCount: (count) => `${formatNumber(count, "ru")} ${pluralizeRu(count, ["отзыв", "отзыва", "отзывов"])}`,
    mine: "Мой отзыв",
    delete: "Удалить отзыв",
    deleteSuccess: "Отзыв удалён",
  };
}

async function boot() {
  initTelegram();
  particleEngine.start();
  state.theme = "dark";
  writeSetting(STORAGE_KEYS.theme, "dark");
	await loadPublicConfig();
	applyAppearance();
  const paymentReturn = Boolean(getPaymentReturnState());
  state.currentPage = getEntryPage();
  state.sidebarOpen = false;
  state.payModalOpen = false;
  state.paymentLaunchModalOpen = false;
  state.paymentLaunchURL = "";
  state.paymentLaunchPurchaseId = 0;
  state.devicesModalOpen = false;
  state.reviewComposeOpen = false;
  state.reviewDetailOpen = false;
  state.activeReviewId = 0;
  state.reviewDraftRating = 0;
  state.reviewDraftComment = "";
  state.reviewBusy = "";
  state.deviceBusyHwid = "";
  state.supportComposeOpen = false;
  state.supportThreadOpen = false;
  state.activeSupportTicketId = 0;
  state.activeSupportThread = null;
  state.supportReplyDraft = "";
  writeSetting(STORAGE_KEYS.page, state.currentPage);
  await refreshDashboard({ initial: true, silent: paymentReturn });
  await handlePostBootstrapFlow();
}

async function loadPublicConfig() {
	if (previewMode) {
		state.publicSettings = null;
		return;
	}
	try {
		const response = await getJSON("/api/mini-app/public-config");
		state.publicSettings = response?.data || null;
	} catch {
		state.publicSettings = null;
	}
}

function initTelegram() {
  syncAppViewportHeight();
  if (!initTelegram.viewportResizeBound) {
    window.addEventListener("resize", syncAppViewportHeight, { passive: true });
    initTelegram.viewportResizeBound = true;
  }
  if (!tg) return;
  tg.ready();
  tg.expand();
  syncAppViewportHeight();
  if (typeof tg.onEvent === "function" && !initTelegram.viewportEventBound) {
    tg.onEvent("viewportChanged", syncAppViewportHeight);
    initTelegram.viewportEventBound = true;
  }
  if (tg.BackButton && !initTelegram.backButtonBound) {
    tg.BackButton.onClick(handleNativeBackButton);
    initTelegram.backButtonBound = true;
  }
}

function syncAppViewportHeight() {
  const telegramHeight = Number(tg?.viewportHeight || tg?.viewportStableHeight || 0);
  const fallbackHeight = Number(window.innerHeight || document.documentElement.clientHeight || 0);
  const nextHeight = telegramHeight > 0 ? telegramHeight : fallbackHeight;
  if (nextHeight > 0) {
    document.documentElement.style.setProperty("--app-viewport-height", `${Math.round(nextHeight)}px`);
  }
}

async function handlePostBootstrapFlow() {
  if (!state.data) return;

  const paymentReturn = getPaymentReturnState();
  if (paymentReturn) {
    clearPendingPayment();
    clearPaymentReturnState();
    moveToDashboard();
    if (paymentReturn.status === "paid") {
      showToast(t().paymentSuccess, "success");
      return;
    }
    showToast(t().paymentCancelled, "danger");
    return;
  }

  clearPendingPayment();

	if (urlParams.get("admin") === "broadcast" && isAdminUser()) {
		state.currentPage = "admin";
		state.adminSection = "broadcast";
		writeSetting(STORAGE_KEYS.page, "admin");
		render();
		await refreshAdminBroadcast({ forceButtons: true });
		return;
	}

	const entryPromoCode = normalizePromoCodeValue(urlParams.get("promo") || "");
	if (entryPromoCode) {
		state.currentPage = "buy";
		state.promoCodeDraft = entryPromoCode;
		writeSetting(STORAGE_KEYS.page, "buy");
		render();
		await applyPromoCode({ silent: true });
	}
}

async function refreshDashboard({ initial = false, silent = false, forceSubscriptionCheck = false } = {}) {
  if (!silent) {
    if (!state.data || initial) state.loading = true;
    else state.refreshing = true;
    render();
  }

  try {
    if (!hasAuth() && previewMode) {
      state.data = deepClone(previewPayload);
      if (previewAdminMode) {
        const runtime = buildPreviewRuntimeSettings();
        state.data.support.isAdmin = true;
        state.data.runtime = runtime;
        state.data.admin = { settings: runtime, events: [] };
		state.adminSettingsDraft = deepClone(runtime);
		seedEditableCopy(state.adminSettingsDraft);
		state.currentPage = "dashboard";
        state.adminSection = "layout";
		state.adminLayoutEditing = true;
      }
      state.locale = pickLocale(previewPayload.user.languageCode);
      ensureSelections();
      state.subscriptionGate = null;
      state.error = "";
      return;
    }

    if (!hasAuth()) {
      state.data = null;
      state.subscriptionGate = null;
      state.error = "";
      return;
    }

    const bootstrapHeaders = {};
    if (initial) bootstrapHeaders["X-Bootstrap-Mode"] = "fast";
    if (forceSubscriptionCheck) bootstrapHeaders["X-Force-Channel-Check"] = "1";
    const response = await post("/api/mini-app/bootstrap", null, bootstrapHeaders);
    state.data = response.data;
		state.publicSettings = response.data?.runtime || state.publicSettings;
		state.maintenance = null;
    state.locale = pickLocale(response.data.user.languageCode);
		syncAdminSettingsDraft();
    ensureSelections();
    state.subscriptionGate = null;
    state.error = "";
    if (initial) scheduleDashboardHydration();
  } catch (error) {
    if ((error?.code === "unauthorized" || error?.code === "google_not_linked") && !tg?.initData) {
      const shouldNotifyGoogleLink = error?.code === "google_not_linked" && Boolean(readGoogleAuth());
      clearBrowserTelegramAuth();
      clearGoogleAuth();
      state.data = null;
      state.subscriptionGate = null;
      state.error = "";
      if (shouldNotifyGoogleLink) showToast(error.message || mapApiErrorMessage(error.code, error.rawMessage), "danger");
      return;
    }

    if (error?.code === "channel_subscription_required" || error?.code === "channel_subscription_check_failed") {
      state.data = null;
      state.error = "";
      state.subscriptionGate = {
        ...(error?.meta || {}),
        reason: error.code,
      };
      return;
    }

		if (error?.code === "maintenance") {
			state.data = null;
			state.error = "";
			state.subscriptionGate = null;
			state.maintenance = error?.meta || getRuntimeSettings()?.maintenance || {};
			return;
		}

    const message = error?.message || t().errorTitle;
    if (!state.data || initial) state.error = message;
    else showToast(message);
  } finally {
    state.loading = false;
    state.refreshing = false;
    render();
  }
}

function getBrowserAuthHeaders() {
  const telegramLogin = readBrowserTelegramAuth();
  if (telegramLogin) return { "X-Telegram-Login-Data": telegramLogin };

  const googleToken = readGoogleAuth();
  if (googleToken) return { "X-Google-ID-Token": googleToken };

  return {};
}

function requestTimeoutForURL(url) {
  const path = String(url || "");
  if (path.includes("/api/mini-app/bootstrap")) return 30000;
  if (path.includes("/api/mini-app/purchase")) return 45000;
  return 15000;
}

async function post(url, body, extraHeaders = null) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), requestTimeoutForURL(url));
  try {
    const response = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Telegram-Init-Data": tg?.initData || "",
        ...(tg?.initData ? {} : getBrowserAuthHeaders()),
        ...(extraHeaders || {}),
      },
      body: JSON.stringify(body || {}),
      signal: controller.signal,
    });
    const payload = await response.json().catch(() => null);
    if (!response.ok || !payload?.ok) {
      const err = new Error(mapApiErrorMessage(payload?.error?.code, payload?.error?.message || "Request failed"));
      err.code = payload?.error?.code || "";
      err.meta = payload?.error?.meta || null;
      err.rawMessage = payload?.error?.message || "";
      throw err;
    }
    return payload;
  } catch (error) {
    if (error?.name === "AbortError") throw new Error(t().timeout);
    throw error;
  } finally {
    clearTimeout(timeout);
  }
}

async function getJSON(url) {
	const controller = new AbortController();
	const timeout = setTimeout(() => controller.abort(), requestTimeoutForURL(url));
	try {
		const response = await fetch(url, { method: "GET", cache: "no-store", signal: controller.signal });
		const payload = await response.json().catch(() => null);
		if (!response.ok || !payload?.ok) throw new Error(payload?.error?.message || "Request failed");
		return payload;
	} finally {
		clearTimeout(timeout);
	}
}

function mapApiErrorMessage(code, fallback) {
  const copy = t();
  const messages = {
    agreement_required: copy.agreementRequired,
    unsupported_payment_method: copy.paymentUnavailable,
    unsupported_plan: state.locale === "en" ? "This plan is unavailable" : "Этот тариф недоступен",
    promo_code_required: copy.promoCodeRequired,
    promo_not_found: copy.promoInvalid,
    promo_expired: copy.promoExpired,
    promo_inactive: copy.promoInactive,
    promo_limit_reached: copy.promoLimitReached,
    promo_already_used: copy.promoAlreadyUsed,
    promo_pending: copy.promoPending,
    promo_invalid_format: copy.promoInvalidFormat,
    promo_invalid_discount: copy.promoInvalidDiscount,
    promo_invalid_expiry: copy.promoInvalidExpiry,
    promo_invalid_limit: copy.promoInvalidLimit,
    promo_already_exists: copy.promoAlreadyExists,
    promo_create_failed: copy.promoCreateFailed,
    promo_delete_failed: copy.promoDeleteFailed,
    promo_failed: copy.promoUnavailable,
    promo_unavailable: copy.promoUnavailable,
    invalid_subscription_query: state.locale === "en" ? "Enter a panel ID or subscription username" : "Введите ID из панели или имя подписки",
    subscription_not_found: state.locale === "en" ? "Subscription not found" : "Подписка не найдена",
    target_not_registered: state.locale === "en" ? "The new account must start the bot with /start first" : "Новый аккаунт должен сначала запустить бота командой /start",
    subscription_rebind_failed: state.locale === "en" ? "Could not rebind the subscription" : "Не удалось перепривязать подписку",
    google_not_configured: googleAuthCopy().loginUnavailable,
    google_not_linked: state.locale === "en" ? "Link Gmail in the mini app first" : "\u0421\u043d\u0430\u0447\u0430\u043b\u0430 \u043f\u0440\u0438\u0432\u044f\u0436\u0438\u0442\u0435 Gmail \u0432 mini app",
    google_already_linked: state.locale === "en" ? "This Gmail is already linked to another account" : "\u042d\u0442\u0430 Gmail-\u043f\u043e\u0447\u0442\u0430 \u0443\u0436\u0435 \u043f\u0440\u0438\u0432\u044f\u0437\u0430\u043d\u0430 \u043a \u0434\u0440\u0443\u0433\u043e\u043c\u0443 \u0430\u043a\u043a\u0430\u0443\u043d\u0442\u0443",
    google_invalid: googleAuthCopy().loginFailed,
    google_link_failed: googleAuthCopy().linkFailed,
    too_many_requests: state.locale === "en" ? "Too many requests, please slow down a bit" : "Слишком много запросов, попробуйте чуть позже",
  };
  return messages[code] || fallback || t().errorTitle;
}

function runtimePlanToPayload(plan, index = 0) {
	const months = Math.max(0, Number(plan?.months || 0));
	const trafficGb = Math.max(0, Number(plan?.trafficGb || 0));
	return {
		id: String(plan?.id || `draft_${index}`),
		months,
		priceRub: Math.max(0, Number(plan?.priceRub || 0)),
		priceStars: Math.max(0, Number(plan?.priceStars || 0)),
		trafficLimitBytes: trafficGb > 0 ? trafficGb * (1024 ** 3) : 0,
		deviceLimitCount: Math.max(0, Number(plan?.deviceLimit || 0)),
		variant: trafficGb === 0 || plan?.unlimitedTraffic ? "unlimited" : "regular",
		wide: Boolean(plan?.wide),
		titleRu: String(plan?.titleRu || ""),
		titleEn: String(plan?.titleEn || ""),
		enabled: plan?.enabled !== false,
		adminDraft: true,
	};
}

function getDisplayedPlans() {
	if (state.adminPlanEditing) {
		return (state.adminSettingsDraft?.plans || []).map(runtimePlanToPayload);
	}
	return state.data?.plans || [];
}

function ensureSelections() {
  const plans = getDisplayedPlans();
  if (!plans.length) {
    state.selectedPlanId = "";
    state.selectedPlanMonths = null;
  } else {
    let selected = plans.find((plan) => planKey(plan) === state.selectedPlanId);
    if (!selected && state.selectedPlanMonths) {
      selected = plans.find((plan) => Number(plan.months) === Number(state.selectedPlanMonths) && String(plan.variant || "regular") === "regular");
    }
    if (!selected) selected = plans.find((plan) => plan.recommended) || plans[0];
    state.selectedPlanId = planKey(selected);
    state.selectedPlanMonths = selected.months;
  }

  const methods = getAvailableMethods(getSelectedPlan()).map((item) => item.id);
  if (!methods.length) state.paymentMethod = "";
  else if (!methods.includes(state.paymentMethod)) {
    state.paymentMethod = methods[0];
    writeSetting(STORAGE_KEYS.payMethod, state.paymentMethod);
  }

  if (state.currentPage === "admin" && !isAdminUser()) {
    state.currentPage = "dashboard";
    writeSetting(STORAGE_KEYS.page, state.currentPage);
  }
	if (state.currentPage !== "admin" && !pageFeatureEnabled(state.currentPage)) {
		state.currentPage = "dashboard";
		writeSetting(STORAGE_KEYS.page, state.currentPage);
	}
}

function isAdminUser() {
  return Boolean(state.data?.support?.isAdmin);
}

function getBottomNavPages() {
	const pages = state.adminLayoutEditing ? ["dashboard", "settings"] : BOTTOM_NAV;
	return pages.filter((page) => PAGES.includes(page) && (page !== "admin" || isAdminUser()) && pageFeatureEnabled(page));
}

function render({ preserveScroll = true, scrollTop = null } = {}) {
  const nextScrollTop = preserveScroll ? getCurrentScrollTop() : scrollTop;
  const supportThreadScrollState = captureSupportThreadScrollState();
  const activeModalName = getActiveModalName();
  animatedModalName = activeModalName && activeModalName !== previousActiveModalName ? activeModalName : "";
  previousActiveModalName = activeModalName;
  const modalOpen = state.supportComposeOpen || state.supportThreadOpen || state.devicesModalOpen || state.payModalOpen || state.paymentLaunchModalOpen || state.reviewComposeOpen || state.reviewDetailOpen || state.adminPlanEditorModalOpen;
  document.body.classList.toggle("has-open-modal", modalOpen);
  document.body.classList.toggle("is-install-guide", isInstallGuideMode());
	document.body.classList.toggle("is-layout-editing", state.adminLayoutEditing);
	document.body.classList.toggle("is-plan-editing", state.adminPlanEditing);
	syncAdminLayoutBackgroundAnimation();
  pageAnimationEnabled = Boolean(state.animatePageEntry);
  state.animatePageEntry = false;
  applyAppearance();
  if (isInstallGuideMode()) {
    app.innerHTML = renderInstallGuidePage();
    bindRootActions();
    return;
  }
  if (state.loading && !state.data) return void (app.innerHTML = renderStateScreen("loading"));
	const publicMaintenance = !hasAuth() && Boolean(getRuntimeSettings()?.maintenance?.enabled) ? getRuntimeSettings().maintenance : null;
	if (!state.data && (state.maintenance || publicMaintenance)) {
		app.innerHTML = renderStateScreen("maintenance", "", state.maintenance || publicMaintenance);
		return bindRootActions();
	}
  if (!state.data && !hasAuth() && !previewMode) {
    app.innerHTML = renderStateScreen("telegram");
    bindRootActions();
    mountTelegramLoginWidget();
    mountGoogleLoginWidgets();
    return;
  }
  if (state.subscriptionGate && !state.data) {
    app.innerHTML = renderStateScreen("subscription", "", state.subscriptionGate);
    return bindRootActions();
  }
  if (state.error && !state.data) {
    app.innerHTML = renderStateScreen("error", state.error);
    return bindRootActions();
  }
  if (!state.data) {
    app.innerHTML = renderStateScreen("telegram");
    bindRootActions();
    mountTelegramLoginWidget();
    mountGoogleLoginWidgets();
    return;
  }

	app.innerHTML = `
    <div class="app-shell ${state.adminLayoutEditing ? "app-shell--layout-editor" : ""}">
      <div class="page-scroll">${renderPages()}</div>
      ${state.adminPlanEditing ? "" : renderBottomNav()}
		${state.adminLayoutEditing ? renderAdminSaveBar("admin-save-bar--layout-editor") : ""}
		${state.adminPlanEditing ? renderAdminPlanSaveBar() : ""}
      ${isModalVisible("support-compose", state.supportComposeOpen) ? renderSupportComposerModal() : ""}
      ${isModalVisible("support-thread", state.supportThreadOpen) ? renderSupportThreadModal() : ""}
      ${isModalVisible("devices", state.devicesModalOpen) ? renderDevicesModal() : ""}
      ${isModalVisible("pay", state.payModalOpen) ? renderPayModal() : ""}
      ${isModalVisible("payment-launch", state.paymentLaunchModalOpen) ? renderPaymentLaunchModal() : ""}
      ${isModalVisible("review-compose", state.reviewComposeOpen) ? renderReviewComposerModal() : ""}
      ${isModalVisible("review-detail", state.reviewDetailOpen) ? renderReviewDetailModal() : ""}
		${state.adminPlanEditorModalOpen ? renderAdminPlanEditorModal() : ""}
    </div>
  `;
  bindRootActions();
  mountAdminContentTabs();
  restoreScrollPosition(nextScrollTop);
	mountRuntimeLayout();
  syncBottomNavIndicator();
  restoreSupportThreadScrollState(supportThreadScrollState);
  syncToastAnchor();
  syncNativeBackButton();
  syncSupportPolling();
	syncAdminBroadcastPolling();
  mountGoogleLoginWidgets();
  pageAnimationEnabled = false;
}

function mountAdminContentTabs() {
	const tabs = app.querySelector(".admin-content-tabs");
	if (!tabs) return;

	tabs.scrollLeft = Math.max(0, Number(state.adminContentTabsScrollLeft) || 0);
	tabs.addEventListener("scroll", () => {
		state.adminContentTabsScrollLeft = tabs.scrollLeft;
	}, { passive: true });
	tabs.addEventListener("wheel", (event) => {
		if (tabs.scrollWidth <= tabs.clientWidth) return;
		const delta = Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY;
		if (!delta) return;
		event.preventDefault();
		tabs.scrollLeft += delta;
	}, { passive: false });

	let pointerID = null;
	let startX = 0;
	let startScrollLeft = 0;
	let dragged = false;
	const finishDrag = (event) => {
		if (pointerID === null || (event && event.pointerId !== pointerID)) return;
		pointerID = null;
		tabs.classList.remove("is-dragging");
	};
	tabs.addEventListener("pointerdown", (event) => {
		tabs.dataset.dragMoved = "false";
		if (event.pointerType !== "mouse" || event.button !== 0) return;
		pointerID = event.pointerId;
		startX = event.clientX;
		startScrollLeft = tabs.scrollLeft;
		dragged = false;
		tabs.classList.add("is-dragging");
		window.addEventListener("pointerup", finishDrag, { once: true });
		window.addEventListener("pointercancel", finishDrag, { once: true });
	});
	tabs.addEventListener("pointermove", (event) => {
		if (pointerID === null || event.pointerId !== pointerID) return;
		const offset = event.clientX - startX;
		if (!dragged && Math.abs(offset) < 5) return;
		dragged = true;
		tabs.dataset.dragMoved = "true";
		event.preventDefault();
		tabs.scrollLeft = startScrollLeft - offset;
	});
	tabs.addEventListener("click", (event) => {
		if (tabs.dataset.dragMoved !== "true") return;
		event.preventDefault();
		event.stopPropagation();
		tabs.dataset.dragMoved = "false";
	}, true);
}

function renderSidebar() {
  const copy = t();
  const links = state.data.links || {};
  const mainItems = [
    renderSidebarPageItem("dashboard", "home", copy.pageDashboard),
    renderSidebarPageItem("buy", "cart", copy.pageBuy),
    renderSidebarPageItem("setup", "shield", copy.pageSetup),
    renderSidebarPageItem("support", "chat", copy.pageSupport),
    renderSidebarPageItem("faq", "question", copy.pageFaq),
    renderSidebarPageItem("reviews", "star", copy.feedback),
  ].join("");
  const serviceItems = [
    renderSidebarPageItem("referrals", "users", copy.pageReferrals),
    renderSidebarPageItem("servers", "server", copy.pageServers || copy.serverStatus),
    renderSidebarPageItem("settings", "profile", copy.pageSettings),
    renderSidebarPageItem("terms", "doc", copy.tos),
  ].join("");
  const linkItems = [
    links.channel ? renderSidebarLinkItem(links.channel, "broadcast", copy.channel) : "",
  ].join("");
  return `
    <div class="sidebar__top">
      <div class="sidebar__brand-block">
        <div class="sidebar__brand-mark">${escapeHtml(state.data.brand.name)}</div>
        <div class="sidebar__brand-note">Mini App</div>
      </div>
      <button class="sidebar__close" type="button" data-action="close-sidebar" aria-label="Close">${icon("close")}</button>
    </div>
    <div class="sidebar__section">${copy.pageDashboard}</div>
    <div class="sidebar__group">
      ${mainItems}
    </div>
    <div class="sidebar__section">${copy.pageReferrals}</div>
    <div class="sidebar__group">
      ${serviceItems}
    </div>
    ${linkItems ? `
      <div class="sidebar__section">${copy.settingsLinks}</div>
      <div class="sidebar__group">
        ${linkItems}
      </div>
    ` : ""}
  `;
}

function renderPages() {
  return [
    renderDashboardPage(),
    renderBuyPage(),
    renderSetupPage(),
    renderSupportPage(),
    renderFaqPage(),
    renderReviewsPage(),
    renderReferralsPage(),
    renderServersPage(),
    renderSettingsPage(),
    renderMediaPage(),
    renderLoginMethodsPage(),
    renderPaymentsPage(),
    renderTermsPage(),
    renderAdminPage(),
  ].join("");
}

function renderAdminPage() {
	if (state.adminLayoutEditing || state.adminPlanEditing) return `<section class="page admin-page ${pageClass("admin")}" id="page-admin"></section>`;
	syncAdminSettingsDraft();
	if (state.adminSection === "promocodes") return renderAdminPromocodesPage();
	if (state.adminSection === "subscriptions") return renderAdminSubscriptionsPage();
	if (state.adminSection === "maintenance") return renderAdminMaintenancePage();
	if (state.adminSection === "diagnostics") return renderAdminDiagnosticsPage();
	if (state.adminSection === "features") return renderAdminFeaturesPage();
	if (state.adminSection === "content") return renderAdminContentPage();
	if (state.adminSection === "appearance") return renderAdminAppearancePage();
	if (state.adminSection === "layout") return renderAdminLayoutPage();
	if (state.adminSection === "plans") return renderAdminPlansPage();
	if (state.adminSection === "trial") return renderAdminTrialPage();
	if (state.adminSection === "broadcast") return renderAdminBroadcastPage();
	if (state.adminSection === "integrations") return renderAdminIntegrationsPage();
	const english = state.locale === "en";
	return `
		<section class="page admin-page ${pageClass("admin")}" id="page-admin">
			${renderAdminMenuGroup(english ? "System" : "Система", [
				[english ? "Maintenance mode" : "Режим аварии", english ? "Block access during maintenance" : "Отключить доступ для пользователей", "maintenance", "wrench"],
				[english ? "Diagnostics" : "Диагностика", english ? "Errors and service health" : "Ошибки и состояние сервисов", "diagnostics", "alert"],
				[english ? "Functions" : "Управление функциями", english ? "Payments, login, trials and sections" : "Оплаты, вход, триалы и разделы", "features", "sliders"],
			])}
			${renderAdminMenuGroup(english ? "Interface" : "Интерфейс", [
				[english ? "Content" : "Редактор контента", english ? "Texts, links, FAQ and custom buttons" : "Тексты, ссылки, FAQ и свои кнопки", "content", "doc"],
				[english ? "Appearance" : "Оформление", english ? "Colors, background and frames" : "Цвета, фон и рамки", "appearance", "palette"],
				[english ? "UI builder" : "Конструктор UI", english ? "Order, visibility and dimensions" : "Порядок, видимость и размеры", "layout", "grid"],
				[english ? "Plans" : "Тарифы", english ? "Prices, limits and order" : "Цены, лимиты и порядок", "plans", "cartShopping"],
				[english ? "Trial" : "Триал", english ? "Trial limits and squads" : "Срок, лимиты и сквады", "trial", "ticketPlus"],
			])}
			${renderAdminMenuGroup(english ? "Operations" : "Операции", [
				[english ? "Integrations" : "Интеграции", english ? "Payment providers and webhooks" : "Платёжные системы и webhook-адреса", "integrations", "sliders"],
				[english ? "Broadcast" : "Рассылка", english ? "Message, buttons and delivery" : "Сообщение, кнопки и отправка", "broadcast", "broadcast"],
				[english ? "Subscription binding" : "Привязка подписок", english ? "Transfer access between accounts" : "Перенос доступа между аккаунтами", "subscriptions", "key"],
				[english ? "Promo codes" : "Промокоды", english ? "Create and monitor codes" : "Создание и статистика кодов", "promocodes", "ticketPlus"],
			])}
		</section>
	`;
}

function renderAdminMenuGroup(label, items) {
	return `<section class="admin-menu-group"><h2 class="admin-menu-group__title"><span></span>${escapeHtml(label)}</h2><div class="admin-menu-group__rows">${items.map(([title, hint, value, iconName]) => renderMenuRow(title, hint, "open-admin-section", value, iconName, { showTail: true })).join("")}</div></section>`;
}

function integrationDraft(item) {
	if (!state.adminIntegrationDrafts[item.id]) {
		state.adminIntegrationDrafts[item.id] = {
			enabled: Boolean(item.enabled),
			fields: Object.fromEntries((item.fields || []).map((field) => [field.key, field.secret ? "" : String(field.value || "")])),
		};
	}
	return state.adminIntegrationDrafts[item.id];
}

function renderAdminIntegrationsPage() {
	const items = Array.isArray(state.data?.admin?.integrations) ? state.data.admin.integrations : [];
	const groups = [
		["Платёжные системы", items.filter((item) => item.kind === "payment")],
		["Служебные интеграции", items.filter((item) => item.kind !== "payment")],
	];
	return `<section class="page admin-page ${pageClass("admin")}" id="page-admin"><div class="admin-integrations">
		<header class="admin-integrations__header"><span>ИНТЕГРАЦИИ</span><h2>Платежи и уведомления</h2><p>Ключи хранятся зашифрованно. Включённая платёжная система сразу появляется у пользователей.</p></header>
		${groups.map(([label, providers]) => providers.length ? `<section class="admin-integrations__group"><h3>${escapeHtml(label)}</h3><div class="admin-integrations__list">${providers.map(renderAdminIntegrationRow).join("")}</div></section>` : "").join("")}
	</div></section>`;
}

function renderAdminIntegrationRow(item) {
	const draft = integrationDraft(item);
	const open = state.adminIntegrationOpen === item.id;
	const busy = state.adminIntegrationBusy === item.id;
	const status = item.enabled && item.configured ? "Работает" : item.configured ? "Выключено" : "Не настроено";
	return `<article class="admin-integration ${open ? "is-open" : ""}">
		<button class="admin-integration__summary" type="button" data-action="admin-integration-open" data-value="${escapeAttribute(item.id)}" aria-expanded="${open ? "true" : "false"}">
			<img src="${escapeAttribute(item.logo || BRAND_MARK_URL)}" alt="" aria-hidden="true"><span><strong>${escapeHtml(item.name)}</strong><small>${escapeHtml(item.description || "")}</small></span><i class="admin-integration__status ${item.enabled && item.configured ? "is-active" : ""}">${escapeHtml(status)}</i>${icon("chevron")}
		</button>
		${open ? `<div class="admin-integration__body">
			<label class="admin-integration__toggle"><span><strong>Включить интеграцию</strong><small>${item.kind === "payment" ? "Показывать этот способ оплаты" : "Отправлять уведомления об оплатах"}</small></span><input type="checkbox" data-integration-provider="${escapeAttribute(item.id)}" data-integration-enabled ${draft.enabled ? "checked" : ""}></label>
			<div class="admin-integration__fields">${(item.fields || []).map((field) => `<label class="admin-field"><span>${escapeHtml(field.label)}${field.required ? " *" : ""}</span><input class="admin-field__control" type="${field.secret ? "password" : "text"}" autocomplete="off" spellcheck="false" data-integration-provider="${escapeAttribute(item.id)}" data-integration-field="${escapeAttribute(field.key)}" value="${escapeAttribute(draft.fields[field.key] || "")}" placeholder="${escapeAttribute(field.secret && field.configured ? "Ключ сохранён — оставьте пустым" : (field.placeholder || ""))}">${field.help ? `<small>${escapeHtml(field.help)}</small>` : ""}</label>`).join("")}</div>
			${item.webhookUrl ? `<div class="admin-integration__webhook"><span>Webhook URL</span><code>${escapeHtml(item.webhookUrl)}</code><button type="button" data-action="admin-integration-copy-webhook" data-value="${escapeAttribute(item.webhookUrl)}" aria-label="Скопировать webhook">${icon("copy")}</button></div>` : ""}
			<button class="admin-integration__save" type="button" data-action="admin-integration-save" data-value="${escapeAttribute(item.id)}" ${busy ? "disabled" : ""}>${icon(busy ? "refresh" : "check")}<span>${busy ? "Сохраняем" : "Сохранить"}</span></button>
		</div>` : ""}
	</article>`;
}

function renderAdminBroadcastPage() {
	const english = state.locale === "en";
	const draft = state.adminBroadcast || { status: "idle", buttons: [], recipientCount: 0, sentCount: 0, failedCount: 0 };
	const running = draft.status === "running";
	const awaiting = draft.status === "awaiting_message";
	const hasMessage = Boolean(draft.sourceKind);
	const buttons = state.adminBroadcastButtonsDraft || [];
	const total = Math.max(0, Number(draft.recipientCount || 0));
	const processed = Math.min(total, Math.max(0, Number(draft.sentCount || 0) + Number(draft.failedCount || 0)));
	const progress = total > 0 ? Math.round((processed / total) * 100) : 0;
	const captureBusy = state.adminBroadcastBusy === "capture";
	const saveBusy = state.adminBroadcastBusy === "buttons";
	const previewBusy = state.adminBroadcastBusy === "preview";
	const sendBusy = state.adminBroadcastBusy === "send";

	return `
		<section class="page admin-page ${pageClass("admin")}" id="page-admin">
			<div class="admin-broadcast">
				<header class="admin-broadcast__header">
					<div><span>${english ? "Delivery" : "Рассылка"}</span><h2>${english ? "Message to users" : "Сообщение пользователям"}</h2></div>
					<span class="admin-broadcast__status admin-broadcast__status--${escapeAttribute(draft.status || "idle")}">${escapeHtml(broadcastStatusLabel(draft.status, english))}</span>
				</header>

				<section class="admin-broadcast__section">
					<div class="admin-broadcast__section-head"><div><span>1</span><div><strong>${english ? "Message" : "Сообщение"}</strong><small>${english ? "Created in Telegram" : "Создаётся в Telegram"}</small></div></div></div>
					${hasMessage ? `<div class="admin-broadcast__message"><span>${escapeHtml(broadcastKindLabel(draft.sourceKind, english))}</span><p>${escapeHtml(draft.sourcePreview || (english ? "Message without text" : "Сообщение без текста"))}</p></div>` : `<p class="admin-broadcast__empty">${awaiting ? (english ? "Send the next message to the bot." : "Отправьте следующее сообщение боту.") : (english ? "No message selected yet." : "Сообщение пока не выбрано.")}</p>`}
					<button class="admin-broadcast__primary" type="button" data-action="admin-broadcast-capture" ${running || captureBusy ? "disabled" : ""}>${icon(captureBusy ? "refresh" : "send")}<span>${escapeHtml(hasMessage ? (english ? "Replace message" : "Заменить сообщение") : (english ? "Write a message" : "Написать сообщение"))}</span></button>
				</section>

				<section class="admin-broadcast__section">
					<div class="admin-broadcast__section-head"><div><span>2</span><div><strong>${english ? "Buttons" : "Кнопки"}</strong><small>${english ? "Up to 8 buttons" : "До 8 кнопок"}</small></div></div><button class="admin-icon-button" type="button" data-action="admin-broadcast-add-button" ${running || buttons.length >= 8 ? "disabled" : ""} aria-label="${english ? "Add button" : "Добавить кнопку"}">${icon("plus")}</button></div>
					<div class="admin-broadcast__buttons">${buttons.length ? buttons.map((button, index) => renderAdminBroadcastButton(button, index, running, english)).join("") : `<p class="admin-broadcast__empty">${english ? "The message will be sent without buttons." : "Сообщение будет отправлено без кнопок."}</p>`}</div>
					${buttons.length ? `<button class="admin-broadcast__secondary" type="button" data-action="admin-broadcast-save-buttons" ${running || saveBusy || !state.adminBroadcastButtonsDirty ? "disabled" : ""}>${icon(saveBusy ? "refresh" : "check")}<span>${english ? "Save buttons" : "Сохранить кнопки"}</span></button>` : ""}
				</section>

				<section class="admin-broadcast__section admin-broadcast__section--delivery">
					<div class="admin-broadcast__section-head"><div><span>3</span><div><strong>${english ? "Delivery" : "Отправка"}</strong><small>${english ? "Preview before launch" : "Сначала проверьте предпросмотр"}</small></div></div></div>
					${running || total > 0 ? `<div class="admin-broadcast__progress"><div><span>${english ? "Progress" : "Прогресс"}</span><strong>${progress}%</strong></div><i><b style="width:${progress}%"></b></i><p>${english ? "Sent" : "Отправлено"}: ${Number(draft.sentCount || 0)} · ${english ? "Errors" : "Ошибок"}: ${Number(draft.failedCount || 0)} · ${english ? "Total" : "Всего"}: ${total}</p></div>` : ""}
					${draft.lastError ? `<p class="admin-broadcast__error">${escapeHtml(draft.lastError)}</p>` : ""}
					<div class="admin-broadcast__actions">
						<button type="button" data-action="admin-broadcast-preview" ${!hasMessage || running || previewBusy ? "disabled" : ""}>${icon(previewBusy ? "refresh" : "eye")}<span>${english ? "Preview" : "Предпросмотр"}</span></button>
						<button class="admin-broadcast__send" type="button" data-action="admin-broadcast-open-confirm" ${!hasMessage || running || sendBusy ? "disabled" : ""}>${icon(sendBusy ? "refresh" : "send")}<span>${english ? "Send" : "Запустить"}</span></button>
					</div>
					<button class="admin-broadcast__reset" type="button" data-action="admin-broadcast-reset" ${running ? "disabled" : ""}>${icon("reset")}<span>${english ? "Clear draft" : "Очистить черновик"}</span></button>
				</section>
			</div>
			${state.adminBroadcastConfirmOpen ? renderAdminBroadcastConfirm(english) : ""}
		</section>
	`;
}

function renderAdminBroadcastButton(button, index, disabled, english) {
	const type = button?.type === "promo" ? "promo" : "url";
	const style = ["primary", "success", "danger"].includes(button?.style) ? button.style : "";
	const styles = english
		? [["", "Default"], ["primary", "Blue"], ["success", "Green"], ["danger", "Red"]]
		: [["", "Обычная"], ["primary", "Синяя"], ["success", "Зелёная"], ["danger", "Красная"]];
	return `<div class="admin-broadcast-button">
		<div class="admin-broadcast-button__head"><strong>${english ? "Button" : "Кнопка"} ${index + 1}</strong><button class="admin-icon-button admin-icon-button--danger" type="button" data-action="admin-broadcast-remove-button" data-value="${index}" ${disabled ? "disabled" : ""} aria-label="${english ? "Delete" : "Удалить"}">${icon("trash")}</button></div>
		<div class="admin-broadcast-button__grid">
			<label><span>${english ? "Type" : "Тип"}</span><select data-broadcast-index="${index}" data-broadcast-field="type" ${disabled ? "disabled" : ""}><option value="url" ${type === "url" ? "selected" : ""}>${english ? "Link" : "Ссылка"}</option><option value="promo" ${type === "promo" ? "selected" : ""}>${english ? "Promo code" : "Промокод"}</option></select></label>
			<label><span>${english ? "Text or tg-emoji code" : "Текст или tg-emoji код"}</span><input type="text" maxlength="256" value="${escapeAttribute(button?.text || "")}" data-broadcast-index="${index}" data-broadcast-field="text" placeholder="${english ? "Emoji code + Open" : "Код emoji + Открыть"}" ${disabled ? "disabled" : ""}></label>
		</div>
		<label><span>${type === "promo" ? (english ? "Promo code" : "Промокод") : "URL"}</span><input type="${type === "promo" ? "text" : "url"}" maxlength="256" value="${escapeAttribute(type === "promo" ? (button?.promoCode || "") : (button?.url || ""))}" data-broadcast-index="${index}" data-broadcast-field="${type === "promo" ? "promoCode" : "url"}" placeholder="${type === "promo" ? "LINK20" : "https://..."}" ${disabled ? "disabled" : ""}></label>
		<label><span>${english ? "Premium emoji ID or full code" : "ID premium emoji или полный код"}</span><input type="text" maxlength="128" value="${escapeAttribute(button?.iconCustomEmojiId || "")}" data-broadcast-index="${index}" data-broadcast-field="iconCustomEmojiId" placeholder="5206222720416643915" ${disabled ? "disabled" : ""}></label>
		<fieldset class="admin-broadcast-style" ${disabled ? "disabled" : ""}>
			<legend>${english ? "Button color" : "Цвет кнопки"}</legend>
			<div>${styles.map(([value, label]) => `<button type="button" class="admin-broadcast-style__option ${style === value ? "is-selected" : ""}" data-action="admin-broadcast-set-style" data-broadcast-index="${index}" data-value="${value}" aria-pressed="${style === value ? "true" : "false"}" ${disabled ? "disabled" : ""}><i data-style="${value || "default"}"></i><span>${label}</span></button>`).join("")}</div>
		</fieldset>
	</div>`;
}

function renderAdminBroadcastConfirm(english) {
	return `<div class="modal open"><button class="modal__backdrop" type="button" data-action="admin-broadcast-close-confirm" aria-label="${english ? "Cancel" : "Отмена"}"></button><div class="modal__sheet admin-broadcast-confirm" role="dialog" aria-modal="true" aria-labelledby="broadcast-confirm-title"><div class="modal__header"><div><div class="section-label">${english ? "Broadcast" : "Рассылка"}</div><div class="modal__title" id="broadcast-confirm-title">${english ? "Send to all users?" : "Отправить всем пользователям?"}</div></div><button class="header__btn" type="button" data-action="admin-broadcast-close-confirm" aria-label="${english ? "Close" : "Закрыть"}">${icon("close")}</button></div><p>${english ? "The broadcast starts immediately. Check the preview before sending." : "Рассылка начнётся сразу. Перед отправкой проверьте предпросмотр."}</p><div class="admin-broadcast-confirm__actions"><button type="button" data-action="admin-broadcast-close-confirm">${english ? "Cancel" : "Отмена"}</button><button class="admin-broadcast-confirm__send" type="button" data-action="admin-broadcast-send">${icon("send")}<span>${english ? "Start" : "Запустить"}</span></button></div></div></div>`;
}

function broadcastStatusLabel(status, english) {
	const labels = english
		? { idle: "Empty", awaiting_message: "Waiting", draft: "Draft", running: "Sending", finished: "Finished", failed: "Stopped" }
		: { idle: "Пусто", awaiting_message: "Ожидает", draft: "Черновик", running: "Отправка", finished: "Завершено", failed: "Остановлено" };
	return labels[status] || labels.idle;
}

function broadcastKindLabel(kind, english) {
	const labels = english
		? { text: "Text", photo: "Photo", video: "Video", animation: "Animation", document: "File", audio: "Audio", voice: "Voice", video_note: "Video message", sticker: "Sticker" }
		: { text: "Текст", photo: "Фото", video: "Видео", animation: "Анимация", document: "Файл", audio: "Аудио", voice: "Голосовое", video_note: "Видеосообщение", sticker: "Стикер" };
	return labels[kind] || (english ? "Message" : "Сообщение");
}

function renderAdminMaintenancePage() {
	return renderAdminEditorPage("Режим аварии", `
		${renderAdminToggle("Технические работы включены", "maintenance.enabled")}
		${renderAdminSettingField("Заголовок", "maintenance.titleRu")}
		${renderAdminSettingField("Текст", "maintenance.textRu", { textarea: true, rows: 3 })}
		${renderAdminSettingField("Причина", "maintenance.reasonRu")}
	`);
}

function renderAdminFeaturesPage() {
	const labels = {
		mini_app: "Mini app", google: "Gmail", stars: "Telegram Stars", trials: "Триалы", referrals: "Реферальная система", reviews: "Отзывы", support: "Поддержка", media: "Медиа", promocodes: "Промокоды", server_status: "Статус серверов", web_version: "Web-версия", pwa_install: "Установка на рабочий стол",
	};
	return renderAdminEditorPage(state.locale === "en" ? "Functions" : "Управление функциями", `<div class="admin-toggle-list">${Object.entries(labels).map(([key, label]) => renderAdminToggle(label, `features.${key}`)).join("")}</div>`);
}

function renderAdminContentPage() {
	const sections = [
		["start", "Главное меню"],
		["verification", "Верификация"],
		["commerce", "Тарифы и оплата"],
		["success", "После покупки"],
		["support", "Поддержка"],
		["notifications", "Уведомления"],
		["profile", "Профиль"],
		["faq", "FAQ"],
		["advanced", "Тексты RU"],
	];
	const active = sections.some(([id]) => id === state.adminContentSection) ? state.adminContentSection : "start";
	state.adminContentSection = active;
	return renderAdminEditorPage("Редактор контента", `
		<nav class="admin-content-tabs" aria-label="Разделы контента">${sections.map(([id, label]) => `<button type="button" class="${id === active ? "is-active" : ""}" data-action="admin-content-section" data-value="${id}" aria-current="${id === active ? "page" : "false"}">${escapeHtml(label)}</button>`).join("")}</nav>
		<div class="admin-content-panel">${renderAdminContentSection(active)}</div>
	`);
}

function renderAdminContentSection(section) {
	switch (section) {
		case "verification": return renderAdminVerificationContent();
		case "commerce": return renderAdminCommerceContent();
		case "success": return renderAdminSuccessContent();
		case "support": return renderAdminSupportContent();
		case "notifications": return renderAdminNotificationContent();
		case "profile": return renderAdminProfileContent();
		case "faq": return renderAdminFAQContent();
		case "advanced": return `<section class="admin-editor__section"><h3>Тексты RU</h3>${renderAdminJSONField("JSON подписей mini app", "content.copy.ru", 16)}</section>`;
		default: return renderAdminStartContent();
	}
}

function renderAdminStartContent() {
	return `<section class="admin-editor__section"><h3>Сервис и сообщение /start</h3>
		${renderAdminSettingField("Название сервиса", "content.brandName")}
		${renderAdminSettingField("Контакт администрации", "content.adminContact", { placeholder: "@username" })}
		${renderAdminSettingField("Логотип mini app", "content.logoUrl", { placeholder: "/mini-app/assets/brand-mark.png или URL" })}
		${renderAdminBannerField("Баннер главного меню", "content.startImage", "/assets/telegram/menu/banner.png")}
		${renderAdminSettingField("Текст сообщения", "content.startTextRu", { textarea: true, rows: 7 })}
	</section>
	<section class="admin-editor__section"><h3>Кнопки главного меню</h3>
		${renderAdminTelegramButton("Попробовать бесплатно", "content.startMenu.trialButton")}
		${renderAdminTelegramButton("Вход", "content.startMenu.dashboardButton")}
		${renderAdminTelegramButton("Тарифы", "content.startMenu.plansButton")}
		${renderAdminTelegramButton("Чат с поддержкой", "content.startMenu.supportButton")}
	</section>`;
}

function renderAdminVerificationContent() {
	return `<section class="admin-editor__section"><h3>Канал для проверки</h3>
		${renderAdminSettingField("Ссылка на канал", "content.links.channel", { type: "url", placeholder: "https://t.me/channel; пусто — проверка отключена" })}
		${renderAdminSettingField("ID приватного канала", "content.verification.channelChatId", { placeholder: "-1001234567890; для публичного канала не нужен" })}
		<div class="admin-empty-line">Бот должен быть администратором канала. Для публичного канала достаточно ссылки. Очистите ссылку, чтобы отключить верификацию.</div>
	</section>
	<section class="admin-editor__section"><h3>Сообщение верификации</h3>
		${renderAdminBannerField("Баннер верификации", "content.verification.banner", "/assets/telegram/verification/banner.png")}
		${renderAdminSettingField("Текст сообщения", "content.verification.text", { textarea: true, rows: 9 })}
	</section>
	<section class="admin-editor__section"><h3>Кнопки</h3>
		${renderAdminTelegramButton("Канал", "content.verification.channelButton")}
		${renderAdminTelegramButton("Подтверждение подписки", "content.verification.confirmButton")}
	</section>
	<section class="admin-editor__section"><h3>Результат проверки</h3>
		${renderAdminSettingField("Ошибка проверки", "content.verification.checkFailedText")}
		${renderAdminSettingField("Подписка не найдена", "content.verification.notSubscribedText")}
		${renderAdminSettingField("Доступ открыт", "content.verification.verifiedText")}
	</section>`;
}

function renderAdminCommerceContent() {
	return `<section class="admin-editor__section"><h3>Экраны покупки</h3>
		${renderAdminBannerField("Общий баннер тарифов и оплаты", "content.commerce.banner", "/assets/telegram/commerce/banner.png")}
		${renderAdminSettingField("Экран выбора тарифа", "content.commerce.tariffsText", { textarea: true, rows: 4 })}
		${renderAdminSettingField("Экран выбора способа оплаты", "content.commerce.paymentMethodsText", { textarea: true, rows: 4 })}
		${renderAdminSettingField("Экран перехода к оплате", "content.commerce.paymentReadyText", { textarea: true, rows: 4 })}
	</section>
	<section class="admin-editor__section"><h3>Способы оплаты</h3>
		${renderAdminTelegramButton("СБП | Карта", "content.commerce.yookassaButton")}
		${renderAdminTelegramButton("CryptoPay", "content.commerce.cryptoButton")}
		${renderAdminTelegramButton("Telegram Stars", "content.commerce.starsButton")}
	</section>
	<section class="admin-editor__section"><h3>Общие кнопки покупки</h3>
		${renderAdminTelegramButton("Оплатить", "content.commerce.payButton")}
		${renderAdminTelegramButton("Назад", "content.commerce.backButton")}
	</section>`;
}

function renderAdminSuccessContent() {
	const busy = state.adminBusy === "test-success";
	return `<section class="admin-editor__section">
		<div class="admin-editor__section-head"><h3>Подписка активирована</h3><button class="admin-reminder-test" type="button" data-action="admin-test-success" ${state.adminBusy ? "disabled" : ""}><span>${busy ? "Отправляем" : "Отправить тест"}</span></button></div>
		${renderAdminBannerField("Баннер успешной покупки", "content.commerce.successBanner", "/assets/telegram/success/banner.png")}
		${renderAdminSettingField("Текст после оплаты", "content.commerce.successText", { textarea: true, rows: 7 })}
		${renderAdminTelegramButton("Переход в личный кабинет", "content.commerce.successButton")}
	</section>`;
}

function renderAdminSupportContent() {
	const interfaceFields = [
		["Кнопка нового обращения", "content.copy.ru.newTicket"],
		["Описание кнопки", "content.copy.ru.newTicketHint"],
		["Часто задаваемые вопросы", "content.copy.ru.supportFaqTitle"],
		["Описание FAQ", "content.copy.ru.supportFaqHint"],
		["Вкладка открытых", "content.copy.ru.supportOpenTab"],
		["Вкладка истории", "content.copy.ru.supportHistoryTab"],
		["Подсказка для администратора", "content.copy.ru.supportAdminHint"],
		["Нет открытых — заголовок", "content.copy.ru.supportNoOpenTitle"],
		["Нет открытых — описание", "content.copy.ru.supportNoOpenHint"],
		["История пуста — заголовок", "content.copy.ru.supportNoHistoryTitle"],
		["История пуста — описание", "content.copy.ru.supportNoHistoryHint"],
	];
	const composerFields = [
		["Заголовок формы", "content.copy.ru.supportCreateTitle"],
		["Название поля темы", "content.copy.ru.supportSubjectLabel"],
		["Подсказка темы", "content.copy.ru.supportSubjectPlaceholder"],
		["Название поля сообщения", "content.copy.ru.supportMessageLabel"],
		["Подсказка сообщения", "content.copy.ru.supportMessagePlaceholder"],
		["Кнопка отправки", "content.copy.ru.supportSendButton"],
		["Закрыть обращение", "content.copy.ru.supportCloseButton"],
		["Закрыто — заголовок", "content.copy.ru.supportClosedTitle"],
		["Закрыто — описание", "content.copy.ru.supportClosedHint"],
		["Поле ответа", "content.copy.ru.supportReplyPlaceholder"],
		["Загрузка переписки", "content.copy.ru.supportLoadingThread"],
	];
	const resultFields = [
		["Обращение создано", "content.copy.ru.ticketCreated"],
		["Ответ отправлен", "content.copy.ru.replySent"],
		["Обращение закрыто", "content.copy.ru.ticketClosedToast"],
	];
	return `<section class="admin-editor__section"><h3>Интерфейс поддержки</h3>
		<div class="admin-editor__grid admin-editor__grid--two">${interfaceFields.map(([label, path]) => renderAdminSettingField(label, path)).join("")}</div>
	</section>
	<section class="admin-editor__section"><h3>Создание и переписка</h3>
		<div class="admin-editor__grid admin-editor__grid--two">${composerFields.map(([label, path]) => renderAdminSettingField(label, path)).join("")}</div>
	</section>
	<section class="admin-editor__section"><h3>Результаты действий</h3>
		<div class="admin-editor__grid admin-editor__grid--two">${resultFields.map(([label, path]) => renderAdminSettingField(label, path)).join("")}</div>
	</section>
	<section class="admin-editor__section"><h3>Telegram-уведомления</h3>
		<div class="admin-empty-line">Доступны переменные: {ticket_id}, {subject}, {name}, {username}, {subscription}, {message}. Сообщение пользователя или поддержки всегда отправляется цитатой.</div>
		${renderAdminSettingField("Новое обращение — админу", "content.support.newTicketText", { textarea: true, rows: 9 })}
		${renderAdminSettingField("Ответ пользователя — админу", "content.support.customerReplyText", { textarea: true, rows: 9 })}
		${renderAdminSettingField("Ответ поддержки — пользователю", "content.support.adminReplyText", { textarea: true, rows: 7 })}
		${renderAdminSettingField("Обращение закрыто — пользователю", "content.support.closedText", { textarea: true, rows: 5 })}
	</section>
	<section class="admin-editor__section"><h3>Кнопка уведомления</h3>${renderAdminTelegramButton("Открыть Mini app", "content.support.openButton")}</section>`;
}

function renderAdminNotificationContent() {
	return `<section class="admin-editor__section"><h3>Напоминания о подписке</h3>
		${renderAdminReminderTemplate("expiring", "Подписка скоро закончится", "content.copy.ru.subscriptionExpiringTemplate")}
		${renderAdminReminderTemplate("expired", "Подписка закончилась", "content.copy.ru.subscriptionExpiredTemplate")}
		<div class="admin-editor__subsection"><h4>Кнопка перехода к тарифам</h4>
			${renderAdminSettingField("Текст кнопки", "content.copy.ru.subscriptionRenewButton")}
			${renderAdminSettingField("Premium emoji ID или код", "content.subscriptionReminderButton.iconCustomEmojiId", { placeholder: "5206222720416643915 или <tg-emoji ...>" })}
			${renderAdminTelegramButtonStyle("content.subscriptionReminderButton.style")}
		</div>
		<div class="admin-empty-line">Переменная {date} подставляет дату окончания. Поддерживается HTML Telegram.</div>
	</section>
	<section class="admin-editor__section"><h3>Системные уведомления mini app</h3>
		${[["trialActivated", "Триал активирован"], ["paymentOpened", "Оплата открыта"], ["paymentUnavailable", "Оплата недоступна"], ["paymentSuccess", "Оплата завершена"], ["paymentPending", "Оплата ожидается"], ["paymentCancelled", "Оплата отменена"], ["promoApplied", "Промокод применён"], ["promoCodeRequired", "Промокод не введён"], ["copied", "Ссылка скопирована"], ["deleteSuccess", "Устройство удалено"], ["noAccess", "Нет активной подписки"], ["timeout", "Сервер не ответил"]].map(([key, label]) => renderAdminSettingField(label, `content.copy.ru.${key}`)).join("")}
	</section>`;
}

function renderAdminProfileContent() {
	const custom = state.adminSettingsDraft?.content?.customLinks || [];
	return `<section class="admin-editor__section"><h3>Ссылки</h3>
		${renderAdminSettingField("Поддержка", "content.links.support", { type: "url" })}
	</section>
	<section class="admin-editor__section"><div class="admin-editor__section-head"><h3>Свои кнопки профиля</h3><button class="admin-icon-button" type="button" data-action="admin-add-custom-link" aria-label="Добавить кнопку">${icon("plus")}</button></div>
		${custom.length ? custom.map((item, index) => renderAdminCustomLink(item, index)).join("") : `<div class="admin-empty-line">Свои кнопки не добавлены</div>`}
	</section>`;
}

function renderAdminFAQContent() {
	const items = state.adminSettingsDraft?.content?.faq?.ru || [];
	return `<section class="admin-editor__section"><div class="admin-editor__section-head"><h3>Часто задаваемые вопросы</h3><button class="admin-icon-button" type="button" data-action="admin-add-faq" aria-label="Добавить вопрос">${icon("plus")}</button></div>
		<div class="admin-faq-editor">${items.length ? items.map((item, index) => renderAdminFAQItem(item, index, items.length)).join("") : `<div class="admin-empty-line">Вопросы пока не добавлены</div>`}</div>
	</section>`;
}

function renderAdminFAQItem(item, index, total) {
	return `<article class="admin-faq-item"><div class="admin-faq-item__head"><strong>Вопрос ${index + 1}</strong><div>
		<button type="button" data-action="admin-move-faq" data-value="${index}" data-direction="-1" ${index === 0 ? "disabled" : ""} aria-label="Переместить выше">${icon("arrowUp")}</button>
		<button type="button" data-action="admin-move-faq" data-value="${index}" data-direction="1" ${index === total - 1 ? "disabled" : ""} aria-label="Переместить ниже">${icon("arrowDown")}</button>
		<button type="button" class="admin-icon-button--danger" data-action="admin-remove-faq" data-value="${index}" aria-label="Удалить вопрос">${icon("trash")}</button>
	</div></div>
	${renderAdminSettingField("Вопрос", `content.faq.ru.${index}.question`)}
	${renderAdminSettingField("Ответ", `content.faq.ru.${index}.answer`, { textarea: true, rows: 5 })}</article>`;
}

function renderAdminBannerField(label, path, example) {
	return `<div class="admin-banner-field">${renderAdminSettingField(label, path, { placeholder: `${example} или HTTPS URL` })}<small>Папка: <code>${escapeHtml(example.substring(0, example.lastIndexOf("/") + 1))}</code>. Оставьте поле пустым, чтобы отправлять без баннера.</small></div>`;
}

function renderAdminTelegramButton(title, path) {
	return `<div class="admin-telegram-button"><h4>${escapeHtml(title)}</h4>
		<div class="admin-editor__grid admin-editor__grid--two">${renderAdminSettingField("Текст", `${path}.text`)}${renderAdminSettingField("Premium emoji ID или код", `${path}.iconCustomEmojiId`, { placeholder: "5206222720416643915 или <tg-emoji ...>" })}</div>
		${renderAdminTelegramButtonStyle(`${path}.style`)}
	</div>`;
}

function renderAdminReminderTemplate(kind, label, path) {
	const busy = state.adminBusy === `test-reminder-${kind}`;
	return `<div class="admin-reminder-template">
		<div class="admin-reminder-template__head"><h4>${escapeHtml(label)}</h4><button class="admin-reminder-test" type="button" data-action="admin-test-reminder" data-value="${escapeAttribute(kind)}" ${state.adminBusy ? "disabled" : ""}><span>${busy ? "Отправляем" : "Отправить тест"}</span></button></div>
		${renderAdminSettingField("Текст уведомления", path, { textarea: true, rows: 7 })}
	</div>`;
}

function renderAdminTelegramButtonStyle(path) {
	const current = String(getDeepValue(state.adminSettingsDraft, path, "") || "");
	const inputName = `telegram-button-style-${String(path).replace(/[^a-z0-9]+/gi, "-")}`;
	const options = [
		["", "Без цвета"],
		["primary", "Синяя"],
		["success", "Зелёная"],
		["danger", "Красная"],
	];
	return `<fieldset class="admin-reminder-style"><legend>Цвет кнопки</legend><div>${options.map(([value, label]) => `<label class="admin-reminder-style__option"><input type="radio" name="${escapeAttribute(inputName)}" value="${escapeAttribute(value)}" data-setting-path="${escapeAttribute(path)}" data-setting-type="text" ${current === value ? "checked" : ""}><i data-style="${escapeAttribute(value)}" aria-hidden="true"></i><span>${escapeHtml(label)}</span></label>`).join("")}</div></fieldset>`;
}

function renderAdminCustomLink(item, index) {
	return `<div class="admin-repeat-row"><div class="admin-repeat-row__head"><strong>${escapeHtml(item.labelRu || `Кнопка ${index + 1}`)}</strong><button class="admin-icon-button admin-icon-button--danger" type="button" data-action="admin-remove-custom-link" data-value="${index}" aria-label="Удалить">${icon("trash")}</button></div>
		${renderAdminSettingField("Название", `content.customLinks.${index}.labelRu`)}
		${renderAdminSettingField("Описание", `content.customLinks.${index}.hintRu`)}
		${renderAdminSettingField("URL", `content.customLinks.${index}.url`, { type: "url" })}
		<div class="admin-editor__grid admin-editor__grid--two">${renderAdminSettingField("ID", `content.customLinks.${index}.id`)}${renderAdminSettingField("Иконка", `content.customLinks.${index}.icon`, { placeholder: "external" })}</div>
	</div>`;
}

function renderAdminAppearancePage() {
	const groups = [
		["Основа интерфейса", [["background", "Сплошной фон"], ["text", "Название подписки, дата и выбранный тариф"], ["muted", "Описания и подписи"], ["border", "Рамки"]]],
		["Карточки", [["surface", "Обычные карточки"], ["surfaceStrong", "Выбранные элементы"]]],
		["Кнопки и навигация", [["button", "Фон кнопок и тарифов"], ["buttonText", "Текст кнопок и тарифов"], ["icon", "Все SVG-иконки"], ["accent", "Акцент"]]],
		["Состояния", [["success", "Успех"], ["danger", "Ошибка"], ["unlimitedBadge", "Метка «Безлимит»"]]],
		["Фон «Волны»", [["waveBackground", "Фон за точками"], ["waveDot", "Точки"]]],
		["Фон «Движущаяся сетка»", [["gridBackground", "Фон за линиями"], ["gridLine", "Линии"], ["gridGlowLeft", "Свечение слева"], ["gridGlowRight", "Свечение справа"]]],
		["Фон «Сетка 2»", [["grid2Background", "Фон за сеткой"], ["grid2Line", "Цвет сетки"], ["grid2Glow", "Нижняя подсветка"]]],
	];
	return renderAdminEditorPage(state.locale === "en" ? "Appearance" : "Оформление", `
		<label class="admin-field"><span>Фон</span><select class="admin-field__control" data-setting-path="appearance.backgroundMode"><option value="animated" ${getDeepValue(state.adminSettingsDraft, "appearance.backgroundMode") === "animated" ? "selected" : ""}>Волны</option><option value="grid" ${getDeepValue(state.adminSettingsDraft, "appearance.backgroundMode") === "grid" ? "selected" : ""}>Движущаяся сетка</option><option value="grid2" ${getDeepValue(state.adminSettingsDraft, "appearance.backgroundMode") === "grid2" ? "selected" : ""}>Сетка 2</option><option value="solid" ${getDeepValue(state.adminSettingsDraft, "appearance.backgroundMode") === "solid" ? "selected" : ""}>Сплошной цвет</option></select></label>
		<div class="admin-toggle-list">${renderAdminToggle("Компактный режим", "appearance.compact")}${renderAdminToggle("Показывать рамки", "appearance.showFrames")}</div>
		<div class="admin-appearance-groups">${groups.map(([title, colors]) => `<section class="admin-editor__section admin-appearance-group"><h3>${escapeHtml(title)}</h3><div class="admin-color-grid">${colors.map(([key, label]) => renderAdminColorField(label, `appearance.colors.${key}`)).join("")}</div></section>`).join("")}</div>
	`);
}

function renderAdminLayoutPage() {
	ensureAdminVisualLayoutDraft();
	const category = ADMIN_LAYOUT_CATEGORIES.some((item) => item.id === state.adminLayoutCategory) ? state.adminLayoutCategory : "dashboard";
	state.adminLayoutCategory = category;
	const entries = getAdminLayoutDraftEntries(category);
	const visible = entries.filter(({ item }) => item.visible !== false);
	const hidden = entries.filter(({ item }) => item.visible === false);
	const selected = getAdminLayoutSelection();
	const title = state.locale === "en" ? "Visual UI editor" : "\u0412\u0438\u0437\u0443\u0430\u043b\u044c\u043d\u044b\u0439 \u0440\u0435\u0434\u0430\u043a\u0442\u043e\u0440 UI";
	const hint = state.locale === "en"
		? "Drag elements directly. Pull the corner to resize."
		: "\u0414\u0432\u0438\u0433\u0430\u0439\u0442\u0435 \u044d\u043b\u0435\u043c\u0435\u043d\u0442\u044b \u043f\u0430\u043b\u044c\u0446\u0435\u043c \u0438\u043b\u0438 \u043c\u044b\u0448\u044c\u044e. \u0420\u0430\u0441\u0442\u044f\u0433\u0438\u0432\u0430\u0439\u0442\u0435 \u0437\u0430 \u0443\u0433\u043e\u043b.";
	return renderAdminEditorPage(title, `
		<div class="admin-ui-editor" data-admin-ui-editor>
			<p class="admin-ui-editor__hint">${escapeHtml(hint)}</p>
			<div class="admin-ui-categories" role="tablist" aria-label="${escapeAttribute(title)}">
				${ADMIN_LAYOUT_CATEGORIES.map((item) => `<button type="button" role="tab" aria-selected="${item.id === category}" class="admin-ui-category ${item.id === category ? "active" : ""}" data-action="admin-layout-category" data-value="${item.id}">${icon(item.icon)}<span>${escapeHtml(state.locale === "en" ? item.labelEn : item.labelRu)}</span></button>`).join("")}
			</div>
			${renderAdminLayoutToolbar(selected)}
			${renderAdminLayoutScene(category, visible)}
			${renderAdminHiddenLayoutItems(category, hidden)}
			<div class="admin-ui-editor__footer"><button type="button" data-action="admin-layout-reset-category">${icon("reset")}<span>${state.locale === "en" ? "Reset screen" : "\u0421\u0431\u0440\u043e\u0441\u0438\u0442\u044c \u044d\u043a\u0440\u0430\u043d"}</span></button></div>
		</div>
	`);
}

function ensureAdminVisualLayoutDraft() {
	const draft = state.adminSettingsDraft;
	if (!draft) return;
	if (!draft.layout) draft.layout = {};
	if (!Array.isArray(draft.layout.elements)) draft.layout.elements = [];
	draft.layout.elements = draft.layout.elements.filter((item) => !(item?.area === "dashboard" && ["brand", "subscription", "actions"].includes(item?.id)));
	for (const fallback of ADMIN_LAYOUT_DEFAULTS) {
		const current = draft.layout.elements.find((item) => item?.area === fallback.area && item?.id === fallback.id);
		if (!current) {
			draft.layout.elements.push(deepClone(fallback));
			continue;
		}
		if (!current.align) current.align = fallback.align;
		if (fallback.group && !current.group) current.group = fallback.group;
		if (!Number.isFinite(Number(current.offsetX))) current.offsetX = 0;
		if (!Number.isFinite(Number(current.offsetY))) current.offsetY = 0;
		if (!Number.isFinite(Number(current.width)) || Number(current.width) <= 0) current.width = fallback.width;
		if (!Number.isFinite(Number(current.height)) || Number(current.height) <= 0) current.height = fallback.height;
	}
}

function getAdminLayoutDraftEntries(area) {
	return (state.adminSettingsDraft?.layout?.elements || [])
		.map((item, index) => ({ item, index, key: `${item.area}:${item.id}` }))
		.filter(({ item }) => item?.area === area && !(area === "dashboard" && ["brand", "subscription", "actions"].includes(item?.id)))
		.sort((left, right) => Number(left.item.order || 0) - Number(right.item.order || 0));
}

function getAdminLayoutSelection() {
	if (String(state.adminLayoutSelection || "").startsWith("plan:")) {
		const id = String(state.adminLayoutSelection).slice(5);
		const index = (state.adminSettingsDraft?.plans || []).findIndex((plan) => plan.id === id);
		return index >= 0 ? { type: "plan", item: state.adminSettingsDraft.plans[index], index, key: `plan:${id}` } : null;
	}
	const entries = getAdminLayoutDraftEntries(state.adminLayoutCategory);
	const entry = entries.find(({ key }) => key === state.adminLayoutSelection) || entries.find(({ item }) => item.visible !== false) || entries[0];
	if (!entry) return null;
	state.adminLayoutSelection = entry.key;
	return { type: "layout", ...entry };
}

function adminLayoutMeta(area, id) {
	return ADMIN_LAYOUT_META[`${area}:${id}`] || [id, "grid"];
}

function renderAdminLayoutToolbar(selected) {
	if (!selected) return "";
	if (selected.type === "plan") {
		const label = selected.item.titleRu || selected.item.titleEn || selected.item.id;
		return `<div class="admin-ui-tools" aria-label="${escapeAttribute(label)}"><strong>${escapeHtml(label)}</strong><div>
			<button type="button" data-action="admin-layout-toggle-plan-wide" aria-pressed="${Boolean(selected.item.wide)}" title="${state.locale === "en" ? "Full width" : "\u041d\u0430 \u0432\u0441\u044e \u0448\u0438\u0440\u0438\u043d\u0443"}">${icon("frame")}</button>
			<button type="button" data-action="admin-layout-hide-plan" title="${state.locale === "en" ? "Hide" : "\u0421\u043a\u0440\u044b\u0442\u044c"}">${icon("eyeOff")}</button>
			<button type="button" data-action="admin-layout-reset-plan" title="${state.locale === "en" ? "Reset" : "\u0421\u0431\u0440\u043e\u0441\u0438\u0442\u044c"}">${icon("reset")}</button>
		</div></div>`;
	}
	const [label] = adminLayoutMeta(selected.item.area, selected.item.id);
	return `<div class="admin-ui-tools" aria-label="${escapeAttribute(label)}"><strong>${escapeHtml(label)}</strong><div>
		<button type="button" data-action="admin-layout-toggle-frame" aria-pressed="${Boolean(selected.item.framed)}" title="${state.locale === "en" ? "Frame" : "\u0420\u0430\u043c\u043a\u0430"}">${icon("frame")}</button>
		<button type="button" data-action="admin-layout-hide-item" title="${state.locale === "en" ? "Hide" : "\u0421\u043a\u0440\u044b\u0442\u044c"}">${icon("eyeOff")}</button>
		<button type="button" data-action="admin-layout-reset-item" title="${state.locale === "en" ? "Reset" : "\u0421\u0431\u0440\u043e\u0441\u0438\u0442\u044c"}">${icon("reset")}</button>
	</div></div>`;
}

function renderAdminLayoutScene(area, entries) {
	const category = ADMIN_LAYOUT_CATEGORIES.find((item) => item.id === area);
	const label = state.locale === "en" ? category?.labelEn : category?.labelRu;
	return `<div class="admin-ui-device">
		<div class="admin-ui-device__bar"><span></span><strong>${escapeHtml(label || area)}</strong><span></span></div>
		<div class="admin-ui-canvas admin-ui-canvas--${escapeAttribute(area)}" data-ui-canvas="${escapeAttribute(area)}">
			${entries.map((entry) => renderAdminLayoutNode(entry)).join("") || `<div class="admin-ui-canvas__empty">${state.locale === "en" ? "All elements are hidden" : "\u0412\u0441\u0435 \u044d\u043b\u0435\u043c\u0435\u043d\u0442\u044b \u0441\u043a\u0440\u044b\u0442\u044b"}</div>`}
		</div>
	</div>`;
}

function renderAdminLayoutNode(entry) {
	const { item, index, key } = entry;
	const [label, iconName] = adminLayoutMeta(item.area, item.id);
	const selected = state.adminLayoutSelection === key;
	const isNavigation = item.area === "navigation";
	const width = isNavigation ? Math.max(36, Math.min(72, Number(item.width || 44))) : Math.max(35, Math.min(100, Number(item.width || 100)));
	const height = isNavigation ? Math.max(32, Math.min(64, Number(item.height || 38))) : Math.max(36, Math.min(720, Number(item.height || 52)));
	const style = `--editor-width:${width}${isNavigation ? "px" : "%"};--editor-height:${height}px;--editor-x:${Math.max(-160, Math.min(160, Number(item.offsetX || 0)))}px;--editor-y:${Math.max(-160, Math.min(160, Number(item.offsetY || 0)))}px`;
	return `<article class="admin-ui-node ${selected ? "is-selected" : ""} ${item.framed ? "is-framed" : ""}" tabindex="0" role="button" aria-pressed="${selected}" aria-label="${escapeAttribute(label)}" data-ui-layout-index="${index}" data-ui-layout-key="${escapeAttribute(key)}" data-ui-align="${escapeAttribute(item.align || "left")}" style="${escapeAttribute(style)}">
		<span class="admin-ui-node__grab" aria-hidden="true">${icon("move")}</span>
		<div class="admin-ui-node__content">${renderAdminLayoutPreview(item, label, iconName)}</div>
		<span class="admin-ui-node__resize" data-ui-resize-handle aria-hidden="true">${icon("resize")}</span>
	</article>`;
}

function renderAdminLayoutPreview(item, label, iconName) {
	const key = `${item.area}:${item.id}`;
	if (key === "buy:plans") return `<div class="admin-ui-preview-plans">${renderAdminVisualPlans()}</div>`;
	if (key === "buy:checkout") return `<div class="admin-ui-preview-checkout"><small>\u0412\u044b\u0431\u0440\u0430\u043d\u043d\u044b\u0439 \u0442\u0430\u0440\u0438\u0444</small><strong>6 \u043c\u0435\u0441\u044f\u0446\u0435\u0432</strong><span>${icon("wallet")}\u041a\u0430\u0440\u0442\u0430 / \u0421\u0411\u041f</span><span>\u041f\u0440\u043e\u043c\u043e\u043a\u043e\u0434</span><b>\u041e\u043f\u043b\u0430\u0442\u0438\u0442\u044c 350 \u0420</b></div>`;
	if (key === "support:actions") return `<div class="admin-ui-preview-support-actions"><span>${icon("plus")}<b>\u041d\u043e\u0432\u043e\u0435 \u043e\u0431\u0440\u0430\u0449\u0435\u043d\u0438\u0435</b></span><span>${icon("question")}<b>\u0427\u0430\u0441\u0442\u044b\u0435 \u0432\u043e\u043f\u0440\u043e\u0441\u044b</b></span></div>`;
	if (key === "support:tabs") return `<div class="admin-ui-preview-tabs"><b>\u041e\u0442\u043a\u0440\u044b\u0442\u044b\u0435</b><span>\u0418\u0441\u0442\u043e\u0440\u0438\u044f</span></div>`;
	if (key === "support:tickets") return `<div class="admin-ui-preview-empty">${icon("openTickets")}<strong>\u041d\u0435\u0442 \u043e\u0442\u043a\u0440\u044b\u0442\u044b\u0445 \u043e\u0431\u0440\u0430\u0449\u0435\u043d\u0438\u0439</strong><span>\u0417\u0434\u0435\u0441\u044c \u043f\u043e\u044f\u0432\u044f\u0442\u0441\u044f \u0432\u0430\u0448\u0438 \u0442\u0438\u043a\u0435\u0442\u044b</span></div>`;
	if (item.area === "navigation") return `<div class="admin-ui-preview-nav">${icon(iconName)}<span>${escapeHtml(label)}</span></div>`;
	return `<div class="admin-ui-preview-row"><span>${icon(iconName)}</span><span><strong>${escapeHtml(label)}</strong><small>Link-Bot</small></span><i>${icon("chevronRight")}</i></div>`;
}

function renderAdminVisualPlans() {
	return (state.adminSettingsDraft?.plans || []).map((plan, index) => {
		if (plan.enabled === false) return "";
		const selected = state.adminLayoutSelection === `plan:${plan.id}`;
		return `<div class="admin-ui-plan ${plan.wide ? "is-wide" : ""} ${selected ? "is-selected" : ""}" data-ui-plan-index="${index}" data-ui-plan-id="${escapeAttribute(plan.id)}" tabindex="0" role="button" aria-pressed="${selected}"><strong>${escapeHtml(plan.titleRu || plan.titleEn || plan.id)}</strong><span>${plan.unlimitedTraffic ? "\u0411\u0435\u0437\u043b\u0438\u043c\u0438\u0442\u043d\u044b\u0439 \u0442\u0440\u0430\u0444\u0438\u043a" : `${Number(plan.trafficGb || 0).toLocaleString("ru-RU")} \u0413\u0411`}</span><b>${Number(plan.priceRub || 0)} \u0420</b><i data-ui-plan-resize aria-hidden="true">${icon("resize")}</i></div>`;
	}).join("");
}

function renderAdminHiddenLayoutItems(area, hidden) {
	const disabledPlans = area === "buy" ? (state.adminSettingsDraft?.plans || []).filter((plan) => plan.enabled === false) : [];
	if (!hidden.length && !disabledPlans.length) return "";
	const title = state.locale === "en" ? "Hidden elements" : "\u0421\u043a\u0440\u044b\u0442\u044b\u0435 \u044d\u043b\u0435\u043c\u0435\u043d\u0442\u044b";
	return `<div class="admin-ui-hidden"><strong>${escapeHtml(title)}</strong><div>
		${hidden.map(({ item, key }) => { const [label, iconName] = adminLayoutMeta(item.area, item.id); return `<button type="button" data-action="admin-layout-show-item" data-value="${escapeAttribute(key)}">${icon(iconName)}<span>${escapeHtml(label)}</span></button>`; }).join("")}
		${disabledPlans.map((plan) => `<button type="button" data-action="admin-layout-show-plan" data-value="${escapeAttribute(plan.id)}">${icon("cartShopping")}<span>${escapeHtml(plan.titleRu || plan.titleEn || plan.id)}</span></button>`).join("")}
	</div></div>`;
}

function renderAdminPlansPage() {
	const plans = state.adminSettingsDraft?.plans || [];
	return renderAdminEditorPage(state.locale === "en" ? "Plans" : "Тарифы", `<div class="admin-plan-editor">${plans.map((plan, index) => renderAdminPlanEditor(plan, index)).join("")}</div>`);
}

function adminSquads() {
	const squads = state.data?.admin?.squads || {};
	return {
		internal: Array.isArray(squads.internal) ? squads.internal : [],
		external: Array.isArray(squads.external) ? squads.external : [],
	};
}

function updateAdminSquadSelection(currentValues, uuid, checked) {
	const available = adminSquads().internal.map((item) => String(item.uuid));
	const current = Array.isArray(currentValues) && currentValues.length ? currentValues.map(String) : [...available];
	const selected = new Set(current);
	if (checked) selected.add(String(uuid));
	else selected.delete(String(uuid));
	const result = available.filter((value) => selected.has(value));
	return result.length === available.length ? [] : result;
}

function renderInternalSquadSelector(selectedValues, inputKey) {
	const squads = adminSquads().internal;
	const selected = Array.isArray(selectedValues) ? selectedValues.map(String) : [];
	const allSelected = selected.length === 0;
	if (!squads.length) return `<div class="admin-squads__empty">Список внутренних сквадов недоступен</div>`;
	return `<div class="admin-squads__list">${squads.map((squad) => `<label class="admin-squad-option"><input type="checkbox" data-input="${escapeAttribute(inputKey)}" value="${escapeAttribute(squad.uuid)}" ${allSelected || selected.includes(String(squad.uuid)) ? "checked" : ""}><span><strong>${escapeHtml(squad.name || "Без названия")}</strong><small>${escapeHtml(squad.uuid)}</small></span></label>`).join("")}</div><p class="admin-squads__hint">Если выбраны все, новые пользователи добавляются во все внутренние сквады.</p>`;
}

function renderExternalSquadSelector(selectedValue, inputKey) {
	const squads = adminSquads().external;
	return `<label class="admin-field admin-field--full"><span>Внешний сквад</span><select class="admin-field__control" data-input="${escapeAttribute(inputKey)}"><option value="">Без внешнего сквада</option>${squads.map((squad) => `<option value="${escapeAttribute(squad.uuid)}" ${String(selectedValue || "") === String(squad.uuid) ? "selected" : ""}>${escapeHtml(squad.name || squad.uuid)}</option>`).join("")}</select></label>`;
}

function renderAdminTrialPage() {
	const trial = state.adminSettingsDraft?.trial || {};
	return renderAdminEditorPage("Триал", `<div class="admin-trial-editor">
		<div class="admin-toggle-list">${renderAdminToggle("Триал включен", "trial.enabled")}${renderAdminToggle("Безлимитный трафик", "trial.unlimitedTraffic")}</div>
		<div class="admin-editor__grid admin-editor__grid--three">${renderAdminSettingField("Срок, дней", "trial.days", { type: "number", min: 0, max: 365 })}${renderAdminSettingField("Трафик, ГБ", "trial.trafficGb", { type: "number", min: 0 })}${renderAdminSettingField("Устройств", "trial.deviceLimit", { type: "number", min: 0 })}</div>
		<div class="admin-editor__grid">${renderAdminSettingField("Тег в панели", "trial.tag")}${renderAdminTrialResetStrategy()}</div>
		<section class="admin-squads"><h3>Внутренние сквады</h3>${renderInternalSquadSelector(trial.internalSquadUuids, "admin-trial-internal-squad")}${renderExternalSquadSelector(trial.externalSquadUuid, "admin-trial-external-squad")}</section>
	</div>`);
}

function renderAdminTrialResetStrategy() {
	const value = String(state.adminSettingsDraft?.trial?.trafficResetStrategy || "MONTH");
	return `<label class="admin-field"><span>Сброс трафика</span><select class="admin-field__control" data-setting-path="trial.trafficResetStrategy" data-setting-type="text"><option value="NO_RESET" ${value === "NO_RESET" ? "selected" : ""}>Не сбрасывать</option><option value="DAY" ${value === "DAY" ? "selected" : ""}>Каждый день</option><option value="WEEK" ${value === "WEEK" ? "selected" : ""}>Каждую неделю</option><option value="MONTH" ${value === "MONTH" ? "selected" : ""}>Каждый месяц</option></select></label>`;
}

function renderAdminPlanEditor(plan, index) {
	return `<div class="admin-repeat-row"><div class="admin-repeat-row__head"><strong>${escapeHtml(plan.titleRu || plan.id)}</strong><div class="admin-builder-row__actions"><button type="button" data-action="admin-move-plan" data-value="${index}" data-direction="-1">${icon("arrowUp")}</button><button type="button" data-action="admin-move-plan" data-value="${index}" data-direction="1">${icon("arrowDown")}</button></div></div>
		<div class="admin-toggle-list admin-toggle-list--inline">${renderAdminToggle("Активен", `plans.${index}.enabled`)}${renderAdminToggle("Безлимит", `plans.${index}.unlimitedTraffic`)}${renderAdminToggle("На всю ширину", `plans.${index}.wide`)}</div>
		${renderAdminSettingField("Название", `plans.${index}.titleRu`)}
		<div class="admin-editor__grid">${renderAdminSettingField("Цена ₽", `plans.${index}.priceRub`, { type: "number", min: 0 })}${renderAdminSettingField("Устройств", `plans.${index}.deviceLimit`, { type: "number", min: 0 })}</div>
		${renderAdminSettingField("Трафик, ГБ", `plans.${index}.trafficGb`, { type: "number", min: 0 })}
	</div>`;
}

function renderAdminDiagnosticsPage() {
	const events = state.data?.admin?.events || [];
	return `<section class="page admin-page ${pageClass("admin")}" id="page-admin"><div class="admin-diagnostics-summary"><strong>${events.length}</strong><span>${state.locale === "en" ? "open events" : "открытых событий"}</span></div><div class="admin-event-list">${events.length ? events.map(renderAdminEvent).join("") : `<div class="admin-empty-state">${icon("check")}<strong>${state.locale === "en" ? "No active errors" : "Активных ошибок нет"}</strong></div>`}</div></section>`;
}

function renderAdminEvent(event) {
	return `<article class="admin-event admin-event--${escapeAttribute(event.severity || "warning")}"><div class="admin-event__top"><span>${escapeHtml(event.category || "system")}</span><time>${escapeHtml(formatSupportDate(event.lastSeenAt))}</time></div><strong>${escapeHtml(event.operation || "unknown")}</strong><p>${escapeHtml(event.message || "")}</p><div class="admin-event__foot"><span>${escapeHtml(`${event.occurrenceCount || 1}x`)}</span><button type="button" data-action="admin-resolve-event" data-value="${escapeAttribute(event.id)}">${state.locale === "en" ? "Resolve" : "Закрыть"}</button></div></article>`;
}

function renderAdminEditorPage(title, body) {
	return `<section class="page admin-page ${pageClass("admin")}" id="page-admin"><div class="admin-editor"><h2 class="admin-editor__title">${escapeHtml(title)}</h2>${body}</div>${renderAdminSaveBar()}</section>`;
}

function renderAdminSaveBar(className = "") {
	const busy = state.adminBusy === "save-settings";
	const status = busy
		? (state.locale === "en" ? "Saving..." : "Сохраняем...")
		: state.adminSettingsDirty
			? (state.locale === "en" ? "Unsaved changes" : "Есть несохранённые изменения")
			: (state.locale === "en" ? "Changes saved" : "Изменения сохранены");
	const resetButton = state.adminLayoutEditing
		? `<button class="admin-save-bar__reset" type="button" data-action="admin-layout-reset-category" ${busy ? "disabled" : ""} aria-label="${state.locale === "en" ? "Reset current screen" : "Сбросить текущий экран"}">${icon("reset")}<span>${state.locale === "en" ? "Reset" : "Сбросить"}</span></button>`
		: "";
	return `<div class="admin-save-bar ${className}" role="status" aria-live="polite"><span>${escapeHtml(status)}</span><div class="admin-save-bar__actions">${resetButton}<button type="button" data-action="admin-save-settings" ${busy || !state.adminSettingsDirty ? "disabled" : ""}>${icon(busy ? "refresh" : "check")}<span>${state.locale === "en" ? "Save" : "Сохранить"}</span></button>${state.adminLayoutEditing ? `<button class="admin-save-bar__close" type="button" data-action="admin-layout-exit" aria-label="${state.locale === "en" ? "Exit editor" : "Выйти из редактора"}">${icon("close")}</button>` : ""}</div></div>`;
}

function renderAdminPlanSaveBar() {
	const busy = state.adminBusy === "save-settings";
	const status = busy
		? (state.locale === "en" ? "Saving..." : "Сохраняем...")
		: state.adminSettingsDirty
			? (state.locale === "en" ? "Unsaved changes" : "Есть несохранённые изменения")
			: (state.locale === "en" ? "Changes saved" : "Изменения сохранены");
	return `<div class="admin-save-bar admin-save-bar--plan-editor" role="status" aria-live="polite"><span>${escapeHtml(status)}</span><div class="admin-save-bar__actions"><button class="admin-save-bar__add" type="button" data-action="admin-add-plan" ${busy ? "disabled" : ""}>${icon("plus")}<span>${state.locale === "en" ? "Add" : "Добавить"}</span></button><button class="admin-save-bar__reset" type="button" data-action="admin-reset-plans" ${busy ? "disabled" : ""}>${icon("reset")}<span>${state.locale === "en" ? "Reset" : "Сбросить"}</span></button><button type="button" data-action="admin-save-settings" ${busy || !state.adminSettingsDirty ? "disabled" : ""}>${icon(busy ? "refresh" : "check")}<span>${state.locale === "en" ? "Save" : "Сохранить"}</span></button><button class="admin-save-bar__close" type="button" data-action="admin-plan-exit" aria-label="${state.locale === "en" ? "Exit plan editor" : "Выйти из редактора тарифов"}">${icon("close")}</button></div></div>`;
}

function renderAdminPlanEditorModalLegacy() {
	const plan = state.adminPlanFormDraft || {};
	const english = state.locale === "en";
	return `<div class="modal open"><button class="modal__backdrop" type="button" data-action="admin-close-plan-modal"></button><div class="modal__sheet modal__sheet--plan-editor"><div class="modal__header"><div><div class="section-label">${english ? "Plan editor" : "Редактор тарифа"}</div><div class="modal__title">${english ? "Plan parameters" : "Параметры тарифа"}</div></div><button class="header__btn" type="button" data-action="admin-close-plan-modal">${icon("close")}</button></div><div class="admin-plan-modal__grid"><label class="admin-field"><span>${english ? "Duration, months" : "Срок, месяцев"}</span><input class="admin-field__control" type="number" min="1" max="120" inputmode="numeric" data-input="admin-plan-months" value="${escapeAttribute(plan.months ?? 0)}"></label><label class="admin-field"><span>${english ? "Price, RUB" : "Цена, ₽"}</span><input class="admin-field__control" type="number" min="1" max="1000000" inputmode="numeric" data-input="admin-plan-price" value="${escapeAttribute(plan.priceRub ?? 0)}"></label><label class="admin-field"><span>${english ? "Traffic, GB (0 = unlimited)" : "Трафик, ГБ (0 = безлимит)"}</span><input class="admin-field__control" type="number" min="0" max="1000000" inputmode="numeric" data-input="admin-plan-traffic" value="${escapeAttribute(plan.trafficGb ?? 0)}"></label><label class="admin-field"><span>${english ? "Devices (0 = unlimited)" : "Устройства (0 = безлимит)"}</span><input class="admin-field__control" type="number" min="0" max="1000" inputmode="numeric" data-input="admin-plan-devices" value="${escapeAttribute(plan.deviceLimit ?? 0)}"></label></div><button class="btn btn--green-filled admin-plan-modal__save" type="button" data-action="admin-apply-plan-edit">${icon("check")}${english ? "Apply" : "Применить"}</button></div></div>`;
}

function renderAdminPlanEditorModal() {
	const plan = state.adminPlanFormDraft || {};
	const stars = Math.max(0, Math.round(Number(plan.priceRub || 0) / 1.47));
	return `<div class="modal open"><button class="modal__backdrop" type="button" data-action="admin-close-plan-modal"></button><div class="modal__sheet modal__sheet--plan-editor">
		<div class="modal__header"><div><div class="section-label">РЕДАКТОР ТАРИФА</div><div class="modal__title">Параметры тарифа</div></div><button class="header__btn" type="button" data-action="admin-close-plan-modal">${icon("close")}</button></div>
		<div class="admin-plan-modal__body">
		<div class="admin-plan-modal__grid">
			<label class="admin-field"><span>Срок, месяцев</span><input class="admin-field__control" type="number" min="1" max="120" inputmode="numeric" data-input="admin-plan-months" value="${escapeAttribute(plan.months ?? 0)}"></label>
			<label class="admin-field"><span>Цена, ₽</span><input class="admin-field__control" type="number" min="1" max="1000000" inputmode="numeric" data-input="admin-plan-price" value="${escapeAttribute(plan.priceRub ?? 0)}"></label>
			<label class="admin-field"><span>Трафик, ГБ (0 = безлимит)</span><input class="admin-field__control" type="number" min="0" max="1000000" inputmode="numeric" data-input="admin-plan-traffic" value="${escapeAttribute(plan.trafficGb ?? 0)}"></label>
			<label class="admin-field"><span>Устройства (0 = безлимит)</span><input class="admin-field__control" type="number" min="0" max="1000" inputmode="numeric" data-input="admin-plan-devices" value="${escapeAttribute(plan.deviceLimit ?? 0)}"></label>
		</div>
		<div class="admin-plan-stars"><span>Telegram Stars</span><strong>${escapeHtml(String(stars))}</strong><small>Автоматически: 1 ⭐ = 1,47 ₽</small></div>
		<section class="admin-squads"><h3>Внутренние сквады</h3>${renderInternalSquadSelector(plan.internalSquadUuids, "admin-plan-internal-squad")}${renderExternalSquadSelector(plan.externalSquadUuid, "admin-plan-external-squad")}</section>
		<button class="btn btn--green-filled admin-plan-modal__save" type="button" data-action="admin-apply-plan-edit">${icon("check")}Применить</button>
		</div>
	</div></div>`;
}

function renderAdminSettingField(label, path, options = {}) {
	const value = getDeepValue(state.adminSettingsDraft, path, "");
	const attributes = [`data-setting-path="${escapeAttribute(path)}"`, `data-setting-type="${escapeAttribute(options.type || "text")}"`];
	if (options.min != null) attributes.push(`min="${options.min}"`);
	if (options.max != null) attributes.push(`max="${options.max}"`);
	if (options.placeholder) attributes.push(`placeholder="${escapeAttribute(options.placeholder)}"`);
	const control = options.textarea
		? `<textarea class="admin-field__control admin-field__textarea" rows="${options.rows || 4}" ${attributes.join(" ")}>${escapeHtml(value)}</textarea>`
		: `<input class="admin-field__control" type="${options.type === "number" ? "number" : options.type === "url" ? "url" : "text"}" value="${escapeAttribute(value)}" ${attributes.join(" ")}>`;
	return `<label class="admin-field ${options.compact ? "admin-field--compact" : ""}"><span>${escapeHtml(label)}</span>${control}</label>`;
}

function renderAdminJSONField(label, path, rows = 8) {
	const value = state.adminJSONDrafts[path] ?? JSON.stringify(getDeepValue(state.adminSettingsDraft, path, {}), null, 2);
	return `<label class="admin-field"><span>${escapeHtml(label)}</span><textarea class="admin-field__control admin-field__textarea admin-field__code" rows="${rows}" data-setting-path="${escapeAttribute(path)}" data-setting-type="json">${escapeHtml(value)}</textarea></label>`;
}

function renderAdminToggle(label, path, compact = false) {
	const checked = Boolean(getDeepValue(state.adminSettingsDraft, path, false));
	return `<label class="admin-toggle ${compact ? "admin-toggle--compact" : ""}"><span>${escapeHtml(label)}</span><input type="checkbox" data-setting-path="${escapeAttribute(path)}" data-setting-type="boolean" ${checked ? "checked" : ""}><i aria-hidden="true"></i></label>`;
}

function renderAdminColorField(label, path) {
	const value = String(getDeepValue(state.adminSettingsDraft, path, "#000000"));
	return `<label class="admin-color-field"><input type="color" value="${escapeAttribute(value)}" data-setting-path="${escapeAttribute(path)}" data-setting-type="color"><span><strong>${escapeHtml(label)}</strong><em>${escapeHtml(value)}</em></span></label>`;
}

function renderAdminSubscriptionsPage() {
  const copy = t();
  const item = state.adminSubscriptionResult;
  const status = String(item?.status || "-").toUpperCase();
  const statusActive = status === "ACTIVE";
  const expires = item?.expiresAt ? formatDateLabel(item.expiresAt, state.locale) : "-";
  const currentTelegramID = item?.currentTelegramId ? String(item.currentTelegramId) : "-";
  return `
    <section class="page ${pageClass("admin")}" id="page-admin">
      <div class="card admin-subscription-panel">
        <div class="section-label">${escapeHtml(copy.adminSubscriptionTitle || "Subscription binding")}</div>
        <div class="note note--top">${escapeHtml(copy.adminSubscriptionHint || "")}</div>
        <div class="admin-subscription-search">
          <label class="support-field">
            <span class="support-field__label">${escapeHtml(copy.adminSubscriptionQueryLabel || "ID or username")}</span>
            <input class="support-field__input" type="text" maxlength="128" autocomplete="off" spellcheck="false" placeholder="${escapeAttribute(copy.adminSubscriptionQueryPlaceholder || "")}" value="${escapeAttribute(state.adminSubscriptionQuery)}" data-input="admin-subscription-query">
          </label>
          <button class="btn admin-subscription-find" type="button" data-action="admin-find-subscription" ${state.adminBusy === "find-subscription" ? "disabled" : ""}>${icon(state.adminBusy === "find-subscription" ? "refresh" : "key")}${escapeHtml(copy.adminSubscriptionFind || "Find subscription")}</button>
        </div>
        ${item ? `
          <div class="admin-subscription-result" aria-live="polite">
            <dl class="admin-subscription-details">
              <div><dt>${escapeHtml(copy.adminSubscriptionPanelID || "Panel ID")}</dt><dd>${escapeHtml(String(item.id || "-"))}</dd></div>
              <div><dt>${escapeHtml(copy.adminSubscriptionUsername || "Username")}</dt><dd>${escapeHtml(item.username || "-")}</dd></div>
              <div><dt>${escapeHtml(copy.adminSubscriptionCurrentTelegram || "Current Telegram ID")}</dt><dd>${escapeHtml(currentTelegramID)}</dd></div>
              <div><dt>${escapeHtml(copy.adminSubscriptionStatus || "Status")}</dt><dd class="admin-subscription-status admin-subscription-status--${statusActive ? "active" : "inactive"}">${escapeHtml(status)}</dd></div>
              <div><dt>${escapeHtml(copy.adminSubscriptionExpires || "Expires")}</dt><dd>${escapeHtml(expires)}</dd></div>
            </dl>
            <label class="support-field admin-subscription-target">
              <span class="support-field__label">${escapeHtml(copy.adminSubscriptionTargetLabel || "New Telegram ID")}</span>
              <input class="support-field__input" type="text" inputmode="numeric" autocomplete="off" maxlength="20" placeholder="${escapeAttribute(copy.adminSubscriptionTargetPlaceholder || "")}" value="${escapeAttribute(state.adminSubscriptionTargetTelegramID)}" data-input="admin-subscription-target">
              <span class="support-field__hint">${escapeHtml(copy.adminSubscriptionTargetHint || "")}</span>
            </label>
            <button class="btn admin-subscription-submit" type="button" data-action="admin-rebind-subscription" ${state.adminBusy === "rebind-subscription" ? "disabled" : ""}>${icon(state.adminBusy === "rebind-subscription" ? "refresh" : "key")}${escapeHtml(copy.adminSubscriptionRebind || "Rebind subscription")}</button>
          </div>
        ` : ""}
      </div>
    </section>
  `;
}

function renderAdminPromocodesPage() {
  const copy = t();
  const admin = state.data?.admin || { promoCodes: [] };
  const items = Array.isArray(admin.promoCodes) ? admin.promoCodes : [];
  return `
    <section class="page ${pageClass("admin")}" id="page-admin">
      <div class="admin-editor admin-promo-admin">
      <section class="admin-editor__section admin-promo-panel admin-promo-panel--form">
        <div class="admin-editor__section-head"><h3>${escapeHtml(copy.adminPromoTitle || "Promo codes")}</h3></div>
        <div class="admin-promo-form">
          <label class="admin-field">
            <span>${escapeHtml(copy.adminPromoCodeLabel || copy.promoCode)}</span>
            <input class="admin-field__control" type="text" maxlength="32" placeholder="${escapeAttribute(copy.promoCodePlaceholder || "")}" value="${escapeAttribute(state.adminPromoCodeDraft)}" data-input="admin-promo-code">
          </label>
          <div class="admin-promo-grid">
            <label class="admin-field">
              <span>${escapeHtml(copy.adminPromoDiscountLabel || "Discount, %")}</span>
              <input class="admin-field__control" type="number" min="1" max="99" inputmode="numeric" placeholder="20" value="${escapeAttribute(state.adminPromoDiscountDraft)}" data-input="admin-promo-discount">
            </label>
            <label class="admin-field">
              <span>${escapeHtml(copy.adminPromoLimitLabel || "User limit")}</span>
              <input class="admin-field__control" type="number" min="0" inputmode="numeric" placeholder="${escapeAttribute(copy.adminPromoLimitPlaceholder || "")}" value="${escapeAttribute(state.adminPromoLimitDraft)}" data-input="admin-promo-limit">
            </label>
          </div>
          <label class="admin-field">
            <span>${escapeHtml(copy.adminPromoExpiresLabel || "Valid until")}</span>
            <input class="admin-field__control" type="datetime-local" value="${escapeAttribute(state.adminPromoExpiresDraft)}" data-input="admin-promo-expires">
          </label>
          <button class="btn admin-promo-submit" type="button" data-action="admin-create-promo" ${state.adminBusy === "create-promo" ? "disabled" : ""}>${icon(state.adminBusy === "create-promo" ? "refresh" : "shield")}${escapeHtml(copy.adminPromoCreate || "Create promo code")}</button>
        </div>
      </section>
      <section class="admin-editor__section admin-promo-panel admin-promo-panel--list">
        <div class="admin-editor__section-head"><h3>${escapeHtml(copy.adminPromoListTitle || "Created codes")}</h3><span class="admin-promo-count">${items.length}</span></div>
        ${items.length ? `
          <div class="admin-promo-list">
            ${items.map((item) => renderAdminPromoRow(item)).join("")}
          </div>
        ` : `
          <div class="empty-state">
            <div class="empty-state__icon">${icon("ticketPlus")}</div>
            <div class="empty-state__title">${escapeHtml(copy.adminPromoEmpty || "No promo codes yet")}</div>
          </div>
        `}
      </section>
      </div>
    </section>
  `;
}

function renderDashboardPage() {
  const copy = t();
  const active = isSubscriptionActive();
  const trialEligible = state.data.trial.enabled && state.data.trial.eligible;
  const title = active ? getCurrentSubscriptionPlanLabel() : trialEligible ? getTrialPlanLabel() : getInactiveSubscriptionLabel();
  const expires = active ? `${getUntilLabel()} ${formatShortDateLabel(state.data.subscription.expiresAt, state.locale)}` : "";
  const avatarLabel = getDashboardUserLabel();
  const trafficLabel = active ? formatTrafficBadgeLabel(state.data.subscription.trafficUsedBytes, state.data.subscription.trafficLimitBytes, state.locale) : "";
  const deviceLabel = active ? formatDeviceBadgeLabel(state.data.subscription.deviceUsedCount, state.data.subscription.deviceLimitCount, state.locale) : "";
	const primaryAction = active
		? `<button class="btn" type="button" data-action="go-page" data-value="buy">${icon("cartShopping")}${copy.extend}</button>`
		: `<button class="btn ${trialEligible ? "" : "btn--green"}" type="button" data-action="go-page" data-value="buy">${icon("cart")}${copy.buySubscription}</button>`;
	const secondaryAction = active
		? `<button class="btn btn--green" type="button" data-action="go-page" data-value="setup">${icon("arrowDownSquare")}${copy.setup}</button>`
		: trialEligible
			? `<button class="btn btn--green" type="button" data-action="activate-trial">${icon("cart")}${copy.activateTrial}</button>`
			: "";
	const actionStack = `<div class="action-stack action-stack--dashboard">${renderLayoutDetail("dashboard", "primary_action", primaryAction, "runtime-detail-item--action")}${secondaryAction ? renderLayoutDetail("dashboard", "secondary_action", secondaryAction, "runtime-detail-item--action") : ""}</div>`;

	const blocks = {
		brand: `<div class="hero-center hero-center--brand">${renderLayoutDetail("dashboard", "logo", `<div class="hero-brand" style="--runtime-logo-width:${Math.max(48, Math.min(220, Number(getRuntimeSettings()?.layout?.logoWidth || 188)))}px"><img src="${escapeAttribute(resolveBrandMarkURL(state.data.brand.logoUrl))}" alt="${escapeAttribute(state.data.brand.name || "Link-Bot")}" loading="eager" draggable="false"></div>`, "runtime-detail-item--logo")}${renderLayoutDetail("dashboard", "username", `<div class="hero-handle">${escapeHtml(avatarLabel)}</div>`, "runtime-detail-item--username")}</div>`,
		subscription: `<div class="dashboard-compact"><div class="card card--status card--status-compact"><div class="sub-bar sub-bar--status"><div class="sub-bar__row">${renderLayoutDetail("dashboard", "plan_name", `<div class="sub-bar__name">${title}</div>`, "runtime-detail-item--status")}${active ? renderLayoutDetail("dashboard", "expires", `<div class="sub-bar__date"><span class="sub-bar__date-icon">${icon("calendarDays")}</span><span>${expires}</span></div>`, "runtime-detail-item--status") : ""}</div>${active ? `<div class="sub-bar__row sub-bar__row--pills">${trafficLabel ? renderLayoutDetail("dashboard", "traffic", `<span class="sub-pill"><span class="sub-pill__icon">${icon("chartLine")}</span><span>${escapeHtml(trafficLabel)}</span></span>`, "runtime-detail-item--pill") : ""}${deviceLabel ? renderLayoutDetail("dashboard", "devices", `<button class="sub-pill sub-pill--button" type="button" data-action="open-devices-modal"><span>${escapeHtml(deviceLabel)}</span><span class="sub-pill__edit">${icon("userPen")}</span></button>`, "runtime-detail-item--pill") : ""}</div>` : ""}</div></div></div>`,
		actions: `<div class="dashboard-compact">${actionStack}</div>`,
	};
	return `<section class="page ${pageClass("dashboard")}" id="page-dashboard">${renderRuntimeLayoutArea("dashboard", blocks)}</section>`;
}

function renderBuyPage() {
  const copy = t();
  const plan = getSelectedPlan();
  const method = getSelectedPaymentMethod();
  const promoStatus = getPromoStatus();
  const payLabel = plan && method ? `${copy.pay} ${formatPlanCheckoutPrice(plan, method.id, state.locale)}` : copy.paymentUnavailable;
  const displayedPlans = getDisplayedPlans();
  if (!displayedPlans.length) {
    return `<section class="page page-buy--empty ${state.adminPlanEditing ? "page-buy--admin-editor" : ""} ${pageClass("buy")}" id="page-buy">
      <div class="commerce-empty" role="status">
        <div class="commerce-empty__icon">${icon("cartShopping")}</div>
        <div class="commerce-empty__title">${escapeHtml(copy.noPlansTitle)}</div>
        <div class="commerce-empty__hint">${escapeHtml(state.adminPlanEditing ? copy.adminNoPlansHint : copy.noPlansHint)}</div>
      </div>
    </section>`;
  }
  const planList = `<div class="pricing-list ${state.adminPlanEditing ? "pricing-list--admin" : ""}" style="--plan-columns:${Math.max(1, Math.min(2, Number(getRuntimeSettings()?.layout?.planColumns || 2)))}">${displayedPlans.map((item) => renderPlanCard(item, planKey(item) === planKey(plan))).join("")}</div>`;
  const methodTitle = method?.label || copy.noPaymentMethodsTitle;
  const methodHint = method?.hint || copy.noPaymentMethodsHint;
  const checkoutDisabled = !plan || !method || state.busyMethod || state.adminPlanEditing;
  const checkout = `<div class="card card--checkout">
		<div class="summary-row checkout-summary"><div><div class="summary-row__title">${copy.selectedPlan}</div><div class="summary-row__value">${plan ? getPlanDisplayTitle(plan, state.locale) : "—"}</div></div>${plan?.recommended ? `<span class="badge badge--inline">${copy.best}</span>` : plan?.savingsPercent ? `<span class="badge badge--inline">${copy.savings(plan.savingsPercent)}</span>` : ""}</div>
        <div class="payment-stack">
		<button class="pay-selector checkout-payment ${method ? "" : "checkout-payment--empty"}" type="button" data-action="open-pay-modal" ${method ? "" : "disabled aria-disabled=\"true\""}><span class="pay-selector__icon ${method ? "pay-selector__icon--brand" : ""}">${method ? renderPaymentMethodLogo(method) : icon("wallet")}</span><span class="pay-selector__copy"><strong>${escapeHtml(methodTitle)}</strong><span>${escapeHtml(methodHint)}</span></span><span class="pay-selector__tail">${method ? icon("pencil") : ""}</span></button>
        ${featureEnabled("promocodes") ? `<div class="promo-box checkout-promo">
          <span class="support-field__label">${escapeHtml(copy.promoCode || "Promo code")}</span>
          <div class="promo-box__row">
            <input class="support-field__input promo-box__input" type="text" maxlength="32" autocomplete="off" autocapitalize="characters" spellcheck="false" enterkeyhint="done" aria-describedby="promo-status" placeholder="${escapeAttribute(copy.promoCodePlaceholder || "")}" value="${escapeAttribute(state.promoCodeDraft)}" data-input="promo-code">
          </div>
          <div class="promo-box__status ${promoStatus ? `promo-box__status--${escapeAttribute(promoStatus.type)}` : "promo-box__status--empty"}" id="promo-status" data-promo-status role="status" aria-live="polite">${promoStatus ? escapeHtml(promoStatus.message) : ""}</div>
        </div>` : ""}
        ${state.data.meta?.starsNeedPriorPurchase ? `<div class="note">${copy.starsNeedPriorPurchase}</div>` : ""}
		<button class="btn btn--green-filled buy-action" type="button" data-action="pay-selected" ${checkoutDisabled ? "disabled aria-disabled=\"true\"" : ""}>${icon(state.busyMethod ? "refresh" : "cart")}${payLabel}</button>
        </div>
      </div>`;
  return `<section class="page ${state.adminPlanEditing ? "page-buy--admin-editor" : ""} ${pageClass("buy")}" id="page-buy">${planList}${checkout}</section>`;
}

function renderSetupPage() {
  const copy = t();
  const active = isSubscriptionActive() && state.data.subscription.hasAccessLink;
  const steps = (SETUP_STEPS[state.locale] || SETUP_STEPS.ru)[state.selectedPlatform] || [];
  return `
    <section class="page ${pageClass("setup")}" id="page-setup">
      <div class="setup-orbit"><span class="setup-orbit__ring"></span><span class="setup-orbit__ring"></span><span class="setup-orbit__ring"></span><span class="setup-orbit__ring"></span><span class="setup-orbit__center">${icon(platformIcon(state.selectedPlatform))}</span></div>
      <div class="setup-title">${copy.setupTitle}</div>
      <div class="setup-subtitle">${copy.setupHint}</div>
      <div class="platform-grid">${PLATFORMS.map((platform) => renderPlatformButton(platform, platform === state.selectedPlatform)).join("")}</div>
      ${active ? `
        <div class="card">
          <div class="section-label">${copy.instructions}</div>
          <div class="step-list">${steps.map((step, index) => `<div class="step-item"><span class="step-item__index">${index + 1}</span><span>${escapeHtml(step)}</span></div>`).join("")}</div>
          <div class="setup-access-stack">
            <div class="action-stack action-stack--compact"><button class="btn btn--green-filled" type="button" data-action="open-access">${icon("arrow")}${copy.openAccess}</button></div>
            <div class="quick-link quick-link--flat"><div class="quick-link__copy"><span>${copy.accessLink}</span><strong>${escapeHtml(maskLink(state.data.subscription.subscriptionLink))}</strong></div><button class="quick-link__action" type="button" data-action="copy-access" aria-label="${escapeAttribute(copy.copyAccess)}">${icon("copy")}</button></div>
          </div>
        </div>
      ` : `
        <div class="card">
          <div class="empty-state"><div class="empty-state__icon">${icon("shield")}</div><div class="empty-state__title">${copy.setupMissing}</div><div class="empty-state__desc">${copy.setupMissingHint}</div><button class="btn mt-16" type="button" data-action="go-page" data-value="buy">${icon("cart")}${copy.buySubscription}</button></div>
        </div>
      `}
    </section>
  `;
}

function renderSupportPage() {
  const copy = t();
  const support = state.data.support || { isAdmin: false, openTickets: [], historyTickets: [] };
  const scopy = supportText();
  const showingHistory = state.supportTab === "history";
  const tickets = showingHistory ? (support.historyTickets || []) : (support.openTickets || []);
  const emptyTitle = showingHistory ? scopy.noHistoryTitle : scopy.noOpenTitle;
  const emptyHint = showingHistory ? scopy.noHistoryHint : scopy.noOpenHint;
  const emptyIcon = showingHistory ? "historyTickets" : "openTickets";

  const actions = support.isAdmin ? `` : `
        <div class="support-top-grid">
		<button class="card card--interactive menu-card" type="button" data-action="open-support-compose">
            <span class="menu-card__icon">${icon("ticketPlus")}</span>
            <span><strong class="menu-card__title">${copy.newTicket}</strong><span class="menu-card__hint">${copy.newTicketHint}</span></span>
		</button>
		<button class="card card--interactive menu-card" type="button" data-action="go-page" data-value="faq">
            <span class="menu-card__icon">${icon("question")}</span>
            <span><strong class="menu-card__title">${scopy.faq}</strong><span class="menu-card__hint">${scopy.faqHint}</span></span>
		</button>
        </div>
      `;
	const tabs = `<div class="tabs tabs--support">${SUPPORT_TABS.map((tab) => `<button class="tab ${state.supportTab === tab ? "active" : ""}" type="button" data-action="switch-support-tab" data-value="${tab}">${tab === "open" ? scopy.open : scopy.history}</button>`).join("")}</div>`;
	const adminIntro = support.isAdmin ? `<div class="note note--top support-admin-hint">${escapeHtml(scopy.adminHint)}</div>` : "";
	const ticketContent = tickets.length ? `<div class="support-ticket-list">${tickets.map((ticket) => renderSupportTicketCard(ticket, support.isAdmin)).join("")}</div>` : `<div class="card"><div class="empty-state"><div class="empty-state__icon">${icon(emptyIcon)}</div><div class="empty-state__title">${emptyTitle}</div><div class="empty-state__desc">${emptyHint}</div></div></div>`;
  return `<section class="page page-support ${pageClass("support")}" id="page-support">${actions}${adminIntro}${tabs}${ticketContent}</section>`;
}

function renderSupportTicketCard(ticket, isAdmin) {
  const scopy = supportText();
  const title = supportTicketTitle(ticket);
  const metaParts = [];
  if (isAdmin && ticket.customerName) metaParts.push(ticket.customerName);
  if (ticket.subscriptionLabel) metaParts.push(ticket.subscriptionLabel);

  return `
    <button class="card card--interactive support-ticket-card" type="button" data-action="open-support-ticket" data-value="${ticket.id}">
      <div class="support-ticket-card__top">
        <div class="support-ticket-card__title-wrap">
          <strong class="support-ticket-card__title">${escapeHtml(title)}</strong>
          ${metaParts.length ? `<span class="support-ticket-card__meta">${escapeHtml(metaParts.join(" | "))}</span>` : ""}
        </div>
        ${ticket.unreadCount > 0 ? `<span class="support-ticket-card__badge">${formatNumber(ticket.unreadCount, state.locale)}</span>` : ""}
      </div>
      <div class="support-ticket-card__preview">${escapeHtml(ticket.preview || (isAdmin ? scopy.admin : scopy.you))}</div>
      <div class="support-ticket-card__bottom">
        <span>${escapeHtml(formatSupportStatus(ticket.status))}</span>
        <span>${escapeHtml(formatSupportDate(ticket.updatedAt))}</span>
      </div>
    </button>
  `;
}

function renderFaqPage() {
	const configured = getRuntimeSettings()?.content?.faq?.ru;
	const faqs = Array.isArray(configured) && configured.length
		? configured.map((item) => [item.question || "", item.answer || ""])
		: FAQS.ru;
  return `
    <section class="page ${pageClass("faq")}" id="page-faq">
      ${faqs.map(([question, answer], index) => `<button class="card faq-item ${state.selectedFaqIndex === index ? "open" : ""}" type="button" data-action="toggle-faq" data-value="${index}"><span class="faq-item__header"><span class="faq-item__question">${escapeHtml(question)}</span><span class="faq-item__arrow">${icon("chevron")}</span></span><span class="faq-item__answer">${escapeHtml(answer)}</span></button>`).join("")}
    </section>
  `;
}

function renderReviewsPage() {
  const copy = reviewsText();
  const reviews = state.data?.reviews || { count: 0, average: 0, canCreate: true, items: [], myReview: null };
  const items = Array.isArray(reviews.items) ? reviews.items : [];
  const countLabel = copy.reviewsCount(reviews.count || 0);

  return `
    <section class="page ${pageClass("reviews")}" id="page-reviews">
      <div class="card reviews-hero">
        <div class="reviews-hero__score">${formatAverageRating(reviews.average || 0, state.locale)}</div>
        <div class="reviews-hero__stars">${renderRatingStars(Math.round(reviews.average || 0), false, "reviews-hero__star")}</div>
        <div class="reviews-hero__meta">${escapeHtml(countLabel)}</div>
      </div>
      ${reviews.canCreate ? `
        <button class="btn reviews-cta" type="button" data-action="open-review-compose">${icon("star")}${escapeHtml(copy.leaveReview)}</button>
      ` : `
        <div class="card reviews-mine">
          <div class="reviews-mine__copy">
            <strong>${escapeHtml(copy.thanksTitle)}</strong>
            <span>${escapeHtml(copy.thanksHint)}</span>
          </div>
          <button class="btn reviews-mine__btn" type="button" data-action="open-review-detail" data-value="${escapeAttribute(String(reviews.myReview?.id || 0))}">${escapeHtml(copy.viewMine)}</button>
        </div>
      `}
      ${items.length ? `
        <div class="reviews-list">
          ${items.map((item) => renderReviewCard(item)).join("")}
        </div>
      ` : `
        <div class="card">
          <div class="empty-state">
            <div class="empty-state__icon">${icon("star")}</div>
            <div class="empty-state__title">${escapeHtml(copy.emptyTitle)}</div>
            <div class="empty-state__desc">${escapeHtml(copy.emptyHint)}</div>
          </div>
        </div>
      `}
    </section>
  `;
}

function renderReviewCard(review) {
  const preview = String(review?.comment || "").trim();
  return `
    <button class="card card--interactive review-card" type="button" data-action="open-review-detail" data-value="${escapeAttribute(String(review?.id || 0))}">
      <div class="review-card__top">
        <strong class="review-card__username">${escapeHtml(review?.username || "user")}</strong>
        <div class="review-card__stars">${renderRatingStars(Number(review?.rating || 0), false)}</div>
      </div>
      <div class="review-card__comment">${escapeHtml(preview)}</div>
      <div class="review-card__bottom">
        <span>${escapeHtml(formatReviewDate(review?.createdAt))}</span>
        ${review?.isMine ? `<span class="review-card__badge">${escapeHtml(reviewsText().mine)}</span>` : ""}
      </div>
    </button>
  `;
}

function renderReferralsPage() {
  const copy = t();
  const rewardLabel = formatReferralRewardLabel(state.data?.referral, state.locale);
  return `
    <section class="page ${pageClass("referrals")}" id="page-referrals">
      <div class="section-label">${copy.referralsTitle}</div>
      <div class="note note--top">${copy.referralsHint}</div>
      <div class="card"><div class="action-stack action-stack--compact"><button class="btn" type="button" data-action="share-referral">${icon("share")}${copy.shareTelegram}</button><button class="btn" type="button" data-action="copy-referral">${icon("copy")}${copy.copyReferral}</button></div></div>
      <div class="card"><div class="info-row"><span>${copy.invited}</span><span class="info-row__value">${formatNumber(state.data.referral.count || 0, state.locale)}</span></div><div class="info-row"><span>${copy.bonusDays}</span><span class="info-row__value">${escapeHtml(rewardLabel)}</span></div><div class="info-row"><span>${copy.shareTelegram}</span><span class="info-row__value">${state.data.referral.shareUrl ? escapeHtml(linkHint(state.data.referral.shareUrl)) : "—"}</span></div></div>
      <div class="card"><div class="empty-state"><div class="empty-state__icon">${icon("users")}</div><div class="empty-state__title">${copy.referralsTitle}</div><div class="empty-state__desc">${copy.referralsHint}</div></div></div>
    </section>
  `;
}

function renderServersPage() {
  const copy = t();
  const counts = getServerCounts();
  const items = getVisibleServers();

  return `
    <section class="page ${pageClass("servers")}" id="page-servers">
      <div class="server-summary-grid">
        <div class="server-summary-card"><strong>${formatNumber(counts.total, state.locale)}</strong><span>${copy.serverTotal}</span></div>
        <div class="server-summary-card server-summary-card--online"><strong>${formatNumber(counts.online, state.locale)}</strong><span>${copy.serverOnline}</span></div>
        <div class="server-summary-card server-summary-card--offline"><strong>${formatNumber(counts.offline, state.locale)}</strong><span>${copy.serverOffline}</span></div>
      </div>
      <div class="server-toolbar">
        <div class="tabs server-tabs">
          <button class="tab ${state.serverFilter === "all" ? "active" : ""}" type="button" data-action="set-server-filter" data-value="all">${copy.serverAll}</button>
          <button class="tab ${state.serverFilter === "online" ? "active" : ""}" type="button" data-action="set-server-filter" data-value="online">${copy.serverOnline}</button>
          <button class="tab ${state.serverFilter === "offline" ? "active" : ""}" type="button" data-action="set-server-filter" data-value="offline">${copy.serverOffline}</button>
        </div>
      </div>
      ${items.length ? `<div class="server-list">${items.map((item) => renderServerCard(item)).join("")}</div>` : `<div class="card"><div class="empty-state"><div class="empty-state__icon">${icon("server")}</div><div class="empty-state__title">${copy.serverStatusEmpty}</div><div class="empty-state__desc">${copy.serverStatusHint}</div></div></div>`}
    </section>
  `;
}

function renderSettingsPage() {
	const groups = { main: [], purchases: [], programs: [], help: [], account: [] };
	for (const item of getProfileItems()) groups[item.group]?.push(item);
	const labels = state.locale === "en"
		? { main: "Main", purchases: "Purchases and bonuses", programs: "Programs", help: "Help", account: "Account" }
		: { main: "Главная", purchases: "Покупки и бонусы", programs: "Программы", help: "Помощь", account: "Аккаунт" };
	const orderedGroups = PROFILE_GROUP_ORDER
		.map((key) => [key, groups[key]])
		.filter(([, items]) => state.adminLayoutEditing || items.length);
	return `
		<section class="page profile-page ${state.adminLayoutEditing ? "profile-page--sorting" : ""} ${pageClass("settings")}" id="page-settings">
			${orderedGroups.map(([key, items]) => `
				<section class="profile-group ${state.adminLayoutEditing ? "profile-group--dropzone" : ""}" aria-labelledby="profile-${key}" data-profile-group="${escapeAttribute(key)}">
					<h2 class="profile-group__title" id="profile-${key}"><span></span>${escapeHtml(labels[key])}</h2>
					<div class="profile-group__rows">${items.map(renderProfileItem).join("")}${state.adminLayoutEditing && !items.length ? `<div class="profile-group__empty">${state.locale === "en" ? "Drag a button here" : "Перетащите кнопку сюда"}</div>` : ""}</div>
				</section>
			`).join("")}
		</section>
	`;
}

function getProfileItems() {
	const copy = t();
	const links = state.data?.links || {};
	const definitions = {
		server_status: { group: "main", label: copy.serverStatus, hint: formatServerStatusHint(), action: "go-page", value: "servers", icon: "server", feature: "server_status" },
		media: { group: "main", label: mediaLabel(), hint: mediaHint(), action: "go-page", value: "media", icon: "youtube", feature: "media" },
		news: { group: "main", label: copy.channel, hint: linkHint(links.channel), action: "open-link", value: links.channel, icon: "broadcast" },
		payments: { group: "purchases", label: copy.paymentsTitle || "Payments", hint: copy.paymentsHint || "", action: "go-page", value: "payments", icon: "wallet" },
		referrals: { group: "programs", label: copy.referralSystem, hint: copy.referralsHint, action: "go-page", value: "referrals", icon: "users", feature: "referrals" },
		reviews: { group: "programs", label: copy.feedback, hint: reviewsSummaryHint(), action: "go-page", value: "reviews", icon: "star", feature: "reviews" },
		login_methods: { group: "account", label: loginMethodsLabel(), hint: loginMethodsHint(), action: "go-page", value: "login-methods", icon: "lockAlt" },
		web_version: { group: "account", label: webVersionLabel(), hint: webVersionHint(), action: "open-web-version", value: "", icon: "arrowUpRightSquare", feature: "web_version" },
		pwa_install: { group: "account", label: addToHomeLabel(), hint: addToHomeHint(), action: "open-install-guide", value: "", icon: "download", feature: "pwa_install" },
		terms: { group: "help", label: copy.tos, hint: replaceRuntimeBrandTokens(copy.tosHint || copy.tos), action: "go-page", value: "terms", icon: "doc" },
	};
	const configured = getLayoutElements("profile");
	const order = configured.length ? configured : Object.keys(definitions).map((id, index) => ({ id, order: index, visible: true, width: 100, height: 52, framed: true }));
	const items = order.map((layout) => {
		const item = definitions[layout.id];
		if (!item || ((!item.value && item.action === "open-link") || (item.feature && !featureEnabled(item.feature)))) return null;
		return { ...item, group: layout.group || item.group, id: layout.id, layout };
	}).filter(Boolean);
	const custom = getRuntimeSettings()?.content?.customLinks || [];
	custom.forEach((item, index) => {
		if (!item?.url) return;
		items.push({
			id: `custom.${item.id || index}`,
			group: "main",
			label: item.labelRu,
			hint: item.hintRu,
			action: "open-link",
			value: item.url,
			icon: item.icon || "external",
			layout: { width: 100, height: 52, framed: true },
		});
	});
	return items;
}

function renderProfileItem(item) {
	const layout = item.layout || {};
	const sorting = state.adminLayoutEditing;
	return `<button class="profile-row ${layout.framed === false ? "profile-row--flat" : ""} ${sorting ? "profile-row--sortable" : ""}" ${sorting ? `data-profile-layout-id="${escapeAttribute(item.id)}"` : ""} type="button" data-action="${escapeAttribute(item.action)}" ${item.value ? `data-value="${escapeAttribute(item.value)}"` : ""}><span class="profile-row__icon">${icon(item.icon)}</span><span class="profile-row__body"><strong>${escapeHtml(item.label || item.id)}</strong>${item.hint ? `<span>${escapeHtml(item.hint)}</span>` : ""}</span><span class="profile-row__tail">${icon(sorting ? "move" : "chevronRight")}</span></button>`;
}

function renderInstallGuidePage() {
  const platform = state.installGuidePlatform === "ios" ? "ios" : "android";
  const guide = INSTALL_GUIDES[platform];
  const nextPlatform = platform === "ios" ? "android" : "ios";

  return `
    <section class="install-guide install-guide--${platform}" aria-labelledby="install-guide-title">
      <div class="install-guide__card">
        <div class="install-guide__platform">${icon(guide.icon)}</div>
        <h1 class="install-guide__title" id="install-guide-title">${escapeHtml(guide.title)}</h1>
        <div class="install-guide__browser"><span>${icon(guide.browserIcon)}</span><span>${escapeHtml(guide.browser)}</span></div>
        <div class="install-guide__steps">
          ${guide.steps.map((step, index) => renderInstallGuideStep(step, index)).join("")}
        </div>
        <div class="install-guide__actions ${!guide.otherBrowser ? "install-guide__actions--single" : ""}">
          ${guide.otherBrowser ? `<button class="install-guide__link" type="button" data-action="open-web-version">${escapeHtml(guide.otherBrowser)}</button>` : ""}
          <button class="install-guide__link" type="button" data-action="set-install-platform" data-value="${nextPlatform}">${escapeHtml(guide.alternate)}</button>
        </div>
      </div>
    </section>
  `;
}

function renderInstallGuideStep(step, index) {
  return `
    <div class="install-step">
      <div class="install-step__rail">
        <span class="install-step__number">${index + 1}</span>
      </div>
      <div class="install-step__icon">${icon(step.icon)}</div>
      <div class="install-step__copy">
        <strong>${escapeHtml(step.title)}</strong>
        <span>${escapeHtml(step.text)}</span>
      </div>
    </div>
  `;
}

function renderMediaPage() {
  return `
    <section class="page page-media ${pageClass("media")}" id="page-media">
      <div class="card media-empty-card">
        <div class="empty-state media-empty-state">
          <div class="empty-state__icon media-empty-state__icon">${icon("youtube")}</div>
          <div class="empty-state__title">${escapeHtml(mediaComingSoonLabel())}</div>
        </div>
      </div>
    </section>
  `;
}

function renderLoginMethodsPage() {
  const user = state.data?.user || {};
  const googleLinked = Boolean(user.googleLinked);
  const telegramLinked = user.telegramLinked !== false;
  const gmailEmail = String(user.googleEmail || "").trim();
  const providerLabel = String(user.authProvider || "telegram") === "google" ? gmailLabel() : telegramLabel();
  const gmailStatus = googleLinked ? (gmailEmail || authLinkedLabel()) : authNotLinkedLabel();
  const gmailAction = googleLinked ? "" : `
    <div class="login-method-action">
      <div class="google-button-shell google-button-shell--method" ${state.loginMethodBusy === "google" ? "data-google-disabled=\"1\"" : ""}>
        <button class="btn btn--google" type="button" data-action="google-link-login" ${state.loginMethodBusy === "google" ? "disabled" : ""}>
          <span class="btn__icon" aria-hidden="true">${icon("googleColor")}</span>
          <span>${escapeHtml(state.loginMethodBusy === "google" ? (state.locale === "en" ? "Linking..." : "\u041f\u0440\u0438\u0432\u044f\u0437\u044b\u0432\u0430\u0435\u043c...") : gmailLinkButtonLabel())}</span>
        </button>
      </div>
    </div>
  `;

  return `
    <section class="page ${pageClass("login-methods")}" id="page-login-methods">
      <div class="card login-method-card">
        <div class="login-method-card__head">
          <span class="login-method-card__icon">${icon("lockAlt")}</span>
          <div>
            <div class="section-label">${escapeHtml(loginMethodsLabel())}</div>
            <div class="login-method-card__title">${escapeHtml(state.locale === "en" ? "Account sign-in" : "\u0412\u0445\u043e\u0434 \u0432 \u0430\u043a\u043a\u0430\u0443\u043d\u0442")}</div>
            <div class="login-method-card__hint">${escapeHtml(state.locale === "en" ? `Current method: ${providerLabel}` : `\u0421\u0435\u0439\u0447\u0430\u0441 \u0432\u0445\u043e\u0434: ${providerLabel}`)}</div>
          </div>
        </div>
        <div class="login-method-list">
          <div class="login-method-row">
            <span class="login-method-row__icon">${icon("telegram")}</span>
            <span class="login-method-row__body"><strong>${escapeHtml(telegramLabel())}</strong><span>${escapeHtml(telegramLinked ? authLinkedLabel() : authNotLinkedLabel())}</span></span>
            <span class="login-method-row__status ${telegramLinked ? "is-linked" : ""}">${escapeHtml(telegramLinked ? authLinkedLabel() : authNotLinkedLabel())}</span>
          </div>
          <div class="login-method-row">
            <span class="login-method-row__icon">${icon("google")}</span>
            <span class="login-method-row__body"><strong>${escapeHtml(gmailLabel())}</strong><span>${escapeHtml(googleLinked ? gmailStatus : gmailLinkHint())}</span></span>
            <span class="login-method-row__status ${googleLinked ? "is-linked" : ""}">${escapeHtml(gmailStatus)}</span>
          </div>
        </div>
        ${gmailAction}
      </div>
    </section>
  `;
}

function renderPaymentsPage() {
  const copy = t();
  const payments = state.data?.payments || {};
  const history = Array.isArray(payments.history) ? payments.history : [];

  return `
    <section class="page ${pageClass("payments")}" id="page-payments">
      <div class="card payments-history">
        <div class="section-label">${escapeHtml(copy.paymentHistory || "Purchase history")}</div>
        ${history.length ? `<div class="payment-list">${history.map((item) => renderPaymentHistoryItem(item)).join("")}</div>` : `<div class="empty-state"><div class="empty-state__icon">${icon("historyTickets")}</div><div class="empty-state__title">${escapeHtml(copy.paymentHistoryEmpty || "No purchases yet")}</div></div>`}
      </div>
    </section>
  `;
}

function renderTermsPage() {
  const article = getRuntimeTermsArticle();
  const effectiveDate = formatTermsDate(state.data?.meta?.now || new Date().toISOString());

  return `
    <section class="page ${pageClass("terms")}" id="page-terms">
      <article class="terms-article">
        <div class="terms-article__eyebrow">${escapeHtml(article.title)}</div>
        <h1 class="terms-article__title">${escapeHtml(article.title)}</h1>
        <div class="terms-article__meta">${escapeHtml(article.effectiveLabel)} — ${escapeHtml(effectiveDate)}</div>
        <p class="terms-article__lead"><strong>${escapeHtml(article.jurisdiction)}</strong></p>
        ${article.intro.map((paragraph) => `<p class="terms-article__lead">${escapeHtml(paragraph)}</p>`).join("")}
        <div class="terms-article__divider"></div>
        ${article.sections.map(renderTermsSection).join("")}
        <section class="terms-article__section">
          <h2 class="terms-article__section-title">${escapeHtml(article.contactTitle)}</h2>
          <ul class="terms-article__list">
            ${article.contacts.map((item) => `<li>${escapeHtml(item)}</li>`).join("")}
          </ul>
        </section>
        <div class="terms-article__footer">${escapeHtml(article.footer)}</div>
      </article>
    </section>
  `;
}

function getRuntimeTermsArticle() {
  const source = TERMS_ARTICLE[state.locale] || TERMS_ARTICLE.ru;
	return replaceRuntimeBrandTokens(source);
}

function replaceRuntimeBrandTokens(source) {
  const content = getRuntimeSettings()?.content || {};
  const brandName = String(content.brandName || state.data?.brand?.name || "Link-Bot").trim() || "Link-Bot";
  const supportURL = String(content.links?.support || state.data?.links?.support || "").trim();
  const supportMatch = supportURL.match(/(?:https?:\/\/)?t\.me\/([A-Za-z0-9_]{5,32})/i);
  const adminContact = String(content.adminContact || (supportMatch ? `@${supportMatch[1]}` : "@your_support_username")).trim();
  const replaceTokens = (value) => {
    if (Array.isArray(value)) return value.map(replaceTokens);
    if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, replaceTokens(item)]));
    if (typeof value !== "string") return value;
    return value.replaceAll("Link-Bot", brandName).replaceAll("@your_support_username", adminContact);
  };
  return replaceTokens(source);
}

function renderTermsSection(section) {
  const paragraphs = (section.paragraphs || []).map((paragraph) => `<p class="terms-article__paragraph">${escapeHtml(paragraph)}</p>`).join("");
  const items = Array.isArray(section.items) && section.items.length ? `
    <ul class="terms-article__list">
      ${section.items.map((item) => `<li>${escapeHtml(item)}</li>`).join("")}
    </ul>
  ` : "";
  return `
    <section class="terms-article__section">
      <h2 class="terms-article__section-title">${escapeHtml(section.title)}</h2>
      ${paragraphs}
      ${items}
    </section>
  `;
}

function renderThemeSwitch() {
  const checked = state.theme === "light";
  return `
    <label class="theme-switch" aria-label="${escapeAttribute(t().theme)}">
      <input class="theme-switch__input" data-input="theme-toggle" type="checkbox" aria-label="${escapeAttribute(t().theme)}" ${checked ? "checked" : ""}>
      <span class="theme-switch__slider"></span>
    </label>
  `;
}

function renderStateScreen(kind, message = "", meta = null) {
	if (kind === "maintenance") {
		const title = meta?.titleRu || "Технические работы";
		const text = meta?.textRu || "Сервис временно недоступен. Попробуйте немного позже.";
		const reason = meta?.reasonRu || "Плановые работы";
		return `
			<div class="state-screen state-screen--maintenance">
				<section class="maintenance-card" aria-labelledby="maintenance-title">
					<div class="maintenance-card__icon">${icon("maintenanceKey")}</div>
					<div class="maintenance-card__eyebrow">${escapeHtml(getRuntimeSettings()?.content?.brandName || "Link-Bot")}</div>
					<h1 class="maintenance-card__title" id="maintenance-title">${escapeHtml(title)}</h1>
					<p class="maintenance-card__text">${escapeHtml(text)}</p>
					<div class="maintenance-card__reason"><span>Причина</span><strong>${escapeHtml(reason)}</strong></div>
					<div class="maintenance-card__waiting" role="status" aria-label="Технические работы"><span></span><span></span><span></span></div>
				</section>
			</div>
		`;
	}
  if (kind === "loading") return `
    <div class="state-screen state-screen--loader">
      <div class="loader" aria-label="${escapeAttribute(t().appName)} loading">
        <svg id="cloud" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" aria-hidden="true">
          <defs>
            <filter id="roundness">
              <feGaussianBlur in="SourceGraphic" stdDeviation="1.5"></feGaussianBlur>
              <feColorMatrix values="1 0 0 0 0 0 1 0 0 0 0 0 1 0 0 0 0 0 20 -10"></feColorMatrix>
            </filter>
            <mask id="shapes" maskUnits="userSpaceOnUse" maskContentUnits="userSpaceOnUse" x="0" y="0" width="100" height="100">
              <g fill="white">
                <polygon points="50 37.5 82 75 18 75 50 37.5"></polygon>
                <circle cx="20" cy="60" r="15"></circle>
                <circle cx="36" cy="56" r="16"></circle>
                <circle cx="50" cy="48" r="20"></circle>
                <circle cx="64" cy="56" r="16"></circle>
                <circle cx="80" cy="60" r="15"></circle>
              </g>
            </mask>
            <mask id="clipping" maskUnits="userSpaceOnUse" maskContentUnits="userSpaceOnUse" x="0" y="0" width="100" height="100">
              <g id="lines" filter="url(#roundness)">
                <g mask="url(#shapes)" stroke="white">
                  <line x1="-50" y1="-40" x2="150" y2="-40"></line>
                  <line x1="-50" y1="-31" x2="150" y2="-31"></line>
                  <line x1="-50" y1="-22" x2="150" y2="-22"></line>
                  <line x1="-50" y1="-13" x2="150" y2="-13"></line>
                  <line x1="-50" y1="-4" x2="150" y2="-4"></line>
                  <line x1="-50" y1="5" x2="150" y2="5"></line>
                  <line x1="-50" y1="14" x2="150" y2="14"></line>
                  <line x1="-50" y1="23" x2="150" y2="23"></line>
                  <line x1="-50" y1="32" x2="150" y2="32"></line>
                  <line x1="-50" y1="41" x2="150" y2="41"></line>
                  <line x1="-50" y1="50" x2="150" y2="50"></line>
                  <line x1="-50" y1="59" x2="150" y2="59"></line>
                  <line x1="-50" y1="68" x2="150" y2="68"></line>
                  <line x1="-50" y1="77" x2="150" y2="77"></line>
                  <line x1="-50" y1="86" x2="150" y2="86"></line>
                  <line x1="-50" y1="95" x2="150" y2="95"></line>
                  <line x1="-50" y1="104" x2="150" y2="104"></line>
                  <line x1="-50" y1="113" x2="150" y2="113"></line>
                  <line x1="-50" y1="122" x2="150" y2="122"></line>
                  <line x1="-50" y1="131" x2="150" y2="131"></line>
                  <line x1="-50" y1="140" x2="150" y2="140"></line>
                </g>
              </g>
            </mask>
          </defs>
          <rect x="0" y="0" width="100" height="100" rx="0" ry="0" mask="url(#clipping)"></rect>
          <g>
            <path d="M33.52,68.12 C35.02,62.8 39.03,58.52 44.24,56.69 C49.26,54.93 54.68,55.61 59.04,58.4 C59.04,58.4 56.24,60.53 56.24,60.53 C55.45,61.13 55.68,62.37 56.63,62.64 C56.63,62.64 67.21,65.66 67.21,65.66 C67.98,65.88 68.75,65.3 68.74,64.5 C68.74,64.5 68.68,53.5 68.68,53.5 C68.67,52.51 67.54,51.95 66.75,52.55 C66.75,52.55 64.04,54.61 64.04,54.61 C57.88,49.79 49.73,48.4 42.25,51.03 C35.2,53.51 29.78,59.29 27.74,66.49 C27.29,68.08 28.22,69.74 29.81,70.19 C30.09,70.27 30.36,70.31 30.63,70.31 C31.94,70.31 33.14,69.44 33.52,68.12Z"></path>
            <path d="M69.95,74.85 C68.35,74.4 66.7,75.32 66.25,76.92 C64.74,82.24 60.73,86.51 55.52,88.35 C50.51,90.11 45.09,89.43 40.73,86.63 C40.73,86.63 43.53,84.51 43.53,84.51 C44.31,83.91 44.08,82.67 43.13,82.4 C43.13,82.4 32.55,79.38 32.55,79.38 C31.78,79.16 31.02,79.74 31.02,80.54 C31.02,80.54 31.09,91.54 31.09,91.54 C31.09,92.53 32.22,93.09 33.01,92.49 C33.01,92.49 35.72,90.43 35.72,90.43 C39.81,93.63 44.77,95.32 49.84,95.32 C52.41,95.32 55,94.89 57.51,94.01 C64.56,91.53 69.99,85.75 72.02,78.55 C72.47,76.95 71.54,75.3 69.95,74.85Z"></path>
          </g>
        </svg>
      </div>
    </div>
  `;
  if (kind === "telegram") {
    const copy = browserAuthCopy();
    return `
      <div class="state-screen state-screen--browser-auth">
        <section class="browser-auth" aria-labelledby="browser-auth-title">
          <img class="browser-auth__logo" src="${BRAND_MARK_URL}" alt="Link-Bot">
          <div class="browser-auth__eyebrow">Link-Bot Web</div>
          <h1 class="browser-auth__title" id="browser-auth-title">${escapeHtml(copy.title)}</h1>
          <p class="browser-auth__text">${escapeHtml(copy.text)}</p>
          <div class="browser-auth__actions">
            <div class="telegram-login-widget" id="telegram-login-widget" aria-live="polite"></div>
            <button class="browser-auth__telegram" type="button" data-action="telegram-browser-login">
              <span class="browser-auth__telegram-icon" aria-hidden="true">${icon("telegram")}</span>
              <span>${escapeHtml(telegramLoginButtonLabel())}</span>
            </button>
            <div class="google-button-shell google-button-shell--browser" data-google-mode="login">
              <button class="browser-auth__google" type="button" data-action="google-browser-login">
                <span class="browser-auth__google-icon" aria-hidden="true">${icon("googleColor")}</span>
                <span>${escapeHtml(gmailLoginButtonLabel())}</span>
              </button>
              ${getGoogleClientID() ? `<div class="google-login-widget google-login-widget--overlay" data-google-mode="login" aria-hidden="true"></div>` : ""}
            </div>
          </div>
        </section>
      </div>
    `;
  }
  if (kind === "subscription") {
    const copy = t();
    const channelTitle = String(meta?.channelTitle || copy.appName || "Link-Bot");
    const channelUrl = String(meta?.channelUrl || "");
    const imageUrl = String(meta?.imageUrl || "");
    return `
      <div class="state-screen">
        <div class="verify-gate">
          <div class="verify-gate__message">
            ${imageUrl ? `<img class="verify-gate__media" src="${escapeAttribute(imageUrl)}" alt="${escapeAttribute(channelTitle)}">` : ""}
            <div class="verify-gate__body">
              <div class="verify-gate__title">${escapeHtml(copy.subscriptionGateTitle)}</div>
              <p class="verify-gate__text">${escapeHtml(copy.subscriptionGateLead(channelTitle))}</p>
              <p class="verify-gate__text">${escapeHtml(copy.subscriptionGateNews)}</p>
              <p class="verify-gate__text">${escapeHtml(copy.subscriptionGateHint)}</p>
            </div>
          </div>
          <div class="verify-gate__actions">
            ${channelUrl ? `<button class="verify-gate__button" type="button" data-action="open-link" data-value="${escapeAttribute(channelUrl)}"><span class="verify-gate__button-text">${escapeHtml(copy.subscriptionGateOpen)}</span><span class="verify-gate__button-icon">${icon("external")}</span></button>` : ""}
            <button class="verify-gate__button" type="button" data-action="refresh"><span class="verify-gate__button-text">${escapeHtml(copy.subscriptionGateRetry)}</span></button>
          </div>
        </div>
      </div>
    `;
  }
  return `<div class="state-screen"><div class="state-card"><div class="state-card__eyebrow">${escapeHtml(t().appName)}</div><div class="state-card__title">${escapeHtml(t().errorTitle)}</div><div class="state-card__text">${escapeHtml(message)}</div><button class="btn mt-16" type="button" data-action="refresh">${icon("refresh")}${escapeHtml(t().retry)}</button></div></div>`;
}

function renderBottomNav() {
  const pages = getBottomNavPages();
  const activePage = getBottomNavActivePage();
  let activeIndex = pages.indexOf(activePage);
  if (activeIndex < 0) activeIndex = previousBottomNavIndex >= 0 ? previousBottomNavIndex : 0;
  const previousIndex = previousBottomNavIndex >= 0 ? previousBottomNavIndex : activeIndex;
  pendingBottomNavAnimation = {
    activeIndex,
    previousIndex,
    shouldAnimate: previousBottomNavIndex >= 0 && previousIndex !== activeIndex,
  };
  previousBottomNavIndex = activeIndex;

  return `
    <nav class="bottom-nav" style="--nav-active-index: ${activeIndex}; --nav-prev-index: ${previousIndex}; --nav-count: ${pages.length};" data-active-index="${activeIndex}" data-prev-index="${previousIndex}">
      <span class="bottom-nav__indicator" aria-hidden="true"></span>
      ${pages.map((page) => renderBottomNavItem(page, activePage)).join("")}
    </nav>
  `;
}

function renderBottomNavItem(page, activePage = getBottomNavActivePage()) {
  const label = getPageTitle(page, true);
  return `<button class="bottom-nav__item ${activePage === page ? "active" : ""}" type="button" data-action="go-page" data-value="${page}" aria-label="${escapeAttribute(label)}" title="${escapeAttribute(label)}"><span class="bottom-nav__icon">${icon(bottomNavIcon(page))}</span><span class="bottom-nav__label">${escapeHtml(label)}</span></button>`;
}

function syncBottomNavIndicator() {
  const nav = app.querySelector(".bottom-nav");
  const indicator = nav?.querySelector(".bottom-nav__indicator");
  const items = nav ? Array.from(nav.querySelectorAll(".bottom-nav__item")) : [];
  if (!nav || !indicator || !items.length) return;

  const pending = pendingBottomNavAnimation;
  pendingBottomNavAnimation = null;
  const activeIndex = clampIndex(Number(nav.dataset.activeIndex), items.length);
  const previousIndex = clampIndex(Number(nav.dataset.prevIndex), items.length, activeIndex);
  const activeItem = items[activeIndex] || items[0];
  const previousItem = items[previousIndex] || activeItem;
  const indicatorWidth = 22;
  const indicatorHeight = 3;
  const navRect = nav.getBoundingClientRect();
  const previousRect = previousItem.getBoundingClientRect();
  const activeRect = activeItem.getBoundingClientRect();
  const from = Math.round(previousRect.left - navRect.left + ((previousRect.width - indicatorWidth) / 2));
  const to = Math.round(activeRect.left - navRect.left + ((activeRect.width - indicatorWidth) / 2));
  const reduceMotion = Boolean(window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches);

  (indicator.getAnimations?.() || []).forEach((animation) => animation.cancel());
  indicator.style.width = `${indicatorWidth}px`;
  indicator.style.height = `${indicatorHeight}px`;
  indicator.style.transition = "none";

  if (!pending?.shouldAnimate || reduceMotion || from === to) {
    indicator.style.transform = `translate3d(${to}px, 0, 0)`;
    return;
  }

  const fromTransform = `translate3d(${from}px, 0, 0) scaleX(.9)`;
  const toTransform = `translate3d(${to}px, 0, 0) scaleX(1)`;
  indicator.style.transform = fromTransform;

  if (typeof indicator.animate === "function") {
    const animation = indicator.animate(
      [{ transform: fromTransform }, { transform: toTransform }],
      { duration: 340, easing: "cubic-bezier(.22, 1, .36, 1)", fill: "both" },
    );
    animation.onfinish = () => {
      indicator.style.transform = `translate3d(${to}px, 0, 0)`;
      animation.cancel();
    };
    return;
  }

  indicator.offsetWidth;
  indicator.style.transition = "transform .34s cubic-bezier(.22, 1, .36, 1)";
  requestAnimationFrame(() => {
    indicator.style.transform = toTransform;
  });
}

function clampIndex(value, length, fallback = 0) {
  const index = Number.isFinite(value) ? value : fallback;
  return Math.max(0, Math.min(length - 1, index));
}

function renderSidebarPageItem(page, iconName, label) {
  return `<button class="sidebar__item ${state.currentPage === page ? "active" : ""}" type="button" data-action="go-page" data-value="${page}"><span class="sidebar__item-icon">${icon(iconName)}</span><span class="sidebar__item-label">${escapeHtml(label)}</span></button>`;
}

function renderSidebarLinkItem(url, iconName, label) {
  return `<button class="sidebar__item" type="button" data-action="open-link" data-value="${escapeAttribute(url)}"><span class="sidebar__item-icon">${icon(iconName)}</span><span class="sidebar__item-label">${escapeHtml(label)}</span><span class="sidebar__item-ext">${icon("external")}</span></button>`;
}

function renderSupportLinkRows() {
  const copy = t();
  const links = state.data.links || {};
  return [
    links.support ? renderMenuRow(copy.support, linkHint(links.support), "open-link", links.support, "chat") : "",
    renderMenuRow(copy.serverStatus, formatServerStatusHint(), "go-page", "servers", "server"),
    renderMenuRow(copy.feedback, reviewsSummaryHint(), "go-page", "reviews", "star"),
    links.channel ? renderMenuRow(copy.channel, linkHint(links.channel), "open-link", links.channel, "broadcast") : "",
  ].filter(Boolean).join("");
}

function renderServerCard(server) {
  const flag = countryFlag(server.countryCode);
  return `
    <div class="card server-card">
      <div class="server-card__row">
        <span class="server-card__dot ${server.online ? "is-online" : "is-offline"}"></span>
        <div class="server-card__copy">
          <strong>
            ${flag ? `<span class="server-card__flag" aria-hidden="true">${flag}</span>` : ""}
            <span class="server-card__name">${escapeHtml(server.name)}</span>
          </strong>
        </div>
      </div>
    </div>
  `;
}

function getServerItems() {
  return state.data?.servers?.items || [];
}

function getServerCounts() {
  const items = getServerItems();
  const online = items.filter((item) => item.online).length;
  return { total: items.length, online, offline: Math.max(0, items.length - online) };
}

function getVisibleServers() {
  const items = getServerItems();
  if (state.serverFilter === "online") return items.filter((item) => item.online);
  if (state.serverFilter === "offline") return items.filter((item) => !item.online);
  return items;
}

function formatServerStatusHint() {
  const counts = getServerCounts();
  return `${counts.online}/${counts.total} ${t().serverOnline}`;
}

function getPlanCardTitle(months, locale) {
  if (locale === "en") {
    switch (Number(months || 0)) {
      case 1: return "1 month";
      case 3: return "3 months";
      case 6: return "6 months";
      case 12: return "12 months";
      default: return `${months} months`;
    }
  }

  switch (Number(months || 0)) {
    case 1: return "1 месяц";
    case 3: return "3 месяца";
    case 6: return "6 месяцев";
    case 12: return "12 месяцев";
    default: return `${months} месяцев`;
  }
}

function getPlanBaseTitle(plan, locale) {
	const configuredTitle = locale === "en" ? plan?.titleEn : plan?.titleRu;
	return String(configuredTitle || "").trim() || getPlanCardTitle(plan?.months, locale);
}

function planHasUnlimitedTraffic(plan) {
	if (String(plan?.variant || "") === "unlimited" || plan?.unlimitedTraffic === true) return true;
	return plan && plan.trafficLimitBytes !== undefined && Number(plan.trafficLimitBytes) <= 0;
}

function getPlanDisplayTitle(plan, locale) {
	const title = getPlanBaseTitle(plan, locale);
	if (!planHasUnlimitedTraffic(plan)) return title;
	return locale === "en" ? `${title} · Unlimited` : `${title} · Безлимит`;
}

function getPlanDetails(planOrMonths, locale) {
  const isEn = locale === "en";
  const plan = typeof planOrMonths === "object" && planOrMonths ? planOrMonths : null;
  const months = Number((plan ? plan.months : planOrMonths) || 0);
  const trafficLimit = plan ? Number(plan.trafficLimitBytes || 0) : NaN;
  const deviceLimit = plan ? Number(plan.deviceLimitCount || 0) : NaN;

  if (plan && Number.isFinite(trafficLimit)) {
    const traffic = trafficLimit <= 0
      ? (isEn ? "Unlimited traffic" : "Безлимитный трафик")
      : `${formatTrafficLimitValue(trafficLimit, locale)} ${isEn ? "GB" : "ГБ"}`;
    const devices = Number.isFinite(deviceLimit) && deviceLimit <= 0
      ? (isEn ? "Unlimited devices" : "Безлимит устройств")
      : `${isEn ? "Up to" : "До"} ${formatNumber(deviceLimit || 5, locale)} ${isEn ? "devices" : "устройств"}`;
    return { traffic, devices };
  }

  switch (months) {
    case 1:
      return { traffic: isEn ? "150 GB" : "150 ГБ", devices: isEn ? "Up to 5 devices" : "До 5 устройств" };
    case 3:
      return { traffic: isEn ? "500 GB" : "500 ГБ", devices: isEn ? "Up to 7 devices" : "До 7 устройств" };
    case 6:
      return { traffic: isEn ? "1,000 GB" : "1,000 ГБ", devices: isEn ? "Up to 10 devices" : "До 10 устройств" };
    case 12:
      return { traffic: isEn ? "Unlimited traffic" : "Безлимитный трафик", devices: isEn ? "Unlimited devices" : "Безлимит устройств" };
    default:
      return { traffic: isEn ? "Traffic included" : "Трафик включён", devices: isEn ? "Up to 5 devices" : "До 5 устройств" };
  }
}

function planPricePrefix(locale) {
  return locale === "en" ? "from" : "от";
}

function renderMenuRow(label, hint, action, value, iconName, options = {}) {
  const { showTail = action === "open-link", tailIcon = "chevronRight" } = options;
  return `<button class="menu-row" type="button" data-action="${action}" ${value ? `data-value="${escapeAttribute(value)}"` : ""}><span class="menu-row__icon">${icon(iconName)}</span><span class="menu-row__body"><strong>${escapeHtml(label)}</strong><span>${escapeHtml(hint)}</span></span>${showTail ? `<span class="menu-row__tail">${icon(tailIcon)}</span>` : ""}</button>`;
}

function renderMetric(label, value) {
  return `<div class="metric"><span>${escapeHtml(label)}</span><strong>${escapeHtml(value)}</strong></div>`;
}

function renderPlanCard(plan, selected) {
  const key = planKey(plan);
  const details = getPlanDetails(plan, state.locale);
  const hasDuration = Number(plan?.months || 0) > 0;
  const unlimited = hasDuration && planHasUnlimitedTraffic(plan);
  const title = hasDuration ? getPlanBaseTitle(plan, state.locale) : (state.locale === "en" ? "New plan" : "Новый тариф");
  const price = Number(plan?.priceRub || 0) > 0 ? formatCurrency(plan.priceRub, state.locale) : "—";
  const content = `
      <div class="pricing-card__content">
        <div class="pricing-card__copy">
          <div class="pricing-card__name-row">
			<div class="pricing-card__name">${escapeHtml(title)}</div>
			${unlimited ? `<span class="pricing-card__unlimited-badge">${state.locale === "en" ? "Unlimited" : "Безлимит"}</span>` : ""}
          </div>
          <div class="pricing-card__spec">${escapeHtml(Number(plan?.months || 0) > 0 ? details.traffic : (state.locale === "en" ? "Set traffic" : "Укажите трафик"))}</div>
          <div class="pricing-card__spec">${escapeHtml(Number(plan?.months || 0) > 0 ? details.devices : (state.locale === "en" ? "Set device limit" : "Укажите устройства"))}</div>
        </div>
        <div class="pricing-card__price-stack">
          <div class="pricing-card__price-row">
            <div class="pricing-card__price-line"><strong>${escapeHtml(price)}</strong></div>
          </div>
        </div>
      </div>
  `;
	if (state.adminPlanEditing) {
		const moveLabel = state.locale === "en" ? "Drag to reorder plan" : "Перетащить тариф";
		const widthLabel = plan.wide
			? (state.locale === "en" ? "Make plan compact" : "Сделать тариф узким")
			: (state.locale === "en" ? "Make plan full width" : "Растянуть тариф");
		return `<article class="pricing-card pricing-card--admin ${unlimited ? "pricing-card--unlimited" : ""} ${plan.wide ? "pricing-card--wide" : ""} ${plan.enabled === false ? "is-draft" : ""}" data-admin-plan-id="${escapeAttribute(key)}">${content}<div class="pricing-card__admin-actions"><button class="pricing-card__admin-drag" type="button" data-admin-plan-drag aria-label="${escapeAttribute(moveLabel)}" title="${escapeAttribute(moveLabel)}">${icon("move")}</button><button type="button" data-action="admin-toggle-plan-wide" data-value="${escapeAttribute(key)}" aria-label="${escapeAttribute(widthLabel)}" title="${escapeAttribute(widthLabel)}" aria-pressed="${Boolean(plan.wide)}">${icon("resize")}</button><button type="button" data-action="admin-edit-plan" data-value="${escapeAttribute(key)}" aria-label="${state.locale === "en" ? "Edit plan" : "Редактировать тариф"}" title="${state.locale === "en" ? "Edit plan" : "Редактировать тариф"}">${icon("pencil")}</button><button type="button" data-action="admin-delete-plan" data-value="${escapeAttribute(key)}" aria-label="${state.locale === "en" ? "Delete plan" : "Удалить тариф"}" title="${state.locale === "en" ? "Delete plan" : "Удалить тариф"}">${icon("trash")}</button></div></article>`;
	}
  return `<button class="pricing-card ${unlimited ? "pricing-card--unlimited" : ""} ${plan.wide ? "pricing-card--wide" : ""} ${selected ? "selected" : ""}" type="button" data-action="select-plan" data-value="${escapeAttribute(key)}">${content}</button>`;
}

function renderPaymentHistoryItem(item) {
  const copy = t();
  const planLabel = item.planLabel || getPlanCardTitle(item.months, state.locale);
  const amountLabel = formatPaymentAmount(item.amount, item.currency, item.invoiceType);
  const method = paymentHistoryMethodMeta(item, copy);
  const statusLabel = formatPaymentStatus(item.status);
  const dateLabel = formatPaymentDate(item.paidAt || item.createdAt);

  return `
    <div class="payment-item payment-item--${escapeAttribute(method.id)}">
      <span class="payment-item__logo" title="${escapeAttribute(method.label)}"><img src="${escapeAttribute(method.logo)}" alt="${escapeAttribute(method.label)}" loading="lazy"></span>
      <div class="payment-item__main">
        <strong>${escapeHtml(planLabel)}</strong>
        <span>${escapeHtml(dateLabel)}</span>
      </div>
      <div class="payment-item__meta">
        <span>${escapeHtml(amountLabel)}</span>
        <span class="payment-item__status payment-item__status--${escapeAttribute(String(item.status || "unknown").toLowerCase())}">${escapeHtml(statusLabel)}</span>
        ${item.isAutoPayment ? `<span class="badge badge--inline">${escapeHtml(copy.autopayTitle || "Autopay")}</span>` : ""}
      </div>
    </div>
  `;
}

function paymentHistoryMethodMeta(item, copy) {
  const invoiceType = String(item?.invoiceType || "").toLowerCase();
  const title = String(item?.paymentMethodTitle || "").trim();
  const normalized = `${invoiceType} ${title}`.toLowerCase();
  const providers = ["lava", "wata", "platega", "freekassa", "heleket", "pally"];
  const provider = providers.find((name) => normalized.includes(name));
  if (provider) {
    const meta = paymentMethodMeta(provider);
    return { id: provider, label: title || meta?.label || provider, logo: PAYMENT_LOGO_URLS[provider] };
  }
  if (invoiceType === "telegram" || normalized.includes("star") || normalized.includes("звезд") || normalized.includes("звёзд")) {
    return { id: "stars", label: title || copy.payMethodStars || "Telegram Stars", logo: PAYMENT_LOGO_URLS.stars };
  }
  if (invoiceType === "crypto" || normalized.includes("crypto") || normalized.includes("крипт")) {
    return { id: "crypto", label: title || copy.payMethodCrypto || "CryptoPay", logo: PAYMENT_LOGO_URLS.crypto };
  }
  if (normalized.includes("сбп") || normalized.includes("sbp")) {
    return { id: "sbp", label: title || copy.payMethodSbp || "СБП", logo: PAYMENT_LOGO_URLS.sbp };
  }
  return { id: "card", label: title || paymentMethodTitleForHistory(invoiceType, copy), logo: PAYMENT_LOGO_URLS.card };
}

function renderAdminPromoRow(item) {
  const copy = t();
  const statusKey = item?.status === "expired"
    ? "adminPromoStatusExpired"
    : item?.status === "inactive"
      ? "adminPromoStatusInactive"
      : item?.status === "exhausted"
        ? "adminPromoStatusExhausted"
        : "adminPromoStatusActive";
  const limit = Number(item?.maxRedemptions || 0);
  const used = Number(item?.redemptionCount || 0);
  const active = item?.status === "active";
  const expiresLabel = item?.expiresAt
    ? `${copy.promoExpiresAt || "Valid until"} ${formatShortDateLabel(item.expiresAt, state.locale)}`
    : (copy.adminPromoNoExpiry || "No expiry");
  const createdLabel = formatDateLabel(item?.createdAt, state.locale);
  return `
    <div class="admin-promo-row admin-promo-row--${active ? "active" : "inactive"}">
      <div class="admin-promo-row__main">
        <div class="admin-promo-row__title">
          <strong class="admin-promo-row__code">${escapeHtml(item?.code || "")}</strong>
          <span class="admin-promo-row__status">${escapeHtml(copy[statusKey] || item?.status || "Active")}</span>
        </div>
        <div class="admin-promo-row__meta">
          <span>-${formatNumber(item?.discountPercent || 0, state.locale)}%</span>
          <span>${escapeHtml(typeof copy.adminPromoUsage === "function" ? copy.adminPromoUsage(used, limit) : `${used}/${limit || "∞"}`)}</span>
          ${!limit ? `<span>${escapeHtml(copy.adminPromoUnlimited || "Unlimited")}</span>` : ""}
          <span>${escapeHtml(expiresLabel)}</span>
        </div>
      </div>
      <div class="admin-promo-row__side">
        <button class="admin-promo-row__delete" type="button" data-action="admin-delete-promo" data-value="${escapeAttribute(item?.id || "")}" ${state.adminBusy === `delete-${item?.id}` ? "disabled" : ""} aria-label="${escapeAttribute(copy.adminPromoDelete || "Delete")}">${icon(state.adminBusy === `delete-${item?.id}` ? "refresh" : "trash")}</button>
        <span>${escapeHtml(createdLabel)}</span>
      </div>
    </div>
  `;
}

function renderPlatformButton(platform, selected) {
  const label = { windows: "Windows", android: "Android", iphone: "iPhone", mac: "macOS" }[platform];
  return `<button class="platform-btn ${selected ? "selected" : ""}" type="button" data-action="select-platform" data-value="${platform}"><span class="platform-btn__icon">${icon(platformIcon(platform))}</span><span class="platform-btn__label">${label}</span></button>`;
}

function renderPayModal() {
  const copy = t();
  return `<div class="modal open ${modalStateClass("pay")}"><button class="modal__backdrop" type="button" data-action="close-pay-modal"></button><div class="modal__sheet"><div class="modal__header"><div><div class="section-label">${copy.paymentMethod}</div><div class="modal__title">${copy.choosePaymentMethod}</div></div><button class="header__btn" type="button" data-action="close-pay-modal" aria-label="${state.locale === "en" ? "Close payment methods" : "Закрыть способы оплаты"}">${icon("close")}</button></div><div class="menu-list">${getAvailableMethods().map((method) => `<button class="pay-row ${state.paymentMethod === method.id ? "selected" : ""}" type="button" data-action="select-pay-method" data-value="${method.id}" aria-pressed="${state.paymentMethod === method.id}"><span class="pay-row__icon pay-row__icon--brand">${renderPaymentMethodLogo(method)}</span><span class="pay-row__copy"><strong>${escapeHtml(method.label)}</strong><span>${escapeHtml(method.hint)}</span></span><span class="pay-row__check">${state.paymentMethod === method.id ? icon("check") : ""}</span></button>`).join("") || `<div class="note">${copy.paymentUnavailable}</div>`}</div></div></div>`;
}

function renderPaymentLaunchModal() {
  const copy = t();
  return `
    <div class="modal open ${modalStateClass("payment-launch")}">
      <button class="modal__backdrop" type="button" data-action="close-payment-launch"></button>
      <div class="modal__sheet modal__sheet--payment-launch">
        <div class="modal__header">
          <div>
            <div class="section-label">${escapeHtml(copy.paymentMethod || "Payment")}</div>
            <div class="modal__title">${escapeHtml(copy.paymentBrowserTitle || "Open payment")}</div>
          </div>
          <button class="header__btn" type="button" data-action="close-payment-launch">${icon("close")}</button>
        </div>
        <div class="note note--top">${escapeHtml(copy.paymentBrowserText || "We'll open the payment in your external browser.")}</div>
        <div class="action-stack action-stack--compact">
          <button class="btn" type="button" data-action="launch-payment-browser">${icon("external")}${escapeHtml(copy.paymentBrowserOpen || "Open in browser")}</button>
        </div>
      </div>
    </div>
  `;
}

function renderDevicesModal() {
  const copy = deviceText();
  const devices = getSubscriptionDevices();
  const used = formatNumber(state.data?.subscription?.deviceUsedCount || 0, state.locale);
  const limit = formatNumber(state.data?.subscription?.deviceLimitCount || 0, state.locale);

  return `
    <div class="modal open ${modalStateClass("devices")}">
      <button class="modal__backdrop" type="button" data-action="close-devices-modal"></button>
      <div class="modal__sheet modal__sheet--devices">
        <div class="modal__header modal__header--devices">
          <div>
            <div class="modal__title">${escapeHtml(copy.title)}</div>
            <div class="device-modal__subtitle">${escapeHtml(copy.connected(used, limit))}</div>
          </div>
          <button class="header__btn" type="button" data-action="close-devices-modal">${icon("close")}</button>
        </div>
        ${devices.length ? `<div class="device-list">${devices.map((device) => renderDeviceCard(device)).join("")}</div>` : `
          <div class="device-empty">
            <div class="device-empty__icon">${icon("users")}</div>
            <strong>${escapeHtml(copy.emptyTitle)}</strong>
            <span>${escapeHtml(copy.emptyHint)}</span>
          </div>
        `}
      </div>
    </div>
  `;
}

function renderReviewComposerModal() {
  const copy = reviewsText();
  return `
    <div class="modal open ${modalStateClass("review-compose")}">
      <button class="modal__backdrop" type="button" data-action="close-review-compose"></button>
      <div class="modal__sheet modal__sheet--review">
        <div class="modal__header">
          <div>
            <div class="section-label">${escapeHtml(t().feedback)}</div>
            <div class="modal__title">${escapeHtml(copy.leaveReview)}</div>
          </div>
          <button class="header__btn" type="button" data-action="close-review-compose">${icon("close")}</button>
        </div>
        <div class="review-field">
          <span class="review-field__label">${escapeHtml(copy.ratingLabel)}</span>
          <div class="review-stars review-stars--input">${renderRatingStars(state.reviewDraftRating, true)}</div>
        </div>
        <label class="support-field">
          <span class="support-field__label">${escapeHtml(copy.commentLabel)}</span>
          <textarea class="support-field__textarea review-field__textarea" rows="5" maxlength="2000" placeholder="${escapeAttribute(copy.commentPlaceholder)}" data-input="review-comment">${escapeHtml(state.reviewDraftComment)}</textarea>
        </label>
        <div class="review-bonus-note">${escapeHtml(copy.rewardHint)}</div>
        <button class="btn reviews-submit" type="button" data-action="submit-review" ${state.reviewBusy ? "disabled" : ""}>${icon(state.reviewBusy ? "refresh" : "send")}${escapeHtml(copy.submit)}</button>
      </div>
    </div>
  `;
}

function renderReviewDetailModal() {
  const copy = reviewsText();
  const review = getActiveReview();
  if (!review) return "";
  const deleteBusy = state.reviewBusy === `delete-review-${Number(review.id || 0)}`;
  return `
    <div class="modal open ${modalStateClass("review-detail")}">
      <button class="modal__backdrop" type="button" data-action="close-review-detail"></button>
      <div class="modal__sheet modal__sheet--review-detail">
        <div class="modal__header">
          <div>
            <div class="section-label">${escapeHtml(t().feedback)}</div>
            <div class="modal__title">${escapeHtml(review.username || "user")}</div>
          </div>
          <button class="header__btn" type="button" data-action="close-review-detail">${icon("close")}</button>
        </div>
        <div class="review-detail__stars">${renderRatingStars(Number(review.rating || 0), false, "review-detail__star")}</div>
        <div class="review-detail__date">${escapeHtml(formatReviewDate(review.createdAt))}</div>
        <div class="review-detail__comment">${escapeHtml(review.comment || "")}</div>
        ${isAdminUser() ? `
          <div class="review-detail__actions">
            <button class="btn btn--ghost review-detail__delete" type="button" data-action="admin-delete-review" data-value="${escapeAttribute(String(review.id || 0))}" ${deleteBusy ? "disabled" : ""}>${icon(deleteBusy ? "refresh" : "trash")}${escapeHtml(copy.delete)}</button>
          </div>
        ` : ""}
      </div>
    </div>
  `;
}

function renderDeviceCard(device) {
  const copy = deviceText();
  const busy = state.deviceBusyHwid === device.hwid;
  const title = getDeviceTitle(device);
  const secondary = getDeviceSecondary(device);
  return `
    <div class="device-card">
      <div class="device-card__top">
        <strong class="device-card__title">${escapeHtml(title)}</strong>
        <button class="device-card__delete ${busy ? "is-loading" : ""}" type="button" data-action="delete-device" data-value="${escapeAttribute(device.hwid || "")}" ${busy ? "disabled" : ""} aria-label="${escapeAttribute(copy.title)}">${icon(busy ? "refresh" : "trash")}</button>
      </div>
      ${secondary ? `<div class="device-card__meta">${escapeHtml(secondary)}</div>` : ""}
      <div class="device-card__foot">${escapeHtml(`${copy.added} ${formatDeviceCreatedAt(device.createdAt)}`)}</div>
    </div>
  `;
}

function renderSupportComposerModal() {
  const scopy = supportText();
  return `
    <div class="modal open ${modalStateClass("support-compose")}">
      <button class="modal__backdrop" type="button" data-action="close-support-compose"></button>
      <div class="modal__sheet modal__sheet--support">
        <div class="modal__header">
          <div>
            <div class="section-label">${escapeHtml(t().supportTitle)}</div>
            <div class="modal__title">${escapeHtml(scopy.createTitle)}</div>
          </div>
          <button class="header__btn" type="button" data-action="close-support-compose">${icon("close")}</button>
        </div>
        <label class="support-field">
          <span class="support-field__label">${escapeHtml(scopy.subject)}</span>
          <input class="support-field__input" type="text" maxlength="120" placeholder="${escapeAttribute(scopy.subjectPlaceholder)}" value="${escapeAttribute(state.supportDraftSubject)}" data-input="support-subject">
        </label>
        <label class="support-field">
          <span class="support-field__label">${escapeHtml(scopy.message)}</span>
          <textarea class="support-field__textarea" rows="6" maxlength="2000" placeholder="${escapeAttribute(scopy.messagePlaceholder)}" data-input="support-message">${escapeHtml(state.supportDraftMessage)}</textarea>
        </label>
        <button class="btn btn--green-filled" type="button" data-action="submit-support-ticket" ${state.supportBusy ? "disabled" : ""}>${icon(state.supportBusy === "create-ticket" ? "refresh" : "send")}${escapeHtml(scopy.send)}</button>
      </div>
    </div>
  `;
}

function renderSupportThreadModal() {
  const thread = state.activeSupportThread;
  const scopy = supportText();
  if (!thread) {
    return `
      <div class="modal open ${modalStateClass("support-thread")}">
        <button class="modal__backdrop" type="button" data-action="close-support-thread"></button>
        <div class="modal__sheet modal__sheet--thread">
          <div class="modal__header">
            <div><div class="section-label">${escapeHtml(t().supportTitle)}</div><div class="modal__title">${escapeHtml(scopy.loadingThread)}</div></div>
            <button class="header__btn" type="button" data-action="close-support-thread">${icon("close")}</button>
          </div>
        </div>
      </div>
    `;
  }

  const ticket = thread.ticket || {};
  const support = state.data.support || {};
  const title = supportTicketTitle(ticket);
  const metaLines = [];
  if (support.isAdmin) {
    if (ticket.customerName) metaLines.push(`${scopy.customer}: ${ticket.customerName}`);
    if (ticket.customerUsername) metaLines.push(`Telegram: ${formatTelegramUsername(ticket.customerUsername)}`);
    if (ticket.subscriptionLabel) metaLines.push(`${scopy.subscription}: ${ticket.subscriptionLabel}`);
  }

  return `
    <div class="modal open ${modalStateClass("support-thread")}">
      <button class="modal__backdrop" type="button" data-action="close-support-thread"></button>
      <div class="modal__sheet modal__sheet--thread">
        <div class="modal__header support-thread__header">
          <div class="support-thread__headcopy">
            <div class="section-label">${escapeHtml(t().supportTitle)}</div>
            <div class="modal__title">${escapeHtml(title)}</div>
            ${metaLines.length ? `<div class="support-thread__meta">${metaLines.map((line) => `<span>${escapeHtml(line)}</span>`).join("")}</div>` : ""}
          </div>
          <div class="support-thread__headactions">
            ${thread.canClose ? `<button class="support-thread__close" type="button" data-action="close-support-ticket" ${state.supportBusy ? "disabled" : ""}>${escapeHtml(scopy.closeTicket)}</button>` : ""}
            <button class="header__btn" type="button" data-action="close-support-thread">${icon("close")}</button>
          </div>
        </div>
        <div class="support-thread__messages" id="support-thread-messages">
          ${(thread.messages || []).map((message) => renderSupportMessage(message)).join("")}
        </div>
        ${thread.canReply ? `
          <div class="support-reply">
            <textarea class="support-reply__textarea" rows="3" maxlength="2000" placeholder="${escapeAttribute(scopy.replyPlaceholder)}" data-input="support-reply">${escapeHtml(state.supportReplyDraft)}</textarea>
            <button class="btn btn--green-filled support-reply__send" type="button" data-action="send-support-message" ${state.supportBusy ? "disabled" : ""}>${icon(state.supportBusy === "send-support-message" ? "refresh" : "send")}${escapeHtml(scopy.send)}</button>
          </div>
        ` : `
          <div class="support-thread__closed">
            <strong>${escapeHtml(scopy.closed)}</strong>
            <span>${escapeHtml(scopy.closedHint)}</span>
          </div>
        `}
      </div>
    </div>
  `;
}

function renderSupportMessage(message) {
  const scopy = supportText();
  const fromAdmin = message.authorRole === "admin";
  const viewerIsAdmin = Boolean(state.data?.support?.isAdmin);
  const isMine = viewerIsAdmin ? fromAdmin : !fromAdmin;
  const peerName = String(state.activeSupportThread?.ticket?.customerName || "").trim() || (state.locale === "en" ? "Customer" : "Пользователь");
  const authorLabel = isMine ? scopy.you : (fromAdmin ? scopy.admin : peerName);
  return `
    <div class="support-message ${isMine ? "support-message--mine" : "support-message--peer"} ${fromAdmin ? "support-message--admin-author" : "support-message--customer-author"} ${message.pending ? "support-message--pending" : ""}">
      <div class="support-message__bubble">
        <span class="support-message__author">${escapeHtml(authorLabel)}</span>
        <div class="support-message__body">${escapeHtml(message.body || "")}</div>
        <span class="support-message__time">${escapeHtml(formatSupportDate(message.createdAt))}</span>
      </div>
    </div>
  `;
}

function bindRootActions() {
  if (bindRootActions.bound) return;
  bindRootActions.bound = true;

  app.addEventListener("click", async (event) => {
    const target = event.target.closest("[data-action]");
    if (!target) return;
    const action = target.dataset.action;
    const value = target.dataset.value || "";
		const editableNode = event.target.closest?.("[data-layout-edit-key]");
		if (state.adminLayoutEditing && editableNode) {
			selectAdminLayoutNode(editableNode);
			if (suppressNextLayoutClick) {
				suppressNextLayoutClick = false;
				event.preventDefault();
				return;
			}
			const navigationTap = editableNode.classList.contains("bottom-nav__item") && action === "go-page" && value !== "admin";
			if (!navigationTap) {
				event.preventDefault();
				return;
			}
		}
		if (state.adminLayoutEditing && event.target.closest?.("[data-profile-layout-id]")) {
			event.preventDefault();
			return;
		}

    try {
      if (action === "refresh") return await refreshDashboard({ forceSubscriptionCheck: Boolean(state.subscriptionGate) });
      if (action === "telegram-browser-login") return await startTelegramBrowserLogin(target);
      if (action === "google-browser-login") return await startGoogleLogin("login", target);
      if (action === "google-link-login") return await startGoogleLogin("link", target);
      if (action === "open-sidebar") { state.sidebarOpen = true; render(); return; }
      if (action === "close-sidebar") { state.sidebarOpen = false; render(); return; }
      if (action === "open-admin-section") {
		if (value === "layout") return enterAdminLayoutEditor();
		if (value === "plans") return enterAdminPlanEditor();
		state.adminSection = value || "home";
		haptic("light");
		render();
		if (value === "broadcast") void refreshAdminBroadcast({ forceButtons: true });
		return;
	  }
      if (action === "close-admin-section") { state.adminSection = "home"; state.adminBroadcastConfirmOpen = false; haptic("light"); render(); return; }
			if (action === "admin-layout-exit") return exitAdminLayoutEditor();
			if (action === "admin-plan-exit") return exitAdminPlanEditor();
			if (action === "admin-add-plan") return addAdminPlan();
			if (action === "admin-reset-plans") return resetAdminPlans();
			if (action === "admin-toggle-plan-wide") return toggleAdminPlanWide(value);
			if (action === "admin-edit-plan") return openAdminPlanEditor(value);
			if (action === "admin-delete-plan") return deleteAdminPlan(value);
			if (action === "admin-close-plan-modal") return closeAdminPlanEditorModal();
			if (action === "admin-apply-plan-edit") return applyAdminPlanEdit();
      if (action === "go-home") return setPage("dashboard");
      if (action === "go-page") return value === "setup" ? openSubscriptionAccess() : setPage(value);
      if (action === "open-review-compose") { state.reviewComposeOpen = true; render(); return; }
      if (action === "close-review-compose") return requestModalClose("review-compose", () => { state.reviewComposeOpen = false; state.reviewDraftRating = 0; state.reviewDraftComment = ""; state.reviewBusy = ""; });
      if (action === "set-review-rating") { state.reviewDraftRating = Number(value) || 0; haptic("light"); render(); return; }
      if (action === "submit-review") return await submitReview();
      if (action === "open-review-detail") { state.activeReviewId = Number(value) || 0; state.reviewDetailOpen = Boolean(state.activeReviewId); render(); return; }
      if (action === "close-review-detail") return requestModalClose("review-detail", () => { state.activeReviewId = 0; state.reviewDetailOpen = false; });
      if (action === "admin-delete-review") return await deleteAdminReview(Number(value));
      if (action === "open-devices-modal") { state.devicesModalOpen = true; render(); return; }
      if (action === "close-devices-modal") return requestModalClose("devices", () => { state.devicesModalOpen = false; state.deviceBusyHwid = ""; });
      if (action === "delete-device") return await deleteDevice(value);
      if (action === "switch-support-tab") { state.supportTab = SUPPORT_TABS.includes(value) ? value : "open"; render(); void refreshSupport({ silent: true }); return; }
      if (action === "open-support-compose") { state.supportComposeOpen = true; render(); return; }
      if (action === "close-support-compose") return requestModalClose("support-compose", () => { state.supportComposeOpen = false; state.supportDraftSubject = ""; state.supportDraftMessage = ""; });
      if (action === "open-support-ticket") return await openSupportTicket(Number(value));
      if (action === "close-support-thread") return requestModalClose("support-thread", () => { state.supportThreadOpen = false; state.activeSupportTicketId = 0; state.activeSupportThread = null; state.supportReplyDraft = ""; });
      if (action === "submit-support-ticket") return await submitSupportTicket();
      if (action === "send-support-message") return await sendSupportMessage();
      if (action === "close-support-ticket") return await closeSupportTicket();
      if (action === "set-server-filter") { state.serverFilter = ["all", "online", "offline"].includes(value) ? value : "all"; render(); return; }
      if (action === "toggle-faq") { const index = Number(value); state.selectedFaqIndex = state.selectedFaqIndex === index ? -1 : index; render(); return; }
      if (action === "select-platform") { state.selectedPlatform = PLATFORMS.includes(value) ? value : "windows"; haptic("light"); render(); return; }
      if (action === "select-plan") {
        const selectedPlan = (state.data?.plans || []).find((plan) => planKey(plan) === value);
        if (selectedPlan) {
          state.selectedPlanId = planKey(selectedPlan);
          state.selectedPlanMonths = selectedPlan.months;
          ensureSelections();
        }
        haptic("light");
        render();
        return;
      }
      if (action === "open-pay-modal") { state.payModalOpen = true; render(); return; }
      if (action === "close-pay-modal") return requestModalClose("pay", () => { state.payModalOpen = false; });
      if (action === "close-payment-launch") return requestModalClose("payment-launch", () => { state.paymentLaunchModalOpen = false; state.paymentLaunchURL = ""; state.paymentLaunchPurchaseId = 0; });
      if (action === "launch-payment-browser") return openPreparedPaymentInBrowser();
      if (action === "select-pay-method") {
        state.paymentMethod = value;
        writeSetting(STORAGE_KEYS.payMethod, value);
        return requestModalClose("pay", () => { state.payModalOpen = false; });
      }
      if (action === "apply-promo") return await applyPromoCode();
      if (action === "pay-selected") return await startPayment();
      if (action === "admin-create-promo") return await createAdminPromoCode();
      if (action === "admin-delete-promo") return await deleteAdminPromoCode(Number(value));
      if (action === "admin-find-subscription") return await findAdminSubscription();
      if (action === "admin-rebind-subscription") return await rebindAdminSubscription();
			if (action === "admin-broadcast-capture") return await startAdminBroadcastCapture();
			if (action === "admin-broadcast-add-button") return addAdminBroadcastButton();
			if (action === "admin-broadcast-remove-button") return removeAdminBroadcastButton(Number(value));
			if (action === "admin-broadcast-set-style") return setAdminBroadcastButtonStyle(Number(target.dataset.broadcastIndex), value);
			if (action === "admin-broadcast-save-buttons") return await saveAdminBroadcastButtons();
			if (action === "admin-broadcast-preview") return await previewAdminBroadcast();
			if (action === "admin-broadcast-open-confirm") { state.adminBroadcastConfirmOpen = true; render({ preserveScroll: true }); return; }
			if (action === "admin-broadcast-close-confirm") { state.adminBroadcastConfirmOpen = false; render({ preserveScroll: true }); return; }
			if (action === "admin-broadcast-send") return await sendAdminBroadcast();
			if (action === "admin-broadcast-reset") return await resetAdminBroadcast();
			if (action === "admin-layout-category") return setAdminLayoutCategory(value);
			if (action === "admin-layout-toggle-frame") return toggleSelectedAdminLayoutFrame();
			if (action === "admin-layout-hide-item") return hideSelectedAdminLayoutItem();
			if (action === "admin-layout-show-item") return showAdminLayoutItem(value);
			if (action === "admin-layout-reset-item") return resetSelectedAdminLayoutItem();
			if (action === "admin-layout-reset-category") return resetAdminLayoutCategory();
			if (action === "admin-layout-toggle-plan-wide") return toggleSelectedAdminPlanWide();
			if (action === "admin-layout-hide-plan") return hideSelectedAdminPlan();
			if (action === "admin-layout-show-plan") return showAdminPlan(value);
			if (action === "admin-layout-reset-plan") return resetSelectedAdminPlan();
			if (action === "admin-test-reminder") return await testAdminSubscriptionReminder(value);
			if (action === "admin-test-success") return await testAdminSubscriptionSuccess();
			if (action === "admin-save-settings") return await saveAdminSettings();
			if (action === "admin-content-section") { state.adminContentSection = value || "start"; render(); return; }
			if (action === "admin-add-custom-link") return addAdminCustomLink();
			if (action === "admin-remove-custom-link") return removeAdminCustomLink(Number(value));
			if (action === "admin-add-faq") return addAdminFAQItem();
			if (action === "admin-remove-faq") return removeAdminFAQItem(Number(value));
			if (action === "admin-move-faq") return moveAdminFAQItem(Number(value), Number(target.dataset.direction || 0));
			if (action === "admin-move-layout") return moveAdminLayoutElement(Number(value), Number(target.dataset.direction || 0));
			if (action === "admin-move-plan") return moveAdminPlan(Number(value), Number(target.dataset.direction || 0));
			if (action === "admin-resolve-event") return await resolveAdminEvent(Number(value));
			if (action === "admin-integration-open") { state.adminIntegrationOpen = state.adminIntegrationOpen === value ? "" : value; render({ preserveScroll: true }); return; }
			if (action === "admin-integration-save") return await saveAdminIntegration(value);
			if (action === "admin-integration-copy-webhook") return copyToClipboard(value).then(() => showToast("Webhook скопирован", "success"));
      if (action === "activate-trial") return await activateTrial();
      if (action === "open-web-version") return openWebVersion();
      if (action === "open-install-guide") return openInstallGuide();
      if (action === "set-install-platform") { state.installGuidePlatform = value === "ios" ? "ios" : "android"; haptic("light"); render(); return; }
      if (action === "open-access") return openSubscriptionAccess();
      if (action === "copy-access") return state.data?.subscription?.subscriptionLink ? copyToClipboard(state.data.subscription.subscriptionLink).then(() => showToast(t().copied)) : showToast(t().noAccess);
      if (action === "share-referral") return state.data?.referral?.shareUrl ? openExternal(state.data.referral.shareUrl) : undefined;
      if (action === "copy-referral") return state.data?.referral?.shareUrl ? copyToClipboard(state.data.referral.shareUrl).then(() => showToast(t().copied)) : undefined;
      if (action === "open-link") return openExternal(value);
        if (action === "set-theme") { state.theme = value === "light" ? "light" : "dark"; writeSetting(STORAGE_KEYS.theme, state.theme); applyAppearance(); render(); return; }
      } catch (error) {
        showToast(error?.message || t().errorTitle);
    }
  });

    app.addEventListener("input", (event) => {
      const target = event.target;
		const integrationProvider = target?.dataset?.integrationProvider;
		if (integrationProvider && state.adminIntegrationDrafts[integrationProvider]) {
			const draft = state.adminIntegrationDrafts[integrationProvider];
			if (target.hasAttribute("data-integration-enabled")) draft.enabled = Boolean(target.checked);
			if (target.dataset.integrationField) draft.fields[target.dataset.integrationField] = target.value;
			return;
		}
		const broadcastIndex = Number(target?.dataset?.broadcastIndex);
		const broadcastField = target?.dataset?.broadcastField;
		if (Number.isInteger(broadcastIndex) && broadcastIndex >= 0 && broadcastField && state.adminBroadcastButtonsDraft[broadcastIndex]) {
			const button = state.adminBroadcastButtonsDraft[broadcastIndex];
			button[broadcastField] = target.value;
			if (broadcastField === "type") {
				button.type = target.value === "promo" ? "promo" : "url";
				state.adminBroadcastButtonsDirty = true;
				render({ preserveScroll: true });
				return;
			}
			state.adminBroadcastButtonsDirty = true;
			const saveButton = app.querySelector('[data-action="admin-broadcast-save-buttons"]');
			if (saveButton) saveButton.disabled = false;
			return;
		}
		const settingPath = target?.dataset?.settingPath;
		if (settingPath && state.adminSettingsDraft) {
			const type = target.dataset.settingType || "text";
			if (type === "json") {
				state.adminJSONDrafts[settingPath] = target.value;
				try { setDeepValue(state.adminSettingsDraft, settingPath, JSON.parse(target.value)); } catch { /* validated before save */ }
			} else {
				const value = type === "boolean" ? Boolean(target.checked) : type === "number" ? Number(target.value || 0) : target.value;
				setDeepValue(state.adminSettingsDraft, settingPath, value);
			}
			state.adminSettingsDirty = true;
			const saveButton = app.querySelector('[data-action="admin-save-settings"]');
			if (saveButton) saveButton.disabled = false;
			const saveLabel = app.querySelector(".admin-save-bar > span");
			syncAdminSaveBarDOM();
			if (saveLabel) saveLabel.textContent = "Есть несохранённые изменения";
			syncAdminSaveBarDOM();
			return;
		}
		const inputKey = target?.dataset?.input;
		if (!inputKey) return;
		if (inputKey === "admin-plan-internal-squad" && state.adminPlanFormDraft) {
			state.adminPlanFormDraft.internalSquadUuids = updateAdminSquadSelection(state.adminPlanFormDraft.internalSquadUuids, target.value, Boolean(target.checked));
			return;
		}
		if (inputKey === "admin-plan-external-squad" && state.adminPlanFormDraft) {
			state.adminPlanFormDraft.externalSquadUuid = target.value;
			return;
		}
		if (inputKey === "admin-trial-internal-squad" && state.adminSettingsDraft?.trial) {
			state.adminSettingsDraft.trial.internalSquadUuids = updateAdminSquadSelection(state.adminSettingsDraft.trial.internalSquadUuids, target.value, Boolean(target.checked));
			state.adminSettingsDirty = true;
			syncAdminSaveBarDOM();
			return;
		}
		if (inputKey === "admin-trial-external-squad" && state.adminSettingsDraft?.trial) {
			state.adminSettingsDraft.trial.externalSquadUuid = target.value;
			state.adminSettingsDirty = true;
			syncAdminSaveBarDOM();
			return;
		}
		if (inputKey.startsWith("admin-plan-") && state.adminPlanFormDraft) {
			const numeric = Math.max(0, Number(target.value || 0));
			if (inputKey === "admin-plan-months") state.adminPlanFormDraft.months = numeric;
			if (inputKey === "admin-plan-price") state.adminPlanFormDraft.priceRub = numeric;
			if (inputKey === "admin-plan-traffic") state.adminPlanFormDraft.trafficGb = numeric;
			if (inputKey === "admin-plan-devices") state.adminPlanFormDraft.deviceLimit = numeric;
			return;
		}

      if (inputKey === "theme-toggle") {
        state.theme = "dark";
        writeSetting(STORAGE_KEYS.theme, state.theme);
        applyAppearance();
        return;
      }
      if (inputKey === "payment-agreement") {
        state.paymentAgreementAccepted = Boolean(target.checked);
        return;
      }
      if (inputKey === "promo-code") {
        state.promoCodeDraft = target.value;
        if (state.appliedPromo && normalizePromoCodeValue(target.value) !== state.appliedPromo.code) {
          state.appliedPromo = null;
        }
        state.promoValidation = null;
        syncPromoCheckoutDom({ price: true });
        schedulePromoAutoApply();
        return;
      }
      if (inputKey === "admin-promo-code") { state.adminPromoCodeDraft = target.value.toUpperCase(); return; }
      if (inputKey === "admin-promo-discount") { state.adminPromoDiscountDraft = target.value; return; }
      if (inputKey === "admin-promo-limit") { state.adminPromoLimitDraft = target.value; return; }
      if (inputKey === "admin-promo-expires") { state.adminPromoExpiresDraft = target.value; return; }
      if (inputKey === "admin-subscription-query") {
        state.adminSubscriptionQuery = target.value;
        state.adminSubscriptionResult = null;
        state.adminSubscriptionTargetTelegramID = "";
        return;
      }
      if (inputKey === "admin-subscription-target") {
        state.adminSubscriptionTargetTelegramID = target.value.replace(/\D+/g, "");
        if (target.value !== state.adminSubscriptionTargetTelegramID) target.value = state.adminSubscriptionTargetTelegramID;
        return;
      }

      if (inputKey === "support-subject") state.supportDraftSubject = target.value;
      if (inputKey === "support-message") state.supportDraftMessage = target.value;
      if (inputKey === "support-reply") state.supportReplyDraft = target.value;
      if (inputKey === "review-comment") state.reviewDraftComment = target.value;
  });

	app.addEventListener("pointerdown", beginAdminProfilePointer);
	app.addEventListener("pointerdown", beginAdminPlanPointer);
	app.addEventListener("pointerdown", beginAdminLayoutPointer);
	app.addEventListener("keydown", (event) => {
		const planHandle = event.target.closest?.("[data-admin-plan-drag]");
		if (state.adminPlanEditing && planHandle && ["ArrowLeft", "ArrowUp", "ArrowRight", "ArrowDown"].includes(event.key)) {
			event.preventDefault();
			const node = planHandle.closest("[data-admin-plan-id]");
			const plans = state.adminSettingsDraft?.plans || [];
			const index = plans.findIndex((item) => String(item.id) === String(node?.dataset?.adminPlanId || ""));
			moveAdminPlan(index, ["ArrowLeft", "ArrowUp"].includes(event.key) ? -1 : 1);
			return;
		}
		if (!(["Enter", " "].includes(event.key))) return;
		const node = event.target.closest?.("[data-layout-edit-key]");
		if (!state.adminLayoutEditing || !node) return;
		event.preventDefault();
		selectAdminLayoutNode(node);
	});
	window.addEventListener("pointermove", moveAdminLayoutPointer, { passive: false });
	window.addEventListener("pointermove", moveAdminProfilePointer, { passive: false });
	window.addEventListener("pointermove", moveAdminPlanPointer, { passive: false });
	window.addEventListener("pointerup", endAdminLayoutPointer);
	window.addEventListener("pointerup", endAdminProfilePointer);
	window.addEventListener("pointerup", endAdminPlanPointer);
	window.addEventListener("pointercancel", cancelAdminLayoutPointer);
	window.addEventListener("pointercancel", cancelAdminProfilePointer);
	window.addEventListener("pointercancel", cancelAdminPlanPointer);

  document.addEventListener("visibilitychange", () => {
    document.documentElement.dataset.pageActive = document.hidden ? "false" : "true";
    syncBackgroundEngines();
    if (document.visibilityState === "visible" && hasAuth()) safeRefresh().catch(() => {});
  });

  reducedMotionMedia?.addEventListener?.("change", () => {
    configureBackgroundPerformance();
    syncBackgroundEngines();
  });

  window.addEventListener("resize", () => {
    syncToastAnchor();
  });
}

async function testAdminSubscriptionReminder(kind) {
	if (!state.adminSettingsDraft || state.adminBusy) return;
	const normalizedKind = kind === "expired" ? "expired" : "expiring";
	const templateKey = normalizedKind === "expired" ? "subscriptionExpiredTemplate" : "subscriptionExpiringTemplate";
	const buttonSettings = getDeepValue(state.adminSettingsDraft, "content.subscriptionReminderButton", {}) || {};
	state.adminBusy = `test-reminder-${normalizedKind}`;
	render({ preserveScroll: true });
	try {
		await post("/api/mini-app/admin/reminders/test", {
			kind: normalizedKind,
			template: getDeepValue(state.adminSettingsDraft, `content.copy.ru.${templateKey}`, "") || copybook.ru[templateKey],
			buttonText: getDeepValue(state.adminSettingsDraft, "content.copy.ru.subscriptionRenewButton", "") || copybook.ru.subscriptionRenewButton,
			iconCustomEmojiId: buttonSettings.iconCustomEmojiId || "",
			buttonStyle: buttonSettings.style || "",
		});
		state.adminBusy = "";
		render({ preserveScroll: true });
		showToast("Тестовое уведомление отправлено вам в Telegram", "success");
	} catch (error) {
		state.adminBusy = "";
		render({ preserveScroll: true });
		throw error;
	}
}

async function testAdminSubscriptionSuccess() {
	if (!state.adminSettingsDraft || state.adminBusy) return;
	const commerce = getDeepValue(state.adminSettingsDraft, "content.commerce", {}) || {};
	const button = commerce.successButton || {};
	state.adminBusy = "test-success";
	render({ preserveScroll: true });
	try {
		await post("/api/mini-app/admin/success/test", {
			text: commerce.successText || "",
			banner: commerce.successBanner || "",
			buttonText: button.text || "",
			iconCustomEmojiId: button.iconCustomEmojiId || "",
			buttonStyle: button.style || "",
		});
		state.adminBusy = "";
		render({ preserveScroll: true });
		showToast("Тестовое сообщение отправлено вам в Telegram", "success");
	} catch (error) {
		state.adminBusy = "";
		render({ preserveScroll: true });
		throw error;
	}
}

async function saveAdminSettings() {
	if (!state.adminSettingsDraft || state.adminBusy) return;
	for (const [path, raw] of Object.entries(state.adminJSONDrafts)) {
		try {
			setDeepValue(state.adminSettingsDraft, path, JSON.parse(raw));
		} catch {
			showToast(`Некорректный JSON: ${path}`, "danger");
			return;
		}
	}
	state.adminBusy = "save-settings";
	syncAdminSaveBarDOM();
	try {
		const response = await post("/api/mini-app/admin/settings/update", { settings: state.adminSettingsDraft });
		state.publicSettings = response.data;
		if (state.data) {
			state.data.runtime = response.data;
			if (state.data.admin) state.data.admin.settings = response.data;
		}
		state.adminSettingsDraft = deepClone(response.data);
		seedEditableCopy(state.adminSettingsDraft);
		if (state.adminPlanEditing) {
			state.adminPlanBaseline = deepClone(response.data.plans || []);
			if (state.data) state.data.plans = (response.data.plans || []).filter((plan) => plan.enabled !== false).map(runtimePlanToPayload);
			ensureSelections();
		}
		state.adminSettingsDirty = false;
		state.adminJSONDrafts = {};
		state.adminBusy = "";
		render({ preserveScroll: true });
		showToast(state.locale === "en" ? "Settings saved" : "Настройки сохранены", "success");
	} catch (error) {
		state.adminBusy = "";
		syncAdminSaveBarDOM();
		throw error;
	}
}

function syncAdminSaveBarDOM() {
	const busy = state.adminBusy === "save-settings";
	const status = state.adminSettingsDirty
		? (state.locale === "en" ? "Unsaved changes" : "\u0415\u0441\u0442\u044c \u043d\u0435\u0441\u043e\u0445\u0440\u0430\u043d\u0451\u043d\u043d\u044b\u0435 \u0438\u0437\u043c\u0435\u043d\u0435\u043d\u0438\u044f")
		: (state.locale === "en" ? "All changes saved" : "\u0412\u0441\u0435 \u0438\u0437\u043c\u0435\u043d\u0435\u043d\u0438\u044f \u0441\u043e\u0445\u0440\u0430\u043d\u0435\u043d\u044b");
	app.querySelectorAll(".admin-save-bar").forEach((bar) => {
		const label = bar.querySelector(":scope > span");
		const button = bar.querySelector('[data-action="admin-save-settings"]');
		if (label) label.textContent = status;
		if (!button) return;
		button.disabled = busy || !state.adminSettingsDirty;
		button.innerHTML = `${icon(busy ? "refresh" : "check")}<span>${state.locale === "en" ? "Save" : "\u0421\u043e\u0445\u0440\u0430\u043d\u0438\u0442\u044c"}</span>`;
	});
}

function setAdminBroadcastDraft(draft, { forceButtons = false } = {}) {
	state.adminBroadcast = draft || { status: "idle", buttons: [], recipientCount: 0, sentCount: 0, failedCount: 0 };
	if (forceButtons || !state.adminBroadcastButtonsDirty) {
		state.adminBroadcastButtonsDraft = deepClone(Array.isArray(state.adminBroadcast.buttons) ? state.adminBroadcast.buttons : []);
		state.adminBroadcastButtonsDirty = false;
	}
}

async function refreshAdminBroadcast({ silent = false, forceButtons = false } = {}) {
	if (state.adminSection !== "broadcast" || state.adminBroadcastBusy === "state") return;
	if (!silent) {
		state.adminBroadcastBusy = "state";
		render({ preserveScroll: true });
	}
	try {
		const response = await post("/api/mini-app/admin/broadcast/state", {});
		setAdminBroadcastDraft(response.data, { forceButtons });
		state.adminBroadcastBusy = "";
		render({ preserveScroll: true });
	} catch (error) {
		state.adminBroadcastBusy = "";
		if (!silent) throw error;
	}
}

async function startAdminBroadcastCapture() {
	if (state.adminBroadcastBusy) return;
	state.adminBroadcastBusy = "capture";
	render({ preserveScroll: true });
	try {
		const response = await post("/api/mini-app/admin/broadcast/capture/start", {});
		setAdminBroadcastDraft(response.data);
		state.adminBroadcastBusy = "";
		render({ preserveScroll: true });
		showToast(state.locale === "en" ? "Send the next message to the bot" : "Отправьте следующее сообщение боту", "success");
		if (tg?.close) setTimeout(() => tg.close(), 450);
	} catch (error) {
		state.adminBroadcastBusy = "";
		render({ preserveScroll: true });
		throw error;
	}
}

function addAdminBroadcastButton() {
	if (state.adminBroadcastButtonsDraft.length >= 8 || state.adminBroadcast?.status === "running") return;
	state.adminBroadcastButtonsDraft.push({ id: `button_${Date.now().toString(36)}`, type: "url", text: "", iconCustomEmojiId: "", style: "", url: "https://", promoCode: "" });
	state.adminBroadcastButtonsDirty = true;
	haptic("light");
	render({ preserveScroll: true });
}

function setAdminBroadcastButtonStyle(index, style) {
	if (!Number.isInteger(index) || index < 0 || index >= state.adminBroadcastButtonsDraft.length || state.adminBroadcast?.status === "running") return;
	state.adminBroadcastButtonsDraft[index].style = ["primary", "success", "danger"].includes(style) ? style : "";
	state.adminBroadcastButtonsDirty = true;
	haptic("light");
	render({ preserveScroll: true });
}

function removeAdminBroadcastButton(index) {
	if (!Number.isInteger(index) || index < 0 || index >= state.adminBroadcastButtonsDraft.length || state.adminBroadcast?.status === "running") return;
	state.adminBroadcastButtonsDraft.splice(index, 1);
	state.adminBroadcastButtonsDirty = true;
	haptic("light");
	render({ preserveScroll: true });
}

async function saveAdminBroadcastButtons({ silent = false } = {}) {
	if (state.adminBroadcastBusy || (!state.adminBroadcastButtonsDirty && silent)) return state.adminBroadcast;
	state.adminBroadcastBusy = "buttons";
	if (!silent) render({ preserveScroll: true });
	try {
		const response = await post("/api/mini-app/admin/broadcast/buttons", { buttons: state.adminBroadcastButtonsDraft });
		setAdminBroadcastDraft(response.data, { forceButtons: true });
		state.adminBroadcastBusy = "";
		render({ preserveScroll: true });
		if (!silent) showToast(state.locale === "en" ? "Buttons saved" : "Кнопки сохранены", "success");
		return response.data;
	} catch (error) {
		state.adminBroadcastBusy = "";
		render({ preserveScroll: true });
		throw error;
	}
}

async function ensureAdminBroadcastButtonsSaved() {
	if (!state.adminBroadcastButtonsDirty) return state.adminBroadcast;
	return saveAdminBroadcastButtons({ silent: true });
}

async function previewAdminBroadcast() {
	if (state.adminBroadcastBusy) return;
	await ensureAdminBroadcastButtonsSaved();
	state.adminBroadcastBusy = "preview";
	render({ preserveScroll: true });
	try {
		const response = await post("/api/mini-app/admin/broadcast/preview", {});
		setAdminBroadcastDraft(response.data);
		state.adminBroadcastBusy = "";
		render({ preserveScroll: true });
		showToast(state.locale === "en" ? "Preview sent to the bot" : "Предпросмотр отправлен в бот", "success");
	} catch (error) {
		state.adminBroadcastBusy = "";
		render({ preserveScroll: true });
		throw error;
	}
}

async function sendAdminBroadcast() {
	if (state.adminBroadcastBusy) return;
	state.adminBroadcastConfirmOpen = false;
	await ensureAdminBroadcastButtonsSaved();
	state.adminBroadcastBusy = "send";
	render({ preserveScroll: true });
	try {
		const response = await post("/api/mini-app/admin/broadcast/send", {});
		setAdminBroadcastDraft(response.data);
		state.adminBroadcastBusy = "";
		render({ preserveScroll: true });
		showToast(state.locale === "en" ? "Broadcast started" : "Рассылка запущена", "success");
	} catch (error) {
		state.adminBroadcastBusy = "";
		render({ preserveScroll: true });
		throw error;
	}
}

async function resetAdminBroadcast() {
	if (state.adminBroadcastBusy || state.adminBroadcast?.status === "running") return;
	state.adminBroadcastBusy = "reset";
	render({ preserveScroll: true });
	try {
		const response = await post("/api/mini-app/admin/broadcast/reset", {});
		setAdminBroadcastDraft(response.data, { forceButtons: true });
		state.adminBroadcastBusy = "";
		render({ preserveScroll: true });
		showToast(state.locale === "en" ? "Draft cleared" : "Черновик очищен", "success");
	} catch (error) {
		state.adminBroadcastBusy = "";
		render({ preserveScroll: true });
		throw error;
	}
}

function syncAdminBroadcastPolling() {
	const active = state.currentPage === "admin" && state.adminSection === "broadcast" && ["awaiting_message", "running"].includes(state.adminBroadcast?.status);
	if (!active) {
		if (adminBroadcastPollTimer) clearInterval(adminBroadcastPollTimer);
		adminBroadcastPollTimer = 0;
		return;
	}
	if (adminBroadcastPollTimer) return;
	adminBroadcastPollTimer = setInterval(() => {
		if (document.visibilityState === "visible") void refreshAdminBroadcast({ silent: true });
	}, 2000);
}

function addAdminCustomLink() {
	if (!state.adminSettingsDraft?.content) return;
	const items = state.adminSettingsDraft.content.customLinks || (state.adminSettingsDraft.content.customLinks = []);
	items.push({ id: `link_${Date.now().toString(36)}`, labelRu: "Новая кнопка", hintRu: "", url: "https://", icon: "external" });
	state.adminSettingsDirty = true;
	render();
}

function removeAdminCustomLink(index) {
	const items = state.adminSettingsDraft?.content?.customLinks;
	if (!Array.isArray(items) || index < 0 || index >= items.length) return;
	items.splice(index, 1);
	state.adminSettingsDirty = true;
	render();
}

function addAdminFAQItem() {
	if (!state.adminSettingsDraft?.content) return;
	if (!state.adminSettingsDraft.content.faq) state.adminSettingsDraft.content.faq = { ru: [] };
	const items = state.adminSettingsDraft.content.faq.ru || (state.adminSettingsDraft.content.faq.ru = []);
	if (items.length >= 100) return;
	items.push({ question: "", answer: "" });
	state.adminSettingsDirty = true;
	haptic("light");
	render({ preserveScroll: true });
}

function removeAdminFAQItem(index) {
	const items = state.adminSettingsDraft?.content?.faq?.ru;
	if (!Array.isArray(items) || index < 0 || index >= items.length) return;
	items.splice(index, 1);
	state.adminSettingsDirty = true;
	haptic("light");
	render({ preserveScroll: true });
}

function moveAdminFAQItem(index, direction) {
	const items = state.adminSettingsDraft?.content?.faq?.ru;
	const next = index + Math.sign(direction);
	if (!Array.isArray(items) || index < 0 || index >= items.length || next < 0 || next >= items.length) return;
	[items[index], items[next]] = [items[next], items[index]];
	state.adminSettingsDirty = true;
	haptic("light");
	render({ preserveScroll: true });
}

function markAdminLayoutDirty() {
	state.adminSettingsDirty = true;
	syncAdminSaveBarDOM();
}

function syncAdminLayoutBackgroundAnimation() {
	syncBackgroundEngines();
}

function enterAdminPlanEditor() {
	syncAdminSettingsDraft();
	if (!state.adminSettingsDraft) return;
	state.adminPlanBaseline = deepClone(state.adminSettingsDraft.plans || []);
	state.adminPlanEditing = true;
	state.adminLayoutEditing = false;
	state.adminPlanEditorModalOpen = false;
	state.adminPlanEditingID = "";
	state.adminPlanFormDraft = null;
	state.adminSection = "plans";
	state.currentPage = "buy";
	state.sidebarOpen = false;
	previousBottomNavIndex = -1;
	ensureSelections();
	haptic("light");
	render({ preserveScroll: false, scrollTop: 0 });
}

function exitAdminPlanEditor() {
	finishAdminPlanPointer();
	state.adminPlanEditing = false;
	state.adminPlanEditorModalOpen = false;
	state.adminPlanEditingID = "";
	state.adminPlanFormDraft = null;
	state.adminPlanBaseline = null;
	state.currentPage = "admin";
	state.adminSection = "home";
	previousBottomNavIndex = -1;
	haptic("light");
	render({ preserveScroll: false, scrollTop: 0 });
}

function addAdminPlan() {
	const plans = state.adminSettingsDraft?.plans;
	if (!Array.isArray(plans)) return;
	const id = `custom_${Date.now().toString(36)}`;
	const plan = { id, enabled: false, months: 0, titleRu: "", titleEn: "", priceRub: 0, priceStars: 0, trafficGb: 0, unlimitedTraffic: true, deviceLimit: 0, wide: false, internalSquadUuids: [], externalSquadUuid: "" };
	plans.push(plan);
	state.adminSettingsDirty = true;
	state.adminPlanEditingID = id;
	state.adminPlanFormDraft = deepClone(plan);
	state.adminPlanEditorModalOpen = true;
	ensureSelections();
	haptic("light");
	render({ preserveScroll: true });
}

function openAdminPlanEditor(id) {
	const plan = state.adminSettingsDraft?.plans?.find((item) => item.id === id);
	if (!plan) return;
	state.adminPlanEditingID = id;
	state.adminPlanFormDraft = deepClone(plan);
	state.adminPlanEditorModalOpen = true;
	haptic("light");
	render({ preserveScroll: true });
}

function closeAdminPlanEditorModal() {
	state.adminPlanEditorModalOpen = false;
	state.adminPlanEditingID = "";
	state.adminPlanFormDraft = null;
	render({ preserveScroll: true });
}

function applyAdminPlanEdit() {
	const draft = state.adminPlanFormDraft;
	const plans = state.adminSettingsDraft?.plans;
	const index = plans?.findIndex((item) => item.id === state.adminPlanEditingID) ?? -1;
	if (!draft || index < 0) return;
	const months = Math.trunc(Number(draft.months || 0));
	const priceRub = Math.trunc(Number(draft.priceRub || 0));
	const trafficGb = Math.trunc(Number(draft.trafficGb || 0));
	const deviceLimit = Math.trunc(Number(draft.deviceLimit || 0));
	if (months < 1 || months > 120) return showToast(state.locale === "en" ? "Enter a duration from 1 to 120 months" : "Укажите срок от 1 до 120 месяцев", "danger");
	if (priceRub < 1 || priceRub > 1000000) return showToast(state.locale === "en" ? "Enter a valid price" : "Укажите корректную цену", "danger");
	if (trafficGb < 0 || trafficGb > 1000000 || deviceLimit < 0 || deviceLimit > 1000) return showToast(state.locale === "en" ? "Check the plan limits" : "Проверьте лимиты тарифа", "danger");
	const current = plans[index];
	plans[index] = {
		...current,
		enabled: true,
		months,
		priceRub,
		trafficGb,
		unlimitedTraffic: trafficGb === 0,
		deviceLimit,
		titleRu: getPlanCardTitle(months, "ru"),
		titleEn: getPlanCardTitle(months, "en"),
		priceStars: Math.max(0, Math.round(priceRub / 1.47)),
		internalSquadUuids: Array.isArray(draft.internalSquadUuids) ? [...draft.internalSquadUuids] : [],
		externalSquadUuid: String(draft.externalSquadUuid || ""),
	};
	state.adminSettingsDirty = true;
	state.adminPlanEditorModalOpen = false;
	state.adminPlanEditingID = "";
	state.adminPlanFormDraft = null;
	ensureSelections();
	haptic("light");
	render({ preserveScroll: true });
}

function deleteAdminPlan(id) {
	const plans = state.adminSettingsDraft?.plans;
	const index = plans?.findIndex((item) => item.id === id) ?? -1;
	if (index < 0) return;
	const confirmed = window.confirm(state.locale === "en" ? "Delete this plan?" : "Удалить этот тариф?");
	if (!confirmed) return;
	plans.splice(index, 1);
	state.adminSettingsDirty = true;
	ensureSelections();
	haptic("light");
	render({ preserveScroll: true });
}

function toggleAdminPlanWide(id) {
	const plan = state.adminSettingsDraft?.plans?.find((item) => String(item.id) === String(id));
	if (!plan) return;
	plan.wide = !Boolean(plan.wide);
	state.adminSettingsDirty = true;
	haptic("light");
	render({ preserveScroll: true });
}

function resetAdminPlans() {
	if (!state.adminSettingsDraft || !Array.isArray(state.adminPlanBaseline)) return;
	state.adminSettingsDraft.plans = deepClone(state.adminPlanBaseline);
	state.adminSettingsDirty = true;
	state.adminPlanEditorModalOpen = false;
	state.adminPlanEditingID = "";
	state.adminPlanFormDraft = null;
	ensureSelections();
	haptic("light");
	render({ preserveScroll: false, scrollTop: 0 });
}

function enterAdminLayoutEditor() {
	syncAdminSettingsDraft();
	ensureAdminVisualLayoutDraft();
	state.adminLayoutEditing = true;
	state.adminSection = "layout";
	state.adminLayoutCategory = "dashboard";
	state.adminLayoutSelection = "";
	state.currentPage = "dashboard";
	state.sidebarOpen = false;
	previousBottomNavIndex = -1;
	haptic("light");
	render({ preserveScroll: false, scrollTop: 0 });
}

function exitAdminLayoutEditor() {
	finishAdminLayoutPointer();
	finishAdminProfilePointer();
	state.adminLayoutEditing = false;
	state.adminLayoutSelection = "";
	state.adminLayoutCategory = "dashboard";
	state.currentPage = "admin";
	state.adminSection = "home";
	previousBottomNavIndex = -1;
	haptic("light");
	render({ preserveScroll: false, scrollTop: 0 });
}

function setAdminLayoutCategory(category) {
	if (!ADMIN_LAYOUT_CATEGORIES.some((item) => item.id === category)) return;
	state.adminLayoutCategory = category;
	const entry = getAdminLayoutDraftEntries(category).find(({ item }) => item.visible !== false) || getAdminLayoutDraftEntries(category)[0];
	state.adminLayoutSelection = entry?.key || "";
	haptic("light");
	render();
}

function toggleSelectedAdminLayoutFrame() {
	const selected = getAdminLayoutSelection();
	if (selected?.type !== "layout") return;
	selected.item.framed = !selected.item.framed;
	markAdminLayoutDirty();
	render();
}

function hideSelectedAdminLayoutItem() {
	const selected = getAdminLayoutSelection();
	if (selected?.type !== "layout") return;
	selected.item.visible = false;
	const next = getAdminLayoutDraftEntries(state.adminLayoutCategory).find(({ item, key }) => item.visible !== false && key !== selected.key);
	state.adminLayoutSelection = next?.key || "";
	markAdminLayoutDirty();
	render();
}

function showAdminLayoutItem(key) {
	const entry = getAdminLayoutDraftEntries(state.adminLayoutCategory).find((item) => item.key === key);
	if (!entry) return;
	entry.item.visible = true;
	state.adminLayoutSelection = entry.key;
	markAdminLayoutDirty();
	render();
}

function resetSelectedAdminLayoutItem() {
	const selected = getAdminLayoutSelection();
	if (selected?.type !== "layout") return;
	const fallback = ADMIN_LAYOUT_DEFAULTS.find((item) => item.area === selected.item.area && item.id === selected.item.id);
	if (!fallback) return;
	Object.assign(selected.item, deepClone(fallback));
	state.adminLayoutSelection = `${fallback.area}:${fallback.id}`;
	markAdminLayoutDirty();
	render();
}

function resetAdminLayoutCategory() {
	finishAdminLayoutPointer();
	const items = state.adminSettingsDraft?.layout?.elements;
	if (!Array.isArray(items)) return;
	const resetAreas = new Set([state.adminLayoutCategory]);
	for (const fallback of ADMIN_LAYOUT_DEFAULTS.filter((item) => resetAreas.has(item.area))) {
		const index = items.findIndex((item) => item.area === fallback.area && item.id === fallback.id);
		if (index >= 0) items[index] = deepClone(fallback);
		else items.push(deepClone(fallback));
	}
	const first = getAdminLayoutDraftEntries(state.adminLayoutCategory)[0];
	state.adminLayoutSelection = first?.key || "";
	markAdminLayoutDirty();
	render();
	showToast(state.locale === "en" ? "Screen reset. Save the changes." : "Экран сброшен. Сохраните изменения.", "success");
}

function toggleSelectedAdminPlanWide() {
	const selected = getAdminLayoutSelection();
	if (selected?.type !== "plan") return;
	selected.item.wide = !selected.item.wide;
	markAdminLayoutDirty();
	render();
}

function hideSelectedAdminPlan() {
	const selected = getAdminLayoutSelection();
	if (selected?.type !== "plan") return;
	selected.item.enabled = false;
	state.adminLayoutSelection = "buy:plans";
	markAdminLayoutDirty();
	render();
}

function showAdminPlan(id) {
	const plan = (state.adminSettingsDraft?.plans || []).find((item) => item.id === id);
	if (!plan) return;
	plan.enabled = true;
	state.adminLayoutSelection = `plan:${plan.id}`;
	markAdminLayoutDirty();
	render();
}

function resetSelectedAdminPlan() {
	const selected = getAdminLayoutSelection();
	if (selected?.type !== "plan") return;
	selected.item.enabled = true;
	selected.item.wide = selected.item.id === "12m";
	markAdminLayoutDirty();
	render();
}

function selectAdminLayoutNode(node) {
	if (!state.adminLayoutEditing || !node) return;
	const key = node.dataset.layoutEditKey || "";
	if (!key) return;
	state.adminLayoutSelection = key;
	app.querySelectorAll("[data-layout-edit-key].is-selected").forEach((item) => item.classList.remove("is-selected"));
	node.classList.add("is-selected");
}

function getAdminLayoutItemForNode(node) {
	const key = String(node?.dataset?.runtimeLayoutKey || node?.dataset?.layoutEditKey || "");
	const separator = key.indexOf(":");
	if (separator < 1) return null;
	const area = key.slice(0, separator);
	const id = key.slice(separator + 1);
	return state.adminSettingsDraft?.layout?.elements?.find((item) => item?.area === area && item?.id === id) || null;
}

function getRuntimeLayoutItemForNode(node) {
	const key = String(node?.dataset?.runtimeLayoutKey || node?.dataset?.layoutEditKey || "");
	const separator = key.indexOf(":");
	if (separator < 1) return null;
	const area = key.slice(0, separator);
	const id = key.slice(separator + 1);
	const settings = state.adminLayoutEditing ? state.adminSettingsDraft : getRuntimeSettings();
	return settings?.layout?.elements?.find((item) => item?.area === area && item?.id === id) || null;
}

function rebindAdminLayoutDOMIndexes() {
	app.querySelectorAll("[data-layout-edit-key]").forEach((node) => {
		const item = getAdminLayoutItemForNode(node);
		if (!item) return;
		const index = state.adminSettingsDraft.layout.elements.indexOf(item);
		node.dataset.uiLayoutIndex = String(index);
	});
}

function createAdminLayoutPlaceholder(node, rect) {
	const computed = window.getComputedStyle(node);
	const placeholder = document.createElement("div");
	placeholder.className = "layout-editor-placeholder";
	placeholder.setAttribute("aria-hidden", "true");
	placeholder.style.width = `${rect.width}px`;
	placeholder.style.height = `${rect.height}px`;
	placeholder.style.flex = "0 0 auto";
	placeholder.style.gridColumn = computed.gridColumn;
	placeholder.style.gridRow = computed.gridRow;
	placeholder.style.alignSelf = computed.alignSelf;
	placeholder.style.justifySelf = computed.justifySelf;
	placeholder.style.marginTop = computed.marginTop;
	placeholder.style.marginRight = computed.marginRight;
	placeholder.style.marginBottom = computed.marginBottom;
	placeholder.style.marginLeft = computed.marginLeft;
	return placeholder;
}

function mountRuntimeLayoutSurface(surface, kind) {
	if (!surface || surface.dataset.layoutRuntimeMounted === "true") return;
	const selector = kind === "navigation" ? ".bottom-nav" : ".page.active";
	const nodes = Array.from(surface.querySelectorAll("[data-runtime-layout-key]"))
		.filter((node) => node.closest(selector) === surface && getRuntimeLayoutItemForNode(node));
	if (!nodes.length) return;

	const surfaceRect = surface.getBoundingClientRect();
	const records = nodes.map((node) => {
		const rect = node.getBoundingClientRect();
		const parent = node.parentElement;
		const parentRect = parent.getBoundingClientRect();
		const parentOriginX = parentRect.left + parent.clientLeft - parent.scrollLeft;
		const parentOriginY = parentRect.top + parent.clientTop - parent.scrollTop;
		return {
			node,
			rect,
			parent,
			parentOriginX,
			parentOriginY,
			placeholder: createAdminLayoutPlaceholder(node, rect),
		};
	});

	surface.dataset.layoutRuntimeMounted = "true";
	surface.classList.add("layout-runtime-surface", `layout-runtime-surface--${kind}`);
	if (state.adminLayoutEditing) surface.classList.add("layout-editor-surface", `layout-editor-surface--${kind}`);
	for (const record of records) {
		record.parent.classList.add("layout-runtime-host");
		record.parent.replaceChild(record.placeholder, record.node);
		record.parent.appendChild(record.node);
	}

	let overlay = null;
	if (state.adminLayoutEditing) {
		overlay = document.createElement("div");
		overlay.className = `layout-editor-grid layout-editor-grid--${kind}`;
		overlay.setAttribute("aria-hidden", "true");
		overlay.innerHTML = '<span class="layout-snap-guide layout-snap-guide--x"></span><span class="layout-snap-guide layout-snap-guide--y"></span>';
		surface.appendChild(overlay);
	}

	let migrated = false;
	for (const record of records) {
		const { node, rect, parent, parentOriginX, parentOriginY } = record;
		const item = getRuntimeLayoutItemForNode(node);
		if (!item) continue;
		const flowLocalLeft = rect.left - parentOriginX;
		const flowLocalTop = rect.top - parentOriginY;
		const stored = hasStoredLayoutPosition(item);
		const localLeft = stored ? Number(item.positionX) : flowLocalLeft;
		const localTop = stored ? Number(item.positionY) : flowLocalTop;

		if (state.adminLayoutEditing && !stored) {
			item.positionX = Math.round(localLeft * 100) / 100;
			item.positionY = Math.round(localTop * 100) / 100;
			item.offsetX = 0;
			item.offsetY = 0;
			migrated = true;
		}

		const parentPageX = parentOriginX - surfaceRect.left;
		const parentPageY = parentOriginY - surfaceRect.top;
		node.classList.add("runtime-layout-mounted");
		if (state.adminLayoutEditing) node.classList.add("layout-editable--mounted");
		node.dataset.editorSurfaceKind = kind;
		node.dataset.editorParentOriginX = String(parentPageX);
		node.dataset.editorParentOriginY = String(parentPageY);
		node.dataset.editorParentWidth = String(Math.max(1, parent.clientWidth));
		setAdminLayoutMountedGeometry(node, parentPageX + localLeft, parentPageY + localTop, rect.width, rect.height, { expandSurface: false });
	}

	let requiredHeight = 0;
	for (const { node } of records) {
		const rect = node.getBoundingClientRect();
		requiredHeight = Math.max(requiredHeight, rect.bottom - surfaceRect.top + 12);
	}
	if (kind !== "navigation") {
		surface.style.setProperty("--layout-runtime-min-height", `${Math.ceil(requiredHeight)}px`);
		if (overlay) overlay.style.height = `${Math.ceil(requiredHeight)}px`;
	}
	if (migrated) {
		state.adminSettingsDirty = true;
		syncAdminSaveBarDOM();
	}
	if (state.adminLayoutEditing) rebindAdminLayoutDOMIndexes();
}

function mountRuntimeLayout() {
	if (state.currentPage === "dashboard") {
		mountRuntimeLayoutSurface(app.querySelector("#page-dashboard.page.active"), "page");
	}
}

function setAdminLayoutMountedGeometry(node, pageLeft, pageTop, width, height, { expandSurface = true } = {}) {
	if (!node) return;
	const parentOriginX = Number(node.dataset.editorParentOriginX || 0);
	const parentOriginY = Number(node.dataset.editorParentOriginY || 0);
	node.dataset.editorPageLeft = String(pageLeft);
	node.dataset.editorPageTop = String(pageTop);
	node.dataset.editorWidth = String(width);
	node.dataset.editorHeight = String(height);
	node.style.setProperty("--editor-local-left", `${pageLeft - parentOriginX}px`);
	node.style.setProperty("--editor-local-top", `${pageTop - parentOriginY}px`);
	node.style.setProperty("--editor-pixel-width", `${width}px`);
	node.style.setProperty("--editor-pixel-height", `${height}px`);
	const surface = expandSurface ? node.closest(node.dataset.editorSurfaceKind === "navigation" ? ".bottom-nav" : ".page.active") : null;
	if (surface && node.dataset.editorSurfaceKind !== "navigation") {
		const bottom = pageTop + height + 24;
		const current = Number.parseFloat(surface.style.getPropertyValue("--layout-runtime-min-height")) || surface.scrollHeight;
		if (bottom > current) {
			surface.style.setProperty("--layout-runtime-min-height", `${Math.ceil(bottom)}px`);
			const overlay = surface.querySelector(":scope > .layout-editor-grid");
			if (overlay) overlay.style.height = `${Math.ceil(bottom)}px`;
		}
	}
}

function syncAdminLayoutMountedNodes() {
	app.querySelectorAll("[data-layout-edit-key].runtime-layout-mounted").forEach((node) => {
		const item = getAdminLayoutItemForNode(node);
		if (!item) return;
		const surface = node.closest(node.dataset.editorSurfaceKind === "navigation" ? ".bottom-nav" : ".page.active");
		const parent = node.parentElement;
		if (!surface || !parent) return;
		const surfaceRect = surface.getBoundingClientRect();
		const parentRect = parent.getBoundingClientRect();
		const parentOriginX = parentRect.left + parent.clientLeft - parent.scrollLeft - surfaceRect.left;
		const parentOriginY = parentRect.top + parent.clientTop - parent.scrollTop - surfaceRect.top;
		const parentWidth = Math.max(1, Number(node.dataset.editorParentWidth || 1));
		const width = item.area === "navigation" ? Number(item.width || 44) : parentWidth * Number(item.width || 100) / 100;
		setAdminLayoutMountedGeometry(node, parentOriginX + Number(item.positionX || 0), parentOriginY + Number(item.positionY || 0), width, Number(item.height || 52));
	});
}

function getAdminLayoutSiblingRects(node) {
	const surfaceKind = node.dataset.editorSurfaceKind;
	const surface = node.closest(surfaceKind === "navigation" ? ".bottom-nav" : ".page.active");
	if (!surface) return [];
	const surfaceRect = surface.getBoundingClientRect();
	return Array.from(surface.querySelectorAll("[data-layout-edit-key].layout-editable--mounted"))
		.filter((candidate) => candidate !== node)
		.map((candidate) => {
			const rect = candidate.getBoundingClientRect();
			return {
				left: rect.left - surfaceRect.left,
				top: rect.top - surfaceRect.top,
				width: rect.width,
				height: rect.height,
			};
		});
}

function snapAdminLayoutPosition(value, size, siblings, axis) {
	let position = Math.round(value / ADMIN_LAYOUT_GRID_SIZE) * ADMIN_LAYOUT_GRID_SIZE;
	let guide = position;
	let bestDistance = Math.abs(position - value);
	const sourceAnchors = [0, size / 2, size];
	for (const sibling of siblings) {
		const start = axis === "x" ? sibling.left : sibling.top;
		const length = axis === "x" ? sibling.width : sibling.height;
		const targetAnchors = [start, start + length / 2, start + length];
		for (const sourceAnchor of sourceAnchors) {
			for (const targetAnchor of targetAnchors) {
				const candidate = targetAnchor - sourceAnchor;
				const distance = Math.abs(candidate - value);
				if (distance <= ADMIN_LAYOUT_SNAP_THRESHOLD && distance <= bestDistance + 0.5) {
					position = candidate;
					guide = targetAnchor;
					bestDistance = distance;
				}
			}
		}
	}
	return { position: Math.round(position * 100) / 100, guide: Math.round(guide * 100) / 100 };
}

function snapAdminLayoutEdge(value, siblings, axis) {
	let edge = Math.round(value / ADMIN_LAYOUT_GRID_SIZE) * ADMIN_LAYOUT_GRID_SIZE;
	let guide = edge;
	let bestDistance = Math.abs(edge - value);
	for (const sibling of siblings) {
		const start = axis === "x" ? sibling.left : sibling.top;
		const length = axis === "x" ? sibling.width : sibling.height;
		for (const target of [start, start + length / 2, start + length]) {
			const distance = Math.abs(target - value);
			if (distance <= ADMIN_LAYOUT_SNAP_THRESHOLD && distance <= bestDistance + 0.5) {
				edge = target;
				guide = target;
				bestDistance = distance;
			}
		}
	}
	return { edge: Math.round(edge * 100) / 100, guide: Math.round(guide * 100) / 100 };
}

function showAdminLayoutGuides(surface, x = null, y = null, guides = null) {
	const overlay = guides?.overlay || surface?.querySelector(":scope > .layout-editor-grid");
	const vertical = guides?.verticalGuide || overlay?.querySelector(".layout-snap-guide--x");
	const horizontal = guides?.horizontalGuide || overlay?.querySelector(".layout-snap-guide--y");
	if (vertical) {
		vertical.classList.toggle("is-visible", Number.isFinite(x));
		if (Number.isFinite(x)) vertical.style.setProperty("--guide-position", `${x}px`);
	}
	if (horizontal) {
		horizontal.classList.toggle("is-visible", Number.isFinite(y));
		if (Number.isFinite(y)) horizontal.style.setProperty("--guide-position", `${y}px`);
	}
}

function applyAdminLayoutNodeStyle(node, item) {
	if (!node || !item) return;
	if (node.classList.contains("runtime-layout-mounted")) {
		const surface = node.closest(node.dataset.editorSurfaceKind === "navigation" ? ".bottom-nav" : ".page.active");
		const parent = node.parentElement;
		if (!surface || !parent) return;
		const surfaceRect = surface.getBoundingClientRect();
		const parentRect = parent.getBoundingClientRect();
		const parentOriginX = parentRect.left + parent.clientLeft - parent.scrollLeft - surfaceRect.left;
		const parentOriginY = parentRect.top + parent.clientTop - parent.scrollTop - surfaceRect.top;
		const parentWidth = Math.max(1, Number(node.dataset.editorParentWidth || 1));
		const width = item.area === "navigation" ? Number(item.width || 44) : parentWidth * Number(item.width || 100) / 100;
		setAdminLayoutMountedGeometry(node, parentOriginX + Number(item.positionX || 0), parentOriginY + Number(item.positionY || 0), width, Number(item.height || 52));
		return;
	}
	if (node.classList.contains("bottom-nav__item")) {
		node.style.setProperty("--nav-item-width", `${item.width}px`);
		node.style.setProperty("--nav-item-height", `${item.height}px`);
		node.style.setProperty("--nav-item-x", `${item.offsetX}px`);
		node.style.setProperty("--nav-item-y", `${item.offsetY}px`);
		return;
	}
	if (node.classList.contains("profile-row")) {
		node.style.setProperty("--profile-row-width", `${item.width}%`);
		node.style.setProperty("--profile-row-height", `${item.height}px`);
		node.style.setProperty("--profile-row-x", `${item.offsetX}px`);
		node.style.setProperty("--profile-row-y", `${item.offsetY}px`);
		return;
	}
	node.style.setProperty("--runtime-width", `${item.width}%`);
	node.style.setProperty("--runtime-height", `${item.height}px`);
	node.style.setProperty("--runtime-x", `${item.offsetX}px`);
	node.style.setProperty("--runtime-y", `${item.offsetY}px`);
}

function beginAdminLayoutPointer(event) {
	if (!state.adminLayoutEditing || event.button > 0 || !event.isPrimary) return;
	const layoutNode = event.target.closest?.("[data-layout-edit-key]");
	if (!layoutNode) return;
	const item = getAdminLayoutItemForNode(layoutNode);
	if (!item) return;
	selectAdminLayoutNode(layoutNode);
	const surface = layoutNode.closest(layoutNode.dataset.editorSurfaceKind === "navigation" ? ".bottom-nav" : ".page.active") || app;
	const surfaceRect = surface.getBoundingClientRect();
	const nodeRect = layoutNode.getBoundingClientRect();
	const parent = layoutNode.parentElement;
	const parentRect = parent?.getBoundingClientRect?.() || surfaceRect;
	const parentOriginX = parentRect.left + (parent?.clientLeft || 0) - (parent?.scrollLeft || 0) - surfaceRect.left;
	const parentOriginY = parentRect.top + (parent?.clientTop || 0) - (parent?.scrollTop || 0) - surfaceRect.top;
	const overlay = surface.querySelector?.(":scope > .layout-editor-grid") || null;
	const startLeft = nodeRect.left - surfaceRect.left;
	const startTop = nodeRect.top - surfaceRect.top;
	const startWidth = nodeRect.width;
	const startHeight = nodeRect.height;
	layoutNode.dataset.editorParentOriginX = String(parentOriginX);
	layoutNode.dataset.editorParentOriginY = String(parentOriginY);
	adminLayoutPointer = {
		mode: event.target.closest?.("[data-ui-resize-handle]") ? "resize" : "move",
		pointerId: event.pointerId,
		startX: event.clientX,
		startY: event.clientY,
		item,
		node: layoutNode,
		surface,
		surfaceWidth: surfaceRect.width,
		surfaceHeight: Math.max(surfaceRect.height, surface.scrollHeight),
		siblings: getAdminLayoutSiblingRects(layoutNode),
		startLeft,
		startTop,
		startWidth,
		startHeight,
		parentOriginX,
		parentOriginY,
		parentWidth: Math.max(1, Number(layoutNode.dataset.editorParentWidth || surfaceRect.width)),
		overlay,
		verticalGuide: overlay?.querySelector(".layout-snap-guide--x") || null,
		horizontalGuide: overlay?.querySelector(".layout-snap-guide--y") || null,
		pendingGeometry: null,
		original: deepClone(item),
		moved: false,
	};
	layoutNode.setPointerCapture?.(event.pointerId);
	layoutNode.classList.add("is-manipulating");
	document.body.classList.add("is-editing-layout");
}

function moveAdminLayoutPointer(event) {
	const pointer = adminLayoutPointer;
	if (!pointer || event.pointerId !== pointer.pointerId) return;
	const dx = event.clientX - pointer.startX;
	const dy = event.clientY - pointer.startY;
	if (!pointer.moved && Math.hypot(dx, dy) < 3) return;
	event.preventDefault();
	adminLayoutPendingPoint = { pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY };
	if (!adminLayoutPointerFrame) adminLayoutPointerFrame = requestAnimationFrame(flushAdminLayoutPointerFrame);
}

function flushAdminLayoutPointerFrame() {
	adminLayoutPointerFrame = 0;
	const point = adminLayoutPendingPoint;
	adminLayoutPendingPoint = null;
	const pointer = adminLayoutPointer;
	if (!pointer || !point || point.pointerId !== pointer.pointerId) return;
	const dx = point.clientX - pointer.startX;
	const dy = point.clientY - pointer.startY;
	if (!pointer.moved && Math.hypot(dx, dy) < 3) return;
	pointer.moved = true;
	const item = pointer.item;
	if (pointer.mode === "resize") {
		const isNavigation = item.area === "navigation";
		const isCompactDashboardLabel = item.area === "dashboard" && ["username", "plan_name"].includes(item.id);
		const minWidth = isNavigation ? 28 : (isCompactDashboardLabel ? 64 : 32);
		const minHeight = isNavigation ? 24 : 20;
		const maxWidth = Math.max(minWidth, pointer.surfaceWidth - pointer.startLeft);
		const maxHeight = isNavigation ? 96 : Math.max(minHeight, pointer.surfaceHeight - pointer.startTop + 240);
		const snappedRight = snapAdminLayoutEdge(pointer.startLeft + Math.max(minWidth, Math.min(maxWidth, pointer.startWidth + dx)), pointer.siblings, "x");
		const snappedBottom = snapAdminLayoutEdge(pointer.startTop + Math.max(minHeight, Math.min(maxHeight, pointer.startHeight + dy)), pointer.siblings, "y");
		const width = Math.max(minWidth, Math.min(maxWidth, snappedRight.edge - pointer.startLeft));
		const height = Math.max(minHeight, Math.min(maxHeight, snappedBottom.edge - pointer.startTop));
		item.width = isNavigation ? Math.round(width) : Math.round((width / pointer.parentWidth) * 10000) / 100;
		item.height = Math.round(height);
		pointer.pendingGeometry = { left: pointer.startLeft, top: pointer.startTop, width, height };
		setAdminLayoutMountedGeometry(pointer.node, pointer.startLeft, pointer.startTop, width, height, { expandSurface: false });
		showAdminLayoutGuides(pointer.surface, snappedRight.guide, snappedBottom.guide, pointer);
	} else {
		const maxLeft = Math.max(0, pointer.surfaceWidth - pointer.startWidth);
		const maxTop = Math.max(0, pointer.surfaceHeight - pointer.startHeight + 240);
		const snappedX = snapAdminLayoutPosition(Math.max(0, Math.min(maxLeft, pointer.startLeft + dx)), pointer.startWidth, pointer.siblings, "x");
		const snappedY = snapAdminLayoutPosition(Math.max(0, Math.min(maxTop, pointer.startTop + dy)), pointer.startHeight, pointer.siblings, "y");
		item.positionX = Math.round(Math.max(-2000, Math.min(2000, snappedX.position - pointer.parentOriginX)) * 100) / 100;
		item.positionY = Math.round(Math.max(-2000, Math.min(4000, snappedY.position - pointer.parentOriginY)) * 100) / 100;
		item.offsetX = 0;
		item.offsetY = 0;
		pointer.pendingGeometry = { left: snappedX.position, top: snappedY.position, width: pointer.startWidth, height: pointer.startHeight };
		pointer.node.style.setProperty("--editor-drag-x", `${snappedX.position - pointer.startLeft}px`);
		pointer.node.style.setProperty("--editor-drag-y", `${snappedY.position - pointer.startTop}px`);
		showAdminLayoutGuides(pointer.surface, snappedX.guide, snappedY.guide, pointer);
	}
}

function flushPendingAdminLayoutPointer(event = null) {
	const pointer = adminLayoutPointer;
	if (!pointer) return;
	if (event && event.pointerId === pointer.pointerId) {
		const dx = event.clientX - pointer.startX;
		const dy = event.clientY - pointer.startY;
		if (pointer.moved || Math.hypot(dx, dy) >= 3) {
			adminLayoutPendingPoint = { pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY };
		}
	}
	if (adminLayoutPointerFrame) {
		cancelAnimationFrame(adminLayoutPointerFrame);
		adminLayoutPointerFrame = 0;
	}
	if (adminLayoutPendingPoint) flushAdminLayoutPointerFrame();
}

function commitAdminLayoutPointerGeometry(pointer) {
	if (!pointer?.pendingGeometry) return;
	const geometry = pointer.pendingGeometry;
	pointer.node.style.removeProperty("--editor-drag-x");
	pointer.node.style.removeProperty("--editor-drag-y");
	setAdminLayoutMountedGeometry(pointer.node, geometry.left, geometry.top, geometry.width, geometry.height);
}

function endAdminLayoutPointer(event) {
	const pointer = adminLayoutPointer;
	if (!pointer || event.pointerId !== pointer.pointerId) return;
	flushPendingAdminLayoutPointer(event);
	if (pointer.moved) {
		commitAdminLayoutPointerGeometry(pointer);
		markAdminLayoutDirty();
		suppressNextLayoutClick = true;
		window.setTimeout(() => { suppressNextLayoutClick = false; }, 0);
		haptic("light");
	}
	finishAdminLayoutPointer();
}

function cancelAdminLayoutPointer(event) {
	const pointer = adminLayoutPointer;
	if (!pointer || event.pointerId !== pointer.pointerId) return;
	if (adminLayoutPointerFrame) cancelAnimationFrame(adminLayoutPointerFrame);
	adminLayoutPointerFrame = 0;
	adminLayoutPendingPoint = null;
	Object.assign(pointer.item, pointer.original);
	pointer.node.style.removeProperty("--editor-drag-x");
	pointer.node.style.removeProperty("--editor-drag-y");
	applyAdminLayoutNodeStyle(pointer.node, pointer.item);
	finishAdminLayoutPointer();
}

function finishAdminLayoutPointer() {
	const pointer = adminLayoutPointer;
	if (adminLayoutPointerFrame) cancelAnimationFrame(adminLayoutPointerFrame);
	adminLayoutPointerFrame = 0;
	adminLayoutPendingPoint = null;
	pointer?.node?.style.removeProperty("--editor-drag-x");
	pointer?.node?.style.removeProperty("--editor-drag-y");
	pointer?.node?.classList.remove("is-manipulating");
	showAdminLayoutGuides(pointer?.surface, null, null, pointer);
	if (pointer?.node?.hasPointerCapture?.(pointer.pointerId)) pointer.node.releasePointerCapture(pointer.pointerId);
	document.body.classList.remove("is-editing-layout");
	adminLayoutPointer = null;
}

function beginAdminProfilePointer(event) {
	if (!state.adminLayoutEditing || state.currentPage !== "settings" || event.button > 0 || !event.isPrimary) return;
	const node = event.target.closest?.("[data-profile-layout-id]");
	if (!node) return;
	const item = state.adminSettingsDraft?.layout?.elements?.find((entry) => entry.area === "profile" && entry.id === node.dataset.profileLayoutId);
	if (!item) return;
	adminProfilePointer = {
		pointerId: event.pointerId,
		startX: event.clientX,
		startY: event.clientY,
		clientX: event.clientX,
		clientY: event.clientY,
		node,
		item,
		moved: false,
	};
	node.setPointerCapture?.(event.pointerId);
}

function moveAdminProfilePointer(event) {
	const pointer = adminProfilePointer;
	if (!pointer || event.pointerId !== pointer.pointerId) return;
	pointer.clientX = event.clientX;
	pointer.clientY = event.clientY;
	if (!pointer.moved && Math.hypot(event.clientX - pointer.startX, event.clientY - pointer.startY) < 6) return;
	event.preventDefault();
	pointer.moved = true;
	pointer.node.classList.add("is-profile-dragging");
	app.querySelectorAll(".profile-group--drop-target").forEach((node) => node.classList.remove("profile-group--drop-target"));
	const hit = document.elementFromPoint(event.clientX, event.clientY);
	hit?.closest?.("[data-profile-group]")?.classList.add("profile-group--drop-target");
}

function endAdminProfilePointer(event) {
	const pointer = adminProfilePointer;
	if (!pointer || event.pointerId !== pointer.pointerId) return;
	if (pointer.moved) {
		const hit = document.elementFromPoint(pointer.clientX, pointer.clientY);
		const groupNode = hit?.closest?.("[data-profile-group]");
		const targetNode = hit?.closest?.("[data-profile-layout-id]");
		if (groupNode) {
			moveAdminProfileItem(pointer.item, groupNode.dataset.profileGroup, targetNode?.dataset?.profileLayoutId || "");
		}
		suppressNextLayoutClick = true;
		window.setTimeout(() => { suppressNextLayoutClick = false; }, 0);
	}
	finishAdminProfilePointer();
	if (pointer.moved) render({ preserveScroll: true });
}

function cancelAdminProfilePointer(event) {
	if (!adminProfilePointer || event.pointerId !== adminProfilePointer.pointerId) return;
	finishAdminProfilePointer();
}

function finishAdminProfilePointer() {
	const pointer = adminProfilePointer;
	pointer?.node?.classList.remove("is-profile-dragging");
	app.querySelectorAll(".profile-group--drop-target").forEach((node) => node.classList.remove("profile-group--drop-target"));
	if (pointer?.node?.hasPointerCapture?.(pointer.pointerId)) pointer.node.releasePointerCapture(pointer.pointerId);
	adminProfilePointer = null;
}

function moveAdminProfileItem(source, group, targetID) {
	if (!source || !PROFILE_GROUP_ORDER.includes(group)) return;
	const items = (state.adminSettingsDraft?.layout?.elements || [])
		.filter((item) => item.area === "profile" && !String(item.id).startsWith("group_"));
	const grouped = Object.fromEntries(PROFILE_GROUP_ORDER.map((name) => [name, []]));
	for (const item of items) {
		const itemGroup = PROFILE_GROUP_ORDER.includes(item.group) ? item.group : PROFILE_DEFAULT_GROUPS[item.id] || "main";
		if (item !== source) grouped[itemGroup].push(item);
	}
	for (const name of PROFILE_GROUP_ORDER) grouped[name].sort((left, right) => Number(left.order || 0) - Number(right.order || 0));
	const targetItems = grouped[group];
	const targetIndex = targetID ? targetItems.findIndex((item) => item.id === targetID) : -1;
	source.group = group;
	targetItems.splice(targetIndex >= 0 ? targetIndex : targetItems.length, 0, source);
	PROFILE_GROUP_ORDER.forEach((name, groupIndex) => {
		grouped[name].forEach((item, itemIndex) => { item.order = groupIndex * 100 + itemIndex; });
	});
	markAdminLayoutDirty();
	haptic("light");
}

function beginAdminPlanPointer(event) {
	if (!state.adminPlanEditing || adminPlanPointer || (event.pointerType === "mouse" && event.button !== 0)) return;
	const handle = event.target.closest?.("[data-admin-plan-drag]");
	const node = handle?.closest?.("[data-admin-plan-id]");
	const sourceID = String(node?.dataset?.adminPlanId || "");
	if (!handle || !node || !sourceID) return;
	event.preventDefault();
	adminPlanPointer = {
		pointerId: event.pointerId,
		startX: event.clientX,
		startY: event.clientY,
		clientX: event.clientX,
		clientY: event.clientY,
		sourceID,
		node,
		handle,
		moved: false,
	};
	handle.setPointerCapture?.(event.pointerId);
}

function moveAdminPlanPointer(event) {
	const pointer = adminPlanPointer;
	if (!pointer || event.pointerId !== pointer.pointerId) return;
	pointer.clientX = event.clientX;
	pointer.clientY = event.clientY;
	if (!pointer.moved && Math.hypot(event.clientX - pointer.startX, event.clientY - pointer.startY) < 6) return;
	pointer.moved = true;
	event.preventDefault();
	pointer.node.classList.add("is-plan-dragging");
	app.querySelectorAll(".is-plan-drop-target").forEach((node) => node.classList.remove("is-plan-drop-target"));
	const hit = document.elementFromPoint(event.clientX, event.clientY);
	const target = hit?.closest?.("[data-admin-plan-id]");
	if (target && target !== pointer.node) target.classList.add("is-plan-drop-target");
	const scroller = app.querySelector(".page-scroll");
	if (scroller) {
		const rect = scroller.getBoundingClientRect();
		const edge = 72;
		if (event.clientY < rect.top + edge) scroller.scrollBy({ top: -12, behavior: "auto" });
		else if (event.clientY > rect.bottom - edge) scroller.scrollBy({ top: 12, behavior: "auto" });
	}
}

function endAdminPlanPointer(event) {
	const pointer = adminPlanPointer;
	if (!pointer || event.pointerId !== pointer.pointerId) return;
	let changed = false;
	if (pointer.moved) {
		const hit = document.elementFromPoint(pointer.clientX, pointer.clientY);
		const target = hit?.closest?.("[data-admin-plan-id]");
		const targetID = String(target?.dataset?.adminPlanId || "");
		if (targetID && targetID !== pointer.sourceID) changed = reorderAdminPlansByID(pointer.sourceID, targetID);
		suppressNextLayoutClick = true;
		window.setTimeout(() => { suppressNextLayoutClick = false; }, 0);
	}
	finishAdminPlanPointer();
	if (changed) {
		state.adminSettingsDirty = true;
		haptic("light");
		render({ preserveScroll: true });
	}
}

function cancelAdminPlanPointer(event) {
	if (!adminPlanPointer || event.pointerId !== adminPlanPointer.pointerId) return;
	finishAdminPlanPointer();
}

function finishAdminPlanPointer() {
	const pointer = adminPlanPointer;
	pointer?.node?.classList.remove("is-plan-dragging");
	app.querySelectorAll(".is-plan-drop-target").forEach((node) => node.classList.remove("is-plan-drop-target"));
	if (pointer?.handle?.hasPointerCapture?.(pointer.pointerId)) pointer.handle.releasePointerCapture(pointer.pointerId);
	adminPlanPointer = null;
}

function reorderAdminPlansByID(sourceID, targetID) {
	const plans = state.adminSettingsDraft?.plans;
	if (!Array.isArray(plans)) return false;
	const sourceIndex = plans.findIndex((item) => String(item.id) === String(sourceID));
	const targetIndex = plans.findIndex((item) => String(item.id) === String(targetID));
	if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) return false;
	const [plan] = plans.splice(sourceIndex, 1);
	plans.splice(targetIndex, 0, plan);
	return true;
}

function findAdminLayoutDropTarget(pointer, clientX, clientY) {
	const selector = pointer.type === "plan" ? "[data-ui-plan-index]" : "[data-ui-layout-index]";
	const candidates = Array.from(pointer.canvas.querySelectorAll(selector)).filter((node) => node !== pointer.node);
	return candidates.find((node) => {
		const rect = node.getBoundingClientRect();
		return clientX >= rect.left && clientX <= rect.right && clientY >= rect.top && clientY <= rect.bottom;
	}) || null;
}

function swapAdminLayoutOrder(sourceIndex, targetIndex) {
	const items = state.adminSettingsDraft?.layout?.elements;
	const source = items?.[sourceIndex];
	const target = items?.[targetIndex];
	if (!source || !target || source.area !== target.area) return;
	[source.order, target.order] = [target.order, source.order];
}

function reorderAdminPlans(sourceIndex, targetIndex) {
	const plans = state.adminSettingsDraft?.plans;
	if (!Array.isArray(plans) || sourceIndex === targetIndex || sourceIndex < 0 || targetIndex < 0 || sourceIndex >= plans.length || targetIndex >= plans.length) return;
	const [plan] = plans.splice(sourceIndex, 1);
	plans.splice(targetIndex, 0, plan);
}

function moveAdminLayoutElement(index, direction) {
	const items = state.adminSettingsDraft?.layout?.elements;
	const current = items?.[index];
	if (!current || !direction) return;
	const ordered = items.map((item, itemIndex) => ({ item, itemIndex })).filter(({ item }) => item.area === current.area).sort((a, b) => a.item.order - b.item.order);
	const position = ordered.findIndex(({ itemIndex }) => itemIndex === index);
	const target = ordered[position + direction];
	if (!target) return;
	const order = current.order;
	current.order = target.item.order;
	target.item.order = order;
	state.adminSettingsDirty = true;
	render();
}

function moveAdminPlan(index, direction) {
	const plans = state.adminSettingsDraft?.plans;
	const target = index + direction;
	if (!Array.isArray(plans) || index < 0 || target < 0 || target >= plans.length) return;
	[plans[index], plans[target]] = [plans[target], plans[index]];
	state.adminSettingsDirty = true;
	haptic("light");
	render({ preserveScroll: true });
}

async function resolveAdminEvent(id) {
	if (!id) return;
	await post("/api/mini-app/admin/events/resolve", { id });
	if (state.data?.admin?.events) state.data.admin.events = state.data.admin.events.filter((item) => Number(item.id) !== id);
	render();
	showToast(state.locale === "en" ? "Event resolved" : "Событие закрыто", "success");
}

async function submitReview() {
  const copy = reviewsText();
  const reviews = state.data?.reviews || {};
  if (!reviews.canCreate) return showToast(copy.alreadyReviewed);
  if (state.reviewDraftRating < 1 || state.reviewDraftRating > 5) return showToast(copy.ratingRequired);
  if (!state.reviewDraftComment.trim()) return showToast(copy.commentPlaceholder);

  state.reviewBusy = "submit-review";
  render();
  try {
    const response = await post("/api/mini-app/reviews/create", {
      rating: state.reviewDraftRating,
      comment: state.reviewDraftComment.trim(),
    });
    state.reviewBusy = "";
    state.reviewComposeOpen = false;
    state.reviewDraftRating = 0;
    state.reviewDraftComment = "";
    state.data = response.data;
    state.locale = pickLocale(response.data?.user?.languageCode || state.locale);
    ensureSelections();
    render();
    showToast(response.message || copy.rewardToast, "success");
  } catch (error) {
    state.reviewBusy = "";
    render();
    throw error;
  }
}

async function deleteAdminReview(id) {
  if (!id || !isAdminUser()) return;
  state.reviewBusy = `delete-review-${id}`;
  render();
  try {
    const response = await post("/api/mini-app/admin/reviews/delete", { id });
    state.data.reviews = response.data;
    state.reviewBusy = "";
    state.activeReviewId = 0;
    state.reviewDetailOpen = false;
    render();
    showToast(reviewsText().deleteSuccess, "success");
  } catch (error) {
    state.reviewBusy = "";
    render();
    throw error;
  }
}

async function refreshSupport({ silent = false } = {}) {
  if (!state.data) return;
  const previous = JSON.stringify(state.data.support || {});
  const response = await post("/api/mini-app/support/refresh");
  state.data.support = response.data;
  const changed = previous !== JSON.stringify(state.data.support || {});
  if (!silent || (changed && !state.supportThreadOpen && !state.supportComposeOpen)) render();
}

async function openSupportTicket(ticketId, { silent = false } = {}) {
  if (!ticketId) return;
  const shouldShowLoading = !silent && (!state.supportThreadOpen || state.activeSupportTicketId !== ticketId || !state.activeSupportThread);
  if (shouldShowLoading) {
    state.activeSupportTicketId = ticketId;
    state.activeSupportThread = null;
    state.supportThreadOpen = true;
    render();
  }
  const response = await post("/api/mini-app/support/thread", { ticketId });
  const previous = JSON.stringify(state.activeSupportThread || {});
  state.activeSupportTicketId = ticketId;
  state.activeSupportThread = response.data;
  state.supportThreadOpen = true;
  const changed = previous !== JSON.stringify(response.data || {});
  if (!silent || changed) render();
  void refreshSupport({ silent: true });
}

async function submitSupportTicket() {
  const scopy = supportText();
  if (state.supportBusy) return;
  if (!state.supportDraftMessage.trim()) return showToast(scopy.messagePlaceholder);

  state.supportBusy = "create-ticket";
  render();
  try {
    const response = await post("/api/mini-app/support/create", {
      subject: state.supportDraftSubject.trim(),
      message: state.supportDraftMessage.trim(),
    });
    state.supportBusy = "";
    state.supportComposeOpen = false;
    state.supportDraftSubject = "";
    state.supportDraftMessage = "";
    state.activeSupportThread = response.data;
    state.activeSupportTicketId = response.data?.ticket?.id || 0;
    state.supportThreadOpen = true;
    render();
    void refreshSupport({ silent: true });
    showToast(scopy.ticketCreated, "success");
  } catch (error) {
    state.supportBusy = "";
    render();
    throw error;
  }
}

async function sendSupportMessage() {
  const scopy = supportText();
  if (state.supportBusy) return;
  const message = state.supportReplyDraft.trim();
  if (!state.activeSupportTicketId || !message) return showToast(scopy.replyPlaceholder);

  const previousThread = cloneSupportThread(state.activeSupportThread);
  state.supportBusy = "send-support-message";
  state.supportReplyDraft = "";
  appendOptimisticSupportMessage(message);
  render();
  try {
    const response = await post("/api/mini-app/support/send", {
      ticketId: state.activeSupportTicketId,
      message,
    });
    state.supportBusy = "";
    state.activeSupportThread = response.data;
    render();
    void refreshSupport({ silent: true });
    showToast(scopy.replySent, "success");
  } catch (error) {
    state.supportBusy = "";
    state.activeSupportThread = previousThread;
    if (!state.supportReplyDraft.trim()) state.supportReplyDraft = message;
    render();
    throw error;
  }
}

async function closeSupportTicket() {
  const scopy = supportText();
  if (state.supportBusy) return;
  if (!state.activeSupportTicketId) return;

  const previousThread = cloneSupportThread(state.activeSupportThread);
  state.supportBusy = "close-support-ticket";
  if (state.activeSupportThread?.ticket) {
    state.activeSupportThread = {
      ...state.activeSupportThread,
      ticket: { ...state.activeSupportThread.ticket, status: "closed" },
      canReply: false,
      canClose: false,
    };
  }
  render();
  try {
    const response = await post("/api/mini-app/support/close", { ticketId: state.activeSupportTicketId });
    state.supportBusy = "";
    state.activeSupportThread = response.data;
    render();
    void refreshSupport({ silent: true });
    showToast(scopy.ticketClosedToast, "success");
  } catch (error) {
    state.supportBusy = "";
    state.activeSupportThread = previousThread;
    render();
    throw error;
  }
}

function appendOptimisticSupportMessage(message) {
  if (!state.activeSupportThread) return;
  const messages = Array.isArray(state.activeSupportThread.messages) ? state.activeSupportThread.messages : [];
  state.activeSupportThread = {
    ...state.activeSupportThread,
    messages: [
      ...messages,
      {
        id: -Date.now(),
        authorRole: isAdminUser() ? "admin" : "customer",
        body: message,
        createdAt: new Date().toISOString(),
        pending: true,
      },
    ],
  };
}

function cloneSupportThread(thread) {
  if (!thread) return null;
  try {
    return JSON.parse(JSON.stringify(thread));
  } catch {
    return thread;
  }
}

function syncSupportPolling() {
  window.clearInterval(supportListPollTimer);
  window.clearInterval(supportThreadPollTimer);
  supportListPollTimer = 0;
  supportThreadPollTimer = 0;

  if (state.currentPage !== "support" || !hasAuth() || previewMode) return;

  if (state.supportThreadOpen && state.activeSupportTicketId) {
    supportThreadPollTimer = window.setInterval(() => {
      openSupportTicket(state.activeSupportTicketId, { silent: true }).catch(() => {});
    }, 4000);
    return;
  }

  supportListPollTimer = window.setInterval(() => {
    refreshSupport({ silent: true }).catch(() => {});
  }, 6000);
}

async function activateTrial() {
  const response = await post("/api/mini-app/trial/activate");
  state.data = response.data;
  state.locale = pickLocale(response.data.user.languageCode);
  ensureSelections();
  state.error = "";
  render();
  showToast(t().trialActivated);
}

async function deleteDevice(hwid) {
  const copy = deviceText();
  const userUuid = String(state.data?.subscription?.userUuid || "").trim();
  if (!hwid || !userUuid) return;

  state.deviceBusyHwid = hwid;
  render();
  try {
    const response = await post("/api/mini-app/devices/delete", { userUuid, hwid });
    state.data.subscription = response.data;
    state.deviceBusyHwid = "";
    render();
    showToast(copy.deleted, "success");
  } catch (error) {
    state.deviceBusyHwid = "";
    render();
    throw error;
  }
}

function getPromoStatus() {
  const appliedPromo = getActivePromo();
  return state.promoValidation || (appliedPromo ? {
    type: "success",
    message: t().promoApplied || "Promo code applied",
  } : null);
}

function updatePromoStatusDom() {
  const statusNode = app.querySelector("[data-promo-status]");
  if (!statusNode) return;
  const status = getPromoStatus();
  statusNode.textContent = status?.message || "";
  statusNode.className = `promo-box__status ${status ? `promo-box__status--${status.type}` : "promo-box__status--empty"}`;
}

function updateCheckoutPriceDom() {
  const action = app.querySelector(".buy-action");
  if (!action) return;
  const plan = getSelectedPlan();
  const method = getSelectedPaymentMethod();
  const payLabel = plan ? `${t().pay} ${formatPlanCheckoutPrice(plan, method?.id, state.locale)}` : t().pay;
  action.disabled = Boolean(state.busyMethod);
  action.innerHTML = `${icon(state.busyMethod ? "refresh" : "cart")}${escapeHtml(payLabel)}`;
}

function syncPromoCheckoutDom(options = {}) {
  updatePromoStatusDom();
  if (options.price) updateCheckoutPriceDom();
}

function schedulePromoAutoApply() {
  clearTimeout(promoApplyTimer);
  const code = normalizePromoCodeValue(state.promoCodeDraft);
  promoApplySeq += 1;
  if (!code) {
    state.promoBusy = "";
    state.appliedPromo = null;
    state.promoValidation = null;
    syncPromoCheckoutDom({ price: true });
    return;
  }
  const seq = promoApplySeq;
  promoApplyTimer = setTimeout(() => {
    void applyPromoCode({ silent: true, seq });
  }, 650);
}

function getPromoValidationMessage(error) {
  if (state.locale === "en") {
    if (error?.code === "promo_not_found") return "This promo code does not exist";
    return error?.message || "Promo code not found";
  }
  if (error?.code === "promo_not_found") return "Данного промокода не существует";
  return error?.message || "Данного промокода не существует";
}

async function applyPromoCode(options = {}) {
  const code = normalizePromoCodeValue(state.promoCodeDraft);
  const silent = Boolean(options.silent);
  const seq = options.seq || ++promoApplySeq;
  if (!code) {
    state.promoValidation = null;
    return silent ? undefined : showToast(t().promoCodeRequired || "Enter a promo code");
  }

  state.promoBusy = "apply";
  state.promoValidation = {
    type: "pending",
    message: state.locale === "en" ? "Checking promo code..." : "Проверяем промокод...",
  };
  if (silent) syncPromoCheckoutDom();
  else render();
  try {
    const response = await post("/api/mini-app/promocode/apply", { code });
    if (seq !== promoApplySeq || normalizePromoCodeValue(state.promoCodeDraft) !== code) return;
    state.appliedPromo = response.data;
    state.promoCodeDraft = response.data?.code || code;
    state.promoBusy = "";
    state.promoValidation = {
      type: "success",
      message: state.locale === "en" ? "Promo code applied" : "Промокод применен",
    };
    if (silent) syncPromoCheckoutDom({ price: true });
    else render();
    if (!silent) showToast(t().promoApplied || "Promo code applied", "success");
  } catch (error) {
    if (seq !== promoApplySeq || normalizePromoCodeValue(state.promoCodeDraft) !== code) return;
    state.promoBusy = "";
    state.appliedPromo = null;
    state.promoValidation = {
      type: "error",
      message: getPromoValidationMessage(error),
    };
    if (silent) syncPromoCheckoutDom({ price: true });
    else render();
    if (!silent) showToast(state.promoValidation.message, "danger");
  }
}

async function startPayment() {
  const plan = getSelectedPlan();
  const method = getSelectedPaymentMethod()?.id || "";
  if (!plan || !method) return showToast(t().paymentUnavailable);

  state.busyMethod = method;
  render();
  let navigatingAway = false;
  try {
    const response = await post("/api/mini-app/purchase", {
      planId: plan.id || "",
      months: plan.months,
      paymentMethod: method,
      agreementAccepted: true,
      promoCode: getActivePromo()?.code || "",
    });
    const { action, url, purchaseId } = response.data;
    if (action === "open_invoice") {
      if (typeof tg?.openInvoice === "function") {
        tg.openInvoice(url, async (status) => {
          if (status === "paid") {
            state.appliedPromo = null;
            state.promoCodeDraft = "";
            await safeRefresh();
            showToast(t().paymentSuccess, "success");
            return;
          }
          if (status && status !== "pending") {
            showToast(t().paymentCancelled, "danger");
          }
        });
      }
      else openExternal(url);
    } else if (action === "open_in_app") {
      state.appliedPromo = null;
      state.promoCodeDraft = "";
      if (shouldLaunchYookassaInBrowser()) {
        state.payModalOpen = false;
        state.paymentLaunchURL = "";
        state.paymentLaunchPurchaseId = 0;
        state.paymentLaunchModalOpen = false;
        const numericPurchaseId = Number(purchaseId) || 0;
        if (numericPurchaseId > 0) {
          storePendingPayment({
            purchaseId: numericPurchaseId,
            startedAt: Date.now(),
          });
        }
        openExternal(url);
        showToast(t().paymentOpened);
        setTimeout(() => safeRefresh(), 4000);
      } else {
        navigatingAway = true;
        navigateInMiniApp(url);
      }
    } else {
      state.appliedPromo = null;
      state.promoCodeDraft = "";
      openExternal(url);
      showToast(t().paymentOpened);
      setTimeout(() => safeRefresh(), 4000);
    }
  } catch (error) {
    if (String(error?.code || "").startsWith("promo_")) {
      state.appliedPromo = null;
    }
    throw error;
  } finally {
    state.busyMethod = "";
    if (!navigatingAway) render();
  }
}

async function createAdminPromoCode() {
  const code = normalizePromoCodeValue(state.adminPromoCodeDraft);
  const discountPercent = Number.parseInt(String(state.adminPromoDiscountDraft || "").trim(), 10);
  const maxRedemptions = Number.parseInt(String(state.adminPromoLimitDraft || "").trim(), 10);
  const expiresAt = String(state.adminPromoExpiresDraft || "").trim();
  const expiresDate = expiresAt ? new Date(expiresAt) : null;

  if (expiresDate && Number.isNaN(expiresDate.getTime())) {
    return showToast(t().promoInvalidExpiry || "Enter a valid expiry date");
  }
  if (String(state.adminPromoLimitDraft || "").trim() && (!Number.isFinite(maxRedemptions) || maxRedemptions < 0)) {
    return showToast(t().promoInvalidLimit || "Enter a valid user limit");
  }

  state.adminBusy = "create-promo";
  render();
  try {
    const payload = {
      code,
      discountPercent,
      expiresAt: expiresDate ? expiresDate.toISOString() : "",
      maxRedemptions: Number.isFinite(maxRedemptions) && maxRedemptions > 0 ? maxRedemptions : 0,
    };
    const response = await post("/api/mini-app/admin/promocodes/create", payload);
    state.data.admin = response.data;
    state.adminPromoCodeDraft = "";
    state.adminPromoDiscountDraft = "";
    state.adminPromoLimitDraft = "";
    state.adminPromoExpiresDraft = "";
    state.adminBusy = "";
    render();
    showToast(t().promoCreateSuccess || "Promo code created", "success");
  } catch (error) {
    state.adminBusy = "";
    render();
    throw error;
  }
}

async function deleteAdminPromoCode(id) {
  if (!id) return;
  state.adminBusy = `delete-${id}`;
  render();
  try {
    const response = await post("/api/mini-app/admin/promocodes/delete", { id });
    state.data.admin = response.data;
    state.adminBusy = "";
    render();
    showToast(t().promoDeleteSuccess || "Promo code deleted", "success");
  } catch (error) {
    state.adminBusy = "";
    render();
    throw error;
  }
}

async function saveAdminIntegration(provider) {
	const draft = state.adminIntegrationDrafts[provider];
	if (!draft || state.adminIntegrationBusy) return;
	state.adminIntegrationBusy = provider;
	render({ preserveScroll: true });
	try {
		const response = await post("/api/mini-app/admin/integrations/update", {
			provider,
			enabled: Boolean(draft.enabled),
			fields: draft.fields || {},
		});
		const integrations = state.data?.admin?.integrations || [];
		const index = integrations.findIndex((item) => item.id === provider);
		if (index >= 0) integrations[index] = response.data;
		state.adminIntegrationDrafts[provider] = {
			enabled: Boolean(response.data?.enabled),
			fields: Object.fromEntries((response.data?.fields || []).map((field) => [field.key, field.secret ? "" : String(field.value || "")])),
		};
		state.adminIntegrationBusy = "";
		render({ preserveScroll: true });
		showToast("Интеграция сохранена", "success");
	} catch (error) {
		state.adminIntegrationBusy = "";
		render({ preserveScroll: true });
		throw error;
	}
}

async function findAdminSubscription() {
  const query = String(state.adminSubscriptionQuery || "").trim();
  if (!query) return showToast(mapApiErrorMessage("invalid_subscription_query"));

  state.adminBusy = "find-subscription";
  state.adminSubscriptionResult = null;
  state.adminSubscriptionTargetTelegramID = "";
  render();
  try {
    const response = await post("/api/mini-app/admin/subscriptions/find", { query });
    state.adminSubscriptionResult = response.data || null;
    state.adminBusy = "";
    render();
  } catch (error) {
    state.adminBusy = "";
    render();
    throw error;
  }
}

async function rebindAdminSubscription() {
  const copy = t();
  const item = state.adminSubscriptionResult;
  const targetTelegramId = Number.parseInt(String(state.adminSubscriptionTargetTelegramID || "").trim(), 10);
  if (!item?.userUuid || !Number.isSafeInteger(targetTelegramId) || targetTelegramId <= 0) {
    return showToast(state.locale === "en" ? "Enter a valid Telegram ID" : "Введите корректный Telegram ID");
  }

  const question = typeof copy.adminSubscriptionConfirm === "function"
    ? copy.adminSubscriptionConfirm(item.username || item.id || "subscription", targetTelegramId)
    : `Rebind subscription to ${targetTelegramId}?`;
  if (!window.confirm(question)) return;

  state.adminBusy = "rebind-subscription";
  render();
  try {
    const response = await post("/api/mini-app/admin/subscriptions/rebind", {
      userUuid: item.userUuid,
      targetTelegramId,
    });
    state.adminSubscriptionResult = response.data || item;
    state.adminSubscriptionTargetTelegramID = "";
    state.adminBusy = "";
    render();
    showToast(copy.adminSubscriptionSuccess || "Subscription rebound", "success");
  } catch (error) {
    state.adminBusy = "";
    render();
    throw error;
  }
}

async function safeRefresh() {
  await refreshDashboard({ silent: true });
}

function moveToDashboard() {
  state.animatePageEntry = state.currentPage !== "dashboard";
  state.currentPage = "dashboard";
  state.sidebarOpen = false;
  state.payModalOpen = false;
  state.paymentLaunchModalOpen = false;
  state.paymentLaunchURL = "";
  state.paymentLaunchPurchaseId = 0;
  state.devicesModalOpen = false;
  state.deviceBusyHwid = "";
  state.supportComposeOpen = false;
  state.supportThreadOpen = false;
  state.activeSupportTicketId = 0;
  state.activeSupportThread = null;
  state.supportReplyDraft = "";
  writeSetting(STORAGE_KEYS.page, state.currentPage);
  render({ preserveScroll: false, scrollTop: 0 });
}

function setPage(page) {
  const nextPage = normalizePage(page);
	if (state.adminPlanEditing) {
		if (nextPage !== "buy") return;
		state.currentPage = "buy";
		render({ preserveScroll: true });
		return;
	}
	if (state.adminLayoutEditing) {
		if (!new Set(["dashboard", "settings"]).has(nextPage)) return;
		const sameEditorPage = nextPage === state.currentPage;
		state.currentPage = nextPage;
		state.adminLayoutCategory = nextPage === "settings" ? "profile" : nextPage;
		state.adminLayoutSelection = "";
		previousBottomNavIndex = sameEditorPage ? previousBottomNavIndex : -1;
		haptic("light");
		render({ preserveScroll: false, scrollTop: 0 });
		return;
	}
  const samePage = nextPage === state.currentPage;
  rememberCurrentScroll();
  if (samePage && !state.sidebarOpen && !state.payModalOpen && !state.paymentLaunchModalOpen && !state.devicesModalOpen && !state.reviewComposeOpen && !state.reviewDetailOpen) return;
  state.animatePageEntry = !samePage;
  state.currentPage = nextPage;
  state.sidebarOpen = false;
  state.payModalOpen = false;
  state.paymentLaunchModalOpen = false;
  state.paymentLaunchURL = "";
  state.paymentLaunchPurchaseId = 0;
  state.devicesModalOpen = false;
  state.reviewComposeOpen = false;
  state.reviewDetailOpen = false;
  state.activeReviewId = 0;
  state.reviewDraftRating = 0;
  state.reviewDraftComment = "";
  state.reviewBusy = "";
  state.deviceBusyHwid = "";
  if (nextPage !== "admin") {
    state.adminSection = "home";
    state.adminBusy = "";
    state.adminPromoCodeDraft = "";
    state.adminPromoDiscountDraft = "";
    state.adminPromoLimitDraft = "";
    state.adminPromoExpiresDraft = "";
    state.adminSubscriptionQuery = "";
    state.adminSubscriptionTargetTelegramID = "";
    state.adminSubscriptionResult = null;
  }
  if (nextPage !== "support") {
    state.supportComposeOpen = false;
    state.supportThreadOpen = false;
    state.activeSupportTicketId = 0;
    state.activeSupportThread = null;
    state.supportReplyDraft = "";
  }
  writeSetting(STORAGE_KEYS.page, state.currentPage);
  haptic("light");
  render({ preserveScroll: samePage, scrollTop: samePage ? state.scrollTopByPage[state.currentPage] ?? 0 : 0 });
}

function getCurrentScrollTop() {
  const scroll = app.querySelector(".page-scroll");
  return scroll ? scroll.scrollTop : null;
}

function rememberCurrentScroll() {
  const top = getCurrentScrollTop();
  if (top === null) return;
  state.scrollTopByPage[state.currentPage] = top;
}

function restoreScrollPosition(scrollTop) {
  if (scrollTop === null || scrollTop === undefined) return;
  requestAnimationFrame(() => {
    const scroll = app.querySelector(".page-scroll");
    if (!scroll) return;
    scroll.scrollTop = scrollTop;
    state.scrollTopByPage[state.currentPage] = scrollTop;
  });
}

function getSupportThreadMessagesElement() {
  return app.querySelector("#support-thread-messages");
}

function captureSupportThreadScrollState() {
  const thread = getSupportThreadMessagesElement();
  if (!thread) return null;
  const maxScrollTop = Math.max(0, thread.scrollHeight - thread.clientHeight);
  const top = thread.scrollTop;
  const stickToBottom = maxScrollTop - top <= 28;
  state.supportThreadScrollTop = top;
  return { top, stickToBottom };
}

function restoreSupportThreadScrollState(scrollState) {
  const thread = getSupportThreadMessagesElement();
  if (!thread || !scrollState) return;
  requestAnimationFrame(() => {
    const maxScrollTop = Math.max(0, thread.scrollHeight - thread.clientHeight);
    thread.scrollTop = scrollState.stickToBottom ? maxScrollTop : Math.min(scrollState.top, maxScrollTop);
    state.supportThreadScrollTop = thread.scrollTop;
  });
}

function syncToastAnchor() {
  document.documentElement.style.setProperty("--toast-top", "18px");
}

function syncNativeBackButton() {
  if (!tg?.BackButton) return;
  if (shouldShowNativeBackButton()) tg.BackButton.show();
  else tg.BackButton.hide();
}

function shouldShowNativeBackButton() {
	return Boolean(
		state.adminLayoutEditing || state.adminPlanEditing || state.adminPlanEditorModalOpen ||
    state.supportThreadOpen ||
    state.supportComposeOpen ||
    state.devicesModalOpen ||
    state.payModalOpen ||
    state.paymentLaunchModalOpen ||
    state.reviewComposeOpen ||
    state.reviewDetailOpen ||
    (state.currentPage === "admin" && state.adminSection !== "home") ||
    state.currentPage !== "dashboard"
  );
}

function getNativeBackTargetPage() {
  if (state.currentPage === "faq") return "support";
  if (["servers", "referrals", "reviews", "media", "login-methods", "payments", "terms"].includes(state.currentPage)) return "settings";
  if (["buy", "setup", "support", "settings", "admin"].includes(state.currentPage)) return "dashboard";
  return "dashboard";
}

function handleNativeBackButton() {
	if (state.adminPlanEditorModalOpen) return closeAdminPlanEditorModal();
	if (state.adminPlanEditing) return exitAdminPlanEditor();
	if (state.adminLayoutEditing) return exitAdminLayoutEditor();
  if (state.supportThreadOpen) return requestModalClose("support-thread", () => { state.supportThreadOpen = false; state.activeSupportTicketId = 0; state.activeSupportThread = null; state.supportReplyDraft = ""; });
  if (state.supportComposeOpen) return requestModalClose("support-compose", () => { state.supportComposeOpen = false; state.supportDraftSubject = ""; state.supportDraftMessage = ""; });
  if (state.devicesModalOpen) return requestModalClose("devices", () => { state.devicesModalOpen = false; state.deviceBusyHwid = ""; });
  if (state.payModalOpen) return requestModalClose("pay", () => { state.payModalOpen = false; });
  if (state.paymentLaunchModalOpen) return requestModalClose("payment-launch", () => { state.paymentLaunchModalOpen = false; state.paymentLaunchURL = ""; state.paymentLaunchPurchaseId = 0; });
  if (state.reviewComposeOpen) return requestModalClose("review-compose", () => { state.reviewComposeOpen = false; state.reviewDraftRating = 0; state.reviewDraftComment = ""; state.reviewBusy = ""; });
  if (state.reviewDetailOpen) return requestModalClose("review-detail", () => { state.activeReviewId = 0; state.reviewDetailOpen = false; });
  if (state.currentPage === "admin" && state.adminSection !== "home") {
    state.adminSection = "home";
    return render();
  }
  return setPage(getNativeBackTargetPage());
}

function getActiveModalName() {
	if (state.adminPlanEditorModalOpen) return "admin-plan-editor";
  if (state.supportThreadOpen) return "support-thread";
  if (state.supportComposeOpen) return "support-compose";
  if (state.devicesModalOpen) return "devices";
  if (state.payModalOpen) return "pay";
  if (state.paymentLaunchModalOpen) return "payment-launch";
  if (state.reviewComposeOpen) return "review-compose";
  if (state.reviewDetailOpen) return "review-detail";
  return "";
}

function isModalVisible(name, isOpen) {
  return isOpen || closingModalName === name;
}

function modalStateClass(name) {
  if (closingModalName === name) return "modal--closing";
  if (animatedModalName === name) return "modal--animate";
  return "";
}

function requestModalClose(name, onClosed) {
  window.clearTimeout(closingModalTimer);
  closingModalName = name;
  animatedModalName = "";
  render();
  closingModalTimer = window.setTimeout(() => {
    closingModalName = "";
    previousActiveModalName = "";
    onClosed();
    render();
  }, MODAL_CLOSE_MS);
}

function applyAppearance() {
	const appearance = getRuntimeSettings()?.appearance || {};
	const colors = appearance.colors || {};
	const accentColor = colors.accent || PALETTE.accent.accent;
	const unlimitedBadgeColor = colors.unlimitedBadge || "#949494";
	const accent = {
		accent: accentColor,
		strong: accentColor,
		soft: hexToRGBA(accentColor, 0.12),
		border: hexToRGBA(accentColor, 0.3),
		particle: hexToRGBA(accentColor, 0.34),
	};
  state.theme = "dark";
  document.documentElement.dataset.theme = "dark";
	const backgroundMode = ["animated", "grid", "grid2", "solid"].includes(appearance.backgroundMode) ? appearance.backgroundMode : "animated";
	document.documentElement.dataset.background = backgroundMode;
	document.documentElement.dataset.frames = appearance.showFrames === false ? "off" : "on";
	document.documentElement.dataset.compact = appearance.compact === false ? "off" : "on";
	const variables = {
		"--bg": colors.background,
		"--surface": colors.surface,
		"--surface-strong": colors.surfaceStrong,
		"--surface-soft": colors.surfaceStrong ? hexToRGBA(colors.surfaceStrong, 0.9) : "",
		"--text": colors.text,
		"--muted": colors.muted,
		"--muted-2": colors.muted ? hexToRGBA(colors.muted, 0.7) : "",
		"--line": colors.border,
		"--line-soft": colors.border ? hexToRGBA(colors.border, 0.58) : "",
		"--control-bg": colors.button,
		"--control-border": colors.border,
		"--control-border-soft": colors.border ? hexToRGBA(colors.border, 0.72) : "",
		"--button-text": colors.buttonText,
		"--icon-color": colors.icon,
		"--glass-selected": colors.surfaceStrong,
		"--success": colors.success,
		"--danger": colors.danger,
		"--grid-background": colors.gridBackground,
		"--grid-line": colors.gridLine,
		"--grid-glow-left": colors.gridGlowLeft ? hexToRGBA(colors.gridGlowLeft, 0.28) : "",
		"--grid-glow-right": colors.gridGlowRight ? hexToRGBA(colors.gridGlowRight, 0.28) : "",
		"--grid2-background": colors.grid2Background,
		"--grid2-line": colors.grid2Line,
		"--grid2-glow": colors.grid2Glow,
		"--wave-background": colors.waveBackground,
		"--wave-dot": colors.waveDot,
	};
	Object.entries(variables).forEach(([name, value]) => { if (value) document.documentElement.style.setProperty(name, value); });
  document.documentElement.style.setProperty("--accent", accent.accent);
  document.documentElement.style.setProperty("--accent-strong", accent.strong);
  document.documentElement.style.setProperty("--accent-soft", accent.soft);
  document.documentElement.style.setProperty("--accent-border", accent.border);
  document.documentElement.style.setProperty("--particle-color", accent.particle);
  document.documentElement.style.setProperty("--unlimited-badge-color", unlimitedBadgeColor);
  document.documentElement.style.setProperty("--unlimited-badge-bg", hexToRGBA(unlimitedBadgeColor, 0.3));
  document.documentElement.style.setProperty("--unlimited-badge-border", hexToRGBA(unlimitedBadgeColor, 0.72));
  updateWaveColorFilter(colors.waveBackground || "#000000", colors.waveDot || "#ebebeb");
  particleEngine.setColor(accent.particle);
  if (themeMeta) themeMeta.setAttribute("content", PALETTE.themeColor.dark);
	if (tg) {
    const color = PALETTE.themeColor.dark;
    if (typeof tg.setHeaderColor === "function") tg.setHeaderColor(color);
    if (typeof tg.setBackgroundColor === "function") tg.setBackgroundColor(color);
  }
	syncBackgroundEngines();
}

function syncBackgroundEngines() {
	const backgroundMode = document.documentElement.dataset.background || "animated";
	const paused = document.hidden || state.adminLayoutEditing || state.adminPlanEditing;
	const waveShouldRun = backgroundMode === "animated" && !paused;
	particleEngine.setPaused?.(backgroundMode !== "animated" || paused);
	const wave = window.__linkBotWave;
	if (!wave) return;
	if (waveShouldRun) wave.resume?.();
	else wave.pause?.();
}

function updateWaveColorFilter(backgroundHex, dotHex) {
	const matrix = document.getElementById("wave-color-matrix");
	if (!matrix) return;
	const background = hexToRGBComponents(backgroundHex, [0, 0, 0]);
	const dot = hexToRGBComponents(dotHex, [235, 235, 235]);
	const rows = background.map((base, channel) => {
		const delta = (dot[channel] - base) / 255;
		return [delta * 0.2126, delta * 0.7152, delta * 0.0722, 0, base / 255];
	});
	matrix.setAttribute("values", [...rows.flat(), 0, 0, 0, 1, 0].map((value) => Number(value).toFixed(6)).join(" "));
}

function hexToRGBComponents(hex, fallback) {
	const value = String(hex || "").replace("#", "");
	if (!/^[0-9a-f]{6}$/i.test(value)) return fallback;
	const number = Number.parseInt(value, 16);
	return [(number >> 16) & 255, (number >> 8) & 255, number & 255];
}

function hexToRGBA(hex, alpha) {
	const value = String(hex || "").replace("#", "");
	if (!/^[0-9a-f]{6}$/i.test(value)) return `rgba(186,23,61,${alpha})`;
	const number = Number.parseInt(value, 16);
	return `rgba(${(number >> 16) & 255},${(number >> 8) & 255},${number & 255},${alpha})`;
}

function getPageTitle(page, short = false) {
  const copy = t();
	if (page === "admin" && !short && state.adminSection !== "home") {
		const labels = state.locale === "en" ? {
			maintenance: "Maintenance", diagnostics: "Diagnostics", features: "Functions", content: "Content", appearance: "Appearance", layout: "UI builder", plans: "Plans", broadcast: "Broadcast", subscriptions: "Subscription binding", promocodes: "Promo codes", integrations: "Integrations",
		} : {
			maintenance: "Режим аварии", diagnostics: "Диагностика", features: "Функции", content: "Контент", appearance: "Оформление", layout: "Конструктор UI", plans: "Тарифы", broadcast: "Рассылка", subscriptions: "Привязка подписок", promocodes: "Промокоды", integrations: "Интеграции",
		};
		return labels[state.adminSection] || copy.pageAdmin || "Admin panel";
	}
  const map = {
    dashboard: short ? copy.navDashboard : copy.pageDashboard,
    buy: short ? copy.navBuy : copy.pageBuy,
    setup: copy.pageSetup,
    support: short ? copy.navSupport : copy.pageSupport,
    faq: copy.pageFaq,
    reviews: copy.feedback,
    referrals: copy.pageReferrals,
    servers: copy.pageServers || copy.serverStatus,
    settings: short ? copy.navSettings : copy.pageSettings,
    media: mediaLabel(),
    "login-methods": loginMethodsLabel(),
    payments: copy.paymentsTitle || "Payments",
    terms: copy.tos,
    admin: short ? (copy.navAdmin || "Admin") : (copy.pageAdmin || "Admin panel"),
  };
  return map[page] || copy.pageDashboard;
}

function headerBackAction() {
  if (state.currentPage === "dashboard") return 'data-action="open-sidebar"';
  if (["media", "login-methods", "terms"].includes(state.currentPage)) return 'data-action="go-page" data-value="settings"';
  if (state.currentPage === "admin" && state.adminSection !== "home") return 'data-action="close-admin-section"';
  return 'data-action="go-home"';
}

function bottomNavIcon(page) {
  return { dashboard: "houseLine", buy: "cartShopping", support: "headphonesAlt", settings: "userAlt", admin: "grid" }[page] || "houseLine";
}

function getBottomNavActivePage() {
  if (state.currentPage === "faq") return "support";
  if (["media", "login-methods", "payments", "terms", "referrals", "servers"].includes(state.currentPage)) return "settings";
  return state.currentPage;
}

function platformIcon(platform) {
  return { windows: "windows", android: "android", iphone: "apple", mac: "mac" }[platform] || "windows";
}

function isSubscriptionActive() {
  return state.data?.subscription?.status === "active";
}

function supportTicketTitle(ticket) {
  const scopy = supportText();
  return String(ticket?.subject || "").trim() || scopy.ticketFallback(ticket?.id || 0);
}

function formatSupportStatus(status) {
  if (status === "closed") return state.locale === "en" ? "Closed" : "Закрыто";
  return state.locale === "en" ? "Open" : "Открыто";
}

function formatSupportDate(value) {
  if (!value) return "—";
  return new Intl.DateTimeFormat(state.locale === "en" ? "en-US" : "ru-RU", {
    day: "2-digit",
    month: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function formatTelegramUsername(value) {
  const username = String(value || "").trim().replace(/^@+/u, "");
  return username ? `@${username}` : "";
}

function formatTermsDate(value) {
  if (!value) return "—";
  return new Intl.DateTimeFormat(state.locale === "en" ? "en-US" : "ru-RU", {
    day: "numeric",
    month: "long",
    year: "numeric",
  }).format(new Date(value));
}

function formatReviewDate(value) {
  if (!value) return "—";
  return new Intl.DateTimeFormat(state.locale === "en" ? "en-US" : "ru-RU", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(new Date(value));
}

function formatAverageRating(value, locale) {
  const numeric = Number(value || 0);
  return new Intl.NumberFormat(locale === "en" ? "en-US" : "ru-RU", {
    minimumFractionDigits: numeric > 0 && !Number.isInteger(numeric) ? 1 : 0,
    maximumFractionDigits: 1,
  }).format(numeric);
}

function renderRatingStars(rating, interactive = false, className = "") {
  const current = Math.max(0, Math.min(5, Number(rating || 0)));
  return Array.from({ length: 5 }, (_, index) => {
    const starValue = index + 1;
    const active = starValue <= current;
    const classes = ["rating-star"];
    if (className) classes.push(className);
    if (active) classes.push("is-active");
    const iconName = active ? "starFilled" : "star";
    if (!interactive) {
      return `<span class="${classes.join(" ")}">${icon(iconName)}</span>`;
    }
    return `<button class="${classes.join(" ")}" type="button" data-action="set-review-rating" data-value="${starValue}" aria-label="${escapeAttribute(`${starValue}`)}">${icon(iconName)}</button>`;
  }).join("");
}

function reviewsSummaryHint() {
  const reviews = state.data?.reviews || {};
  const count = Number(reviews.count || 0);
  const average = formatAverageRating(reviews.average || 0, state.locale);
  if (!count) return state.locale === "en" ? "No reviews yet" : "Пока нет отзывов";
  return `${average} • ${reviewsText().reviewsCount(count)}`;
}

function getActiveReview() {
  const items = state.data?.reviews?.items || [];
  return items.find((item) => Number(item.id || 0) === Number(state.activeReviewId || 0)) || state.data?.reviews?.myReview || null;
}

function getCurrentSubscriptionPlanLabel() {
  const subscription = state.data?.subscription || {};
  if (subscription.planLabel) return String(subscription.planLabel);
  if (subscription.isTrial) return getTrialPlanLabel();
  switch (Number(subscription.planMonths || 0)) {
    case 1: return state.locale === "en" ? "1 Month" : "Месяц";
    case 3: return state.locale === "en" ? "3 Months" : "3 Месяца";
    case 6: return state.locale === "en" ? "6 Months" : "6 Месяцев";
    case 12: return state.locale === "en" ? "Yearly" : "Годовой";
    default: return state.locale === "en" ? "Subscription active" : "Подписка";
  }
}

function getTrialPlanLabel() {
  return state.locale === "en" ? "Trial" : "Пробный";
}

function getInactiveSubscriptionLabel() {
  return state.locale === "en" ? "No subscription" : "Нет подписки";
}

function getUntilLabel() {
  return state.locale === "en" ? "Until" : "До";
}

function planKey(plan) {
  if (!plan) return "";
  return String(plan.id || plan.months || "");
}

function getSelectedPlan() {
  const plans = getDisplayedPlans();
  return plans.find((plan) => planKey(plan) === state.selectedPlanId) || plans[0] || null;
}

function getAvailableMethods(plan = getSelectedPlan()) {
  return (state.data?.paymentMethods || [])
    .map((item) => paymentMethodMeta(item.id))
    .filter(Boolean)
    .filter((method) => method.id !== "stars" || Number(plan?.priceStars || 0) > 0);
}

function getSelectedPaymentMethod() {
  const methods = getAvailableMethods();
  return methods.find((method) => method.id === state.paymentMethod) || methods[0] || null;
}

function paymentMethodMeta(id) {
  const copy = t();
  const map = {
    sbp: { id: "sbp", label: copy.payMethodSbp, hint: copy.payMethodSbpHint, logo: PAYMENT_LOGO_URLS.sbp },
    card: { id: "card", label: copy.payMethodCard, hint: copy.payMethodCardHint, logo: PAYMENT_LOGO_URLS.card },
    stars: { id: "stars", label: copy.payMethodStars, hint: copy.payMethodStarsHint, logo: PAYMENT_LOGO_URLS.stars },
    crypto: { id: "crypto", label: copy.payMethodCrypto, hint: copy.payMethodCryptoHint, logo: PAYMENT_LOGO_URLS.crypto },
		lava: { id: "lava", label: "LAVA", hint: "Оплата через LAVA", logo: PAYMENT_LOGO_URLS.lava },
		wata: { id: "wata", label: "WATA", hint: "Карты и СБП", logo: PAYMENT_LOGO_URLS.wata },
		platega: { id: "platega", label: "Platega", hint: "Оплата через Platega", logo: PAYMENT_LOGO_URLS.platega },
		freekassa: { id: "freekassa", label: "FreeKassa", hint: "Оплата через FreeKassa", logo: PAYMENT_LOGO_URLS.freekassa },
		heleket: { id: "heleket", label: "Heleket", hint: "Оплата криптовалютой", logo: PAYMENT_LOGO_URLS.heleket },
		pally: { id: "pally", label: "Pally", hint: "Оплата картой или через СБП", logo: PAYMENT_LOGO_URLS.pally },
  };
  return map[id] || null;
}

function renderPaymentMethodLogo(method) {
  const source = String(method?.logo || "").trim();
  if (!source) return icon("wallet");
  return `<img class="payment-brand-logo" src="${escapeAttribute(source)}" alt="" aria-hidden="true">`;
}

function pageClass(page) {
  if (state.currentPage !== page) return "";
  return pageAnimationEnabled ? "active page--animate" : "active";
}

function pickLocale(code) {
  return String(code || "ru").toLowerCase().startsWith("en") ? "en" : "ru";
}

function openExternal(url) {
  if (!url) return;
  haptic("light");
  if (tg) {
    if (typeof tg.openTelegramLink === "function" && /^https:\/\/t\.me\//i.test(url)) return tg.openTelegramLink(url);
    if (typeof tg.openLink === "function") {
      try {
        return tg.openLink(url, { try_instant_view: false });
      } catch {
        return tg.openLink(url);
      }
    }
  }
  window.open(url, "_blank", "noopener,noreferrer");
}

function openSubscriptionAccess() {
  const link = String(state.data?.subscription?.subscriptionLink || "").trim();
  if (!isSubscriptionActive() || !link) {
    showToast(t().noAccess);
    return setPage("buy");
  }
  state.sidebarOpen = false;
  state.payModalOpen = false;
  state.paymentLaunchModalOpen = false;
  state.devicesModalOpen = false;
  openExternal(link);
  render();
}

function shouldLaunchYookassaInBrowser() {
  const platform = String(tg?.platform || "").toLowerCase();
  if (platform === "android" || platform === "ios") return true;
  const ua = String(navigator.userAgent || "").toLowerCase();
  return /android|iphone|ipad|ipod/.test(ua);
}

function openPreparedPaymentInBrowser() {
  const url = String(state.paymentLaunchURL || "").trim();
  if (!url) return;
  if (state.paymentLaunchPurchaseId > 0) {
    storePendingPayment({
      purchaseId: state.paymentLaunchPurchaseId,
      startedAt: Date.now(),
    });
  }
  openExternal(url);
  state.paymentLaunchModalOpen = false;
  state.paymentLaunchURL = "";
  state.paymentLaunchPurchaseId = 0;
  render();
  showToast(t().paymentOpened);
  setTimeout(() => safeRefresh(), 4000);
}

function navigateInMiniApp(url) {
  if (!url) return;
  haptic("light");
  window.location.assign(url);
}

function getPaymentReturnState() {
  if (urlParams.get("paymentReturn") !== "1") return null;
  return {
    status: urlParams.get("paymentStatus") || "pending",
    purchaseId: Number(urlParams.get("purchaseId") || 0),
  };
}

function clearPaymentReturnState() {
  const url = new URL(window.location.href);
  url.searchParams.delete("paymentReturn");
  url.searchParams.delete("paymentStatus");
  url.searchParams.delete("purchaseId");
  url.searchParams.delete("provider");
  const search = url.searchParams.toString();
  const next = `${url.pathname}${search ? `?${search}` : ""}${url.hash || ""}`;
  window.history.replaceState({}, "", next);
}

function readPendingPayment() {
  try {
    const value = window.sessionStorage.getItem(PENDING_PAYMENT_KEY);
    return value ? JSON.parse(value) : null;
  } catch {
    return null;
  }
}

function storePendingPayment(payload) {
  try {
    window.sessionStorage.setItem(PENDING_PAYMENT_KEY, JSON.stringify(payload));
  } catch {}
}

function clearPendingPayment() {
  try {
    window.sessionStorage.removeItem(PENDING_PAYMENT_KEY);
  } catch {}
}

function showResumePaymentPopup() {
  const copy = t();
  return new Promise((resolve) => {
    if (tg && typeof tg.showPopup === "function") {
      tg.showPopup({
        title: copy.resumePaymentTitle,
        message: copy.resumePaymentText,
        buttons: [
          { id: "continue", type: "default", text: copy.resumePaymentContinue },
          { id: "cancel", type: "destructive", text: copy.resumePaymentReturn },
        ],
      }, (buttonId) => resolve(buttonId || "cancel"));
      return;
    }

    const result = window.confirm(copy.resumePaymentText);
    resolve(result ? "continue" : "cancel");
  });
}

function showToast(message, variant = "neutral") {
  const meta = getToastMeta(variant);
  const iconNode = document.createElement("span");
  iconNode.className = "toast__icon";
  iconNode.setAttribute("aria-hidden", "true");
  iconNode.textContent = meta.icon;

  const textNode = document.createElement("span");
  textNode.className = "toast__text";
  textNode.textContent = message;

  toast.replaceChildren(iconNode, textNode);
  toast.className = `toast toast--${variant}`;
  void toast.offsetWidth;
  toast.classList.add("is-visible");

  clearTimeout(showToast.timeout);
  showToast.timeout = setTimeout(() => {
    toast.classList.remove("is-visible");
  }, 2400);
}

function getToastMeta(variant) {
  if (variant === "success") return { icon: "✓" };
  if (variant === "danger") return { icon: "✕" };
  return { icon: "•" };
}

async function copyToClipboard(value) {
  if (navigator.clipboard?.writeText) return navigator.clipboard.writeText(value);
  const input = document.createElement("textarea");
  input.value = value;
  input.setAttribute("readonly", "");
  input.style.position = "absolute";
  input.style.left = "-9999px";
  document.body.appendChild(input);
  input.select();
  document.execCommand("copy");
  document.body.removeChild(input);
}

function formatDateLabel(value, locale) {
  if (!value) return "—";
  return new Intl.DateTimeFormat(locale === "en" ? "en-US" : "ru-RU", { day: "2-digit", month: "short", hour: "2-digit", minute: "2-digit" }).format(new Date(value));
}

function formatShortDateLabel(value, locale) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "вЂ”";
  const day = String(date.getDate()).padStart(2, "0");
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const year = String(date.getFullYear()).slice(-2);
  return `${day}.${month}.${year}`;
}

function formatCurrency(value, locale) {
  return new Intl.NumberFormat(locale === "en" ? "en-US" : "ru-RU", { style: "currency", currency: "RUB", maximumFractionDigits: 0 }).format(value);
}

function formatPaymentAmount(amount, currency, invoiceType) {
  const numeric = Number(amount || 0);
  if (invoiceType === "telegram" || currency === "XTR") {
    return `${formatNumber(numeric, state.locale)} Stars`;
  }
  return formatCurrency(numeric, state.locale);
}

function formatPaymentStatus(status) {
  if (status === "paid") return state.locale === "en" ? "Paid" : "Оплачено";
  if (status === "pending" || status === "new") return state.locale === "en" ? "Pending" : "В ожидании";
  if (status === "cancel" || status === "canceled") return state.locale === "en" ? "Canceled" : "Отменено";
  if (status === "failed") return state.locale === "en" ? "Failed" : "Ошибка";
  return status ? String(status) : "—";
}

function formatPaymentDate(value) {
  if (!value) return "—";
  return new Intl.DateTimeFormat(state.locale === "en" ? "en-US" : "ru-RU", { day: "2-digit", month: "2-digit", year: "2-digit" }).format(new Date(value));
}

function paymentMethodTitleForHistory(invoiceType, copy) {
  if (invoiceType === "telegram") return copy.payMethodStars || "Telegram Stars";
  if (invoiceType === "crypto") return copy.payMethodCrypto || "Crypto";
  return copy.payMethodCard || "Card";
}

function formatNumber(value, locale) {
  return new Intl.NumberFormat(locale === "en" ? "en-US" : "ru-RU").format(Number(value || 0));
}

function formatTrafficBadgeLabel(usedBytes, limitBytes, locale) {
  const limit = Number(limitBytes || 0);
  if (!Number.isFinite(limit)) return "";
  if (limit <= 0) return `${formatTrafficUsageValue(usedBytes, locale)}/∞ GB`;
  return `${formatTrafficUsageValue(usedBytes, locale)}/${formatTrafficLimitValue(limitBytes, locale)} GB`;
}

function formatTrafficGbValue(bytes) {
  const value = Number(bytes || 0);
  if (!Number.isFinite(value) || value <= 0) return 0;
  return value / (1024 ** 3);
}

function formatTrafficUsageValue(bytes, locale) {
  const value = formatTrafficGbValue(bytes);
  if (value <= 0) return "0";
  if (value < 10) return formatCompactDecimal(value, locale, 1);
  return formatCompactDecimal(value, locale, 0);
}

function formatTrafficLimitValue(bytes, locale) {
  const value = formatTrafficGbValue(bytes);
  if (value <= 0) return "0";
  if (value < 10 && !Number.isInteger(value)) return formatCompactDecimal(value, locale, 1);
  return formatCompactDecimal(value, locale, 0);
}

function formatCompactDecimal(value, locale, maximumFractionDigits) {
  return new Intl.NumberFormat(locale === "en" ? "en-US" : "ru-RU", {
    minimumFractionDigits: 0,
    maximumFractionDigits,
  }).format(value);
}

function formatDeviceBadgeLabel(usedCount, limitCount, locale) {
  const limit = Number(limitCount || 0);
  if (!Number.isFinite(limit)) return "";
  const used = formatNumber(Math.max(0, Number(usedCount || 0)), locale);
  if (limit <= 0) return `${used}/\u221E ${locale === "en" ? "devices" : "устройств"}`;
  return `${used}/${formatNumber(limit, locale)} ${locale === "en" ? "devices" : "устройств"}`;
}

function formatReferralRewardLabel(referral, locale) {
  if (!referral?.enabled) return "—";

  const parts = [];
  const bonusDays = Math.max(0, Number(referral?.bonusDays || 0));
  const bonusTrafficBytes = Math.max(0, Number(referral?.bonusTrafficBytes || 0));

  if (bonusDays > 0) {
    if (locale === "en") {
      parts.push(`+${formatNumber(bonusDays, locale)} day${bonusDays === 1 ? "" : "s"}`);
    } else {
      parts.push(`+${formatNumber(bonusDays, locale)} ${pluralizeRu(bonusDays, ["день", "дня", "дней"])}`);
    }
  }

  if (bonusTrafficBytes > 0) {
    parts.push(`+${formatTrafficLimitValue(bonusTrafficBytes, locale)} GB`);
  }

  return parts.length ? parts.join(" · ") : "—";
}

function getSubscriptionDevices() {
  const devices = state.data?.subscription?.devices;
  return Array.isArray(devices) ? devices : [];
}

function getDeviceTitle(device) {
  return String(device?.deviceModel || device?.platform || device?.hwid || "Device").trim();
}

function getDeviceSecondary(device) {
  return [device?.platform, device?.osVersion, device?.userAgent].filter(Boolean).join(" • ");
}

function formatDeviceCreatedAt(value) {
  if (!value) return "—";
  return new Intl.DateTimeFormat(state.locale === "en" ? "en-US" : "ru-RU", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function getDashboardAvatarUrl() {
  return String(state.data?.user?.photoUrl || "").trim();
}

function getDashboardUserLabel() {
  const panelUsername = String(state.data?.user?.panelUsername || "").trim();
  if (panelUsername) return panelUsername;

  const username = String(state.data?.user?.username || "").trim();
  if (username) return username.startsWith("@") ? username : `@${username}`;

  const firstName = String(state.data?.user?.firstName || "").trim();
  if (firstName) return firstName;

  return String(state.data?.brand?.name || "Link-Bot").trim();
}

function getDashboardAvatarFallback() {
  const panelUsername = String(state.data?.user?.panelUsername || "").trim().replace(/^@/, "");
  if (panelUsername) return panelUsername.charAt(0).toUpperCase();

  const username = String(state.data?.user?.username || "").trim().replace(/^@/, "");
  if (username) return username.charAt(0).toUpperCase();

  const firstName = String(state.data?.user?.firstName || "").trim();
  if (firstName) return firstName.charAt(0).toUpperCase();

  return "B";
}

function normalizePromoCodeValue(value) {
  return String(value || "").trim().toUpperCase().replace(/\s+/g, "");
}

function getActivePromo() {
  if (!state.appliedPromo?.code) return null;
  return normalizePromoCodeValue(state.promoCodeDraft) === state.appliedPromo.code ? state.appliedPromo : null;
}

function getDiscountedPlanPrice(plan, methodId) {
  if (!plan) return 0;
  const base = methodId === "stars" ? Number(plan.priceStars || 0) : Number(plan.priceRub || 0);
  if (base <= 0) return 0;
  const promo = getActivePromo();
  if (!promo?.discountPercent) return base;
  return Math.max(1, Math.round(base * (100 - promo.discountPercent) / 100));
}

function formatPlanCheckoutPrice(plan, methodId, locale) {
  if (!plan) return "";
  const amount = getDiscountedPlanPrice(plan, methodId);
  if (methodId === "stars" && plan.priceStars > 0) return `${formatNumber(amount, locale)} Stars`;
  return formatCurrency(amount, locale);
}

function linkHint(link) {
  if (!link) return "";
  try {
    return new URL(link).host.replace(/^www\./i, "");
  } catch {
    return link;
  }
}

function maskLink(link) {
  if (!link) return "";
  try {
    const url = new URL(link);
    return `${url.host}${url.pathname.slice(0, 16)}${url.pathname.length > 16 ? "…" : ""}`;
  } catch {
    return `${link.slice(0, 22)}${link.length > 22 ? "…" : ""}`;
  }
}

function countryFlag(code) {
  const value = String(code || "").trim().toUpperCase();
  if (!/^[A-Z]{2}$/u.test(value)) return "";
  return Array.from(value).map((char) => String.fromCodePoint(127397 + char.charCodeAt(0))).join("");
}

function readSetting(key, fallback) {
  try { return window.localStorage.getItem(key) || fallback; } catch { return fallback; }
}

function writeSetting(key, value) {
  try { window.localStorage.setItem(key, value); } catch { return; }
}

function readSessionSetting(key, fallback) {
  try { return window.sessionStorage.getItem(key) || fallback; } catch { return fallback; }
}

function writeSessionSetting(key, value) {
  try {
    if (value) window.sessionStorage.setItem(key, value);
    else window.sessionStorage.removeItem(key);
  } catch {
    return;
  }
}

function normalizePage(value) {
  if (!PAGES.includes(value)) return "dashboard";
  if (value === "admin" && !isAdminUser()) return "dashboard";
  return value;
}

function getEntryPage() {
  return normalizePage(urlParams.get("page") || "dashboard");
}

function haptic(kind) {
  if (tg?.HapticFeedback?.impactOccurred) tg.HapticFeedback.impactOccurred(kind);
}

function escapeHtml(value) {
  return String(value ?? "").replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char]);
}

function escapeAttribute(value) {
  return escapeHtml(value).replace(/"/g, "&quot;");
}

function pluralizeRu(value, forms) {
  const number = Math.abs(value) % 100;
  const digit = number % 10;
  if (number > 10 && number < 20) return forms[2];
  if (digit > 1 && digit < 5) return forms[1];
  if (digit === 1) return forms[0];
  return forms[2];
}

function createParticleEngine() {
  const canvas = document.getElementById("particles-canvas");
  if (!canvas) {
    return {
      start() {},
      setColor() {},
		setPaused() {},
    };
  }
  const context = canvas.getContext("2d");
  const particles = [];
  let running = false;
	let paused = false;
	let animationFrame = 0;
  let color = "rgba(160,28,28,0.66)";

  function withAlpha(alpha) {
    return color.replace(/[\d.]+\)$/u, `${alpha})`);
  }

  function drawStar(particle) {
    const outer = particle.size;
    const inner = outer * particle.innerRatio;
    const sparkle = 0.9 + Math.sin(particle.wobble * 1.6) * 0.16;
    const spikes = 4;

    context.scale(sparkle, sparkle);
    context.beginPath();
    for (let i = 0; i < spikes * 2; i += 1) {
      const angle = (Math.PI / spikes) * i - Math.PI / 2;
      const radius = i % 2 === 0 ? outer : inner;
      const x = Math.cos(angle) * radius;
      const y = Math.sin(angle) * radius;
      if (i === 0) context.moveTo(x, y);
      else context.lineTo(x, y);
    }
    context.closePath();
    context.fill();

    context.strokeStyle = `rgba(255,255,255,${Math.min(0.18, particle.alpha * 0.72)})`;
    context.lineWidth = 0.7;
    context.stroke();

    context.fillStyle = `rgba(255,255,255,${Math.min(0.24, particle.alpha * 0.95)})`;
    context.beginPath();
    context.arc(0, 0, Math.max(0.8, outer * 0.12), 0, Math.PI * 2);
    context.fill();
  }

  function resize() {
    const ratio = Math.max(window.devicePixelRatio || 1, 1);
    canvas.width = window.innerWidth * ratio;
    canvas.height = window.innerHeight * ratio;
    canvas.style.width = `${window.innerWidth}px`;
    canvas.style.height = `${window.innerHeight}px`;
    context.setTransform(ratio, 0, 0, ratio, 0, 0);
  }

  function seed() {
    particles.length = 0;
    for (let i = 0; i < 42; i += 1) {
      particles.push({
        x: Math.random() * window.innerWidth,
        y: Math.random() * window.innerHeight,
        vx: (Math.random() - 0.5) * 0.18,
        vy: (Math.random() - 0.5) * 0.18,
        size: 2.6 + Math.random() * 3.4,
        innerRatio: 0.34 + Math.random() * 0.14,
        rotation: Math.random() * Math.PI * 2,
        rotSpeed: (Math.random() - 0.5) * 0.01,
        alpha: 0.12 + Math.random() * 0.2,
        sway: 0.16 + Math.random() * 0.54,
        wobble: Math.random() * Math.PI * 2,
        wobbleSpeed: 0.01 + Math.random() * 0.016,
      });
    }
  }

  function frame() {
	animationFrame = 0;
	if (!running || paused) return;
    context.clearRect(0, 0, window.innerWidth, window.innerHeight);
      particles.forEach((particle) => {
        particle.x += particle.vx;
        particle.y += particle.vy;
        particle.rotation += particle.rotSpeed;
        particle.wobble += particle.wobbleSpeed;
        if (particle.x < -20) particle.x = window.innerWidth + 20;
        if (particle.x > window.innerWidth + 20) particle.x = -20;
        if (particle.y < -20) particle.y = window.innerHeight + 20;
        if (particle.y > window.innerHeight + 20) particle.y = -20;
        const offsetX = Math.sin(particle.wobble) * particle.sway;
        const offsetY = Math.cos(particle.wobble * 0.76) * particle.sway * 0.45;
        context.save();
        context.translate(particle.x + offsetX, particle.y + offsetY);
        context.rotate(particle.rotation + Math.sin(particle.wobble) * 0.1);
        context.fillStyle = withAlpha(particle.alpha);
        context.shadowColor = withAlpha(Math.min(0.16, particle.alpha * 0.6));
        context.shadowBlur = 8;
        drawStar(particle);
        context.restore();
      });
	animationFrame = requestAnimationFrame(frame);
  }

  window.addEventListener("resize", () => { resize(); seed(); });

  return {
    start() {
      if (running) return;
      running = true;
      resize();
      seed();
		if (!paused) frame();
    },
    setColor(nextColor) { color = nextColor; },
	setPaused(nextPaused) {
		const shouldPause = Boolean(nextPaused);
		if (paused === shouldPause) return;
		paused = shouldPause;
		if (paused && animationFrame) {
			cancelAnimationFrame(animationFrame);
			animationFrame = 0;
		} else if (!paused && running && !animationFrame) {
			frame();
		}
	},
  };
}

function icon(name) {
  const icons = {
		wrench: `<svg viewBox="0 0 24 24" fill="none"><path d="M14.7 6.2a4.8 4.8 0 0 0-6.1 6.1L3.8 17a2.1 2.1 0 1 0 3 3l4.8-4.8a4.8 4.8 0 0 0 6.1-6.1l-2.8 2.8-2.8-.7-.7-2.8 3.3-2.2Z" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
		maintenanceKey: `<svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false"><path d="M12 8v5m0 3h.01M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
		alert: `<svg viewBox="0 0 24 24" fill="none"><path d="M12 8v5M12 17h.01M10.3 4.9 3.4 17a2 2 0 0 0 1.7 3h13.8a2 2 0 0 0 1.7-3L13.7 4.9a2 2 0 0 0-3.4 0Z" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
		sliders: `<svg viewBox="0 0 24 24" fill="none"><path d="M4 7h10M18 7h2M4 17h2M10 17h10M14 4v6M10 14v6" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/><circle cx="16" cy="7" r="2" stroke="currentColor" stroke-width="1.8"/><circle cx="8" cy="17" r="2" stroke="currentColor" stroke-width="1.8"/></svg>`,
		palette: `<svg viewBox="0 0 24 24" fill="none"><path d="M12 3a9 9 0 0 0 0 18h1.2a1.8 1.8 0 0 0 1.2-3.1 1.8 1.8 0 0 1 1.2-3.1H18A3 3 0 0 0 21 12 9 9 0 0 0 12 3Z" stroke="currentColor" stroke-width="1.8"/><circle cx="7.5" cy="10" r="1" fill="currentColor"/><circle cx="10" cy="6.8" r="1" fill="currentColor"/><circle cx="14.3" cy="6.8" r="1" fill="currentColor"/><circle cx="17" cy="10" r="1" fill="currentColor"/></svg>`,
		eye: `<svg viewBox="0 0 24 24" fill="none"><path d="M2.8 12s3.3-6 9.2-6 9.2 6 9.2 6-3.3 6-9.2 6-9.2-6-9.2-6Z" stroke="currentColor" stroke-width="1.8"/><circle cx="12" cy="12" r="2.6" stroke="currentColor" stroke-width="1.8"/></svg>`,
		eyeOff: `<svg viewBox="0 0 24 24" fill="none"><path d="M3 3l18 18M10.6 6.2c.5-.1.9-.2 1.4-.2 5.9 0 9.2 6 9.2 6a16 16 0 0 1-2.4 3.2M6.2 7.7A15.8 15.8 0 0 0 2.8 12s3.3 6 9.2 6c1 0 2-.2 2.8-.5M9.9 9.9a3 3 0 0 0 4.2 4.2" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
		move: `<svg viewBox="0 0 24 24" fill="none"><path d="M12 3v18M3 12h18M12 3 9 6m3-3 3 3M12 21l-3-3m3 3 3-3M3 12l3-3m-3 3 3 3M21 12l-3-3m3 3-3 3" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
		resize: `<svg viewBox="0 0 24 24" fill="none"><path d="M8 16 16 8M11 18l7-7M16 18h2v-2" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
		frame: `<svg viewBox="0 0 24 24" fill="none"><path d="M4 9V5a1 1 0 0 1 1-1h4M15 4h4a1 1 0 0 1 1 1v4M20 15v4a1 1 0 0 1-1 1h-4M9 20H5a1 1 0 0 1-1-1v-4" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
		reset: `<svg viewBox="0 0 24 24" fill="none"><path d="M4 10a8 8 0 1 1 2.3 7.7M4 4v6h6" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
		image: `<svg viewBox="0 0 24 24" fill="none"><rect x="3" y="4" width="18" height="16" rx="2" stroke="currentColor" stroke-width="1.8"/><circle cx="8.5" cy="9" r="1.5" stroke="currentColor" stroke-width="1.6"/><path d="m4 17 4.5-4 3.5 3 3-3 5 4.5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
		arrowUp: `<svg viewBox="0 0 24 24" fill="none"><path d="m6 14 6-6 6 6" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
		arrowDown: `<svg viewBox="0 0 24 24" fill="none"><path d="m6 10 6 6 6-6" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    telegram: `<svg viewBox="0 0 24 24" fill="none"><path d="M20.8 4.7 17.9 19c-.2 1-.8 1.2-1.6.7l-4.4-3.3-2.1 2c-.2.2-.4.4-.9.4l.3-4.5 8.2-7.4c.4-.3-.1-.5-.5-.2L6.8 13.1 2.5 11.8c-.9-.3-.9-.9.2-1.3L19.4 4c.8-.3 1.5.2 1.4.7Z" fill="currentColor"/></svg>`,
    google: `<svg viewBox="0 0 24 24" fill="none"><path d="M21.5 12.2c0-.7-.1-1.3-.2-1.9H12v3.6h5.3a4.5 4.5 0 0 1-2 3v2.4h3.2c1.9-1.7 3-4.2 3-7.1Z" fill="currentColor"/><path d="M12 22c2.7 0 5-.9 6.6-2.5l-3.2-2.4c-.9.6-2 .9-3.4.9-2.6 0-4.8-1.7-5.6-4.1H3.1v2.5A10 10 0 0 0 12 22Z" fill="currentColor"/><path d="M6.4 13.9a6 6 0 0 1 0-3.8V7.6H3.1a10 10 0 0 0 0 8.8l3.3-2.5Z" fill="currentColor"/><path d="M12 6c1.5 0 2.8.5 3.8 1.5l2.9-2.9A9.7 9.7 0 0 0 12 2a10 10 0 0 0-8.9 5.6l3.3 2.5C7.2 7.7 9.4 6 12 6Z" fill="currentColor"/></svg>`,
    googleColor: `<svg viewBox="0 0 24 24" fill="none"><path d="M21.6 12.2c0-.7-.1-1.3-.2-1.9H12v3.6h5.4a4.6 4.6 0 0 1-2 3v2.4h3.2c1.9-1.7 3-4.2 3-7.1Z" fill="#4285F4"/><path d="M12 22c2.7 0 5-.9 6.6-2.5l-3.2-2.4c-.9.6-2 .9-3.4.9-2.6 0-4.8-1.8-5.7-4.1H3.1v2.5A10 10 0 0 0 12 22Z" fill="#34A853"/><path d="M6.3 13.9a6 6 0 0 1 0-3.8V7.6H3.1a10 10 0 0 0 0 8.8l3.2-2.5Z" fill="#FBBC05"/><path d="M12 6c1.5 0 2.8.5 3.8 1.5l2.9-2.9A9.7 9.7 0 0 0 12 2a10 10 0 0 0-8.9 5.6l3.2 2.5C7.2 7.8 9.4 6 12 6Z" fill="#EA4335"/></svg>`,
    key: `<svg viewBox="0 0 24 24" fill="none"><circle cx="8.5" cy="12.5" r="3.5" stroke="currentColor" stroke-width="1.8"/><path d="M12 12.5h8M17 12.5v3M14.8 12.5v2" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    lockAlt: `<svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false"><path d="M12 14.5V16.5M7 10.0288C7.47142 10 8.05259 10 8.8 10H15.2C15.9474 10 16.5286 10 17 10.0288M7 10.0288C6.41168 10.0647 5.99429 10.1455 5.63803 10.327C5.07354 10.6146 4.6146 11.0735 4.32698 11.638C4 12.2798 4 13.1198 4 14.8V16.2C4 17.8802 4 18.7202 4.32698 19.362C4.6146 19.9265 5.07354 20.3854 5.63803 20.673C6.27976 21 7.11984 21 8.8 21H15.2C16.8802 21 17.7202 21 18.362 20.673C18.9265 20.3854 19.3854 19.9265 19.673 19.362C20 18.7202 20 17.8802 20 16.2V14.8C20 13.1198 20 12.2798 19.673 11.638C19.3854 11.0735 18.9265 10.6146 18.362 10.327C18.0057 10.1455 17.5883 10.0647 17 10.0288M7 10.0288V8C7 5.23858 9.23858 3 12 3C14.7614 3 17 5.23858 17 8V10.0288" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    chrome: `<svg viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="8.5" stroke="currentColor" stroke-width="1.8"/><circle cx="12" cy="12" r="3.2" stroke="currentColor" stroke-width="1.8"/><path d="M12 3.5h7.4M4.8 7.6l3.8 6.7M19.2 7.6 15.4 14.3" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    safari: `<svg viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="8.5" stroke="currentColor" stroke-width="1.8"/><path d="m14.9 8.2-1.7 5-5 1.7 1.7-5 5-1.7Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/></svg>`,
    dotsVertical: `<svg viewBox="0 0 24 24" fill="none"><circle cx="12" cy="5" r="1.7" fill="currentColor"/><circle cx="12" cy="12" r="1.7" fill="currentColor"/><circle cx="12" cy="19" r="1.7" fill="currentColor"/></svg>`,
    download: `<svg viewBox="0 0 24 24" fill="none"><path d="M12 4v10M8 10l4 4 4-4M5 16v2.5A1.5 1.5 0 0 0 6.5 20h11a1.5 1.5 0 0 0 1.5-1.5V16" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    appleOutline: `<svg viewBox="0 0 24 24" fill="none"><path d="M15.6 4.2c-.8.8-1.4 2-1.2 3.3 1.1.1 2.3-.6 3-1.5.7-.8 1.2-2 1.1-3.1-1.1 0-2.2.6-2.9 1.3Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/><path d="M17.5 12.2c0-2 1.6-3 1.7-3.1-.9-1.3-2.4-1.5-2.9-1.5-1.2-.1-2.4.7-3 .7-.7 0-1.7-.7-2.7-.7-1.4 0-2.7.8-3.4 2.1-1.4 2.4-.4 6.1 1 8 .7 1 1.5 2 2.5 2s1.5-.6 2.7-.6 1.6.6 2.7.6 1.9-1 2.5-2c.8-1.1 1.1-2.2 1.1-2.3-.1 0-2.2-.8-2.2-3.2Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/></svg>`,
    shareNodes: `<svg viewBox="0 0 24 24" fill="none"><circle cx="18" cy="5" r="2.4" stroke="currentColor" stroke-width="1.9"/><circle cx="6" cy="12" r="2.4" stroke="currentColor" stroke-width="1.9"/><circle cx="18" cy="19" r="2.4" stroke="currentColor" stroke-width="1.9"/><path d="m8.2 10.9 7.6-4.7M8.2 13.1l7.6 4.7" stroke="currentColor" stroke-width="1.9" stroke-linecap="round"/></svg>`,
    homeOutline: `<svg viewBox="0 0 24 24" fill="none"><path d="m4 11 8-7 8 7v8.5a1.5 1.5 0 0 1-1.5 1.5H15v-6H9v6H5.5A1.5 1.5 0 0 1 4 19.5V11Z" stroke="currentColor" stroke-width="1.9" stroke-linejoin="round"/></svg>`,
    chevronRight: `<svg viewBox="0 0 24 24" fill="none"><path d="m9 6 6 6-6 6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    menu: `<svg viewBox="0 0 24 24" fill="none"><path d="M3 6h18M3 12h18M3 18h18" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    back: `<svg viewBox="0 0 24 24" fill="none"><path d="M19 12H5M12 19l-7-7 7-7" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    refresh: `<svg viewBox="0 0 24 24" fill="none"><path d="M20 11a8 8 0 0 0-14.9-4M4 4v5h5M4 13a8 8 0 0 0 14.9 4M20 20v-5h-5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    close: `<svg viewBox="0 0 24 24" fill="none"><path d="m6 6 12 12M18 6 6 18" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    home: `<svg viewBox="0 0 24 24" fill="none"><path d="M4 10.5 12 4l8 6.5V20H4v-9.5Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/><path d="M9.5 20v-5h5v5" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/></svg>`,
    cart: `<svg viewBox="0 0 24 24" fill="none"><circle cx="9" cy="20" r="1.5" stroke="currentColor" stroke-width="1.6"/><circle cx="18" cy="20" r="1.5" stroke="currentColor" stroke-width="1.6"/><path d="M3 4h2l2.4 10.2a2 2 0 0 0 2 1.5h7.8a2 2 0 0 0 2-1.6L22 7H7" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    cartShopping: `<svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false"><path d="M6.29977 5H21L19 12H7.37671M20 16H8L6 3H3M9 20C9 20.5523 8.55228 21 8 21C7.44772 21 7 20.5523 7 20C7 19.4477 7.44772 19 8 19C8.55228 19 9 19.4477 9 20ZM20 20C20 20.5523 19.5523 21 19 21C18.4477 21 18 20.5523 18 20C18 19.4477 18.4477 19 19 19C19.5523 19 20 19.4477 20 20Z" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    chat: `<svg viewBox="0 0 24 24" fill="none"><path d="M21 15a2 2 0 0 1-2 2H8l-5 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/></svg>`,
    headphonesAlt: `<svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false"><path d="M21 18V12C21 7.02944 16.9706 3 12 3C7.02944 3 3 7.02944 3 12V18M6.75 21C6.05302 21 5.70453 21 5.41473 20.9424C4.22466 20.7056 3.29436 19.7753 3.05764 18.5853C3 18.2955 3 17.947 3 17.25V15.6C3 15.0399 3 14.7599 3.10899 14.546C3.20487 14.3578 3.35785 14.2049 3.54601 14.109C3.75992 14 4.03995 14 4.6 14H6.4C6.96005 14 7.24008 14 7.45399 14.109C7.64215 14.2049 7.79513 14.3578 7.89101 14.546C8 14.7599 8 15.0399 8 15.6V19.75C8 19.9823 8 20.0985 7.98079 20.1951C7.90188 20.5918 7.59178 20.9019 7.19509 20.9808C7.09849 21 6.98233 21 6.75 21ZM17.25 21C17.0177 21 16.9015 21 16.8049 20.9808C16.4082 20.9019 16.0981 20.5918 16.0192 20.1951C16 20.0985 16 19.9823 16 19.75V15.6C16 15.0399 16 14.7599 16.109 14.546C16.2049 14.3578 16.3578 14.2049 16.546 14.109C16.7599 14 17.0399 14 17.6 14H19.4C19.9601 14 20.2401 14 20.454 14.109C20.6422 14.2049 20.7951 14.3578 20.891 14.546C21 14.7599 21 15.0399 21 15.6V17.25C21 17.947 21 18.2955 20.9424 18.5853C20.7056 19.7753 19.7753 20.7056 18.5853 20.9424C18.2955 21 17.947 21 17.25 21Z" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    chartLine: `<svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false"><path d="M21 21H6.2C5.07989 21 4.51984 21 4.09202 20.782C3.71569 20.5903 3.40973 20.2843 3.21799 19.908C3 19.4802 3 18.9201 3 17.8V3M7 15L12 9L16 13L21 7" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    calendarDays: `<svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false"><path d="M3 9H21M7 3V5M17 3V5M6 12H8M11 12H13M16 12H18M6 15H8M11 15H13M16 15H18M6 18H8M11 18H13M16 18H18M6.2 21H17.8C18.9201 21 19.4802 21 19.908 20.782C20.2843 20.5903 20.5903 20.2843 20.782 19.908C21 19.4802 21 18.9201 21 17.8V8.2C21 7.07989 21 6.51984 20.782 6.09202C20.5903 5.71569 20.2843 5.40973 19.908 5.21799C19.4802 5 18.9201 5 17.8 5H6.2C5.0799 5 4.51984 5 4.09202 5.21799C3.71569 5.40973 3.40973 5.71569 3.21799 6.09202C3 6.51984 3 7.07989 3 8.2V17.8C3 18.9201 3 19.4802 3.21799 19.908C3.40973 20.2843 3.71569 20.5903 4.09202 20.782C4.51984 21 5.07989 21 6.2 21Z" stroke="#fff" stroke-width="2" stroke-linecap="round"/></svg>`,
    houseLine: `<svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false"><path d="M9 16.9999H15M3 14.5999V12.1301C3 10.9814 3 10.407 3.14805 9.87807C3.2792 9.40953 3.49473 8.96886 3.78405 8.57768C4.11067 8.13608 4.56404 7.78346 5.47078 7.07822L8.07078 5.056C9.47608 3.96298 10.1787 3.41648 10.9546 3.2064C11.6392 3.02104 12.3608 3.02104 13.0454 3.2064C13.8213 3.41648 14.5239 3.96299 15.9292 5.056L18.5292 7.07822C19.436 7.78346 19.8893 8.13608 20.2159 8.57768C20.5053 8.96886 20.7208 9.40953 20.8519 9.87807C21 10.407 21 10.9814 21 12.1301V14.5999C21 16.8401 21 17.9603 20.564 18.8159C20.1805 19.5685 19.5686 20.1805 18.816 20.564C17.9603 20.9999 16.8402 20.9999 14.6 20.9999H9.4C7.15979 20.9999 6.03969 20.9999 5.18404 20.564C4.43139 20.1805 3.81947 19.5685 3.43597 18.8159C3 17.9603 3 16.8401 3 14.5999Z" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    arrowDownSquare: `<svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false"><path d="M12 16V4M12 16L9 13M12 16L15 13M7 9H6.2C5.0799 9 4.51984 9 4.09202 9.21799C3.71569 9.40973 3.40973 9.71569 3.21799 10.092C3 10.5198 3 11.0799 3 12.2V16.8C3 17.9201 3 18.4802 3.21799 18.908C3.40973 19.2843 3.71569 19.5903 4.09202 19.782C4.51984 20 5.0799 20 6.2 20H17.8C18.9201 20 19.4802 20 19.908 19.782C20.2843 19.5903 20.5903 19.2843 20.782 18.908C21 18.4802 21 17.9201 21 16.8V12.2C21 11.0799 21 10.5198 20.782 10.092C20.5903 9.71569 20.2843 9.40973 19.908 9.21799C19.4802 9 18.9201 9 17.8 9H17" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    arrowUpRightSquare: `<svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false"><path d="M20 4L12 12M20 4V8.5M20 4H15.5M19 12.5V16.8C19 17.9201 19 18.4802 18.782 18.908C18.5903 19.2843 18.2843 19.5903 17.908 19.782C17.4802 20 16.9201 20 15.8 20H7.2C6.0799 20 5.51984 20 5.09202 19.782C4.71569 19.5903 4.40973 19.2843 4.21799 18.908C4 18.4802 4 17.9201 4 16.8V8.2C4 7.0799 4 6.51984 4.21799 6.09202C4.40973 5.71569 4.71569 5.40973 5.09202 5.21799C5.51984 5 6.07989 5 7.2 5H11.5" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    userAlt: `<svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false"><path d="M5 21C5 17.134 8.13401 14 12 14C15.866 14 19 17.134 19 21M16 7C16 9.20914 14.2091 11 12 11C9.79086 11 8 9.20914 8 7C8 4.79086 9.79086 3 12 3C14.2091 3 16 4.79086 16 7Z" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    userPen: `<svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false"><path d="M4 21C4 17.134 7.13401 14 11 14C11.3395 14 11.6734 14.0242 12 14.0709M15 7C15 9.20914 13.2091 11 11 11C8.79086 11 7 9.20914 7 7C7 4.79086 8.79086 3 11 3C13.2091 3 15 4.79086 15 7ZM12.5898 21L14.6148 20.595C14.7914 20.5597 14.8797 20.542 14.962 20.5097C15.0351 20.4811 15.1045 20.4439 15.1689 20.399C15.2414 20.3484 15.3051 20.2848 15.4324 20.1574L19.5898 16C20.1421 15.4477 20.1421 14.5523 19.5898 14C19.0376 13.4477 18.1421 13.4477 17.5898 14L13.4324 18.1574C13.3051 18.2848 13.2414 18.3484 13.1908 18.421C13.1459 18.4853 13.1088 18.5548 13.0801 18.6279C13.0478 18.7102 13.0302 18.7985 12.9948 18.975L12.5898 21Z" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    grid: `<svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false"><path d="M14 5.6C14 5.03995 14 4.75992 14.109 4.54601C14.2049 4.35785 14.3578 4.20487 14.546 4.10899C14.7599 4 15.0399 4 15.6 4H18.4C18.9601 4 19.2401 4 19.454 4.10899C19.6422 4.20487 19.7951 4.35785 19.891 4.54601C20 4.75992 20 5.03995 20 5.6V8.4C20 8.96005 20 9.24008 19.891 9.45399C19.7951 9.64215 19.6422 9.79513 19.454 9.89101C19.2401 10 18.9601 10 18.4 10H15.6C15.0399 10 14.7599 10 14.546 9.89101C14.3578 9.79513 14.2049 9.64215 14.109 9.45399C14 9.24008 14 8.96005 14 8.4V5.6Z" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/><path d="M4 5.6C4 5.03995 4 4.75992 4.10899 4.54601C4.20487 4.35785 4.35785 4.20487 4.54601 4.10899C4.75992 4 5.03995 4 5.6 4H8.4C8.96005 4 9.24008 4 9.45399 4.10899C9.64215 4.20487 9.79513 4.35785 9.89101 4.54601C10 4.75992 10 5.03995 10 5.6V8.4C10 8.96005 10 9.24008 9.89101 9.45399C9.79513 9.64215 9.64215 9.79513 9.45399 9.89101C9.24008 10 8.96005 10 8.4 10H5.6C5.03995 10 4.75992 10 4.54601 9.89101C4.35785 9.79513 4.20487 9.64215 4.10899 9.45399C4 9.24008 4 8.96005 4 8.4V5.6Z" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/><path d="M4 15.6C4 15.0399 4 14.7599 4.10899 14.546C4.20487 14.3578 4.35785 14.2049 4.54601 14.109C4.75992 14 5.03995 14 5.6 14H8.4C8.96005 14 9.24008 14 9.45399 14.109C9.64215 14.2049 9.79513 14.3578 9.89101 14.546C10 14.7599 10 15.0399 10 15.6V18.4C10 18.9601 10 19.2401 9.89101 19.454C9.79513 19.6422 9.64215 19.7951 9.45399 19.891C9.24008 20 8.96005 20 8.4 20H5.6C5.03995 20 4.75992 20 4.54601 19.891C4.35785 19.7951 4.20487 19.6422 4.10899 19.454C4 19.2401 4 18.9601 4 18.4V15.6Z" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/><path d="M14 15.6C14 15.0399 14 14.7599 14.109 14.546C14.2049 14.3578 14.3578 14.2049 14.546 14.109C14.7599 14 15.0399 14 15.6 14H18.4C18.9601 14 19.2401 14 19.454 14.109C19.6422 14.2049 19.7951 14.3578 19.891 14.546C20 14.7599 20 15.0399 20 15.6V18.4C20 18.9601 20 19.2401 19.891 19.454C19.7951 19.6422 19.6422 19.7951 19.454 19.891C19.2401 20 18.9601 20 18.4 20H15.6C15.0399 20 14.7599 20 14.546 19.891C14.3578 19.7951 14.2049 19.6422 14.109 19.454C14 19.2401 14 18.9601 14 18.4V15.6Z" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    profile: `<svg viewBox="0 0 24 24" fill="none"><circle cx="12" cy="7.5" r="4" stroke="currentColor" stroke-width="1.8"/><path d="M4.5 20a7.5 7.5 0 0 1 15 0" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    question: `<svg viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="9" stroke="currentColor" stroke-width="1.8"/><path d="M9.3 9.2a2.8 2.8 0 0 1 5.2 1.4c0 2-2.5 2.3-2.5 4" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/><circle cx="12" cy="17.1" r="1" fill="currentColor"/></svg>`,
    server: `<svg viewBox="0 0 24 24" fill="none"><rect x="3" y="4" width="18" height="6" rx="2" stroke="currentColor" stroke-width="1.8"/><rect x="3" y="14" width="18" height="6" rx="2" stroke="currentColor" stroke-width="1.8"/><circle cx="7" cy="7" r="1" fill="currentColor"/><circle cx="7" cy="17" r="1" fill="currentColor"/></svg>`,
    traffic: `<svg viewBox="0 0 24 24" fill="none"><rect x="4.5" y="5.5" width="15" height="13" rx="3" stroke="currentColor" stroke-width="1.8"/><path d="M8 10h8M8 14h5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/><circle cx="16" cy="14" r="1" fill="currentColor"/></svg>`,
    broadcast: `<svg viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="2" stroke="currentColor" stroke-width="1.8"/><path d="M16.2 7.8a6 6 0 0 1 0 8.4M7.8 16.2a6 6 0 0 1 0-8.4M19 5a10 10 0 0 1 0 14M5 19a10 10 0 0 1 0-14" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    youtube: `<svg viewBox="0 0 24 24" fill="none"><path d="M21 8.1a3.2 3.2 0 0 0-2.3-2.3c-2-.5-6.7-.5-6.7-.5s-4.7 0-6.7.5A3.2 3.2 0 0 0 3 8.1a33.3 33.3 0 0 0 0 7.8 3.2 3.2 0 0 0 2.3 2.3c2 .5 6.7.5 6.7.5s4.7 0 6.7-.5A3.2 3.2 0 0 0 21 15.9a33.3 33.3 0 0 0 0-7.8Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/><path d="m10 9.5 5 2.5-5 2.5v-5Z" fill="currentColor"/></svg>`,
    star: `<svg viewBox="0 0 24 24" fill="none"><path d="m12 3.8 2.5 5.1 5.6.8-4 3.9.9 5.5-5-2.6-5 2.6.9-5.5-4-3.9 5.6-.8L12 3.8Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/></svg>`,
    starFilled: `<svg viewBox="0 0 24 24" fill="none"><path d="m12 3.8 2.5 5.1 5.6.8-4 3.9.9 5.5-5-2.6-5 2.6.9-5.5-4-3.9 5.6-.8L12 3.8Z" fill="currentColor" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/></svg>`,
    doc: `<svg viewBox="0 0 24 24" fill="none"><path d="M7 3.5h7l4 4V20a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 6 20V5A1.5 1.5 0 0 1 7.5 3.5Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/><path d="M14 3.5V8h4" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/></svg>`,
    external: `<svg viewBox="0 0 24 24" fill="none"><path d="M14 5h5v5M10 14 19 5M19 13v5a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    users: `<svg viewBox="0 0 24 24" fill="none"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8ZM22 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    shield: `<svg viewBox="0 0 24 24" fill="none"><path d="M12 3.5 19 6v5.7c0 4.4-2.6 7.5-7 8.8-4.4-1.3-7-4.4-7-8.8V6l7-2.5Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/><path d="m9.5 12 1.8 1.8 3.5-3.8" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    plus: `<svg viewBox="0 0 24 24" fill="none"><path d="M12 5v14M5 12h14" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    ticketPlus: `<svg viewBox="0 0 256 256" fill="currentColor"><path d="M228 128a12 12 0 0 1-12 12H140v76a12 12 0 0 1-24 0V140H40a12 12 0 0 1 0-24h76V40a12 12 0 0 1 24 0v76h76a12 12 0 0 1 12 12Z"/></svg>`,
    faqCustom: `<svg viewBox="0 0 1280 1280" fill="currentColor"><path d="M5970 12090 c-417 -39 -761 -113 -1094 -236 -154 -56 -404 -178 -550 -267 -328 -202 -651 -502 -876 -817 -340 -475 -557 -1060 -660 -1783 -12 -81 -19 -151 -16 -156 3 -5 47 -34 96 -65 l91 -56 672 -80 c370 -44 685 -81 702 -83 l29 -2 28 150 c166 905 518 1459 1115 1759 267 133 553 196 900 196 485 0 899 -141 1253 -427 171 -139 357 -351 469 -537 238 -393 302 -907 165 -1326 -39 -120 -137 -307 -215 -410 -166 -221 -419 -479 -819 -835 -595 -531 -865 -813 -1076 -1125 -408 -602 -572 -1262 -547 -2203 l6 -247 691 2 691 3 2 105 c6 302 37 709 69 895 57 336 189 636 393 890 94 118 451 467 766 750 914 822 1312 1312 1528 1882 223 586 235 1285 32 1901 -150 453 -433 881 -816 1231 -544 498 -1190 775 -2044 876 -162 20 -828 29 -985 15z"/><path d="M5581 2590 c-20 -98 -21 -121 -21 -920 l0 -820 810 0 810 0 0 828 c-1 740 -3 836 -18 917 l-17 90 -771 3 -771 2 -22 -100z"/></svg>`,
    send: `<svg viewBox="0 0 24 24" fill="none"><path d="m21 3-9.5 18-1.8-7.7L2 11.5 21 3Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/><path d="M9.7 13.3 21 3" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    copy: `<svg viewBox="0 0 24 24" fill="none"><rect x="8" y="8" width="11" height="11" rx="2.5" stroke="currentColor" stroke-width="1.8"/><path d="M6 15H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v1" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    share: `<svg viewBox="0 0 24 24" fill="none"><path d="M8 12.5 16.5 8M8 11.5 16.5 16M6 13.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5Zm12 6a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5Zm0-11a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5Z" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    pencil: `<svg viewBox="0 0 24 24" fill="none"><path d="m4 20 4.5-1 9-9a2.1 2.1 0 0 0-3-3l-9 9L4 20Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/><path d="m13.5 7.5 3 3" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    trash: `<svg viewBox="0 0 24 24" fill="none"><path d="M4 7h16M9 7V5.5A1.5 1.5 0 0 1 10.5 4h3A1.5 1.5 0 0 1 15 5.5V7m-8 0 1 11a2 2 0 0 0 2 1.8h4a2 2 0 0 0 2-1.8L17 7" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    arrow: `<svg viewBox="0 0 24 24" fill="none"><path d="M7 17 17 7M9 7h8v8" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    chevron: `<svg viewBox="0 0 24 24" fill="none"><path d="m6 9 6 6 6-6" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    check: `<svg viewBox="0 0 24 24" fill="none"><path d="m5 12 4.5 4.5L19 7" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    calendar: `<svg viewBox="0 0 24 24" fill="none"><rect x="4" y="6" width="16" height="14" rx="3" stroke="currentColor" stroke-width="1.8"/><path d="M8 4v4M16 4v4M4 10h16" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    card: `<svg viewBox="0 0 24 24" fill="none"><rect x="3" y="5" width="18" height="14" rx="3" stroke="currentColor" stroke-width="1.8"/><path d="M3 10h18" stroke="currentColor" stroke-width="1.8"/></svg>`,
    stars: `<svg viewBox="0 0 24 24" fill="none"><path d="m12 3.5 2.6 5.4 6 .9-4.3 4.2 1 5.9L12 17l-5.3 2.9 1-5.9L3.4 9.8l6-.9L12 3.5Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/></svg>`,
    crypto: `<svg viewBox="0 0 24 24" fill="none"><path d="M12 3.5v17M8.5 7.5h5a3 3 0 0 1 0 6h-3a3 3 0 0 0 0 6h5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    wallet: `<svg viewBox="0 0 24 24" fill="none"><path d="M5 7h13a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2Z" stroke="currentColor" stroke-width="1.8"/><path d="M16 12h4" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/><path d="M5 7V6a2 2 0 0 1 2-2h10" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    windows: `<svg viewBox="0 0 24 24" fill="none"><path d="M3.5 5.2 10.5 4v7H3.5V5.2Zm9.2-1.4L20.5 2.5V11h-7.8V3.8ZM3.5 13h7v7L3.5 18.8V13Zm9.2 0h7.8v8.5l-7.8-1.3V13Z" fill="currentColor"/></svg>`,
    android: `<svg viewBox="0 0 24 24" fill="none"><path d="M9.1 6.8 7.8 4.9M14.9 6.8l1.3-1.9" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/><path d="M8 10.1a4 4 0 0 1 8 0V16a2.2 2.2 0 0 1-2.2 2.2h-3.6A2.2 2.2 0 0 1 8 16v-5.9Z" stroke="currentColor" stroke-width="1.9" stroke-linejoin="round"/><path d="M8.1 9.2h7.8" stroke="currentColor" stroke-width="1.9" stroke-linecap="round"/><path d="M10 12.2h.01M14 12.2h.01" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`,
    apple: `<svg viewBox="0 0 24 24" fill="none"><path d="M15.4 4.2c-.8 1-1.4 2.4-1.2 3.8 1.2.1 2.6-.7 3.3-1.7.8-.9 1.4-2.3 1.2-3.6-1.2 0-2.5.8-3.3 1.5Zm2.1 7.9c0-2.2 1.8-3.3 1.9-3.4-1-1.5-2.7-1.7-3.2-1.7-1.4-.2-2.7.8-3.4.8-.8 0-1.8-.8-3-.8-1.6 0-3 .9-3.8 2.3-1.6 2.8-.4 6.9 1.1 9.1.8 1.1 1.6 2.4 2.8 2.4s1.6-.7 3-.7c1.3 0 1.7.7 3 .7s2.1-1.2 2.8-2.3c.9-1.3 1.2-2.6 1.2-2.7-.1 0-2.4-.9-2.4-3.7Z" fill="currentColor"/></svg>`,
    mac: `<svg viewBox="0 0 24 24" fill="none"><rect x="3.5" y="4.5" width="17" height="11.5" rx="2.4" stroke="currentColor" stroke-width="1.8"/><path d="M9 19.5h6M10.2 16.2 9.4 19.5M13.8 16.2l.8 3.3" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    openTickets: `<svg viewBox="0 0 24 24" fill="none"><path d="M20 11.5c0 4.7-3.8 8.5-8.5 8.5-1.3 0-2.6-.3-3.7-.8L4 20l1-3.5A8.4 8.4 0 0 1 3 11.5C3 6.8 6.8 3 11.5 3S20 6.8 20 11.5Z" stroke="currentColor" stroke-width="1.9" stroke-linejoin="round"/></svg>`,
    historyTickets: `<svg viewBox="0 0 24 24" fill="none"><path d="M3 12a9 9 0 1 0 3-6.7" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/><path d="M3 4v4h4M12 7.5V12l3 1.8" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
  };
  return icons[name] || icons.arrow;
}

document.addEventListener("visibilitychange", refreshAfterPossibleGoogleLink);
window.addEventListener("focus", refreshAfterPossibleGoogleLink);

boot();
