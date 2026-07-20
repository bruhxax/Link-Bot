package miniapp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/utils"
)

const googleLinkStateTTL = 15 * time.Minute

type googleLinkStartResponse struct {
	AuthURL string `json:"authUrl"`
}

type googleLinkCompleteRequest struct {
	State         string `json:"state"`
	GoogleIDToken string `json:"googleIdToken"`
}

type googleLinkState struct {
	CustomerID int64  `json:"customerId"`
	TelegramID int64  `json:"telegramId"`
	ExpiresAt  int64  `json:"exp"`
	Nonce      string `json:"nonce"`
}

func (h *Handler) handleStartGoogleLink(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	clientID := strings.TrimSpace(config.GoogleClientID())
	if clientID == "" {
		h.writeError(w, http.StatusServiceUnavailable, "google_not_configured", "Gmail login is not configured")
		return
	}

	nonce, err := randomURLToken(24)
	if err != nil {
		slog.Error("mini app: google link nonce failed", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusInternalServerError, "google_link_failed", "Could not link Gmail")
		return
	}

	stateToken, err := signGoogleLinkState(googleLinkState{
		CustomerID: customer.ID,
		TelegramID: sess.User.ID,
		ExpiresAt:  time.Now().UTC().Add(googleLinkStateTTL).Unix(),
		Nonce:      nonce,
	})
	if err != nil {
		slog.Error("mini app: google link state failed", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusInternalServerError, "google_link_failed", "Could not link Gmail")
		return
	}

	authURL, err := buildGoogleLinkAuthURL(r, clientID, stateToken, nonce)
	if err != nil {
		slog.Error("mini app: google link url failed", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusInternalServerError, "google_link_failed", "Could not link Gmail")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": googleLinkStartResponse{
			AuthURL: authURL,
		},
	})
}

func (h *Handler) handleCompleteGoogleLink(w http.ResponseWriter, r *http.Request) {
	setAPIHeaders(w)
	if h.runtimeSettings != nil && !h.runtimeSettings.FeatureEnabled("google") {
		h.writeError(w, http.StatusServiceUnavailable, "feature_disabled", "Gmail login is disabled")
		return
	}

	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if contentType := strings.TrimSpace(r.Header.Get("Content-Type")); contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		h.writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "JSON required")
		return
	}

	var req googleLinkCompleteRequest
	if err := h.decodeJSONRequest(w, r, 1<<20, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}

	linkState, err := parseGoogleLinkState(req.State)
	if err != nil {
		h.writeError(w, http.StatusUnauthorized, "google_invalid_state", "Gmail link expired")
		return
	}

	identity, err := validateGoogleIDToken(r.Context(), req.GoogleIDToken, config.GoogleClientID())
	if err != nil {
		slog.Warn("mini app: google callback validation failed", "error", err, "telegramId", utils.MaskHalfInt64(linkState.TelegramID))
		if errors.Is(err, errGoogleAuthNotConfigured) {
			h.writeError(w, http.StatusServiceUnavailable, "google_not_configured", "Gmail login is not configured")
			return
		}
		h.writeError(w, http.StatusUnauthorized, "google_invalid", "Gmail authorization failed")
		return
	}
	if identity.Nonce != "" && !hmac.Equal([]byte(identity.Nonce), []byte(linkState.Nonce)) {
		h.writeError(w, http.StatusUnauthorized, "google_invalid", "Gmail authorization failed")
		return
	}

	customer, err := h.customerRepository.FindById(r.Context(), linkState.CustomerID)
	if err != nil || customer == nil || customer.TelegramID != linkState.TelegramID {
		slog.Warn("mini app: google callback customer not found", "error", err, "telegramId", utils.MaskHalfInt64(linkState.TelegramID))
		h.writeError(w, http.StatusUnauthorized, "google_invalid_state", "Gmail link expired")
		return
	}

	customer, err = h.linkGoogleIdentity(r.Context(), customer, identity)
	if err != nil {
		slog.Error("mini app: google callback link failed", "error", err, "telegramId", utils.MaskHalfInt64(linkState.TelegramID))
		if errors.Is(err, errGoogleAlreadyLinked) {
			h.writeError(w, http.StatusConflict, "google_already_linked", "This Gmail is already linked")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "google_link_failed", "Could not link Gmail")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Gmail linked",
		"email":   customerGoogleEmail(customer),
	})
}

