package miniapp

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateLimitRule struct {
	Limit  int
	Window time.Duration
}

type rateLimitEntry struct {
	Count   int
	ResetAt time.Time
}

type requestRateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateLimitEntry
}

func newRequestRateLimiter() *requestRateLimiter {
	return &requestRateLimiter{
		entries: make(map[string]rateLimitEntry),
	}
}

func (r *requestRateLimiter) Allow(key string, rule rateLimitRule, now time.Time) bool {
	if r == nil || rule.Limit <= 0 || rule.Window <= 0 || strings.TrimSpace(key) == "" {
		return true
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.prune(now)

	entry, ok := r.entries[key]
	if !ok || !now.Before(entry.ResetAt) {
		r.entries[key] = rateLimitEntry{
			Count:   1,
			ResetAt: now.Add(rule.Window),
		}
		return true
	}

	if entry.Count >= rule.Limit {
		return false
	}

	entry.Count++
	r.entries[key] = entry
	return true
}

func (r *requestRateLimiter) prune(now time.Time) {
	if len(r.entries) < 1024 {
		return
	}
	for key, entry := range r.entries {
		if !now.Before(entry.ResetAt) {
			delete(r.entries, key)
		}
	}
}

func miniAppRateLimitRule(path string) rateLimitRule {
	switch path {
	case "/api/mini-app/bootstrap":
		return rateLimitRule{Limit: 90, Window: time.Minute}
	case "/api/mini-app/support/refresh", "/api/mini-app/support/thread":
		return rateLimitRule{Limit: 90, Window: time.Minute}
	case "/api/mini-app/promocode/apply":
		return rateLimitRule{Limit: 30, Window: time.Minute}
	case "/api/mini-app/trial/activate":
		return rateLimitRule{Limit: 6, Window: 10 * time.Minute}
	case "/api/mini-app/purchase", "/api/mini-app/purchase/cancel":
		return rateLimitRule{Limit: 12, Window: time.Minute}
	case "/api/mini-app/payments/autopay", "/api/mini-app/payments/remove-method":
		return rateLimitRule{Limit: 20, Window: time.Minute}
	case "/api/mini-app/devices/delete":
		return rateLimitRule{Limit: 20, Window: time.Minute}
	case "/api/mini-app/reviews/create":
		return rateLimitRule{Limit: 5, Window: time.Hour}
	case "/api/mini-app/admin/reviews/delete":
		return rateLimitRule{Limit: 20, Window: time.Minute}
	case "/api/mini-app/support/create":
		return rateLimitRule{Limit: 12, Window: 10 * time.Minute}
	case "/api/mini-app/support/send":
		return rateLimitRule{Limit: 45, Window: time.Minute}
	case "/api/mini-app/support/close":
		return rateLimitRule{Limit: 20, Window: time.Minute}
	case "/api/mini-app/admin/promocodes/create", "/api/mini-app/admin/promocodes/delete":
		return rateLimitRule{Limit: 20, Window: time.Minute}
	default:
		return rateLimitRule{}
	}
}

func rateLimitKey(path string, telegramID int64) string {
	return fmt.Sprintf("%s:%d", path, telegramID)
}

func setCommonSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
}

func setHTMLSecurityHeaders(w http.ResponseWriter) {
	setCommonSecurityHeaders(w)
	w.Header().Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'",
		"base-uri 'self'",
		"object-src 'none'",
		"frame-ancestors 'self' https://web.telegram.org https://*.telegram.org https://t.me",
		"frame-src 'self' https://oauth.telegram.org https://telegram.org https://accounts.google.com",
		"child-src 'self' https://oauth.telegram.org https://telegram.org https://accounts.google.com",
		"worker-src 'self'",
		"form-action 'self'",
		"script-src 'self' https://telegram.org https://accounts.google.com",
		"style-src 'self' 'unsafe-inline'",
		"font-src 'self' data:",
		"img-src 'self' https: data: blob:",
		"media-src 'self' blob:",
		"connect-src 'self' https://telegram.org https://*.telegram.org https://oauth.telegram.org https://accounts.google.com https://oauth2.googleapis.com",
		"manifest-src 'self'",
	}, "; "))
}

func setAPIHeaders(w http.ResponseWriter) {
	setCommonSecurityHeaders(w)
	w.Header().Set("Cache-Control", "no-store")
}

func setStaticHeaders(w http.ResponseWriter, r *http.Request) {
	setCommonSecurityHeaders(w)
	switch {
	case strings.HasSuffix(r.URL.Path, "/sw.js"):
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Service-Worker-Allowed", "/mini-app/")
	case strings.HasSuffix(r.URL.Path, ".js"),
		strings.HasSuffix(r.URL.Path, ".css"),
		strings.HasSuffix(r.URL.Path, ".woff2"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case strings.HasSuffix(r.URL.Path, ".html"):
		w.Header().Set("Cache-Control", "no-store")
	case strings.HasSuffix(r.URL.Path, ".webmanifest"):
		w.Header().Set("Cache-Control", "public, max-age=3600")
	default:
		w.Header().Set("Cache-Control", "public, max-age=604800, stale-while-revalidate=86400")
	}
}
