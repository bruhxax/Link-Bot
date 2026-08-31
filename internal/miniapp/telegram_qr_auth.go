package miniapp

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/webauth"
)

type telegramQRLoginStatusRequest struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

func (h *Handler) handleStartTelegramQRLogin(w http.ResponseWriter, r *http.Request) {
	setAPIHeaders(w)
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if !hasAcceptedContentType(r.Header.Get("Content-Type"), []string{"application/json"}) {
		h.writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "JSON required")
		return
	}
	var request struct{}
	if err := h.decodeJSONRequest(w, r, 1024, &request); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	if h.webLogin == nil {
		h.writeError(w, http.StatusServiceUnavailable, "qr_login_unavailable", "Telegram QR login is unavailable")
		return
	}
	if !h.rateLimiter.Allow("telegram-qr-start:"+publicRequestIP(r), rateLimitRule{Limit: 12, Window: time.Minute}, time.Now().UTC()) {
		h.writeError(w, http.StatusTooManyRequests, "too_many_requests", "Too many requests")
		return
	}

	botURL, err := telegramQRBotURL()
	if err != nil {
		h.writeError(w, http.StatusServiceUnavailable, "qr_login_unavailable", "Telegram QR login is unavailable")
		return
	}
	challenge, err := h.webLogin.Create(time.Now().UTC())
	if err != nil {
		slog.Error("create Telegram QR login challenge failed", "error", err)
		h.writeError(w, http.StatusServiceUnavailable, "qr_login_unavailable", "Telegram QR login is unavailable")
		return
	}
	query := botURL.Query()
	query.Set("start", webauth.StartParameter(challenge.ApprovalToken))
	botURL.RawQuery = query.Encode()

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"id":        challenge.ID,
			"secret":    challenge.Secret,
			"url":       botURL.String(),
			"expiresAt": challenge.ExpiresAt.Format(time.RFC3339),
		},
	})
}

func (h *Handler) handleTelegramQRLoginStatus(w http.ResponseWriter, r *http.Request) {
	setAPIHeaders(w)
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if !hasAcceptedContentType(r.Header.Get("Content-Type"), []string{"application/json"}) {
		h.writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "JSON required")
		return
	}
	if h.webLogin == nil {
		h.writeError(w, http.StatusServiceUnavailable, "qr_login_unavailable", "Telegram QR login is unavailable")
		return
	}
	if !h.rateLimiter.Allow("telegram-qr-status:"+publicRequestIP(r), rateLimitRule{Limit: 150, Window: time.Minute}, time.Now().UTC()) {
		h.writeError(w, http.StatusTooManyRequests, "too_many_requests", "Too many requests")
		return
	}

	var request telegramQRLoginStatusRequest
	if err := h.decodeJSONRequest(w, r, 4096, &request); err != nil || strings.TrimSpace(request.ID) == "" || strings.TrimSpace(request.Secret) == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}

	user, err := h.webLogin.Consume(strings.TrimSpace(request.ID), strings.TrimSpace(request.Secret), time.Now().UTC())
	if errors.Is(err, webauth.ErrPending) {
		h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"status": "pending"}})
		return
	}
	if errors.Is(err, webauth.ErrExpired) || errors.Is(err, webauth.ErrNotFound) {
		h.writeError(w, http.StatusGone, "qr_login_expired", "QR login expired")
		return
	}
	if errors.Is(err, webauth.ErrInvalidSecret) {
		h.writeError(w, http.StatusForbidden, "qr_login_forbidden", "Invalid QR login secret")
		return
	}
	if err != nil {
		slog.Error("consume Telegram QR login challenge failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "qr_login_failed", "Telegram QR login failed")
		return
	}

	sessionData, err := createTelegramBrowserSessionData(telegramUser{
		ID:           user.ID,
		FirstName:    user.FirstName,
		LastName:     user.LastName,
		Username:     user.Username,
		PhotoURL:     user.PhotoURL,
		LanguageCode: user.LanguageCode,
	}, config.TelegramToken(), currentMiniAppTime())
	if err != nil {
		slog.Error("create Telegram QR browser session failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "qr_login_failed", "Telegram QR login failed")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"status":      "authorized",
			"sessionData": sessionData,
		},
	})
}

func telegramQRBotURL() (*url.URL, error) {
	botURL := strings.TrimSpace(config.BotURL())
	parsed, err := url.Parse(botURL)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "t.me") || strings.Trim(parsed.Path, "/") == "" {
		return nil, errors.New("invalid Telegram bot URL")
	}
	return parsed, nil
}

func publicRequestIP(r *http.Request) string {
	for _, candidate := range []string{
		strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]),
		strings.TrimSpace(r.Header.Get("X-Real-IP")),
	} {
		if parsed := net.ParseIP(candidate); parsed != nil {
			return parsed.String()
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		if parsed := net.ParseIP(host); parsed != nil {
			return parsed.String()
		}
	}
	if parsed := net.ParseIP(strings.TrimSpace(r.RemoteAddr)); parsed != nil {
		return parsed.String()
	}
	return "unknown"
}
