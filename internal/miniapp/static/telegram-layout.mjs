// Telegram reports the current presentation, not the BotFather setting.
export function telegramLayout(webApp, fallbackHeight) {
  const inset = (value) => Number.isFinite(Number(value)) ? Math.max(0, Number(value)) : 0;
  const mode = webApp ? (webApp.isFullscreen ? "fullscreen" : webApp.isExpanded ? "fullsize" : "compact") : "browser";
  const safe = webApp?.safeAreaInset || {};
  const content = webApp?.contentSafeAreaInset || {};
  const height = inset(webApp?.viewportStableHeight) || inset(webApp?.viewportHeight) || inset(fallbackHeight);
  return {
    mode,
    height: Math.round(height),
    top: inset(safe.top) + inset(content.top),
    bottom: inset(safe.bottom) + inset(content.bottom),
    left: inset(safe.left) + inset(content.left),
    right: inset(safe.right) + inset(content.right),
  };
}

export function syncTelegramLayout(root, webApp, fallbackHeight) {
  const layout = telegramLayout(webApp, fallbackHeight);
  root.dataset.launchMode = layout.mode;
  if (layout.height > 0) root.style.setProperty("--app-viewport-height", `${layout.height}px`);
  for (const side of ["top", "bottom", "left", "right"]) {
    root.style.setProperty(`--telegram-safe-${side}`, `${layout[side]}px`);
  }
}
