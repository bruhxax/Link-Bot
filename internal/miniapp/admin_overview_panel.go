package miniapp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
)

type overviewPanelCache struct {
	mu           sync.Mutex
	users        []remnawave.PanelUser
	at           time.Time
	failed       time.Time
	err          error
	pending      chan struct{}
	observations []database.PanelPlanObservation
	planErr      error
	history      []panelCatalogSnapshot
	counts       *remnawave.PanelUserCounts
}

type panelCatalogSnapshot struct {
	Users        []remnawave.PanelUser
	Observations []database.PanelPlanObservation
	Revision     string
}

// The loader belongs to the server, not to a tab's request. Cancelled navigation
// must not cancel the shared catalog. Old data stays usable during refresh.
func (h *Handler) panelCatalog(force bool, revision string) (panelCatalogSnapshot, bool, error) {
	c := &h.overviewPanel
	c.mu.Lock()
	defer c.mu.Unlock()
	if h.remnawaveClient == nil {
		return panelCatalogSnapshot{}, false, errors.New("panel is not configured")
	}
	if revision != "" {
		for _, s := range c.history {
			if s.Revision == revision {
				return s, false, nil
			}
		}
		return panelCatalogSnapshot{}, false, errors.New("catalog snapshot expired")
	}
	if c.pending == nil && (force || c.at.IsZero() || time.Since(c.at) > time.Minute) && (force || c.failed.IsZero() || time.Since(c.failed) > 5*time.Second) {
		c.pending = make(chan struct{})
		c.err = nil
		if c.users == nil {
			go h.loadPanelCounts()
		}
		go h.loadPanelCatalog()
	}
	s := panelCatalogSnapshot{Users: c.users, Observations: c.observations}
	if len(c.history) > 0 {
		s = c.history[len(c.history)-1]
	}
	return s, c.pending != nil, c.err
}

func (h *Handler) loadPanelCounts() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stats, err := h.remnawaveClient.GetSystemStats(ctx)
	if err != nil || stats.Users == nil {
		return
	}
	h.overviewPanel.mu.Lock()
	defer h.overviewPanel.mu.Unlock()
	h.overviewPanel.counts = stats.Users
}

func (h *Handler) loadPanelCatalog() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	users, err := h.remnawaveClient.GetUsers(ctx)
	if err == nil && users == nil {
		err = errors.New("panel catalog is missing")
	}
	if err == nil && *users == nil {
		*users = []remnawave.PanelUser{}
	}
	var observations []database.PanelPlanObservation
	var planErr error
	if err == nil && h.customerRepository != nil {
		observations, planErr = h.customerRepository.PanelPlanObservations(ctx)
	}
	c := &h.overviewPanel
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.users = *users
		c.at = time.Now()
		if planErr == nil {
			c.observations = observations
		}
		c.planErr = planErr
		s := panelCatalogSnapshot{Users: *users, Observations: c.observations, Revision: fmt.Sprint(c.at.UnixNano())}
		c.history = append(c.history, s)
		if len(c.history) > 8 {
			c.history = c.history[len(c.history)-8:]
		}
	} else {
		c.failed = time.Now()
	}
	c.err = err
	close(c.pending)
	c.pending = nil
}

