package database

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type AdminUserSummary struct {
	CustomerID       int64
	TelegramID       int64
	TelegramUsername string
	CreatedAt        time.Time
	TrialUsed        bool
	IsBlocked        bool
	SubscriptionName string
	ExpireAt         *time.Time
}

func normalizeAdminUserSearch(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "@")
}

func (cr *CustomerRepository) SearchAdminUsers(ctx context.Context, query string, limit, offset int) ([]AdminUserSummary, int, error) {
	if limit <= 0 || limit > 50 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}
	query = normalizeAdminUserSearch(query)
	search := "%" + query + "%"

	var total int
	if err := cr.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM customer c
		WHERE $1 = ''
		   OR LOWER(COALESCE(c.telegram_username, '')) LIKE $2
		   OR c.telegram_id::TEXT LIKE $2
		   OR EXISTS (
			SELECT 1 FROM customer_subscription s
			WHERE s.customer_id = c.id AND LOWER(s.display_name) LIKE $2
		   )
	`, query, search).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count admin users: %w", err)
	}

	rows, err := cr.pool.Query(ctx, `
		SELECT c.id, c.telegram_id, COALESCE(c.telegram_username, ''), c.created_at,
		       c.trial_used, c.is_blocked, COALESCE(active_subscription.display_name, ''),
		       COALESCE(active_subscription.expire_at, c.expire_at)
		FROM customer c
		LEFT JOIN LATERAL (
			SELECT s.display_name, s.expire_at
			FROM customer_subscription s
			LEFT JOIN customer_subscription_selection selected
			  ON selected.customer_id = c.id AND selected.subscription_id = s.id
			WHERE s.customer_id = c.id
			ORDER BY (selected.subscription_id IS NOT NULL) DESC, s.is_primary DESC, s.position ASC, s.id ASC
			LIMIT 1
		) active_subscription ON TRUE
		WHERE $1 = ''
		   OR LOWER(COALESCE(c.telegram_username, '')) LIKE $2
		   OR c.telegram_id::TEXT LIKE $2
		   OR EXISTS (
			SELECT 1 FROM customer_subscription search_subscription
			WHERE search_subscription.customer_id = c.id
			  AND LOWER(search_subscription.display_name) LIKE $2
		   )
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT $3 OFFSET $4
	`, query, search, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("search admin users: %w", err)
	}
	defer rows.Close()

	items := make([]AdminUserSummary, 0, limit)
	for rows.Next() {
		var item AdminUserSummary
		if err := rows.Scan(
			&item.CustomerID,
			&item.TelegramID,
			&item.TelegramUsername,
			&item.CreatedAt,
			&item.TrialUsed,
			&item.IsBlocked,
			&item.SubscriptionName,
			&item.ExpireAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan admin user: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate admin users: %w", err)
	}
	return items, total, nil
}

func (cr *CustomerRepository) SetBlocked(ctx context.Context, customerID int64, blocked bool) error {
	result, err := cr.pool.Exec(ctx, `
		UPDATE customer
		SET is_blocked = $2,
		    blocked_at = CASE WHEN $2 THEN NOW() ELSE NULL END,
		    autopay_enabled = CASE WHEN $2 THEN FALSE ELSE autopay_enabled END
		WHERE id = $1
	`, customerID, blocked)
	if err != nil {
		return fmt.Errorf("set customer block status: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("customer not found")
	}
	return nil
}
