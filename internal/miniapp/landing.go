package miniapp

import (
	"bytes"
	"context"
	"html"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/runtimeconfig"
)

type landingNodePayload struct {
	Name        string `json:"name"`
	CountryCode string `json:"countryCode,omitempty"`
	Online      bool   `json:"online"`
}

type landingContactPayload struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type landingPayload struct {
	Brand          brandPayload            `json:"brand"`
	Colors         map[string]string       `json:"colors"`
	Glass          bool                    `json:"glass"`
	Plans          []planPayload           `json:"plans"`
	Nodes          []landingNodePayload    `json:"nodes"`
	NodesAvailable bool                    `json:"nodesAvailable"`
	Contacts       []landingContactPayload `json:"contacts"`
}

var landingTelegramUsername = regexp.MustCompile(`(?i)^[a-z0-9_]{5,32}$`)
var landingHexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

var landingCSSColors = []struct {
	name, property string
}{
	{"background", "--bg"}, {"surface", "--surface"}, {"surfaceStrong", "--surface-strong"},
	{"text", "--text"}, {"muted", "--muted"}, {"border", "--border"},
	{"accent", "--accent"}, {"success", "--success"}, {"danger", "--danger"},
}

func landingInitialTheme(colors map[string]string) (string, string) {
	var css strings.Builder
	css.WriteString(":root{")
	for _, entry := range landingCSSColors {
		color := colors[entry.name]
		if !landingHexColor.MatchString(color) {
			continue
		}
		css.WriteString(entry.property)
		css.WriteByte(':')
		css.WriteString(color)
		css.WriteByte(';')
	}
	if accent := colors["accent"]; landingHexColor.MatchString(accent) {
		linear := func(component uint64) float64 {
			value := float64(component) / 255
			if value <= .04045 {
				return value / 12.92
			}
			return math.Pow((value+.055)/1.055, 2.4)
		}
		red, _ := strconv.ParseUint(accent[1:3], 16, 8)
		green, _ := strconv.ParseUint(accent[3:5], 16, 8)
		blue, _ := strconv.ParseUint(accent[5:7], 16, 8)
		ink := "#ffffff"
		if .2126*linear(red)+.7152*linear(green)+.0722*linear(blue) > .35 {
			ink = "#111217"
		}
		css.WriteString("--accent-ink:")
		css.WriteString(ink)
		css.WriteByte(';')
	}
	css.WriteByte('}')
	background := colors["background"]
	if !landingHexColor.MatchString(background) {
		background = "#080808"
	}
	return css.String(), background
}

func (h *Handler) serveLanding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := fs.ReadFile(h.staticFS, "landing.html")
	if err != nil {
		http.Error(w, "site is unavailable", http.StatusInternalServerError)
		return
	}
	settings := h.landingSettings()
	brandName := strings.TrimSpace(settings.Content.BrandName)
	if brandName == "" {
		brandName = runtimeconfig.DefaultSettings().Content.BrandName
	}
	faviconURL := settings.Content.WebPage.FaviconURL
	if faviconURL == "/mini-app/assets/brand-mark.png" {
		faviconURL += "?v=" + h.assetVersion
	}
	initialTheme, background := landingInitialTheme(settings.Appearance.Colors)
	data = bytes.ReplaceAll(data, []byte("__BRAND_NAME__"), []byte(html.EscapeString(brandName)))
	data = bytes.ReplaceAll(data, []byte("__PAGE_DESCRIPTION__"), []byte(html.EscapeString(settings.Content.WebPage.Description)))
	data = bytes.ReplaceAll(data, []byte("__FAVICON_URL__"), []byte(html.EscapeString(faviconURL)))
	data = bytes.ReplaceAll(data, []byte("__CABINET_BASE__"), []byte(html.EscapeString(h.cabinetBaseURL)))
	data = bytes.ReplaceAll(data, []byte("__ASSET_VERSION__"), []byte(h.assetVersion))
	data = bytes.ReplaceAll(data, []byte("__INITIAL_THEME__"), []byte(initialTheme))
	data = bytes.ReplaceAll(data, []byte("__THEME_COLOR__"), []byte(background))
	glassMode := "off"
	if settings.Appearance.Glass {
		glassMode = "on"
	}
	data = bytes.ReplaceAll(data, []byte("__GLASS_MODE__"), []byte(glassMode))
	setHTMLSecurityHeaders(w)
	w.Header().Set("X-Robots-Tag", "index, follow")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
}

