package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v4"
)

// LaveshkaQuote freezes the RUB price and USD invoice together. Pending
// payments must not be recalculated using a subsequently changed exchange rate.
type LaveshkaQuote struct {
	PurchaseID int64
	InvoiceID  string
	MerchantID int64
	RubMinor   int64
	USDMinor   int64
}

func (r *PaymentIntegrationRepository) SaveLaveshkaQuote(ctx context.Context, q LaveshkaQuote) error {
	tag, err := r.pool.Exec(ctx, `
        INSERT INTO laveshka_invoice_quote(purchase_id, invoice_id, merchant_id, rub_minor, usd_minor)
        VALUES ($1,$2,$3,$4,$5)
        ON CONFLICT (purchase_id) DO UPDATE SET purchase_id = EXCLUDED.purchase_id
        WHERE laveshka_invoice_quote.invoice_id = EXCLUDED.invoice_id
          AND laveshka_invoice_quote.merchant_id = EXCLUDED.merchant_id
          AND laveshka_invoice_quote.rub_minor = EXCLUDED.rub_minor
          AND laveshka_invoice_quote.usd_minor = EXCLUDED.usd_minor
    `, q.PurchaseID, q.InvoiceID, q.MerchantID, q.RubMinor, q.USDMinor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("Laveshka invoice quote already exists with different values")
	}
	return nil
}

func (r *PaymentIntegrationRepository) FindLaveshkaQuote(ctx context.Context, invoiceID string) (*LaveshkaQuote, error) {
	var q LaveshkaQuote
	err := r.pool.QueryRow(ctx, `SELECT purchase_id, invoice_id, merchant_id, rub_minor, usd_minor
        FROM laveshka_invoice_quote WHERE invoice_id=$1`, invoiceID).
		Scan(&q.PurchaseID, &q.InvoiceID, &q.MerchantID, &q.RubMinor, &q.USDMinor)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &q, nil
}
