export function reviewRewardDraft(rewards = {}) {
  return {
    days: rewards.days ?? 2, trafficGb: rewards.trafficGb ?? 20, balanceRub: rewards.balanceRub ?? 0,
    daysEnabled: Number(rewards.days ?? 2) > 0,
    trafficEnabled: Number(rewards.trafficGb ?? 20) > 0,
    balanceEnabled: Number(rewards.balanceRub ?? 0) > 0,
    promo: { enabled: false, rewardType: "discount", rewardValue: 0, rewardTrafficGb: 0, discountPercent: 10, expiryDays: 0, ...rewards.promo },
  };
}

function integer(value, min, max) {
  if (String(value).trim() === "") throw new Error("Заполните выбранные поля");
  const number = Number(value);
  if (!Number.isSafeInteger(number) || number < min || number > max) throw new Error(`Укажите целое число от ${min} до ${max}`);
  return number;
}

export function reviewRewardsFromDraft(draft) {
  const promo = { enabled: Boolean(draft.promo.enabled), rewardType: draft.promo.rewardType, rewardValue: 0, rewardTrafficGb: 0, discountPercent: 0, expiryDays: 0 };
  if (promo.enabled) {
    if (!["discount", "balance", "days", "traffic", "days_traffic"].includes(promo.rewardType)) throw new Error("Выберите награду промокода");
    promo.expiryDays = integer(draft.promo.expiryDays, 0, 3650);
    if (promo.rewardType === "discount") promo.discountPercent = integer(draft.promo.discountPercent, 1, 99);
    else promo.rewardValue = integer(draft.promo.rewardValue, 1, ["days", "days_traffic"].includes(promo.rewardType) ? 3650 : 1000000);
    if (promo.rewardType === "days_traffic") promo.rewardTrafficGb = integer(draft.promo.rewardTrafficGb, 1, 1000000);
  }
  return {
    days: draft.daysEnabled ? integer(draft.days, 1, 3650) : 0,
    trafficGb: draft.trafficEnabled ? integer(draft.trafficGb, 1, 1000000) : 0,
    balanceRub: draft.balanceEnabled ? integer(draft.balanceRub, 1, 1000000) : 0,
    promo,
  };
}

export function reviewRewardSummary(rewards = {}, locale = "ru") {
  const en = locale === "en", fa = locale === "fa", parts = [];
  if (rewards.days > 0) parts.push(`+${rewards.days} ${en ? "days" : fa ? "روز" : "дн."}`);
  if (rewards.trafficGb > 0) parts.push(`+${rewards.trafficGb} ${en ? "GB" : fa ? "گیگابایت" : "ГБ"}`);
  if (rewards.balanceRub > 0) parts.push(`+${rewards.balanceRub} ₽ ${en ? "to balance" : fa ? "به موجودی" : "на баланс"}`);
  if (rewards.promo?.enabled) {
    const p = rewards.promo;
    let bonus = p.rewardType === "discount" ? `−${p.discountPercent}%` : p.rewardType === "balance" ? `+${p.rewardValue} ₽` : p.rewardType === "traffic" ? `+${p.rewardValue} ${en ? "GB" : "ГБ"}` : `+${p.rewardValue} ${en ? "days" : "дн."}`;
    if (p.rewardType === "days_traffic") bonus += ` / +${p.rewardTrafficGb} ${en ? "GB" : "ГБ"}`;
    parts.push(`${en ? "personal code" : fa ? "کد شخصی" : "личный промокод"} (${bonus})`);
  }
  return parts.join(" · ");
}
