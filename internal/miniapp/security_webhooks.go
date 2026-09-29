package miniapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
)

const securityWebhookBodyLimit = 64 << 10

type remnawaveSecurityEvent struct {
	Scope     string    `json:"scope"`
	Event     string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	Data      struct {
		LoginAttempt struct {
			Username    string `json:"username"`
			IP          string `json:"ip"`
			UserAgent   string `json:"userAgent"`
			Description string `json:"description"`
		} `json:"loginAttempt"`
	} `json:"data"`
}

// Remnawave signs the exact JSON body with WEBHOOK_SECRET_HEADER using HMAC-SHA256.
func (h *Handler) handleRemnawaveSecurityWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secret := strings.TrimSpace(os.Getenv("REMNAWAVE_WEBHOOK_SECRET"))
	if len(secret) < 32 || h.telegramBot == nil || config.GetAdminTelegramId() == 0 {
		http.Error(w, "webhook unavailable", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, securityWebhookBodyLimit))
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	signature, err := hex.DecodeString(strings.TrimSpace(r.Header.Get("X-Remnawave-Signature")))
	if err != nil || len(signature) != sha256.Size {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	var event remnawaveSecurityEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if event.Scope != "service" || (event.Event != "service.login_attempt_failed" && event.Event != "service.login_attempt_success") {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if event.Timestamp.IsZero() || !event.Timestamp.After(time.Now().Add(-24*time.Hour)) || event.Timestamp.After(time.Now().Add(5*time.Minute)) {
		http.Error(w, "stale event", http.StatusBadRequest)
		return
	}
	if headerTime := strings.TrimSpace(r.Header.Get("X-Remnawave-Timestamp")); headerTime != "" {
		parsed, err := time.Parse(time.RFC3339Nano, headerTime)
		if err != nil || !parsed.Equal(event.Timestamp) {
			http.Error(w, "timestamp mismatch", http.StatusBadRequest)
			return
		}
	}
	login := event.Data.LoginAttempt
	geo := lookupSecurityGeo(r.Context(), login.IP)
	status := "✅ <b>Вход в панель Remnawave</b>"
	if event.Event == "service.login_attempt_failed" {
		status = "🚨 <b>Неудачная попытка входа в Remnawave</b>"
	}
	message := status + "\n\n" +
		"👤 Логин: <code>" + safeSecurityText(login.Username) + "</code>\n" +
		"🌐 IP: <code>" + safeSecurityText(login.IP) + "</code>\n" +
		"📍 Гео: " + safeSecurityText(geo) + "\n" +
		"💻 Устройство: " + safeSecurityText(describeSecurityDevice(login.UserAgent)) + "\n" +
		"🕒 Время: " + formatSecurityTime(event.Timestamp)
	if strings.TrimSpace(login.Description) != "" && login.Description != "–" {
		message += "\nℹ️ " + safeSecurityText(login.Description)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if err := h.sendSecurityAlert(ctx, message); err != nil {
		slog.Warn("remnawave security webhook notification failed", "error", err)
		http.Error(w, "notification failed", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) notifyGoogleRegistration(customer *database.Customer, r *http.Request) {
	if customer == nil || customer.GoogleEmail == nil || h.telegramBot == nil || config.GetAdminTelegramId() == 0 {
		return
	}
	email := *customer.GoogleEmail
	ip := publicRequestIP(r)
	agent := r.UserAgent()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		message := "🆕 <b>Регистрация через Gmail</b>\n\n" +
			"✉️ Email: <code>" + safeSecurityText(email) + "</code>\n" +
			fmt.Sprintf("👤 ID: <code>%d</code>\n", customer.ID) +
			"🌐 IP: <code>" + safeSecurityText(ip) + "</code>\n" +
			"📍 Гео: " + safeSecurityText(lookupSecurityGeo(ctx, ip)) + "\n" +
			"💻 Устройство: " + safeSecurityText(describeSecurityDevice(agent)) + "\n" +
			"🕒 Время: " + formatSecurityTime(time.Now())
		if err := h.sendSecurityAlert(ctx, message); err != nil {
			slog.Warn("google registration notification failed", "error", err)
		}
	}()
}

func (h *Handler) notifyEmailRegistration(customerID int64, email string, r *http.Request) {
	if h.telegramBot == nil || config.GetAdminTelegramId() == 0 {
		return
	}
	ip := publicRequestIP(r)
	agent := r.UserAgent()
	registeredAt := time.Now()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		message := formatEmailRegistrationAlert(email, customerID, ip, agent, lookupSecurityGeo(ctx, ip), registeredAt)
		if err := h.sendSecurityAlert(ctx, message); err != nil {
			slog.Warn("email registration notification failed", "error", err)
		}
	}()
}

func formatEmailRegistrationAlert(email string, customerID int64, ip, agent, geo string, registeredAt time.Time) string {
	return "🆕 <b>Регистрация через почту</b>\n\n" +
		"✉️ Почта: <code>" + safeSecurityText(email) + "</code>\n" +
		fmt.Sprintf("👤 ID: <code>%d</code>\n", customerID) +
		"🌐 IP: <code>" + safeSecurityText(ip) + "</code>\n" +
		"📍 Гео: " + safeSecurityText(geo) + "\n" +
		"💻 Устройство: " + safeSecurityText(describeSecurityDevice(agent)) + "\n" +
		"🕒 Время: " + formatSecurityTime(registeredAt)
}

func (h *Handler) sendSecurityAlert(ctx context.Context, text string) error {
	_, err := h.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: config.GetAdminTelegramId(), Text: text, ParseMode: models.ParseModeHTML,
	})
	return err
}

func safeSecurityText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}
	runes := []rune(value)
	if len(runes) > 160 {
		value = string(runes[:160]) + "…"
	}
	return html.EscapeString(value)
}

func formatSecurityTime(value time.Time) string {
	return value.In(time.FixedZone("MSK", 3*60*60)).Format("02.01.2006 15:04 МСК")
}

func describeSecurityDevice(agent string) string {
	a := strings.ToLower(agent)
	device := "Неизвестное устройство"
	switch {
	case strings.Contains(a, "iphone"):
		device = "iPhone"
	case strings.Contains(a, "ipad"):
		device = "iPad"
	case strings.Contains(a, "android"):
		device = "Android"
	case strings.Contains(a, "windows"):
		device = "Windows"
	case strings.Contains(a, "mac os") || strings.Contains(a, "macintosh"):
		device = "macOS"
	case strings.Contains(a, "linux"):
		device = "Linux"
	}
	browser := ""
	switch {
	case strings.Contains(a, "edg/"):
		browser = "Edge"
	case strings.Contains(a, "firefox/"):
		browser = "Firefox"
	case strings.Contains(a, "chrome/"):
		browser = "Chrome"
	case strings.Contains(a, "safari/"):
		browser = "Safari"
	}
	if browser != "" {
		return device + " · " + browser
	}
	return device
}

func lookupSecurityGeo(ctx context.Context, ipString string) string {
	ip := net.ParseIP(strings.TrimSpace(ipString))
	if ip == nil {
		return "не определено"
	}
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return "локальная сеть"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipwho.is/"+ip.String(), nil)
	if err != nil {
		return "не определено"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "не определено"
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "не определено"
	}
	var location struct {
		Success bool   `json:"success"`
		Country string `json:"country"`
		City    string `json:"city"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<10)).Decode(&location); err != nil || !location.Success {
		return "не определено"
	}
	if location.City != "" && location.Country != "" {
		return location.Country + ", " + location.City + " (примерно)"
	}
	if location.Country != "" {
		return location.Country + " (примерно)"
	}
	return "не определено"
}
