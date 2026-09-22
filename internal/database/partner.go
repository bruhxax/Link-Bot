package database

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type PartnerApplication struct {
	ID                   int64
	CustomerID           int64
	TelegramID           int64
	Username             string
	ResourceURL          string
	RequestedPercent     int
	ExpectedMonthlyUsers int
	Status               string
	CreatedAt            time.Time
}

type Partner struct {
	ID                int64
	CustomerID        int64
	TelegramID        int64
	Username          string
	Code              string
	CommissionPercent int
	IsActive          bool
	CreatedAt         time.Time
}

type PartnerStats struct {
	Visitors           int
	TrialUsers         int
	PayingUsers        int
	PurchaseCount      int
	Revenue            float64
	Commission         float64
	CommissionCurrency string
}

type PartnerRepository struct{ pool *pgxpool.Pool }

func NewPartnerRepository(pool *pgxpool.Pool) *PartnerRepository {
	return &PartnerRepository{pool: pool}
}

func (r *PartnerRepository) ApplicationForCustomer(ctx context.Context, customerID int64) (*PartnerApplication, error) {
	row := r.pool.QueryRow(ctx, `SELECT a.id, a.customer_id, c.telegram_id, COALESCE(c.telegram_username, ''), a.resource_url, a.requested_percent, a.expected_monthly_users, a.status, a.created_at FROM partner_application a JOIN customer c ON c.id = a.customer_id WHERE a.customer_id = $1`, customerID)
	return scanPartnerApplication(row)
}

func (r *PartnerRepository) CreateApplication(ctx context.Context, customerID int64, resourceURL string, percent, monthlyUsers int) (*PartnerApplication, error) {
	row := r.pool.QueryRow(ctx, `INSERT INTO partner_application (customer_id, resource_url, requested_percent, expected_monthly_users) VALUES ($1, $2, $3, $4) ON CONFLICT (customer_id) DO UPDATE SET resource_url = EXCLUDED.resource_url, requested_percent = EXCLUDED.requested_percent, expected_monthly_users = EXCLUDED.expected_monthly_users, status = 'pending', reviewed_at = NULL, reviewed_by = NULL, updated_at = NOW() WHERE partner_application.status <> 'approved' RETURNING id, customer_id, (SELECT telegram_id FROM customer WHERE id = customer_id), COALESCE((SELECT telegram_username FROM customer WHERE id = customer_id), ''), resource_url, requested_percent, expected_monthly_users, status, created_at`, customerID, resourceURL, percent, monthlyUsers)
	return scanPartnerApplication(row)
}

func scanPartnerApplication(row pgx.Row) (*PartnerApplication, error) {
	var item PartnerApplication
	err := row.Scan(&item.ID, &item.CustomerID, &item.TelegramID, &item.Username, &item.ResourceURL, &item.RequestedPercent, &item.ExpectedMonthlyUsers, &item.Status, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan partner application: %w", err)
	}
	return &item, nil
}

func (r *PartnerRepository) ListApplications(ctx context.Context) ([]PartnerApplication, error) {
	rows, err := r.pool.Query(ctx, `SELECT a.id, a.customer_id, c.telegram_id, COALESCE(c.telegram_username, ''), a.resource_url, a.requested_percent, a.expected_monthly_users, a.status, a.created_at FROM partner_application a JOIN customer c ON c.id = a.customer_id ORDER BY CASE a.status WHEN 'pending' THEN 0 WHEN 'approved' THEN 1 ELSE 2 END, a.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list partner applications: %w", err)
	}
	defer rows.Close()
	items := []PartnerApplication{}
	for rows.Next() {
		item, err := scanPartnerApplication(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *PartnerRepository) PartnerForCustomer(ctx context.Context, customerID int64) (*Partner, error) {
	row := r.pool.QueryRow(ctx, `SELECT p.id, p.customer_id, c.telegram_id, COALESCE(c.telegram_username, ''), p.code, p.commission_percent, p.is_active, p.created_at FROM partner p JOIN customer c ON c.id = p.customer_id WHERE p.customer_id = $1`, customerID)
	return scanPartner(row)
}

func scanPartner(row pgx.Row) (*Partner, error) {
	var item Partner
	err := row.Scan(&item.ID, &item.CustomerID, &item.TelegramID, &item.Username, &item.Code, &item.CommissionPercent, &item.IsActive, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan partner: %w", err)
	}
	return &item, nil
}

func (r *PartnerRepository) CreateOrActivate(ctx context.Context, customerID, applicationID int64, percent int) (*Partner, error) {
	for attempt := 0; attempt < 4; attempt++ {
		code, err := newPartnerCode()
		if err != nil {
			return nil, err
		}
		row := r.pool.QueryRow(ctx, `INSERT INTO partner (customer_id, application_id, code, commission_percent, is_active) VALUES ($1, NULLIF($2, 0), $3, $4, TRUE) ON CONFLICT (customer_id) DO UPDATE SET commission_percent = EXCLUDED.commission_percent, is_active = TRUE, application_id = COALESCE(EXCLUDED.application_id, partner.application_id), updated_at = NOW() RETURNING id, customer_id, (SELECT telegram_id FROM customer WHERE id = partner.customer_id), COALESCE((SELECT telegram_username FROM customer WHERE id = partner.customer_id), ''), code, commission_percent, is_active, created_at`, customerID, applicationID, code, percent)
		partner, err := scanPartner(row)
		if err == nil {
			return partner, nil
		}
		if !strings.Contains(err.Error(), "duplicate key") {
			return nil, err
		}
	}
	return nil, errors.New("generate unique partner code")
}

func newPartnerCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}

func (r *PartnerRepository) ReviewApplication(ctx context.Context, applicationID, adminCustomerID int64, approve bool, percent int) (*Partner, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var customerID int64
	if err := tx.QueryRow(ctx, `SELECT customer_id FROM partner_application WHERE id = $1 FOR UPDATE`, applicationID).Scan(&customerID); err != nil {
		return nil, fmt.Errorf("find partner application: %w", err)
	}
	if !approve {
		_, err = tx.Exec(ctx, `UPDATE partner_application SET status = 'rejected', reviewed_at = NOW(), reviewed_by = $2, updated_at = NOW() WHERE id = $1`, applicationID, adminCustomerID)
		if err != nil {
			return nil, err
		}
		return nil, tx.Commit(ctx)
	}
	if percent < 0 || percent > 100 {
		return nil, errors.New("invalid partner percent")
	}
	_, err = tx.Exec(ctx, `UPDATE partner_application SET status = 'approved', reviewed_at = NOW(), reviewed_by = $2, updated_at = NOW() WHERE id = $1`, applicationID, adminCustomerID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.CreateOrActivate(ctx, customerID, applicationID, percent)
}

func (r *PartnerRepository) SetActive(ctx context.Context, partnerID int64, active bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE partner SET is_active = $2, updated_at = NOW() WHERE id = $1`, partnerID, active)
	return err
}
func (r *PartnerRepository) SetPercent(ctx context.Context, partnerID int64, percent int) error {
	_, err := r.pool.Exec(ctx, `UPDATE partner SET commission_percent = $2, updated_at = NOW() WHERE id = $1`, partnerID, percent)
	return err
}

