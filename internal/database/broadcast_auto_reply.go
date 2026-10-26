package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
)

type BroadcastAutoReplySettings struct {
	Enabled           bool      `json:"enabled"`
	Text              string    `json:"text"`
	WindowHours       int       `json:"windowHours"`
	SupportButton     bool      `json:"supportButton"`
	SupportButtonText string    `json:"supportButtonText"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func (r *BroadcastRepository) AutoReplySettings(ctx context.Context) (*BroadcastAutoReplySettings, error) {
	return scanAutoReplySettings(r.pool.QueryRow(ctx, `SELECT enabled,text,window_hours,support_button,support_button_text,updated_at FROM bot_broadcast_auto_reply WHERE id=1`))
}

func scanAutoReplySettings(row broadcastRowScanner) (*BroadcastAutoReplySettings, error) {
	var value BroadcastAutoReplySettings
	err := row.Scan(&value.Enabled, &value.Text, &value.WindowHours, &value.SupportButton, &value.SupportButtonText, &value.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &value, err
}

func (r *BroadcastRepository) SaveAutoReplySettings(ctx context.Context, value BroadcastAutoReplySettings) (*BroadcastAutoReplySettings, error) {
	return scanAutoReplySettings(r.pool.QueryRow(ctx, `UPDATE bot_broadcast_auto_reply SET enabled=$1,text=$2,window_hours=$3,support_button=$4,
		support_button_text=$5,updated_at=GREATEST(NOW(),updated_at+INTERVAL '1 microsecond') WHERE id=1 AND updated_at=$6
		RETURNING enabled,text,window_hours,support_button,support_button_text,updated_at`, value.Enabled, value.Text, value.WindowHours, value.SupportButton, value.SupportButtonText, value.UpdatedAt))
}

func (r *BroadcastRepository) RecordBroadcastRecipient(ctx context.Context, chatID int64, messageID int, sentAt time.Time) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO broadcast_recipient_reply(chat_id,broadcast_message_id,sent_at) VALUES($1,$2,$3)
		ON CONFLICT(chat_id) DO UPDATE SET broadcast_message_id=$2,sent_at=$3,replied_at=NULL,lease_until=NULL,incoming_message_id=NULL
		WHERE broadcast_recipient_reply.broadcast_message_id<$2`, chatID, messageID, sentAt)
	return err
}

func (r *BroadcastRepository) ClaimBroadcastReply(ctx context.Context, chatID int64, incomingID, replyToID int, incomingAt time.Time, hours int) (int, error) {
	var messageID int
	err := r.pool.QueryRow(ctx, `UPDATE broadcast_recipient_reply SET lease_until=NOW()+INTERVAL '1 minute',incoming_message_id=$2
		WHERE chat_id=$1 AND $2>broadcast_message_id AND replied_at IS NULL AND (lease_until IS NULL OR lease_until<=NOW())
		AND sent_at<=$4 AND sent_at>=NOW()-($5::int*INTERVAL '1 hour') AND $4<=NOW()+INTERVAL '5 minutes'
		AND ($3=0 OR broadcast_message_id=$3)
		RETURNING broadcast_message_id`, chatID, incomingID, replyToID, incomingAt, hours).Scan(&messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return messageID, err
}

func (r *BroadcastRepository) FinishBroadcastReply(ctx context.Context, chatID int64, broadcastID, incomingID int, success bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE broadcast_recipient_reply SET replied_at=CASE WHEN $4 THEN NOW() ELSE NULL END,lease_until=NULL
		WHERE chat_id=$1 AND broadcast_message_id=$2 AND incoming_message_id=$3`, chatID, broadcastID, incomingID, success)
	return err
}
