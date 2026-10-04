package database

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v4"
)

const SupportAuthorRoleAI SupportAuthorRole = "ai"

type SupportAIClaim struct{ TicketID, MessageID int64 }

// A persistent lease prevents duplicate replies across processes and permits
// recovery after a restart. Human replies always take ownership of the ticket.
func (r *SupportRepository) ClaimAIMessage(ctx context.Context) (*SupportAIClaim, error) {
	var claim SupportAIClaim
	err := r.pool.QueryRow(ctx, `WITH candidate AS (
	 SELECT s.ticket_id, latest.id FROM support_ai_state s
	 JOIN support_ticket t ON t.id = s.ticket_id
	 JOIN LATERAL (SELECT id, author_role FROM support_message WHERE ticket_id = s.ticket_id ORDER BY id DESC LIMIT 1) latest ON TRUE
	 WHERE t.status = 'open' AND s.pending AND NOT s.handed_off
	 AND (s.lease_until IS NULL OR s.lease_until < NOW())
	 AND latest.author_role = 'customer' AND latest.id > s.processed_message_id
	 AND NOT EXISTS (SELECT 1 FROM support_message WHERE ticket_id = s.ticket_id AND author_role = 'admin')
	 ORDER BY latest.id LIMIT 1 FOR UPDATE OF s SKIP LOCKED
	) UPDATE support_ai_state s SET claimed_message_id = c.id, lease_until = NOW() + INTERVAL '120 seconds'
	FROM candidate c WHERE s.ticket_id = c.ticket_id RETURNING s.ticket_id, s.claimed_message_id`).Scan(&claim.TicketID, &claim.MessageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &claim, err
}

// FinishAIMessage atomically checks whether the conversation has changed while
// the provider was responding. Stale replies are discarded without hiding newer
// customer messages; the next pass picks them up with their full history.
func (r *SupportRepository) FinishAIMessage(ctx context.Context, claim SupportAIClaim, body string, handoff bool) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM support_ticket WHERE id = $1 FOR UPDATE`, claim.TicketID).Scan(&status); err != nil {
		return false, err
	}
	var claimed int64
	var handedOff, leaseValid bool
	if err = tx.QueryRow(ctx, `SELECT claimed_message_id, handed_off, COALESCE(lease_until > NOW(), FALSE) FROM support_ai_state WHERE ticket_id = $1 FOR UPDATE`, claim.TicketID).Scan(&claimed, &handedOff, &leaseValid); err != nil {
		return false, err
	}
	if claimed != claim.MessageID || !leaseValid {
		return false, nil
	}
	var latest int64
	if err = tx.QueryRow(ctx, `SELECT id FROM support_message WHERE ticket_id = $1 ORDER BY id DESC LIMIT 1`, claim.TicketID).Scan(&latest); err != nil {
		return false, err
	}
	if status != "open" || handedOff || latest != claim.MessageID {
		_, err = tx.Exec(ctx, `UPDATE support_ai_state SET lease_until = NULL WHERE ticket_id = $1`, claim.TicketID)
		if err != nil {
			return false, err
		}
		return false, tx.Commit(ctx)
	}
	if strings.TrimSpace(body) == "" {
		return false, errors.New("empty AI reply")
	}
	_, err = tx.Exec(ctx, `INSERT INTO support_message (ticket_id, author_role, author_telegram_id, body) VALUES ($1, 'ai', 0, $2)`, claim.TicketID, body)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE support_ticket SET updated_at = NOW(), last_message_at = NOW(), last_message_preview = $2, customer_unread_count = customer_unread_count + 1 WHERE id = $1`, claim.TicketID, supportPreview(body))
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE support_ai_state SET processed_message_id = $2, lease_until = NULL, pending = FALSE, handed_off = $3, notification_pending = $3 WHERE ticket_id = $1`, claim.TicketID, claim.MessageID, handoff)
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (r *SupportRepository) ClaimAINotification(ctx context.Context) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `WITH candidate AS (
	 SELECT ticket_id FROM support_ai_state WHERE notification_pending
	 AND (notification_lease_until IS NULL OR notification_lease_until < NOW())
	 ORDER BY ticket_id LIMIT 1 FOR UPDATE SKIP LOCKED
	) UPDATE support_ai_state s SET notification_lease_until = NOW() + INTERVAL '60 seconds'
	FROM candidate c WHERE s.ticket_id = c.ticket_id RETURNING s.ticket_id`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (r *SupportRepository) CompleteAINotification(ctx context.Context, ticketID int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE support_ai_state SET notification_pending = FALSE WHERE ticket_id = $1`, ticketID)
	return err
}

// Operator requests are committed immediately, even while a model request is
// in flight. The lease completion check will discard that model's late answer.
func (r *SupportRepository) HandoffAI(ctx context.Context, ticketID int64, body string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM support_ticket WHERE id = $1 FOR UPDATE`, ticketID).Scan(&status); err != nil {
		return false, err
	}
	if status != "open" {
		return false, nil
	}
	result, err := tx.Exec(ctx, `UPDATE support_ai_state s SET handed_off = TRUE, pending = FALSE, notification_pending = TRUE, lease_until = NULL
	WHERE ticket_id = $1 AND NOT handed_off
	AND NOT EXISTS (SELECT 1 FROM support_message WHERE ticket_id = s.ticket_id AND author_role = 'admin')`, ticketID)
	if err != nil || result.RowsAffected() == 0 {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO support_message (ticket_id, author_role, author_telegram_id, body) VALUES ($1, 'ai', 0, $2)`, ticketID, body)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE support_ticket SET updated_at = NOW(), last_message_at = NOW(), last_message_preview = $2, customer_unread_count = customer_unread_count + 1 WHERE id = $1`, ticketID, supportPreview(body))
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
