package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

var ErrPinnedStateChanged = errors.New("pinned message state changed")

type PinnedMessage struct {
	Enabled          bool       `json:"enabled"`
	Revision         int64      `json:"revision"`
	SourceVersion    int64      `json:"-"`
	SourceChatID     *int64     `json:"-"`
	SourceMessageID  *int       `json:"-"`
	SourcePreview    string     `json:"sourcePreview"`
	SourceKind       string     `json:"sourceKind"`
	CaptureAdminID   *int64     `json:"-"`
	CaptureStartedAt *time.Time `json:"-"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	HasMessage       bool       `json:"hasMessage"`
	AwaitingMessage  bool       `json:"awaitingMessage"`
	Total            int        `json:"total"`
	Completed        int        `json:"completed"`
	Failed           int        `json:"failed"`
	Pending          int        `json:"pending"`
}

type PinnedDelivery struct {
	ChatID        int64
	MessageID     *int
	SourceVersion int64
}

type PinnedMessageRepository struct{ pool *pgxpool.Pool }

func NewPinnedMessageRepository(pool *pgxpool.Pool) *PinnedMessageRepository {
	return &PinnedMessageRepository{pool: pool}
}

func (r *PinnedMessageRepository) Get(ctx context.Context) (*PinnedMessage, error) {
	var s PinnedMessage
	err := r.pool.QueryRow(ctx, `SELECT enabled,revision,source_version,source_chat_id,source_message_id,source_preview,source_kind,capture_admin_id,capture_started_at,updated_at FROM bot_pinned_message WHERE id=1`).Scan(
		&s.Enabled, &s.Revision, &s.SourceVersion, &s.SourceChatID, &s.SourceMessageID, &s.SourcePreview, &s.SourceKind, &s.CaptureAdminID, &s.CaptureStartedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.HasMessage = s.SourceChatID != nil && s.SourceMessageID != nil
	s.AwaitingMessage = s.CaptureAdminID != nil && s.CaptureStartedAt != nil && time.Since(*s.CaptureStartedAt) < 30*time.Minute
	return &s, nil
}

func (r *PinnedMessageRepository) State(ctx context.Context) (*PinnedMessage, error) {
	s, err := r.Get(ctx)
	if err != nil {
		return nil, err
	}
	// New and existing Telegram customers are included, regardless of newsletter preferences.
	query := `SELECT COUNT(*), COUNT(*) FILTER(WHERE d.applied_revision=$1), COUNT(*) FILTER(WHERE d.attempt_revision=$1 AND d.applied_revision<>$1 AND d.last_error<>'') FROM customer c LEFT JOIN bot_pinned_delivery d ON d.chat_id=c.telegram_id WHERE c.telegram_id>0 AND NOT c.is_blocked AND NOT c.telegram_id_is_synthetic`
	if !s.Enabled {
		query = `SELECT COUNT(*),COUNT(*) FILTER(WHERE applied_revision=$1),COUNT(*) FILTER(WHERE attempt_revision=$1 AND applied_revision<>$1 AND last_error<>'') FROM bot_pinned_delivery WHERE message_id IS NOT NULL`
	}
	err = r.pool.QueryRow(ctx, query, s.Revision).Scan(&s.Total, &s.Completed, &s.Failed)
	s.Pending = s.Total - s.Completed - s.Failed
	return s, err
}

func (r *PinnedMessageRepository) StartCapture(ctx context.Context, adminID int64) error {
	result, err := r.pool.Exec(ctx, `UPDATE bot_pinned_message SET capture_admin_id=$1,capture_started_at=NOW() WHERE id=1 AND (capture_admin_id IS NULL OR capture_admin_id=$1 OR capture_started_at<NOW()-INTERVAL '30 minutes')`, adminID)
	if err == nil && result.RowsAffected() == 0 {
		return ErrPinnedStateChanged
	}
	return err
}

func (r *PinnedMessageRepository) CancelCapture(ctx context.Context, adminID int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE bot_pinned_message SET capture_admin_id=NULL,capture_started_at=NULL WHERE id=1 AND capture_admin_id=$1`, adminID)
	return err
}

func (r *PinnedMessageRepository) SaveSource(ctx context.Context, adminID int64, messageID int, kind, preview string) error {
	result, err := r.pool.Exec(ctx, `UPDATE bot_pinned_message SET source_chat_id=$1,source_message_id=$2,source_kind=$3,source_preview=$4,source_version=source_version+1,revision=revision+1,capture_admin_id=NULL,capture_started_at=NULL,updated_at=NOW() WHERE id=1 AND capture_admin_id=$1 AND capture_started_at>NOW()-INTERVAL '30 minutes'`, adminID, messageID, kind, preview)
	if err == nil && result.RowsAffected() == 0 {
		return ErrPinnedStateChanged
	}
	return err
}

