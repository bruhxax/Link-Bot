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

	"link-bot/internal/database"

	"github.com/jackc/pgx/v4/pgxpool"
)

const latestReleaseAPI = "https://api.github.com/repos/bruhxax/Link-Bot/releases/latest"
const latestReleasePage = "https://github.com/bruhxax/Link-Bot/releases/latest"

var releaseVersionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

type adminStatusInfo struct {
	startedAt       time.Time
	version         string
	commit          string
	buildDate       string
	database        *pgxpool.Pool
	updateMu        sync.Mutex
	updateCheckedAt time.Time
	update          adminUpdateStatus
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
	LatestVersion string `json:"latestVersion,omitempty"`
	URL           string `json:"url,omitempty"`
	CheckedAt     string `json:"checkedAt,omitempty"`
}

type adminStatusPayload struct {
	CheckedAt    string             `json:"checkedAt"`
	Bot          adminServiceStatus `json:"bot"`
	Panel        adminServiceStatus `json:"panel"`
	Database     adminServiceStatus `json:"database"`
	Version      string             `json:"version"`
	Commit       string             `json:"commit,omitempty"`
	BuildDate    string             `json:"buildDate,omitempty"`
	HeapBytes    uint64             `json:"heapBytes"`
	ProcessBytes uint64             `json:"processBytes"`
	Goroutines   int                `json:"goroutines"`
	Update       adminUpdateStatus  `json:"update"`
}

func (h *Handler) SetStatusInfo(version, commit, buildDate string, startedAt time.Time, pool *pgxpool.Pool) {
	h.adminStatus.version = strings.TrimSpace(version)
	h.adminStatus.commit = strings.TrimSpace(commit)
	h.adminStatus.buildDate = strings.TrimSpace(buildDate)
	h.adminStatus.startedAt = startedAt
	h.adminStatus.database = pool
}

func (h *Handler) handleAdminStatus(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
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
	group.Add(4)
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
		result.Update = h.checkLatestRelease(ctx, result.Version)
	}()
	group.Wait()
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": result})
}

func (h *Handler) checkLatestRelease(ctx context.Context, currentVersion string) adminUpdateStatus {
	info := &h.adminStatus
	info.updateMu.Lock()
	defer info.updateMu.Unlock()
	cacheTTL := 15 * time.Minute
	if info.update.State == "unknown" {
		cacheTTL = 2 * time.Minute
	}
	if time.Since(info.updateCheckedAt) < cacheTTL && !info.updateCheckedAt.IsZero() {
		return info.update
	}
	result := adminUpdateStatus{State: "unknown", URL: latestReleasePage, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, latestReleaseAPI, nil)
	if err == nil {
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("User-Agent", "Link-Bot-status")
		response, requestErr := (&http.Client{Timeout: 3 * time.Second}).Do(request)
		if requestErr == nil {
			defer response.Body.Close()
			if response.StatusCode == http.StatusOK {
				var release struct {
					TagName string `json:"tag_name"`
				}
				if json.NewDecoder(io.LimitReader(response.Body, 16<<10)).Decode(&release) == nil {
					result.LatestVersion = strings.TrimSpace(release.TagName)
					if newer, comparable := isNewerRelease(currentVersion, result.LatestVersion); comparable {
						result.State = "current"
						if newer {
							result.State = "available"
						}
					}
				}
			}
		}
	}
	info.update = result
	info.updateCheckedAt = time.Now()
	return result
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
