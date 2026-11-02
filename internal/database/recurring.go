package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
)

type RecurringMethod struct {
	CustomerID       int64
	Provider         string
	Reference        string
	Title            string
	PayerPhone       *string
	PlanID           *string
	SourcePurchaseID int64
	Months           int
	Enabled          bool
	LastAttemptAt    *time.Time
	LastChargeStatus *string
	LastChargeError  *string
	SavedAt          time.Time
}

type RecurringSubscription struct {
	Provider         string
	ExternalID       string
	CustomerID       int64
	SourcePurchaseID int64
	PlanID           *string
	Months           int
	Amount           float64
	Active           bool
}

func (cr *CustomerRepository) SaveRecurringSubscription(ctx context.Context, subscription RecurringSubscription) error {
	if subscription.Provider == "" || subscription.ExternalID == "" || subscription.CustomerID <= 0 || subscription.SourcePurchaseID <= 0 || subscription.Months <= 0 || subscription.Amount <= 0 {
		return errors.New("invalid recurring subscription")
	}
	tag, err := cr.pool.Exec(ctx, `INSERT INTO recurring_subscription (provider, external_id, customer_id, source_purchase_id, plan_id, months, amount)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (provider, external_id) DO NOTHING`, subscription.Provider, subscription.ExternalID, subscription.CustomerID, subscription.SourcePurchaseID, subscription.PlanID, subscription.Months, subscription.Amount)
	if err != nil || tag.RowsAffected() > 0 {
		return err
	}
	existing, err := cr.FindRecurringSubscription(ctx, subscription.Provider, subscription.ExternalID)
	if err != nil {
		return err
	}
	if existing == nil || existing.CustomerID != subscription.CustomerID || existing.SourcePurchaseID != subscription.SourcePurchaseID || existing.Months != subscription.Months || existing.Amount != subscription.Amount {
		return errors.New("recurring subscription identity conflict")
	}
	return nil
}

