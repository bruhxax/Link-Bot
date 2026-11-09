package miniapp

import (
	"context"
	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"
)

type landingProtectionPayload struct {
	Enabled         bool   `json:"enabled"`
	State           string `json:"state"`
	Text            string `json:"text"`
	DurationSeconds int    `json:"durationSeconds"`
}

// Only the deployment's private reverse proxy may provide a forwarding chain.
// Walking from the right prevents a client-supplied leftmost header from claiming
// a protected address. Direct public requests use the TCP peer address.
func landingVisitorIP(r *http.Request) string {
	peer := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	addr, err := netip.ParseAddr(peer)
	if err != nil {
		return ""
	}
	addr = addr.Unmap()
	trusted := func(a netip.Addr) bool { return a.IsLoopback() || a.IsPrivate() }
	if !trusted(addr) {
		return addr.String()
	}
	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if len(forwarded) > 32 {
		return ""
	}
	if len(forwarded) == 1 && strings.TrimSpace(forwarded[0]) == "" {
		return addr.String()
	}
	for i := len(forwarded) - 1; i >= 0; i-- {
		hop, e := netip.ParseAddr(strings.TrimSpace(forwarded[i]))
		if e != nil || hop.Zone() != "" {
			return ""
		}
		addr = hop.Unmap()
		if !trusted(addr) {
			return addr.String()
		}
	}
	return addr.String()
}
func protectionForRequest(settings runtimeconfig.Settings, r *http.Request) landingProtectionPayload {
	cfg := settings.Interactive
	out := landingProtectionPayload{Enabled: cfg.Enabled, State: "unprotected", DurationSeconds: cfg.DurationSeconds}
	if !cfg.Enabled {
		return out
	}
	ip := landingVisitorIP(r)
	if ip == "" {
		out.Enabled = false
		return out
	}
	out.Text = cfg.UnprotectedText
	if cfg.ContainsIP(ip) {
		out.State = "protected"
		out.Text = cfg.ProtectedText
	}
	brand := strings.TrimSpace(settings.Content.BrandName)
	if brand == "" {
		brand = "VPN"
	}
	out.Text = strings.ReplaceAll(out.Text, "{brand}", brand)
	return out
}
func (h *Handler) handleLandingProtection(w http.ResponseWriter, r *http.Request) {
	setAPIHeaders(w)
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("Vary", "X-Forwarded-For")
	if r.Method != http.MethodGet {
		h.writeError(w, 405, "method_not_allowed", "Method not allowed")
		return
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": protectionForRequest(h.landingSettings(), r)})
}
func panelAddressHost(value string) string {
	value = strings.TrimSpace(value)
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return strings.Trim(value, "[]")
}
func (h *Handler) handleInteractiveNodes(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.canAdmin("interactive") {
		h.writeError(w, 403, "forbidden", "Недостаточно прав")
		return
	}
	if r.Method != http.MethodPost {
		h.writeError(w, 405, "method_not_allowed", "Method not allowed")
		return
	}
	if h.remnawaveClient == nil {
		h.writeError(w, 503, "panel_unavailable", "Панель не подключена")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	nodes, err := h.remnawaveClient.GetNodesStatus(ctx)
	if err != nil {
		h.writeError(w, 502, "panel_unavailable", "Не удалось получить ноды из панели")
		return
	}
	ips := map[string]bool{}
	unresolved := []string{}
	seen := map[string]bool{}
	for _, node := range nodes {
		host := panelAddressHost(node.Address)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		if addr, err := netip.ParseAddr(host); err == nil && addr.Zone() == "" {
			ips[addr.Unmap().String()] = true
			continue
		}
		lookupCtx, stop := context.WithTimeout(ctx, time.Second)
		addresses, err := net.DefaultResolver.LookupNetIP(lookupCtx, "ip", host)
		stop()
		if err != nil || len(addresses) == 0 {
			unresolved = append(unresolved, host)
			continue
		}
		for _, addr := range addresses {
			ips[addr.Unmap().String()] = true
		}
	}
	result := []string{}
	for ip := range ips {
		result = append(result, ip)
	}
	sort.Strings(result)
	if len(result) > 512 {
		h.writeError(w, 400, "too_many_addresses", "Из панели получено больше 512 адресов. Выберите нужные вручную.")
		return
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"addresses": result, "unresolved": unresolved, "nodes": len(nodes)}})
}
