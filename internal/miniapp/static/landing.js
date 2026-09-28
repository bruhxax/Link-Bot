(() => {
  "use strict";

  const $ = (id) => document.getElementById(id);
  const cabinetBase = document.body.dataset.cabinetBase || window.location.origin;
  const assetVersion = new URL(document.currentScript?.src || window.location.href).searchParams.get("v") || "";
  const supportURL = new URL("/mini-app/?page=support", cabinetBase).href;
  const buyURL = new URL("/mini-app/?page=buy", cabinetBase).href;
  const colorVars = {
    background: "--bg", surface: "--surface", surfaceStrong: "--surface-strong",
    text: "--text", muted: "--muted", border: "--border", accent: "--accent",
    success: "--success", danger: "--danger",
  };
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
    document.title = `${name} — интернет без ограничений`;
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
          <span class="server-card__status"><span class="status-dot ${statusClass}" aria-hidden="true"></span><span class="server-card__status-label">${status}</span></span>
        </article>`;
      }).join("");
    } else {
      list.querySelectorAll(".server-card").forEach((card, index) => {
        const status = available ? (safeNodes[index].online ? "Работает" : "Неактивен") : "Нет данных";
        const statusClass = available ? (safeNodes[index].online ? "status-dot status-dot--online" : "status-dot") : "status-dot status-dot--unknown";
        const dot = card.querySelector(".status-dot");
        const label = card.querySelector(".server-card__status-label");
        if (dot.className !== statusClass) dot.className = statusClass;
        if (label.textContent !== status) label.textContent = status;
      });
    }
    previousNodeStructure = structure;
    const online = available ? safeNodes.filter((node) => node.online).length : 0;
    $("servers-count").textContent = safeNodes.length ? `${safeNodes.length} ${plural(safeNodes.length, "сервер", "сервера", "серверов")}${available ? ` · ${online} онлайн` : ""}` : "";
    $("servers-updated").textContent = available ? "Статус обновляется автоматически." : "Не удалось получить актуальный статус серверов.";
  }

  function formatTraffic(bytes) {
    const value = Number(bytes) || 0;
    if (value <= 0) return "Без ограничения трафика";
    const gb = value / (1024 ** 3);
    return `Трафик: ${new Intl.NumberFormat("ru", { maximumFractionDigits: 1 }).format(gb)} ГБ`;
  }

  function renderPlans(plans) {
    const fingerprint = JSON.stringify(plans);
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
      const rawTitle = String(plan.titleRu || "").trim() || `На ${planDuration}`;
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
        <div class="plan-card__price">${escapeHTML(price)}${priceRub > 0 ? `<small>за ${escapeHTML(planDuration)}</small>` : ""}</div>
        <ul class="plan-card__features"><li>${escapeHTML(formatTraffic(plan.trafficLimitBytes))}</li><li>${deviceLimit > 0 ? `До ${deviceLimit} ${plural(deviceLimit, "устройства", "устройств", "устройств")}` : "Устройства по условиям тарифа"}</li><li>Подключение после оформления</li></ul>
        <a class="button ${plan.recommended ? "button--primary" : "button--ghost"} plan-card__action" href="${buyURL}">Выбрать тариф</a>
      </article>`;
    }).join("");
  }

  function renderContacts(contacts) {
    const fingerprint = JSON.stringify(contacts);
    if (fingerprint === previousContacts) return;
    previousContacts = fingerprint;
    const safeContacts = Array.isArray(contacts) ? contacts : [];
    const cards = [`<article class="contact-card">
      <h3>Написать обращение</h3><p>Создайте обращение в кабинете и следите за ответом в переписке.</p>
      <a class="button button--ghost contact-card__action" href="${supportURL}">Написать обращение в кабинете</a>
    </article>`];
    safeContacts.forEach((contact) => {
      const href = safeURL(contact.url);
      if (!href) return;
      const telegram = /^(t\.me|telegram\.me)$/i.test(new URL(href).hostname);
      const label = String(contact.label || "Связаться с нами");
      cards.push(`<article class="contact-card">
        <h3>${escapeHTML(label)}</h3><p>${telegram ? "Напишите нам напрямую в Telegram." : "Свяжитесь с нами удобным способом."}</p>
        <a class="button button--ghost contact-card__action" href="${escapeHTML(href)}" target="_blank" rel="noopener noreferrer">${telegram ? "Перейти в Telegram" : "Открыть контакт"}</a>
      </article>`);
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
      setColors(data.colors);
      document.documentElement.dataset.glass = data.glass === true ? "on" : "off";
      setBrand(data.brand);
      renderNodes(data.nodes, Boolean(data.nodesAvailable));
      renderPlans(data.plans);
      renderContacts(data.contacts);
    } catch {
      if (!previousNodes) $("servers-list").innerHTML = '<div class="empty-state">Не удалось загрузить серверы. Попробуйте обновить страницу.</div>';
      if (!previousPlans) $("plans-list").innerHTML = '<div class="empty-state">Не удалось загрузить тарифы. Попробуйте обновить страницу.</div>';
      if (!previousContacts) renderContacts([]);
      $("servers-updated").textContent = "Не удалось обновить данные. Повторим попытку автоматически.";
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
