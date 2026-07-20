package database

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	ErrPromoCodeAlreadyExists = errors.New("promo code already exists")
	ErrPromoCodeInvalidFormat = errors.New("promo code has invalid format")
	ErrPromoCodeAlreadyUsed   = errors.New("promo code already used by customer")
	ErrPromoCodeLimitReached  = errors.New("promo code limit reached")

	promoCodePattern = regexp.MustCompile(`^[A-Z0-9_-]{3,32}$`)
)

type PromoCode struct {
	ID                  int64      `db:"id"`
	Code                string     `db:"code"`
	DiscountPercent     int        `db:"discount_percent"`
	IsActive            bool       `db:"is_active"`
	ExpiresAt           *time.Time `db:"expires_at"`
	MaxRedemptions      *int       `db:"max_redemptions"`
	RedemptionCount     int        `db:"redemption_count"`
	DeletedAt           *time.Time `db:"deleted_at"`
	CreatedByTelegramID int64      `db:"created_by_telegram_id"`
	CreatedAt           time.Time  `db:"created_at"`
	UpdatedAt           time.Time  `db:"updated_at"`
}

type PromoCodeRepository struct {
	pool *pgxpool.Pool
}

func NewPromoCodeRepository(pool *pgxpool.Pool) *PromoCodeRepository {
	return &PromoCodeRepository{pool: pool}
}

func NormalizePromoCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func IsValidPromoCode(value string) bool {
	return promoCodePattern.MatchString(NormalizePromoCode(value))
}

var promoCodeSelectColumns = []string{
	"id",
	"code",
	"discount_percent",
	"is_active",
	"expires_at",
	"max_redemptions",
	"redemption_count",
	"deleted_at",
	"created_by_telegram_id",
	"created_at",
	"updated_at",
}

func promoCodeSelectColumnsWithLiveCount(alias string) []string {
	if alias == "" {
		alias = "promo_code"
	}
	columns := append([]string(nil), promoCodeSelectColumns...)
	columns[6] = fmt.Sprintf("(SELECT COUNT(*)::int FROM promo_code_redemption pcr WHERE pcr.promo_code_id = %s.id) AS redemption_count", alias)
	return columns
}

func scanPromoCode(scanner interface {
	Scan(dest ...interface{}) error
}, promo *PromoCode) error {
	return scanner.Scan(
		&promo.ID,
		&promo.Code,
		&promo.DiscountPercent,
		&promo.IsActive,
		&promo.ExpiresAt,
		&promo.MaxRedemptions,
		&promo.RedemptionCount,
		&promo.DeletedAt,
		&promo.CreatedByTelegramID,
		&promo.CreatedAt,
		&promo.UpdatedAt,
	)
}

func (r *PromoCodeRepository) Create(ctx context.Context, promo *PromoCode) (*PromoCode, error) {
	if promo == nil {
		return nil, errors.New("promo code is nil")
	}

	code := NormalizePromoCode(promo.Code)
	if !IsValidPromoCode(code) {
		return nil, ErrPromoCodeInvalidFormat
	}

	existing, err := r.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrPromoCodeAlreadyExists
	}

	builder := sq.Insert("promo_code").
		Columns(
			"code",
			"discount_percent",
			"is_active",
			"expires_at",
			"max_redemptions",
			"created_by_telegram_id",
		).
		Values(
			code,
			promo.DiscountPercent,
			promo.IsActive,
			promo.ExpiresAt,
			promo.MaxRedemptions,
			promo.CreatedByTelegramID,
		).
		Suffix("RETURNING " + strings.Join(promoCodeSelectColumns, ", ")).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build promo insert query: %w", err)
	}

	created := &PromoCode{}
	if err := scanPromoCode(r.pool.QueryRow(ctx, sql, args...), created); err != nil {
		return nil, fmt.Errorf("failed to create promo code: %w", err)
	}

	return created, nil
}

func (r *PromoCodeRepository) FindByCode(ctx context.Context, code string) (*PromoCode, error) {
	normalized := NormalizePromoCode(code)
	if normalized == "" {
		return nil, nil
	}

	builder := sq.Select(promoCodeSelectColumnsWithLiveCount("")...).
		From("promo_code").
		Where(sq.Eq{"code": normalized}).
		Where(sq.Expr("deleted_at IS NULL")).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build promo lookup query: %w", err)
	}

	promo := &PromoCode{}
	if err := scanPromoCode(r.pool.QueryRow(ctx, sql, args...), promo); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query promo code: %w", err)
	}

	return promo, nil
}

