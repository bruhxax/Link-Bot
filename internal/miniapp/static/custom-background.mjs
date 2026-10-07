// The same geometry is used by the public background and the draggable preview.
export const clamp = (value, min, max) => Math.max(min, Math.min(max, Number(value)));
export function backgroundGeometry(vw, vh, mw, mh, settings = {}) {
  if (!(vw > 0 && vh > 0 && mw > 0 && mh > 0)) return { width: 0, height: 0, left: 0, top: 0 };
  const fit = settings.fit === "contain" ? Math.min(vw / mw, vh / mh) : Math.max(vw / mw, vh / mh);
  const ratio = fit * clamp(settings.scale ?? 100, 50, 300) / 100;
  const width = mw * ratio, height = mh * ratio;
  return { width, height, left: (vw - width) * clamp(settings.positionX ?? 50, 0, 100) / 100, top: (vh - height) * clamp(settings.positionY ?? 50, 0, 100) / 100 };
}
export function draggedPosition(position, delta, viewport, media) {
  const space = viewport - media;
  return Math.abs(space) < 0.5 ? position : clamp(position + delta / space * 100, 0, 100);
}
export function gifDelay(frame) { return Math.max(20, Number(frame.delay) || 100); }

export function disposeGIFFrame(ctx, previous, saved, backgroundColor) {
  if (previous?.disposalType === 2) {
    const d = previous.dims;
    if (backgroundColor && previous.transparentIndex === undefined) { ctx.fillStyle = backgroundColor; ctx.fillRect(d.left, d.top, d.width, d.height); }
    else ctx.clearRect(d.left, d.top, d.width, d.height);
  } else if (previous?.disposalType === 3 && saved) ctx.putImageData(saved, 0, 0);
}

const controllers = new Map();
let decoder;
async function decodeGIF(url, signal) {
  decoder ||= import("./gif-decoder.bundle.mjs");
  const response = await fetch(url, { signal, cache: "force-cache" });
  if (!response.ok) throw new Error("Не удалось загрузить GIF");
  const buffer = await response.arrayBuffer();
  if (buffer.byteLength > 50 * 1024 * 1024) throw new Error("GIF больше 50 МБ");
  const { parseGIF, decompressFrame } = await decoder;
  const gif = parseGIF(buffer);
  const frames = gif.frames.filter(frame => frame.image);
  if (!frames.length || frames.length > 1000 || gif.lsd.width * gif.lsd.height > 4_194_304) throw new Error("GIF слишком большой для анимации фона");
  let pixels = 0;
  for (const frame of frames) {
    const d = frame.image.descriptor;
    pixels += d.width * d.height;
    if (!d.width || !d.height || d.left + d.width > gif.lsd.width || d.top + d.height > gif.lsd.height || pixels > 150_000_000) throw new Error("GIF слишком большой для анимации фона");
  }
  return { gif, frames, decompressFrame };
}

