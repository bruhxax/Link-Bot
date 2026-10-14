(() => {
  "use strict";

  const $ = (id) => document.getElementById(id);
  const cabinetBase = document.body.dataset.cabinetBase || window.location.origin;
  const assetVersion = new URL(document.currentScript?.src || window.location.href).searchParams.get("v") || "";
  const cabinetPrefix = document.body.dataset.cabinetBase ? "" : "/cabinet";
  const supportURL = new URL(cabinetPrefix + "/support", cabinetBase).href;
  const buyURL = new URL(cabinetPrefix + "/plans", cabinetBase).href;
  const colorVars = {
    background: "--bg", surface: "--surface", surfaceStrong: "--surface-strong",
    text: "--text", muted: "--muted", border: "--border", accent: "--accent",
    success: "--success", danger: "--danger",
  };
  const visualModule=import(`./landing-appearance.mjs${assetVersion ? `?v=${encodeURIComponent(assetVersion)}` : ""}`);
  const reviewsModule = import(`./landing-reviews.mjs${assetVersion ? `?v=${encodeURIComponent(assetVersion)}` : ""}`);
  let reviewsTicker;
 let landingText={};
 let loading = false;
  let previousNodes = "";
  let previousNodeStructure = "";
  let previousPlans = "";
  let previousContacts = "";
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
  const revealObserver = "IntersectionObserver" in window && !reducedMotion.matches
    ? new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          entry.target.classList.add("visible");
          revealObserver.unobserve(entry.target);
        }
      });
    }, { threshold: .12, rootMargin: "0px 0px -40px 0px" }) : null;

  function observeReveals(root = document) {
    root.querySelectorAll(".reveal:not(.visible)").forEach((element) => {
      if (revealObserver) revealObserver.observe(element);
      else element.classList.add("visible");
    });
  }

  function escapeHTML(value) {
    return String(value ?? "").replace(/[&<>"']/g, (char) => ({
      "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
    })[char]);
  }

  function safeURL(value, allowLocal = false) {
    if (!String(value || "").trim()) return "";
    try {
      const url = new URL(String(value || ""), window.location.origin);
      if (url.protocol !== "https:" && url.protocol !== "http:") return "";
      if (!allowLocal && !/^https?:\/\//i.test(String(value || ""))) return "";
      return url.href;
    } catch { return ""; }
  }

  function setColors(colors) {
    if (!colors || typeof colors !== "object") return;
    Object.entries(colorVars).forEach(([key, property]) => {
      const color = String(colors[key] || "").trim();
      if (/^#[0-9a-f]{6}$/i.test(color)) document.documentElement.style.setProperty(property, color);
    });
    if (/^#[0-9a-f]{6}$/i.test(String(colors.background || ""))) {
      document.querySelector('meta[name="theme-color"]')?.setAttribute("content", colors.background);
    }
    const accent = String(colors.accent || "");
    if (/^#[0-9a-f]{6}$/i.test(accent)) {
      const channels = [1, 3, 5].map((index) => parseInt(accent.slice(index, index + 2), 16) / 255);
      const linear = channels.map((value) => value <= .04045 ? value / 12.92 : ((value + .055) / 1.055) ** 2.4);
      const luminance = .2126 * linear[0] + .7152 * linear[1] + .0722 * linear[2];
      document.documentElement.style.setProperty("--accent-ink", luminance > .35 ? "#111217" : "#ffffff");
    }
  }

  function setBrand(brand) {
    const name = String(brand?.name || "").trim() || "Link-Bot";
    $("brand-name").textContent = name;
    $("cabinet-transition-brand").textContent = name;
    document.title = `${name} — ${landingText.title||"интернет без ограничений"}`;
    const logo = $("brand-logo");
    const url = safeURL(brand?.logoUrl, true);
    if (url && logo.src !== url) logo.src = url;
  }

  function plural(number, one, few, many) {
    const value = Math.abs(Number(number) || 0);
    const remainder = value % 10;
    const hundreds = value % 100;
    return value === 1 || (remainder === 1 && hundreds !== 11) ? one
      : remainder >= 2 && remainder <= 4 && (hundreds < 12 || hundreds > 14) ? few : many;
  }

  function duration(plan) {
    const days = Number(plan.days) || 0;
    const months = Number(plan.months) || 0;
    if (days > 0) return `${days} ${plural(days, "день", "дня", "дней")}`;
    if (months > 0) return `${months} ${plural(months, "месяц", "месяца", "месяцев")}`;
    return "Специальный тариф";
  }

  function countryName(code) {
    const value = String(code || "").trim().toUpperCase();
    if (!/^[A-Z]{2}$/.test(value)) return "Сервер сети";
    try { return new Intl.DisplayNames(["ru"], { type: "region" }).of(value) || value; }
    catch { return value; }
  }

  function countryFlag(code) {
    const value = String(code || "").trim().toUpperCase();
    if (!/^[A-Z]{2}$/.test(value) || value === "XX") return "";
    return `<img class="server-card__flag" src="/mini-app/assets/flags/${value.toLowerCase()}.svg${assetVersion ? `?v=${encodeURIComponent(assetVersion)}` : ""}" alt="" width="24" height="18" loading="lazy">`;
  }

  function renderNodes(nodes, available) {
    $("servers-updated").textContent = available ? "" : "Не удалось получить актуальный статус серверов.";
    $("servers-updated").hidden = available;
    const fingerprint = JSON.stringify({ nodes, available });
    if (fingerprint === previousNodes) return;
    previousNodes = fingerprint;
    const list = $("servers-list");
    const safeNodes = Array.isArray(nodes) ? nodes : [];
    const structure = JSON.stringify(safeNodes.map((node) => [node.name, node.countryCode]));
    if (!safeNodes.length) {
      list.innerHTML = `<div class="empty-state">${available ? "Серверы пока не опубликованы." : "Статус серверов временно недоступен. Попробуйте позже."}</div>`;
    } else if (structure !== previousNodeStructure || list.querySelectorAll(".server-card").length !== safeNodes.length) {
      list.innerHTML = safeNodes.map((node) => {
        const status = available ? (node.online ? "Работает" : "Неактивен") : "Нет данных";
        const statusClass = available ? (node.online ? "status-dot--online" : "") : "status-dot--unknown";
        return `<article class="server-card">
          ${countryFlag(node.countryCode)}
          <div class="server-card__info"><span class="server-card__name">${escapeHTML(node.name || "Сервер")}</span><span class="server-card__country">${escapeHTML(countryName(node.countryCode))}</span></div>
          <span class="server-card__status" role="img" aria-label="${status}"><span class="status-dot ${statusClass}" aria-hidden="true"></span></span>
        </article>`;
      }).join("");
    } else {
      list.querySelectorAll(".server-card").forEach((card, index) => {
        const status = available ? (safeNodes[index].online ? "Работает" : "Неактивен") : "Нет данных";
        const statusClass = available ? (safeNodes[index].online ? "status-dot status-dot--online" : "status-dot") : "status-dot status-dot--unknown";
        const dot = card.querySelector(".status-dot");
        const indicator = card.querySelector(".server-card__status");
        if (dot.className !== statusClass) dot.className = statusClass;
        if (indicator.getAttribute("aria-label") !== status) indicator.setAttribute("aria-label", status);
      });
    }
    previousNodeStructure = structure;
    const online = available ? safeNodes.filter((node) => node.online).length : 0;
    $("servers-count").textContent = safeNodes.length ? `${safeNodes.length} ${plural(safeNodes.length, "сервер", "сервера", "серверов")}${available ? ` · ${online} онлайн` : ""}` : "";
  }

  function formatTraffic(bytes) {
    const value = Number(bytes) || 0;
    if (value <= 0) return "Без ограничения трафика";
    const gb = value / (1024 ** 3);
    return `Трафик: ${new Intl.NumberFormat("ru", { maximumFractionDigits: 1 }).format(gb)} ГБ`;
  }

  function renderPlans(plans) {
    const fingerprint = JSON.stringify([plans,landingText.buyButton]);
    if (fingerprint === previousPlans) return;
    previousPlans = fingerprint;
    const list = $("plans-list");
    const safePlans = Array.isArray(plans) ? plans : [];
    list.dataset.count = String(safePlans.length);
    if (!safePlans.length) {
      list.innerHTML = '<div class="empty-state">Доступные тарифы появятся здесь.</div>';
      return;
    }
    list.innerHTML = safePlans.map((plan) => {
      const planDuration = duration(plan);
      const rawTitle = String(plan.name || "").trim() || String(plan.titleRu || "").trim() || `На ${planDuration}`;
      const title = escapeHTML(rawTitle);
      const durationNote = rawTitle.toLocaleLowerCase("ru") === planDuration.toLocaleLowerCase("ru") ? "" : `<span class="plan-card__duration">${escapeHTML(planDuration)}</span>`;
      const priceRub = Number(plan.priceRub) || 0;
      const priceStars = Number(plan.priceStars) || 0;
      const price = plan.freeOneTime ? "Бесплатно" : priceRub > 0
        ? `${new Intl.NumberFormat("ru").format(priceRub)} ₽` : priceStars > 0 ? `${priceStars} Stars` : "Уточняйте";
      const deviceLimit = Number(plan.deviceLimitCount) || 0;
      const badge = plan.recommended ? "Популярный" : Number(plan.savingsPercent) > 0 ? `Выгода ${Number(plan.savingsPercent)}%` : "";
      return `<article class="plan-card${plan.recommended ? " plan-card--recommended" : ""}">
        <div class="plan-card__top"><div><div class="plan-card__title">${title}</div>${durationNote}</div>${badge ? `<span class="plan-card__badge">${escapeHTML(badge)}</span>` : ""}</div>
        <div class="plan-card__price">${escapeHTML(price)}</div>
        <ul class="plan-card__features"><li>${escapeHTML(formatTraffic(plan.trafficLimitBytes))}</li><li>${deviceLimit > 0 ? `До ${deviceLimit} ${plural(deviceLimit, "устройства", "устройств", "устройств")}` : "Устройства по условиям тарифа"}</li><li>Подключение после оформления</li></ul>
        <a class="button ${plan.recommended ? "button--primary" : "button--ghost"} plan-card__action" href="${buyURL}">${escapeHTML(landingText.buyButton||"Выбрать тариф")}</a>
      </article>`;
    }).join("");
  }

  function renderContacts(contacts) {
    const fingerprint = JSON.stringify([contacts,landingText.contactButton]);
    if (fingerprint === previousContacts) return;
    previousContacts = fingerprint;
    const safeContacts = Array.isArray(contacts) ? contacts : [];
    const arrow = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14m-6-6 6 6-6 6"/></svg>';
    const message = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M8 9h8m-8 4h5M7 19H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-7l-5 3Z"/></svg>';
    const plane = '<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M20.7 3.4c.7-.3 1.3.2 1.1 1L18.5 20c-.2 1-1 1.2-1.8.6l-5-3.7-2.4 2.3c-.3.3-.5.5-1 .5l.4-5.1L18 6.2c.4-.4-.1-.6-.6-.3L5.8 13.2.9 11.7c-1-.3-1-1 .2-1.5Z"/></svg>';
    const cards = [`<a class="contact-card contact-card--support" href="${supportURL}">
      <span class="contact-card__icon">${message}</span>
      <span class="contact-card__body"><strong class="contact-card__title">Обращение</strong><span class="contact-card__meta">Переписка в кабинете</span><span class="contact-card__action">${escapeHTML(landingText.contactButton||"Написать обращение")}</span></span>
      <span class="contact-card__arrow">${arrow}</span>
    </a>`];
    safeContacts.forEach((contact) => {
      const href = safeURL(contact.url);
      if (!href) return;
      const telegram = /^(t\.me|telegram\.me)$/i.test(new URL(href).hostname);
      const label = String(contact.label || "Связаться с нами");
      const username = new URL(href).pathname.match(/^\/([a-z0-9_]{5,32})\/?$/i)?.[1];
      const subtitle = telegram ? (username ? `@${username}` : "Прямая связь в Telegram") : new URL(href).hostname;
      cards.push(`<a class="contact-card${telegram ? " contact-card--telegram" : ""}" href="${escapeHTML(href)}" target="_blank" rel="noopener noreferrer">
        <span class="contact-card__icon">${telegram ? plane : message}</span>
        <span class="contact-card__body"><strong class="contact-card__title">${escapeHTML(telegram ? "Telegram" : label)}</strong><span class="contact-card__meta">${escapeHTML(subtitle)}</span><span class="contact-card__action">${escapeHTML(landingText.contactButton || (telegram ? "Написать в Telegram" : "Открыть контакт"))}</span></span>
        <span class="contact-card__arrow">${arrow}</span>
      </a>`);
    });
    $("contacts-list").innerHTML = cards.join("");
  }

  async function loadLanding() {
    if (loading) return;
    loading = true;
    try {
      const response = await fetch("/api/site/landing", { cache: "no-store", credentials: "same-origin" });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const payload = await response.json();
      if (!payload?.ok || !payload.data) throw new Error("Invalid site data");
      const data = payload.data;
 landingText=data.landing?.text||{};
      setColors(data.colors);
      document.documentElement.dataset.glass = data.glass === true ? "on" : "off";
      setBrand(data.brand);
      renderNodes(data.nodes, Boolean(data.nodesAvailable));
      renderPlans(data.plans);
      renderContacts(data.contacts);
      reviewsModule.then(({ createReviewTicker }) => {
        reviewsTicker ||= createReviewTicker($("landing-reviews"));
        reviewsTicker.setReviews(data.reviews);
      }).catch(() => {});
 (await visualModule).applyLandingAppearance(data);
    } catch {
      if (!previousNodes) $("servers-list").innerHTML = '<div class="empty-state">Не удалось загрузить серверы. Попробуйте обновить страницу.</div>';
      if (!previousPlans) $("plans-list").innerHTML = '<div class="empty-state">Не удалось загрузить тарифы. Попробуйте обновить страницу.</div>';
      if (!previousContacts) renderContacts([]);
      $("servers-updated").textContent = "Не удалось обновить данные. Повторим попытку автоматически.";
      $("servers-updated").hidden = false;
    } finally {
      loading = false;
    }
  }

  $("brand-logo").addEventListener("error", () => {
    if (!$("brand-logo").src.endsWith("/mini-app/assets/brand-mark.png")) $("brand-logo").src = "/mini-app/assets/brand-mark.png";
  });
  $("servers-list").addEventListener("error", (event) => {
    if (event.target?.classList?.contains("server-card__flag")) event.target.remove();
  }, true);
  const cabinetOrigin = new URL(cabinetBase, window.location.href).origin;
  const cabinetTransition = $("cabinet-transition");
  let navigatingToCabinet = false;
  document.addEventListener("click", (event) => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || navigatingToCabinet) return;
    const link = event.target.closest?.("a[href]");
    if (!link || link.target === "_blank" || link.hasAttribute("download")) return;
    const destination = new URL(link.href, window.location.href);
    if (destination.origin !== cabinetOrigin || !destination.pathname.startsWith("/mini-app/")) return;
    if (reducedMotion.matches) return;
    event.preventDefault();
    navigatingToCabinet = true;
    cabinetTransition.hidden = false;
    window.requestAnimationFrame(() => window.requestAnimationFrame(() => cabinetTransition.classList.add("is-visible")));
    window.setTimeout(() => window.location.assign(destination.href), 480);
  });
  window.addEventListener("pageshow", () => {
    navigatingToCabinet = false;
    cabinetTransition.classList.remove("is-visible");
    cabinetTransition.hidden = true;
  });
  if (revealObserver) document.documentElement.classList.add("motion-ready");
  observeReveals();
  const trial = $("hero-trial");
  let trialVisible = !("IntersectionObserver" in window);
  const syncSparks = () => trial.classList.toggle("has-active-sparks", trialVisible && !document.hidden && !reducedMotion.matches);
  if ("IntersectionObserver" in window) {
    new IntersectionObserver(([entry]) => { trialVisible = entry.isIntersecting; syncSparks(); }).observe(trial);
  }
  document.addEventListener("visibilitychange", syncSparks);
  reducedMotion.addEventListener?.("change", syncSparks);
  syncSparks();
  if ("IntersectionObserver" in window) {
    const sectionLinks = [...document.querySelectorAll('.nav-links a[href^="#"]')];
    const setActiveSection = (id) => sectionLinks.forEach((link) => {
      const active = link.getAttribute("href") === `#${id}`;
      link.classList.toggle("active", active);
      if (active) link.setAttribute("aria-current", "location");
      else link.removeAttribute("aria-current");
    });
    sectionLinks.forEach((link) => link.addEventListener("click", () => setActiveSection(link.hash.slice(1))));
    const navObserver = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return;
        setActiveSection(entry.target.id);
      });
    }, { rootMargin: "-20% 0px -65% 0px" });
    document.querySelectorAll("main .section[id]").forEach((section) => navObserver.observe(section));
  }
  loadLanding();
  window.setInterval(() => { if (!document.hidden) loadLanding(); }, 30000);
  document.addEventListener("visibilitychange", () => { if (!document.hidden) loadLanding(); });
})();
