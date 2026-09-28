const SVG_NS = "http://www.w3.org/2000/svg";
const SURFACES = [
  ".card:not(.card--status)", ".pricing-card", ".menu-row", ".profile-row", ".pay-row",
  ".platform-btn", ".quick-link", ".pay-selector", ".menu-card", ".support-ticket-card",
  ".support-message__bubble", ".payments-switch", ".promo-box", ".metric", ".server-card", ".faq-item",
  ".server-summary-card", ".state-card", ".admin-integrations__list", ".review-detail__comment",
  ".media-empty-card", ".empty-design-card", ".admin-user-subscriptions__list",
  ".referral-metrics", ".referral-invite-card", ".wallet-withdrawal", ".wallet-history",
  ".admin-finance-card", ".admin-finance-history__surface", ".server-summary-panel",
  ".review-card", ".login-method-card", ".partner-application-card", ".admin-users__list",
  ".admin-partner-entry", ".admin-push__surface", ".modal__sheet", ".desktop-sidebar",
  ".subscription-switcher__menu", ".notification-popover", ".btn", ".sub-pill",
  ".header__btn", ".chip", ".tab", ".segment", ".tabs", ".bottom-nav",
  ".admin-save-bar__save", ".admin-save-bar__close", ".subscription-switcher__trigger",
  ".device-pack-trigger-row", ".setup-link-button", ".browser-auth__qr-button",
  ".admin-user-subscriptions__tab", ".admin-user-subscription", ".admin-user-controls__group--balance",
  ".admin-status__overview", ".admin-status__service", ".admin-status__section",
  ".admin-settings-search__field", ".admin-settings-search__results", ".admin-finance-provider",
  ".admin-ga4__metrics > div",
  ".admin-finance__more", ".admin-icon-button", ".admin-withdrawal__actions button",
  ".admin-banner-crop-controls button", ".admin-profile-modal__delete", ".admin-users__search", ".admin-users__more",
  ".admin-user-subscription__facts > span", ".admin-user-subscription__link",
  ".admin-user-balance-input input", ".admin-user-control-row input", ".admin-user-control select",
  ".admin-user-block-reason textarea", ".admin-user-block-delete", ".admin-user-subscription__link button",
  ".admin-user-subscription__activate", ".admin-user-balance-actions button", ".admin-user-control-row button",
  ".admin-user-controls__reissue button", ".admin-user-controls__delete button", ".admin-user-controls__danger button",
  ".admin-status__error", ".admin-status__header button", ".admin-status__error button", ".admin-settings-search__empty",
  ".nav-shell", ".plan-card", ".contact-card", ".empty-state", ".button",
].join(",");

const clamp = (value, min, max) => Math.max(min, Math.min(max, value));
const smoothstep = (value) => value * value * (3 - 2 * value);

// Inverse sampling: the bottom rim samples ABOVE itself, so the image stretches
// downwards. The gradient of a rounded rectangle gives diagonal corner normals.
export function glassSample(x, y, width, height, radius, rim) {
  const px = x - width / 2;
  const py = y - height / 2;
  const qx = Math.abs(px) - (width / 2 - radius);
  const qy = Math.abs(py) - (height / 2 - radius);
  const ox = Math.max(qx, 0);
  const oy = Math.max(qy, 0);
  const length = Math.hypot(ox, oy);
  const depth = -(length + Math.min(Math.max(qx, qy), 0) - radius);
  if (depth < 0 || depth >= rim) return { x: 0, y: 0, edge: 0 };
  const nx = (length ? ox / length : qx > qy ? 1 : 0) * Math.sign(px);
  const ny = (length ? oy / length : qx > qy ? 0 : 1) * Math.sign(py);
  const t = clamp(depth / rim, 0, 1);
  const bend = (1 - t) ** 2 * (0.93 + 0.07 * Math.sin(t * Math.PI));
  const edge = (1 - smoothstep(clamp((t - 0.35) / 0.65, 0, 1))) * clamp(depth + 0.5, 0, 1);
  return { x: nx ? -nx * bend : 0, y: ny ? -ny * bend : 0, edge };
}

export function buildGlassMaps(width, height, radius, maxPixels = 240000) {
  const ratio = Math.min(1, Math.sqrt(maxPixels / (width * height)));
  const mapWidth = Math.max(1, Math.round(width * ratio));
  const mapHeight = Math.max(1, Math.round(height * ratio));
  const displacement = new Uint8ClampedArray(mapWidth * mapHeight * 4);
  const mask = new Uint8ClampedArray(displacement.length);
  const rim = clamp(Math.min(width, height) * 0.18, 5, 12);
  const corner = clamp(radius, 0, Math.min(width, height) / 2);
  for (let y = 0; y < mapHeight; y += 1) {
    for (let x = 0; x < mapWidth; x += 1) {
      const field = glassSample((x + 0.5) * width / mapWidth, (y + 0.5) * height / mapHeight, width, height, corner, rim);
      const offset = (y * mapWidth + x) * 4;
      displacement[offset] = Math.round(127.5 + field.x * 127);
      displacement[offset + 1] = Math.round(127.5 + field.y * 127);
      displacement[offset + 2] = 128;
      displacement[offset + 3] = 255;
      mask[offset] = mask[offset + 1] = mask[offset + 2] = 255;
      mask[offset + 3] = Math.round(field.edge * 255);
    }
  }
  return { width: mapWidth, height: mapHeight, displacement, mask, rim };
}

