package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
)

var (
	ErrEmailChallengeCooldown = errors.New("email challenge cooldown")
	ErrEmailCodeInvalid       = errors.New("email code invalid")
	ErrEmailAlreadyRegistered = errors.New("email already registered")
	ErrEmailAlreadyLinked     = errors.New("email is already linked to this account")
	ErrTelegramAlreadyLinked  = errors.New("telegram identity is already linked")
	ErrEmailAccountHasHistory = errors.New("email account has activity and cannot be merged automatically")
	ErrTelegramAccountBlocked = errors.New("telegram account is blocked")
)

// EmailPasswordHash returns the credential hash without exposing it to the client.
func (cr *CustomerRepository) EmailPasswordHash(ctx context.Context, email string) (string, error) {
	var hash string
	err := cr.pool.QueryRow(ctx, `SELECT password_hash FROM customer_email_auth WHERE email = $1`, email).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return hash, err
}

func (cr *CustomerRepository) CreateEmailChallenge(ctx context.Context, id uuid.UUID, email, mode, passwordHash, codeHash string, expiresAt time.Time) error {
	var saved uuid.UUID
	err := cr.pool.QueryRow(ctx, `INSERT INTO email_auth_challenge (id, email, mode, password_hash, code_hash, expires_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)
		ON CONFLICT (email) DO UPDATE SET id = EXCLUDED.id, mode = EXCLUDED.mode,
		password_hash = EXCLUDED.password_hash, code_hash = EXCLUDED.code_hash,
		attempts = 0, expires_at = EXCLUDED.expires_at, created_at = NOW()
		WHERE email_auth_challenge.created_at <= NOW() - INTERVAL '60 seconds'
		RETURNING id`, id, email, mode, passwordHash, codeHash, expiresAt).Scan(&saved)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEmailChallengeCooldown
	}
	return err
}

func (cr *CustomerRepository) CreateEmailLinkChallenge(ctx context.Context, id uuid.UUID, customerID int64, email, passwordHash, codeHash string, expiresAt time.Time) error {
	var saved uuid.UUID
	err := cr.pool.QueryRow(ctx, `INSERT INTO email_auth_challenge (id, customer_id, email, mode, password_hash, code_hash, expires_at)
		VALUES ($1, $2, $3, 'link', $4, $5, $6)
		ON CONFLICT (email) DO UPDATE SET id = EXCLUDED.id, customer_id = EXCLUDED.customer_id,
		mode = 'link', password_hash = EXCLUDED.password_hash, code_hash = EXCLUDED.code_hash,
		attempts = 0, expires_at = EXCLUDED.expires_at, created_at = NOW()
		WHERE email_auth_challenge.created_at <= NOW() - INTERVAL '60 seconds'
		RETURNING id`, id, customerID, email, passwordHash, codeHash, expiresAt).Scan(&saved)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEmailChallengeCooldown
	}
	return err
}

func (cr *CustomerRepository) DeleteEmailChallenge(ctx context.Context, id uuid.UUID) {
	_, _ = cr.pool.Exec(ctx, `DELETE FROM email_auth_challenge WHERE id = $1`, id)
}

