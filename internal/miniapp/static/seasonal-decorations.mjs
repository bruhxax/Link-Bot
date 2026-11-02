const ASSETS = new URL("./assets/", import.meta.url).pathname;
const snowflake = `<svg viewBox="0 0 32 32" fill="none" aria-hidden="true"><g stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M16 2v28M2 16h28M6.1 6.1l19.8 19.8M25.9 6.1 6.1 25.9"/><path d="m12 5 4 4 4-4m-8 22 4-4 4 4M5 12l4 4-4 4m22-4-4 4 4 4M8 5l1 5-5-1m20-4-1 5 5-1M4 23l5-1-1 5m20-4-5-1 1 5" stroke-width="1"/></g><circle cx="16" cy="16" r="2" fill="currentColor"/></svg>`;

// Ornament geometry belongs to the control edge and scales with the control.
const webTrim = `<svg data-preserve-color viewBox="0 0 80 80" preserveAspectRatio="none" aria-hidden="true"><g fill="none" stroke="#d7833e" stroke-width="1" stroke-linecap="round" stroke-linejoin="round"><path d="M80 0 0 0M80 0 9 19M80 0 17 39M80 0 36 58M80 0 58 72M80 0 80 80" opacity=".74"/><path d="M61 0Q63 7 58 11 Q57 15 62 19 Q65 22 70 21 Q75 26 77 31 M43 0 Q46 14 39 17 Q34 28 46 35 Q51 40 60 39 Q68 43 73 52 M23 0 Q26 20 17 25 Q8 43 26 49 Q36 59 49 56 Q62 61 68 69" opacity=".9"/></g><path d="M80 0C58 1 38 5 24 15" fill="none" stroke="#f0994d" stroke-opacity=".32" stroke-width="2"/></svg>`;
// Small, distinct fractures begin at the edge and never cross the label.
const iceFractures = [
  ["M0 12 2 16 3.9 13 5.4 19 7.7 17 9.1 21", "M5.4 19 5.1 26 6.8 29 M7.7 17 8.4 10"],
  ["M100 10 98.1 14 96.3 12 94.6 18 92 16", "M96.3 12 95.8 7 M94.6 18 95.6 25 93.9 28"],
  ["M0 88 1.8 84 4 87 5.9 80 8.3 82", "M4 87 4.9 93 M5.9 80 5.5 74 7.1 72"],
  ["M100 86 97.9 82 96 85 94 78 91.9 80", "M96 85 96.4 92 M94 78 95.2 72 94 69"],
  ["M0 7 1.8 11 3.5 9 6.1 15 M100 91 98.3 87 96.3 90 94.7 84", "M3.5 9 4.1 4 M96.3 90 95.7 95"],
];
function iceCracks(variant) {
  const [main, branch] = iceFractures[variant];
  return `<svg class="seasonal-ice-cracks" data-preserve-color viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true"><path class="seasonal-ice-cracks__main" d="${main}"/><path class="seasonal-ice-cracks__branch" d="${branch}"/></svg>`;
}
function surfaceTrim({cracks = false, variant = 0} = {}) {
  return `<span class="seasonal-surface__trim seasonal-surface__trim--ice" aria-hidden="true">${cracks ? iceCracks(variant) : ""}</span><span class="seasonal-surface__trim seasonal-surface__trim--web" aria-hidden="true">${webTrim}</span>`;
}
const surfaceSelector = [
  "#app #page-dashboard .subscription-switcher__trigger",
  "#app #page-dashboard .action-stack--dashboard .btn",
  "#app .bottom-nav:not(.bottom-nav--editor)",
  ".site-header .nav-shell", ".plans-grid .plan-card",
  ".hero__actions a",
].join(",");
const winterSurfaceSelector = [
  ".page.active :is(.card:not(.card--status),.btn,.pricing-card,.menu-row,.profile-row,.pay-row,.platform-btn,.quick-link,.pay-selector,.support-ticket-card,.payments-switch,.promo-box,.metric,.server-summary-card,.server-card,.state-card,.faq-item,.referral-metrics,.referral-invite-card,.wallet-withdrawal,.wallet-history,.review-card,.login-method-card,.admin-finance-card,.admin-integrations__list,.admin-feature-group,.admin-editor__section,.admin-broadcast__section,.survey-editor)",
  ".modal.open :is(.modal__sheet,.card,.btn,.pricing-card,.pay-row,.platform-btn,.pay-selector)",
].join(",");
const iceControlSelector = ".btn,.platform-btn,.quick-link,.pay-selector,.payments-switch,.subscription-switcher__trigger,.bottom-nav,.nav-shell,.hero__actions a,.menu-row,.pay-row,.profile-row,.menu-card,.support-ticket-card,.faq-item";
const iceCrackSelector = ".btn,.platform-btn,.quick-link,.pay-selector,.subscription-switcher__trigger,.bottom-nav,.nav-shell,.hero__actions a";

export function decorateSeasonalSurfaces(root = document) {
  const selector = document.documentElement.dataset.season === "winter"
    ? `${surfaceSelector},${winterSurfaceSelector}`
    : surfaceSelector;
  let crackIndex = 0;
  for (const element of root.querySelectorAll(selector)) {
    if (element.dataset.seasonalSurface === "on") continue;
    if (element.matches(surfaceSelector)) element.dataset.seasonalWeb = "on";
    const actionStack = element.closest(".action-stack--dashboard");
    if (actionStack) element.dataset.seasonalSlot = actionStack.querySelector(".btn") === element ? "first" : "second";
    element.dataset.seasonalSurface = "on";
    if (document.documentElement.dataset.season === "winter" && getComputedStyle(element).position === "static") element.dataset.seasonalPositioned = "on";
    if (element.matches(iceControlSelector)) element.dataset.seasonalKind = "control";
    const cracks = element.matches(iceCrackSelector);
    element.insertAdjacentHTML("beforeend", surfaceTrim({cracks, variant: cracks ? crackIndex++ % iceFractures.length : 0}));
  }
}