func (cr *CustomerRepository) FindRecurringSubscription(ctx context.Context, provider, externalID string) (*RecurringSubscription, error) {
	result := &RecurringSubscription{}
	err := cr.pool.QueryRow(ctx, `SELECT provider, external_id, customer_id, source_purchase_id, plan_id, months, amount, active FROM recurring_subscription WHERE provider=$1 AND external_id=$2`, provider, externalID).
		Scan(&result.Provider, &result.ExternalID, &result.CustomerID, &result.SourcePurchaseID, &result.PlanID, &result.Months, &result.Amount, &result.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return result, err
}

func (cr *CustomerRepository) MarkRecurringSubscriptionStopped(ctx context.Context, provider, externalID string) error {
	_, err := cr.pool.Exec(ctx, `UPDATE recurring_subscription SET active=FALSE, stopped_at=NOW() WHERE provider=$1 AND external_id=$2`, provider, externalID)
	return err
}

func (cr *CustomerRepository) ListActiveRecurringSubscriptions(ctx context.Context, customerID int64) ([]RecurringSubscription, error) {
	rows, err := cr.pool.Query(ctx, `SELECT provider, external_id, customer_id, source_purchase_id, plan_id, months, amount, active FROM recurring_subscription WHERE customer_id=$1 AND active=TRUE ORDER BY created_at DESC`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RecurringSubscription{}
	for rows.Next() {
		var item RecurringSubscription
		if err := rows.Scan(&item.Provider, &item.ExternalID, &item.CustomerID, &item.SourcePurchaseID, &item.PlanID, &item.Months, &item.Amount, &item.Active); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (cr *CustomerRepository) ClaimDueLavaSubscriptions(ctx context.Context, limit int) ([]RecurringSubscription, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := cr.pool.Query(ctx, `WITH due AS (
		SELECT provider, external_id FROM recurring_subscription
		WHERE provider='lava' AND active=TRUE AND (last_checked_at IS NULL OR last_checked_at < NOW() - INTERVAL '2 minutes')
		ORDER BY last_checked_at NULLS FIRST, created_at
		LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE recurring_subscription s SET last_checked_at=NOW() FROM due
		WHERE s.provider=due.provider AND s.external_id=due.external_id
		RETURNING s.provider, s.external_id, s.customer_id, s.source_purchase_id, s.plan_id, s.months, s.amount, s.active
	) SELECT * FROM claimed`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RecurringSubscription{}
	for rows.Next() {
		var item RecurringSubscription
		if err := rows.Scan(&item.Provider, &item.ExternalID, &item.CustomerID, &item.SourcePurchaseID, &item.PlanID, &item.Months, &item.Amount, &item.Active); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (cr *CustomerRepository) FindRecurringMethod(ctx context.Context, customerID int64) (*RecurringMethod, error) {
	method := &RecurringMethod{}
	err := cr.pool.QueryRow(ctx, `SELECT customer_id, provider, reference, title, payer_phone, plan_id, source_purchase_id, months, enabled, last_attempt_at, last_charge_status, last_charge_error, saved_at FROM recurring_method WHERE customer_id=$1`, customerID).
		Scan(&method.CustomerID, &method.Provider, &method.Reference, &method.Title, &method.PayerPhone, &method.PlanID, &method.SourcePurchaseID, &method.Months, &method.Enabled, &method.LastAttemptAt, &method.LastChargeStatus, &method.LastChargeError, &method.SavedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find recurring method: %w", err)
	}
	return method, nil
}

func (cr *CustomerRepository) SaveRecurringMethod(ctx context.Context, method RecurringMethod) error {
	if method.CustomerID <= 0 || strings.TrimSpace(method.Provider) == "" || strings.TrimSpace(method.Reference) == "" || method.Months <= 0 {
		return errors.New("invalid recurring method")
	}
	_, err := cr.pool.Exec(ctx, `WITH allowed AS (
		SELECT purchase_id FROM recurring_enrollment WHERE purchase_id=$7 AND provider=$2 AND revoked_at IS NULL FOR UPDATE
	), saved AS (INSERT INTO recurring_method (customer_id, provider, reference, title, payer_phone, plan_id, source_purchase_id, months, enabled)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,TRUE FROM allowed WHERE TRUE
		ON CONFLICT (customer_id) DO UPDATE SET provider=EXCLUDED.provider, reference=EXCLUDED.reference,
			title=EXCLUDED.title, payer_phone=EXCLUDED.payer_phone, plan_id=EXCLUDED.plan_id, source_purchase_id=EXCLUDED.source_purchase_id, months=EXCLUDED.months,
			enabled=CASE WHEN recurring_method.source_purchase_id < EXCLUDED.source_purchase_id THEN TRUE ELSE recurring_method.enabled END,
			last_attempt_at=NULL, last_charge_status=NULL, last_charge_error=NULL, saved_at=NOW()
		WHERE recurring_method.source_purchase_id <= EXCLUDED.source_purchase_id
		RETURNING customer_id, months, enabled)
		UPDATE customer SET autopay_enabled=saved.enabled, autopay_plan_months=saved.months FROM saved WHERE customer.id=saved.customer_id`,
		method.CustomerID, method.Provider, method.Reference, method.Title, method.PayerPhone, method.PlanID, method.SourcePurchaseID, method.Months)
	if err != nil {
		return fmt.Errorf("save recurring method: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) SetRecurringEnabled(ctx context.Context, customerID int64, enabled bool) error {
	_, err := cr.pool.Exec(ctx, `WITH updated AS (UPDATE recurring_method SET enabled=$2 WHERE customer_id=$1 RETURNING customer_id)
		UPDATE customer SET autopay_enabled=$2 FROM updated WHERE customer.id=updated.customer_id`, customerID, enabled)
	if err != nil {
		return fmt.Errorf("set recurring enabled: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) RemoveRecurringMethod(ctx context.Context, customerID int64) error {
	_, err := cr.pool.Exec(ctx, `WITH deleted AS (DELETE FROM recurring_method WHERE customer_id=$1 RETURNING customer_id)
		UPDATE customer SET autopay_enabled=FALSE, autopay_plan_months=NULL FROM deleted WHERE customer.id=deleted.customer_id`, customerID)
	if err != nil {
		return fmt.Errorf("remove recurring method: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) CreateRecurringEnrollment(ctx context.Context, purchaseID int64, provider, phone string) error {
	if purchaseID <= 0 || strings.TrimSpace(provider) == "" {
		return errors.New("invalid recurring enrollment")
	}
	_, err := cr.pool.Exec(ctx, `INSERT INTO recurring_enrollment (purchase_id, provider, payer_phone) VALUES ($1,$2,$3) ON CONFLICT (purchase_id) DO NOTHING`, purchaseID, provider, phone)
	return err
}

func (cr *CustomerRepository) HasRecurringEnrollment(ctx context.Context, purchaseID int64, provider string) (bool, error) {
	var exists bool
	err := cr.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM recurring_enrollment WHERE purchase_id=$1 AND provider=$2 AND revoked_at IS NULL)`, purchaseID, provider).Scan(&exists)
	return exists, err
}

func (cr *CustomerRepository) RecurringEnrollmentPhone(ctx context.Context, purchaseID int64, provider string) (string, error) {
	var phone *string
	err := cr.pool.QueryRow(ctx, `SELECT payer_phone FROM recurring_enrollment WHERE purchase_id=$1 AND provider=$2`, purchaseID, provider).Scan(&phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil || phone == nil {
		return "", err
	}
	return *phone, nil
}

func (cr *CustomerRepository) SetRecurringEnrollmentReference(ctx context.Context, purchaseID int64, provider, reference string) error {
	if purchaseID <= 0 || reference == "" {
		return errors.New("invalid recurring enrollment reference")
	}
	_, err := cr.pool.Exec(ctx, `UPDATE recurring_enrollment SET reference=$3 WHERE purchase_id=$1 AND provider=$2`, purchaseID, provider, reference)
	return err
}

func (cr *CustomerRepository) RecurringEnrollmentReference(ctx context.Context, purchaseID int64, provider string) (string, error) {
	var reference *string
	err := cr.pool.QueryRow(ctx, `SELECT reference FROM recurring_enrollment WHERE purchase_id=$1 AND provider=$2 AND revoked_at IS NULL`, purchaseID, provider).Scan(&reference)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil || reference == nil {
		return "", err
	}
	return *reference, nil
}

func (cr *CustomerRepository) RevokeRecurringEnrollment(ctx context.Context, purchaseID int64) error {
	if purchaseID <= 0 {
		return nil
	}
	_, err := cr.pool.Exec(ctx, `UPDATE recurring_enrollment SET revoked_at=NOW() WHERE purchase_id=$1`, purchaseID)
	return err
}
