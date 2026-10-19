package miniapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"link-bot/internal/appupdate"
	"link-bot/internal/database"
	"link-bot/internal/updater"

	"github.com/jackc/pgx/v4/pgxpool"
)

const latestReleasePage = "https://github.com/bruhxax/Link-Bot/releases/latest"
const remnawaveLatestReleaseAPI = "https://api.github.com/repos/remnawave/panel/releases/latest"
const remnawaveLatestReleasePage = "https://github.com/remnawave/panel/releases/latest"

var releaseVersionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

type adminStatusInfo struct {
	startedAt             time.Time
	version               string
	commit                string
	buildDate             string
	database              *pgxpool.Pool
	updateMu              sync.Mutex
	updateCheckedAt       time.Time
	update                adminUpdateStatus
	panelReleaseMu        sync.Mutex
	panelReleaseCheckedAt time.Time
	panelLatestVersion    string
	panelReleaseOK        bool
}

type adminServiceStatus struct {
	State            string `json:"state"`
	LatencyMS        int64  `json:"latencyMs,omitempty"`
	UptimeSeconds    int64  `json:"uptimeSeconds,omitempty"`
	MemoryUsedBytes  int64  `json:"memoryUsedBytes,omitempty"`
	MemoryTotalBytes int64  `json:"memoryTotalBytes,omitempty"`
}

type adminUpdateStatus struct {
	State         string `json:"state"`
	Kind          string `json:"kind,omitempty"`
	LatestVersion string `json:"latestVersion,omitempty"`
	AheadBy       int    `json:"aheadBy,omitempty"`
	URL           string `json:"url,omitempty"`
	CheckedAt     string `json:"checkedAt,omitempty"`
	LatestCommit  string `json:"latestCommit,omitempty"`
	Summary       string `json:"summary,omitempty"`
}

type adminPanelUpdateStatus struct {
	State          string `json:"state"`
	CurrentVersion string `json:"currentVersion,omitempty"`
	LatestVersion  string `json:"latestVersion,omitempty"`
	URL            string `json:"url"`
}

type adminStatusPayload struct {
	CheckedAt    string                 `json:"checkedAt"`
	Bot          adminServiceStatus     `json:"bot"`
	Panel        adminServiceStatus     `json:"panel"`
	Database     adminServiceStatus     `json:"database"`
	Version      string                 `json:"version"`
	Commit       string                 `json:"commit,omitempty"`
	BuildDate    string                 `json:"buildDate,omitempty"`
	HeapBytes    uint64                 `json:"heapBytes"`
	ProcessBytes uint64                 `json:"processBytes"`
	Goroutines   int                    `json:"goroutines"`
	Update       adminUpdateStatus      `json:"update"`
	PanelUpdate  adminPanelUpdateStatus `json:"panelUpdate"`
	Updater      *updater.State         `json:"updater,omitempty"`
	CanUpdate    bool                   `json:"canUpdate"`
}

func (h *Handler) SetStatusInfo(version, commit, buildDate string, startedAt time.Time, pool *pgxpool.Pool) {
	h.adminStatus.version = strings.TrimSpace(version)
	h.adminStatus.commit = strings.TrimSpace(commit)
	h.adminStatus.buildDate = strings.TrimSpace(buildDate)
	h.adminStatus.startedAt = startedAt
	h.adminStatus.database = pool
}

