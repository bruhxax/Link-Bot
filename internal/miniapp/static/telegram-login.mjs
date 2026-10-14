const TELEGRAM_ORIGIN = "https://oauth.telegram.org";

// Telegram's post_message flow, as used by its official Login library.
// Build the URL here so a cached third-party SDK cannot drop redirect_uri.
export function createTelegramLogin(browser = window) {
  let cancel = () => {};

  return {
    auth({client_id, lang}, callback) {
      if (!/^\d+$/.test(String(client_id)) || Number(client_id) <= 0) {
        throw new Error("Telegram client ID is missing");
      }
      cancel();
      const url = new URL("/auth", TELEGRAM_ORIGIN);
      url.searchParams.set("response_type", "post_message");
      url.searchParams.set("client_id", String(client_id));
      // Keep one registered return URL for all cabinet routes and plan links.
      url.searchParams.set("redirect_uri", `${browser.location.origin}/mini-app/`);
      url.searchParams.set("scope", "openid profile telegram:bot_access");
      if (lang) url.searchParams.set("lang", lang);

      // Open synchronously during the click; awaiting a script can block popups.
      const popup = browser.open(url.href, "telegram_oidc_login", "width=550,height=650,menubar=0,toolbar=0");
      if (!popup) throw new Error("Telegram popup was blocked");

      let timer;
      let finished = false;
      const cleanup = () => {
        finished = true;
        browser.removeEventListener("message", onMessage);
        browser.clearTimeout(timer);
        try { popup.close(); } catch { /* The provider may already have closed it. */ }
      };
      const finish = (payload) => {
        if (finished) return;
        cleanup();
        callback(payload);
      };
      const onMessage = (event) => {
        if (event.origin !== TELEGRAM_ORIGIN || event.source !== popup) return;
        let data;
        try { data = typeof event.data === "string" ? JSON.parse(event.data) : event.data; }
        catch { return; }
        if (data?.event !== "auth_result") return;
        if (data.error) return finish({error: String(data.error)});
        if (typeof data.result !== "string" || !data.result.trim()) return finish({error: "missing id_token"});
        // The server verifies the token signature, issuer, audience and expiry.
        finish({id_token: data.result});
      };
      const expiresAt = Date.now() + 5 * 60 * 1000;
      const checkClosed = () => {
        if (finished) return;
        if (popup.closed) return finish({error: "popup_closed"});
        if (Date.now() >= expiresAt) return finish({error: "popup_timeout"});
        timer = browser.setTimeout(checkClosed, 250);
      };
      cancel = cleanup;
      browser.addEventListener("message", onMessage);
      timer = browser.setTimeout(checkClosed, 250);
      popup.focus?.();
    },
  };
}
