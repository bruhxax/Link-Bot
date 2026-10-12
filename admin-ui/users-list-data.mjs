import { formatTraffic } from "./user-overview.mjs";

export const userStatuses = [
  { value: "active", label: "Активные" },
  { value: "expired", label: "Истекшие" },
  { value: "limited", label: "Трафик исчерпан" },
  { value: "disabled", label: "Отключенные" },
  { value: "blocked", label: "Заблокированные" },
  { value: "none", label: "Без подписки" },
];
export function userStatus(user) {
  return user.isBlocked
    ? "blocked"
    : String(user.subscriptionStatus || "none").toLowerCase();
}
export function userType(user) {
  if (userStatus(user) === "none") return "none";
  if (!user.trafficLoaded) return "unknown";
  return Number(user.trafficLimitBytes) > 0 ? "limited" : "unlimited";
}
export function userDateKey(value) {
  const date = new Date(value);
  return value && Number.isFinite(date.getTime())
    ? new Intl.DateTimeFormat("sv-SE", {
        timeZone: "Europe/Moscow",
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
      }).format(date)
    : "";
}
export function userInDateRange(value, range = []) {
  const [from, to] = range;
  if (!from && !to) return true;
  const key = userDateKey(value);
  return Boolean(key) && (!from || key >= from) && (!to || key <= to);
}
export function usersCSV(users) {
  const cell = (value) => {
    let text = String(value ?? "");
    if (/^[=+\-@\t\r]/.test(text)) text = "'" + text;
    return '"' + text.replaceAll('"', '""') + '"';
  };
  const header = [
    "Имя",
    "ID панели",
    "Telegram ID",
    "Статус",
    "Тариф",
    "Истекает",
    "Использовано",
    "Лимит",
    "За всё время",
    "Тег",
    "Описание",
    "Создан",
    "Последнее подключение",
  ];
  const rows = users.map((u) => [
    u.panelUsername || u.username || u.telegramId,
    u.panelId || u.customerId,
    u.telegramId,
    userStatus(u),
    u.planTitle || "",
    u.expiresAt || "",
    u.trafficLoaded ? formatTraffic(u.usedTrafficBytes) : "",
    u.trafficLoaded
      ? u.trafficLimitBytes > 0
        ? formatTraffic(u.trafficLimitBytes)
        : "Безлимит"
      : "",
    u.trafficLoaded ? formatTraffic(u.lifetimeUsedTrafficBytes) : "",
    u.tag || "",
    u.description || "",
    u.panelCreatedAt || u.createdAt || "",
    u.onlineAt || "",
  ]);
  return [header, ...rows].map((row) => row.map(cell).join(";")).join("\r\n");
}
