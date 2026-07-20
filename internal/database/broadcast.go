package database

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

const (
	BroadcastStatusIdle            = "idle"
	BroadcastStatusAwaitingMessage = "awaiting_message"
	BroadcastStatusDraft           = "draft"
	BroadcastStatusRunning         = "running"
	BroadcastStatusFinished        = "finished"
	BroadcastStatusFailed          = "failed"
)

type BroadcastButton struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	Text              string `json:"text"`
	IconCustomEmojiID string `json:"iconCustomEmojiId,omitempty"`
	Style             string `json:"style,omitempty"`
	URL               string `json:"url,omitempty"`
	PromoCode         string `json:"promoCode,omitempty"`
}

type BroadcastDraft struct {
	Status          string            `json:"status"`
	SourceChatID    *int64            `json:"-"`
	SourceMessageID *int              `json:"-"`
	SourceKind      string            `json:"sourceKind"`
	SourcePreview   string            `json:"sourcePreview"`
	Buttons         []BroadcastButton `json:"buttons"`
	RecipientCount  int               `json:"recipientCount"`
	SentCount       int               `json:"sentCount"`
	FailedCount     int               `json:"failedCount"`
	LastError       string            `json:"lastError,omitempty"`
	UpdatedBy       *int64            `json:"-"`
	StartedAt       *time.Time        `json:"startedAt,omitempty"`
	FinishedAt      *time.Time        `json:"finishedAt,omitempty"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}

func (d BroadcastDraft) HasSource() bool {
	return d.SourceChatID != nil && d.SourceMessageID != nil && *d.SourceMessageID > 0
}

type BroadcastRepository struct {
	pool *pgxpool.Pool
}

func NewBroadcastRepository(pool *pgxpool.Pool) *BroadcastRepository {
	return &BroadcastRepository{pool: pool}
}

func (r *BroadcastRepository) Get(ctx context.Context) (*BroadcastDraft, error) {
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `
		SELECT status, source_chat_id, source_message_id, source_kind, source_preview,
		       buttons, recipient_count, sent_count, failed_count, last_error,
		       updated_by, started_at, finished_at, updated_at
		FROM bot_broadcast_draft
		WHERE id = 1
	`))
}

func (r *BroadcastRepository) StartCapture(ctx context.Context, updatedBy int64) (*BroadcastDraft, error) {
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `
		UPDATE bot_broadcast_draft
		SET status = 'awaiting_message', source_chat_id = NULL, source_message_id = NULL,
		    source_kind = '', source_preview = '', recipient_count = 0, sent_count = 0,
		    failed_count = 0, last_error = '', started_at = NULL, finished_at = NULL,
		    updated_by = $1, updated_at = NOW()
		WHERE id = 1 AND status <> 'running'
		RETURNING status, source_chat_id, source_message_id, source_kind, source_preview,
		          buttons, recipient_count, sent_count, failed_count, last_error,
		          updated_by, started_at, finished_at, updated_at
	`, updatedBy))
}

func (r *BroadcastRepository) SaveSource(ctx context.Context, chatID int64, messageID int, kind, preview string, updatedBy int64) (*BroadcastDraft, error) {
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `
		UPDATE bot_broadcast_draft
		SET status = 'draft', source_chat_id = $1, source_message_id = $2,
		    source_kind = $3, source_preview = $4, recipient_count = 0, sent_count = 0,
		    failed_count = 0, last_error = '', started_at = NULL, finished_at = NULL,
		    updated_by = $5, updated_at = NOW()
		WHERE id = 1 AND status = 'awaiting_message'
		RETURNING status, source_chat_id, source_message_id, source_kind, source_preview,
		          buttons, recipient_count, sent_count, failed_count, last_error,
		          updated_by, started_at, finished_at, updated_at
	`, chatID, messageID, kind, preview, updatedBy))
}

func (r *BroadcastRepository) SaveButtons(ctx context.Context, buttons []BroadcastButton, updatedBy int64) (*BroadcastDraft, error) {
	raw, err := json.Marshal(buttons)
	if err != nil {
		return nil, err
	}
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `
		UPDATE bot_broadcast_draft
		SET buttons = $1, updated_by = $2, updated_at = NOW()
		WHERE id = 1 AND status <> 'running'
		RETURNING status, source_chat_id, source_message_id, source_kind, source_preview,
		          buttons, recipient_count, sent_count, failed_count, last_error,
		          updated_by, started_at, finished_at, updated_at
	`, raw, updatedBy))
}

func (r *BroadcastRepository) BeginSend(ctx context.Context, recipientCount int, updatedBy int64) (*BroadcastDraft, error) {
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `
		UPDATE bot_broadcast_draft
		SET status = 'running', recipient_count = $1, sent_count = 0, failed_count = 0,
		    last_error = '', started_at = NOW(), finished_at = NULL,
		    updated_by = $2, updated_at = NOW()
		WHERE id = 1 AND status <> 'running'
		  AND source_chat_id IS NOT NULL AND source_message_id IS NOT NULL
		RETURNING status, source_chat_id, source_message_id, source_kind, source_preview,
		          buttons, recipient_count, sent_count, failed_count, last_error,
		          updated_by, started_at, finished_at, updated_at
	`, recipientCount, updatedBy))
}

func (r *BroadcastRepository) UpdateProgress(ctx context.Context, sent, failed int, lastError string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE bot_broadcast_draft
		SET sent_count = $1, failed_count = $2, last_error = $3, updated_at = NOW()
		WHERE id = 1 AND status = 'running'
	`, sent, failed, lastError)
	return err
}

