(() => {
  "use strict";
  const path = window.location.pathname;
  if (path !== "/mini-app/" && path !== "/mini-app") return;
  if (String(window.Telegram?.WebApp?.initData || "").trim()) return;
  if (window.matchMedia?.("(display-mode: standalone)")?.matches || window.navigator.standalone === true) return;

  // Preserve all explicit cabinet, authentication, purchase-return and deep links.
  const params = new URLSearchParams(window.location.search);
  const cabinetSessionKey = "link-bot-web-cabinet-open";
  for (const key of params.keys()) {
    if (key !== "v" && !key.startsWith("utm_")) {
      try { window.sessionStorage.setItem(cabinetSessionKey, "1"); } catch {}
      return;
    }
  }
  try { if (window.sessionStorage.getItem(cabinetSessionKey) === "1") return; } catch {}
  window.location.replace("/");
})();
