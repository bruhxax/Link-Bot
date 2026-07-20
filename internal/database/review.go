package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

var ErrReviewAlreadyExists = errors.New("review already exists")

type Review struct {
	ID                 int64      `db:"id"`
	CustomerID         int64      `db:"customer_id"`
	TelegramID         int64      `db:"telegram_id"`
	TelegramUsername   string     `db:"telegram_username"`
	Rating             int        `db:"rating"`
	Comment            string     `db:"comment"`
	RewardGranted      bool       `db:"reward_granted"`
	RewardDays         int        `db:"reward_days"`
	RewardTrafficBytes int64      `db:"reward_traffic_bytes"`
	RewardGrantedAt    *time.Time `db:"reward_granted_at"`
	DeletedAt          *time.Time `db:"deleted_at"`
	CreatedAt          time.Time  `db:"created_at"`
	UpdatedAt          time.Time  `db:"updated_at"`
}

type ReviewSummary struct {
	Count   int
	Average float64
}

type ReviewRepository struct {
	pool *pgxpool.Pool
}

func NewReviewRepository(pool *pgxpool.Pool) *ReviewRepository {
	return &ReviewRepository{pool: pool}
}

func (r *ReviewRepository) Create(ctx context.Context, review *Review) (*Review, error) {
	query := `
		INSERT INTO review (
			customer_id, telegram_id, telegram_username, rating, comment,
			reward_granted, reward_days, reward_traffic_bytes, reward_granted_at, deleted_at, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, FALSE, 0, 0, NULL, NULL, NOW(), NOW())
		RETURNING id, customer_id, telegram_id, telegram_username, rating, comment,
		          reward_granted, reward_days, reward_traffic_bytes, reward_granted_at, deleted_at, created_at, updated_at
	`

	created := &Review{}
	err := r.pool.QueryRow(
		ctx,
		query,
		review.CustomerID,
		review.TelegramID,
		review.TelegramUsername,
		review.Rating,
		review.Comment,
	).Scan(
		&created.ID,
		&created.CustomerID,
		&created.TelegramID,
		&created.TelegramUsername,
		&created.Rating,
		&created.Comment,
		&created.RewardGranted,
		&created.RewardDays,
		&created.RewardTrafficBytes,
		&created.RewardGrantedAt,
		&created.DeletedAt,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrReviewAlreadyExists
		}
		return nil, fmt.Errorf("insert review: %w", err)
	}

	return created, nil
}

func (r *ReviewRepository) FindByCustomerID(ctx context.Context, customerID int64) (*Review, error) {
	query := sq.Select(
		"id", "customer_id", "telegram_id", "telegram_username", "rating", "comment",
		"reward_granted", "reward_days", "reward_traffic_bytes", "reward_granted_at", "deleted_at", "created_at", "updated_at",
	).
		From("review").
		Where(sq.Eq{"customer_id": customerID}).
		Where("deleted_at IS NULL").
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build find review query: %w", err)
	}

	item := &Review{}
	err = r.pool.QueryRow(ctx, sql, args...).Scan(
		&item.ID,
		&item.CustomerID,
		&item.TelegramID,
		&item.TelegramUsername,
		&item.Rating,
		&item.Comment,
		&item.RewardGranted,
		&item.RewardDays,
		&item.RewardTrafficBytes,
		&item.RewardGrantedAt,
		&item.DeletedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query review: %w", err)
	}

	return item, nil
}

func (r *ReviewRepository) FindAnyByCustomerID(ctx context.Context, customerID int64) (*Review, error) {
	query := sq.Select(
		"id", "customer_id", "telegram_id", "telegram_username", "rating", "comment",
		"reward_granted", "reward_days", "reward_traffic_bytes", "reward_granted_at", "deleted_at", "created_at", "updated_at",
	).
		From("review").
		Where(sq.Eq{"customer_id": customerID}).
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build find any review query: %w", err)
	}

	item := &Review{}
	err = r.pool.QueryRow(ctx, sql, args...).Scan(
		&item.ID,
		&item.CustomerID,
		&item.TelegramID,
		&item.TelegramUsername,
		&item.Rating,
		&item.Comment,
		&item.RewardGranted,
		&item.RewardDays,
		&item.RewardTrafficBytes,
		&item.RewardGrantedAt,
		&item.DeletedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query any review: %w", err)
	}

	return item, nil
}

func (r *ReviewRepository) ListLatest(ctx context.Context, limit int) ([]Review, error) {
	if limit <= 0 {
		limit = 50
	}

	query := sq.Select(
		"id", "customer_id", "telegram_id", "telegram_username", "rating", "comment",
		"reward_granted", "reward_days", "reward_traffic_bytes", "reward_granted_at", "deleted_at", "created_at", "updated_at",
	).
		From("review").
		Where("deleted_at IS NULL").
		OrderBy("created_at DESC").
		Limit(uint64(limit)).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list reviews query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query reviews: %w", err)
	}
	defer rows.Close()

	items := make([]Review, 0, limit)
	for rows.Next() {
		var item Review
		if err := rows.Scan(
			&item.ID,
			&item.CustomerID,
			&item.TelegramID,
			&item.TelegramUsername,
			&item.Rating,
			&item.Comment,
			&item.RewardGranted,
			&item.RewardDays,
			&item.RewardTrafficBytes,
			&item.RewardGrantedAt,
			&item.DeletedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan review: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reviews: %w", err)
	}

	return items, nil
}

func (r *ReviewRepository) GetSummary(ctx context.Context) (*ReviewSummary, error) {
	row := r.pool.QueryRow(ctx, `SELECT COUNT(*), COALESCE(AVG(rating::numeric), 0) FROM review WHERE deleted_at IS NULL`)

	summary := &ReviewSummary{}
	if err := row.Scan(&summary.Count, &summary.Average); err != nil {
		return nil, fmt.Errorf("query review summary: %w", err)
	}

	return summary, nil
}

func (r *ReviewRepository) SoftDeleteByID(ctx context.Context, reviewID int64) error {
	result, err := r.pool.Exec(
		ctx,
		`UPDATE review
		 SET deleted_at = NOW(),
		     updated_at = NOW()
		 WHERE id = $1
		   AND deleted_at IS NULL`,
		reviewID,
	)
	if err != nil {
		return fmt.Errorf("soft delete review: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *ReviewRepository) MarkRewardGranted(ctx context.Context, reviewID int64, rewardDays int, rewardTrafficBytes int64) error {
	result, err := r.pool.Exec(
		ctx,
		`UPDATE review
		 SET reward_granted = TRUE,
		     reward_days = $2,
		     reward_traffic_bytes = $3,
		     reward_granted_at = NOW(),
		     updated_at = NOW()
		 WHERE id = $1`,
		reviewID,
		rewardDays,
		rewardTrafficBytes,
	)
	if err != nil {
		return fmt.Errorf("update review reward: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
