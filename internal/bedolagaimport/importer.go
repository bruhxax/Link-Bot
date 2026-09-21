// Package bedolagaimport imports active Bedolaga accounts into Link-Bot.
package bedolagaimport

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

const maxSubscriptionsPerCustomer = 10

// Options contains database pools that have already been opened by the caller.
type Options struct {
	Source *pgxpool.Pool
	Target *pgxpool.Pool
	Apply  bool
}

// Report describes a migration without exposing user records or connection data.
type Report struct {
	Users                int
	ActiveSubscriptions  int
	Balances             int
	Referrals            int
	SkippedSubscriptions int
	CreatedCustomers     int
	UpdatedSubscriptions int
	CreatedSubscriptions int
	AppliedBalances      int
	AppliedReferrals     int
}

type sourceUser struct {
	ID            int64
	TelegramID    int64
	Username      string
	CreatedAt     time.Time
	BalanceCents  int64
	ReferrerID    *int64
	Subscriptions []sourceSubscription
}

type sourceSubscription struct {
	ID            int64
	Status        string
	IsTrial       bool
	CreatedAt     time.Time
	ExpireAt      *time.Time
	Link          string
	PanelUserID   *int64
	PanelUserUUID *uuid.UUID
}

type targetSubscription struct {
	ID            int64
	Position      int
	IsPrimary     bool
	PanelUserID   *int64
	PanelUserUUID *uuid.UUID
	Link          string
}

// Run reads and validates Bedolaga data, then optionally writes it
// as one serializable transaction to Link-Bot. Only active and trial subscriptions
// are imported: Link-Bot has no equivalent for Bedolaga's limited or disabled state.
func Run(ctx context.Context, options Options) (Report, error) {
	if options.Source == nil || options.Target == nil {
		return Report{}, errors.New("source and target database connections are required")
	}
	users, report, err := loadSource(ctx, options.Source)
	if err != nil {
		return Report{}, err
	}
	if err := validate(users); err != nil {
		return Report{}, err
	}
	if !options.Apply {
		return report, nil
	}
	if err := apply(ctx, options.Target, users, &report); err != nil {
		return Report{}, err
	}
	return report, nil
}

func loadSource(ctx context.Context, source *pgxpool.Pool) ([]sourceUser, Report, error) {
	rows, err := source.Query(ctx, `
		SELECT u.id, u.telegram_id, COALESCE(u.username, ''), u.created_at,
		       COALESCE(u.balance_kopeks, 0), u.referred_by_id,
		       s.id, s.status, s.is_trial, s.created_at, s.end_date,
		       COALESCE(s.subscription_url, ''), s.remnawave_id,
		       NULLIF(s.remnawave_uuid, '')
		FROM users AS u
		LEFT JOIN subscriptions AS s ON s.user_id = u.id
		WHERE u.telegram_id IS NOT NULL
		ORDER BY u.id, s.created_at DESC NULLS LAST, s.id DESC NULLS LAST
	`)
	if err != nil {
		return nil, Report{}, fmt.Errorf("read Bedolaga users and subscriptions: %w", err)
	}
	defer rows.Close()

	byID := make(map[int64]*sourceUser)
	ordered := make([]*sourceUser, 0)
	report := Report{}
	for rows.Next() {
		var (
			userID, telegramID, balance int64
			username                    string
			createdAt                   time.Time
			referrerID                  *int64
			subID                       *int64
			status                      *string
			isTrial                     *bool
			subCreatedAt                *time.Time
			expireAt                    *time.Time
			link                        *string
			panelUserID                 *int64
			panelUserUUIDText           *string
		)
		if err := rows.Scan(&userID, &telegramID, &username, &createdAt, &balance, &referrerID,
			&subID, &status, &isTrial, &subCreatedAt, &expireAt, &link, &panelUserID, &panelUserUUIDText); err != nil {
			return nil, Report{}, fmt.Errorf("scan Bedolaga account: %w", err)
		}
		user := byID[userID]
		if user == nil {
			user = &sourceUser{ID: userID, TelegramID: telegramID, Username: username, CreatedAt: createdAt, BalanceCents: balance, ReferrerID: referrerID}
			byID[userID] = user
			ordered = append(ordered, user)
			report.Users++
			if balance > 0 {
				report.Balances++
			}
			if referrerID != nil {
				report.Referrals++
			}
		}
		if subID == nil || status == nil || isTrial == nil || subCreatedAt == nil {
			continue
		}
		if !importableStatus(*status) {
			report.SkippedSubscriptions++
			continue
		}
		subscription := sourceSubscription{ID: *subID, Status: *status, IsTrial: *isTrial, CreatedAt: *subCreatedAt, ExpireAt: expireAt, PanelUserID: panelUserID}
		if link != nil {
			subscription.Link = strings.TrimSpace(*link)
		}
		if panelUserUUIDText != nil {
			parsed, err := uuid.Parse(strings.TrimSpace(*panelUserUUIDText))
			if err != nil {
				return nil, Report{}, fmt.Errorf("Bedolaga subscription %d has invalid Remnawave UUID: %w", subscription.ID, err)
			}
			subscription.PanelUserUUID = &parsed
		}
		if subscription.PanelUserID == nil && subscription.PanelUserUUID == nil && subscription.Link == "" {
			return nil, Report{}, fmt.Errorf("Bedolaga subscription %d has no Remnawave identity or subscription link", subscription.ID)
		}
		user.Subscriptions = append(user.Subscriptions, subscription)
		report.ActiveSubscriptions++
	}
	if err := rows.Err(); err != nil {
		return nil, Report{}, fmt.Errorf("iterate Bedolaga accounts: %w", err)
	}
	users := make([]sourceUser, 0, len(ordered))
	for _, user := range ordered {
		users = append(users, *user)
	}
	return users, report, nil
}

func importableStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "trial":
		return true
	default:
		return false
	}
}

func validate(users []sourceUser) error {
	for _, user := range users {
		if user.TelegramID <= 0 {
			return fmt.Errorf("Bedolaga user %d has invalid Telegram ID", user.ID)
		}
		if user.BalanceCents < 0 {
			return fmt.Errorf("Bedolaga user %d has a negative balance", user.ID)
		}
		if len(user.Subscriptions) > maxSubscriptionsPerCustomer {
			return fmt.Errorf("Bedolaga user %d has %d active/trial subscriptions; Link-Bot supports at most %d", user.ID, len(user.Subscriptions), maxSubscriptionsPerCustomer)
		}
	}
	return nil
}

func apply(ctx context.Context, target *pgxpool.Pool, users []sourceUser, report *Report) error {
	tx, err := target.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin Link-Bot migration transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, user := range users {
		customerID, created, err := upsertCustomer(ctx, tx, user)
		if err != nil {
			return err
		}
		if created {
			report.CreatedCustomers++
		}
		if err := importSubscriptions(ctx, tx, customerID, user, report); err != nil {
			return err
		}
		if user.BalanceCents > 0 {
			applied, err := importBalance(ctx, tx, customerID, user)
			if err != nil {
				return err
			}
			if applied {
				report.AppliedBalances++
			}
		}
	}
	telegramBySourceID := make(map[int64]int64, len(users))
	for _, user := range users {
		telegramBySourceID[user.ID] = user.TelegramID
	}
	for _, user := range users {
		if user.ReferrerID == nil {
			continue
		}
		referrerTelegramID, ok := telegramBySourceID[*user.ReferrerID]
		if !ok || referrerTelegramID == user.TelegramID {
			continue
		}
		applied, err := importReferral(ctx, tx, referrerTelegramID, user.TelegramID)
		if err != nil {
			return err
		}
		if applied {
			report.AppliedReferrals++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Link-Bot migration transaction: %w", err)
	}
	return nil
}

func upsertCustomer(ctx context.Context, tx pgx.Tx, user sourceUser) (int64, bool, error) {
	var customerID int64
	var inserted bool
	err := tx.QueryRow(ctx, `
		INSERT INTO customer (telegram_id, telegram_username, created_at, trial_used)
		VALUES ($1, NULLIF($2, ''), $3, TRUE)
		ON CONFLICT (telegram_id) DO UPDATE
		SET telegram_username = COALESCE(NULLIF(EXCLUDED.telegram_username, ''), customer.telegram_username),
		    trial_used = customer.trial_used OR EXCLUDED.trial_used
		RETURNING id, (xmax = 0)
	`, user.TelegramID, strings.TrimSpace(user.Username), user.CreatedAt).Scan(&customerID, &inserted)
	if err != nil {
		return 0, false, fmt.Errorf("upsert Link-Bot customer for Bedolaga user %d: %w", user.ID, err)
	}
	return customerID, inserted, nil
}

func importSubscriptions(ctx context.Context, tx pgx.Tx, customerID int64, user sourceUser, report *Report) error {
	if len(user.Subscriptions) == 0 {
		return nil
	}
	for _, source := range user.Subscriptions {
		if err := verifySubscriptionOwnership(ctx, tx, customerID, source); err != nil {
			return err
		}
	}
	targetSubscriptions, err := loadTargetSubscriptions(ctx, tx, customerID)
	if err != nil {
		return err
	}
	usedPositions := make(map[int]bool, len(targetSubscriptions))
	for _, item := range targetSubscriptions {
		usedPositions[item.Position] = true
	}
	for index, source := range user.Subscriptions {
		match, err := findTargetMatch(source, targetSubscriptions)
		if err != nil {
			return fmt.Errorf("resolve Bedolaga subscription %d: %w", source.ID, err)
		}
		isPrimary := index == 0
		if match == nil && isPrimary {
			for i := range targetSubscriptions {
				item := &targetSubscriptions[i]
				if item.IsPrimary && item.PanelUserID == nil && strings.TrimSpace(item.Link) == "" {
					match = item
					break
				}
			}
		}
		if match != nil {
			if err := updateSubscription(ctx, tx, match.ID, source); err != nil {
				return err
			}
			report.UpdatedSubscriptions++
			continue
		}
		position := nextPosition(usedPositions)
		if position == 0 {
			return fmt.Errorf("Link-Bot customer %d has no free subscription slots", customerID)
		}
		if err := insertSubscription(ctx, tx, customerID, position, isPrimary && len(targetSubscriptions) == 0, source, index); err != nil {
			return err
		}
		usedPositions[position] = true
		report.CreatedSubscriptions++
	}
	primary := user.Subscriptions[0]
	if _, err := tx.Exec(ctx, `
		UPDATE customer
		SET subscription_link = COALESCE(NULLIF($2, ''), subscription_link),
		    expire_at = CASE WHEN $3::timestamptz IS NULL THEN expire_at
		                     WHEN expire_at IS NULL OR expire_at < $3 THEN $3
		                     ELSE expire_at END
		WHERE id = $1
	`, customerID, primary.Link, primary.ExpireAt); err != nil {
		return fmt.Errorf("sync primary Link-Bot subscription for customer %d: %w", customerID, err)
	}
	return nil
}

func loadTargetSubscriptions(ctx context.Context, tx pgx.Tx, customerID int64) ([]targetSubscription, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, position, is_primary, panel_user_id, panel_user_uuid, COALESCE(subscription_link, '')
		FROM customer_subscription WHERE customer_id = $1 ORDER BY position FOR UPDATE
	`, customerID)
	if err != nil {
		return nil, fmt.Errorf("load Link-Bot subscriptions for customer %d: %w", customerID, err)
	}
	defer rows.Close()
	result := make([]targetSubscription, 0)
	for rows.Next() {
		var item targetSubscription
		if err := rows.Scan(&item.ID, &item.Position, &item.IsPrimary, &item.PanelUserID, &item.PanelUserUUID, &item.Link); err != nil {
			return nil, fmt.Errorf("scan Link-Bot subscription: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func findTargetMatch(source sourceSubscription, candidates []targetSubscription) (*targetSubscription, error) {
	for i := range candidates {
		candidate := &candidates[i]
		if source.PanelUserID != nil && candidate.PanelUserID != nil && *source.PanelUserID == *candidate.PanelUserID {
			return candidate, nil
		}
		if source.PanelUserUUID != nil && candidate.PanelUserUUID != nil && *source.PanelUserUUID == *candidate.PanelUserUUID {
			return candidate, nil
		}
		if source.Link != "" && strings.TrimSpace(source.Link) == strings.TrimSpace(candidate.Link) {
			return candidate, nil
		}
	}
	return nil, nil
}

func verifySubscriptionOwnership(ctx context.Context, tx pgx.Tx, customerID int64, source sourceSubscription) error {
	rows, err := tx.Query(ctx, `
		SELECT customer_id
		FROM customer_subscription
		WHERE ($1::bigint IS NOT NULL AND panel_user_id = $1)
		   OR ($2::uuid IS NOT NULL AND panel_user_uuid = $2)
		   OR (NULLIF($3, '') IS NOT NULL AND BTRIM(subscription_link) = BTRIM($3))
		FOR UPDATE
	`, source.PanelUserID, source.PanelUserUUID, source.Link)
	if err != nil {
		return fmt.Errorf("check Link-Bot ownership for Bedolaga subscription %d: %w", source.ID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var ownerID int64
		if err := rows.Scan(&ownerID); err != nil {
			return fmt.Errorf("read Link-Bot subscription owner: %w", err)
		}
		if ownerID != customerID {
			return fmt.Errorf("Bedolaga subscription %d is already linked to another Link-Bot customer", source.ID)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate Link-Bot subscription owners: %w", err)
	}
	return nil
}

func nextPosition(used map[int]bool) int {
	for position := 1; position <= maxSubscriptionsPerCustomer; position++ {
		if !used[position] {
			return position
		}
	}
	return 0
}

func updateSubscription(ctx context.Context, tx pgx.Tx, targetID int64, source sourceSubscription) error {
	if _, err := tx.Exec(ctx, `
		UPDATE customer_subscription
		SET panel_user_id = COALESCE($2, panel_user_id),
		    panel_user_uuid = COALESCE($3, panel_user_uuid),
		    subscription_link = COALESCE(NULLIF($4, ''), subscription_link),
		    expire_at = CASE WHEN $5::timestamptz IS NULL THEN expire_at
		                     WHEN expire_at IS NULL OR expire_at < $5 THEN $5
		                     ELSE expire_at END,
		    updated_at = NOW()
		WHERE id = $1
	`, targetID, source.PanelUserID, source.PanelUserUUID, source.Link, source.ExpireAt); err != nil {
		return fmt.Errorf("update Link-Bot subscription %d: %w", targetID, err)
	}
	return nil
}

func insertSubscription(ctx context.Context, tx pgx.Tx, customerID int64, position int, primary bool, source sourceSubscription, index int) error {
	displayName := fmt.Sprintf("Перенесена %d", index+1)
	if primary {
		displayName = "Основная"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO customer_subscription (
			customer_id, display_name, position, is_primary, panel_user_id,
			panel_user_uuid, subscription_link, expire_at
		) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8)
	`, customerID, displayName, position, primary, source.PanelUserID, source.PanelUserUUID, source.Link, source.ExpireAt); err != nil {
		return fmt.Errorf("insert Link-Bot subscription for Bedolaga subscription %d: %w", source.ID, err)
	}
	return nil
}

