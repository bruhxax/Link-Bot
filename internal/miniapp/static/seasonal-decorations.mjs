const ASSETS = new URL("./assets/", import.meta.url).pathname;
const snowflake = `<svg viewBox="0 0 32 32" fill="none" aria-hidden="true"><g stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M16 2v28M2 16h28M6.1 6.1l19.8 19.8M25.9 6.1 6.1 25.9"/><path d="m12 5 4 4 4-4m-8 22 4-4 4 4M5 12l4 4-4 4m22-4-4 4 4 4M8 5l1 5-5-1m20-4-1 5 5-1M4 23l5-1-1 5m20-4-5-1 1 5" stroke-width="1"/></g><circle cx="16" cy="16" r="2" fill="currentColor"/></svg>`;

function garland() {
  const colors = ["#f7bc6a", "#77dcff", "#f48caa", "#a6e7b9"];
  const loops = Array.from({length: 8}, (_, i) => {
    const x = 60 + i * 120;
    const color = colors[i % colors.length];
    const needles = Array.from({length: 8}, (_, n) => {
      const dx = (n - 3.5) * 4;
      return `<path d="M${x + dx} 42q${dx < 0 ? -10 : 10} -9 ${dx < 0 ? -15 : 15} -13" stroke="${n % 2 ? "#365d4c" : "#4a8064"}" stroke-width="2.4" stroke-linecap="round"/>`;
    }).join("");
    return `<g>${needles}<path d="M${x} 40v8" stroke="#443f3d" stroke-width="2"/><rect x="${x-5}" y="47" width="10" height="5" rx="2" fill="#b6a582"/><path d="M${x-8} 54c0-5 16-5 16 0v11c0 5-4 9-8 9s-8-4-8-9Z" fill="${color}" stroke="#fff" stroke-opacity=".65"/><path d="M${x-4} 54v9" stroke="#fff" stroke-opacity=".6" stroke-width="2" stroke-linecap="round"/><circle cx="${x}" cy="60" r="14" fill="${color}" opacity=".4" filter="url(#bulb-glow)"/></g>`;
  }).join("");
  const cable = Array.from({length: 8}, (_, i) => `Q ${60+i*120} 62 ${120+i*120} 19`).join(" ");
  return `<svg class="seasonal-garland" viewBox="0 0 960 86" preserveAspectRatio="xMidYMin slice" aria-hidden="true"><defs><filter id="bulb-glow"><feGaussianBlur stdDeviation="5"/></filter></defs><path d="M0 19 ${cable}" fill="none" stroke="#284234" stroke-width="3" stroke-linecap="round"/><path d="M0 17 ${cable}" fill="none" stroke="#98ad81" stroke-opacity=".45" stroke-width="1"/>${loops}</svg>`;
}

const GARLAND = garland();

function normalize(settings = {}) {
  const mode = ["winter", "halloween"].includes(settings.mode) ? settings.mode : "off";
  return {
    mode,
    intensity: Math.min(100, Math.max(10, Number(settings.intensity) || 55)),
    animated: settings.animated !== false,
    winterGarland: settings.winterGarland !== false,
    winterDrifts: settings.winterDrifts !== false,
    winterSnow: settings.winterSnow !== false,
    halloweenSkulls: settings.halloweenSkulls !== false,
    halloweenFog: settings.halloweenFog !== false,
    halloweenLeaves: settings.halloweenLeaves !== false,
  };
}

function scene(settings) {
  if (settings.mode === "winter") {
    const flakes = settings.winterSnow ? Array.from({length: 16}, (_, i) => {
      const left = (i * 61 + 13) % 100;
      const delay = -(i * 1.7 % 18).toFixed(1);
      const duration = 14 + i % 6 * 2;
      const size = 9 + i % 4 * 5;
      return `<span class="seasonal-flake" style="--x:${left}%;--still-y:${(i*37+17)%87}%;--delay:${delay}s;--duration:${duration}s;--size:${size}px">${snowflake}</span>`;
    }).join("") : "";
    return `${settings.winterGarland ? GARLAND : ""}${flakes}${settings.winterDrifts ? `<img class="seasonal-drift" src="${ASSETS}season-snowdrift.svg" alt="">` : ""}`;
  }
  if (settings.mode === "halloween") {
    const skulls = settings.halloweenSkulls ? Array.from({length: 5}, (_, i) => `<img class="seasonal-skull" src="${ASSETS}season-skull.svg" alt="" style="--x:${[8,86,23,76,47][i]}%;--y:${[18,27,63,74,43][i]}%;--size:${[42,35,30,46,24][i]}px;--delay:${-i*2}s">`).join("") : "";
    const leaves = settings.halloweenLeaves ? Array.from({length: 11}, (_, i) => `<img class="seasonal-leaf" src="${ASSETS}season-leaf.svg" alt="" style="--x:${(i*83+9)%100}%;--still-y:${(i*47+8)%88}%;--delay:${-(i*1.4).toFixed(1)}s;--duration:${12+i%5*3}s;--size:${14+i%4*6}px">`).join("") : "";
    return `${settings.halloweenFog ? `<span class="seasonal-fog seasonal-fog--one"></span><span class="seasonal-fog seasonal-fog--two"></span>` : ""}${skulls}${leaves}`;
  }
  return "";
}

export function renderSeasonalPreview(raw) {
  const settings = normalize(raw);
  const label = settings.mode === "winter" ? "Зимний вечер" : settings.mode === "halloween" ? "Тихая ночь" : "Без украшений";
  return `<div class="seasonal-preview" data-season="${settings.mode}"><div class="seasonal-preview__card"><span>ПРЕДПРОСМОТР</span><strong>${label}</strong><small>Украшения поверх вашей темы</small></div><div class="seasonal-decor seasonal-decor--preview ${settings.animated ? "" : "seasonal-decor--still"}" style="--seasonal-opacity:${settings.intensity/100}" aria-hidden="true">${scene(settings)}</div></div>`;
}

let lastKey = "";
export function syncSeasonalDecorations(raw, {enabled = true, surface = "cabinet"} = {}) {
  const settings = normalize(raw);
  const key = JSON.stringify([settings, enabled, surface]);
  if (key === lastKey && document.getElementById("seasonal-decorations")) return;
  lastKey = key;
  document.documentElement.dataset.season = enabled ? settings.mode : "off";
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
