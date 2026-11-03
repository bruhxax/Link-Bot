package database

import (
	"context"
	"fmt"
	"time"
)

// ReferralLeader contains totals for one user who invited at least two people.
// Monetary amounts are limited to paid RUB purchases; other currencies are not mixed in.
type ReferralLeader struct {
	TelegramID     int64     `json:"telegramId"`
	Username       string    `json:"username"`
	Invited        int64     `json:"invited"`
	Paying         int64     `json:"paying"`
	PurchaseCount  int64     `json:"purchaseCount"`
	RevenueCents   int64     `json:"revenueCents"`
	RewardsCents   int64     `json:"rewardsCents"`
	WithdrawnCents int64     `json:"withdrawnCents"`
	LastInviteAt   time.Time `json:"lastInviteAt"`
}

type ReferralHistoryItem struct {
	ID               int64     `json:"id"`
	ReferrerID       int64     `json:"referrerId"`
	ReferrerUsername string    `json:"referrerUsername"`
	RefereeID        int64     `json:"refereeId"`
	RefereeUsername  string    `json:"refereeUsername"`
	UsedAt           time.Time `json:"usedAt"`
	PurchaseCount    int64     `json:"purchaseCount"`
	RevenueCents     int64     `json:"revenueCents"`
	RewardsCents     int64     `json:"rewardsCents"`
}

func (r *ReferralRepository) ListAdminLeaders(ctx context.Context, search string, limit, offset int) ([]ReferralLeader, int64, error) {
	const base = `FROM (SELECT referrer_id, COUNT(*) invited, MAX(used_at) last_invite_at FROM referral GROUP BY referrer_id HAVING COUNT(*) >= 2) leaders JOIN customer owner ON owner.telegram_id = leaders.referrer_id WHERE ($1 = '' OR COALESCE(owner.telegram_username, '') ILIKE '%' || $1 || '%' OR leaders.referrer_id::text LIKE '%' || $1 || '%')`
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) `+base, search).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count referral leaders: %w", err)
	}
	rows, err := r.pool.Query(ctx, `WITH page AS (
		SELECT leaders.referrer_id, leaders.invited, leaders.last_invite_at, owner.id customer_id, COALESCE(owner.telegram_username, '') username `+base+`
		ORDER BY leaders.invited DESC, leaders.last_invite_at DESC, leaders.referrer_id DESC LIMIT $2 OFFSET $3
	)
	SELECT page.referrer_id, page.username, page.invited, COALESCE(activity.paying,0), COALESCE(activity.purchases,0), COALESCE(activity.revenue_cents,0), COALESCE(rewards.amount_cents,0), COALESCE(withdrawals.amount_cents,0), page.last_invite_at
	FROM page
	LEFT JOIN LATERAL (
		SELECT COUNT(DISTINCT p.customer_id) paying, COUNT(p.id) purchases, COALESCE(ROUND(SUM(p.amount)*100)::bigint,0) revenue_cents
		FROM referral ref JOIN customer invited ON invited.telegram_id = ref.referee_id JOIN purchase p ON p.customer_id = invited.id AND p.status = 'paid' AND p.currency = 'RUB'
		WHERE ref.referrer_id = page.referrer_id
	) activity ON TRUE
	LEFT JOIN LATERAL (SELECT COALESCE(SUM(rr.reward_balance_cents),0) amount_cents FROM referral ref JOIN referral_reward rr ON rr.referral_id = ref.id AND rr.status = 'granted' WHERE ref.referrer_id = page.referrer_id) rewards ON TRUE
	LEFT JOIN LATERAL (SELECT COALESCE(SUM(w.amount_cents),0) amount_cents FROM balance_withdrawal w WHERE w.customer_id = page.customer_id AND w.status = 'approved') withdrawals ON TRUE
	ORDER BY page.invited DESC, page.last_invite_at DESC, page.referrer_id DESC`, search, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list referral leaders: %w", err)
	}
	defer rows.Close()
	items := []ReferralLeader{}
	for rows.Next() {
		var item ReferralLeader
		if err := rows.Scan(&item.TelegramID, &item.Username, &item.Invited, &item.Paying, &item.PurchaseCount, &item.RevenueCents, &item.RewardsCents, &item.WithdrawnCents, &item.LastInviteAt); err != nil {
			return nil, 0, fmt.Errorf("scan referral leader: %w", err)
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *ReferralRepository) ListAdminHistory(ctx context.Context, search string, referrerID int64, limit, offset int) ([]ReferralHistoryItem, int64, error) {
	const base = `FROM referral ref JOIN customer owner ON owner.telegram_id = ref.referrer_id JOIN customer invited ON invited.telegram_id = ref.referee_id WHERE ($1 = '' OR COALESCE(owner.telegram_username,'') ILIKE '%' || $1 || '%' OR COALESCE(invited.telegram_username,'') ILIKE '%' || $1 || '%' OR ref.referrer_id::text LIKE '%' || $1 || '%' OR ref.referee_id::text LIKE '%' || $1 || '%') AND ($2::bigint = 0 OR ref.referrer_id = $2)`
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) `+base, search, referrerID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count referral history: %w", err)
	}
	rows, err := r.pool.Query(ctx, `WITH page AS (SELECT ref.id,ref.referrer_id,ref.referee_id,ref.used_at,COALESCE(owner.telegram_username,'') referrer_username,COALESCE(invited.telegram_username,'') referee_username,invited.id customer_id `+base+` ORDER BY ref.used_at DESC,ref.id DESC LIMIT $3 OFFSET $4)
	SELECT page.id,page.referrer_id,page.referrer_username,page.referee_id,page.referee_username,page.used_at,COALESCE(activity.purchases,0),COALESCE(activity.revenue_cents,0),COALESCE(rewards.amount_cents,0)
	FROM page
	LEFT JOIN LATERAL (SELECT COUNT(p.id) purchases,COALESCE(ROUND(SUM(p.amount)*100)::bigint,0) revenue_cents FROM purchase p WHERE p.customer_id=page.customer_id AND p.status='paid' AND p.currency='RUB') activity ON TRUE
	LEFT JOIN LATERAL (SELECT COALESCE(SUM(rr.reward_balance_cents),0) amount_cents FROM referral_reward rr WHERE rr.referral_id=page.id AND rr.status='granted') rewards ON TRUE
	ORDER BY page.used_at DESC,page.id DESC`, search, referrerID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list referral history: %w", err)
	}
	defer rows.Close()
	items := []ReferralHistoryItem{}
	for rows.Next() {
		var item ReferralHistoryItem
		if err := rows.Scan(&item.ID, &item.ReferrerID, &item.ReferrerUsername, &item.RefereeID, &item.RefereeUsername, &item.UsedAt, &item.PurchaseCount, &item.RevenueCents, &item.RewardsCents); err != nil {
			return nil, 0, fmt.Errorf("scan referral history: %w", err)
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}