func (r *BroadcastRepository) Finish(ctx context.Context, status string, sent, failed int, lastError string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE bot_broadcast_draft
		SET status = $1, sent_count = $2, failed_count = $3, last_error = $4,
		    finished_at = NOW(), updated_at = NOW()
		WHERE id = 1
	`, status, sent, failed, lastError)
	return err
}

func (r *BroadcastRepository) Reset(ctx context.Context, updatedBy int64) (*BroadcastDraft, error) {
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `
		UPDATE bot_broadcast_draft
		SET status = 'idle', source_chat_id = NULL, source_message_id = NULL,
		    source_kind = '', source_preview = '', buttons = '[]'::jsonb,
		    recipient_count = 0, sent_count = 0, failed_count = 0, last_error = '',
		    started_at = NULL, finished_at = NULL, updated_by = $1, updated_at = NOW()
		WHERE id = 1 AND status <> 'running'
		RETURNING status, source_chat_id, source_message_id, source_kind, source_preview,
		          buttons, recipient_count, sent_count, failed_count, last_error,
		          updated_by, started_at, finished_at, updated_at
	`, updatedBy))
}

func (r *BroadcastRepository) RecoverInterrupted(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE bot_broadcast_draft
		SET status = 'failed', last_error = 'Рассылка прервана перезапуском бота',
		    finished_at = NOW(), updated_at = NOW()
		WHERE id = 1 AND status = 'running'
	`)
	return err
}

type broadcastRowScanner interface {
	Scan(dest ...interface{}) error
}

func scanBroadcastDraft(row broadcastRowScanner) (*BroadcastDraft, error) {
	var draft BroadcastDraft
	var rawButtons []byte
	err := row.Scan(
		&draft.Status, &draft.SourceChatID, &draft.SourceMessageID, &draft.SourceKind,
		&draft.SourcePreview, &rawButtons, &draft.RecipientCount, &draft.SentCount,
		&draft.FailedCount, &draft.LastError, &draft.UpdatedBy, &draft.StartedAt,
		&draft.FinishedAt, &draft.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if len(rawButtons) > 0 {
		if err := json.Unmarshal(rawButtons, &draft.Buttons); err != nil {
			return nil, err
		}
	}
	if draft.Buttons == nil {
		draft.Buttons = []BroadcastButton{}
	}
	return &draft, nil
}