func importBalance(ctx context.Context, tx pgx.Tx, customerID int64, user sourceUser) (bool, error) {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", customerID); err != nil {
		return false, fmt.Errorf("lock Link-Bot balance for customer %d: %w", customerID, err)
	}
	var currentBalance int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(amount_cents), 0) FROM balance_transaction WHERE customer_id = $1`, customerID).Scan(&currentBalance); err != nil {
		return false, fmt.Errorf("read Link-Bot balance for customer %d: %w", customerID, err)
	}
	commandTag, err := tx.Exec(ctx, `
		INSERT INTO balance_transaction (customer_id, amount_cents, balance_after_cents, kind, reference_key, description)
		VALUES ($1, $2, $3, 'bedolaga_migration', $4, 'Баланс перенесён из Bedolaga')
		ON CONFLICT (reference_key) DO NOTHING
	`, customerID, user.BalanceCents, currentBalance+user.BalanceCents, fmt.Sprintf("bedolaga:user:%d:balance:v1", user.ID))
	if err != nil {
		return false, fmt.Errorf("import Bedolaga balance for user %d: %w", user.ID, err)
	}
	return commandTag.RowsAffected() == 1, nil
}

func importReferral(ctx context.Context, tx pgx.Tx, referrerTelegramID, refereeTelegramID int64) (bool, error) {
	commandTag, err := tx.Exec(ctx, `
		INSERT INTO referral (referrer_id, referee_id, used_at, bonus_granted)
		SELECT $1, $2, NOW(), TRUE
		WHERE NOT EXISTS (SELECT 1 FROM referral WHERE referee_id = $2)
	`, referrerTelegramID, refereeTelegramID)
	if err != nil {
		return false, fmt.Errorf("import referral %d -> %d: %w", referrerTelegramID, refereeTelegramID, err)
	}
	return commandTag.RowsAffected() == 1, nil
}
