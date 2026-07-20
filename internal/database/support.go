package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type SupportTicketStatus string

const (
	SupportTicketStatusOpen   SupportTicketStatus = "open"
	SupportTicketStatusClosed SupportTicketStatus = "closed"
)

type SupportAuthorRole string

const (
	SupportAuthorRoleCustomer SupportAuthorRole = "customer"
	SupportAuthorRoleAdmin    SupportAuthorRole = "admin"
)

type SupportTicket struct {
	ID                  int64               `db:"id"`
	CustomerID          int64               `db:"customer_id"`
	Status              SupportTicketStatus `db:"status"`
	Subject             string              `db:"subject"`
	CustomerName        string              `db:"customer_name"`
	CustomerUsername    string              `db:"customer_username"`
	SubscriptionLabel   string              `db:"subscription_label"`
	CreatedAt           time.Time           `db:"created_at"`
	UpdatedAt           time.Time           `db:"updated_at"`
	LastMessageAt       time.Time           `db:"last_message_at"`
	ClosedAt            *time.Time          `db:"closed_at"`
	LastMessagePreview  string              `db:"last_message_preview"`
	AdminUnreadCount    int                 `db:"admin_unread_count"`
	CustomerUnreadCount int                 `db:"customer_unread_count"`
}

type SupportMessage struct {
	ID               int64             `db:"id"`
	TicketID         int64             `db:"ticket_id"`
	AuthorRole       SupportAuthorRole `db:"author_role"`
	AuthorTelegramID int64             `db:"author_telegram_id"`
	Body             string            `db:"body"`
	CreatedAt        time.Time         `db:"created_at"`
}

type SupportRepository struct {
	pool *pgxpool.Pool
}

func NewSupportRepository(pool *pgxpool.Pool) *SupportRepository {
	return &SupportRepository{pool: pool}
}