class BackgroundMedia {
  constructor(host) {
    this.host = host;
    this.width = this.height = 0;
    this.frame = 0;
    this.observer = new ResizeObserver(() => this.layout());
    this.observer.observe(host);
  }
  update(settings, options) {
    this.settings = settings; this.options = options;
    this.host.style.setProperty("--custom-dimming", `${clamp(settings?.dimming ?? 0, 0, 90) / 100}`);
    if (this.url !== settings?.url) {
      this.reset();
      this.url = settings?.url;
      if (this.url) void this.load(settings);
    }
    this.layout(); this.playback();
  }
  async load(settings) {
    const url = this.url;
    const abort = new AbortController();
    this.abort = abort;
    try {
      this.host.dataset.loading = "true";
      delete this.host.dataset.error;
      if (settings.type === "gif") {
        const decoded = await decodeGIF(url, abort.signal);
        if (this.url !== url || abort.signal.aborted) return;
        this.gif = decoded;
		const color = decoded.gif.gct?.[decoded.gif.lsd.backgroundColorIndex];
		this.backgroundColor = color ? `rgb(${color.join(",")})` : null;
        this.width = decoded.gif.lsd.width; this.height = decoded.gif.lsd.height;
        this.media = document.createElement("canvas");
        this.media.width = this.width; this.media.height = this.height;
        this.ctx = this.media.getContext("2d", { willReadFrequently: true });
        this.patch = document.createElement("canvas");
        this.patchCtx = this.patch.getContext("2d");
        this.cache = new Map(); this.cacheBytes = 0;
        this.drawFrame(0);
      } else {
        const video = settings.type === "video";
        const media = document.createElement(video ? "video" : "img");
        this.media = media;
        if (video) { media.muted = true; media.defaultMuted = true; media.loop = true; media.playsInline = true; media.setAttribute("playsinline", ""); media.preload = "auto"; }
        else { media.alt = ""; media.draggable = false; }
        media.src = url;
        await new Promise((resolve, reject) => {
          media.addEventListener(video ? "loadeddata" : "load", resolve, { once: true });
          media.addEventListener("error", () => reject(new Error("Браузер не поддерживает этот файл или кодек видео")), { once: true });
          abort.signal.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")), { once: true });
          if (!video && media.complete && media.naturalWidth) resolve();
        });
        if (this.url !== url || abort.signal.aborted) return;
        this.width = video ? media.videoWidth : media.naturalWidth;
        this.height = video ? media.videoHeight : media.naturalHeight;
      }
      this.media.className = "custom-background__media";
      this.host.prepend(this.media);
      delete this.host.dataset.loading;
      this.layout(); this.playback();
    } catch (error) {
      if (error.name === "AbortError" || this.url !== url || abort.signal.aborted) return;
      delete this.host.dataset.loading;
      this.host.dataset.error = error.message;
      this.options.onError?.(error.message);
    }
  }
  drawFrame(index) {
    disposeGIFFrame(this.ctx, this.previous, this.saved, this.backgroundColor);
    if (!this.previous) {
	  this.ctx.clearRect(0, 0, this.width, this.height);
	  if (this.backgroundColor && !this.gif.frames[0].gce?.extras?.transparentColorGiven) { this.ctx.fillStyle = this.backgroundColor; this.ctx.fillRect(0, 0, this.width, this.height); }
	}
    let frame = this.cache.get(index);
    if (!frame) {
      frame = this.gif.decompressFrame(this.gif.frames[index], this.gif.gif.gct, true);
	  // The indexed pixels are a JS array; retain only the compact RGBA patch.
	  delete frame.pixels; delete frame.colorTable;
      // Keep at most 32 MiB of decoded patches. Composited frames are never cached.
      while (this.cacheBytes + frame.patch.byteLength > 32 * 1024 * 1024 && this.cache.size) {
        const key = this.cache.keys().next().value;
        this.cacheBytes -= this.cache.get(key).patch.byteLength; this.cache.delete(key);
      }
      this.cache.set(index, frame); this.cacheBytes += frame.patch.byteLength;
    }
    this.saved = frame.disposalType === 3 ? this.ctx.getImageData(0, 0, this.width, this.height) : null;
    const { width, height, left, top } = frame.dims;
    if (this.patch.width !== width) this.patch.width = width;
    if (this.patch.height !== height) this.patch.height = height;
    this.patchCtx.putImageData(new ImageData(frame.patch, width, height), 0, 0);
    this.ctx.drawImage(this.patch, left, top);
    this.previous = frame; this.frame = index; this.remaining = gifDelay(frame);
  }
  playback() {
    const paused = this.options.paused || document.hidden;
    if (this.media?.tagName === "VIDEO") {
      this.media.playbackRate = clamp(this.settings.speed ?? 100, 10, 200) / 100;
      if (paused) this.media.pause(); else void this.media.play().catch(() => {});
    }
    if (!this.gif || paused) { cancelAnimationFrame(this.raf); this.raf = 0; this.lastTime = 0; return; }
    if (this.raf) return;
    const tick = time => {
      if (!this.host.isConnected) { this.destroy(); controllers.delete(this.host); return; }
      if (this.lastTime) this.remaining -= Math.min(100, time - this.lastTime) * clamp(this.settings.speed ?? 100, 10, 200) / 100;
      this.lastTime = time;
      let steps = 0;
      while (this.remaining <= 0 && steps++ < 10) {
        const overdue = this.remaining;
        this.drawFrame((this.frame + 1) % this.gif.frames.length);
        this.remaining += overdue;
      }
      this.raf = requestAnimationFrame(tick);
    };
    this.raf = requestAnimationFrame(tick);
  }
  layout() {
    if (!this.media) return;
    this.geometry = backgroundGeometry(this.host.clientWidth, this.host.clientHeight, this.width, this.height, this.settings);
    const g = this.geometry;
    Object.assign(this.media.style, { width: `${g.width}px`, height: `${g.height}px`, transform: `translate3d(${g.left}px,${g.top}px,0)` });
  }
  reset() {
    this.abort?.abort(); cancelAnimationFrame(this.raf); this.raf = 0; this.lastTime = 0;
    if (this.media?.tagName === "VIDEO") { this.media.pause(); this.media.removeAttribute("src"); this.media.load(); }
    this.media?.remove(); this.media = null; this.gif = null; this.previous = this.saved = null; this.cache = null;
    this.ctx = this.patch = this.patchCtx = null; this.cacheBytes = 0; this.backgroundColor = null;
    this.width = this.height = 0;
    delete this.host.dataset.loading; delete this.host.dataset.error;
  }
  destroy() { this.reset(); this.observer.disconnect(); }
}

export function syncCustomBackground(host, settings, options = {}) {
  for (const [oldHost, controller] of controllers) if (!oldHost.isConnected) { controller.destroy(); controllers.delete(oldHost); }
  if (!host) return;
  if (!settings) { controllers.get(host)?.destroy(); controllers.delete(host); return; }
  let controller = controllers.get(host);
  if (!controller) { controller = new BackgroundMedia(host); controllers.set(host, controller); }
  controller.update(settings, options);
  return controller;
}

export function probeBackground(url, type) {
  return new Promise((resolve, reject) => {
    const video = type === "video";
    const media = document.createElement(video ? "video" : "img");
    const cleanup = () => { clearTimeout(timeout); media.removeAttribute("src"); if (video) media.load(); };
    const timeout = setTimeout(() => { cleanup(); reject(new Error("Не удалось открыть файл фона")); }, 20_000);
    media.addEventListener(video ? "loadeddata" : "load", async () => {
      try { if (type === "gif") await decodeGIF(url); cleanup(); resolve(); }
      catch (error) { cleanup(); reject(error); }
    }, { once: true });
    media.addEventListener("error", () => { cleanup(); reject(new Error("Браузер не поддерживает этот формат или кодек. Для видео попробуйте MP4 (H.264) или WebM")); }, { once: true });
    if (video) { media.muted = true; media.preload = "auto"; }
    media.src = url;
  });
}
