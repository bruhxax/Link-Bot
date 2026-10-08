// Visual tokens and proportions follow the Remnawave dashboard theme.
// All actions are handled by the existing authenticated application.
export const CONSOLE_GROUPS = [
  ["Управление", "Management", [
    ["users", "Пользователи", "Users", "users", "users"],
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
    ["appearance", "Оформление", "Appearance", "adminAppearance", "appearance"],
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

export function consoleRoute(id) {
  return CONSOLE_GROUPS.flatMap((group) => group[2]).find((route) => route[0] === id);
}

export function visibleConsoleGroups(can) {
  return CONSOLE_GROUPS.map(([ru,en,routes]) => [ru,en,routes.filter(route => can(route[4]) || route[0] === "broadcast" && can("smtp") || route[0] === "reviews" && can("reviews.rewards"))]).filter(group => group[2].length);
}
