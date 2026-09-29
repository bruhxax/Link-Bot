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

func (cr *CustomerRepository) DeleteEmailChallenge(ctx context.Context, id uuid.UUID) {
	_, _ = cr.pool.Exec(ctx, `DELETE FROM email_auth_challenge WHERE id = $1`, id)
}

// CompleteEmailChallenge serializes attempts and consumes the code exactly once.
func (cr *CustomerRepository) CompleteEmailChallenge(ctx context.Context, id uuid.UUID, codeHash, language string) (*Customer, error) {
	tx, err := cr.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var email, mode, passwordHash, expectedHash string
	var attempts int
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `SELECT email, mode, COALESCE(password_hash, ''), code_hash, attempts, expires_at
		FROM email_auth_challenge WHERE id = $1 FOR UPDATE`, id).Scan(&email, &mode, &passwordHash, &expectedHash, &attempts, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEmailCodeInvalid
	}
	if err != nil {
		return nil, err
	}
	if attempts >= 5 || !expiresAt.After(time.Now().UTC()) {
		return nil, ErrEmailCodeInvalid
	}
	if expectedHash != codeHash {
		_, err = tx.Exec(ctx, `UPDATE email_auth_challenge SET attempts = attempts + 1 WHERE id = $1`, id)
		if err != nil {
			return nil, err
		}
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, ErrEmailCodeInvalid
	}

	var customerID int64
	if mode == "register" {
		if passwordHash == "" {
			return nil, fmt.Errorf("missing registration credential")
		}
		err = tx.QueryRow(ctx, `INSERT INTO customer (telegram_id, language, telegram_id_is_synthetic)
			VALUES (nextval('google_customer_telegram_id_seq'), $1, TRUE) RETURNING id`, language).Scan(&customerID)
		if err != nil {
			return nil, err
		}
		err = tx.QueryRow(ctx, `INSERT INTO customer_email_auth (customer_id, email, password_hash)
			VALUES ($1, $2, $3) ON CONFLICT (email) DO NOTHING RETURNING customer_id`, customerID, email, passwordHash).Scan(&customerID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEmailAlreadyRegistered
		}
	} else {
		err = tx.QueryRow(ctx, `SELECT customer_id FROM customer_email_auth WHERE email = $1`, email).Scan(&customerID)
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM email_auth_challenge WHERE id = $1`, id); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return cr.FindById(ctx, customerID)
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
	return email, err
}
