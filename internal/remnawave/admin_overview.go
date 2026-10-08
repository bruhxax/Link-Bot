package remnawave

import (
	"context"
	"github.com/google/uuid"
)

// Numeric IDs and UUIDs come from the selected subscription stored in Link-Bot.
// Telegram lookup is only for a primary subscription without a stored identity.
func (r *Client) GetAdminUserOverview(ctx context.Context, userID int64, userUUID uuid.UUID, primaryTelegramID int64) (*PanelUser, error) {
	if userID > 0 || userUUID != uuid.Nil {
		return r.getPanelUserByIdentity(ctx, userID, userUUID)
	}
	if primaryTelegramID > 0 {
		return r.getPanelUserByTelegramID(ctx, primaryTelegramID)
	}
	return nil, ErrAdminSubscriptionNotFound
}

// GetAdminSubscriptionState fetches user fields once, then adds bounded device
// telemetry. Settings and subscription data come from one panel snapshot.
func (r *Client) GetAdminSubscriptionState(ctx context.Context, userID int64, userUUID uuid.UUID, primaryTelegramID int64) (*UserState, *UserSettings, error) {
	user, err := r.GetAdminUserOverview(ctx, userID, userUUID, primaryTelegramID)
	if err != nil {
		return nil, nil, err
	}
	state, err := r.userStateFromPanelUser(ctx, user, "userId", userID)
	return state, UserSettingsFromPanelUser(user), err
}