func (r *PartnerRepository) ListPartners(ctx context.Context) ([]Partner, error) {
	rows, err := r.pool.Query(ctx, `SELECT p.id, p.customer_id, c.telegram_id, COALESCE(c.telegram_username, ''), p.code, p.commission_percent, p.is_active, p.created_at FROM partner p JOIN customer c ON c.id = p.customer_id ORDER BY p.is_active DESC, p.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Partner{}
	for rows.Next() {
		item, err := scanPartner(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *PartnerRepository) FindActiveByCode(ctx context.Context, code string) (*Partner, error) {
	row := r.pool.QueryRow(ctx, `SELECT p.id, p.customer_id, c.telegram_id, COALESCE(c.telegram_username, ''), p.code, p.commission_percent, p.is_active, p.created_at FROM partner p JOIN customer c ON c.id = p.customer_id WHERE p.code = $1 AND p.is_active = TRUE`, strings.ToUpper(strings.TrimSpace(code)))
	return scanPartner(row)
}

func (r *PartnerRepository) AttachReferral(ctx context.Context, partnerID, customerID int64) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO partner_referral (partner_id, customer_id) VALUES ($1, $2) ON CONFLICT (customer_id) DO NOTHING`, partnerID, customerID)
	return err
}

func (r *PartnerRepository) Stats(ctx context.Context, partnerID int64) (PartnerStats, error) {
	var stats PartnerStats
	err := r.pool.QueryRow(ctx, `SELECT COUNT(pr.id), COUNT(pr.id) FILTER (WHERE c.trial_used), COUNT(DISTINCT p.customer_id) FILTER (WHERE p.status = 'paid'), COUNT(p.id) FILTER (WHERE p.status = 'paid'), COALESCE(SUM(p.amount) FILTER (WHERE p.status = 'paid' AND p.currency = 'RUB'), 0) FROM partner_referral pr JOIN customer c ON c.id = pr.customer_id LEFT JOIN purchase p ON p.customer_id = c.id WHERE pr.partner_id = $1`, partnerID).Scan(&stats.Visitors, &stats.TrialUsers, &stats.PayingUsers, &stats.PurchaseCount, &stats.Revenue)
	if err != nil {
		return stats, err
	}
	stats.CommissionCurrency = "RUB"
	err = r.pool.QueryRow(ctx, `SELECT COALESCE(SUM(commission_amount) FILTER (WHERE currency = 'RUB'), 0) FROM partner_commission WHERE partner_id = $1`, partnerID).Scan(&stats.Commission)
	return stats, err
}

func (r *PartnerRepository) AccrueCommission(ctx context.Context, purchase *Purchase) error {
	if purchase == nil || purchase.ID <= 0 || purchase.Status != PurchaseStatusPaid || purchase.Amount <= 0 || purchase.IsFreePlan || purchase.InvoiceType == InvoiceTypeFree || purchase.PurchaseKind == PurchaseKindGift {
		return nil
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO partner_commission (partner_id, customer_id, purchase_id, amount, currency, commission_percent, commission_amount) SELECT pr.partner_id, pr.customer_id, $1, $2, $3, p.commission_percent, ROUND(($2 * p.commission_percent::numeric / 100), 8) FROM partner_referral pr JOIN partner p ON p.id = pr.partner_id WHERE pr.customer_id = $4 AND p.is_active = TRUE ON CONFLICT (purchase_id) DO NOTHING`, purchase.ID, purchase.Amount, purchase.Currency, purchase.CustomerID)
	return err
}
