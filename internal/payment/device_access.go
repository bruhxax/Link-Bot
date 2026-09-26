package payment

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
)

type deviceGrantStore interface {
	EnsureDeviceBase(context.Context, int64, int) error
	AddDeviceGrant(context.Context, int64, int64, int, time.Time) error
	DeviceLimitState(context.Context, int64, int64, time.Time) (int, int, error)
}

func deviceLimitWithGrants(base, extra int) int {
	if base <= 0 {
		return 0
	}
	return base + extra
}

func (s PaymentService) prepareSubscriptionDevices(ctx context.Context, customer *database.Customer, subscription *database.CustomerSubscription, purchase *database.Purchase, panelState *remnawave.UserState) (int, int, error) {
	return prepareSubscriptionDeviceGrant(ctx, s.purchaseRepository, customer, subscription, purchase, panelState, time.Now().UTC())
}

func prepareSubscriptionDeviceGrant(ctx context.Context, store deviceGrantStore, customer *database.Customer, subscription *database.CustomerSubscription, purchase *database.Purchase, panelState *remnawave.UserState, now time.Time) (int, int, error) {
	initial := purchaseDeviceLimit(purchase)
	if panelState != nil && panelState.Exists {
		initial = maxInt(panelState.DeviceLimit, 0)
	}
	if err := store.EnsureDeviceBase(ctx, subscription.ID, initial); err != nil {
		return 0, 0, err
	}
	if purchase.ExtraDevices > 0 && purchase.DeviceExpiresAt != nil {
		if err := store.AddDeviceGrant(ctx, purchase.ID, subscription.ID, purchase.ExtraDevices, *purchase.DeviceExpiresAt); err != nil {
			return 0, 0, err
		}
	}
	base, extra, err := store.DeviceLimitState(ctx, subscription.ID, purchase.ID, now)
	if err != nil {
		return 0, 0, err
	}
	if purchase.PurchaseKind != database.PurchaseKindExtraDevices {
		nextBase := purchaseDeviceLimit(purchase)
		if shouldAccumulateEntitlements(customerForSubscription(customer, subscription), panelState) {
			nextBase = mergeDeviceLimits(base, nextBase)
		}
		base = nextBase
	}
	limit := deviceLimitWithGrants(base, extra)
	if limit > 10000 {
		return 0, 0, errors.New("resulting device limit is too large")
	}
	return base, limit, nil
}

func (s PaymentService) applySubscriptionDeviceLimit(ctx context.Context, customer *database.Customer, subscription *database.CustomerSubscription, limit int) (*remnawave.PanelUser, error) {
	userID, userUUID := subscriptionPanelIdentity(subscription)
	if userID <= 0 && userUUID == uuid.Nil && !subscription.IsPrimary {
		return nil, errors.New("subscription is not active yet")
	}
	return s.remnawaveClient.SetDeviceLimit(ctx, customer.TelegramID, userID, userUUID, limit)
}

func (s PaymentService) deviceAccessSettings() runtimeconfig.DeviceAccessSettings {
	if s.runtimeSettings != nil {
		return s.runtimeSettings.Snapshot().DeviceAccess
	}
	return runtimeconfig.DefaultDeviceAccess()
}

// This job runs independently of the subscription expiry job: renewing the
// underlying subscription never changes a grant's original expiry.
func (s PaymentService) ProcessDeviceExpirations(ctx context.Context) error {
	if s.purchaseRepository == nil || s.subscriptionRepository == nil || s.remnawaveClient == nil {
		return nil
	}
	settings := s.deviceAccessSettings()
	reminderBefore := time.Now().UTC()
	if settings.ReminderNotification {
		reminderBefore = reminderBefore.Add(time.Duration(settings.ReminderDays) * 24 * time.Hour)
	}
	ids, err := s.purchaseRepository.PendingDeviceSubscriptions(ctx, reminderBefore)
	if err != nil {
		return err
	}
	var result error
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.processDeviceSubscription(ctx, id); err != nil {
			slog.Error("device access: expiry processing failed", "subscription_id", id, "error", err)
			result = errors.Join(result, err)
		}
	}
	return result
}

