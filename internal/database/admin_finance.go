package database

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AdminFinanceSummary contains settled revenue totals. Telegram Stars are kept
// separate from roubles so the admin panel never adds unlike currencies.
type AdminFinanceSummary struct {
	RevenueRub   float64
	RefundsRub   float64
	RevenueStars float64
	RefundsStars float64
	PaymentCount int
}

type AdminFinanceDaily struct {
	Date         time.Time
	RevenueRub   float64
	RefundsRub   float64
	RevenueStars float64
	RefundsStars float64
	PaymentCount int
}

type AdminFinancePayment struct {
	ID                        int64
	Amount                    float64
	Currency                  string
	Status                    PurchaseStatus
	InvoiceType               InvoiceType
	Month                     int
	PurchaseKind              PurchaseKind
	ExtraDevices              int
	TelegramID                int64
	TelegramUsername          string
	YookasaPaymentMethodTitle string
	OccurredAt                time.Time
	WasPaid                   bool
}

type AdminFinanceData struct {
	Summary      AdminFinanceSummary
	Daily        []AdminFinanceDaily
	Payments     []AdminFinancePayment
	PaymentTotal int
}

// LoadAdminFinance aggregates every payment provider stored in purchase. Free
// plans are not payments; balance purchases stay in history but are excluded
// from external revenue totals.
func (pr *PurchaseRepository) LoadAdminFinance(ctx context.Context, from, to time.Time, limit, offset int) (*AdminFinanceData, error) {
	if !from.Before(to) {
		return nil, fmt.Errorf("invalid finance range")
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}

	result := &AdminFinanceData{Payments: make([]AdminFinancePayment, 0, limit)}
	if err := pr.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(amount) FILTER (WHERE UPPER(COALESCE(NULLIF(currency, ''), 'RUB')) <> 'STARS'), 0),
			COALESCE(SUM(amount) FILTER (WHERE status = $3 AND UPPER(COALESCE(NULLIF(currency, ''), 'RUB')) <> 'STARS'), 0),
			COALESCE(SUM(amount) FILTER (WHERE UPPER(COALESCE(NULLIF(currency, ''), 'RUB')) = 'STARS'), 0),
			COALESCE(SUM(amount) FILTER (WHERE status = $3 AND UPPER(COALESCE(NULLIF(currency, ''), 'RUB')) = 'STARS'), 0),
			COUNT(*)
		FROM purchase
		WHERE paid_at >= $1 AND paid_at < $2
		  AND invoice_type NOT IN ($4, $5)
	`, from, to, PurchaseStatusCancel, InvoiceTypeFree, InvoiceTypeBalance).Scan(
		&result.Summary.RevenueRub,
		&result.Summary.RefundsRub,
		&result.Summary.RevenueStars,
		&result.Summary.RefundsStars,
		&result.Summary.PaymentCount,
	); err != nil {
		return nil, fmt.Errorf("load admin finance summary: %w", err)
	}

	rows, err := pr.pool.Query(ctx, `
		SELECT
			(paid_at AT TIME ZONE 'Europe/Moscow')::date,
			COALESCE(SUM(amount) FILTER (WHERE UPPER(COALESCE(NULLIF(currency, ''), 'RUB')) <> 'STARS'), 0),
			COALESCE(SUM(amount) FILTER (WHERE status = $3 AND UPPER(COALESCE(NULLIF(currency, ''), 'RUB')) <> 'STARS'), 0),
			COALESCE(SUM(amount) FILTER (WHERE UPPER(COALESCE(NULLIF(currency, ''), 'RUB')) = 'STARS'), 0),
			COALESCE(SUM(amount) FILTER (WHERE status = $3 AND UPPER(COALESCE(NULLIF(currency, ''), 'RUB')) = 'STARS'), 0),
			COUNT(*)
		FROM purchase
		WHERE paid_at >= $1 AND paid_at < $2
		  AND invoice_type NOT IN ($4, $5)
		GROUP BY 1
		ORDER BY 1
	`, from, to, PurchaseStatusCancel, InvoiceTypeFree, InvoiceTypeBalance)
	if err != nil {
		return nil, fmt.Errorf("load admin finance chart: %w", err)
	}
	for rows.Next() {
		var item AdminFinanceDaily
		if err := rows.Scan(&item.Date, &item.RevenueRub, &item.RefundsRub, &item.RevenueStars, &item.RefundsStars, &item.PaymentCount); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan admin finance chart: %w", err)
		}
		result.Daily = append(result.Daily, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate admin finance chart: %w", err)
	}
	rows.Close()

	if err := pr.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM purchase
		WHERE COALESCE(paid_at, created_at) >= $1 AND COALESCE(paid_at, created_at) < $2
		  AND invoice_type <> $3
	`, from, to, InvoiceTypeFree).Scan(&result.PaymentTotal); err != nil {
		return nil, fmt.Errorf("count admin finance payments: %w", err)
	}

	rows, err = pr.pool.Query(ctx, `
		SELECT p.id, p.amount, COALESCE(NULLIF(p.currency, ''), 'RUB'), p.status, p.invoice_type,
		       p.month, p.purchase_kind, p.extra_devices, c.telegram_id,
		       COALESCE(c.telegram_username, ''), COALESCE(p.yookasa_payment_method_title, ''),
		       COALESCE(p.paid_at, p.created_at), p.paid_at IS NOT NULL
		FROM purchase p
		JOIN customer c ON c.id = p.customer_id
		WHERE COALESCE(p.paid_at, p.created_at) >= $1 AND COALESCE(p.paid_at, p.created_at) < $2
		  AND p.invoice_type <> $3
		ORDER BY COALESCE(p.paid_at, p.created_at) DESC, p.id DESC
		LIMIT $4 OFFSET $5
	`, from, to, InvoiceTypeFree, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("load admin finance payments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item AdminFinancePayment
		if err := rows.Scan(
			&item.ID,
			&item.Amount,
			&item.Currency,
			&item.Status,
			&item.InvoiceType,
			&item.Month,
			&item.PurchaseKind,
			&item.ExtraDevices,
			&item.TelegramID,
			&item.TelegramUsername,
			&item.YookasaPaymentMethodTitle,
			&item.OccurredAt,
			&item.WasPaid,
		); err != nil {
			return nil, fmt.Errorf("scan admin finance payment: %w", err)
		}
		item.Currency = strings.ToUpper(strings.TrimSpace(item.Currency))
		item.TelegramUsername = strings.TrimSpace(item.TelegramUsername)
		item.YookasaPaymentMethodTitle = strings.TrimSpace(item.YookasaPaymentMethodTitle)
		result.Payments = append(result.Payments, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin finance payments: %w", err)
	}
	return result, nil
}