func (h *Handler) serveGoogleLinkCallback(w http.ResponseWriter, r *http.Request) {
	if h.runtimeSettings != nil && !h.runtimeSettings.FeatureEnabled("google") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	setGoogleCallbackHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(googleLinkCallbackHTML()))
}

func buildGoogleLinkAuthURL(r *http.Request, clientID, stateToken, nonce string) (string, error) {
	callbackURL, err := googleLinkCallbackURL(r)
	if err != nil {
		return "", err
	}

	authURL := url.URL{
		Scheme: "https",
		Host:   "accounts.google.com",
		Path:   "/o/oauth2/v2/auth",
	}
	query := authURL.Query()
	query.Set("client_id", clientID)
	query.Set("redirect_uri", callbackURL)
	query.Set("response_type", "id_token")
	query.Set("scope", "openid email profile")
	query.Set("state", stateToken)
	query.Set("nonce", nonce)
	query.Set("prompt", "select_account")
	authURL.RawQuery = query.Encode()
	return authURL.String(), nil
}

func googleLinkCallbackURL(r *http.Request) (string, error) {
	base := strings.TrimSpace(config.GetMiniAppURL())
	if base == "" {
		scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
		if scheme == "" {
			if r.TLS != nil {
				scheme = "https"
			} else {
				scheme = "http"
			}
		}
		host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
		if host == "" {
			host = strings.TrimSpace(r.Host)
		}
		base = scheme + "://" + host + "/mini-app/"
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid mini app url")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/google/callback"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func signGoogleLinkState(state googleLinkState) (string, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := signGoogleLinkBytes([]byte(encodedPayload))
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func parseGoogleLinkState(token string) (googleLinkState, error) {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return googleLinkState{}, fmt.Errorf("invalid google link state")
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return googleLinkState{}, err
	}
	expected := signGoogleLinkBytes([]byte(parts[0]))
	if !hmac.Equal(signature, expected) {
		return googleLinkState{}, fmt.Errorf("invalid google link state signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return googleLinkState{}, err
	}

	var state googleLinkState
	if err := json.Unmarshal(payload, &state); err != nil {
		return googleLinkState{}, err
	}
	if state.CustomerID <= 0 || state.TelegramID <= 0 || state.ExpiresAt <= time.Now().UTC().Unix() || state.Nonce == "" {
		return googleLinkState{}, fmt.Errorf("expired google link state")
	}
	return state, nil
}

func signGoogleLinkBytes(payload []byte) []byte {
	secret := []byte(config.TelegramToken())
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func randomURLToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func setGoogleCallbackHeaders(w http.ResponseWriter) {
	setCommonSecurityHeaders(w)
	w.Header().Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'",
		"base-uri 'self'",
		"object-src 'none'",
		"frame-ancestors 'self' https://web.telegram.org https://*.telegram.org https://t.me",
		"script-src 'unsafe-inline'",
		"style-src 'unsafe-inline'",
		"connect-src 'self'",
		"img-src 'self' data:",
	}, "; "))
}

func googleLinkCallbackHTML() string {
	title := html.EscapeString("Link-Bot")
	return `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <meta name="theme-color" content="#000000">
  <title>` + title + `</title>
  <style>
    :root { color-scheme: dark; font-family: Inter, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #000; color: #fff; }
    * { box-sizing: border-box; }
    body { min-height: 100vh; margin: 0; display: grid; place-items: center; padding: 24px; background: #000; overflow: hidden; }
    body::before { content: ""; position: fixed; inset: -20%; background: radial-gradient(circle at 50% 35%, rgba(255,255,255,.11), transparent 30%), repeating-linear-gradient(105deg, rgba(255,255,255,.13) 0 2px, transparent 2px 18px); opacity: .42; transform: rotate(-6deg); }
    .card { position: relative; width: min(100%, 390px); padding: 28px 24px; border: 1px solid rgba(255,255,255,.16); border-radius: 12px; background: #080a0f; box-shadow: 0 24px 70px rgba(0,0,0,.46); text-align: center; }
    .mark { width: 52px; height: 52px; margin: 0 auto 16px; display: grid; place-items: center; border-radius: 12px; background: #000; }
    .mark img { width: 42px; height: 42px; object-fit: cover; border-radius: 9px; }
    .eyebrow { margin-bottom: 8px; color: rgba(255,255,255,.56); font-size: 11px; font-weight: 800; text-transform: uppercase; letter-spacing: .04em; }
    h1 { margin: 0; font-size: 24px; line-height: 1.15; letter-spacing: 0; }
    p { margin: 12px 0 0; color: rgba(255,255,255,.68); font-size: 14px; line-height: 1.55; }
    .spinner { width: 26px; height: 26px; margin: 18px auto 0; border: 3px solid rgba(255,255,255,.18); border-top-color: #fff; border-radius: 50%; animation: spin .8s linear infinite; }
    .status-icon { width: 34px; height: 34px; margin: 18px auto 0; display: none; border-radius: 50%; border: 1px solid rgba(255,255,255,.24); align-items: center; justify-content: center; font-weight: 900; }
    .actions { margin-top: 18px; display: none; gap: 10px; }
    a { min-height: 42px; display: inline-flex; align-items: center; justify-content: center; padding: 0 16px; border: 1px solid rgba(255,255,255,.24); border-radius: 8px; color: #fff; text-decoration: none; font-size: 13px; font-weight: 800; }
    body.done .spinner { display: none; }
    body.done .status-icon { display: inline-flex; }
    body.done .actions { display: grid; }
    body.success .status-icon { color: #fff; background: rgba(255,255,255,.08); }
    body.error .status-icon { color: #fff; background: rgba(255,255,255,.05); }
    @keyframes spin { to { transform: rotate(360deg); } }
  </style>
</head>
<body>
  <main class="card" aria-live="polite">
    <div class="mark"><img src="/mini-app/assets/brand-mark.png" alt="Link-Bot"></div>
    <div class="eyebrow">Link-Bot</div>
    <h1 id="title">Привязываем Gmail</h1>
    <p id="text">Подождите, завершаем авторизацию Google.</p>
    <div class="spinner" aria-hidden="true"></div>
    <div class="status-icon" id="icon" aria-hidden="true">✓</div>
    <div class="actions"><a href="/mini-app/">Открыть Link-Bot</a></div>
  </main>
  <script>
    (async function () {
      const title = document.getElementById("title");
      const text = document.getElementById("text");
      const icon = document.getElementById("icon");
      function finish(kind, heading, message, mark) {
        document.body.classList.add("done", kind);
        title.textContent = heading;
        text.textContent = message;
        icon.textContent = mark;
        try { history.replaceState(null, "", "/mini-app/google/callback"); } catch (_) {}
      }
      const params = new URLSearchParams(String(location.hash || "").replace(/^#/, ""));
      const error = params.get("error");
      const idToken = params.get("id_token");
      const state = params.get("state");
      if (error) return finish("error", "Не удалось привязать Gmail", "Google вернул ошибку авторизации. Вернитесь в Telegram и попробуйте ещё раз.", "!");
      if (!idToken || !state) return finish("error", "Не удалось привязать Gmail", "Ответ Google неполный. Вернитесь в Telegram и попробуйте ещё раз.", "!");
      try {
        const response = await fetch("/api/mini-app/auth/google/link/complete", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ googleIdToken: idToken, state })
        });
        const payload = await response.json().catch(() => null);
        if (!response.ok || !payload || !payload.ok) {
          const message = payload && payload.error && (payload.error.message || payload.error.code);
          throw new Error(message || "link_failed");
        }
        finish("success", "Gmail привязан", "Вернитесь в Telegram, mini app обновит способ входа автоматически.", "✓");
      } catch (err) {
        finish("error", "Не удалось привязать Gmail", "Почта уже привязана к другому аккаунту или ссылка устарела. Вернитесь в Telegram и попробуйте ещё раз.", "!");
      }
    })();
  </script>
</body>
</html>`
}
