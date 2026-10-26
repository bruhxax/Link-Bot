import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";
import vm from "node:vm";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
function load(context, name, async = false) {
  const start = source.indexOf(`${async ? "async " : ""}function ${name}(`);
  const rest = source.slice(start + 1);
  const next = /\n(?:async )?function /.exec(rest);
  assert.ok(start >= 0 && next, name);
  vm.runInContext(source.slice(start, start + 1 + next.index), context);
}

test("gift methods include an available balance and exclude currency-incompatible methods", () => {
  const context = vm.createContext({
    state: { currentPage: "gift" },
    getSelectedPlan: () => null,
    getSelectedDevicePack: () => ({ priceStars: 100 }),
    getSelectedTrafficPack: () => ({ priceStars: 100 }),
    getConfiguredPaymentMethods: () => [{ id: "balance" }, { id: "card" }, { id: "stars" }],
    paymentMethodBase: id => id,
  });
  load(context, "getAvailableMethods");
  assert.deepEqual(Array.from(context.getAvailableMethods({ priceRub: 199 }), m => m.id), ["balance", "card"]);
  assert.deepEqual(Array.from(context.getAvailableMethods({ priceRub: 0, priceStars: 50 }), m => m.id), ["card", "stars"]);
  context.getConfiguredPaymentMethods = () => [{ id: "card" }];
  assert.deepEqual(Array.from(context.getAvailableMethods({ priceRub: 199 }), m => m.id), ["card"], "a disabled/unavailable balance must never be synthesized");
});

test("gift balance summary uses rubles, cents and the same discount rounding as checkout", () => {
  let promo = null;
  const context = vm.createContext({
    state: { data: { wallet: { balanceCents: 14950 } } },
    getSelectedGiftPlan: () => ({ priceRub: 199, priceStars: 50 }),
    getSelectedPaymentMethod: () => ({ id: "balance" }),
    paymentMethodBase: id => id,
    getActivePromo: () => promo,
  });
  load(context, "giftBalancePayment");
  assert.equal(context.giftBalancePayment().shortfallRub, 49.5);
  assert.equal(context.giftBalancePayment().insufficient, true);
  promo = { discountPercent: 25 };
  assert.equal(context.giftBalancePayment().amountRub, 149);
  assert.equal(context.giftBalancePayment().availableRub, 149.5);
  assert.equal(context.giftBalancePayment().insufficient, false);
  context.state.data.wallet.balanceCents = 14900;
  assert.equal(context.giftBalancePayment().insufficient, false);
  context.state.data.wallet.balanceCents = 14899;
  assert.equal(context.giftBalancePayment().shortfallRub, 0.01);
  assert.equal(context.giftBalancePayment(undefined, { id: "card" }), null);
});

function checkout({ response = { action: "completed", purchaseId: 91 }, failure, insufficient = false } = {}) {
  const calls = [], toast = [];
  const context = vm.createContext({
    state: { giftBusy: "", giftUsernameDraft: "@friend_user", giftValidation: "", data: { user: { username: "sender_user" }, wallet: { balanceCents: 30000 } }, paymentChannelChoices: {}, appliedPromo: { code: "GIFT25" }, promoCodeDraft: "GIFT25" },
    t: () => ({ giftSentTitle: "Подарок отправлен", giftInvalidUsername: "Username invalid", giftSelf: "Self gift", paymentUnavailable: "Unavailable" }),
    getSelectedGiftPlan: () => ({ id: "gift-plan", months: 1 }),
    getSelectedPaymentMethod: () => ({ id: "balance" }),
    normalizeGiftUsername: value => String(value).replace(/^@/, "").toLowerCase(),
    giftBalancePayment: () => ({ insufficient }),
    localizedText: ru => ru,
    showToast: (...args) => toast.push(args),
    render: () => calls.push("render"),
    requirePaymentChannel: () => false, requirePaymentContact: () => false,
    selectedPaymentChannel: () => "", paymentReceiptEmail: () => "", paymentReturnTarget: "web",
    getActivePromo: () => context.state.appliedPromo,
    async post(url, body) { calls.push({ url, body }); if (failure) throw failure; return { data: response }; },
    async safeRefresh() { calls.push("refresh"); context.state.data.wallet.balanceCents = 15100; context.state.data.giftReceipt = { purchaseId: 91, recipientUsername: "@friend_user" }; },
    launchPaymentPage: () => { throw new Error("a balance purchase must never navigate to an empty payment URL"); },
  });
  load(context, "startGiftPayment", true);
  return { context, calls, toast };
}

