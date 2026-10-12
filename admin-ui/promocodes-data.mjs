export const initialPromoDraft = {
  type: "discount",
  unit: "days",
  code: "",
  discount: 20,
  balance: 100,
  days: 30,
  traffic: 10,
  combinedTraffic: 10,
  limit: "",
  expires: "",
};
export function promoCreatePayload(d, now = Date.now()) {
  const code = String(d.code || "")
    .trim()
    .toUpperCase()
    .replaceAll(" ", "");
  if (!/^[A-Z0-9_-]{3,32}$/.test(code))
    throw Error("Код: от 3 до 32 латинских букв, цифр, - или _");
  const rewardType = d.type === "subscription" ? d.unit : d.type;
  const key =
    rewardType === "discount"
      ? "discount"
      : rewardType === "balance"
        ? "balance"
        : rewardType === "traffic"
          ? "traffic"
          : "days";
  if (
    !["discount", "balance", "days", "traffic", "days_traffic"].includes(
      rewardType,
    )
  )
    throw Error("Выберите тип промокода");
  const amount = Number(d[key]),
    max =
      rewardType === "discount"
        ? 99
        : ["days", "days_traffic"].includes(rewardType)
          ? 3650
          : 1000000;
  if (!Number.isSafeInteger(amount) || amount < 1 || amount > max)
    throw Error(`Укажите целое значение бонуса от 1 до ${max}`);
  const combined = Number(d.combinedTraffic);
  if (
    rewardType === "days_traffic" &&
    (!Number.isSafeInteger(combined) || combined < 1 || combined > 1000000)
  )
    throw Error("Укажите целое количество ГБ от 1 до 1 000 000");
  const settings = promoSettingsPayload(d);
  if (settings.expiresAt && new Date(settings.expiresAt).getTime() <= now)
    throw Error("Дата окончания должна быть в будущем");
  return {
    code,
    rewardType,
    rewardValue: rewardType === "discount" ? 0 : amount,
    discountPercent: rewardType === "discount" ? amount : 0,
    rewardTrafficGb: rewardType === "days_traffic" ? combined : 0,
    ...settings,
  };
}
export function promoSettingsPayload(d) {
  const limit = Number(d.limit || 0);
  if (!Number.isSafeInteger(limit) || limit < 0 || limit > 2147483647)
    throw Error("Укажите целый лимит пользователей или оставьте поле пустым");
  const date = d.expires ? new Date(d.expires) : null;
  if (date && Number.isNaN(date.getTime()))
    throw Error("Проверьте дату окончания");
  return { maxRedemptions: limit, expiresAt: date ? date.toISOString() : "" };
}
export function promoStatus(item, now = Date.now()) {
  if (item.isActive === false || item.status === "inactive") return "inactive";
  if (item.expiresAt && new Date(item.expiresAt).getTime() <= now)
    return "expired";
  if (item.maxRedemptions > 0 && item.redemptionCount >= item.maxRedemptions)
    return "exhausted";
  return ["expired", "exhausted"].includes(item.status)
    ? item.status
    : "active";
}
export function promoReward(item) {
  const n = (v) => Number(v || 0).toLocaleString("ru");
  switch (item.rewardType) {
    case "balance":
      return `+${n(item.rewardValue)} ₽ на баланс`;
    case "days":
      return `+${n(item.rewardValue)} дн. к подписке`;
    case "traffic":
      return `+${n(item.rewardValue)} ГБ`;
    case "days_traffic":
      return `+${n(item.rewardValue)} дн. · +${n(item.rewardTrafficGb)} ГБ`;
    default:
      return `−${n(item.discountPercent)}%`;
  }
}
export function promoRows(items, query = "") {
  const needle = String(query).trim().toUpperCase();
  return (items || [])
    .filter((i) => String(i.code).toUpperCase().includes(needle))
    .slice()
    .sort(
      (a, b) =>
        (new Date(b.createdAt).getTime() || 0) -
          (new Date(a.createdAt).getTime() || 0) || b.id - a.id,
    );
}
export function promoLocalDate(value) {
  if (!value) return "";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
