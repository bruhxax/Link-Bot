import { syncPresetBackground } from "./preset-background.mjs";
import { applyCustomCursor } from "./custom-cursor.mjs";
let previous = "";
export function applyLandingAppearance(data) {
  const key = JSON.stringify([data.landing, data.appearance, data.cursor]);
  if (key === previous) return;
  previous = key;
  const settings = data.landing || {},
    appearance = data.appearance || {};
  document.documentElement.dataset.frames = appearance.showFrames === false ? "off" : "on";
  document.documentElement.style.setProperty(
    "--landing-button-radius",
    `${settings.buttonRadius ?? 8}px`,
  );
  void applyCustomCursor(data.cursor);
  const text = settings.text || {};
  for (const element of document.querySelectorAll("[data-landing-text]")) {
    const value = text[element.dataset.landingText];
    if (typeof value === "string" && value.trim()) {
      element.textContent = value;
      const link = element.closest("a[data-landing-icon]");
      if (link) {
        link.setAttribute("aria-label", value);
        link.title = value;
      }
    }
  }
  if (text.title)
    document.title = `${data.brand?.name || "Link-Bot"} — ${text.title}`;
  for (const element of document.querySelectorAll("[data-landing-icon]")) {
    const value = settings.icons?.[element.dataset.landingIcon];
    let image = element.querySelector("img.landing-custom-icon");
    if (value) {
      try {
        const url = new URL(value, location.origin);
        if (!["https:", "http:"].includes(url.protocol)) continue;
        if (!image) {
          image = document.createElement("img");
          image.className = "landing-custom-icon";
          image.alt = "";
          element.prepend(image);
        }
        image.src = url.href;
        element.querySelector("svg")?.setAttribute("hidden", "");
      } catch {}
    } else {
      image?.remove();
      element.querySelector("svg")?.removeAttribute("hidden");
    }
  }
  const shared = settings.background || { mode: "solid" };
  const paint = (parent, selection, id) => {
    let host = document.getElementById(id);
    if (!host) {
      host = document.createElement("div");
      host.id = id;
      host.className = "landing-background";
      host.setAttribute("aria-hidden", "true");
      parent.prepend(host);
    }
    syncPresetBackground(host, selection, appearance, {
      hidden: id !== "landing-shared-background" && !settings.perPage,
    });
  };
  paint(
    document.body,
    settings.perPage ? { mode: "solid" } : shared,
    "landing-shared-background",
  );
  for (const [key, id] of [
    ["home", "top"],
    ["servers", "servers"],
    ["plans", "plans"],
    ["support", "support"],
  ]) {
    const section = document.getElementById(id);
    if (!section) continue;
    section.classList.toggle(
      "has-section-background",
      Boolean(settings.perPage),
    );
    const override = settings.pages?.[key];
    paint(
      section,
      settings.perPage
        ? override?.mode && override.mode !== "inherit"
          ? override
          : shared
        : { mode: "solid" },
      `landing-background-${key}`,
    );
    document.getElementById(`landing-background-${key}`).hidden =
      !settings.perPage;
  }
}
