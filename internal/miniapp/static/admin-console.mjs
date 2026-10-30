// Visual tokens and proportions follow the Remnawave dashboard theme.
// All actions are handled by the existing authenticated application.
export function adminConsoleEnabled({ adminBaseURL, origin, previewMode = false, previewConsole = false }) {
  return Boolean(adminBaseURL && origin === adminBaseURL || previewMode && previewConsole);
}

export const CONSOLE_COLORS = Object.freeze({
  "--bg":"#0c0f13", "--bg-deep":"#0c0f13", "--surface":"#141619", "--surface-strong":"#1b1e24", "--surface-soft":"#181b21",
  "--text":"#e5e7eb", "--muted":"#979ba5", "--muted-2":"#777c87", "--line":"#292d35", "--line-soft":"#23272f",
  "--control-bg":"#20242b", "--control-border":"#323741", "--control-border-soft":"#292d35", "--button-text":"#e5e7eb", "--icon-color":"#c3c7cf",
  "--accent":"#acb8cc", "--accent-strong":"#acb8cc", "--accent-soft":"#acb8cc1f", "--accent-border":"#acb8cc4d", "--accent-contrast":"#151920",
  "--success":"#69b7a7", "--danger":"#d77f86", "--unlimited-badge-color":"#aab5c8", "--unlimited-badge-bg":"#aab5c82e", "--unlimited-badge-border":"#aab5c866",
});

export function applyConsoleAppearance(root) {
  Object.assign(root.dataset,{theme:"dark",glass:"off",frames:"on",background:"none"});
  for (const [name,value] of Object.entries(CONSOLE_COLORS)) root.style.setProperty(name,value);
}

// Navigate inside the current WebView. The Telegram SDK reads its signed launch
// data from the fragment after a cross-origin navigation; fragments never enter
// an HTTP request or server access log. Browser login tokens are not transferred.
export function workspaceNavigationURL(target, telegram) {
  const url = new URL(target);
  url.hash = "";
  if (String(telegram?.initData || "").trim()) {
    const launch = new URLSearchParams();
    launch.set("tgWebAppData", telegram.initData);
    if (telegram.version) launch.set("tgWebAppVersion", telegram.version);
    if (telegram.platform) launch.set("tgWebAppPlatform", telegram.platform);
    if (telegram.themeParams) launch.set("tgWebAppThemeParams", JSON.stringify(telegram.themeParams));
    url.hash = launch.toString();
  }
  return url.toString();
}

const CONSOLE_ROUTES = [
  ["Управление", "Management", [
    ["users", "Пользователи", "Users", "users", "users"],
    ["torrents", "Торренты", "Torrents", "adminTorrents", "torrents"],
    ["abuses", "Абузы", "Abuse alerts", "shield", "abuses"],
    ["subscriptions", "Подписки", "Subscriptions", "adminSubscriptions", "subscriptions"],
    ["plans", "Тарифы", "Plans", "cartShopping", "plans"],
    ["servers", "Ноды", "Nodes", "server", "servers.view", "page"],
    ["support", "Поддержка", "Support", "sms", "support.view", "page"],
    ["reviews", "Отзывы", "Reviews", "star", "reviews.delete", "page"],
    ["administrators", "Администраторы", "Administrators", "users", "administrators"],
  ]],
  ["Коммерция", "Commerce", [
    ["finance", "Финансы", "Finance", "chartLine", "finance"],
    ["analytics", "Аналитика", "Analytics", "chartLine", "analytics"],
    ["promocodes", "Промокоды", "Promo codes", "adminPromocodes", "promocodes"],
    ["referrals", "Рефералы и баланс", "Referrals & balance", "users", "referrals"],
    ["partners", "Партнёры", "Partners", "users", "partners"],
    ["broadcast", "Рассылки и почта", "Broadcasts & mail", "adminBroadcast", "broadcast"],
    ["integrations", "Интеграции", "Integrations", "adminIntegrations", "integrations"],
    ["moynalog", "Мой налог", "My Tax", "adminIntegrations", "moynalog"],
  ]],
  ["Интерфейс", "Interface", [
    ["content", "Редактор контента", "Content editor", "adminContent", "content"],
    ["appearance", "Оформление кабинета", "Cabinet appearance", "adminAppearance", "appearance"],


    ["layout", "Конструктор UI", "UI builder", "grid", "layout"],
    ["subpage", "Страница подписки", "Subscription page", "adminSubscriptions", "subpage"],
    ["localization", "Язык и шрифт", "Language & font", "language", "localization"],
  ]],
  ["Система", "System", [
    ["status", "Статус системы", "System status", "server", "status"],
    ["diagnostics", "Диагностика", "Diagnostics", "adminDiagnostics", "diagnostics"],
    ["features", "Функции", "Features", "adminFeatures", "features"],
    ["trial", "Пробный период", "Trial", "adminTrial", "trial"],
    ["grace", "Доступ после окончания", "Access after expiry", "adminTrial", "grace"],
    ["maintenance", "Режим аварии", "Maintenance", "adminMaintenance", "maintenance"],
    ["ai", "ИИ-помощник", "AI assistant", "sparkles", "ai"],
    ["push", "Push-уведомления", "Push notifications", "adminPush", "push"],
  ]],
];

const allConsoleRoutes = CONSOLE_ROUTES.flatMap(group=>group[2]);
export const CONSOLE_GROUPS = [
  ["Управление","Management",["users","torrents","abuses","partners","broadcast","promocodes","referrals","finance","analytics"]],
  ["Поддержка","Support",["support","reviews","content","ai"]],
  ["Настройки","Settings",["subscriptions","plans","servers","appearance","layout","subpage","integrations","moynalog","administrators","localization","status","diagnostics","features","trial","grace","maintenance","push"]],
].map(([ru,en,ids])=>[ru,en,ids.map(id=>allConsoleRoutes.find(route=>route[0]===id))]);

export function consoleRoute(id) {
  return CONSOLE_GROUPS.flatMap((group) => group[2]).find((route) => route[0] === id);
}

export function visibleConsoleGroups(can) {
  return CONSOLE_GROUPS.map(([ru,en,routes]) => [ru,en,routes.filter(route => can(route[4]) || route[0] === "broadcast" && can("smtp") || route[0] === "reviews" && can("reviews.rewards"))]).filter(group => group[2].length);
}
