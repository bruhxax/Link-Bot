package remnawave

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ReissueUser copies access to a new panel identity while leaving the old one
// untouched for the grace period. Traffic already consumed is deducted from
// the new limit, so the customer does not receive the allowance twice.
func (r *Client) ReissueUser(ctx context.Context, userID int64, userUUID uuid.UUID) (*PanelUser, *PanelUser, error) {
	old, err := r.getPanelUserByIdentity(ctx, userID, userUUID)
	if err != nil {
		return nil, nil, err
	}
	if old == nil || old.ExpireAt.IsZero() || !old.ExpireAt.After(time.Now().UTC()) || strings.EqualFold(old.Status, "DISABLED") || strings.EqualFold(old.Status, "EXPIRED") {
		return nil, nil, errors.New("subscription is not active")
	}
	if old.ID <= 0 && old.UUID == uuid.Nil {
		return nil, nil, errors.New("old subscription identity is missing")
	}
	remaining := old.TrafficLimitBytes
	if remaining > 0 {
		remaining -= old.UserTraffic.UsedTrafficBytes
		if remaining < 1 {
			remaining = 1
		} // Zero means unlimited in Remnawave.
	}
	squads := make([]uuid.UUID, 0, len(old.ActiveInternalSquads))
	for _, squad := range old.ActiveInternalSquads {
		squads = append(squads, squad.UUID)
	}
	username := appendUsernameSuffix(old.Username, "r"+strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
	fields := map[string]any{
		"username":             username,
		"status":               "ACTIVE",
		"expireAt":             old.ExpireAt,
		"trafficLimitBytes":    remaining,
		"trafficLimitStrategy": normalizeTrafficStrategy(old.TrafficLimitStrategy),
		"activeInternalSquads": squads,
	}
	if old.TelegramID != nil {
		fields["telegramId"] = *old.TelegramID
	}
	if old.Description != nil {
		fields["description"] = *old.Description
	}
	if old.HwidDeviceLimit != nil {
		fields["hwidDeviceLimit"] = *old.HwidDeviceLimit
	}
	if old.ExternalSquadUUID != nil && *old.ExternalSquadUUID != uuid.Nil {
		fields["externalSquadUuid"] = *old.ExternalSquadUUID
	}
	if old.Tag != nil && strings.TrimSpace(*old.Tag) != "" {
		fields["tag"] = *old.Tag
	}
	created, err := r.createPanelUser(ctx, fields)
	if err != nil {
		return nil, nil, err
	}
	if created.ID <= 0 && created.UUID == uuid.Nil {
		return old, created, errors.New("new subscription identity is missing")
	}
	if strings.TrimSpace(created.SubscriptionURL) == "" {
		var loaded *PanelUser
		loaded, err = r.getPanelUserByIdentity(ctx, created.ID, created.UUID)
		if err != nil {
			return old, created, fmt.Errorf("load new subscription link: %w", err)
		}
		created = loaded
	}
	if created == nil || strings.TrimSpace(created.SubscriptionURL) == "" {
		return old, created, errors.New("new subscription link is missing")
	}
	return old, created, nil
}
