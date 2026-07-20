package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type InvoiceType string

const (
	InvoiceTypeCrypto    InvoiceType = "crypto"
	InvoiceTypeYookasa   InvoiceType = "yookasa"
	InvoiceTypeTelegram  InvoiceType = "telegram"
	InvoiceTypeTribute   InvoiceType = "tribute"
	InvoiceTypeLava      InvoiceType = "lava"
	InvoiceTypeWata      InvoiceType = "wata"
	InvoiceTypePlatega   InvoiceType = "platega"
	InvoiceTypeFreeKassa InvoiceType = "freekassa"
	InvoiceTypeHeleket   InvoiceType = "heleket"
)

type PurchaseStatus string

const (
	PurchaseStatusNew     PurchaseStatus = "new"
	PurchaseStatusPending PurchaseStatus = "pending"
	PurchaseStatusPaid    PurchaseStatus = "paid"
	PurchaseStatusCancel  PurchaseStatus = "cancel"
)

type Purchase struct {
	ID                        int64          `db:"id"`
	Amount                    float64        `db:"amount"`
	CustomerID                int64          `db:"customer_id"`
	CreatedAt                 time.Time      `db:"created_at"`
	Month                     int            `db:"month"`
	PlanID                    *string        `db:"plan_id"`
	TrafficLimitBytes         *int64         `db:"traffic_limit_bytes"`
	DeviceLimitCount          *int           `db:"device_limit_count"`
	PaidAt                    *time.Time     `db:"paid_at"`
	Currency                  string         `db:"currency"`
	ExpireAt                  *time.Time     `db:"expire_at"`
	Status                    PurchaseStatus `db:"status"`
	InvoiceType               InvoiceType    `db:"invoice_type"`
	PromoCodeID               *int64         `db:"promo_code_id"`
	PromoCodeSnapshot         *string        `db:"promo_code_snapshot"`
	PromoCodeDiscountPercent  *int           `db:"promo_code_discount_percent"`
	CryptoInvoiceID           *int64         `db:"crypto_invoice_id"`
	CryptoInvoiceLink         *string        `db:"crypto_invoice_url"`
	YookasaURL                *string        `db:"yookasa_url"`
	YookasaID                 *uuid.UUID     `db:"yookasa_id"`
	AgreementAccepted         bool           `db:"agreement_accepted"`
	IsAutoPayment             bool           `db:"is_auto_payment"`
	ParentPurchaseID          *int64         `db:"parent_purchase_id"`
	YookasaPaymentMethodID    *uuid.UUID     `db:"yookasa_payment_method_id"`
	YookasaPaymentMethodType  *string        `db:"yookasa_payment_method_type"`
	YookasaPaymentMethodTitle *string        `db:"yookasa_payment_method_title"`
	YookasaPaymentMethodSaved bool           `db:"yookasa_payment_method_saved"`
	ExternalPaymentID         *string        `db:"external_payment_id"`
	ExternalPaymentURL        *string        `db:"external_payment_url"`
}

type PurchaseRepository struct {
	pool *pgxpool.Pool
}

func NewPurchaseRepository(pool *pgxpool.Pool) *PurchaseRepository {
	return &PurchaseRepository{
		pool: pool,
	}
}

var purchaseSelectColumns = []string{
	"id",
	"amount",
	"customer_id",
	"created_at",
	"month",
	"plan_id",
	"traffic_limit_bytes",
	"device_limit_count",
	"paid_at",
	"currency",
	"expire_at",
	"status",
	"invoice_type",
	"promo_code_id",
	"promo_code_snapshot",
	"promo_code_discount_percent",
	"crypto_invoice_id",
	"crypto_invoice_url",
	"yookasa_url",
	"yookasa_id",
	"agreement_accepted",
	"is_auto_payment",
	"parent_purchase_id",
	"yookasa_payment_method_id",
	"yookasa_payment_method_type",
	"yookasa_payment_method_title",
	"yookasa_payment_method_saved",
	"external_payment_id",
	"external_payment_url",
}