function svgNode(name, attributes) {
  const element = document.createElementNS(SVG_NS, name);
  Object.entries(attributes).forEach(([key, value]) => element.setAttribute(key, String(value)));
  return element;
}

function imageURL(data, width, height) {
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  canvas.getContext("2d").putImageData(new ImageData(data, width, height), 0, 0);
  return canvas.toDataURL("image/png");
}

function createFilter(id, width, height, radius, blur, economical) {
  const maps = buildGlassMaps(width, height, radius, economical ? 100000 : 240000);
  const strength = clamp(maps.rim * 1.8, 9, 22);
  const filter = svgNode("filter", {
    id, x: -16, y: -16, width: width + 32, height: height + 32,
    filterUnits: "userSpaceOnUse", primitiveUnits: "userSpaceOnUse", "color-interpolation-filters": "sRGB",
  });
  const add = (name, attributes) => filter.append(svgNode(name, attributes));
  add("feImage", { href: imageURL(maps.displacement, maps.width, maps.height), x: 0, y: 0, width, height, preserveAspectRatio: "none", result: "field" });
  add("feDisplacementMap", { in: "SourceGraphic", in2: "field", scale: strength, xChannelSelector: "R", yChannelSelector: "G", result: "lens" });
  let edgeInput = "lens";
  if (!economical) {
    // Less than half a pixel of dispersion. No flicker, channel jitter, or
    // foreground filters: only background details on the rim split into RGB.
    add("feDisplacementMap", { in: "SourceGraphic", in2: "field", scale: strength - 0.75, xChannelSelector: "R", yChannelSelector: "G", result: "redLens" });
    add("feDisplacementMap", { in: "SourceGraphic", in2: "field", scale: strength + 0.75, xChannelSelector: "R", yChannelSelector: "G", result: "blueLens" });
    add("feColorMatrix", { in: "redLens", type: "matrix", values: "1 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 1 0", result: "red" });
    add("feColorMatrix", { in: "lens", type: "matrix", values: "0 0 0 0 0  0 1 0 0 0  0 0 0 0 0  0 0 0 1 0", result: "green" });
    add("feColorMatrix", { in: "blueLens", type: "matrix", values: "0 0 0 0 0  0 0 0 0 0  0 0 1 0 0  0 0 0 1 0", result: "blue" });
    add("feBlend", { in: "red", in2: "green", mode: "screen", result: "redGreen" });
    add("feBlend", { in: "redGreen", in2: "blue", mode: "screen", result: "dispersion" });
    edgeInput = "dispersion";
  }
  add("feGaussianBlur", { in: edgeInput, stdDeviation: 0.28, result: "softLens" });
  add("feImage", { href: imageURL(maps.mask, maps.width, maps.height), x: 0, y: 0, width, height, preserveAspectRatio: "none", result: "rim" });
  add("feGaussianBlur", { in: "SourceGraphic", stdDeviation: blur, result: "frost" });
  add("feComposite", { in: "softLens", in2: "rim", operator: "in", result: "edge" });
  add("feComposite", { in: "frost", in2: "rim", operator: "out", result: "center" });
  // The masks are complementary; addition avoids a dark seam in their blend.
  add("feComposite", { in: "edge", in2: "center", operator: "arithmetic", k1: 0, k2: 1, k3: 1, k4: 0 });
  return filter;
}