func (r *PinnedMessageRepository) SetEnabled(ctx context.Context, enabled bool, revision int64) error {
	result, err := r.pool.Exec(ctx, `UPDATE bot_pinned_message SET enabled=$1,revision=revision+CASE WHEN enabled<>$1 THEN 1 ELSE 0 END,updated_at=NOW() WHERE id=1 AND revision=$2 AND (NOT $1 OR source_message_id IS NOT NULL)`, enabled, revision)
	if err == nil && result.RowsAffected() == 0 {
		return ErrPinnedStateChanged
	}
	return err
}

func (r *PinnedMessageRepository) Pending(ctx context.Context, s *PinnedMessage, after int64) ([]PinnedDelivery, error) {
	query := `SELECT c.telegram_id,d.message_id,COALESCE(d.source_version,0) FROM customer c LEFT JOIN bot_pinned_delivery d ON d.chat_id=c.telegram_id WHERE c.telegram_id>$2 AND NOT c.is_blocked AND NOT c.telegram_id_is_synthetic AND COALESCE(d.applied_revision,0)<>$1 AND (COALESCE(d.attempt_revision,0)<>$1 OR (d.attempts<3 AND d.retry_at<=NOW())) ORDER BY c.telegram_id LIMIT 50`
	if !s.Enabled {
		query = `SELECT chat_id,message_id,source_version FROM bot_pinned_delivery WHERE chat_id>$2 AND message_id IS NOT NULL AND applied_revision<>$1 AND (attempt_revision<>$1 OR (attempts<3 AND retry_at<=NOW())) ORDER BY chat_id LIMIT 50`
	}
	rows, err := r.pool.Query(ctx, query, s.Revision, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deliveries []PinnedDelivery
	for rows.Next() {
		var d PinnedDelivery
		if err := rows.Scan(&d.ChatID, &d.MessageID, &d.SourceVersion); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// Checkpoint the copy before pinning so a Telegram error never causes another copy.
func (r *PinnedMessageRepository) SaveCopy(ctx context.Context, d PinnedDelivery) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO bot_pinned_delivery(chat_id,message_id,source_version) VALUES($1,$2,$3) ON CONFLICT(chat_id) DO UPDATE SET message_id=$2,source_version=$3`, d.ChatID, d.MessageID, d.SourceVersion)
	return err
}

func (r *PinnedMessageRepository) MarkApplied(ctx context.Context, chatID, revision int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE bot_pinned_delivery SET applied_revision=$2,attempt_revision=$2,attempts=0,last_error='' WHERE chat_id=$1`, chatID, revision)
	return err
}

func (r *PinnedMessageRepository) MarkFailed(ctx context.Context, chatID, revision int64, details string, retryAt time.Time) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO bot_pinned_delivery(chat_id,attempt_revision,attempts,last_error,retry_at) VALUES($1,$2,1,$3,$4) ON CONFLICT(chat_id) DO UPDATE SET attempts=CASE WHEN bot_pinned_delivery.attempt_revision=$2 THEN bot_pinned_delivery.attempts+1 ELSE 1 END,attempt_revision=$2,last_error=$3,retry_at=$4`, chatID, revision, details, retryAt)
	return err
}

// An explicit retry only resets failed attempts, without sending successful copies again.
func (r *PinnedMessageRepository) Retry(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `UPDATE bot_pinned_delivery SET attempts=0,retry_at=NOW(),last_error='' WHERE last_error<>''`)
	return err
}

func (r *PinnedMessageRepository) RetryChats(ctx context.Context, chatIDs []int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE bot_pinned_delivery SET attempts=0,retry_at=NOW(),last_error='' WHERE chat_id=ANY($1) AND last_error<>''`, chatIDs)
	return err
}

// A single worker owns the delivery table even during overlapping application restarts.
func (r *PinnedMessageRepository) WorkerLock(ctx context.Context) (func(), error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(742173064)`).Scan(&locked); err != nil || !locked {
		conn.Release()
		if err == nil {
			err = ErrPinnedStateChanged
		}
		return nil, err
	}
	return func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := conn.Exec(cleanup, `SELECT pg_advisory_unlock(742173064)`)
		if err != nil {
			_ = conn.Conn().Close(cleanup)
		}
		conn.Release()
	}, nil
}