func scanPurchase(scanner interface {
	Scan(dest ...interface{}) error
}, purchase *Purchase) error {
	return scanner.Scan(
		&purchase.ID,
		&purchase.Amount,
		&purchase.CustomerID,
		&purchase.CreatedAt,
		&purchase.Month,
		&purchase.PlanID,
		&purchase.TrafficLimitBytes,
		&purchase.DeviceLimitCount,
		&purchase.PaidAt,
		&purchase.Currency,
		&purchase.ExpireAt,
		&purchase.Status,
		&purchase.InvoiceType,
		&purchase.PromoCodeID,
		&purchase.PromoCodeSnapshot,
		&purchase.PromoCodeDiscountPercent,
		&purchase.CryptoInvoiceID,
		&purchase.CryptoInvoiceLink,
		&purchase.YookasaURL,
		&purchase.YookasaID,
		&purchase.AgreementAccepted,
		&purchase.IsAutoPayment,
		&purchase.ParentPurchaseID,
		&purchase.YookasaPaymentMethodID,
		&purchase.YookasaPaymentMethodType,
		&purchase.YookasaPaymentMethodTitle,
		&purchase.YookasaPaymentMethodSaved,
		&purchase.ExternalPaymentID,
		&purchase.ExternalPaymentURL,
	)
}

func (cr *PurchaseRepository) Create(ctx context.Context, purchase *Purchase) (int64, error) {
	buildInsert := sq.Insert("purchase").
		Columns(
			"amount",
			"customer_id",
			"month",
			"plan_id",
			"traffic_limit_bytes",
			"device_limit_count",
			"currency",
			"expire_at",
			"status",
			"invoice_type",
			"promo_code_id",
			"promo_code_snapshot",
			"promo_code_discount_percent",
			"crypto_invoice_id",
			"crypto_invoice_url",
			"yookasa_url",
			"yookasa_id",
			"agreement_accepted",
			"is_auto_payment",
			"parent_purchase_id",
			"yookasa_payment_method_id",
			"yookasa_payment_method_type",
			"yookasa_payment_method_title",
			"yookasa_payment_method_saved",
			"external_payment_id",
			"external_payment_url",
		).
		Values(
			purchase.Amount,
			purchase.CustomerID,
			purchase.Month,
			purchase.PlanID,
			purchase.TrafficLimitBytes,
			purchase.DeviceLimitCount,
			purchase.Currency,
			purchase.ExpireAt,
			purchase.Status,
			purchase.InvoiceType,
			purchase.PromoCodeID,
			purchase.PromoCodeSnapshot,
			purchase.PromoCodeDiscountPercent,
			purchase.CryptoInvoiceID,
			purchase.CryptoInvoiceLink,
			purchase.YookasaURL,
			purchase.YookasaID,
			purchase.AgreementAccepted,
			purchase.IsAutoPayment,
			purchase.ParentPurchaseID,
			purchase.YookasaPaymentMethodID,
			purchase.YookasaPaymentMethodType,
			purchase.YookasaPaymentMethodTitle,
			purchase.YookasaPaymentMethodSaved,
			purchase.ExternalPaymentID,
			purchase.ExternalPaymentURL,
		).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildInsert.ToSql()
	if err != nil {
		return 0, err
	}

	var id int64
	err = cr.pool.QueryRow(ctx, sql, args...).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (cr *PurchaseRepository) FindByInvoiceTypeAndStatus(ctx context.Context, invoiceType InvoiceType, status PurchaseStatus) (*[]Purchase, error) {
	buildSelect := sq.Select(purchaseSelectColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"invoice_type": invoiceType},
			sq.Eq{"status": status},
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query purchases: %w", err)
	}
	defer rows.Close()

	purchases := []Purchase{}
	for rows.Next() {
		purchase := Purchase{}
		if err := scanPurchase(rows, &purchase); err != nil {
			return nil, fmt.Errorf("failed to scan purchase: %w", err)
		}
		purchases = append(purchases, purchase)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return &purchases, nil
}

func (cr *PurchaseRepository) FindById(ctx context.Context, id int64) (*Purchase, error) {
	buildSelect := sq.Select(purchaseSelectColumns...).
		From("purchase").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}
	purchase := &Purchase{}

	if err := scanPurchase(cr.pool.QueryRow(ctx, sql, args...), purchase); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query purchase: %w", err)
	}

	return purchase, nil
}

