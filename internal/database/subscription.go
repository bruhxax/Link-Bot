package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

const MaxCustomerSubscriptions = 3

var (
	ErrSubscriptionLimitReached     = errors.New("subscription limit reached")
	ErrCustomerSubscriptionNotFound = errors.New("customer subscription not found")
	ErrPrimarySubscriptionDelete    = errors.New("primary subscription cannot be deleted")
)

type CustomerSubscription struct {
	ID               int64      `db:"id"`
	CustomerID       int64      `db:"customer_id"`
	DisplayName      string     `db:"display_name"`
	Position         int        `db:"position"`
	IsPrimary        bool       `db:"is_primary"`
	PanelUserID      *int64     `db:"panel_user_id"`
	PanelUserUUID    *uuid.UUID `db:"panel_user_uuid"`
	SubscriptionLink *string    `db:"subscription_link"`
	ExpireAt         *time.Time `db:"expire_at"`
	CreatedAt        time.Time  `db:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at"`
}

type SubscriptionRepository struct {
	pool *pgxpool.Pool
}

func NewSubscriptionRepository(pool *pgxpool.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{pool: pool}
}

var customerSubscriptionSelectColumns = []string{
	"id",
	"customer_id",
	"display_name",
	"position",
	"is_primary",
	"panel_user_id",
	"panel_user_uuid",
	"subscription_link",
	"expire_at",
	"created_at",
	"updated_at",
}

func scanCustomerSubscription(scanner interface {
	Scan(dest ...interface{}) error
}, subscription *CustomerSubscription) error {
	return scanner.Scan(
		&subscription.ID,
		&subscription.CustomerID,
		&subscription.DisplayName,
		&subscription.Position,
		&subscription.IsPrimary,
		&subscription.PanelUserID,
		&subscription.PanelUserUUID,
		&subscription.SubscriptionLink,
		&subscription.ExpireAt,
		&subscription.CreatedAt,
		&subscription.UpdatedAt,
	)
}

func NormalizeSubscriptionDisplayName(value string) (string, error) {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return "", errors.New("subscription name is required")
	}
	if len([]rune(value)) > 40 {
		return "", errors.New("subscription name is too long")
	}
	return value, nil
}

func (sr *SubscriptionRepository) EnsurePrimary(ctx context.Context, customer *Customer) (*CustomerSubscription, error) {
	if customer == nil || customer.ID <= 0 {
		return nil, errors.New("customer is required")
	}
	_, err := sr.pool.Exec(ctx, `
		INSERT INTO customer_subscription (
			customer_id, display_name, position, is_primary, subscription_link, expire_at
		)
		VALUES ($1, 'Основная', 1, TRUE, $2, $3)
		ON CONFLICT (customer_id, position) DO NOTHING
	`, customer.ID, customer.SubscriptionLink, customer.ExpireAt)
	if err != nil {
		return nil, fmt.Errorf("ensure primary subscription: %w", err)
	}
	return sr.PrimaryByCustomer(ctx, customer.ID)
}

func (sr *SubscriptionRepository) PrimaryByCustomer(ctx context.Context, customerID int64) (*CustomerSubscription, error) {
	return sr.findOne(ctx, `
		SELECT id, customer_id, display_name, position, is_primary, panel_user_id,
		       panel_user_uuid, subscription_link, expire_at, created_at, updated_at
		FROM customer_subscription
		WHERE customer_id = $1 AND is_primary
		LIMIT 1
	`, customerID)
}

func (sr *SubscriptionRepository) FindByID(ctx context.Context, subscriptionID int64) (*CustomerSubscription, error) {
	return sr.findOne(ctx, `
		SELECT id, customer_id, display_name, position, is_primary, panel_user_id,
		       panel_user_uuid, subscription_link, expire_at, created_at, updated_at
		FROM customer_subscription
		WHERE id = $1
	`, subscriptionID)
}

func (sr *SubscriptionRepository) FindForCustomer(ctx context.Context, customerID, subscriptionID int64) (*CustomerSubscription, error) {
	return sr.findOne(ctx, `
		SELECT id, customer_id, display_name, position, is_primary, panel_user_id,
		       panel_user_uuid, subscription_link, expire_at, created_at, updated_at
		FROM customer_subscription
		WHERE id = $1 AND customer_id = $2
	`, subscriptionID, customerID)
}

