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
	DeleteAfterMinutes int               `json:"deleteAfterMinutes"`
	Audience           BroadcastAudience `json:"audience"`
	Status             string            `json:"status"`
	SourceChatID       *int64            `json:"-"`
	SourceMessageID    *int              `json:"-"`
	SourceKind         string            `json:"sourceKind"`
	SourcePreview      string            `json:"sourcePreview"`
	SourceText         string            `json:"sourceText,omitempty"`
	SourceHTML         string            `json:"-"`
	Buttons            []BroadcastButton `json:"buttons"`
	RecipientCount     int               `json:"recipientCount"`
	SentCount          int               `json:"sentCount"`
	FailedCount        int               `json:"failedCount"`
	LastError          string            `json:"lastError,omitempty"`
	UpdatedBy          *int64            `json:"-"`
	StartedAt          *time.Time        `json:"startedAt,omitempty"`
	FinishedAt         *time.Time        `json:"finishedAt,omitempty"`
	UpdatedAt          time.Time         `json:"updatedAt"`
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
		SELECT status, source_chat_id, source_message_id, source_kind, source_preview, source_html,
		       buttons, recipient_count, sent_count, failed_count, last_error,
		       updated_by, started_at, finished_at, updated_at, audience, delete_after_minutes
		FROM bot_broadcast_draft
		WHERE id = 1
	`))
}

func (r *BroadcastRepository) StartCapture(ctx context.Context, updatedBy int64) (*BroadcastDraft, error) {
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `
		UPDATE bot_broadcast_draft
		SET status = 'awaiting_message', source_chat_id = NULL, source_message_id = NULL,
		    source_kind = '', source_preview = '', source_html = '', recipient_count = 0, sent_count = 0,
		    failed_count = 0, last_error = '', started_at = NULL, finished_at = NULL,
		    updated_by = $1, updated_at = GREATEST(NOW(), updated_at + INTERVAL '1 microsecond')
		WHERE id = 1 AND status <> 'running'
		RETURNING status, source_chat_id, source_message_id, source_kind, source_preview, source_html,
		          buttons, recipient_count, sent_count, failed_count, last_error,
		          updated_by, started_at, finished_at, updated_at, audience, delete_after_minutes
	`, updatedBy))
}

func (r *BroadcastRepository) SaveSource(ctx context.Context, chatID int64, messageID int, kind, preview, sourceHTML string, updatedBy int64) (*BroadcastDraft, error) {
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `
		UPDATE bot_broadcast_draft
		SET status = 'draft', source_chat_id = $1, source_message_id = $2,
		    source_kind = $3, source_preview = $4, source_html = $5, recipient_count = 0, sent_count = 0,
		    failed_count = 0, last_error = '', started_at = NULL, finished_at = NULL,
		    updated_by = $6, updated_at = GREATEST(NOW(), updated_at + INTERVAL '1 microsecond')
		WHERE id = 1 AND status = 'awaiting_message' AND updated_by = $6
		RETURNING status, source_chat_id, source_message_id, source_kind, source_preview, source_html,
		          buttons, recipient_count, sent_count, failed_count, last_error,
		          updated_by, started_at, finished_at, updated_at, audience, delete_after_minutes
	`, chatID, messageID, kind, preview, sourceHTML, updatedBy))
}

func (r *BroadcastRepository) SaveButtons(ctx context.Context, buttons []BroadcastButton, updatedBy int64) (*BroadcastDraft, error) {
	raw, err := json.Marshal(buttons)
	if err != nil {
		return nil, err
	}
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `
		UPDATE bot_broadcast_draft
		SET buttons = $1, updated_by = $2, updated_at = GREATEST(NOW(), updated_at + INTERVAL '1 microsecond')
		WHERE id = 1 AND status <> 'running'
		RETURNING status, source_chat_id, source_message_id, source_kind, source_preview, source_html,
		          buttons, recipient_count, sent_count, failed_count, last_error,
		          updated_by, started_at, finished_at, updated_at, audience, delete_after_minutes
	`, raw, updatedBy))
}

func (r *BroadcastRepository) BeginSend(ctx context.Context, recipientCount int, updatedBy int64, expected time.Time) (*BroadcastDraft, error) {
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `
		UPDATE bot_broadcast_draft
		SET status = 'running', recipient_count = $1, sent_count = 0, failed_count = 0,
		    last_error = '', started_at = NOW(), finished_at = NULL,
		    updated_by = $2, updated_at = GREATEST(NOW(), updated_at + INTERVAL '1 microsecond')
		WHERE id = 1 AND status <> 'running'
		  AND source_chat_id IS NOT NULL AND source_message_id IS NOT NULL
		  AND updated_at = $3
		RETURNING status, source_chat_id, source_message_id, source_kind, source_preview, source_html,
		          buttons, recipient_count, sent_count, failed_count, last_error,
		          updated_by, started_at, finished_at, updated_at, audience, delete_after_minutes
	`, recipientCount, updatedBy, expected))
}

func (r *BroadcastRepository) UpdateProgress(ctx context.Context, sent, failed int, lastError string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE bot_broadcast_draft
		SET sent_count = $1, failed_count = $2, last_error = $3, updated_at = GREATEST(NOW(), updated_at + INTERVAL '1 microsecond')
		WHERE id = 1 AND status = 'running'
	`, sent, failed, lastError)
	return err
}