func (cr *PurchaseRepository) FindByExternalPaymentID(ctx context.Context, invoiceType InvoiceType, externalID string) (*Purchase, error) {
	buildSelect := sq.Select(purchaseSelectColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"invoice_type": invoiceType},
			sq.Eq{"external_payment_id": externalID},
		}).
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}
	purchase := &Purchase{}
	if err := scanPurchase(cr.pool.QueryRow(ctx, sql, args...), purchase); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return purchase, nil
}

func (p *PurchaseRepository) UpdateFields(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	buildUpdate := sq.Update("purchase").
		PlaceholderFormat(sq.Dollar).
		Where(sq.Eq{"id": id})

	for field, value := range updates {
		buildUpdate = buildUpdate.Set(field, value)
	}

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := p.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update customer: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no customer found with id: %d", id)
	}

	return nil
}

func (pr *PurchaseRepository) MarkAsPaid(ctx context.Context, purchaseID int64) error {
	currentTime := time.Now()

	updates := map[string]interface{}{
		"status":  PurchaseStatusPaid,
		"paid_at": currentTime,
	}

	return pr.UpdateFields(ctx, purchaseID, updates)
}

func (pr *PurchaseRepository) GetOrAssignPaymentOrderNumber(ctx context.Context, purchaseID int64) (int64, error) {
	if purchaseID <= 0 {
		return 0, fmt.Errorf("invalid purchase id: %d", purchaseID)
	}

	tx, err := pr.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin payment order number transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Serializes successful-payment numbering without blocking unrelated purchases.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(827364115)); err != nil {
		return 0, fmt.Errorf("lock payment order numbering: %w", err)
	}

	var orderNumber int64
	err = tx.QueryRow(ctx, `
		SELECT order_number
		FROM payment_notification_order
		WHERE purchase_id = $1
	`, purchaseID).Scan(&orderNumber)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return 0, fmt.Errorf("commit existing payment order number: %w", err)
		}
		return orderNumber, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("query payment order number: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO payment_notification_order (purchase_id, order_number)
		SELECT $1, COALESCE(MAX(order_number), 0) + 1
		FROM payment_notification_order
		RETURNING order_number
	`, purchaseID).Scan(&orderNumber)
	if err != nil {
		return 0, fmt.Errorf("assign payment order number: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit payment order number: %w", err)
	}
	return orderNumber, nil
}

func buildLatestActiveTributesQuery(customerIDs []int64) sq.SelectBuilder {
	return sq.
		Select(purchaseSelectColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"invoice_type": InvoiceTypeTribute},
			sq.Eq{"customer_id": customerIDs},
			sq.Expr("created_at = (SELECT MAX(created_at) FROM purchase p2 WHERE p2.customer_id = purchase.customer_id AND p2.invoice_type = ?)", InvoiceTypeTribute),
		}).
		Where(sq.NotEq{"status": PurchaseStatusCancel})
}

func (pr *PurchaseRepository) FindLatestActiveTributesByCustomerIDs(
	ctx context.Context,
	customerIDs []int64,
) (*[]Purchase, error) {
	if len(customerIDs) == 0 {
		empty := make([]Purchase, 0)
		return &empty, nil
	}

	builder := buildLatestActiveTributesQuery(customerIDs).PlaceholderFormat(sq.Dollar)

	sql, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := pr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query purchases: %w", err)
	}
	defer rows.Close()

	var purchases []Purchase
	for rows.Next() {
		var p Purchase
		if err := scanPurchase(rows, &p); err != nil {
			return nil, fmt.Errorf("scan purchase: %w", err)
		}
		purchases = append(purchases, p)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return &purchases, nil
}

func (pr *PurchaseRepository) FindByCustomerIDAndInvoiceTypeLast(
	ctx context.Context,
	customerID int64,
	invoiceType InvoiceType,
) (*Purchase, error) {

	query := sq.Select(purchaseSelectColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"invoice_type": invoiceType},
		}).
		OrderBy("created_at DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	p := &Purchase{}
	if err := scanPurchase(pr.pool.QueryRow(ctx, sql, args...), p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query purchase: %w", err)
	}

	return p, nil
}

func (pr *PurchaseRepository) FindSuccessfulPaidPurchaseByCustomer(ctx context.Context, customerID int64) (*Purchase, error) {
	query := sq.Select(purchaseSelectColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"status": PurchaseStatusPaid},
			sq.Or{
				sq.Eq{"invoice_type": InvoiceTypeCrypto},
				sq.Eq{"invoice_type": InvoiceTypeYookasa},
			},
		}).
		OrderBy("paid_at DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	p := &Purchase{}
	if err := scanPurchase(pr.pool.QueryRow(ctx, sql, args...), p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query purchase: %w", err)
	}

	return p, nil
}

func (pr *PurchaseRepository) FindSuccessfulPurchaseByCustomer(ctx context.Context, customerID int64) (*Purchase, error) {
	query := sq.Select(purchaseSelectColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"status": PurchaseStatusPaid},
		}).
		OrderBy("paid_at DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	p := &Purchase{}
	if err := scanPurchase(pr.pool.QueryRow(ctx, sql, args...), p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query purchase: %w", err)
	}

	return p, nil
}

func (pr *PurchaseRepository) FindHighestSuccessfulPurchaseByCustomer(ctx context.Context, customerID int64) (*Purchase, error) {
	query := sq.Select(purchaseSelectColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"status": PurchaseStatusPaid},
			sq.Gt{"month": 0},
		}).
		OrderBy("month DESC", "paid_at DESC NULLS LAST", "created_at DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	p := &Purchase{}
	if err := scanPurchase(pr.pool.QueryRow(ctx, sql, args...), p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query purchase: %w", err)
	}

	return p, nil
}

func (pr *PurchaseRepository) FindLatestSuccessfulYookasaPurchaseByCustomer(ctx context.Context, customerID int64) (*Purchase, error) {
	query := sq.Select(purchaseSelectColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"status": PurchaseStatusPaid},
			sq.Eq{"invoice_type": InvoiceTypeYookasa},
		}).
		OrderBy("paid_at DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	p := &Purchase{}
	if err := scanPurchase(pr.pool.QueryRow(ctx, sql, args...), p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query purchase: %w", err)
	}

	return p, nil
}

func (pr *PurchaseRepository) HasPendingAutoPaymentByCustomer(ctx context.Context, customerID int64) (bool, error) {
	query := sq.Select("1").
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"is_auto_payment": true},
			sq.Or{
				sq.Eq{"status": PurchaseStatusNew},
				sq.Eq{"status": PurchaseStatusPending},
			},
		}).
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("build query: %w", err)
	}

	var exists int
	err = pr.pool.QueryRow(ctx, sql, args...).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("query pending auto payment: %w", err)
	}

	return true, nil
}

func (pr *PurchaseRepository) HasPendingPromoPurchaseByCustomer(ctx context.Context, customerID int64, promoCodeID int64) (bool, error) {
	query := sq.Select("1").
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"promo_code_id": promoCodeID},
			sq.Or{
				sq.Eq{"status": PurchaseStatusNew},
				sq.Eq{"status": PurchaseStatusPending},
			},
		}).
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("build query: %w", err)
	}

	var exists int
	if err := pr.pool.QueryRow(ctx, sql, args...).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("query purchase: %w", err)
	}

	return true, nil
}

func (pr *PurchaseRepository) CountActivePromoReservations(ctx context.Context, promoCodeID int64) (int, error) {
	query := sq.Select("COUNT(*)::int").
		From("purchase").
		Where(sq.And{
			sq.Eq{"promo_code_id": promoCodeID},
			sq.Or{
				sq.Eq{"status": PurchaseStatusNew},
				sq.Eq{"status": PurchaseStatusPending},
			},
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	var count int
	if err := pr.pool.QueryRow(ctx, sql, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("query promo reservations: %w", err)
	}

	return count, nil
}

func (pr *PurchaseRepository) ListByCustomer(ctx context.Context, customerID int64, limit int) ([]Purchase, error) {
	if limit <= 0 {
		limit = 20
	}

	query := sq.Select(purchaseSelectColumns...).
		From("purchase").
		Where(sq.Eq{"customer_id": customerID}).
		OrderBy("created_at DESC").
		Limit(uint64(limit)).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := pr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query purchases: %w", err)
	}
	defer rows.Close()

	result := make([]Purchase, 0)
	for rows.Next() {
		var p Purchase
		if err := scanPurchase(rows, &p); err != nil {
			return nil, fmt.Errorf("scan purchase: %w", err)
		}
		result = append(result, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchases: %w", err)
	}

	return result, nil
}
