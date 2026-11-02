const ASSETS = new URL("./assets/", import.meta.url).pathname;
const snowflake = `<svg viewBox="0 0 32 32" fill="none" aria-hidden="true"><g stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M16 2v28M2 16h28M6.1 6.1l19.8 19.8M25.9 6.1 6.1 25.9"/><path d="m12 5 4 4 4-4m-8 22 4-4 4 4M5 12l4 4-4 4m22-4-4 4 4 4M8 5l1 5-5-1m20-4-1 5 5-1M4 23l5-1-1 5m20-4-5-1 1 5" stroke-width="1"/></g><circle cx="16" cy="16" r="2" fill="currentColor"/></svg>`;

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
    const leaves = settings.halloweenLeaves ? Array.from({length: 7}, (_, i) => `<img class="seasonal-leaf" src="${ASSETS}season-leaf.svg" alt="" style="--x:${(i*83+9)%100}%;--still-y:${(i*47+8)%88}%;--delay:${-(i*2.9).toFixed(1)}s;--duration:${20+i%4*5}s;--size:${13+i%3*5}px">`).join("") : "";
    return `<span class="seasonal-wash seasonal-wash--halloween"></span>${leaves}`;
  }
  return "";
}

export function renderSeasonalPreview(raw) {
  const settings = normalize(raw);
  return `<div class="seasonal-preview" data-season="${settings.mode}" data-season-drifts="${settings.winterDrifts ? "on" : "off"}" data-season-webs="${settings.halloweenWebs ? "on" : "off"}" data-season-spider="${settings.halloweenSpider ? "on" : "off"}" style="--seasonal-strength:${settings.intensity/100}">
    <div class="seasonal-preview__switcher">Основная <span>⌄</span><span class="seasonal-spider" aria-hidden="true"><svg viewBox="0 0 32 48" fill="none"><path d="M16 0v25" stroke="currentColor" stroke-width=".8"/><path d="m12 33-7-5-3 3m10 6-8 1-2 5m18-10 7-5 3 3m-10 6 8 1 2 5" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/><ellipse cx="16" cy="37" rx="5" ry="6" fill="currentColor"/><circle cx="16" cy="30" r="3" fill="currentColor"/></svg></span></div>
    <div class="seasonal-preview__details"><strong>Подписка</strong><small>5 устройств · 100 ГБ</small></div>
    <div class="seasonal-preview__button">Продлить</div><div class="seasonal-preview__button">Подключиться</div>
    <div class="seasonal-preview__nav"><span>⌂</span><span>▣</span><span>✉</span><span>♙</span><span>▦</span></div>
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
