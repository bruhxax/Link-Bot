const ASSETS = new URL("./assets/", import.meta.url).pathname;
const snowflake = `<svg viewBox="0 0 32 32" fill="none" aria-hidden="true"><g stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M16 2v28M2 16h28M6.1 6.1l19.8 19.8M25.9 6.1 6.1 25.9"/><path d="m12 5 4 4 4-4m-8 22 4-4 4 4M5 12l4 4-4 4m22-4-4 4 4 4M8 5l1 5-5-1m20-4-1 5 5-1M4 23l5-1-1 5m20-4-5-1 1 5" stroke-width="1"/></g><circle cx="16" cy="16" r="2" fill="currentColor"/></svg>`;

// Ornament geometry belongs to the control edge and scales with the control.
const webTrim = `<svg data-preserve-color viewBox="0 0 80 80" preserveAspectRatio="none" aria-hidden="true"><g fill="none" stroke="#d7833e" stroke-width="1" stroke-linecap="round" stroke-linejoin="round"><path d="M80 0 0 0M80 0 9 19M80 0 17 39M80 0 36 58M80 0 58 72M80 0 80 80" opacity=".74"/><path d="M61 0Q63 7 58 11 Q57 15 62 19 Q65 22 70 21 Q75 26 77 31 M43 0 Q46 14 39 17 Q34 28 46 35 Q51 40 60 39 Q68 43 73 52 M23 0 Q26 20 17 25 Q8 43 26 49 Q36 59 49 56 Q62 61 68 69" opacity=".9"/></g><path d="M80 0C58 1 38 5 24 15" fill="none" stroke="#f0994d" stroke-opacity=".32" stroke-width="2"/></svg>`;
// Fine fractures sit inside the ice surface; the control keeps its own border.
const iceCracks = `<svg class="seasonal-ice-cracks" data-preserve-color viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true"><g fill="none" stroke-linecap="round" stroke-linejoin="round" vector-effect="non-scaling-stroke"><g stroke="#173d5d" stroke-width="1.8" opacity=".42"><path d="M0 12 4 16 8 13 11 21 15 19 19 27 23 26M11 21 9 31 12 37M19 27 21 16 25 11M100 75 96 70 93 72 89 63 85 66 80 58 77 60M89 63 91 51 87 45M85 66 82 76 78 80M100 8 97 12 94 11 91 18 88 17"/></g><g stroke="#d7f3ff" stroke-width=".85" opacity=".74"><path d="M0 12 4 16 8 13 11 21 15 19 19 27 23 26M11 21 9 31 12 37M19 27 21 16 25 11M100 75 96 70 93 72 89 63 85 66 80 58 77 60M89 63 91 51 87 45M85 66 82 76 78 80M100 8 97 12 94 11 91 18 88 17"/></g><g stroke="#f1fbff" stroke-width=".5" opacity=".65"><path d="M4 16 2 24M15 19 15 10M96 70 98 60M80 58 76 49M94 11 95 24"/></g></g></svg>`;
function surfaceTrim() {
  return `<span class="seasonal-surface__trim seasonal-surface__trim--ice" aria-hidden="true">${iceCracks}</span><span class="seasonal-surface__trim seasonal-surface__trim--web" aria-hidden="true">${webTrim}</span>`;
}
const surfaceSelector = [
  "#app #page-dashboard .subscription-switcher__trigger",
  "#app #page-dashboard .action-stack--dashboard .btn",
  "#app .bottom-nav:not(.bottom-nav--editor)",
  ".site-header .nav-shell", ".plans-grid .plan-card",
  ".hero__actions a",
].join(",");

export function decorateSeasonalSurfaces(root = document) {
  const selector = document.documentElement.dataset.season === "winter"
    ? `${surfaceSelector},#app .card:not(.card--status),#app .btn`
    : surfaceSelector;
  for (const element of root.querySelectorAll(selector)) {
    if (element.dataset.seasonalSurface === "on") continue;
    if (element.matches(surfaceSelector)) element.dataset.seasonalWeb = "on";
    const actionStack = element.closest(".action-stack--dashboard");
    if (actionStack) element.dataset.seasonalSlot = actionStack.querySelector(".btn") === element ? "first" : "second";
    element.dataset.seasonalSurface = "on";
    element.insertAdjacentHTML("beforeend", surfaceTrim());
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
    <div class="seasonal-preview__switcher">Основная <span>⌄</span>${surfaceTrim()}<span class="seasonal-spider" aria-hidden="true"><svg data-preserve-color viewBox="0 0 32 48" fill="none"><path d="M16 0v25" stroke="currentColor" stroke-width=".8"/><path d="m12 33-7-5-3 3m10 6-8 1-2 5m18-10 7-5 3 3m-10 6 8 1 2 5" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/><ellipse cx="16" cy="37" rx="5" ry="6" fill="currentColor"/><circle cx="16" cy="30" r="3" fill="currentColor"/></svg></span></div>
    <div class="seasonal-preview__details"><strong>Подписка</strong><small>5 устройств · 100 ГБ</small></div>
    <div class="seasonal-preview__button">Продлить${surfaceTrim()}</div><div class="seasonal-preview__button">Подключиться${surfaceTrim()}</div>
    <div class="seasonal-preview__nav"><span>⌂</span><span>▣</span><span>✉</span><span>♙</span><span>▦</span>${surfaceTrim()}</div>
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
