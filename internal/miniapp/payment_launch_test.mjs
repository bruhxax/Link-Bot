import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const flows = source.slice(source.indexOf("async function startPayment("), source.indexOf("function closeGiftReceipt("));
const launch = source.slice(source.indexOf("function shouldLaunchPaymentInBrowser("), source.indexOf("function openPreparedPaymentInBrowser("));
const navigate = source.slice(source.indexOf("function navigateInMiniApp("), source.indexOf("function getPaymentReturnState("));
const browser = source.slice(source.indexOf("function openBrowserExternal("), source.indexOf("function isSupportSubscriptionLink("));
const link = "https://checkout.example/pay/order#token=checkout-token";

function harness({ platform = "tdesktop", ua = "Windows", navigatorPlatform = "Win32", touches = 0, method = "payhot", action = "open_in_app", devicePack = null, trafficPack = null } = {}) {
  const calls = [], renders = [], invoices = [], timers = [], browserOptions = [];
  const state = {
    appliedPromo: { code: "WORK" }, promoCodeDraft: "WORK", busyMethod: "", giftBusy: "", giftUsernameDraft: "recipient",
    data: { user: { username: "sender" } }, payModalOpen: true,
    paymentLaunchModalOpen: true, paymentLaunchURL: "old", paymentLaunchPurchaseId: 1,
  };
  const copy = { paymentOpened: "Payment opened", giftPaymentStarted: "Gift payment started", paymentUnavailable: "Unavailable", paymentSuccess: "Paid" };
  const plan = { id: "one-month", months: 1, priceRub: 89, priceStars: 20, trafficLimitBytes: 150 };
  const context = vm.createContext({
    state, navigator: { userAgent: ua, platform: navigatorPlatform, maxTouchPoints: touches },
    tg: { platform, openInvoice: (url, callback) => invoices.push({ url, callback }), openLink: (url, options) => { browserOptions.push(options); calls.push({ type: "external", url }); } },
    window: { location: { assign: url => calls.push({ type: "navigate", url }) } },
    t: () => copy, localizedText: ru => ru,
    getSelectedPlan: () => plan, getSelectedGiftPlan: () => plan,
    getSelectedDevicePack: () => devicePack, getSelectedTrafficPack: () => trafficPack,
    getSelectedPaymentMethod: () => ({ id: method }), getActivePromo: () => state.appliedPromo,
    paymentReceiptEmail: () => "", requirePaymentContact: () => false,
    normalizeGiftUsername: value => String(value).toLowerCase(), paymentReturnTarget: "telegram",
    render: options => renders.push(options), haptic: () => {},
    showToast: message => calls.push({ type: "toast", message }),
    openExternal: url => calls.push({ type: "external", url }),
    storePendingPayment: payload => calls.push({ type: "pending", ...payload }),
    async safeRefresh() { calls.push({ type: "refresh" }); },
    setTimeout: (callback, delay) => timers.push({ callback, delay }),
    async post(url, body) {
      calls.push({ type: "purchase", url, body });
      return { data: { action, url: link, purchaseId: 77 } };
    },
  });
  vm.runInContext(flows + "\n" + launch + "\n" + navigate + "\n" + browser, context);
  return { context, state, calls, renders, invoices, timers, browserOptions };
}

