package database

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v4"
)

var ErrSupportOperatorConflict = errors.New("ticket already assigned to another operator or closed")

// Lock the ticket before the AI state, like FinishAIMessage. Claiming a ticket
// invalidates even an in-flight provider reply in the same transaction.
func (r *SupportRepository) SetOperator(ctx context.Context, ticketID, operatorID int64, name string, release, owner bool) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status string
	var assigned int64
	if err = tx.QueryRow(ctx, `SELECT status, operator_telegram_id FROM support_ticket WHERE id=$1 FOR UPDATE`, ticketID).Scan(&status, &assigned); err != nil {
		return err
	}
	if !supportOperatorAllowed(status, assigned, operatorID, release, owner) {
		return ErrSupportOperatorConflict
	}
	if release {
		operatorID, name = 0, ""
	}
	if _, err = tx.Exec(ctx, `UPDATE support_ticket SET operator_telegram_id=$2, operator_name=$3, updated_at=NOW() WHERE id=$1`, ticketID, operatorID, name); err != nil {
		return err
	}
	// Releasing returns the ticket to the human queue; it must not restart AI.
	if _, err = tx.Exec(ctx, `INSERT INTO support_ai_state(ticket_id,pending,handed_off) VALUES($1,FALSE,TRUE)
	ON CONFLICT(ticket_id) DO UPDATE SET pending=FALSE, handed_off=TRUE, lease_until=NULL, notification_pending=FALSE`, ticketID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func supportOperatorAllowed(status string, assigned, actor int64, release, owner bool) bool {
	if status != "open" || actor <= 0 {
		return false
	}
	if release {
		return assigned != 0 && (assigned == actor || owner)
	}
	return assigned == 0 || assigned == actor
}

func (r *SupportRepository) AIHandedOff(ctx context.Context, ticketID int64) (bool, error) {
	var handedOff bool
	err := r.pool.QueryRow(ctx, `SELECT handed_off FROM support_ai_state WHERE ticket_id=$1`, ticketID).Scan(&handedOff)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return handedOff, err
}
