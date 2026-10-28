export const CUSTOMER_ROUTES = Object.freeze({
  dashboard: "dashboard", buy: "plans", gift: "gift", setup: "setup",
  support: "support", faq: "faq", reviews: "reviews", referrals: "referrals",
  partner: "partners", servers: "servers", settings: "profile", media: "media",
  "login-methods": "account/security", payments: "payments", terms: "terms", privacy: "privacy",
});
export const ADMIN_SECTIONS = Object.freeze([
  "torrents",
  "home", "users", "subscriptions", "plans", "servers", "support", "reviews", "administrators",
  "finance", "analytics", "promocodes", "referrals", "partners", "broadcast", "integrations", "moynalog",
  "content", "appearance", "landing", "cursors", "layout", "subpage", "localization", "status", "diagnostics", "features",
  "trial", "grace", "maintenance", "ai", "push", "smtp",
]);

export function readAppRoute({ pathname = "/", search = "", adminDedicated = false } = {}) {
  const query = new URLSearchParams(search);
  const path = pathname.replace(/^\/+|\/+$/g, "");
  const legacy = path === "mini-app" || !path;
  const adminPrefix = path === "admin" || path.startsWith("admin/");
  const cabinetPath = path.replace(/^cabinet(?:\/|$)/, "");
  if (path === "login" || path === "logout" || cabinetPath === "login" || cabinetPath === "logout") {
    return { page: "dashboard", section: "home", login: cabinetPath === "login", logout: cabinetPath === "logout" };
  }
  if (adminDedicated || adminPrefix || legacy && query.get("page") === "admin") {
    const section = legacy || path === "admin" ? query.get("section") || (path === "mini-app" ? "" : "home") : adminPrefix ? path.slice(6) : path || "home";
    return { page: "admin", section: !section || ADMIN_SECTIONS.includes(section) ? section : "home" };
  }
  if (cabinetPath.startsWith("pages/")) {
    try { return { page: "custom-page", section: "home", customPageID: decodeURIComponent(cabinetPath.slice(6)) }; } catch { return null; }
  }
  const page = Object.keys(CUSTOMER_ROUTES).find(key => CUSTOMER_ROUTES[key] === cabinetPath);
  if (page) return { page, section: "home" };
  if (legacy || path === "cabinet") return { page: query.get("page") || "", section: query.get("section") || "" };
  return null;
}

export function appRoutePath({ page = "dashboard", section = "home", customPageID = "", adminWorkspace = false, adminDedicated = false, cabinetDedicated = false } = {}) {
  if (page === "admin" || adminWorkspace) {
    const selected = page === "admin" ? section : page;
    const safeSection = ADMIN_SECTIONS.includes(selected) ? selected : "home";
    return (adminDedicated ? "" : "/admin") + (safeSection === "home" ? "/" : "/" + safeSection);
  }
  const route = page === "custom-page" && customPageID ? "pages/" + encodeURIComponent(customPageID) : CUSTOMER_ROUTES[page] || "dashboard";
  return (cabinetDedicated ? "" : "/cabinet") + "/" + route;
}

export function cabinetNavigationURL({ cabinetBaseURL = "", publicBaseURL = "", origin, page = "dashboard" }) {
  const base = cabinetBaseURL || publicBaseURL || origin;
  const prefix = cabinetBaseURL ? "" : "/cabinet";
  return new URL(prefix + "/" + (CUSTOMER_ROUTES[page] || page), base).href;
}

export function browserLogoutURL({ cabinetBaseURL = "", publicBaseURL = "", origin }) {
  const targetOrigin = new URL(cabinetBaseURL || publicBaseURL || origin).origin;
  // The cabinet must clear its own origin's stored credentials as well.
  return cabinetNavigationURL({ cabinetBaseURL, publicBaseURL, origin, page: targetOrigin === origin ? "login" : "logout" });
}
