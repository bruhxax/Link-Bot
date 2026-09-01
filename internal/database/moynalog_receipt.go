package database

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

const (
	MoyNalogReceiptProcessing = "processing"
	MoyNalogReceiptSucceeded  = "succeeded"
	MoyNalogReceiptFailed     = "failed"
	MoyNalogReceiptUncertain  = "uncertain"
)

// MoyNalogReceipt is the local idempotency record for a purchase receipt.
// A purchase can be claimed only once, so payment callbacks cannot create
// duplicate income records in the taxpayer cabinet.
type MoyNalogReceipt struct {
	PurchaseID   int64      `json:"purchaseId"`
	Status       string     `json:"status"`
	ReceiptUUID  string     `json:"receiptUuid,omitempty"`
	ItemName     string     `json:"itemName"`
	Amount       float64    `json:"amount"`
	InvoiceType  string     `json:"invoiceType,omitempty"`
	Error        string     `json:"error,omitempty"`
	AttemptCount int        `json:"attemptCount"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	SucceededAt  *time.Time `json:"succeededAt,omitempty"`
}

type MoyNalogReceiptRepository struct {
	pool *pgxpool.Pool
}

func NewMoyNalogReceiptRepository(pool *pgxpool.Pool) *MoyNalogReceiptRepository {
	return &MoyNalogReceiptRepository{pool: pool}
}

func (r *MoyNalogReceiptRepository) Claim(ctx context.Context, purchaseID int64, amount float64, itemName string) (bool, error) {
	var claimed bool
	err := r.pool.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO moynalog_receipt (purchase_id, status, amount, item_name)
			VALUES ($1, 'processing', $2, $3)
			ON CONFLICT (purchase_id) DO NOTHING
			RETURNING TRUE
		)
		SELECT COALESCE((SELECT TRUE FROM inserted), FALSE)
	`, purchaseID, amount, strings.TrimSpace(itemName)).Scan(&claimed)
	return claimed, err
}

func (r *MoyNalogReceiptRepository) RetryFailed(ctx context.Context, purchaseID int64) (bool, error) {
	result, err := r.pool.Exec(ctx, `
		UPDATE moynalog_receipt
		SET status = 'processing', error = '', attempt_count = attempt_count + 1, updated_at = NOW()
		WHERE purchase_id = $1 AND status = 'failed'
	`, purchaseID)
	return err == nil && result.RowsAffected() == 1, err
}

func (r *MoyNalogReceiptRepository) MarkSucceeded(ctx context.Context, purchaseID int64, receiptUUID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE moynalog_receipt
		SET status = 'succeeded', receipt_uuid = $2, error = '', succeeded_at = NOW(), updated_at = NOW()
		WHERE purchase_id = $1 AND status = 'processing'
	`, purchaseID, strings.TrimSpace(receiptUUID))
	return err
}

func (r *MoyNalogReceiptRepository) MarkFailed(ctx context.Context, purchaseID int64, status, message string) error {
	if status != MoyNalogReceiptFailed && status != MoyNalogReceiptUncertain {
		status = MoyNalogReceiptFailed
	}
	message = strings.TrimSpace(message)
	if len([]rune(message)) > 1200 {
		message = string([]rune(message)[:1200])
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE moynalog_receipt
		SET status = $2, error = $3, updated_at = NOW()
		WHERE purchase_id = $1 AND status = 'processing'
	`, purchaseID, status, message)
	return err
}

// RecoverInterrupted turns abandoned in-flight operations into "uncertain".
// Retrying such an operation automatically could duplicate a receipt if FNS
// accepted it immediately before the process stopped.
func (r *MoyNalogReceiptRepository) RecoverInterrupted(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE moynalog_receipt
		SET status = 'uncertain',
		    error = 'Отправка была прервана. Проверьте чек в кабинете «Мой налог» вручную.',
		    updated_at = NOW()
		WHERE status = 'processing' AND updated_at < NOW() - INTERVAL '5 minutes'
	`)
	return err
}

func (r *MoyNalogReceiptRepository) List(ctx context.Context, limit int) ([]MoyNalogReceipt, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := r.pool.Query(ctx, `
		SELECT receipt.purchase_id, receipt.status, receipt.receipt_uuid, receipt.item_name,
		       receipt.amount, purchase.invoice_type, receipt.error, receipt.attempt_count,
		       receipt.created_at, receipt.updated_at, receipt.succeeded_at
		FROM moynalog_receipt AS receipt
		JOIN purchase ON purchase.id = receipt.purchase_id
		ORDER BY receipt.updated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]MoyNalogReceipt, 0, limit)
	for rows.Next() {
		var item MoyNalogReceipt
		if err := rows.Scan(&item.PurchaseID, &item.Status, &item.ReceiptUUID, &item.ItemName,
			&item.Amount, &item.InvoiceType, &item.Error, &item.AttemptCount,
			&item.CreatedAt, &item.UpdatedAt, &item.SucceededAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MoyNalogReceiptRepository) Get(ctx context.Context, purchaseID int64) (*MoyNalogReceipt, error) {
	var item MoyNalogReceipt
	err := r.pool.QueryRow(ctx, `
		SELECT receipt.purchase_id, receipt.status, receipt.receipt_uuid, receipt.item_name,
		       receipt.amount, purchase.invoice_type, receipt.error, receipt.attempt_count,
		       receipt.created_at, receipt.updated_at, receipt.succeeded_at
		FROM moynalog_receipt AS receipt
		JOIN purchase ON purchase.id = receipt.purchase_id
		WHERE receipt.purchase_id = $1
	`, purchaseID).Scan(&item.PurchaseID, &item.Status, &item.ReceiptUUID, &item.ItemName,
		&item.Amount, &item.InvoiceType, &item.Error, &item.AttemptCount,
		&item.CreatedAt, &item.UpdatedAt, &item.SucceededAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &item, err
}
