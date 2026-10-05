package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v4"
)

type Administrator struct {
	CustomerID  int64    `json:"customerId"`
	TelegramID  int64    `json:"telegramId"`
	Username    string   `json:"username"`
	Role        string   `json:"role"`
	Color       string   `json:"color"`
	Permissions []string `json:"permissions"`
	IsOwner     bool     `json:"isOwner"`
}

func (cr *CustomerRepository) AdministratorForTelegramID(ctx context.Context, telegramID int64) (*Administrator, error) {
	var a Administrator
	err := cr.pool.QueryRow(ctx, `SELECT c.id, c.telegram_id, COALESCE(c.telegram_username,''), a.role_name, a.color, a.permissions
 FROM administrator a JOIN customer c ON c.id=a.customer_id WHERE c.telegram_id=$1 AND NOT c.is_blocked`, telegramID).
		Scan(&a.CustomerID, &a.TelegramID, &a.Username, &a.Role, &a.Color, &a.Permissions)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load administrator: %w", err)
	}
	return &a, nil
}

func (cr *CustomerRepository) ListAdministrators(ctx context.Context, ownerID int64, query string, limit, offset int) ([]Administrator, int, error) {
	query = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(query)), "@")
	search := "%" + query + "%"
	const filter = ` FROM customer c LEFT JOIN administrator a ON a.customer_id=c.id
 WHERE (a.customer_id IS NOT NULL OR c.telegram_id=$1)
 AND ($2='' OR LOWER(COALESCE(c.telegram_username,'')) LIKE $3 OR c.telegram_id::TEXT LIKE $3 OR LOWER(COALESCE(a.role_name,'Главный администратор')) LIKE $3)`
	var total int
	if err := cr.pool.QueryRow(ctx, "SELECT COUNT(*)"+filter, ownerID, query, search).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := cr.pool.Query(ctx, `SELECT c.id,c.telegram_id,COALESCE(c.telegram_username,''),
 CASE WHEN c.telegram_id=$1 THEN 'Главный администратор' ELSE a.role_name END,
 CASE WHEN c.telegram_id=$1 THEN '#69a8d4' ELSE a.color END,
 CASE WHEN c.telegram_id=$1 THEN '{}'::TEXT[] ELSE a.permissions END,c.telegram_id=$1`+filter+
		` ORDER BY (c.telegram_id=$1) DESC, a.created_at DESC, c.id DESC LIMIT $4 OFFSET $5`, ownerID, query, search, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]Administrator, 0, limit)
	for rows.Next() {
		var a Administrator
		if err := rows.Scan(&a.CustomerID, &a.TelegramID, &a.Username, &a.Role, &a.Color, &a.Permissions, &a.IsOwner); err != nil {
			return nil, 0, err
		}
		result = append(result, a)
	}
	return result, total, rows.Err()
}

func (cr *CustomerRepository) SaveAdministrator(ctx context.Context, a Administrator, ownerID int64) error {
	// The owner is implicit, never editable, and blocked accounts cannot receive access.
	tag, err := cr.pool.Exec(ctx, `INSERT INTO administrator(customer_id,role_name,color,permissions,updated_by)
 SELECT id,$2,$3,$4,$5 FROM customer WHERE id=$1 AND telegram_id<>$5 AND NOT is_blocked
 ON CONFLICT(customer_id) DO UPDATE SET role_name=EXCLUDED.role_name,color=EXCLUDED.color,
 permissions=EXCLUDED.permissions,updated_by=EXCLUDED.updated_by,updated_at=NOW()`, a.CustomerID, a.Role, a.Color, a.Permissions, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("пользователь недоступен для назначения")
	}
	return nil
}

func (cr *CustomerRepository) RemoveAdministrator(ctx context.Context, customerID, ownerID int64) error {
	tag, err := cr.pool.Exec(ctx, `DELETE FROM administrator a USING customer c WHERE a.customer_id=c.id AND c.id=$1 AND c.telegram_id<>$2`, customerID, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("администратор не найден или защищён")
	}
	return nil
}
