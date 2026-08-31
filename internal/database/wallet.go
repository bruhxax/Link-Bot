package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	ErrInsufficientBalance       = errors.New("insufficient balance")
	ErrWithdrawalAlreadyResolved = errors.New("withdrawal is already resolved")
)

type BalanceTransaction struct {
	ID                int64     `json:"id"`
	CustomerID        int64     `json:"customerId"`
	AmountCents       int64     `json:"amountCents"`
	BalanceAfterCents int64     `json:"balanceAfterCents"`
	Kind              string    `json:"kind"`
	Description       string    `json:"description"`
	CreatedAt         time.Time `json:"createdAt"`
}

type BalanceWithdrawal struct {
	ID            int64      `json:"id"`
	CustomerID    int64      `json:"customerId"`
	TelegramID    int64      `json:"telegramId,omitempty"`
	Username      string     `json:"username,omitempty"`
	AmountCents   int64      `json:"amountCents"`
	PayoutDetails string     `json:"payoutDetails"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"createdAt"`
	ResolvedAt    *time.Time `json:"resolvedAt,omitempty"`
	ResolvedBy    *int64     `json:"resolvedBy,omitempty"`
}

type WalletRepository struct {
	pool *pgxpool.Pool
}

func NewWalletRepository(pool *pgxpool.Pool) *WalletRepository {
	return &WalletRepository{pool: pool}
}

func (r *WalletRepository) Balance(ctx context.Context, customerID int64) (int64, error) {
	var balance int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_cents), 0)
		FROM balance_transaction
		WHERE customer_id = $1
	`, customerID).Scan(&balance); err != nil {
		return 0, fmt.Errorf("load balance: %w", err)
	}
	return balance, nil
}

func (r *WalletRepository) Apply(ctx context.Context, customerID, amountCents int64, kind, referenceKey, description string) (int64, bool, error) {
	if customerID <= 0 || amountCents == 0 || strings.TrimSpace(referenceKey) == "" {
		return 0, false, errors.New("invalid balance transaction")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin balance transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", customerID); err != nil {
		return 0, false, fmt.Errorf("lock balance: %w", err)
	}
	var existingBalance int64
	err = tx.QueryRow(ctx, `SELECT balance_after_cents FROM balance_transaction WHERE reference_key = $1`, referenceKey).Scan(&existingBalance)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return 0, false, err
		}
		return existingBalance, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, fmt.Errorf("check balance reference: %w", err)
	}

	var balance int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(amount_cents), 0) FROM balance_transaction WHERE customer_id = $1`, customerID).Scan(&balance); err != nil {
		return 0, false, fmt.Errorf("load locked balance: %w", err)
	}
	next := balance + amountCents
	if next < 0 {
		return balance, false, ErrInsufficientBalance
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO balance_transaction (customer_id, amount_cents, balance_after_cents, kind, reference_key, description)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, customerID, amountCents, next, strings.TrimSpace(kind), strings.TrimSpace(referenceKey), strings.TrimSpace(description)); err != nil {
		return 0, false, fmt.Errorf("insert balance transaction: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("commit balance transaction: %w", err)
	}
	return next, true, nil
}

