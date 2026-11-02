const ASSETS = new URL("./assets/", import.meta.url).pathname;
const snowflake = `<svg viewBox="0 0 32 32" fill="none" aria-hidden="true"><g stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M16 2v28M2 16h28M6.1 6.1l19.8 19.8M25.9 6.1 6.1 25.9"/><path d="m12 5 4 4 4-4m-8 22 4-4 4 4M5 12l4 4-4 4m22-4-4 4 4 4M8 5l1 5-5-1m20-4-1 5 5-1M4 23l5-1-1 5m20-4-5-1 1 5" stroke-width="1"/></g><circle cx="16" cy="16" r="2" fill="currentColor"/></svg>`;

// Ornament geometry belongs to the control edge and scales with the control.
const frostTrim = `<svg viewBox="0 0 240 32" preserveAspectRatio="none" aria-hidden="true"><defs><linearGradient id="season-frost" x2="0" y2="1"><stop stop-color="#f8fdff"/><stop offset=".38" stop-color="#d8ecfb"/><stop offset=".8" stop-color="#83afcf"/><stop offset="1" stop-color="#557e9f" stop-opacity="0"/></linearGradient></defs><path d="M0 9c19-1 30-4 43-5 12 0 17 2 28 0 15-4 30-6 44-4 10 1 17 6 30 5 17-1 24-5 37-3 11 2 14 6 25 5 11-1 20-4 33-2v13c-14-2-20 0-30-1-13-2-21-5-32-3-18 3-32-1-43-1-11 1-18 4-32 2-15-2-25-3-38 1-11 3-23 0-34-2C19 12 9 14 0 15Z" fill="url(#season-frost)" opacity=".94"/><path d="M0 9c19-1 30-4 43-5 12 0 17 2 28 0 15-4 30-6 44-4 10 1 17 6 30 5 17-1 24-5 37-3 11 2 14 6 25 5 11-1 20-4 33-2" fill="none" stroke="#f5fcff" stroke-width="1.5" opacity=".9"/><g fill="#f3fbff"><circle cx="42" cy="17" r="1.1"/><circle cx="88" cy="19" r=".8"/><circle cx="112" cy="13" r="1"/><circle cx="157" cy="17" r=".9"/><circle cx="193" cy="20" r="1.1"/><circle cx="218" cy="16" r=".7"/></g></svg>`;
const webTrim = `<svg viewBox="0 0 112 112" aria-hidden="true"><defs><linearGradient id="season-web"><stop stop-color="#f6bd80" stop-opacity=".92"/><stop offset=".72" stop-color="#b8612d" stop-opacity=".6"/><stop offset="1" stop-color="#7a3f23" stop-opacity=".12"/></linearGradient></defs><g fill="none" stroke="url(#season-web)" stroke-width="1.8" stroke-linecap="round"><path d="M112 0 5 0M112 0 12 25M112 0 22 51M112 0 42 74M112 0 70 94M112 0 101 105M112 0 112 112"/><path d="M89 0 Q93 8 85 13 Q84 17 90 23 Q91 26 98 25 Q103 29 106 34 M67 0 Q70 14 63 18 Q59 28 70 37 Q73 43 81 42 Q94 45 99 60 M46 0 Q49 19 42 25 Q34 40 48 49 Q57 57 68 55 Q86 57 94 78 M23 0 Q25 25 19 32 Q9 53 29 62 Q41 73 55 71 Q76 75 86 95"/></g><path d="M112 0C79 2 54 9 39 22" fill="none" stroke="#ffb56a" stroke-opacity=".26" stroke-width="3"/></svg>`;
let trimSequence = 0;
function surfaceTrim() {
  const id = ++trimSequence;
  return `<span class="seasonal-surface__trim seasonal-surface__trim--frost" aria-hidden="true">${frostTrim.replaceAll("season-frost", `season-frost-${id}`)}</span><span class="seasonal-surface__trim seasonal-surface__trim--web" aria-hidden="true">${webTrim.replaceAll("season-web", `season-web-${id}`)}</span>`;
}
const surfaceSelector = [
  "#app #page-dashboard .subscription-switcher__trigger",
  "#app #page-dashboard .action-stack--dashboard .btn",
  "#app .bottom-nav:not(.bottom-nav--editor)",
  ".site-header .nav-shell", ".plans-grid .plan-card",
  ".hero__actions a",
].join(",");

export function decorateSeasonalSurfaces(root = document) {
  for (const element of root.querySelectorAll(surfaceSelector)) {
    if (element.dataset.seasonalSurface === "on") continue;
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
    <div class="seasonal-preview__switcher">Основная <span>⌄</span>${surfaceTrim()}<span class="seasonal-spider" aria-hidden="true"><svg viewBox="0 0 32 48" fill="none"><path d="M16 0v25" stroke="currentColor" stroke-width=".8"/><path d="m12 33-7-5-3 3m10 6-8 1-2 5m18-10 7-5 3 3m-10 6 8 1 2 5" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/><ellipse cx="16" cy="37" rx="5" ry="6" fill="currentColor"/><circle cx="16" cy="30" r="3" fill="currentColor"/></svg></span></div>
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
