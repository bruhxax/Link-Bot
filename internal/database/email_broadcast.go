package database

import (
	"context"
	"errors"
	"time"
)

type EmailBroadcastDraft struct {
	Status         string     `json:"status"`
	Subject        string     `json:"subject"`
	Body           string     `json:"body"`
	RecipientCount int        `json:"recipientCount"`
	SentCount      int        `json:"sentCount"`
	FailedCount    int        `json:"failedCount"`
	LastError      string     `json:"lastError,omitempty"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
}

var ErrEmailBroadcastRunning = errors.New("email broadcast is running")

func (cr *CustomerRepository) EmailBroadcastDraft(ctx context.Context) (*EmailBroadcastDraft, error) {
	var draft EmailBroadcastDraft
	err := cr.pool.QueryRow(ctx, `SELECT status, subject, body, recipient_count, sent_count, failed_count,
		last_error, started_at, finished_at FROM email_broadcast WHERE id = 1`).Scan(
		&draft.Status, &draft.Subject, &draft.Body, &draft.RecipientCount, &draft.SentCount,
		&draft.FailedCount, &draft.LastError, &draft.StartedAt, &draft.FinishedAt)
	if err != nil {
		return nil, err
	}
	return &draft, nil
}

func (cr *CustomerRepository) SaveEmailBroadcastDraft(ctx context.Context, subject, body string, updatedBy int64) (*EmailBroadcastDraft, error) {
	command, err := cr.pool.Exec(ctx, `UPDATE email_broadcast SET status = 'draft', subject = $1, body = $2,
		recipient_count = 0, sent_count = 0, failed_count = 0, last_error = '',
		started_at = NULL, finished_at = NULL, updated_by = $3, updated_at = NOW()
		WHERE id = 1 AND status <> 'running'`, subject, body, updatedBy)
	if err != nil {
		return nil, err
	}
	if command.RowsAffected() == 0 {
		return nil, ErrEmailBroadcastRunning
	}
	return cr.EmailBroadcastDraft(ctx)
}

func (cr *CustomerRepository) EmailBroadcastRecipients(ctx context.Context) ([]string, error) {
	rows, err := cr.pool.Query(ctx, `SELECT e.email FROM customer_email_auth e
		ORDER BY e.customer_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

func (cr *CustomerRepository) BeginEmailBroadcast(ctx context.Context, recipientCount int, updatedBy int64) (*EmailBroadcastDraft, error) {
	command, err := cr.pool.Exec(ctx, `UPDATE email_broadcast SET status = 'running', recipient_count = $1,
		sent_count = 0, failed_count = 0, last_error = '', started_at = NOW(),
		finished_at = NULL, updated_by = $2, updated_at = NOW()
		WHERE id = 1 AND status IN ('draft', 'finished', 'failed') AND subject <> '' AND body <> ''`, recipientCount, updatedBy)
	if err != nil {
		return nil, err
	}
	if command.RowsAffected() == 0 {
		return nil, ErrEmailBroadcastRunning
	}
	return cr.EmailBroadcastDraft(ctx)
}

func (cr *CustomerRepository) UpdateEmailBroadcastProgress(ctx context.Context, sent, failed int, lastError string) error {
	_, err := cr.pool.Exec(ctx, `UPDATE email_broadcast SET sent_count = $1, failed_count = $2,
		last_error = $3, updated_at = NOW() WHERE id = 1 AND status = 'running'`, sent, failed, lastError)
	return err
}

func (cr *CustomerRepository) FinishEmailBroadcast(ctx context.Context, status string, sent, failed int, lastError string) error {
	_, err := cr.pool.Exec(ctx, `UPDATE email_broadcast SET status = $1, sent_count = $2,
		failed_count = $3, last_error = $4, finished_at = NOW(), updated_at = NOW()
		WHERE id = 1 AND status = 'running'`, status, sent, failed, lastError)
	return err
}

func (cr *CustomerRepository) RecoverEmailBroadcast(ctx context.Context) error {
	_, err := cr.pool.Exec(ctx, `UPDATE email_broadcast SET status = 'failed',
		last_error = 'Рассылка прервана перезапуском сервера', finished_at = NOW(), updated_at = NOW()
		WHERE id = 1 AND status = 'running'`)
	return err
}