func (r *BroadcastRepository) Finish(ctx context.Context, status string, sent, failed int, lastError string, skipped ...int) error {
	countSkipped := 0
	if len(skipped) > 0 {
		countSkipped = skipped[0]
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE bot_broadcast_draft
		SET status = $1, sent_count = $2, failed_count = $3, last_error = $4,
		    recipient_count = GREATEST(0, recipient_count - $5),
		    finished_at = NOW(), updated_at = GREATEST(NOW(), updated_at + INTERVAL '1 microsecond')
		WHERE id = 1 AND status = 'running'
	`, status, sent, failed, lastError, countSkipped)
	return err
}

func (r *BroadcastRepository) Reset(ctx context.Context, updatedBy int64) (*BroadcastDraft, error) {
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `
		UPDATE bot_broadcast_draft
		SET status = 'idle', source_chat_id = NULL, source_message_id = NULL,
		    source_kind = '', source_preview = '', source_html = '', buttons = '[]'::jsonb, audience = '{}'::jsonb, delete_after_minutes=0,
		    recipient_count = 0, sent_count = 0, failed_count = 0, last_error = '',
		    started_at = NULL, finished_at = NULL, updated_by = $1, updated_at = GREATEST(NOW(), updated_at + INTERVAL '1 microsecond')
		WHERE id = 1 AND status <> 'running'
		RETURNING status, source_chat_id, source_message_id, source_kind, source_preview, source_html,
		          buttons, recipient_count, sent_count, failed_count, last_error,
		          updated_by, started_at, finished_at, updated_at, audience, delete_after_minutes
	`, updatedBy))
}

func (r *BroadcastRepository) RecoverInterrupted(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE bot_broadcast_draft
		SET status = 'failed', last_error = 'Рассылка прервана перезапуском бота',
		    finished_at = NOW(), updated_at = GREATEST(NOW(), updated_at + INTERVAL '1 microsecond')
		WHERE id = 1 AND status = 'running'
	`)
	return err
}

type broadcastRowScanner interface {
	Scan(dest ...interface{}) error
}

func scanBroadcastDraft(row broadcastRowScanner) (*BroadcastDraft, error) {
	var draft BroadcastDraft
	var rawButtons, rawAudience []byte
	err := row.Scan(
		&draft.Status, &draft.SourceChatID, &draft.SourceMessageID, &draft.SourceKind,
		&draft.SourcePreview, &draft.SourceHTML, &rawButtons, &draft.RecipientCount, &draft.SentCount,
		&draft.FailedCount, &draft.LastError, &draft.UpdatedBy, &draft.StartedAt,
		&draft.FinishedAt, &draft.UpdatedAt, &rawAudience, &draft.DeleteAfterMinutes,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(rawAudience, &draft.Audience); err != nil {
		return nil, err
	}
	if err := draft.Audience.Normalize(); err != nil {
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
	draft.SourceText = draft.SourcePreview
	if draft.SourceHTML != "" {
		draft.SourceText = draft.SourceHTML
		if draft.SourceKind == "html" {
			draft.SourceText = broadcastPreviewText(draft.SourceHTML)
		}
	}
	return &draft, nil
}
