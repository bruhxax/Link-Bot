package remnawave

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EnsureReviewAccessTarget provisions a missing primary account, then applies
// the previously saved absolute expiry. Existing accounts are never extended
// relatively, even when a previous create response was lost.
func (r *Client) EnsureReviewAccessTarget(ctx context.Context, customerID, telegramID, trafficBytes int64, deviceLimit int, expiresAt *time.Time) (*PanelUser, error) {
	user, err := r.getPanelUserByTelegramID(ctx, telegramID)
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return nil, err
		}
		user, err = r.createOrRecoverUserWithOptions(ctx, customerID, telegramID, int(trafficBytes), deviceLimit, 0, legacyProvisioningOptions(false))
		if err != nil {
			return nil, err
		}
	}
	var targetTraffic *int64
	if trafficBytes > 0 && user.TrafficLimitBytes > 0 {
		targetTraffic = &trafficBytes
	}
	return r.ApplyUserAccessTarget(ctx, user.ID, user.UUID, expiresAt, targetTraffic)
}

// Recover the deterministic secondary username after a lost create response.
// A review for an empty additional slot must never attach the primary account.
func (r *Client) EnsureReviewSecondaryAccessTarget(ctx context.Context, customerID, telegramID, subscriptionID, trafficBytes int64, deviceLimit int, expiresAt *time.Time) (*PanelUser, error) {
	options := legacyProvisioningOptions(false)
	options.UsernameSuffix = fmt.Sprintf("s%d", subscriptionID)
	username := appendUsernameSuffix(generateUsername(options.UsernameTemplate, customerID, telegramID), options.UsernameSuffix)
	filters := url.Values{"telegramId": {strconv.FormatInt(telegramID, 10)}, "size": {"20"}}
	users, err := r.streamUsers(ctx, filters)
	if err != nil {
		return nil, err
	}
	var user *PanelUser
	for i := range users {
		if users[i].Username == username {
			user = &users[i]
			break
		}
	}
	if user == nil {
		user, err = r.createUserWithOptions(ctx, customerID, telegramID, int(trafficBytes), deviceLimit, 0, options)
		if err != nil {
			return nil, err
		}
	}
	var traffic *int64
	if trafficBytes > 0 && user.TrafficLimitBytes > 0 {
		traffic = &trafficBytes
	}
	return r.ApplyUserAccessTarget(ctx, user.ID, user.UUID, expiresAt, traffic)
}

func (r *Client) AdjustUserAccess(ctx context.Context, userID int64, userUUID uuid.UUID, days int, trafficBytes int64) (*PanelUser, error) {
	if days <= 0 && trafficBytes <= 0 {
		return nil, errors.New("access adjustment is required")
	}
	user, err := r.getPanelUserByIdentity(ctx, userID, userUUID)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{"status": "ACTIVE"}
	if days > 0 {
		fields["expireAt"] = getNewExpire(days, user.ExpireAt)
	}
	if trafficBytes > 0 {
		if user.TrafficLimitBytes <= 0 {
			return nil, errors.New("unlimited traffic cannot be increased")
		}
		if trafficBytes > math.MaxInt64-user.TrafficLimitBytes {
			return nil, fmt.Errorf("traffic limit is too large")
		}
		fields["trafficLimitBytes"] = user.TrafficLimitBytes + trafficBytes
	}
	return r.patchPanelUser(ctx, user, fields)
}

// ApplyUserAccessTarget retries a promo reward without adding the same days or
// traffic twice. A newer, greater limit is never reduced by a retry.
func (r *Client) ApplyUserAccessTarget(ctx context.Context, userID int64, userUUID uuid.UUID, expireAt *time.Time, trafficLimitBytes *int64) (*PanelUser, error) {
	user, err := r.getPanelUserByIdentity(ctx, userID, userUUID)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{}
	if expireAt != nil && user.ExpireAt.Before(*expireAt) {
		fields["expireAt"] = expireAt.UTC()
		fields["status"] = "ACTIVE"
	}
	if trafficLimitBytes != nil {
		if user.TrafficLimitBytes <= 0 {
			return nil, errors.New("unlimited traffic cannot be increased")
		}
		if user.TrafficLimitBytes < *trafficLimitBytes {
			fields["trafficLimitBytes"] = *trafficLimitBytes
		}
	}
	if len(fields) == 0 {
		return user, nil
	}
	return r.patchPanelUser(ctx, user, fields)
}

func (r *Client) SetUserBlocked(ctx context.Context, userID int64, userUUID uuid.UUID, blocked bool) (*PanelUser, error) {
	user, err := r.getPanelUserByIdentity(ctx, userID, userUUID)
	if err != nil {
		if errors.Is(err, ErrAdminSubscriptionNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if blocked {
		return r.patchPanelUser(ctx, user, map[string]any{"status": "DISABLED"})
	}
	if user.ExpireAt.IsZero() || !user.ExpireAt.After(time.Now().UTC()) {
		return user, nil
	}
	return r.patchPanelUser(ctx, user, map[string]any{"status": "ACTIVE"})
}
