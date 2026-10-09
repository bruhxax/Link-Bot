import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { buildFinanceProviders } from "./static/finance-providers.mjs";

const catalog = [
  ["yookassa", "YooKassa", "card"],
  ["tribute", "Tribute", "tribute"],
  ["telegram", "Telegram Stars", "stars"],
];

test("historical YooKassa invoice names populate one catalog card with combined totals", () => {
  const cards = buildFinanceProviders(
    [
      {
        key: "yookasa",
        currency: "RUB",
        revenue: 5383,
        refunds: 99,
        paymentCount: 25,
      },
      {
        key: " YOOKASSA ",
        currency: "rub",
        revenue: 100,
        refunds: 0,
        paymentCount: 1,
      },
    ],
    catalog,
  );
  assert.equal(cards.filter((card) => card.key === "yookassa").length, 1);
  assert.equal(cards[0].logo, "card");
  assert.equal(cards[0].paymentCount, 26);
  assert.deepEqual(cards[0].totals, [
    { currency: "RUB", revenue: 5483, refunds: 99 },
  ]);
});

test("provider aliases and unknown providers merge before rendering without mixing currencies", () => {
  const cards = buildFinanceProviders(
    [
      { key: "tribute", currency: "RUB", revenue: 50, paymentCount: 1 },
      { key: "tribute_shop", currency: "STARS", revenue: 70, paymentCount: 2 },
      { key: "CUSTOM", currency: "RUB", revenue: 20, paymentCount: 1 },
      { key: "custom", currency: "RUB", revenue: 30, paymentCount: 1 },
    ],
    catalog,
  );
  const tribute = cards.find((card) => card.key === "tribute");
  assert.equal(tribute.paymentCount, 3);
  assert.deepEqual(tribute.totals, [
    { currency: "RUB", revenue: 50, refunds: 0 },
    { currency: "STARS", revenue: 70, refunds: 0 },
  ]);
  assert.equal(cards.filter((card) => card.key === "custom").length, 1);
  assert.equal(
    cards.find((card) => card.key === "custom").totals[0].revenue,
    50,
  );
});

test("actual finance markup renders the stored yookasa statistics in its branded card", () => {
  const source = readFileSync(
    new URL("./static/app.js", import.meta.url),
    "utf8",
  );
  const snippet = source.slice(
    source.indexOf("const ADMIN_FINANCE_PROVIDERS ="),
    source.indexOf("function renderAdminSiteAnalytics("),
  );
  const context = vm.createContext({
    buildFinanceProviders,
    escapeHtml: String,
    escapeAttribute: String,
    icon: () => "<svg/>",
    PAYMENT_LOGO_URLS: { card: "yookassa.png" },
    formatFinanceAmount: (value, currency) => `${value} ${currency}`,
  });
  vm.runInContext(snippet, context);
  const html = context.renderAdminFinanceProviders([
    {
      key: "yookasa",
      name: "YooKassa",
      currency: "RUB",
      revenue: 5383,
      paymentCount: 25,
    },
  ]);
  assert.equal(html.match(/<strong>YooKassa<\/strong>/g).length, 1);
  const card = html.match(
    /<article[^>]*>.*?<strong>YooKassa<\/strong>.*?<\/article>/,
  )[0];
  assert.ok(card.includes("5383 RUB"));
  assert.ok(card.includes("25 оплат"));
  assert.ok(card.includes('src="yookassa.png"'));
});
