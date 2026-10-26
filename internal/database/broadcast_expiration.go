package database

import (
	"context"
	"errors"
	"time"
)

const MaxBroadcastDeleteAfterMinutes = 47 * 60

var ErrBroadcastExpiration = errors.New("Укажите срок от 1 минуты до 47 часов или отключите удаление")

func ValidateBroadcastExpiration(minutes int) error {
	if minutes < 0 || minutes > MaxBroadcastDeleteAfterMinutes {
		return ErrBroadcastExpiration
	}
	return nil
}

func (r *BroadcastRepository) SaveExpiration(ctx context.Context, minutes int, actor int64, expected time.Time) (*BroadcastDraft, error) {
	if err := ValidateBroadcastExpiration(minutes); err != nil {
		return nil, err
	}
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `UPDATE bot_broadcast_draft
		SET delete_after_minutes=$1, updated_by=$2, updated_at=GREATEST(NOW(),updated_at+INTERVAL '1 microsecond')
		WHERE id=1 AND status<>'running' AND updated_at=$3
		RETURNING status,source_chat_id,source_message_id,source_kind,source_preview,source_html,
		buttons,recipient_count,sent_count,failed_count,last_error,updated_by,started_at,finished_at,updated_at,audience,delete_after_minutes`, minutes, actor, expected))
}

type BroadcastDeletion struct {
	ID        int64
	ChatID    int64
	MessageID int
	SentAt    time.Time
	DeleteAt  time.Time
	Attempts  int
}

// These records belong to sent messages, independently of the editable draft.
func (r *BroadcastRepository) ScheduleDeletion(ctx context.Context, chatID int64, messageID int, sentAt time.Time, minutes int) error {
	if err := ValidateBroadcastExpiration(minutes); err != nil {
		return err
	}
	if minutes == 0 || messageID <= 0 || chatID <= 0 {
		return errors.New("invalid temporary broadcast delivery")
	}
	due := sentAt.Add(time.Duration(minutes) * time.Minute)
	_, err := r.pool.Exec(ctx, `INSERT INTO broadcast_message_deletion(chat_id,message_id,sent_at,delete_at,next_attempt_at)
		VALUES($1,$2,$3,$4,$4) ON CONFLICT(chat_id,message_id) DO NOTHING`, chatID, messageID, sentAt, due)
	return err
}

func (r *BroadcastRepository) ClaimDeletions(ctx context.Context, limit int) ([]BroadcastDeletion, error) {
	rows, err := r.pool.Query(ctx, `WITH due AS (
		SELECT id FROM broadcast_message_deletion WHERE status='pending' AND next_attempt_at<=NOW()
		AND delete_at<=NOW() AND (lease_until IS NULL OR lease_until<=NOW())
		ORDER BY next_attempt_at,id LIMIT $1 FOR UPDATE SKIP LOCKED)
		UPDATE broadcast_message_deletion d SET lease_until=NOW()+INTERVAL '5 minutes',attempts=attempts+1
		FROM due WHERE d.id=due.id RETURNING d.id,d.chat_id,d.message_id,d.sent_at,d.delete_at,d.attempts`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []BroadcastDeletion
	for rows.Next() {
		var item BroadcastDeletion
		if err := rows.Scan(&item.ID, &item.ChatID, &item.MessageID, &item.SentAt, &item.DeleteAt, &item.Attempts); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *BroadcastRepository) FinishDeletion(ctx context.Context, item BroadcastDeletion, status, lastError string, next time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE broadcast_message_deletion SET status=$3,last_error=$4,next_attempt_at=$5,lease_until=NULL,
		finished_at=CASE WHEN $3='pending' THEN NULL ELSE NOW() END WHERE id=$1 AND attempts=$2 AND status='pending'`, item.ID, item.Attempts, status, lastError, next)
	return err
}
