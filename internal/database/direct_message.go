package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
)

type DirectMessageDraft struct {
	AdminTelegramID int64      `json:"-"`
	CustomerID      int64      `json:"customerId"`
	Status          string     `json:"status"`
	SourceChatID    *int64     `json:"-"`
	SourceMessageID *int       `json:"-"`
	SourceKind      string     `json:"sourceKind"`
	SourcePreview   string     `json:"sourcePreview"`
	SourceHTML      string     `json:"-"`
	PreviewedAt     *time.Time `json:"previewedAt,omitempty"`
	SentAt          *time.Time `json:"sentAt,omitempty"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func (d *DirectMessageDraft) HasSource() bool {
	return d != nil && d.SourceChatID != nil && d.SourceMessageID != nil && *d.SourceMessageID > 0
}

const directMessageColumns = `admin_telegram_id, customer_id, status, source_chat_id, source_message_id,
       source_kind, source_preview, source_html, previewed_at, sent_at, updated_at`

func (r *BroadcastRepository) GetDirect(ctx context.Context, adminID int64) (*DirectMessageDraft, error) {
	return scanDirectMessage(r.pool.QueryRow(ctx, `SELECT `+directMessageColumns+` FROM bot_direct_message_drafts WHERE admin_telegram_id = $1`, adminID))
}

func (r *BroadcastRepository) StartDirectCapture(ctx context.Context, adminID, customerID int64) (*DirectMessageDraft, error) {
	return scanDirectMessage(r.pool.QueryRow(ctx, `
		INSERT INTO bot_direct_message_drafts (admin_telegram_id, customer_id) VALUES ($1, $2)
		ON CONFLICT (admin_telegram_id) DO UPDATE SET
			customer_id = EXCLUDED.customer_id, status = 'awaiting_message', source_chat_id = NULL,
			source_message_id = NULL, source_kind = '', source_preview = '', source_html = '',
			previewed_at = NULL, sent_at = NULL, updated_at = NOW()
		WHERE bot_direct_message_drafts.status <> 'sending'
		RETURNING `+directMessageColumns, adminID, customerID))
}

func (r *BroadcastRepository) CancelDirectCapture(ctx context.Context, adminID int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM bot_direct_message_drafts WHERE admin_telegram_id = $1 AND status = 'awaiting_message'`, adminID)
	return err
}

func (r *BroadcastRepository) SaveDirectSource(ctx context.Context, adminID int64, messageID int, kind, preview, sourceHTML string) (*DirectMessageDraft, error) {
	return scanDirectMessage(r.pool.QueryRow(ctx, `
		UPDATE bot_direct_message_drafts SET status = 'draft', source_chat_id = $1,
			source_message_id = $2, source_kind = $3, source_preview = $4, source_html = $5,
			previewed_at = NULL, updated_at = NOW()
		WHERE admin_telegram_id = $1 AND status = 'awaiting_message'
		RETURNING `+directMessageColumns, adminID, messageID, kind, preview, sourceHTML))
}

func (r *BroadcastRepository) MarkDirectPreviewed(ctx context.Context, adminID int64, messageID int) (*DirectMessageDraft, error) {
	return scanDirectMessage(r.pool.QueryRow(ctx, `
		UPDATE bot_direct_message_drafts SET previewed_at = NOW(), updated_at = NOW()
		WHERE admin_telegram_id = $1 AND status IN ('draft', 'sent') AND source_message_id = $2
		RETURNING `+directMessageColumns, adminID, messageID))
}

func (r *BroadcastRepository) BeginDirectSend(ctx context.Context, adminID, customerID int64, expectedUpdatedAt time.Time) (*DirectMessageDraft, error) {
	return scanDirectMessage(r.pool.QueryRow(ctx, `
		UPDATE bot_direct_message_drafts SET status = 'sending', updated_at = NOW()
		WHERE admin_telegram_id = $1 AND customer_id = $2 AND updated_at = $3 AND status IN ('draft', 'sent')
			AND source_message_id IS NOT NULL
		RETURNING `+directMessageColumns, adminID, customerID, expectedUpdatedAt))
}

func (r *BroadcastRepository) FinishDirectSend(ctx context.Context, adminID int64, status string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE bot_direct_message_drafts SET status = $2, sent_at = CASE WHEN $2 = 'sent' THEN NOW() ELSE sent_at END,
			updated_at = NOW() WHERE admin_telegram_id = $1 AND status = 'sending'
	`, adminID, status)
	return err
}

func (r *BroadcastRepository) RecoverDirectSend(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `UPDATE bot_direct_message_drafts SET status = 'interrupted', updated_at = NOW() WHERE status = 'sending'`)
	return err
}

type directMessageRowScanner interface {
	Scan(dest ...interface{}) error
}

func scanDirectMessage(row directMessageRowScanner) (*DirectMessageDraft, error) {
	var draft DirectMessageDraft
	err := row.Scan(&draft.AdminTelegramID, &draft.CustomerID, &draft.Status, &draft.SourceChatID,
		&draft.SourceMessageID, &draft.SourceKind, &draft.SourcePreview, &draft.SourceHTML,
		&draft.PreviewedAt, &draft.SentAt, &draft.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &draft, nil
}