func (r *WalletRepository) ListTransactions(ctx context.Context, customerID int64, limit int) ([]BalanceTransaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, customer_id, amount_cents, balance_after_cents, kind, description, created_at
		FROM balance_transaction
		WHERE customer_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, customerID, limit)
	if err != nil {
		return nil, fmt.Errorf("list balance transactions: %w", err)
	}
	defer rows.Close()
	items := make([]BalanceTransaction, 0)
	for rows.Next() {
		var item BalanceTransaction
		if err := rows.Scan(&item.ID, &item.CustomerID, &item.AmountCents, &item.BalanceAfterCents, &item.Kind, &item.Description, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *WalletRepository) CreateWithdrawal(ctx context.Context, customerID, amountCents int64, payoutDetails string) (*BalanceWithdrawal, error) {
	payoutDetails = strings.TrimSpace(payoutDetails)
	if customerID <= 0 || amountCents <= 0 || payoutDetails == "" || len([]rune(payoutDetails)) > 1000 {
		return nil, errors.New("invalid withdrawal request")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", customerID); err != nil {
		return nil, err
	}
	var balance int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(amount_cents), 0) FROM balance_transaction WHERE customer_id = $1`, customerID).Scan(&balance); err != nil {
		return nil, err
	}
	if balance < amountCents {
		return nil, ErrInsufficientBalance
	}
	item := &BalanceWithdrawal{CustomerID: customerID, AmountCents: amountCents, PayoutDetails: payoutDetails, Status: "pending"}
	if err := tx.QueryRow(ctx, `
		INSERT INTO balance_withdrawal (customer_id, amount_cents, payout_details)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`, customerID, amountCents, payoutDetails).Scan(&item.ID, &item.CreatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO balance_transaction (customer_id, amount_cents, balance_after_cents, kind, reference_key, description)
		VALUES ($1, $2, $3, 'withdrawal_hold', $4, 'Заявка на вывод средств')
	`, customerID, -amountCents, balance-amountCents, fmt.Sprintf("withdrawal:%d", item.ID)); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *WalletRepository) ListWithdrawalsByCustomer(ctx context.Context, customerID int64, limit int) ([]BalanceWithdrawal, error) {
	return r.listWithdrawals(ctx, `
		SELECT w.id, w.customer_id, c.telegram_id, COALESCE(c.telegram_username, ''), w.amount_cents,
		       w.payout_details, w.status, w.created_at, w.resolved_at, w.resolved_by
		FROM balance_withdrawal w
		JOIN customer c ON c.id = w.customer_id
		WHERE w.customer_id = $1
		ORDER BY w.created_at DESC, w.id DESC LIMIT $2
	`, customerID, limit)
}

func (r *WalletRepository) ListPendingWithdrawals(ctx context.Context, limit int) ([]BalanceWithdrawal, error) {
	return r.listWithdrawals(ctx, `
		SELECT w.id, w.customer_id, c.telegram_id, COALESCE(c.telegram_username, ''), w.amount_cents,
		       w.payout_details, w.status, w.created_at, w.resolved_at, w.resolved_by
		FROM balance_withdrawal w
		JOIN customer c ON c.id = w.customer_id
		WHERE w.status = 'pending'
		ORDER BY w.created_at ASC, w.id ASC LIMIT $1
	`, limit)
}

func (r *WalletRepository) listWithdrawals(ctx context.Context, query string, args ...interface{}) ([]BalanceWithdrawal, error) {
	if len(args) > 0 {
		if value, ok := args[len(args)-1].(int); ok && (value <= 0 || value > 100) {
			args[len(args)-1] = 30
		}
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]BalanceWithdrawal, 0)
	for rows.Next() {
		var item BalanceWithdrawal
		if err := rows.Scan(&item.ID, &item.CustomerID, &item.TelegramID, &item.Username, &item.AmountCents, &item.PayoutDetails, &item.Status, &item.CreatedAt, &item.ResolvedAt, &item.ResolvedBy); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *WalletRepository) ResolveWithdrawal(ctx context.Context, withdrawalID, adminTelegramID int64, approve bool) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var customerID, amountCents int64
	var status string
	if err := tx.QueryRow(ctx, `SELECT customer_id, amount_cents, status FROM balance_withdrawal WHERE id = $1 FOR UPDATE`, withdrawalID).Scan(&customerID, &amountCents, &status); err != nil {
		return err
	}
	if status != "pending" {
		return ErrWithdrawalAlreadyResolved
	}
	nextStatus := "approved"
	if !approve {
		nextStatus = "rejected"
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", customerID); err != nil {
			return err
		}
		var balance int64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(amount_cents), 0) FROM balance_transaction WHERE customer_id = $1`, customerID).Scan(&balance); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO balance_transaction (customer_id, amount_cents, balance_after_cents, kind, reference_key, description)
			VALUES ($1, $2, $3, 'withdrawal_refund', $4, 'Возврат отклонённой заявки на вывод')
		`, customerID, amountCents, balance+amountCents, fmt.Sprintf("withdrawal-refund:%d", withdrawalID)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE balance_withdrawal SET status = $2, resolved_at = NOW(), resolved_by = $3 WHERE id = $1
	`, withdrawalID, nextStatus, adminTelegramID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
