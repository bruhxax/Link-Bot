package miniapp

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

// Load only the subscriptions on the current page, without HWID requests or
// downloading the entire panel catalog. Failed requests remain explicitly unknown.
func (h *Handler) enrichAdminUserOverview(ctx context.Context, items []database.AdminUserSummary, rows []adminUserSummaryPayload) {
	if h.remnawaveClient == nil || len(items) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	jobs := make(chan int, len(items))
	for i := range items {
		jobs <- i
	}
	close(jobs)
	var workers sync.WaitGroup
	for n := 0; n < min(6, len(items)); n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				item := items[i]
				var id int64
				var identity uuid.UUID
				if item.PanelUserID != nil {
					id = *item.PanelUserID
				}
				if item.PanelUserUUID != nil {
					identity = *item.PanelUserUUID
				}
				var telegramID int64
				if item.SubscriptionIsPrimary {
					telegramID = item.TelegramID
				}
				user, err := h.remnawaveClient.GetAdminUserOverview(ctx, id, identity, telegramID)
				if err == nil && user != nil {
					applyAdminUserOverview(&rows[i], user)
				}
			}
		}()
	}
	workers.Wait()
}

func applyAdminUserOverview(row *adminUserSummaryPayload, user *remnawave.PanelUser) {
	row.TrafficLoaded = true
	row.TrafficLimitBytes = user.TrafficLimitBytes
	row.UsedTrafficBytes = user.UserTraffic.UsedTrafficBytes
	row.PanelUsername = user.Username
	row.PanelID = user.ID
	row.TrafficLimitStrategy = user.TrafficLimitStrategy
	row.LifetimeUsedTrafficBytes = user.UserTraffic.LifetimeUsedTrafficBytes
	if user.Description != nil {
		row.Description = *user.Description
	}
	if user.Tag != nil {
		row.Tag = *user.Tag
	}
	if !user.CreatedAt.IsZero() {
		row.PanelCreatedAt = user.CreatedAt.UTC().Format(time.RFC3339)
	}
	if user.UserTraffic.OnlineAt != nil {
		row.OnlineAt = user.UserTraffic.OnlineAt.UTC().Format(time.RFC3339)
	}
	if user.SubscriptionURL != "" {
		row.SubscriptionLink = user.SubscriptionURL
	}
	if !user.ExpireAt.IsZero() {
		row.ExpiresAt = user.ExpireAt.UTC().Format(time.RFC3339)
	}
	if row.IsBlocked {
		row.SubscriptionStatus = "blocked"
		return
	}
	if status := strings.ToLower(user.Status); status != "" {
		row.SubscriptionStatus = status
	}
}
