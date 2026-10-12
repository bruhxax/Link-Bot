package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AdminUserSummary struct {
	AdminRole             string
	AdminColor            string
	CustomerID            int64
	TelegramID            int64
	TelegramUsername      string
	CreatedAt             time.Time
	TrialUsed             bool
	IsBlocked             bool
	SubscriptionName      string
	ExpireAt              *time.Time
	SubscriptionLink      string
	SubscriptionCount     int
	SubscriptionIsPrimary bool
	PanelUserID           *int64
	PanelUserUUID         *uuid.UUID
	PlanID                string
	PlanMonths            int
	PlanDays              int
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
		WITH matched_customers AS MATERIALIZED (
			SELECT c.* FROM customer c WHERE $1 = ''
			OR LOWER(COALESCE(c.telegram_username, '')) LIKE $2 OR c.telegram_id::TEXT LIKE $2
			OR EXISTS (SELECT 1 FROM customer_subscription search_subscription WHERE search_subscription.customer_id=c.id AND LOWER(search_subscription.display_name) LIKE $2)
			ORDER BY c.created_at DESC,c.id DESC LIMIT $3 OFFSET $4
		)
		SELECT c.id, c.telegram_id, COALESCE(c.telegram_username, ''), c.created_at,
		       c.trial_used, c.is_blocked, COALESCE(active_subscription.display_name, ''),
		       COALESCE(active_subscription.expire_at, c.expire_at), COALESCE(a.role_name,''), COALESCE(a.color,''),
		       COALESCE(active_subscription.subscription_link, c.subscription_link, ''),
		       active_subscription.panel_user_id, active_subscription.panel_user_uuid,
		       COALESCE(active_subscription.is_primary, TRUE),
		       (SELECT COUNT(*) FROM customer_subscription counted WHERE counted.customer_id = c.id),
		       COALESCE(term.plan_id,''),COALESCE(term.month,0),COALESCE(term.days,0)
		FROM matched_customers c
		LEFT JOIN administrator a ON a.customer_id=c.id
		LEFT JOIN LATERAL (
			SELECT s.id, s.display_name, s.expire_at, s.subscription_link, s.panel_user_id, s.panel_user_uuid, s.is_primary
			FROM customer_subscription s
			LEFT JOIN customer_subscription_selection selected
			  ON selected.customer_id = c.id AND selected.subscription_id = s.id
			WHERE s.customer_id = c.id
			ORDER BY (selected.subscription_id IS NOT NULL) DESC, s.is_primary DESC, s.position ASC, s.id ASC
			LIMIT 1
		) active_subscription ON TRUE
		LEFT JOIN LATERAL (
			SELECT p.plan_id,p.month,p.days FROM purchase p
			WHERE p.subscription_id=active_subscription.id AND p.status='paid' AND (p.month>0 OR p.days>0)
			ORDER BY COALESCE(p.paid_at,p.created_at) DESC,p.id DESC LIMIT 1
		) term ON TRUE
		ORDER BY c.created_at DESC, c.id DESC
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
			&item.AdminRole,
			&item.AdminColor,
			&item.SubscriptionLink,
			&item.PanelUserID,
			&item.PanelUserUUID,
			&item.SubscriptionIsPrimary,
			&item.SubscriptionCount,
			&item.PlanID,
			&item.PlanMonths,
			&item.PlanDays,
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

func (cr *CustomerRepository) SetBlocked(ctx context.Context, customerID int64, blocked bool, reason string) error {
	reason = strings.TrimSpace(reason)
	result, err := cr.pool.Exec(ctx, `
		UPDATE customer
		SET is_blocked = $2,
		    blocked_at = CASE WHEN $2 THEN NOW() ELSE NULL END,
		    blocked_reason = CASE WHEN $2 THEN NULLIF($3, '') ELSE NULL END,
		    autopay_enabled = CASE WHEN $2 THEN FALSE ELSE autopay_enabled END
		WHERE id = $1
	`, customerID, blocked, reason)
	if err != nil {
		return fmt.Errorf("set customer block status: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("customer not found")
	}
	return nil
}
