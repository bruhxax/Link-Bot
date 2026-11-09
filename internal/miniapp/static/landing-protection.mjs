// One status request and one display period per visit. The regular landing
// refresh never restarts this timer or replays the banner entrance.
export function createProtectionBanner(
  element,
  nav,
  { fetcher = fetch, setTimer = setTimeout, clearTimer = clearTimeout } = {},
) {
  let timer,
    request,
    animation,
    disposed = false,
    generation = 0;
  const reduced = matchMedia("(prefers-reduced-motion: reduce)");
  const text = element.querySelector("[data-protection-text]");
  function layout() {
    const rect = nav.getBoundingClientRect();
    element.style.top = `${Math.round(rect.bottom + 9)}px`;
    element.style.maxWidth = `${Math.round(rect.width)}px`;
    if (!element.hidden)
      document.documentElement.style.setProperty(
        "--landing-header-space",
        `${Math.ceil(element.getBoundingClientRect().bottom + 24)}px`,
      );
  }
  const observer =
    typeof ResizeObserver === "function" ? new ResizeObserver(layout) : null;
  observer?.observe(nav);
  observer?.observe(element);
  window.addEventListener("resize", layout, { passive: true });
  function hide() {
    clearTimer(timer);
    animation?.cancel();
    if (element.hidden) return;
    if (reduced.matches || typeof element.animate !== "function") {
      element.hidden = true;
      return;
    }
    animation = element.animate(
      [
        { opacity: 1, translate: "0 0" },
        { opacity: 0, translate: "0 -7px" },
      ],
      { duration: 240, easing: "cubic-bezier(.4,0,1,1)" },
    );
    const running = animation;
    running.finished
      .then(() => {
        if (animation === running) element.hidden = true;
      })
      .catch(() => {});
  }
  async function show() {
    const turn = ++generation;
    clearTimer(timer);
    request?.abort();
    animation?.cancel();
    element.hidden = true;
    request = new AbortController();
    try {
      const response = await fetcher("/api/site/protection", {
        cache: "no-store",
        credentials: "same-origin",
        signal: request.signal,
      });
      if (!response.ok) throw Error("status unavailable");
      const payload = await response.json(),
        data = payload?.data;
      if (
        disposed ||
        turn !== generation ||
        !payload?.ok ||
        !data?.enabled ||
        !data.text
      )
        return;
      text.textContent = String(data.text);
      element.dataset.protected = String(data.state === "protected");
      element.hidden = false;
      layout();
      if (!reduced.matches && typeof element.animate === "function")
        animation = element.animate(
          [
            { opacity: 0, translate: "0 -7px" },
            { opacity: 1, translate: "0 0" },
          ],
          { duration: 320, easing: "cubic-bezier(.22,1,.36,1)" },
        );
      const seconds = Number(data.durationSeconds);
      if (seconds > 0) timer = setTimer(hide, Math.min(3600, seconds) * 1000);
    } catch {
      if (turn === generation) element.hidden = true;
    }
  }
  return {
    show,
    hide,
    destroy() {
      disposed = true;
      ++generation;
      clearTimer(timer);
      request?.abort();
      animation?.cancel();
      observer?.disconnect();
      window.removeEventListener("resize", layout);
      element.hidden = true;
    },
  };
}