// CompleteEmailChallenge serializes attempts and consumes the code exactly once.
func (cr *CustomerRepository) CompleteEmailChallenge(ctx context.Context, id uuid.UUID, codeHash, language string) (*Customer, bool, error) {
	tx, err := cr.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)

	var email, mode, passwordHash, expectedHash string
	var attempts int
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `SELECT email, mode, COALESCE(password_hash, ''), code_hash, attempts, expires_at
		FROM email_auth_challenge WHERE id = $1 FOR UPDATE`, id).Scan(&email, &mode, &passwordHash, &expectedHash, &attempts, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, ErrEmailCodeInvalid
	}
	if err != nil {
		return nil, false, err
	}
	if attempts >= 5 || !expiresAt.After(time.Now().UTC()) || (mode != "register" && mode != "login") {
		return nil, false, ErrEmailCodeInvalid
	}
	if expectedHash != codeHash {
		_, err = tx.Exec(ctx, `UPDATE email_auth_challenge SET attempts = attempts + 1 WHERE id = $1`, id)
		if err != nil {
			return nil, false, err
		}
		if err = tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		return nil, false, ErrEmailCodeInvalid
	}

	var customerID int64
	if mode == "register" {
		if passwordHash == "" {
			return nil, false, fmt.Errorf("missing registration credential")
		}
		err = tx.QueryRow(ctx, `INSERT INTO customer (telegram_id, language, telegram_id_is_synthetic)
			VALUES (nextval('google_customer_telegram_id_seq'), $1, TRUE) RETURNING id`, language).Scan(&customerID)
		if err != nil {
			return nil, false, err
		}
		err = tx.QueryRow(ctx, `INSERT INTO customer_email_auth (customer_id, email, password_hash)
			VALUES ($1, $2, $3) ON CONFLICT (email) DO NOTHING RETURNING customer_id`, customerID, email, passwordHash).Scan(&customerID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, ErrEmailAlreadyRegistered
		}
	} else {
		err = tx.QueryRow(ctx, `SELECT customer_id FROM customer_email_auth WHERE email = $1`, email).Scan(&customerID)
	}
	if err != nil {
		return nil, false, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM email_auth_challenge WHERE id = $1`, id); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	customer, err := cr.FindById(ctx, customerID)
	return customer, mode == "register", err
}

// CompleteEmailLinkChallenge binds the verified address only to the account
// that requested it. The row lock consumes the code once, even under retries.
func (cr *CustomerRepository) CompleteEmailLinkChallenge(ctx context.Context, id uuid.UUID, customerID int64, codeHash string) (string, error) {
	tx, err := cr.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var email, mode, passwordHash, expectedHash string
	var ownerID int64
	var attempts int
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `SELECT email, mode, COALESCE(customer_id, 0), COALESCE(password_hash, ''), code_hash, attempts, expires_at
		FROM email_auth_challenge WHERE id = $1 FOR UPDATE`, id).Scan(&email, &mode, &ownerID, &passwordHash, &expectedHash, &attempts, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrEmailCodeInvalid
	}
	if err != nil {
		return "", err
	}
	if mode != "link" || ownerID != customerID || attempts >= 5 || !expiresAt.After(time.Now().UTC()) || passwordHash == "" {
		return "", ErrEmailCodeInvalid
	}
	if expectedHash != codeHash {
		if _, err = tx.Exec(ctx, `UPDATE email_auth_challenge SET attempts = attempts + 1 WHERE id = $1`, id); err != nil {
			return "", err
		}
		if err = tx.Commit(ctx); err != nil {
			return "", err
		}
		return "", ErrEmailCodeInvalid
	}
	var existing int64
	err = tx.QueryRow(ctx, `SELECT customer_id FROM customer_email_auth WHERE customer_id = $1 OR email = $2 LIMIT 1`, customerID, email).Scan(&existing)
	if err == nil {
		return "", ErrEmailAlreadyRegistered
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO customer_email_auth (customer_id, email, password_hash) VALUES ($1, $2, $3)`, customerID, email, passwordHash); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM email_auth_challenge WHERE id = $1`, id); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return email, nil
}

func (cr *CustomerRepository) FindByEmailIdentity(ctx context.Context, telegramID int64, email string) (*Customer, error) {
	var id int64
	err := cr.pool.QueryRow(ctx, `SELECT c.id FROM customer_email_auth e JOIN customer c ON c.id = e.customer_id WHERE c.telegram_id = $1 AND e.email = $2`, telegramID, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return cr.FindById(ctx, id)
}

func (cr *CustomerRepository) EmailForCustomer(ctx context.Context, customerID int64) (string, error) {
	var email string
	err := cr.pool.QueryRow(ctx, `SELECT email FROM customer_email_auth WHERE customer_id = $1`, customerID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return email, err
}

// LinkTelegramIdentity upgrades a synthetic email account when the verified
// Telegram identity is new. Existing Telegram accounts can absorb a fresh,
// unused email account; active purchases or user data require manual support.
func (cr *CustomerRepository) LinkTelegramIdentity(ctx context.Context, sourceID, telegramID int64, username string) (*Customer, error) {
	if sourceID <= 0 || telegramID <= 0 {
		return nil, ErrTelegramAlreadyLinked
	}
	tx, err := cr.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var synthetic bool
	var sourceTelegramID int64
	err = tx.QueryRow(ctx, `SELECT telegram_id, telegram_id_is_synthetic FROM customer WHERE id = $1 FOR UPDATE`, sourceID).Scan(&sourceTelegramID, &synthetic)
	if err != nil {
		return nil, err
	}
	if !synthetic {
		return nil, ErrTelegramAlreadyLinked
	}
	var email string
	err = tx.QueryRow(ctx, `SELECT email FROM customer_email_auth WHERE customer_id = $1 FOR UPDATE`, sourceID).Scan(&email)
	if err != nil {
		return nil, err
	}
	var targetID int64
	var targetBlocked bool
	err = tx.QueryRow(ctx, `SELECT id, is_blocked FROM customer WHERE telegram_id = $1 FOR UPDATE`, telegramID).Scan(&targetID, &targetBlocked)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// A new Telegram identity can keep the email account's purchases and
		// subscription. Referrals use telegram_id as a foreign key, so a
		// referenced synthetic ID needs a manual merge instead.
		var hasReferral bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referral WHERE referrer_id = $1 OR referee_id = $1)`, sourceTelegramID).Scan(&hasReferral)
		if err != nil {
			return nil, err
		}
		if hasReferral {
			return nil, ErrEmailAccountHasHistory
		}
		_, err = tx.Exec(ctx, `UPDATE customer SET telegram_id = $1, telegram_id_is_synthetic = FALSE WHERE id = $2`, telegramID, sourceID)
		if err != nil {
			return nil, err
		}
		targetID = sourceID
	} else {
		if targetBlocked {
			return nil, ErrTelegramAccountBlocked
		}
		// Absorbing an email profile into an existing Telegram profile is
		// safe only when deleting the source cannot discard user activity.
		var hasHistory bool
		err = tx.QueryRow(ctx, `SELECT c.trial_used OR c.expire_at IS NOT NULL OR c.autopay_enabled OR c.yookasa_payment_method_id IS NOT NULL
			OR EXISTS(SELECT 1 FROM purchase WHERE customer_id = c.id OR gift_recipient_customer_id = c.id)
			OR EXISTS(SELECT 1 FROM balance_transaction WHERE customer_id = c.id)
			OR EXISTS(SELECT 1 FROM balance_withdrawal WHERE customer_id = c.id)
			OR EXISTS(SELECT 1 FROM customer_subscription WHERE customer_id = c.id AND (position <> 1 OR display_name <> 'Основная' OR panel_user_id IS NOT NULL OR subscription_link IS NOT NULL OR expire_at IS NOT NULL))
			OR EXISTS(SELECT 1 FROM trial_activation_claim WHERE customer_id = c.id)
			OR EXISTS(SELECT 1 FROM subscription_notification_delivery WHERE customer_id = c.id)
			OR EXISTS(SELECT 1 FROM subscription_grace_delivery WHERE customer_id = c.id)
			OR EXISTS(SELECT 1 FROM promo_code_redemption WHERE customer_id = c.id)
			OR EXISTS(SELECT 1 FROM partner_application WHERE customer_id = c.id OR reviewed_by = c.id)
			OR EXISTS(SELECT 1 FROM partner WHERE customer_id = c.id)
			OR EXISTS(SELECT 1 FROM partner_referral WHERE customer_id = c.id)
			OR EXISTS(SELECT 1 FROM partner_commission WHERE customer_id = c.id)
			OR EXISTS(SELECT 1 FROM bot_direct_message_drafts WHERE customer_id = c.id)
			OR EXISTS(SELECT 1 FROM review WHERE customer_id = c.id)
			OR EXISTS(SELECT 1 FROM referral WHERE referrer_id = c.telegram_id OR referee_id = c.telegram_id)
			OR EXISTS(SELECT 1 FROM support_ticket WHERE customer_id = c.id)
			FROM customer c WHERE c.id = $1`, sourceID).Scan(&hasHistory)
		if err != nil {
			return nil, err
		}
		if hasHistory {
			return nil, ErrEmailAccountHasHistory
		}
		var sourceGoogleLinked bool
		if err = tx.QueryRow(ctx, `SELECT google_subject IS NOT NULL FROM customer WHERE id = $1`, sourceID).Scan(&sourceGoogleLinked); err != nil {
			return nil, err
		}
		if sourceGoogleLinked {
			return nil, ErrEmailAccountHasHistory
		}
		var occupied bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer_email_auth WHERE customer_id = $1)`, targetID).Scan(&occupied)
		if err != nil {
			return nil, err
		}
		if occupied {
			return nil, ErrTelegramAlreadyLinked
		}
		if _, err = tx.Exec(ctx, `UPDATE customer_email_auth SET customer_id = $1 WHERE customer_id = $2`, targetID, sourceID); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM customer WHERE id = $1`, sourceID); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	if username != "" {
		_ = cr.UpdateTelegramUsername(ctx, targetID, username)
	}
	return cr.FindById(ctx, targetID)
}
