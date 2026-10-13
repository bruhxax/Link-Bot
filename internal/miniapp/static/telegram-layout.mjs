// Telegram reports the current presentation, not the BotFather setting.
export function telegramDevice(webApp, navigatorInfo = {}) {
  const platform = String(webApp?.platform || "").toLowerCase();
  if (["android", "android_x", "ios"].includes(platform)) return "mobile";
  if (["tdesktop", "macos", "unigram", "windows", "linux"].includes(platform)) return "desktop";
  const ua = String(navigatorInfo.userAgent || "");
  const iPadDesktop = navigatorInfo.platform === "MacIntel" && Number(navigatorInfo.maxTouchPoints) > 1;
  if (/Android|iPhone|iPad|iPod|Mobile/i.test(ua) || iPadDesktop) return "mobile";
  if (/Windows|Macintosh|Mac OS|Linux|CrOS/i.test(ua) || ["web", "weba", "webk"].includes(platform)) return "desktop";
  return "unknown";
}

// Both chat buttons and the main Mini App enter here. Native startup may report
// fullscreen after ready(), so a desktop exit also handles that late event.
export function configureTelegramLaunch(webApp, navigatorInfo = {}) {
  if (!String(webApp?.initData || "").trim()) return;
  const device = telegramDevice(webApp, navigatorInfo);
  if (device === "unknown") return;
  let disabled = false;
  let exitRequests = 0;
  let expanded = false;
  const expandMobile = () => {
    if (device !== "mobile" || expanded) return;
    expanded = true;
    try { webApp.expand?.(); } catch { /* older clients keep their native mode */ }
  };
  const supportsFullscreen = typeof webApp.isVersionAtLeast === "function"
    ? webApp.isVersionAtLeast("8.0")
    : typeof webApp.requestFullscreen === "function" && typeof webApp.exitFullscreen === "function";
  if (!supportsFullscreen) { expandMobile(); return; }
  const exitDesktopFullscreen = () => {
    if (disabled || device !== "desktop" || exitRequests >= 2) return;
    exitRequests += 1;
    try { webApp.exitFullscreen?.(); } catch { disabled = true; }
  };
  webApp.onEvent?.("fullscreenChanged", () => {
    if (webApp.isFullscreen) exitDesktopFullscreen();
  });
  webApp.onEvent?.("fullscreenFailed", (event) => {
    if (event?.error === "ALREADY_FULLSCREEN") return;
    disabled = true;
    expandMobile();
  });
  if (device === "desktop") {
    // Request even before the SDK sees the BotFather launch mode.
    exitDesktopFullscreen();
  } else if (!webApp.isFullscreen) {
    try {
      if (typeof webApp.requestFullscreen === "function") webApp.requestFullscreen();
      else expandMobile();
    } catch { disabled = true; expandMobile(); }
  }
}

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
