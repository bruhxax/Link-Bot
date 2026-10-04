package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
)

type ReviewRewardClaim struct {
	ReviewID           int64
	TargetInitialized  bool
	SubscriptionID     *int64
	PanelUserID        int64
	PanelUserUUID      *uuid.UUID
	TargetExpiresAt    *time.Time
	TargetTrafficBytes *int64
	AccessApplied      bool
}

func (r *ReviewRepository) AcquireRewardLock(ctx context.Context, customerID int64) (func(), error) {
	return acquireSubscriptionRewardLock(ctx, r.pool, customerID)
}

func (r *ReviewRepository) RewardClaim(ctx context.Context, reviewID int64) (*ReviewRewardClaim, error) {
	if _, err := r.pool.Exec(ctx, `INSERT INTO review_reward_claim (review_id) VALUES ($1) ON CONFLICT DO NOTHING`, reviewID); err != nil {
		return nil, err
	}
	claim := &ReviewRewardClaim{ReviewID: reviewID}
	err := r.pool.QueryRow(ctx, `SELECT target_initialized, subscription_id, panel_user_id, panel_user_uuid, target_expires_at, target_traffic_bytes, access_applied FROM review_reward_claim WHERE review_id = $1`, reviewID).Scan(&claim.TargetInitialized, &claim.SubscriptionID, &claim.PanelUserID, &claim.PanelUserUUID, &claim.TargetExpiresAt, &claim.TargetTrafficBytes, &claim.AccessApplied)
	return claim, err
}

func (r *ReviewRepository) SaveRewardTarget(ctx context.Context, c *ReviewRewardClaim) error {
	_, err := r.pool.Exec(ctx, `UPDATE review_reward_claim SET target_initialized=TRUE, subscription_id=$2, panel_user_id=$3, panel_user_uuid=$4, target_expires_at=$5, target_traffic_bytes=$6 WHERE review_id=$1 AND target_initialized=FALSE`, c.ReviewID, c.SubscriptionID, c.PanelUserID, c.PanelUserUUID, c.TargetExpiresAt, c.TargetTrafficBytes)
	return err
}

func (r *ReviewRepository) MarkAccessApplied(ctx context.Context, reviewID int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE review_reward_claim SET access_applied=TRUE WHERE review_id=$1`, reviewID)
	return err
}

func (p *PromoCode) AvailableToCustomer(customerID int64) bool {
	return p != nil && (p.OwnerCustomerID == nil || (customerID > 0 && *p.OwnerCustomerID == customerID))
}

func (r *PromoCodeRepository) FindReviewPromo(ctx context.Context, reviewID, customerID int64) (*PromoCode, error) {
	p := &PromoCode{}
	err := scanPromoCode(r.pool.QueryRow(ctx, `SELECT `+strings.Join(promoCodeSelectColumns, ", ")+` FROM promo_code WHERE review_id=$1 AND owner_customer_id=$2`, reviewID, customerID), p)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

// The unique review ID makes issuing a personal code safe to retry, including
// after an ambiguous database/network failure.
func (r *PromoCodeRepository) IssueReviewPromo(ctx context.Context, reviewID, customerID int64, p *PromoCode) (*PromoCode, error) {
	if err := NormalizePromoReward(p); err != nil {
		return nil, err
	}
	code := "REVIEW-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:20])
	_, err := r.pool.Exec(ctx, `INSERT INTO promo_code (code, discount_percent, is_active, expires_at, max_redemptions, created_by_telegram_id, reward_type, reward_value, reward_traffic_gb, owner_customer_id, review_id) VALUES ($1,$2,TRUE,$3,1,0,$4,$5,$6,$7,$8) ON CONFLICT (review_id) DO NOTHING`, code, p.DiscountPercent, p.ExpiresAt, p.RewardType, p.RewardValue, p.RewardTrafficGB, customerID, reviewID)
	if err != nil {
		return nil, fmt.Errorf("issue review promo: %w", err)
	}
	return r.FindReviewPromo(ctx, reviewID, customerID)
}
