(() => {
  const rawID = document.querySelector('meta[name="yandex-metrika-counter-id"]')?.content?.trim();
  if (!/^[1-9][0-9]{0,11}$/.test(rawID || "")) return;
  const id = Number(rawID);
  const pages = new Set(["dashboard", "buy", "gift", "setup", "support", "faq", "reviews", "referrals", "partner", "servers", "settings", "media", "login-methods", "payments", "terms", "privacy", "custom-page", "admin"]);
  const cleanURL = `${location.origin}${location.pathname}`;
  let referrer = "";
  try {
    const previous = new URL(document.referrer);
    if (previous.protocol === "https:" || previous.protocol === "http:") referrer = `${previous.origin}${previous.pathname}`;
  } catch { /* Direct visit. */ }
  window.ym = window.ym || function () { (window.ym.a = window.ym.a || []).push(arguments); };
  window.ym.l = Date.now();
  // Send only explicit, sanitized pageviews; forms and session replay are excluded.
  window.ym(id, "init", { defer: true, accurateTrackBounce: true, clickmap: false, trackLinks: false, webvisor: false, sendTitle: false });
  let previousURL = "";
  function pageview(page) {
    const url = pages.has(page) ? `${cleanURL}#${page}` : cleanURL;
    if (url === previousURL) return;
    window.ym(id, "hit", url, { referer: previousURL || referrer, title: "Link-Bot" });
    previousURL = url;
  }
  const entry = new URLSearchParams(location.search).get("page");
  pageview(location.pathname.startsWith("/mini-app/") ? (pages.has(entry) ? entry : "dashboard") : "");
  window.addEventListener("miniapp:pageview", (event) => { if (pages.has(event.detail?.page)) pageview(event.detail.page); });
  const script = document.createElement("script");
  script.async = true;
  script.src = "https://mc.yandex.ru/metrika/tag.js";
  document.head.appendChild(script);
})();
