package miniapp

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

// Bound the whole card, not each subscription in sequence. Every slot keeps
// its own panel identity; secondary subscriptions never fall back to Telegram.
func (h *Handler) loadAdminSubscriptionDetails(ctx context.Context, customer *database.Customer, active *database.CustomerSubscription, subscriptions []database.CustomerSubscription, blocked bool) []adminUserSubscriptionPayload {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	items := make([]adminUserSubscriptionPayload, len(subscriptions))
	var workers sync.WaitGroup
	jobs := make(chan int, len(subscriptions))
	for i := range subscriptions {
		jobs <- i
	}
	close(jobs)
	for n := 0; n < min(6, len(subscriptions)); n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				subscription := &subscriptions[i]
				item := adminUserSubscriptionPayload{ID: subscription.ID, Name: subscription.DisplayName, IsPrimary: subscription.IsPrimary,
					IsSelected: active != nil && active.ID == subscription.ID, Status: adminSubscriptionStatus(subscription.ExpireAt, blocked), DeviceLimit: -1}
				if subscription.ExpireAt != nil {
					item.ExpiresAt = subscription.ExpireAt.UTC().Format(time.RFC3339)
				}
				if h.remnawaveClient != nil {
					id, identity := customerSubscriptionIdentity(subscription)
					var telegramID int64
					if subscription.IsPrimary {
						telegramID = customer.TelegramID
					}
					state, settings, err := h.remnawaveClient.GetAdminSubscriptionState(ctx, id, identity, telegramID)
					if err != nil {
						if !blocked {
							if errors.Is(err, remnawave.ErrAdminSubscriptionNotFound) {
								item.Status = "none"
								item.ExpiresAt = ""
							} else {
								item.Status = "unavailable"
							}
						}
					} else if state != nil && state.Exists {
						item.Settings, item.PanelID, item.PanelUsername = settings, state.UserID, state.PanelUsername
						item.Devices, item.DevicesLoaded, item.UsedDevices = state.Devices, state.DevicesLoaded, state.UsedDevices
						item.TrafficLimitBytes, item.UsedTrafficBytes, item.LifetimeUsedTrafficBytes = state.TrafficLimitBytes, state.UsedTrafficBytes, state.LifetimeUsedTrafficBytes
						item.DeviceLimit = state.DeviceLimit
						item.ExpiresAt = ""
						if state.ExpireAt != nil {
							item.ExpiresAt = state.ExpireAt.UTC().Format(time.RFC3339)
						}
						if state.SubscriptionLink != nil {
							item.SubscriptionLink = strings.TrimSpace(*state.SubscriptionLink)
						}
						switch {
						case blocked:
							item.Status = "blocked"
						case strings.EqualFold(state.PanelStatus, "limited"), strings.EqualFold(state.PanelStatus, "disabled"):
							item.Status = strings.ToLower(state.PanelStatus)
						case state.Active:
							item.Status = "active"
						default:
							item.Status = "expired"
						}
					}
				}
				items[i] = item
			}
		}()
	}
	workers.Wait()
	return items
}