export function createGlassOptics() {
  const root = document.documentElement;
  const cache = new Map();
  const mounted = new Map();
  let enabled = false;
  let requested = false;
  let frame = 0;
  let serial = 0;
  let definitions;
  let svg;
  const ua = navigator.userAgent;
  // CSS.supports only checks the grammar; WebKit/Gecko accept url() without
  // actually applying displacement to a backdrop. Keep their frosted fallback.
  const refraction = /(?:Chrome|Chromium|Edg|OPR|SamsungBrowser)\//.test(ua)
    && !/(?:iPhone|iPad|iPod)/.test(ua)
    && !(navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1)
    && CSS.supports("backdrop-filter", 'url("#glass-optics-probe")');
  const reduceTransparency = window.matchMedia?.("(prefers-reduced-transparency: reduce)");
  const resizeObserver = typeof ResizeObserver === "function" ? new ResizeObserver(schedule) : null;

  function schedule() {
    if (enabled && !frame && !document.hidden) frame = requestAnimationFrame(refresh);
  }

  function remove(element) {
    element.removeAttribute("data-glass-optics");
    element.style.removeProperty("--glass-optics-filter");
    resizeObserver?.unobserve(element);
    mounted.delete(element);
  }

  function refresh() {
    frame = 0;
    if (!enabled) return;
    const candidates = [...document.querySelectorAll(SURFACES)];
    const visible = new Set();
    const started = performance.now();
    let built = 0;
    let deferred = false;
    for (const element of candidates) {
      if (element.closest(".page:not(.active)")) continue;
      const rect = element.getBoundingClientRect();
      if (rect.width < 24 || rect.height < 24 || rect.bottom < -32 || rect.top > innerHeight + 32 || rect.right < 0 || rect.left > innerWidth) continue;
      const style = getComputedStyle(element);
      if (style.visibility === "hidden" || style.display === "none") continue;
      const backdrop = style.backdropFilter || style.webkitBackdropFilter || "none";
      if (!mounted.has(element) && !backdrop.includes("blur(")) continue;
      // Close glyphs and transparent rows intentionally have no glass pane.
      if (element.matches('button:has(> [data-app-icon="close"])')) continue;
      visible.add(element);
      if (!refraction) {
        if (!mounted.has(element)) {
          element.dataset.glassOptics = "rim";
          mounted.set(element, "rim");
        }
        continue;
      }
      const width = Math.round(rect.width);
      const height = Math.round(rect.height);
      const radius = Math.round(Math.min(parseFloat(style.borderTopLeftRadius) || 0, width / 2, height / 2));
      const menu = element.matches(".modal__sheet, .subscription-switcher__menu, .notification-popover");
      const blurValue = style.getPropertyValue(menu ? "--glass-menu-backdrop" : "--glass-backdrop").match(/blur\(([\d.]+)px\)/);
      const blur = element.matches(".admin-settings-search__results, .admin-settings-search__empty") ? 18
        : Number(blurValue?.[1] || (menu ? 8 : 2.5));
      const economical = Number(navigator.hardwareConcurrency || 8) <= 4 || root.dataset.performance === "low" || root.dataset.performance === "reduced";
      const key = `${width}:${height}:${radius}:${blur}:${economical}`;
      if (mounted.get(element) === key) continue;
      let entry = cache.get(key);
      if (!entry) {
        // Build new geometry in short batches. Scrolling and resizing never
        // regenerate maps at animation frame rate; identical rows share them.
        if (built > 0 && performance.now() - started > 8) {
          deferred = true;
          continue;
        }
        if (!definitions) {
          svg = svgNode("svg", { width: 0, height: 0, "aria-hidden": "true", focusable: "false", class: "glass-optics-definitions" });
          definitions = svgNode("defs", {});
          svg.append(definitions);
          document.body.append(svg);
        }
        const id = `glass-optics-${++serial}`;
        const filter = createFilter(id, width, height, radius, blur, economical);
        built += 1;
        definitions.append(filter);
        entry = { id, filter };
        cache.set(key, entry);
      }
      element.style.setProperty("--glass-optics-filter", `url("#${entry.id}") saturate(1.08)`);
      element.dataset.glassOptics = "refraction";
      mounted.set(element, key);
      resizeObserver?.observe(element);
    }
    for (const element of mounted.keys()) if (!visible.has(element) || !element.isConnected) remove(element);
    if (cache.size > 36) {
      const used = new Set(mounted.values());
      for (const [key, entry] of cache) {
        if (!used.has(key)) {
          entry.filter.remove();
          cache.delete(key);
        }
      }
    }
    if (deferred) schedule();
  }

  // Sheets can live outside #app. Watch their content too, excluding the SVG
  // definitions we generate ourselves to avoid scheduling redundant frames.
  new MutationObserver((records) => {
    if (records.some(({ target }) => !target.closest?.(".glass-optics-definitions"))) schedule();
  }).observe(document.body, { childList: true, subtree: true });
  new MutationObserver(schedule).observe(root, { attributes: true, attributeFilter: ["data-performance"] });
  document.addEventListener("scroll", schedule, { capture: true, passive: true });
  window.addEventListener("resize", schedule, { passive: true });
  document.addEventListener("visibilitychange", schedule);

  function setEnabled(next) {
    requested = Boolean(next);
    const active = requested && !reduceTransparency?.matches;
    if (enabled === active) return;
    enabled = active;
    if (enabled) schedule();
    else {
      cancelAnimationFrame(frame);
      frame = 0;
      for (const element of mounted.keys()) remove(element);
      definitions?.replaceChildren();
      cache.clear();
    }
    root.dataset.glassOptics = enabled ? (refraction ? "refraction" : "rim") : "off";
  }
  reduceTransparency?.addEventListener("change", () => setEnabled(requested));
  return { setEnabled };
}
