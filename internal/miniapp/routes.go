package miniapp

import (
	"net/http"
	"net/url"
	"strings"
)

var cabinetRoutes = map[string]bool{
	"dashboard": true, "plans": true, "gift": true, "setup": true, "support": true,
	"faq": true, "reviews": true, "referrals": true, "partners": true, "servers": true,
	"profile": true, "media": true, "account/security": true, "payments": true,
	"terms": true, "privacy": true, "login": true, "logout": true,
}

var adminSections = map[string]bool{
	"home": true, "users": true, "subscriptions": true, "plans": true, "servers": true,
	"support": true, "reviews": true, "administrators": true, "finance": true, "analytics": true,
	"promocodes": true, "referrals": true, "partners": true, "broadcast": true, "integrations": true,
	"moynalog": true, "content": true, "appearance": true, "landing": true, "cursors": true, "layout": true, "subpage": true,
	"localization": true, "status": true, "diagnostics": true, "features": true, "trial": true,
	"grace": true, "maintenance": true, "ai": true, "push": true, "smtp": true,
}

func matchesOriginHost(raw string, r *http.Request) bool {
	origin, err := url.Parse(raw)
	return raw != "" && err == nil && strings.EqualFold(origin.Host, r.Host)
}

func (h *Handler) isCabinetHost(r *http.Request) bool { return matchesOriginHost(h.cabinetBaseURL, r) }

func validCabinetRoute(route string) bool {
	return cabinetRoutes[route] || strings.HasPrefix(route, "pages/") && len(strings.TrimPrefix(route, "pages/")) > 0 && !strings.Contains(strings.TrimPrefix(route, "pages/"), "/")
}

func (h *Handler) cabinetPageURL(page string) string {
	if h.cabinetBaseURL != "" {
		return strings.TrimRight(h.cabinetBaseURL, "/") + "/" + page
	}
	return strings.TrimRight(h.publicBaseURL, "/") + "/cabinet/" + page
}

// Only known application paths reach the SPA; unknown files and API paths stay 404.
func (h *Handler) serveAppRoute(w http.ResponseWriter, r *http.Request) bool {
	route := strings.Trim(r.URL.Path, "/")
	if h.isAdminHost(r) {
		if route == "login" || route == "logout" {
			http.Redirect(w, r, h.cabinetPageURL(route), http.StatusFound)
			return true
		}
		if route == "" || adminSections[route] {
			h.serveIndex(w, r)
			return true
		}
	}
	if h.isCabinetHost(r) && (route == "" || validCabinetRoute(route)) {
		h.serveIndex(w, r)
		return true
	}
	if route == "cabinet" || strings.HasPrefix(route, "cabinet/") && validCabinetRoute(strings.TrimPrefix(route, "cabinet/")) {
		h.serveIndex(w, r)
		return true
	}
	return false
}
