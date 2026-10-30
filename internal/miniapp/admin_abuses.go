package miniapp

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
)

type abuseSubscription struct {
	CustomerID     int64     `json:"customerId"`
	SubscriptionID int64     `json:"subscriptionId"`
	Username       string    `json:"username"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"createdAt"`
}
type abuseFinding struct {
	Key            string                 `json:"key"`
	Kind           string                 `json:"kind"`
	Username       string                 `json:"username"`
	CustomerID     int64                  `json:"customerId"`
	SubscriptionID int64                  `json:"subscriptionId,omitempty"`
	DeviceLimit    int                    `json:"deviceLimit,omitempty"`
	IPs            []remnawave.ObservedIP `json:"ips,omitempty"`
	HWID           string                 `json:"hwid,omitempty"`
	Subscriptions  []abuseSubscription    `json:"subscriptions,omitempty"`
	ObservedAt     time.Time              `json:"observedAt"`
}
type abuseSnapshot struct {
	Items        []abuseFinding `json:"items"`
	CheckedAt    time.Time      `json:"checkedAt,omitempty"`
	CheckedUsers int            `json:"checkedUsers"`
	FailedChecks int            `json:"failedChecks"`
	Error        string         `json:"error,omitempty"`
}

func (h *Handler) handleAdminAbuseScan(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.canAdmin("abuses") {
		h.writeError(w, http.StatusForbidden, "forbidden", "Нет доступа к проверке абузов")
		return
	}
	var req struct {
		Refresh bool `json:"refresh"`
	}
	if err := h.decodeJSONRequest(w, r, 1024, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	if h.remnawaveClient == nil || h.customerRepository == nil || h.runtimeSettings == nil {
		h.writeError(w, http.StatusServiceUnavailable, "unavailable", "Панель подписок недоступна")
		return
	}
	h.abuseMu.Lock()
	if !h.abuseScanning && (req.Refresh || h.abuseSnapshot.CheckedAt.IsZero() || time.Since(h.abuseSnapshot.CheckedAt) > 5*time.Minute) {
		h.abuseScanning = true
		go h.scanAbuses()
	}
	snapshot, scanning := h.abuseSnapshot, h.abuseScanning
	h.abuseMu.Unlock()
	if snapshot.Items == nil {
		snapshot.Items = []abuseFinding{}
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"items": snapshot.Items, "checkedAt": snapshot.CheckedAt, "checkedUsers": snapshot.CheckedUsers, "failedChecks": snapshot.FailedChecks, "error": snapshot.Error, "scanning": scanning}})
}

func (h *Handler) scanAbuses() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	snapshot := h.collectAbuses(ctx)
	h.abuseMu.Lock()
	h.abuseSnapshot = snapshot
	h.abuseScanning = false
	h.abuseMu.Unlock()
}

func abusePanelKey(id int64, userUUID uuid.UUID) string {
	if id > 0 {
		return "id:" + strconv.FormatInt(id, 10)
	}
	return "uuid:" + userUUID.String()
}

type abuseTarget struct {
	User        remnawave.PanelUser
	Observation database.PanelPlanObservation
	Trial       bool
}
type abuseResult struct {
	Target   abuseTarget
	IPs      []remnawave.ObservedIP
	Devices  []remnawave.UserDevice
	Failures int
}