func (h *Handler) handleAdminStatus(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.isAdministrator() {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	checkedAt := time.Now().UTC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	result := adminStatusPayload{
		CheckedAt:    checkedAt.Format(time.RFC3339),
		Bot:          adminServiceStatus{State: "unknown"},
		Panel:        adminServiceStatus{State: "unknown"},
		Database:     adminServiceStatus{State: "unknown"},
		Version:      h.adminStatus.version,
		Commit:       h.adminStatus.commit,
		BuildDate:    h.adminStatus.buildDate,
		HeapBytes:    memory.HeapAlloc,
		ProcessBytes: memory.Sys,
		Goroutines:   runtime.NumGoroutine(),
	}
	if result.Version == "" {
		result.Version = "dev"
	}
	if !h.adminStatus.startedAt.IsZero() {
		result.Bot.UptimeSeconds = int64(time.Since(h.adminStatus.startedAt).Seconds())
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	var group sync.WaitGroup
	var panelVersion, panelLatestVersion string
	var panelReleaseOK bool
	group.Add(5)
	go func() {
		defer group.Done()
		if h.telegramBot == nil {
			return
		}
		started := time.Now()
		probeCtx, done := context.WithTimeout(ctx, 4*time.Second)
		defer done()
		_, err := h.telegramBot.GetMe(probeCtx)
		result.Bot.LatencyMS = time.Since(started).Milliseconds()
		result.Bot.State = "online"
		if err != nil {
			result.Bot.State = "offline"
		}
	}()
	go func() {
		defer group.Done()
		if h.remnawaveClient == nil {
			return
		}
		started := time.Now()
		probeCtx, done := context.WithTimeout(ctx, 4*time.Second)
		defer done()
		err := h.remnawaveClient.Ping(probeCtx)
		result.Panel.LatencyMS = time.Since(started).Milliseconds()
		result.Panel.State = "online"
		if err != nil {
			result.Panel.State = "offline"
			return
		}
		if version, versionErr := h.remnawaveClient.GetVersion(probeCtx); versionErr == nil {
			panelVersion = version
		}
		if stats, statsErr := h.remnawaveClient.GetSystemStats(probeCtx); statsErr == nil {
			result.Panel.UptimeSeconds = stats.UptimeSeconds
			result.Panel.MemoryUsedBytes = stats.MemoryUsedBytes
			result.Panel.MemoryTotalBytes = stats.MemoryTotalBytes
		}
	}()
	go func() {
		defer group.Done()
		if h.adminStatus.database == nil {
			return
		}
		started := time.Now()
		probeCtx, done := context.WithTimeout(ctx, 3*time.Second)
		defer done()
		err := h.adminStatus.database.Ping(probeCtx)
		result.Database.LatencyMS = time.Since(started).Milliseconds()
		result.Database.State = "online"
		if err != nil {
			result.Database.State = "offline"
		}
	}()
	go func() {
		defer group.Done()
		result.Update = h.checkAvailableUpdate(ctx, result.Version, result.Commit, r.URL.Query().Get("refresh") == "1")
	}()
	go func() {
		defer group.Done()
		panelLatestVersion, panelReleaseOK = h.checkPanelLatestRelease(ctx, r.URL.Query().Get("refresh") == "1")
	}()
	if sess.access().IsOwner {
		result.CanUpdate = true
		group.Add(1)
		go func() {
			defer group.Done()
			status, err := (updater.Client{}).Request(ctx, false)
			if err != nil {
				status.Message = err.Error()
			}
			result.Updater = &status
		}()
	}
	group.Wait()
	result.PanelUpdate = resolvePanelUpdateStatus(panelVersion, panelLatestVersion, panelReleaseOK)
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": result})
}

func (h *Handler) checkPanelLatestRelease(ctx context.Context, force bool) (string, bool) {
	info := &h.adminStatus
	info.panelReleaseMu.Lock()
	defer info.panelReleaseMu.Unlock()
	cacheTTL := 5 * time.Minute
	if !info.panelReleaseOK {
		cacheTTL = time.Minute
	}
	if !force && !info.panelReleaseCheckedAt.IsZero() && time.Since(info.panelReleaseCheckedAt) < cacheTTL {
		return info.panelLatestVersion, info.panelReleaseOK
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	version, ok := fetchGitHubRelease(probeCtx, &http.Client{Timeout: 4 * time.Second}, remnawaveLatestReleaseAPI)
	info.panelLatestVersion = version
	info.panelReleaseOK = ok
	info.panelReleaseCheckedAt = time.Now()
	return version, ok
}

func resolvePanelUpdateStatus(currentVersion, latestVersion string, latestOK bool) adminPanelUpdateStatus {
	result := adminPanelUpdateStatus{
		State: "unknown", CurrentVersion: strings.TrimSpace(currentVersion),
		LatestVersion: strings.TrimSpace(latestVersion), URL: remnawaveLatestReleasePage,
	}
	if !latestOK {
		return result
	}
	if newer, comparable := isNewerRelease(result.CurrentVersion, result.LatestVersion); comparable {
		result.State = "current"
		if newer {
			result.State = "available"
		}
	}
	return result
}

func (h *Handler) checkAvailableUpdate(ctx context.Context, currentVersion, currentCommit string, force bool) adminUpdateStatus {
	info := &h.adminStatus
	info.updateMu.Lock()
	defer info.updateMu.Unlock()
	cacheTTL := 5 * time.Minute
	if info.update.State == "unknown" {
		cacheTTL = time.Minute
	}
	if !force && time.Since(info.updateCheckedAt) < cacheTTL && !info.updateCheckedAt.IsZero() {
		return info.update
	}
	result := adminUpdateStatus{State: "unknown", URL: latestReleasePage, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 4 * time.Second}
	currentCommit = strings.TrimSpace(currentCommit)
	checked := appupdate.Check(probeCtx, client, currentVersion, currentCommit)
	result = adminUpdateStatus{State: checked.State, Kind: checked.Kind, LatestVersion: checked.LatestVersion, LatestCommit: checked.LatestCommit, Summary: checked.Summary, AheadBy: checked.AheadBy, URL: checked.URL, CheckedAt: checked.CheckedAt}
	info.update = result
	info.updateCheckedAt = time.Now()
	return result
}

func fetchGitHubStatusJSON(ctx context.Context, client *http.Client, url string, maxBytes int64, target any) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "Link-Bot-status")
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	return json.NewDecoder(io.LimitReader(response.Body, maxBytes)).Decode(target) == nil
}

func fetchGitHubRelease(ctx context.Context, client *http.Client, url string) (string, bool) {
	var release struct {
		TagName string `json:"tag_name"`
	}
	if !fetchGitHubStatusJSON(ctx, client, url, 16<<10, &release) {
		return "", false
	}
	return strings.TrimSpace(release.TagName), release.TagName != ""
}

func isNewerRelease(current, latest string) (bool, bool) {
	a := releaseVersionPattern.FindStringSubmatch(strings.TrimSpace(current))
	b := releaseVersionPattern.FindStringSubmatch(strings.TrimSpace(latest))
	if len(a) != 4 || len(b) != 4 {
		return false, false
	}
	for i := 1; i < 4; i++ {
		currentPart, _ := strconv.Atoi(a[i])
		latestPart, _ := strconv.Atoi(b[i])
		if latestPart != currentPart {
			return latestPart > currentPart, true
		}
	}
	return false, true
}
