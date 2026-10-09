export function formatTraffic(bytes) {
  bytes = Math.max(0, Number(bytes) || 0);
  if (!bytes) return "0 B";
  const unit = Math.min(4, Math.floor(Math.log(bytes) / Math.log(1024)));
  return `${(bytes / 1024 ** unit).toFixed(unit ? 2 : 0)} ${["B", "KiB", "MiB", "GiB", "TiB"][unit]}`;
}

export function trafficOverview(user) {
  if (!user.trafficLoaded) return null;
  const limit = Math.max(0, Number(user.trafficLimitBytes) || 0);
  const used = Math.max(0, Number(user.usedTrafficBytes) || 0);
  const percent = limit ? (used * 100) / limit : 0;
  return {
    used: formatTraffic(used),
    limit: limit ? formatTraffic(limit) : "∞",
    lifetime: formatTraffic(user.lifetimeUsedTrafficBytes),
    percent,
    remaining: Math.max(0, 100 - percent),
    progress: limit ? Math.min(100, percent) : 100,
    strategy:
      {
        MONTH: "за месяц",
        MONTH_ROLLING: "за месяц ↻",
        WEEK: "за неделю",
        DAY: "за день",
        NO_RESET: "∞",
      }[user.trafficLimitStrategy] || "∞",
    color: percent > 95 ? "red" : percent > 80 ? "yellow" : "teal",
  };
}

export function relativeTime(value, now = Date.now()) {
  const date = Date.parse(value);
  if (!Number.isFinite(date)) return "—";
  const seconds = Math.round((date - now) / 1000);
  const [unit, divisor] =
    Math.abs(seconds) < 60
      ? ["second", 1]
      : Math.abs(seconds) < 3600
        ? ["minute", 60]
        : Math.abs(seconds) < 86400
          ? ["hour", 3600]
          : Math.abs(seconds) < 2592000
            ? ["day", 86400]
            : Math.abs(seconds) < 31536000
              ? ["month", 2592000]
              : ["year", 31536000];
  return new Intl.RelativeTimeFormat("ru", { numeric: "auto" }).format(
    Math.round(seconds / divisor),
    unit,
  );
}

export function expirationText(value, now = Date.now()) {
  const date = Date.parse(value);
  if (!Number.isFinite(date)) return "—";
  if (new Date(date).getUTCFullYear() >= 2099) return "∞";
  return `${date > now ? "Истекает" : "Истекла"} ${relativeTime(value, now)}`;
}
