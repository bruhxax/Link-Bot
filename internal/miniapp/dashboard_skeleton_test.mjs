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
  return { skeleton: () => context.renderDashboardSkeleton(), page: () => context.renderDashboardPage() };
}

const userData = (status, trial = { enabled: false, eligible: false }, user = {}) => ({
  subscription: { status }, trial, user, brand: { name: "Test" },
});
const details = html => [...html.matchAll(/data-detail="([^"]+)"/g)].map(match => match[1]);

test("unknown account loads only common elements without inventing subscription or trial", () => {
  const html = harness().skeleton();
  assert.deepEqual(details(html), ["logo", "username", "primary_action"]);
  assert.doesNotMatch(html, /data-block="subscription"|dashboard-skeleton__(plan|date|pill)/);
});

for (const [name, data] of [
  ["no subscription or trial", userData("none")],
  ["expired subscription and used trial", userData("expired", { enabled: true, eligible: false })],
  ["eligible trial", userData("none", { enabled: true, eligible: true })],
  ["email user needs Telegram link for trial", userData("none", { enabled: true, eligible: false }, { email: "test@example.com", telegramLinked: false })],
  ["active subscription", userData("active")],
]) {
  test(`skeleton matches actual dashboard elements: ${name}`, () => {
    const view = harness({ data });
    assert.deepEqual(details(view.skeleton()), details(view.page()));
    assert.equal(view.skeleton().includes('data-detail="plan_name"'), data.subscription.status === "active");
  });
}

test("fast bootstrap does not guess optional elements before subscription check", () => {
  const view = harness({ data: userData("active"), hydrating: true });
  assert.deepEqual(details(view.skeleton()), ["logo", "username", "primary_action"]);
  assert.equal(view.page().includes(view.skeleton()), true);
});

test("empty and hidden widgets have no loading placeholders; configured widgets remain", () => {
  const elements = [
    { id: "promo_widget", promoCode: "   " },
    { id: "notification_widget", notificationText: "", visible: true },
    { id: "banner_1", bannerUrl: "banner.png", visible: false },
    { id: "banner_2", bannerUrl: "" },
    { id: "banner_3", bannerUrl: "banner.png" },
    { id: "empty_card_1" },
  ];
  const view = harness({ data: userData("none"), elements });
  assert.doesNotMatch(view.skeleton(), /data-block="(?:promo_widget|notification_widget|banner_1|banner_2)"/);
  assert.match(view.skeleton(), /data-block="banner_3"/);
  assert.match(view.skeleton(), /empty-design-card/);
  elements[0].promoCode = "GIFT";
  elements[1].notificationText = "News";
  for (const html of [view.skeleton(), view.page()]) {
    assert.match(html, /data-block="promo_widget"/);
    assert.match(html, /data-block="notification_widget"/);
  }
});

test("hidden details and unavailable badges have no skeleton; disabled switcher stays absent", () => {
  const data = userData("active");
  data.subscription.trafficLimitBytes = "invalid";
  data.subscription.deviceLimitCount = "invalid";
  const view = harness({ data, elements: [{ id: "expires", visible: false }], features: { additional_subscriptions: false } });
  assert.deepEqual(details(view.skeleton()), details(view.page()));
  assert.doesNotMatch(view.skeleton(), /data-detail="(?:expires|traffic|devices)"|data-block="subscription_switcher"/);
});

test("layout editor still shows configured editable widgets and switcher", () => {
  const view = harness({ data: userData("none"), editing: true, features: { additional_subscriptions: false }, elements: [{ id: "promo_widget" }, { id: "notification_widget" }] });
  for (const html of [view.skeleton(), view.page()]) {
    assert.match(html, /data-block="subscription_switcher"/);
    assert.match(html, /data-block="promo_widget"/);
    assert.match(html, /data-block="notification_widget"/);
  }
});
