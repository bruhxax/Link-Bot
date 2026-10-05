import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const dashboardSource = source.slice(source.indexOf("function getDashboardSecondaryAction()"), source.indexOf("function renderPromoGiftWidget("));

function harness({ data = null, hydrating = false, elements = [], features = {}, editing = false } = {}) {
  const context = vm.createContext({
    state: { data, dashboardHydrating: hydrating, adminLayoutEditing: editing, currentPage: "dashboard", locale: "ru" },
    subscriptionSwitchAnimation: "",
    getRuntimeSettings: () => ({ layout: {} }),
    featureEnabled: name => features[name] !== false,
    getLayoutElements: () => elements.filter(item => item.visible !== false),
    getLayoutElement: (area, id) => elements.find(item => item.id === id),
    isSubscriptionActive: () => data?.subscription?.status === "active",
    renderLayoutDetail: (area, id, content) => elements.find(item => item.id === id)?.visible === false ? "" : `<div data-detail="${id}">${content}</div>`,
    renderRuntimeLayoutArea: (area, blocks) => Object.entries(blocks).filter(([id]) => elements.find(item => item.id === id)?.visible !== false).map(([id, content]) => `<div data-block="${id}">${content}</div>`).join(""),
    isBannerLayoutID: id => /^banner_[1-9]\d*$/.test(id),
    isEmptyLayoutCardID: id => /^empty_card_\d+$/.test(id),
    localizedText: ru => ru,
    escapeAttribute: String,
    escapeHtml: String,
    formatTrafficBadgeLabel: (used, limit) => Number.isFinite(Number(limit || 0)) ? "traffic" : "",
    formatDeviceBadgeLabel: (used, limit) => Number.isFinite(Number(limit || 0)) ? "devices" : "",
    t: () => ({}),
    getCurrentSubscriptionPlanLabel: () => "Plan",
    getUntilLabel: () => "Until",
    formatShortDateLabel: () => "Date",
    getDashboardUserLabel: () => "User",
    icon: () => "",
    emailAuthText: ru => ru,
    renderSubscriptionSwitcher: () => "switcher",
    resolveBrandMarkURL: () => "logo.svg",
    renderPromoGiftWidget: () => "promo",
    renderNotificationWidget: () => "notification",
    renderDashboardBanner: () => "banner",
    isWideCabinet: () => true,
    hasStoredLayoutPosition: () => false,
    pageClass: () => "active",
  });
  vm.runInContext(dashboardSource, context);
  return { state: context.state, context, loading: () => context.renderDashboardLoading(), page: () => context.renderDashboardPage() };
}

const userData = (status, trial = { enabled: false, eligible: false }, user = {}) => ({
  subscription: { status }, trial, user, brand: { name: "Test" },
});
const details = html => [...html.matchAll(/data-detail="([^"]+)"/g)].map(match => match[1]);

test("unknown account has a neutral accessible indicator without guessing dashboard geometry", () => {
  const html = harness().loading();
  assert.deepEqual(details(html), []);
  assert.match(html, /role="status" aria-busy="true"/);
  assert.match(html, /Загружаем главную страницу/);
  assert.doesNotMatch(html, /data-block=|data-runtime-layout-key=|dashboard-skeleton|<button|<img/);
});

test("initial authenticated load mounts no fake dashboard, sidebar or guessed navigation", () => {
  const view = harness();
  view.state.loading = true;
  view.context.app = { innerHTML: "" };
  view.context.hasAuth = () => true;
  view.context.renderBottomNav = () => { throw new Error("navigation requires account roles"); };
  view.context.mountRuntimeLayout = () => { throw new Error("geometry requires real dashboard content"); };
  const start = source.indexOf("if (state.loading && !state.data) {");
  const end = source.indexOf("if (!state.data && (state.maintenance", start);
  assert.ok(start >= 0 && end > start);
  vm.runInContext(`function renderInitialDashboard() { ${source.slice(start, end)} }`, view.context);
  view.context.renderInitialDashboard();
  assert.match(view.context.app.innerHTML, /dashboard-loading-status/);
  assert.doesNotMatch(view.context.app.innerHTML, /bottom-nav|desktop-sidebar|data-runtime|dashboard-skeleton/);
  assert.equal(view.state.dashboardRevealPending, true);
});

for (const [name, data, secondaryExpected] of [
  ["no subscription or trial", userData("none"), false],
  ["expired subscription and used trial", userData("expired", { enabled: true, eligible: false }), false],
  ["eligible trial", userData("none", { enabled: true, eligible: true }), true],
  ["email user needs Telegram link for trial", userData("none", { enabled: true, eligible: false }, { email: "test@example.com", telegramLinked: false }), true],
  ["active subscription", userData("active"), true],
]) {
  test(`ready dashboard has only the account's actual elements: ${name}`, () => {
    const view = harness({ data });
    const html = view.page();
    assert.equal(html.includes('data-detail="plan_name"'), data.subscription.status === "active");
    assert.equal(html.includes('data-detail="secondary_action"'), secondaryExpected);
    assert.doesNotMatch(html, /dashboard-loading-status|dashboard-skeleton/);
  });
}

test("fast bootstrap does not guess optional elements before subscription check", () => {
  const view = harness({ data: userData("active"), hydrating: true });
  assert.deepEqual(details(view.page()), []);
  assert.equal(view.page().includes(view.loading()), true);
  view.state.dashboardHydrating = false;
  const html = view.page();
  assert.match(html, /data-detail="plan_name"/);
  assert.match(html, /data-detail="secondary_action"/);
  assert.match(html, /dashboard-reveal/);
  assert.doesNotMatch(html, /dashboard-loading-status/);
});

test("empty and hidden widgets stay absent in the ready dashboard; configured widgets remain", () => {
  const elements = [
    { id: "promo_widget", promoCode: "   " },
    { id: "notification_widget", notificationText: "", visible: true },
    { id: "banner_1", bannerUrl: "banner.png", visible: false },
    { id: "banner_2", bannerUrl: "" },
    { id: "banner_3", bannerUrl: "banner.png" },
    { id: "empty_card_1" },
  ];
  const view = harness({ data: userData("none"), elements });
  assert.doesNotMatch(view.page(), /data-block="(?:promo_widget|notification_widget|banner_1|banner_2)"/);
  assert.match(view.page(), /data-block="banner_3"/);
  assert.match(view.page(), /empty-design-card/);
  elements[0].promoCode = "GIFT";
  elements[1].notificationText = "News";
  const html = view.page();
  assert.match(html, /data-block="promo_widget"/);
  assert.match(html, /data-block="notification_widget"/);
});

test("hidden details, unavailable badges and disabled switcher stay absent", () => {
  const data = userData("active");
  data.subscription.trafficLimitBytes = "invalid";
  data.subscription.deviceLimitCount = "invalid";
  const view = harness({ data, elements: [{ id: "expires", visible: false }], features: { additional_subscriptions: false } });
  assert.doesNotMatch(view.page(), /data-detail="(?:expires|traffic|devices)"|data-block="subscription_switcher"/);
});

test("layout editor still shows configured editable widgets and switcher", () => {
  const view = harness({ data: userData("none"), editing: true, features: { additional_subscriptions: false }, elements: [{ id: "promo_widget" }, { id: "notification_widget" }] });
  const html = view.page();
  assert.match(html, /data-block="subscription_switcher"/);
  assert.match(html, /data-block="promo_widget"/);
  assert.match(html, /data-block="notification_widget"/);
});
