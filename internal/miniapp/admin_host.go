package miniapp

import (
	"net/http"
	"net/url"
	"strings"
)

// Use the actual request host, never an untrusted X-Forwarded-Host header.
func (h *Handler) isAdminHost(r *http.Request) bool {
	if h.adminBaseURL == "" {
		return false
	}
	origin, err := url.Parse(h.adminBaseURL)
	return err == nil && strings.EqualFold(r.Host, origin.Host)
}

func (h *Handler) redirectToAdmin(w http.ResponseWriter, r *http.Request) {
	target, _ := url.Parse(h.adminBaseURL + "/")
	query := url.Values{"page": {"admin"}}
	// Never forward login tokens or arbitrary redirect URLs between origins.
	if section := r.URL.Query().Get("section"); section != "" {
		query.Set("section", section)
	}
	target.RawQuery = query.Encode()
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (h *Handler) serveAdminEntry(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" && r.URL.Path != "/admin/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.adminBaseURL != "" && !h.isAdminHost(r) {
		h.redirectToAdmin(w, r)
		return
	}
	if h.adminBaseURL == "" {
		query := url.Values{"page": {"admin"}}
		if section := r.URL.Query().Get("section"); section != "" {
			query.Set("section", section)
		}
		http.Redirect(w, r, "/mini-app/?"+query.Encode(), http.StatusFound)
		return
	}
	h.serveIndex(w, r)
}
