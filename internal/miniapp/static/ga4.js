(() => {
  const id = document.querySelector('meta[name="ga4-measurement-id"]')?.content?.trim();
  if (!/^G-[A-Z0-9]+$/.test(id || "")) return;

  window.dataLayer = window.dataLayer || [];
  window.gtag = function () { window.dataLayer.push(arguments); };
  window.gtag("js", new Date());
  // Telegram and browser login data may live in URL parameters. Never send them to GA4.
  const pageLocation = `${location.origin}${location.pathname}`;
  window.gtag("set", { page_location: pageLocation });
  window.gtag("config", id, { send_page_view: false, page_location: pageLocation });
  window.gtag("event", "page_view", { page_location: pageLocation, page_title: document.title });

  const script = document.createElement("script");
  script.async = true;
  script.src = `https://www.googletagmanager.com/gtag/js?id=${encodeURIComponent(id)}`;
  document.head.appendChild(script);
})();