func (r *SupportRepository) CreateTicket(ctx context.Context, ticket *SupportTicket, firstMessage *SupportMessage) (*SupportTicket, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	now := time.Now().UTC()
	preview := supportPreview(firstMessage.Body)

	query := `
		INSERT INTO support_ticket (
			customer_id, status, subject, customer_name, customer_username, subscription_label,
			created_at, updated_at, last_message_at, last_message_preview, admin_unread_count, customer_unread_count
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $7, $8, 1, 0)
		RETURNING id, customer_id, status, subject, customer_name, customer_username, subscription_label,
		          created_at, updated_at, last_message_at, closed_at, last_message_preview, admin_unread_count, customer_unread_count
	`

	createdTicket := &SupportTicket{}
	err = tx.QueryRow(
		ctx,
		query,
		ticket.CustomerID,
		SupportTicketStatusOpen,
		strings.TrimSpace(ticket.Subject),
		strings.TrimSpace(ticket.CustomerName),
		strings.TrimSpace(ticket.CustomerUsername),
		strings.TrimSpace(ticket.SubscriptionLabel),
		now,
		preview,
	).Scan(
		&createdTicket.ID,
		&createdTicket.CustomerID,
		&createdTicket.Status,
		&createdTicket.Subject,
		&createdTicket.CustomerName,
		&createdTicket.CustomerUsername,
		&createdTicket.SubscriptionLabel,
		&createdTicket.CreatedAt,
		&createdTicket.UpdatedAt,
		&createdTicket.LastMessageAt,
		&createdTicket.ClosedAt,
		&createdTicket.LastMessagePreview,
		&createdTicket.AdminUnreadCount,
		&createdTicket.CustomerUnreadCount,
	)
	if err != nil {
		return nil, fmt.Errorf("insert support ticket: %w", err)
	}

	msgQuery := `
		INSERT INTO support_message (ticket_id, author_role, author_telegram_id, body, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, ticket_id, author_role, author_telegram_id, body, created_at
	`
	err = tx.QueryRow(
		ctx,
		msgQuery,
		createdTicket.ID,
		SupportAuthorRoleCustomer,
		firstMessage.AuthorTelegramID,
		strings.TrimSpace(firstMessage.Body),
		now,
	).Scan(
		&firstMessage.ID,
		&firstMessage.TicketID,
		&firstMessage.AuthorRole,
		&firstMessage.AuthorTelegramID,
		&firstMessage.Body,
		&firstMessage.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert support message: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return createdTicket, nil
}

func (r *SupportRepository) FindTicketByID(ctx context.Context, id int64) (*SupportTicket, error) {
	query := sq.Select(
		"id", "customer_id", "status", "subject", "customer_name", "customer_username", "subscription_label",
		"created_at", "updated_at", "last_message_at", "closed_at", "last_message_preview", "admin_unread_count", "customer_unread_count",
	).
		From("support_ticket").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	ticket := &SupportTicket{}
	err = r.pool.QueryRow(ctx, sql, args...).Scan(
		&ticket.ID,
		&ticket.CustomerID,
		&ticket.Status,
		&ticket.Subject,
		&ticket.CustomerName,
		&ticket.CustomerUsername,
		&ticket.SubscriptionLabel,
		&ticket.CreatedAt,
		&ticket.UpdatedAt,
		&ticket.LastMessageAt,
		&ticket.ClosedAt,
		&ticket.LastMessagePreview,
		&ticket.AdminUnreadCount,
		&ticket.CustomerUnreadCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query support ticket: %w", err)
	}

	return ticket, nil
}

func (r *SupportRepository) ListTicketsByCustomer(ctx context.Context, customerID int64, status SupportTicketStatus) ([]SupportTicket, error) {
	query := sq.Select(
		"id", "customer_id", "status", "subject", "customer_name", "customer_username", "subscription_label",
		"created_at", "updated_at", "last_message_at", "closed_at", "last_message_preview", "admin_unread_count", "customer_unread_count",
	).
		From("support_ticket").
		Where(sq.And{sq.Eq{"customer_id": customerID}, sq.Eq{"status": status}}).
		OrderBy("last_message_at DESC").
		PlaceholderFormat(sq.Dollar)

	return r.listTickets(ctx, query)
}

func (r *SupportRepository) ListTicketsForAdmin(ctx context.Context, status SupportTicketStatus) ([]SupportTicket, error) {
	query := sq.Select(
		"id", "customer_id", "status", "subject", "customer_name", "customer_username", "subscription_label",
		"created_at", "updated_at", "last_message_at", "closed_at", "last_message_preview", "admin_unread_count", "customer_unread_count",
	).
		From("support_ticket").
		Where(sq.Eq{"status": status}).
		OrderBy("last_message_at DESC").
		PlaceholderFormat(sq.Dollar)

	return r.listTickets(ctx, query)
}

func (r *SupportRepository) listTickets(ctx context.Context, query sq.SelectBuilder) ([]SupportTicket, error) {
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query support tickets: %w", err)
	}
	defer rows.Close()

	var tickets []SupportTicket
	for rows.Next() {
		var ticket SupportTicket
		if err := rows.Scan(
			&ticket.ID,
			&ticket.CustomerID,
			&ticket.Status,
			&ticket.Subject,
			&ticket.CustomerName,
			&ticket.CustomerUsername,
			&ticket.SubscriptionLabel,
			&ticket.CreatedAt,
			&ticket.UpdatedAt,
			&ticket.LastMessageAt,
			&ticket.ClosedAt,
			&ticket.LastMessagePreview,
			&ticket.AdminUnreadCount,
			&ticket.CustomerUnreadCount,
		); err != nil {
			return nil, fmt.Errorf("scan support ticket: %w", err)
		}
		tickets = append(tickets, ticket)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate support tickets: %w", err)
	}

	return tickets, nil
}

func (r *SupportRepository) ListMessagesByTicket(ctx context.Context, ticketID int64) ([]SupportMessage, error) {
	query := sq.Select("id", "ticket_id", "author_role", "author_telegram_id", "body", "created_at").
		From("support_message").
		Where(sq.Eq{"ticket_id": ticketID}).
		OrderBy("created_at ASC", "id ASC").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query support messages: %w", err)
	}
	defer rows.Close()

	var messages []SupportMessage
	for rows.Next() {
		var message SupportMessage
		if err := rows.Scan(
			&message.ID,
			&message.TicketID,
			&message.AuthorRole,
			&message.AuthorTelegramID,
			&message.Body,
			&message.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan support message: %w", err)
		}
		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate support messages: %w", err)
	}

	return messages, nil
}