for (const flow of ["startPayment", "startGiftPayment"]) {
  for (const action of ["open_in_app", "open_link"]) {
    test(`${flow}: ${action} stays inside desktop Mini App for every hosted provider`, async () => {
      for (const method of ["payhot", "sbp", "card", "crypto", "lava", "wata", "platega", "freekassa", "heleket", "pally", "rollypay", "cispay", "anore", "mulenpay", "aurapay", "antilopay", "paritypay", "tribute", "cloudpayments", "datagio", "kassaai"]) {
        const h = harness({ method, action });
        await h.context[flow]();
        assert.deepEqual(h.calls.filter(call => call.type === "navigate"), [{ type: "navigate", url: link }], method);
        assert.equal(h.calls.some(call => call.type === "external"), false, method);
        assert.equal(h.calls.find(call => call.type === "pending").purchaseId, 77, method);
        assert.equal(h.state.payModalOpen, false);
        assert.equal(h.state.paymentLaunchModalOpen, false);
        assert.equal(h.state.busyMethod, "");
        assert.equal(h.state.giftBusy, "");
        assert.equal(h.renders.length, 1, "do not render an obsolete page after navigation");
      }
    });
    test(`${flow}: ${action} opens browser on phones and iPad with desktop user agent`, async () => {
      for (const device of [
        { platform: "android", ua: "Android" }, { platform: "ios", ua: "iPhone" },
        { platform: "web", ua: "iPhone" }, { platform: "unknown", ua: "Android" },
        { platform: "unknown", ua: "Macintosh", navigatorPlatform: "MacIntel", touches: 5 },
      ]) {
        const h = harness({ ...device, action });
        await h.context[flow]();
        assert.deepEqual(h.calls.filter(call => call.type === "external"), [{ type: "external", url: link }]);
        assert.equal(h.calls.some(call => call.type === "navigate"), false);
        assert.equal(h.browserOptions[0].try_browser, true, "request the browser on mobile Telegram");
        assert.equal(h.calls.find(call => call.type === "pending").purchaseId, 77);
        assert.equal(h.timers[0].delay, 4000);
        await h.timers[0].callback();
        assert.equal(h.calls.filter(call => call.type === "refresh").length, 1);
        assert.equal(h.renders.length, 2);
      }
    });
  }
}

test("native Telegram Stars checkout stays native for purchases and gifts", async () => {
  for (const flow of ["startPayment", "startGiftPayment"]) {
    const h = harness({ method: "stars", action: "open_invoice" });
    await h.context[flow]();
    assert.equal(h.invoices.length, 1);
    assert.equal(h.invoices[0].url, link);
    assert.equal(h.calls.some(call => ["external", "navigate"].includes(call.type)), false);
    await h.invoices[0].callback("paid");
    assert.equal(h.calls.filter(call => call.type === "refresh").length, 1);
  }
});

test("completed balance payments refresh the account without opening a checkout page", async () => {
  const h = harness({ method: "balance", action: "completed" });
  await h.context.startPayment();
  assert.equal(h.calls.some(call => ["external", "navigate", "pending"].includes(call.type)), false);
  assert.equal(h.calls.filter(call => call.type === "refresh").length, 1);
});

test("desktop browsers use the same tab on Windows, macOS and Linux", () => {
  for (const device of [{ platform: "", ua: "Windows" }, { platform: "macos", ua: "Macintosh", navigatorPlatform: "MacIntel" }, { platform: "web", ua: "Linux" }]) {
    const h = harness(device);
    assert.equal(h.context.launchPaymentPage(link, 77), true);
    assert.equal(h.calls.some(call => call.type === "external"), false);
  }
});

test("device and traffic pack purchases follow the same desktop/mobile rule", async () => {
  for (const mobile of [false, true]) {
    for (const type of ["device", "traffic"]) {
      const h = harness({ platform: mobile ? "android" : "tdesktop", devicePack: type === "device" ? { id: "extra-device", priceRub: 20 } : null, trafficPack: type === "traffic" ? { id: "extra-traffic", priceRub: 20 } : null });
      await h.context.startPayment({ deviceOnly: type === "device", trafficOnly: type === "traffic" });
      const purchase = h.calls.find(call => call.type === "purchase");
      assert.equal(purchase.body.paymentMethod, "payhot");
      assert.equal(purchase.body[type === "device" ? "devicePackId" : "trafficPackId"], `extra-${type}`);
      assert.equal(h.calls.some(call => call.type === "external"), mobile);
      assert.equal(h.calls.some(call => call.type === "navigate"), !mobile);
    }
  }
});
