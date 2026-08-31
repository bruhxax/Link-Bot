package database

import (
	"context"
	"errors"
	"fmt"
	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"time"
)

type Referral struct {
	ID           int64     `db:"id"`
	ReferrerID   int64     `db:"referrer_id"`
	RefereeID    int64     `db:"referee_id"`
	UsedAt       time.Time `db:"used_at"`
	BonusGranted bool      `db:"bonus_granted"`
}

type ReferralReward struct {
	ID                 int64      `db:"id"`
	ReferralID         int64      `db:"referral_id"`
	EventType          string     `db:"event_type"`
	PurchaseID         *int64     `db:"purchase_id"`
	RewardDays         int        `db:"reward_days"`
	RewardTrafficBytes int64      `db:"reward_traffic_bytes"`
	RewardBalanceCents int64      `db:"reward_balance_cents"`
	Status             string     `db:"status"`
	CreatedAt          time.Time  `db:"created_at"`
	GrantedAt          *time.Time `db:"granted_at"`
}

type ReferralRepository struct {
	pool *pgxpool.Pool
}

func NewReferralRepository(pool *pgxpool.Pool) *ReferralRepository {
	return &ReferralRepository{pool: pool}
}

func (r *ReferralRepository) Create(ctx context.Context, referrerID, refereeID int64) (*Referral, error) {
	query := sq.Insert("referral").
		Columns("referrer_id", "referee_id", "used_at", "bonus_granted").
		Values(referrerID, refereeID, sq.Expr("NOW()"), false).
		Suffix("RETURNING id, referrer_id, referee_id, used_at, bonus_granted").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build insert referral query: %w", err)
	}

	row := r.pool.QueryRow(ctx, sql, args...)
	var ref Referral
	if err := row.Scan(&ref.ID, &ref.ReferrerID, &ref.RefereeID, &ref.UsedAt, &ref.BonusGranted); err != nil {
		return nil, fmt.Errorf("failed to scan inserted referral: %w", err)
	}
	return &ref, nil
}

func (r *ReferralRepository) FindByReferrer(ctx context.Context, referrerID int64) ([]Referral, error) {
	query := sq.Select("id", "referrer_id", "referee_id", "used_at", "bonus_granted").
		From("referral").
		Where(sq.Eq{"referrer_id": referrerID}).
		OrderBy("used_at DESC").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select referrals by referrer query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query referrals by referrer: %w", err)
	}
	defer rows.Close()

	var list []Referral
	for rows.Next() {
		var ref Referral
		if err := rows.Scan(&ref.ID, &ref.ReferrerID, &ref.RefereeID, &ref.UsedAt, &ref.BonusGranted); err != nil {
			return nil, fmt.Errorf("failed to scan referral row: %w", err)
		}
		list = append(list, ref)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating referral rows: %w", rows.Err())
	}
	return list, nil
}

func (r *ReferralRepository) CountByReferrer(ctx context.Context, referrerID int64) (int, error) {
	query := sq.Select("COUNT(*)").
		From("referral").
		Where(sq.Eq{"referrer_id": referrerID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count referrals by referrer query: %w", err)
	}

	var count int
	if err := r.pool.QueryRow(ctx, sql, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to scan count of referrals: %w", err)
	}
	return count, nil
}

func (r *ReferralRepository) CountGrantedByReferrer(ctx context.Context, referrerID int64) (int, error) {
	query := sq.Select("COUNT(*)").
		From("referral").
		Where(sq.Eq{
			"referrer_id":   referrerID,
			"bonus_granted": true,
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count granted referrals by referrer query: %w", err)
	}

	var count int
	if err := r.pool.QueryRow(ctx, sql, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to scan count of granted referrals: %w", err)
	}
	return count, nil
}

func (r *ReferralRepository) FindByReferee(ctx context.Context, refereeID int64) (*Referral, error) {
	query := sq.Select("id", "referrer_id", "referee_id", "used_at", "bonus_granted").
		From("referral").
		Where(sq.Eq{"referee_id": refereeID}).
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select referral by referee query: %w", err)
	}

	var ref Referral
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&ref.ID, &ref.ReferrerID, &ref.RefereeID, &ref.UsedAt, &ref.BonusGranted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query referral by referee: %w", err)
	}
	return &ref, nil
}

func (r *ReferralRepository) MarkBonusGranted(ctx context.Context, referralID int64) error {
	query := sq.Update("referral").
		Set("bonus_granted", true).
		Where(sq.Eq{"id": referralID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update bonus_granted query: %w", err)
	}

	res, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to execute update bonus_granted: %w", err)
	}
	if res.RowsAffected() == 0 {
		return errors.New("no referral record updated")
	}
	return nil
}

func (r *ReferralRepository) ClaimReward(ctx context.Context, referralID int64, eventType string, purchaseID *int64, days int, trafficBytes, balanceCents int64) (*ReferralReward, bool, error) {
	var reward ReferralReward
	err := r.pool.QueryRow(ctx, `
		INSERT INTO referral_reward (
			referral_id, event_type, purchase_id, reward_days, reward_traffic_bytes, reward_balance_cents
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING
		RETURNING id, referral_id, event_type, purchase_id, reward_days, reward_traffic_bytes,
		          reward_balance_cents, status, created_at, granted_at
	`, referralID, eventType, purchaseID, days, trafficBytes, balanceCents).Scan(
		&reward.ID, &reward.ReferralID, &reward.EventType, &reward.PurchaseID, &reward.RewardDays,
		&reward.RewardTrafficBytes, &reward.RewardBalanceCents, &reward.Status, &reward.CreatedAt, &reward.GrantedAt,
	)
	if err == nil {
		return &reward, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("claim referral reward: %w", err)
	}
	return nil, false, nil
}

func (r *ReferralRepository) MarkRewardGranted(ctx context.Context, rewardID int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE referral_reward
		SET status = 'granted', error_message = NULL, granted_at = NOW()
		WHERE id = $1
	`, rewardID)
	if err != nil {
		return fmt.Errorf("mark referral reward granted: %w", err)
	}
	return nil
}

func (r *ReferralRepository) MarkRewardFailed(ctx context.Context, rewardID int64, rewardErr error) error {
	message := ""
	if rewardErr != nil {
		message = rewardErr.Error()
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE referral_reward SET status = 'failed', error_message = $2 WHERE id = $1
	`, rewardID, message)
	if err != nil {
		return fmt.Errorf("mark referral reward failed: %w", err)
	}
	return nil
}

func (r *ReferralRepository) CountGrantedRewards(ctx context.Context, referrerID int64, eventType string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM referral_reward rr
		JOIN referral r ON r.id = rr.referral_id
		WHERE r.referrer_id = $1 AND rr.event_type = $2 AND rr.status = 'granted'
	`, referrerID, eventType).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count granted referral rewards: %w", err)
	}
	return count, nil
}