func (r *SupportRepository) AddCustomerMessage(ctx context.Context, ticketID int64, telegramID int64, body, customerName, customerUsername, subscriptionLabel string) (*SupportMessage, error) {
	return r.addMessage(ctx, ticketID, SupportAuthorRoleCustomer, telegramID, body, ticketUpdateMeta{
		customerName:      customerName,
		customerUsername:  customerUsername,
		subscriptionLabel: subscriptionLabel,
	})
}

func (r *SupportRepository) AddAdminMessage(ctx context.Context, ticketID int64, telegramID int64, body string) (*SupportMessage, error) {
	return r.addMessage(ctx, ticketID, SupportAuthorRoleAdmin, telegramID, body, ticketUpdateMeta{})
}

type ticketUpdateMeta struct {
	customerName      string
	customerUsername  string
	subscriptionLabel string
}

func (r *SupportRepository) addMessage(ctx context.Context, ticketID int64, role SupportAuthorRole, telegramID int64, body string, meta ticketUpdateMeta) (*SupportMessage, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var status SupportTicketStatus
	if err := tx.QueryRow(ctx, `SELECT status FROM support_ticket WHERE id = $1 FOR UPDATE`, ticketID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("lock support ticket: %w", err)
	}
	if status == SupportTicketStatusClosed {
		return nil, fmt.Errorf("support ticket is closed")
	}

	now := time.Now().UTC()
	cleanBody := strings.TrimSpace(body)
	preview := supportPreview(cleanBody)

	message := &SupportMessage{}
	err = tx.QueryRow(
		ctx,
		`INSERT INTO support_message (ticket_id, author_role, author_telegram_id, body, created_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, ticket_id, author_role, author_telegram_id, body, created_at`,
		ticketID, role, telegramID, cleanBody, now,
	).Scan(
		&message.ID,
		&message.TicketID,
		&message.AuthorRole,
		&message.AuthorTelegramID,
		&message.Body,
		&message.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert support message: %w", err)
	}

	updateQuery := `
		UPDATE support_ticket
		SET updated_at = $2,
		    last_message_at = $2,
		    last_message_preview = $3,
		    admin_unread_count = CASE WHEN $4 = 'customer' THEN admin_unread_count + 1 ELSE 0 END,
		    customer_unread_count = CASE WHEN $4 = 'admin' THEN customer_unread_count + 1 ELSE 0 END,
		    customer_name = CASE WHEN $4 = 'customer' AND $5 <> '' THEN $5 ELSE customer_name END,
		    customer_username = CASE WHEN $4 = 'customer' AND $6 <> '' THEN $6 ELSE customer_username END,
		    subscription_label = CASE WHEN $4 = 'customer' AND $7 <> '' THEN $7 ELSE subscription_label END
		WHERE id = $1
	`
	if _, err := tx.Exec(ctx, updateQuery, ticketID, now, preview, role, strings.TrimSpace(meta.customerName), strings.TrimSpace(meta.customerUsername), strings.TrimSpace(meta.subscriptionLabel)); err != nil {
		return nil, fmt.Errorf("update support ticket: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return message, nil
}

func (r *SupportRepository) CloseTicket(ctx context.Context, ticketID int64) error {
	now := time.Now().UTC()
	result, err := r.pool.Exec(
		ctx,
		`UPDATE support_ticket
		 SET status = $2, updated_at = $3, closed_at = $3, admin_unread_count = 0
		 WHERE id = $1 AND status <> $2`,
		ticketID, SupportTicketStatusClosed, now,
	)
	if err != nil {
		return fmt.Errorf("close support ticket: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	return nil
}

func (r *SupportRepository) MarkSeenByCustomer(ctx context.Context, ticketID int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE support_ticket SET customer_unread_count = 0 WHERE id = $1`, ticketID)
	if err != nil {
		return fmt.Errorf("mark support ticket seen by customer: %w", err)
	}
	return nil
}

func (r *SupportRepository) MarkSeenByAdmin(ctx context.Context, ticketID int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE support_ticket SET admin_unread_count = 0 WHERE id = $1`, ticketID)
	if err != nil {
		return fmt.Errorf("mark support ticket seen by admin: %w", err)
	}
	return nil
}

func supportPreview(body string) string {
	body = strings.Join(strings.Fields(strings.TrimSpace(body)), " ")
	runes := []rune(body)
	if len(runes) > 120 {
		return string(runes[:120]) + "..."
	}
	return body
}
