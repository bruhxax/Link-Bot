import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const cardSource = source.slice(source.indexOf("function renderPlanCard("), source.indexOf("function renderPaymentHistoryItem("));
const context = vm.createContext({
  state: { locale: "ru", adminPlanEditing: false },
  planKey: (plan) => plan.id,
  getPlanDetails: () => ({ traffic: "100 ГБ", devices: "1 устройство" }),
  planHasUnlimitedTraffic: () => false,
  getPlanBaseTitle: () => "Месяц",
  planHasCustomTitle: () => false,
  renderDeviceTrafficNote: () => "",
  formatCurrency: (rub) => `${rub} ₽`,
  localizedText: (ru) => ru,
  escapeHtml: (value) => String(value),
  escapeAttribute: (value) => String(value),
  Date,
  Number,
});
vm.runInContext(cardSource, context);

test("plan card shows only the current and crossed-out original prices", () => {
  const card = context.renderPlanCard({ id: "month", months: 1, priceRub: 67, originalPriceRub: 100 }, false);
  assert.match(card, /<strong[^>]*>67 ₽<\/strong><del>100 ₽<\/del>/);
  assert.doesNotMatch(card, /Экономия|Save|صرفه‌جویی/);
});

test("expired discount does not show a crossed-out price", () => {
  const card = context.renderPlanCard({ id: "month", months: 1, priceRub: 67, originalPriceRub: 100, discountEndsAt: "2020-01-01T00:00:00Z" }, false);
  assert.match(card, /<strong[^>]*>100 ₽<\/strong>/);
  assert.doesNotMatch(card, /<del>/);
});