function normalize(settings = {}) {
  const mode = ["winter", "halloween"].includes(settings.mode) ? settings.mode : "off";
  return {
    mode,
    intensity: Math.min(100, Math.max(10, Number(settings.intensity) || 55)),
    animated: settings.animated !== false,
    winterDrifts: settings.winterDrifts !== false,
    winterSnow: settings.winterSnow !== false,
    halloweenWebs: settings.halloweenWebs !== false,
    halloweenSpider: settings.halloweenSpider !== false,
    halloweenLeaves: settings.halloweenLeaves !== false,
  };
}

function scene(settings) {
  if (settings.mode === "winter") {
    const flakes = settings.winterSnow ? Array.from({length: 10}, (_, i) => {
      const left = (i * 67 + 11) % 100;
      const delay = -(i * 2.3 % 22).toFixed(1);
      const duration = 19 + i % 4 * 4;
      const size = 7 + i % 4 * 3;
      return `<span class="seasonal-flake" style="--x:${left}%;--still-y:${(i*37+17)%87}%;--delay:${delay}s;--duration:${duration}s;--size:${size}px">${snowflake}</span>`;
    }).join("") : "";
    return `<span class="seasonal-wash seasonal-wash--winter"></span>${flakes}`;
  }
  if (settings.mode === "halloween") {
    const leaves = settings.halloweenLeaves ? Array.from({length: 5}, (_, i) => `<img class="seasonal-leaf" src="${ASSETS}season-leaf.svg" alt="" style="--x:${(i*83+9)%100}%;--still-y:${(i*47+8)%88}%;--delay:${-(i*2.9).toFixed(1)}s;--duration:${20+i%4*5}s;--size:${13+i%3*5}px">`).join("") : "";
    return `<span class="seasonal-wash seasonal-wash--halloween"></span>${leaves}`;
  }
  return "";
}

export function renderSeasonalPreview(raw) {
  const settings = normalize(raw);
  return `<div class="seasonal-preview" data-season="${settings.mode}" data-season-drifts="${settings.winterDrifts ? "on" : "off"}" data-season-webs="${settings.halloweenWebs ? "on" : "off"}" data-season-spider="${settings.halloweenSpider ? "on" : "off"}" style="--seasonal-strength:${settings.intensity/100}">
    <div class="seasonal-preview__switcher">Основная <span>⌄</span>${surfaceTrim({cracks:true,variant:1})}<span class="seasonal-spider" aria-hidden="true"><svg data-preserve-color viewBox="0 0 32 48" fill="none"><path d="M16 0v25" stroke="currentColor" stroke-width=".8"/><path d="m12 33-7-5-3 3m10 6-8 1-2 5m18-10 7-5 3 3m-10 6 8 1 2 5" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/><ellipse cx="16" cy="37" rx="5" ry="6" fill="currentColor"/><circle cx="16" cy="30" r="3" fill="currentColor"/></svg></span></div>
    <div class="seasonal-preview__details"><strong>Подписка</strong><small>5 устройств · 100 ГБ</small></div>
    <div class="seasonal-preview__button">Продлить${surfaceTrim({cracks:true,variant:0})}</div><div class="seasonal-preview__button">Подключиться${surfaceTrim({cracks:true,variant:3})}</div>
    <div class="seasonal-preview__nav"><span>⌂</span><span>▣</span><span>✉</span><span>♙</span><span>▦</span>${surfaceTrim({cracks:true,variant:2})}</div>
    <div class="seasonal-decor seasonal-decor--preview ${settings.animated ? "" : "seasonal-decor--still"}" style="--seasonal-opacity:${settings.intensity/100}" aria-hidden="true">${scene(settings)}</div>
  </div>`;
}

let lastKey = "";
export function syncSeasonalDecorations(raw, {enabled = true, surface = "cabinet"} = {}) {
  const settings = normalize(raw);
  const key = JSON.stringify([settings, enabled, surface]);
  if (key === lastKey && document.getElementById("seasonal-decorations")) return;
  lastKey = key;
  document.documentElement.dataset.season = enabled ? settings.mode : "off";
  document.documentElement.style.setProperty("--seasonal-strength", String(settings.intensity / 100));
  document.documentElement.dataset.seasonDrifts = enabled && settings.winterDrifts ? "on" : "off";
  document.documentElement.dataset.seasonWebs = enabled && settings.halloweenWebs ? "on" : "off";
  document.documentElement.dataset.seasonSpider = enabled && settings.halloweenSpider ? "on" : "off";
  document.getElementById("seasonal-decorations")?.remove();
  if (!enabled || settings.mode === "off") return;
  const host = document.createElement("div");
  host.id = "seasonal-decorations";
  host.className = `seasonal-decor ${settings.animated ? "" : "seasonal-decor--still"}`;
  host.dataset.surface = surface;
  host.dataset.season = settings.mode;
  host.style.setProperty("--seasonal-opacity", String(settings.intensity / 100));
  host.setAttribute("aria-hidden", "true");
  host.innerHTML = scene(settings);
  document.body.append(host);
}