func (h *Handler) collectAbuses(ctx context.Context) abuseSnapshot {
	result := abuseSnapshot{Items: []abuseFinding{}, CheckedAt: time.Now()}
	users, err := h.remnawaveClient.GetUsers(ctx)
	if err != nil {
		result.Error = "Не удалось получить пользователей Remnawave"
		slog.Warn("abuse scan users", "error", err)
		return result
	}
	observations, err := h.customerRepository.PanelPlanObservations(ctx)
	if err != nil {
		result.Error = "Не удалось сопоставить подписки"
		slog.Warn("abuse scan observations", "error", err)
		return result
	}
	byID := map[int64]remnawave.PanelUser{}
	byUUID := map[uuid.UUID]remnawave.PanelUser{}
	byLink := map[string]remnawave.PanelUser{}
	for _, user := range *users {
		if user.ID > 0 {
			byID[user.ID] = user
		}
		if user.UUID != uuid.Nil {
			byUUID[user.UUID] = user
		}
		if user.SubscriptionURL != "" {
			byLink[user.SubscriptionURL] = user
		}
	}
	trialSettings := runtimeconfig.TrialSettings{}
	if h.runtimeSettings != nil {
		trialSettings = h.runtimeSettings.Snapshot().Trial
	}
	targets := make([]abuseTarget, 0, len(observations))
	seenSubscriptions := map[int64]bool{}
	for _, observation := range observations {
		if seenSubscriptions[observation.SubscriptionID] {
			continue
		}
		var user remnawave.PanelUser
		var found bool
		if observation.PanelID != nil {
			user, found = byID[*observation.PanelID]
		}
		if !found && observation.PanelUUID != nil {
			user, found = byUUID[*observation.PanelUUID]
		}
		if !found && observation.Link != "" {
			user, found = byLink[observation.Link]
		}
		if !found {
			continue
		}
		seenSubscriptions[observation.SubscriptionID] = true
		targets = append(targets, abuseTarget{User: user, Observation: observation, Trial: panelIsTrial(user, observation, trialSettings)})
	}
	jobs := make(chan abuseTarget)
	results := make(chan abuseResult, len(targets))
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for target := range jobs {
				entry := abuseResult{Target: target}
				if target.Trial {
					var err error
					entry.Devices, err = h.remnawaveClient.GetUserHWIDDevices(ctx, target.User.ID, target.User.UUID)
					if err != nil {
						entry.Failures++
						slog.Warn("abuse scan hwid", "error", err, "panelUserId", target.User.ID)
					}
				}
				if target.User.HwidDeviceLimit != nil && *target.User.HwidDeviceLimit > 0 && strings.EqualFold(target.User.Status, "ACTIVE") {
					var err error
					entry.IPs, err = h.remnawaveClient.FetchUserIPs(ctx, target.User.ID, target.User.UUID)
					if err != nil {
						entry.Failures++
						slog.Warn("abuse scan ips", "error", err, "panelUserId", target.User.ID)
					}
				}
				results <- entry
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, target := range targets {
			select {
			case jobs <- target:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { workers.Wait(); close(results) }()
	groups := map[string][]abuseSubscription{}
	for entry := range results {
		result.CheckedUsers++
		result.FailedChecks += entry.Failures
		target := entry.Target
		if target.User.HwidDeviceLimit != nil && len(entry.IPs) > *target.User.HwidDeviceLimit && *target.User.HwidDeviceLimit > 0 {
			result.Items = append(result.Items, abuseFinding{Key: "sharing:" + abusePanelKey(target.User.ID, target.User.UUID), Kind: "sharing", Username: target.User.Username, CustomerID: target.Observation.CustomerID, SubscriptionID: target.Observation.SubscriptionID, DeviceLimit: *target.User.HwidDeviceLimit, IPs: entry.IPs, ObservedAt: result.CheckedAt})
		}
		if target.Trial {
			for _, device := range entry.Devices {
				hwid := strings.TrimSpace(device.Hwid)
				if hwid == "" {
					continue
				}
				groups[hwid] = append(groups[hwid], abuseSubscription{CustomerID: target.Observation.CustomerID, SubscriptionID: target.Observation.SubscriptionID, Username: target.User.Username, Name: target.User.Username, CreatedAt: target.User.CreatedAt})
			}
		}
	}
	for hwid, subscriptions := range groups {
		unique := map[int64]abuseSubscription{}
		for _, sub := range subscriptions {
			unique[sub.SubscriptionID] = sub
		}
		if len(unique) < 2 {
			continue
		}
		subscriptions = subscriptions[:0]
		for _, sub := range unique {
			subscriptions = append(subscriptions, sub)
		}
		sort.Slice(subscriptions, func(i, j int) bool { return subscriptions[i].CreatedAt.Before(subscriptions[j].CreatedAt) })
		latest := subscriptions[len(subscriptions)-1]
		result.Items = append(result.Items, abuseFinding{Key: "trial:" + hwid, Kind: "trial", Username: latest.Username, CustomerID: latest.CustomerID, HWID: hwid, Subscriptions: subscriptions, ObservedAt: result.CheckedAt})
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].Kind != result.Items[j].Kind {
			return result.Items[i].Kind < result.Items[j].Kind
		}
		return result.Items[i].Username < result.Items[j].Username
	})
	if ctx.Err() != nil {
		result.Error = "Проверка прервана по времени; часть подписок не проверена"
	}
	return result
}
