package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"link-bot/utils"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type CustomerRepository struct {
	pool *pgxpool.Pool
}

var ErrSubscriptionTransferTargetNotFound = errors.New("subscription transfer target customer not found")

func NewCustomerRepository(poll *pgxpool.Pool) *CustomerRepository {
	return &CustomerRepository{pool: poll}
}

type Customer struct {
	ID                            int64      `db:"id"`
	TelegramID                    int64      `db:"telegram_id"`
	ExpireAt                      *time.Time `db:"expire_at"`
	CreatedAt                     time.Time  `db:"created_at"`
	SubscriptionLink              *string    `db:"subscription_link"`
	Language                      string     `db:"language"`
	ChannelSubscriptionVerifiedAt *time.Time `db:"channel_subscription_verified_at"`
	TrialUsed                     bool       `db:"trial_used"`
	AutoPaymentEnabled            bool       `db:"autopay_enabled"`
	AutoPaymentPlanMonths         *int       `db:"autopay_plan_months"`
	YookasaPaymentMethodID        *uuid.UUID `db:"yookasa_payment_method_id"`
	YookasaPaymentMethodType      *string    `db:"yookasa_payment_method_type"`
	YookasaPaymentMethodTitle     *string    `db:"yookasa_payment_method_title"`
	YookasaPaymentMethodSavedAt   *time.Time `db:"yookasa_payment_method_saved_at"`
	YookasaLastChargeAt           *time.Time `db:"yookasa_last_charge_at"`
	YookasaLastChargeStatus       *string    `db:"yookasa_last_charge_status"`
	YookasaLastChargeError        *string    `db:"yookasa_last_charge_error"`
	GoogleSubject                 *string    `db:"google_subject"`
	GoogleEmail                   *string    `db:"google_email"`
	GoogleEmailVerified           bool       `db:"google_email_verified"`
	GoogleLinkedAt                *time.Time `db:"google_linked_at"`
}

var customerSelectColumns = []string{
	"id",
	"telegram_id",
	"expire_at",
	"created_at",
	"subscription_link",
	"language",
	"channel_subscription_verified_at",
	"trial_used",
	"autopay_enabled",
	"autopay_plan_months",
	"yookasa_payment_method_id",
	"yookasa_payment_method_type",
	"yookasa_payment_method_title",
	"yookasa_payment_method_saved_at",
	"yookasa_last_charge_at",
	"yookasa_last_charge_status",
	"yookasa_last_charge_error",
	"google_subject",
	"google_email",
	"google_email_verified",
	"google_linked_at",
}

func scanCustomer(scanner interface {
	Scan(dest ...interface{}) error
}, customer *Customer) error {
	return scanner.Scan(
		&customer.ID,
		&customer.TelegramID,
		&customer.ExpireAt,
		&customer.CreatedAt,
		&customer.SubscriptionLink,
		&customer.Language,
		&customer.ChannelSubscriptionVerifiedAt,
		&customer.TrialUsed,
		&customer.AutoPaymentEnabled,
		&customer.AutoPaymentPlanMonths,
		&customer.YookasaPaymentMethodID,
		&customer.YookasaPaymentMethodType,
		&customer.YookasaPaymentMethodTitle,
		&customer.YookasaPaymentMethodSavedAt,
		&customer.YookasaLastChargeAt,
		&customer.YookasaLastChargeStatus,
		&customer.YookasaLastChargeError,
		&customer.GoogleSubject,
		&customer.GoogleEmail,
		&customer.GoogleEmailVerified,
		&customer.GoogleLinkedAt,
	)
}

func (cr *CustomerRepository) FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]Customer, error) {
	buildSelect := sq.Select(customerSelectColumns...).
		From("customer").
		Where(
			sq.And{
				sq.NotEq{"expire_at": nil},
				sq.GtOrEq{"expire_at": startDate},
				sq.LtOrEq{"expire_at": endDate},
			},
		).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query customers by expiration range: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var customer Customer
		if err := scanCustomer(rows, &customer); err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, customer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over customer rows: %w", err)
	}

	return &customers, nil
}

