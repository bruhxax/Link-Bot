package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v4"
)

var ErrTrialIdentityAlreadyClaimed = errors.New("trial identity is already claimed")

func (cr *CustomerRepository) ReserveTrialActivation(ctx context.Context, customerID int64, googleSubject, browserDeviceHash string) error {
	if customerID <= 0 {
		return fmt.Errorf("invalid trial customer")
	}

	googleSubject = strings.TrimSpace(googleSubject)
	browserDeviceHash = strings.TrimSpace(browserDeviceHash)
	var claimID int64
	err := cr.pool.QueryRow(ctx, `
		INSERT INTO trial_activation_claim (customer_id, google_subject, browser_device_hash)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''))
		ON CONFLICT DO NOTHING
		RETURNING id
	`, customerID, googleSubject, browserDeviceHash).Scan(&claimID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTrialIdentityAlreadyClaimed
	}
	if err != nil {
		return fmt.Errorf("reserve trial activation: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) MarkTrialActivationGranted(ctx context.Context, customerID int64) error {
	command, err := cr.pool.Exec(ctx, `
		UPDATE trial_activation_claim
		SET status = 'granted', granted_at = COALESCE(granted_at, NOW())
		WHERE customer_id = $1
	`, customerID)
	if err != nil {
		return fmt.Errorf("mark trial activation granted: %w", err)
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("trial activation claim not found")
	}
	return nil
}

func (cr *CustomerRepository) ReleaseTrialActivation(ctx context.Context, customerID int64) error {
	_, err := cr.pool.Exec(ctx, `
		DELETE FROM trial_activation_claim
		WHERE customer_id = $1 AND status = 'processing'
	`, customerID)
	if err != nil {
		return fmt.Errorf("release trial activation: %w", err)
	}
	return nil
}
