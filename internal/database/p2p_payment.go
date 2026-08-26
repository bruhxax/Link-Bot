package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4"
)

type P2PPaymentStatus string

const (
	P2PPaymentStatusPending    P2PPaymentStatus = "pending"
	P2PPaymentStatusProcessing P2PPaymentStatus = "processing"
	P2PPaymentStatusApproved   P2PPaymentStatus = "approved"
	P2PPaymentStatusRejected   P2PPaymentStatus = "rejected"
)

type P2PDestinationSnapshot struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Details     string `json:"details"`
	Description string `json:"description,omitempty"`
}

type P2PPaymentRequest struct {
	PurchaseID            int64                  `db:"purchase_id"`
	DestinationSnapshot   P2PDestinationSnapshot `db:"destination_snapshot"`
	SenderReference       string                 `db:"sender_reference"`
	Status                P2PPaymentStatus       `db:"status"`
	NotificationChatID    *int64                 `db:"notification_chat_id"`
	NotificationMessageID *int64                 `db:"notification_message_id"`
	SubmittedAt           time.Time              `db:"submitted_at"`
	ReviewedAt            *time.Time             `db:"reviewed_at"`
	ReviewedBy            *int64                 `db:"reviewed_by"`
}

func (pr *PurchaseRepository) CreateP2PPaymentRequest(ctx context.Context, request *P2PPaymentRequest) error {
	if request == nil || request.PurchaseID <= 0 {
		return errors.New("invalid P2P payment request")
	}
	snapshot, err := json.Marshal(request.DestinationSnapshot)
	if err != nil {
		return fmt.Errorf("marshal P2P destination snapshot: %w", err)
	}
	_, err = pr.pool.Exec(ctx, `
		INSERT INTO p2p_payment_request (purchase_id, destination_snapshot, sender_reference, status)
		VALUES ($1, $2, $3, $4)
	`, request.PurchaseID, snapshot, request.SenderReference, P2PPaymentStatusPending)
	if err != nil {
		return fmt.Errorf("create P2P payment request: %w", err)
	}
	return nil
}

func (pr *PurchaseRepository) FindP2PPaymentRequest(ctx context.Context, purchaseID int64) (*P2PPaymentRequest, error) {
	request := &P2PPaymentRequest{}
	var snapshot []byte
	err := pr.pool.QueryRow(ctx, `
		SELECT purchase_id, destination_snapshot, sender_reference, status,
		       notification_chat_id, notification_message_id, submitted_at, reviewed_at, reviewed_by
		FROM p2p_payment_request
		WHERE purchase_id = $1
	`, purchaseID).Scan(
		&request.PurchaseID,
		&snapshot,
		&request.SenderReference,
		&request.Status,
		&request.NotificationChatID,
		&request.NotificationMessageID,
		&request.SubmittedAt,
		&request.ReviewedAt,
		&request.ReviewedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find P2P payment request: %w", err)
	}
	if err := json.Unmarshal(snapshot, &request.DestinationSnapshot); err != nil {
		return nil, fmt.Errorf("decode P2P destination snapshot: %w", err)
	}
	return request, nil
}

func (pr *PurchaseRepository) SetP2PNotificationMessage(ctx context.Context, purchaseID, chatID, messageID int64) error {
	command, err := pr.pool.Exec(ctx, `
		UPDATE p2p_payment_request
		SET notification_chat_id = $2, notification_message_id = $3
		WHERE purchase_id = $1
	`, purchaseID, chatID, messageID)
	if err != nil {
		return fmt.Errorf("save P2P notification message: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("P2P payment request %d not found", purchaseID)
	}
	return nil
}

// ClaimP2PApproval atomically reserves an undecided request. A processing
// request may be reclaimed after five minutes so a crash between fulfillment
// and finalization cannot leave the payment permanently stuck.
func (pr *PurchaseRepository) ClaimP2PApproval(ctx context.Context, purchaseID, adminID int64) (bool, error) {
	command, err := pr.pool.Exec(ctx, `
		UPDATE p2p_payment_request
		SET status = $2, reviewed_at = NOW(), reviewed_by = $3
		WHERE purchase_id = $1
		  AND (status = $4 OR (status = $2 AND reviewed_at < NOW() - INTERVAL '5 minutes'))
	`, purchaseID, P2PPaymentStatusProcessing, adminID, P2PPaymentStatusPending)
	if err != nil {
		return false, fmt.Errorf("claim P2P approval: %w", err)
	}
	return command.RowsAffected() == 1, nil
}

func (pr *PurchaseRepository) ReleaseP2PApproval(ctx context.Context, purchaseID int64) error {
	_, err := pr.pool.Exec(ctx, `
		UPDATE p2p_payment_request
		SET status = $2, reviewed_at = NULL, reviewed_by = NULL
		WHERE purchase_id = $1 AND status = $3
	`, purchaseID, P2PPaymentStatusPending, P2PPaymentStatusProcessing)
	if err != nil {
		return fmt.Errorf("release P2P approval: %w", err)
	}
	return nil
}

func (pr *PurchaseRepository) MarkP2PApproved(ctx context.Context, purchaseID, adminID int64) error {
	command, err := pr.pool.Exec(ctx, `
		UPDATE p2p_payment_request
		SET status = $2, reviewed_at = NOW(), reviewed_by = $3
		WHERE purchase_id = $1 AND status = $4
	`, purchaseID, P2PPaymentStatusApproved, adminID, P2PPaymentStatusProcessing)
	if err != nil {
		return fmt.Errorf("finish P2P approval: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("P2P approval %d is no longer processing", purchaseID)
	}
	return nil
}

func (pr *PurchaseRepository) RejectP2PPayment(ctx context.Context, purchaseID, adminID int64) (bool, error) {
	tx, err := pr.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin P2P rejection: %w", err)
	}
	defer tx.Rollback(ctx)

	command, err := tx.Exec(ctx, `
		UPDATE p2p_payment_request
		SET status = $2, reviewed_at = NOW(), reviewed_by = $3
		WHERE purchase_id = $1 AND status = $4
	`, purchaseID, P2PPaymentStatusRejected, adminID, P2PPaymentStatusPending)
	if err != nil {
		return false, fmt.Errorf("reject P2P request: %w", err)
	}
	if command.RowsAffected() != 1 {
		return false, nil
	}
	purchaseCommand, err := tx.Exec(ctx, `
		UPDATE purchase
		SET status = $2
		WHERE id = $1 AND invoice_type = $3 AND status IN ($4, $5)
	`, purchaseID, PurchaseStatusCancel, InvoiceTypeP2P, PurchaseStatusNew, PurchaseStatusPending)
	if err != nil {
		return false, fmt.Errorf("cancel rejected P2P purchase: %w", err)
	}
	if purchaseCommand.RowsAffected() != 1 {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit P2P rejection: %w", err)
	}
	return true, nil
}