func (s PaymentService) processDeviceSubscription(ctx context.Context, subscriptionID int64) error {
	release, err := s.purchaseRepository.LockDeviceLimit(ctx, subscriptionID)
	if err != nil {
		return err
	}
	defer func() {
		if err := release(); err != nil {
			slog.Error("device access: unlock failed", "error", err)
		}
	}()
	subscription, err := s.subscriptionRepository.FindByID(ctx, subscriptionID)
	if err != nil || subscription == nil {
		return err
	}
	customer, err := s.customerRepository.FindById(ctx, subscription.CustomerID)
	if err != nil || customer == nil {
		return err
	}
	now := time.Now().UTC()
	base, extra, err := s.purchaseRepository.DeviceLimitState(ctx, subscriptionID, 0, now)
	if err != nil {
		return err
	}
	limit := deviceLimitWithGrants(base, extra)
	if _, err := s.applySubscriptionDeviceLimit(ctx, customer, subscription, limit); err != nil {
		return err
	}
	if err := s.purchaseRepository.MarkDeviceGrantsExpired(ctx, subscriptionID, now); err != nil {
		return err
	}
	return s.notifyDeviceGrants(ctx, customer, subscription, limit)
}

func (s PaymentService) notifyDeviceGrants(ctx context.Context, customer *database.Customer, subscription *database.CustomerSubscription, limit int) error {
	grants, err := s.purchaseRepository.DeviceGrants(ctx, subscription.ID)
	if err != nil {
		return err
	}
	settings := s.deviceAccessSettings()
	now := time.Now().UTC()
	for _, grant := range grants {
		expired := !grant.ExpiresAt.After(now)
		// A delayed callback must not announce an already expired grant as active.
		if grant.PurchaseNotifiedAt == nil {
			if err := s.sendDeviceGrantNotification(ctx, customer, subscription, grant, limit, "purchase", settings.PurchaseTemplate, settings.PurchaseNotification && !expired); err != nil {
				return err
			}
		}
		if expired && grant.ExpiredAppliedAt != nil && grant.ExpiryNotifiedAt == nil {
			if err := s.sendDeviceGrantNotification(ctx, customer, subscription, grant, limit, "expiry", settings.ExpiryTemplate, settings.ExpiryNotification); err != nil {
				return err
			}
		} else if !expired && grant.ReminderNotifiedAt == nil && settings.ReminderNotification && grant.ExpiresAt.Sub(now) <= time.Duration(settings.ReminderDays)*24*time.Hour {
			if err := s.sendDeviceGrantNotification(ctx, customer, subscription, grant, limit, "reminder", settings.ReminderTemplate, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s PaymentService) sendDeviceGrantNotification(ctx context.Context, customer *database.Customer, subscription *database.CustomerSubscription, grant database.DeviceGrant, limit int, kind, template string, enabled bool) error {
	if enabled && customer.TelegramID > 0 && s.telegramBot != nil {
		limitText := fmt.Sprint(limit)
		if limit <= 0 {
			limitText = "Безлимит"
		}
		zone := time.FixedZone("MSK", 3*60*60)
		text := strings.NewReplacer("{devices}", fmt.Sprint(grant.Devices), "{limit}", limitText,
			"{subscription}", html.EscapeString(subscription.DisplayName), "{expires}", grant.ExpiresAt.In(zone).Format("02.01.2006 15:04"),
			"{days}", fmt.Sprint(maxInt(0, int(grant.ExpiresAt.Sub(time.Now()).Hours()/24)))).Replace(template)
		if _, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: customer.TelegramID, ParseMode: models.ParseModeHTML, Text: text}); err != nil {
			return err
		}
	}
	return s.purchaseRepository.MarkDeviceGrantNotified(ctx, grant.PurchaseID, kind)
}