func (h *Handler) handleLandingData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	settings := h.landingSettings()
	plans := h.buildPlans()
	sort.SliceStable(plans, func(i, j int) bool {
		left, right := landingPlanDays(plans[i]), landingPlanDays(plans[j])
		if left != right {
			return left < right
		}
		if plans[i].PriceRub != plans[j].PriceRub {
			return plans[i].PriceRub < plans[j].PriceRub
		}
		return plans[i].ID < plans[j].ID
	})

	colors := make(map[string]string)
	for _, name := range []string{"background", "surface", "surfaceStrong", "text", "muted", "border", "accent", "success", "danger"} {
		colors[name] = settings.Appearance.Colors[name]
	}
	nodes, nodesAvailable := h.landingNodeStatus()
	h.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": landingPayload{
			Brand: brandPayload{
				Name:    settings.Content.BrandName,
				LogoURL: settings.Content.LogoURL,
			},
			Colors:         colors,
			Glass:          settings.Appearance.Glass,
			Plans:          plans,
			Nodes:          nodes,
			NodesAvailable: nodesAvailable,
			Contacts:       landingContacts(settings, h.runtimeLink("support", config.SupportURL())),
		},
	})
}

func (h *Handler) landingSettings() runtimeconfig.Settings {
	if h.runtimeSettings != nil {
		return h.runtimeSettings.Snapshot()
	}
	return runtimeconfig.DefaultSettings()
}

func landingPlanDays(plan planPayload) int {
	if plan.Days > 0 {
		return plan.Days
	}
	return plan.Months * 30
}

func landingContacts(settings runtimeconfig.Settings, supportURL string) []landingContactPayload {
	contacts := make([]landingContactPayload, 0, 2)
	username := strings.TrimPrefix(strings.TrimSpace(settings.Content.AdminContact), "@")
	if landingTelegramUsername.MatchString(username) {
		contacts = append(contacts, landingContactPayload{Label: "Telegram", URL: "https://t.me/" + username})
	}
	if parsed, err := url.Parse(strings.TrimSpace(supportURL)); err == nil && parsed.Host != "" && (parsed.Scheme == "https" || parsed.Scheme == "http") {
		duplicate := false
		for _, contact := range contacts {
			duplicate = strings.EqualFold(strings.TrimSuffix(contact.URL, "/"), strings.TrimSuffix(parsed.String(), "/"))
			if duplicate {
				break
			}
		}
		if !duplicate {
			label := "Связаться с нами"
			if strings.EqualFold(parsed.Hostname(), "t.me") || strings.EqualFold(parsed.Hostname(), "telegram.me") {
				label = "Telegram"
			}
			contacts = append(contacts, landingContactPayload{Label: label, URL: parsed.String()})
		}
	}
	return contacts
}

func (h *Handler) landingNodeStatus() ([]landingNodePayload, bool) {
	h.landingNodesMu.Lock()
	defer h.landingNodesMu.Unlock()

	now := time.Now()
	if !h.landingNodesCheckedAt.IsZero() && now.Sub(h.landingNodesCheckedAt) < 20*time.Second {
		return append([]landingNodePayload{}, h.landingNodes...), h.landingNodesAvailable
	}
	if h.remnawaveClient == nil {
		h.landingNodes = []landingNodePayload{}
		h.landingNodesAvailable = true
		h.landingNodesCheckedAt = now
		return []landingNodePayload{}, true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	servers, err := h.buildServersPayload(ctx)
	if err != nil {
		slog.Warn("landing: node status unavailable", "error", err)
		h.landingNodesAvailable = false
		h.landingNodesCheckedAt = now.Add(-10 * time.Second)
		return append([]landingNodePayload{}, h.landingNodes...), false
	}
	nodes := make([]landingNodePayload, 0, len(servers.Items))
	for index, node := range servers.Items {
		name := node.Name
		if name == "" || strings.EqualFold(name, node.Address) {
			name = "Сервер " + strconv.Itoa(index+1)
		}
		nodes = append(nodes, landingNodePayload{
			Name:        name,
			CountryCode: node.CountryCode,
			Online:      node.Online,
		})
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})
	h.landingNodes = nodes
	h.landingNodesAvailable = true
	h.landingNodesCheckedAt = now
	return append([]landingNodePayload{}, nodes...), true
}