test("completed balance gift refreshes balance and receipt without opening a payment URL", async () => {
  const page = checkout();
  await page.context.startGiftPayment();
  const request = page.calls.find(call => typeof call === "object");
  assert.equal(request.url, "/api/mini-app/gifts/purchase");
  assert.equal(request.body.paymentMethod, "balance");
  assert.equal(request.body.username, "friend_user");
  assert.equal(request.body.planId, "gift-plan");
  assert.equal(request.body.promoCode, "GIFT25");
  assert.equal(page.context.state.promoCodeDraft, "");
  assert.equal(page.context.state.appliedPromo, null);
  assert.equal(page.context.state.data.wallet.balanceCents, 15100);
  assert.equal(page.context.state.data.giftReceipt.purchaseId, 91);
  assert.equal(page.context.state.giftBusy, "");
  assert.deepEqual(page.toast, [["Подарок отправлен", "success"]]);
});

test("rapid double click sends one request while the gift purchase is in progress", async () => {
  const page = checkout();
  let finish;
  let requests = 0;
  page.context.post = () => { requests++; return new Promise(resolve => { finish = resolve; }); };
  const pending = page.context.startGiftPayment();
  await page.context.startGiftPayment();
  assert.equal(requests, 1);
  assert.equal(page.context.state.giftBusy, "balance");
  finish({ data: { action: "completed", purchaseId: 91 } });
  await pending;
  assert.equal(page.context.state.giftBusy, "");
});

test("insufficient balance, failed checkout and self gifts never show a success receipt", async () => {
  const short = checkout({ insufficient: true });
  await short.context.startGiftPayment();
  assert.equal(short.calls.length, 0);
  assert.match(short.toast[0][0], /Недостаточно/);
  const failure = Object.assign(new Error("Недостаточно средств на балансе"), { code: "insufficient_balance" });
  const page = checkout({ failure });
  await assert.rejects(page.context.startGiftPayment(), error => error === failure);
  assert.equal(page.context.state.giftBusy, "");
  assert.equal(page.context.state.promoCodeDraft, "GIFT25");
  assert.equal(page.context.state.giftUsernameDraft, "@friend_user");
  assert.equal(page.context.state.data.giftReceipt, undefined);
  assert.equal(page.toast.length, 0);
  const own = checkout();
  own.context.state.giftUsernameDraft = "@sender_user";
  await own.context.startGiftPayment();
  assert.equal(own.context.state.giftValidation, "Self gift");
  assert.ok(own.calls.every(call => typeof call === "string"));
});


test("typing/removing a discount refreshes gift affordability and pay label without replacing the username", () => {
  let discount = false;
  const hint = { textContent: "", classList: { toggle(name, value) { this.error = value; } } };
  const label = { textContent: "" };
  const button = { disabled: false, querySelector: () => label };
  const context = vm.createContext({
    state: { giftUsernameDraft: "@friend_user", giftBusy: "", data: { user: { username: "sender_user" } } },
    app: { querySelector: selector => selector === ".gift-balance-hint" ? hint : button },
    getSelectedGiftPlan: () => ({ id: "one-month" }), getSelectedPaymentMethod: () => ({ id: "balance" }),
    giftBalancePayment: () => ({ insufficient: !discount }),
    giftBalanceHint: balance => balance.insufficient ? "Missing 50 RUB" : "Balance 150 RUB",
    giftPaymentLabel: () => discount ? "Pay 149 RUB" : "Pay 199 RUB",
    normalizeGiftUsername: value => String(value || "").replace(/^@/, ""),
    updateAnimatedText: (node, text) => { node.textContent = text; },
  });
  load(context, "syncGiftBalancePaymentDOM");
  context.syncGiftBalancePaymentDOM();
  assert.equal(button.disabled, true);
  assert.equal(label.textContent, "Pay 199 RUB");
  discount = true;
  context.syncGiftBalancePaymentDOM();
  assert.equal(button.disabled, false);
  assert.equal(hint.classList.error, false);
  assert.equal(label.textContent, "Pay 149 RUB");
  discount = false;
  context.syncGiftBalancePaymentDOM();
  assert.equal(button.disabled, true);
  assert.equal(context.state.giftUsernameDraft, "@friend_user");
  context.state.giftBusy = "balance";
  discount = true;
  context.syncGiftBalancePaymentDOM();
  assert.equal(button.disabled, true);
});


test("gift balance hint preserves kopeks in available funds and shortfall", () => {
  const context = vm.createContext({
    state: { locale: "ru" },
    intlLocale: () => "ru-RU",
    localizedText: ru => ru,
  });
  load(context, "formatMoneyCents");
  load(context, "giftBalanceHint");
  assert.equal(context.giftBalanceHint({ availableRub: 124.5, shortfallRub: 74.5, insufficient: true }), "На балансе 124,50\u00a0₽ · Не хватает 74,50\u00a0₽");
  assert.equal(context.giftBalanceHint({ availableRub: 300, shortfallRub: 0, insufficient: false }), "На балансе 300\u00a0₽");
});
