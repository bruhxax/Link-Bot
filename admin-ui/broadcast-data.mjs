export const BROADCAST_BUTTON_TYPES = [
  { value: "url", label: "Ссылка" },
  { value: "promo", label: "Промокод" },
  { value: "main", label: "Главное меню" },
  { value: "reviews", label: "Отзывы" },
  { value: "referrals", label: "Реферальная система" },
  { value: "login_methods", label: "Способы входа" },
  { value: "support", label: "Поддержка" },
  { value: "devices", label: "Докупить устройства" },
];
export const BROADCAST_CATEGORIES = [
  { value: "all", label: "Все пользователи" },
  { value: "active", label: "Активные" },
  { value: "none", label: "Неактивные" },
  { value: "paid", label: "Покупали тариф" },
  { value: "expired", label: "Подписка истекла" },
  { value: "trial", label: "Активный триал" },
  { value: "trial_used", label: "Использовали триал" },
];
export const BROADCAST_STYLES = [
  { value: "", label: "Обычная" },
  { value: "primary", label: "Синяя" },
  { value: "success", label: "Зелёная" },
  { value: "danger", label: "Красная" },
];
export const BROADCAST_STATUS = {
  idle: "Нет сообщения",
  awaiting_message: "Ожидаем сообщение в боте",
  draft: "Черновик готов",
  running: "Рассылка идёт",
  finished: "Завершена",
  failed: "Ошибка отправки",
};
export function broadcastMetrics(draft) {
  const total = Math.max(0, Number(draft?.recipientCount) || 0),
    sent = Math.max(0, Number(draft?.sentCount) || 0),
    failed = Math.max(0, Number(draft?.failedCount) || 0);
  return {
    total,
    sent,
    failed,
    percent: total
      ? Math.min(100, Math.round(((sent + failed) / total) * 100))
      : 0,
    errorPercent: total ? Math.round((failed / total) * 1000) / 10 : 0,
  };
}
export function broadcastButtonText(text) {
  return String(text || "").replace(
    /<tg-emoji\b[^>]*>([\s\S]*?)<\/tg-emoji>/gi,
    "$1",
  );
}