func (sr *SubscriptionRepository) findOne(ctx context.Context, query string, args ...interface{}) (*CustomerSubscription, error) {
	var result CustomerSubscription
	if err := scanCustomerSubscription(sr.pool.QueryRow(ctx, query, args...), &result); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query customer subscription: %w", err)
	}
	return &result, nil
}

func (sr *SubscriptionRepository) ListByCustomer(ctx context.Context, customerID int64) ([]CustomerSubscription, error) {
	rows, err := sr.pool.Query(ctx, `
		SELECT id, customer_id, display_name, position, is_primary, panel_user_id,
		       panel_user_uuid, subscription_link, expire_at, created_at, updated_at
		FROM customer_subscription
		WHERE customer_id = $1
		ORDER BY position ASC, id ASC
	`, customerID)
	if err != nil {
		return nil, fmt.Errorf("list customer subscriptions: %w", err)
	}
	defer rows.Close()

	result := make([]CustomerSubscription, 0, MaxCustomerSubscriptions)
	for rows.Next() {
		var subscription CustomerSubscription
		if err := scanCustomerSubscription(rows, &subscription); err != nil {
			return nil, fmt.Errorf("scan customer subscription: %w", err)
		}
		result = append(result, subscription)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate customer subscriptions: %w", err)
	}
	return result, nil
}

func (sr *SubscriptionRepository) ActiveForCustomer(ctx context.Context, customer *Customer) (*CustomerSubscription, error) {
	primary, err := sr.EnsurePrimary(ctx, customer)
	if err != nil {
		return nil, err
	}
	selected, err := sr.findOne(ctx, `
		SELECT s.id, s.customer_id, s.display_name, s.position, s.is_primary, s.panel_user_id,
		       s.panel_user_uuid, s.subscription_link, s.expire_at, s.created_at, s.updated_at
		FROM customer_subscription_selection AS selected
		JOIN customer_subscription AS s ON s.id = selected.subscription_id
		WHERE selected.customer_id = $1 AND s.customer_id = $1
	`, customer.ID)
	if err != nil {
		return nil, err
	}
	if selected != nil {
		return selected, nil
	}
	return primary, nil
}