func (cr *CustomerRepository) ClaimSubscriptionNotification(ctx context.Context, customerID int64, expireAt time.Time, kind string) (bool, error) {
	result, err := cr.pool.Exec(ctx, `
		INSERT INTO subscription_notification_delivery (customer_id, expire_at, kind)
		VALUES ($1, $2, $3)
		ON CONFLICT (customer_id, expire_at, kind) DO NOTHING
	`, customerID, expireAt, kind)
	if err != nil {
		return false, fmt.Errorf("failed to claim subscription notification: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (cr *CustomerRepository) ReleaseSubscriptionNotification(ctx context.Context, customerID int64, expireAt time.Time, kind string) error {
	_, err := cr.pool.Exec(ctx, `
		DELETE FROM subscription_notification_delivery
		WHERE customer_id = $1 AND expire_at = $2 AND kind = $3
	`, customerID, expireAt, kind)
	if err != nil {
		return fmt.Errorf("failed to release subscription notification: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) FindAutoPaymentEligible(ctx context.Context, dueBefore time.Time) ([]Customer, error) {
	buildSelect := sq.Select(customerSelectColumns...).
		From("customer").
		Where(sq.And{
			sq.Eq{"autopay_enabled": true},
			sq.NotEq{"yookasa_payment_method_id": nil},
			sq.NotEq{"autopay_plan_months": nil},
			sq.NotEq{"expire_at": nil},
			sq.LtOrEq{"expire_at": dueBefore},
		}).
		OrderBy("expire_at ASC").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build auto payment query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query auto payment customers: %w", err)
	}
	defer rows.Close()

	customers := make([]Customer, 0)
	for rows.Next() {
		var customer Customer
		if err := scanCustomer(rows, &customer); err != nil {
			return nil, fmt.Errorf("failed to scan auto payment customer: %w", err)
		}
		customers = append(customers, customer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating auto payment customers: %w", err)
	}

	return customers, nil
}

func (cr *CustomerRepository) FindById(ctx context.Context, id int64) (*Customer, error) {
	buildSelect := sq.Select(customerSelectColumns...).
		From("customer").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	var customer Customer
	if err := scanCustomer(cr.pool.QueryRow(ctx, sql, args...), &customer); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query customer: %w", err)
	}

	return &customer, nil
}

func (cr *CustomerRepository) FindByTelegramId(ctx context.Context, telegramId int64) (*Customer, error) {
	buildSelect := sq.Select(customerSelectColumns...).
		From("customer").
		Where(sq.Eq{"telegram_id": telegramId}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	var customer Customer
	if err := scanCustomer(cr.pool.QueryRow(ctx, sql, args...), &customer); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query customer: %w", err)
	}

	return &customer, nil
}

func (cr *CustomerRepository) TransferSubscriptionCache(
	ctx context.Context,
	oldTelegramID int64,
	newTelegramID int64,
	expireAt time.Time,
	subscriptionLink string,
) error {
	if newTelegramID <= 0 {
		return ErrSubscriptionTransferTargetNotFound
	}

	tx, err := cr.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin subscription cache transfer: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		UPDATE customer
		SET expire_at = $2,
		    subscription_link = $3
		WHERE telegram_id = $1
	`, newTelegramID, expireAt.UTC(), strings.TrimSpace(subscriptionLink))
	if err != nil {
		return fmt.Errorf("update subscription transfer target: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrSubscriptionTransferTargetNotFound
	}

	if oldTelegramID > 0 && oldTelegramID != newTelegramID {
		if _, err := tx.Exec(ctx, `
			UPDATE customer
			SET expire_at = NULL,
			    subscription_link = NULL,
			    autopay_enabled = FALSE,
			    autopay_plan_months = NULL
			WHERE telegram_id = $1
		`, oldTelegramID); err != nil {
			return fmt.Errorf("clear previous subscription owner cache: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit subscription cache transfer: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) Create(ctx context.Context, customer *Customer) (*Customer, error) {
	return cr.FindOrCreate(ctx, customer)
}

func (cr *CustomerRepository) FindOrCreate(ctx context.Context, customer *Customer) (*Customer, error) {
	query := `
		INSERT INTO customer (
			telegram_id,
			expire_at,
			language,
			trial_used,
			autopay_enabled,
			autopay_plan_months,
			yookasa_payment_method_id,
			yookasa_payment_method_type,
			yookasa_payment_method_title,
			yookasa_payment_method_saved_at,
			yookasa_last_charge_at,
			yookasa_last_charge_status,
			yookasa_last_charge_error
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (telegram_id) DO UPDATE SET telegram_id = customer.telegram_id
			RETURNING id, telegram_id, expire_at, created_at, subscription_link, language, channel_subscription_verified_at, trial_used, autopay_enabled, autopay_plan_months,
			          yookasa_payment_method_id, yookasa_payment_method_type, yookasa_payment_method_title, yookasa_payment_method_saved_at,
			          yookasa_last_charge_at, yookasa_last_charge_status, yookasa_last_charge_error,
			          google_subject, google_email, google_email_verified, google_linked_at
		`

	row := cr.pool.QueryRow(
		ctx,
		query,
		customer.TelegramID,
		customer.ExpireAt,
		customer.Language,
		customer.TrialUsed,
		customer.AutoPaymentEnabled,
		customer.AutoPaymentPlanMonths,
		customer.YookasaPaymentMethodID,
		customer.YookasaPaymentMethodType,
		customer.YookasaPaymentMethodTitle,
		customer.YookasaPaymentMethodSavedAt,
		customer.YookasaLastChargeAt,
		customer.YookasaLastChargeStatus,
		customer.YookasaLastChargeError,
	)

	var result Customer
	if err := scanCustomer(row, &result); err != nil {
		return nil, fmt.Errorf("failed to find or create customer: %w", err)
	}

	slog.Info("user found or created in bot database", "telegramId", utils.MaskHalfInt64(result.TelegramID))
	return &result, nil
}

func (cr *CustomerRepository) FindByGoogleSubject(ctx context.Context, subject string) (*Customer, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, nil
	}

	buildSelect := sq.Select(customerSelectColumns...).
		From("customer").
		Where(sq.Eq{"google_subject": subject}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build google subject select query: %w", err)
	}

	var customer Customer
	if err := scanCustomer(cr.pool.QueryRow(ctx, sql, args...), &customer); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query customer by google subject: %w", err)
	}

	return &customer, nil
}

func (cr *CustomerRepository) FindByGoogleEmail(ctx context.Context, email string) (*Customer, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, nil
	}

	buildSelect := sq.Select(customerSelectColumns...).
		From("customer").
		Where("lower(google_email) = ?", email).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build google email select query: %w", err)
	}

	var customer Customer
	if err := scanCustomer(cr.pool.QueryRow(ctx, sql, args...), &customer); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query customer by google email: %w", err)
	}

	return &customer, nil
}

func (cr *CustomerRepository) LinkGoogleIdentity(ctx context.Context, customerID int64, subject, email string, verified bool) (*Customer, error) {
	subject = strings.TrimSpace(subject)
	email = strings.ToLower(strings.TrimSpace(email))
	if customerID <= 0 || subject == "" || email == "" {
		return nil, fmt.Errorf("invalid google identity link request")
	}

	query := `
		UPDATE customer
		SET google_subject = $2,
		    google_email = $3,
		    google_email_verified = $4,
		    google_linked_at = COALESCE(google_linked_at, NOW())
		WHERE id = $1
		RETURNING id, telegram_id, expire_at, created_at, subscription_link, language, channel_subscription_verified_at, trial_used, autopay_enabled, autopay_plan_months,
		          yookasa_payment_method_id, yookasa_payment_method_type, yookasa_payment_method_title, yookasa_payment_method_saved_at,
		          yookasa_last_charge_at, yookasa_last_charge_status, yookasa_last_charge_error,
		          google_subject, google_email, google_email_verified, google_linked_at
	`

	var result Customer
	if err := scanCustomer(cr.pool.QueryRow(ctx, query, customerID, subject, email, verified), &result); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no customer found with id: %s", utils.MaskHalfInt64(customerID))
		}
		return nil, fmt.Errorf("failed to link google identity: %w", err)
	}

	return &result, nil
}

func (cr *CustomerRepository) UpdateFields(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	buildUpdate := sq.Update("customer").
		PlaceholderFormat(sq.Dollar).
		Where(sq.Eq{"id": id})

	for field, value := range updates {
		buildUpdate = buildUpdate.Set(field, value)
	}

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update customer: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("no customer found with id: %s", utils.MaskHalfInt64(id))
	}

	return nil
}

func (cr *CustomerRepository) FindByTelegramIds(ctx context.Context, telegramIDs []int64) ([]Customer, error) {
	buildSelect := sq.Select(customerSelectColumns...).
		From("customer").
		Where(sq.Eq{"telegram_id": telegramIDs}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query customers: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var customer Customer
		if err := scanCustomer(rows, &customer); err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over customer rows: %w", err)
	}

	return customers, nil
}

func (cr *CustomerRepository) ListAllTelegramIDs(ctx context.Context) ([]int64, error) {
	buildSelect := sq.Select("telegram_id").
		From("customer").
		OrderBy("telegram_id ASC").
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build telegram id list query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query telegram ids: %w", err)
	}
	defer rows.Close()

	telegramIDs := make([]int64, 0)
	for rows.Next() {
		var telegramID int64
		if err := rows.Scan(&telegramID); err != nil {
			return nil, fmt.Errorf("failed to scan telegram id: %w", err)
		}
		telegramIDs = append(telegramIDs, telegramID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating telegram ids: %w", err)
	}

	return telegramIDs, nil
}

func (cr *CustomerRepository) CreateBatch(ctx context.Context, customers []Customer) error {
	if len(customers) == 0 {
		return nil
	}
	builder := sq.Insert("customer").
		Columns(
			"telegram_id",
			"expire_at",
			"language",
			"subscription_link",
			"trial_used",
			"autopay_enabled",
			"autopay_plan_months",
			"yookasa_payment_method_id",
			"yookasa_payment_method_type",
			"yookasa_payment_method_title",
			"yookasa_payment_method_saved_at",
			"yookasa_last_charge_at",
			"yookasa_last_charge_status",
			"yookasa_last_charge_error",
		).
		PlaceholderFormat(sq.Dollar)

	for _, cust := range customers {
		builder = builder.Values(
			cust.TelegramID,
			cust.ExpireAt,
			cust.Language,
			cust.SubscriptionLink,
			cust.TrialUsed,
			cust.AutoPaymentEnabled,
			cust.AutoPaymentPlanMonths,
			cust.YookasaPaymentMethodID,
			cust.YookasaPaymentMethodType,
			cust.YookasaPaymentMethodTitle,
			cust.YookasaPaymentMethodSavedAt,
			cust.YookasaLastChargeAt,
			cust.YookasaLastChargeStatus,
			cust.YookasaLastChargeError,
		)
	}

	sqlStr, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build batch insert query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("failed to execute batch insert: %w", err)
	}

	return nil
}

func (cr *CustomerRepository) UpdateBatch(ctx context.Context, customers []Customer) error {
	if len(customers) == 0 {
		return nil
	}
	query := "UPDATE customer SET expire_at = c.expire_at, subscription_link = c.subscription_link FROM (VALUES "
	var args []interface{}
	for i, cust := range customers {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("($%d::bigint, $%d::timestamp, $%d::text)", i*3+1, i*3+2, i*3+3)
		args = append(args, cust.TelegramID, cust.ExpireAt, cust.SubscriptionLink)
	}
	query += ") AS c(telegram_id, expire_at, subscription_link) WHERE customer.telegram_id = c.telegram_id"

	_, err := cr.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute batch update: %w", err)
	}

	return nil
}

func (cr *CustomerRepository) DeleteByNotInTelegramIds(ctx context.Context, telegramIDs []int64) error {
	if len(telegramIDs) == 0 {
		return nil
	}

	buildDelete := sq.Delete("customer").
		Where(sq.NotEq{"telegram_id": telegramIDs}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildDelete.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	if _, err = cr.pool.Exec(ctx, sqlStr, args...); err != nil {
		return fmt.Errorf("failed to execute delete query: %w", err)
	}

	return nil
}
