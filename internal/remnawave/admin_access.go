package remnawave

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

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