func (h *Handler) overviewPanelUsers(ctx context.Context, force bool) ([]remnawave.PanelUser, error) {
	s, pending, err := h.panelCatalog(force, "")
	if pending {
		h.overviewPanel.mu.Lock()
		done := h.overviewPanel.pending
		h.overviewPanel.mu.Unlock()
		if done != nil {
			select {
			case <-done:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		s, _, err = h.panelCatalog(false, "")
	}
	return s.Users, err
}

func panelOverview(users []remnawave.PanelUser, observations []database.PanelPlanObservation, settings runtimeconfig.Settings, days int, now time.Time) (*database.OverviewUsers, []database.OverviewPlan) {
	zone := time.FixedZone("MSK", 3*60*60)
	local := now.In(zone)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	from := today.AddDate(0, 0, -(days - 1))
	week := today.AddDate(0, 0, -6)
	stats := &database.OverviewUsers{Source: "panel", Total: len(users), Daily: []database.OverviewPoint{}}
	daily := map[string]int{}
	plans := map[string]database.OverviewPlan{}
	byID := map[int64]database.PanelPlanObservation{}
	byUUID := map[uuid.UUID]database.PanelPlanObservation{}
	byLink := map[string]database.PanelPlanObservation{}
	for _, o := range observations {
		if o.PanelID != nil && *o.PanelID > 0 {
			byID[*o.PanelID] = o
		}
		if o.PanelUUID != nil && *o.PanelUUID != uuid.Nil {
			byUUID[*o.PanelUUID] = o
		}
		if o.Link != "" {
			byLink[o.Link] = o
		}
	}
	for _, user := range users {
		status := strings.ToUpper(strings.TrimSpace(user.Status))
		active := false
		switch {
		case status == "DISABLED":
			stats.Disabled++
		case status == "EXPIRED" || (!user.ExpireAt.IsZero() && !user.ExpireAt.After(now)):
			stats.Expired++
		case status == "LIMITED":
			stats.Limited++
		case status == "ACTIVE" || status == "" && user.ExpireAt.After(now):
			stats.Active++
			active = true
		default:
			stats.Other++
		}
		if !user.CreatedAt.IsZero() && !user.CreatedAt.After(now) {
			if !user.CreatedAt.Before(today) {
				stats.Today++
			}
			if !user.CreatedAt.Before(week) {
				stats.Week++
			}
			if !user.CreatedAt.Before(from) {
				daily[user.CreatedAt.In(zone).Format("2006-01-02")]++
			}
		}
		if !active {
			continue
		}
		o, ok := byUUID[user.UUID]
		if !ok {
			o, ok = byID[user.ID]
			// A recreated panel account may reuse the numeric ID.
			if ok && o.PanelUUID != nil && *o.PanelUUID != uuid.Nil && user.UUID != uuid.Nil && *o.PanelUUID != user.UUID {
				ok = false
			}
		}
		if !ok {
			o = byLink[user.SubscriptionURL]
		}
		months, planDays := o.Months, o.Days
		if months <= 0 && planDays <= 0 {
			for _, p := range settings.Plans {
				if p.ID == o.PlanID && o.PlanID != "" {
					months, planDays = p.Months, p.Days
					break
				}
			}
		}
		if months <= 0 && planDays <= 0 {
			// Infer only when all plans with these live limits agree on duration.
			matched := false
			ambiguous := false
			for _, p := range settings.Plans {
				traffic := int64(p.TrafficGB) * 1024 * 1024 * 1024
				if p.UnlimitedTraffic {
					traffic = 0
				}
				deviceLimit := 0
				if user.HwidDeviceLimit != nil {
					deviceLimit = *user.HwidDeviceLimit
				}
				if traffic != user.TrafficLimitBytes || p.DeviceLimit != deviceLimit {
					continue
				}
				if matched && (months != p.Months || planDays != p.Days) {
					ambiguous = true
				}
				months, planDays = p.Months, p.Days
				matched = true
			}
			if ambiguous {
				months, planDays = 0, 0
			}
		}
		unlimited := user.TrafficLimitBytes <= 0
		key := fmt.Sprintf("%d:%d:%t", months, planDays, unlimited)
		p := plans[key]
		p.Key, p.Months, p.Days, p.Unlimited = key, months, planDays, unlimited
		p.Label = overviewPlanLabel(months, planDays, unlimited)
		p.Count++
		plans[key] = p
	}
	for date := from; !date.After(today); date = date.AddDate(0, 0, 1) {
		key := date.Format("2006-01-02")
		stats.Daily = append(stats.Daily, database.OverviewPoint{Date: key, Value: float64(daily[key])})
	}
	result := make([]database.OverviewPlan, 0, len(plans))
	for _, p := range plans {
		result = append(result, p)
	}
	duration := func(p database.OverviewPlan) int {
		if p.Months > 0 {
			return p.Months * 30
		}
		if p.Days > 0 {
			return p.Days
		}
		return 1000000
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := duration(result[i]), duration(result[j])
		if a != b {
			return a < b
		}
		if result[i].Unlimited != result[j].Unlimited {
			return !result[i].Unlimited
		}
		return result[i].Key < result[j].Key
	})
	return stats, result
}

func overviewPlanLabel(months, days int, unlimited bool) string {
	label := "Индивидуальный тариф"
	if months > 0 {
		label = fmt.Sprintf("%d %s", months, ruPlural(months, "месяц", "месяца", "месяцев"))
	} else if days > 0 {
		label = fmt.Sprintf("%d %s", days, ruPlural(days, "день", "дня", "дней"))
	}
	if unlimited {
		label += " · Безлимит"
	} else {
		label += " · Лимитный"
	}
	return label
}

func ruPlural(n int, one, few, many string) string {
	if n%100 >= 11 && n%100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}