func (r *PromoCodeRepository) ListLatest(ctx context.Context, limit int) ([]PromoCode, error) {
	if limit <= 0 {
		limit = 20
	}

	builder := sq.Select(promoCodeSelectColumnsWithLiveCount("")...).
		From("promo_code").
		Where(sq.Expr("deleted_at IS NULL")).
		OrderBy("created_at DESC", "id DESC").
		Limit(uint64(limit)).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build promo list query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query promo codes: %w", err)
	}
	defer rows.Close()

	result := make([]PromoCode, 0, limit)
	for rows.Next() {
		var promo PromoCode
		if err := scanPromoCode(rows, &promo); err != nil {
			return nil, fmt.Errorf("failed to scan promo code: %w", err)
		}
		result = append(result, promo)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate promo codes: %w", err)
	}

	return result, nil
}

func (r *PromoCodeRepository) Delete(ctx context.Context, id int64) error {
	builder := sq.Update("promo_code").
		Set("code", sq.Expr("code || '__DELETED_' || id::text || '_' || floor(extract(epoch from now()))::text")).
		Set("is_active", false).
		Set("deleted_at", time.Now().UTC()).
		Set("updated_at", time.Now().UTC()).
		Where(sq.Eq{"id": id}).
		Where(sq.Expr("deleted_at IS NULL")).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build promo delete query: %w", err)
	}

	if _, err := r.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("failed to delete promo code: %w", err)
	}

	return nil
}

func (r *PromoCodeRepository) HasCustomerRedemption(ctx context.Context, promoCodeID int64, customerID int64) (bool, error) {
	builder := sq.Select("1").
		From("promo_code_redemption").
		Where(sq.Eq{
			"promo_code_id": promoCodeID,
			"customer_id":   customerID,
		}).
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := builder.ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build promo redemption query: %w", err)
	}

	var exists int
	if err := r.pool.QueryRow(ctx, sql, args...).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed to query promo redemption: %w", err)
	}

	return true, nil
}

func (r *PromoCodeRepository) CompleteRedemption(ctx context.Context, promo *PromoCode, customerID int64, purchaseID int64) error {
	if promo == nil || promo.ID == 0 || customerID == 0 || purchaseID == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin promo redemption tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var current PromoCode
	queryBuilder := sq.Select(promoCodeSelectColumnsWithLiveCount("")...).
		From("promo_code").
		Where(sq.Eq{"id": promo.ID}).
		Where(sq.Expr("deleted_at IS NULL")).
		Suffix("FOR UPDATE").
		PlaceholderFormat(sq.Dollar)

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build promo lock query: %w", err)
	}

	if err := scanPromoCode(tx.QueryRow(ctx, query, args...), &current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPromoCodeLimitReached
		}
		return fmt.Errorf("failed to lock promo code: %w", err)
	}

	if current.MaxRedemptions != nil && current.RedemptionCount >= *current.MaxRedemptions {
		return ErrPromoCodeLimitReached
	}

	insertBuilder := sq.Insert("promo_code_redemption").
		Columns("promo_code_id", "customer_id", "purchase_id").
		Values(current.ID, customerID, purchaseID).
		PlaceholderFormat(sq.Dollar)

	insertSQL, insertArgs, err := insertBuilder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build promo redemption insert: %w", err)
	}

	if _, err := tx.Exec(ctx, insertSQL, insertArgs...); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return ErrPromoCodeAlreadyUsed
		}
		return fmt.Errorf("failed to insert promo redemption: %w", err)
	}

	updateBuilder := sq.Update("promo_code").
		Set("redemption_count", sq.Expr("(SELECT COUNT(*)::int FROM promo_code_redemption WHERE promo_code_id = promo_code.id)")).
		Set("updated_at", time.Now().UTC()).
		Where(sq.Eq{"id": current.ID}).
		PlaceholderFormat(sq.Dollar)

	updateSQL, updateArgs, err := updateBuilder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build promo counter update: %w", err)
	}

	if _, err := tx.Exec(ctx, updateSQL, updateArgs...); err != nil {
		return fmt.Errorf("failed to update promo redemption count: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit promo redemption tx: %w", err)
	}

	return nil
}