func (sr *SubscriptionRepository) SetActive(ctx context.Context, customerID, subscriptionID int64) error {
	result, err := sr.pool.Exec(ctx, `
		INSERT INTO customer_subscription_selection (customer_id, subscription_id, updated_at)
		SELECT $1, id, NOW()
		FROM customer_subscription
		WHERE id = $2 AND customer_id = $1
		ON CONFLICT (customer_id) DO UPDATE
		SET subscription_id = EXCLUDED.subscription_id,
		    updated_at = NOW()
	`, customerID, subscriptionID)
	if err != nil {
		return fmt.Errorf("select customer subscription: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrCustomerSubscriptionNotFound
	}
	return nil
}

func (sr *SubscriptionRepository) CreateAdditional(ctx context.Context, customerID int64, displayName string) (*CustomerSubscription, error) {
	name, err := NormalizeSubscriptionDisplayName(displayName)
	if err != nil {
		return nil, err
	}
	tx, err := sr.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create subscription: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", customerID); err != nil {
		return nil, fmt.Errorf("lock customer subscriptions: %w", err)
	}
	var positions []int
	rows, err := tx.Query(ctx, `SELECT position FROM customer_subscription WHERE customer_id = $1 ORDER BY position`, customerID)
	if err != nil {
		return nil, fmt.Errorf("load subscription positions: %w", err)
	}
	for rows.Next() {
		var position int
		if err := rows.Scan(&position); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan subscription position: %w", err)
		}
		positions = append(positions, position)
	}
	rows.Close()
	if len(positions) >= MaxCustomerSubscriptions {
		return nil, ErrSubscriptionLimitReached
	}
	used := make(map[int]bool, len(positions))
	for _, position := range positions {
		used[position] = true
	}
	position := 2
	for position <= MaxCustomerSubscriptions && used[position] {
		position++
	}
	if position > MaxCustomerSubscriptions {
		return nil, ErrSubscriptionLimitReached
	}

	var result CustomerSubscription
	if err := scanCustomerSubscription(tx.QueryRow(ctx, `
		INSERT INTO customer_subscription (customer_id, display_name, position, is_primary)
		VALUES ($1, $2, $3, FALSE)
		RETURNING id, customer_id, display_name, position, is_primary, panel_user_id,
		          panel_user_uuid, subscription_link, expire_at, created_at, updated_at
	`, customerID, name, position), &result); err != nil {
		return nil, fmt.Errorf("create customer subscription: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO customer_subscription_selection (customer_id, subscription_id, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (customer_id) DO UPDATE
		SET subscription_id = EXCLUDED.subscription_id,
		    updated_at = NOW()
	`, customerID, result.ID); err != nil {
		return nil, fmt.Errorf("select created subscription: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create subscription: %w", err)
	}
	return &result, nil
}

func (sr *SubscriptionRepository) Rename(ctx context.Context, customerID, subscriptionID int64, displayName string) error {
	name, err := NormalizeSubscriptionDisplayName(displayName)
	if err != nil {
		return err
	}
	result, err := sr.pool.Exec(ctx, `
		UPDATE customer_subscription
		SET display_name = $3, updated_at = NOW()
		WHERE id = $2 AND customer_id = $1
	`, customerID, subscriptionID, name)
	if err != nil {
		return fmt.Errorf("rename customer subscription: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrCustomerSubscriptionNotFound
	}
	return nil
}

func (sr *SubscriptionRepository) DeleteAdditional(ctx context.Context, customerID, subscriptionID int64) error {
	result, err := sr.pool.Exec(ctx, `
		DELETE FROM customer_subscription
		WHERE id = $2 AND customer_id = $1 AND NOT is_primary
	`, customerID, subscriptionID)
	if err != nil {
		return fmt.Errorf("delete customer subscription: %w", err)
	}
	if result.RowsAffected() == 0 {
		subscription, findErr := sr.FindForCustomer(ctx, customerID, subscriptionID)
		if findErr != nil {
			return findErr
		}
		if subscription != nil && subscription.IsPrimary {
			return ErrPrimarySubscriptionDelete
		}
		return ErrCustomerSubscriptionNotFound
	}
	return nil
}

func (sr *SubscriptionRepository) UpdatePanelState(ctx context.Context, subscription *CustomerSubscription, panelUserID int64, panelUserUUID uuid.UUID, subscriptionLink string, expireAt time.Time) error {
	link := strings.TrimSpace(subscriptionLink)
	var linkValue *string
	if link != "" {
		linkValue = &link
	}
	expire := expireAt.UTC()
	return sr.UpdatePanelAccess(ctx, subscription, panelUserID, panelUserUUID, linkValue, &expire)
}

func (sr *SubscriptionRepository) UpdatePanelAccess(ctx context.Context, subscription *CustomerSubscription, panelUserID int64, panelUserUUID uuid.UUID, subscriptionLink *string, expireAt *time.Time) error {
	if subscription == nil || subscription.ID <= 0 {
		return ErrCustomerSubscriptionNotFound
	}
	tx, err := sr.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin subscription state update: %w", err)
	}
	defer tx.Rollback(ctx)

	var uuidValue interface{}
	if panelUserUUID != uuid.Nil {
		uuidValue = panelUserUUID
	}
	var linkValue interface{}
	if subscriptionLink != nil {
		link := strings.TrimSpace(*subscriptionLink)
		if link != "" {
			linkValue = link
		}
	}
	var expireValue interface{}
	if expireAt != nil && !expireAt.IsZero() {
		expireValue = expireAt.UTC()
	}
	if _, err := tx.Exec(ctx, `
		UPDATE customer_subscription
		SET panel_user_id = COALESCE(NULLIF($2, 0), panel_user_id),
		    panel_user_uuid = $3,
		    subscription_link = $4,
		    expire_at = $5,
		    updated_at = NOW()
		WHERE id = $1
	`, subscription.ID, panelUserID, uuidValue, linkValue, expireValue); err != nil {
		return fmt.Errorf("update customer subscription state: %w", err)
	}
	if subscription.IsPrimary {
		if _, err := tx.Exec(ctx, `
			UPDATE customer
			SET subscription_link = NULLIF($2, ''), expire_at = $3
			WHERE id = $1
		`, subscription.CustomerID, linkValue, expireValue); err != nil {
			return fmt.Errorf("update primary subscription cache: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit subscription state update: %w", err)
	}
	return nil
}
