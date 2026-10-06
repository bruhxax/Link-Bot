package database

import (
	"context"
	"encoding/json"
	"time"
)

type AdminActivityDetail struct {
	Label  string `json:"label"`
	Value  string `json:"value,omitempty"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

type AdminActivity struct {
	ID               int64                 `json:"id"`
	ActorTelegramID  int64                 `json:"actorTelegramId"`
	ActorName        string                `json:"actorName"`
	ActorRole        string                `json:"actorRole"`
	Action           string                `json:"action"`
	Category         string                `json:"category"`
	Title            string                `json:"title"`
	TargetCustomerID int64                 `json:"targetCustomerId,omitempty"`
	TargetTelegramID int64                 `json:"targetTelegramId,omitempty"`
	TargetName       string                `json:"targetName,omitempty"`
	Details          []AdminActivityDetail `json:"details"`
	Status           string                `json:"status"`
	CreatedAt        time.Time             `json:"createdAt"`
	FinishedAt       *time.Time            `json:"finishedAt,omitempty"`
}

type AdminActivityQuery struct {
	TelegramID int64  `json:"telegramId"`
	BeforeID   int64  `json:"beforeId"`
	Limit      int    `json:"limit"`
	Category   string `json:"category"`
	Query      string `json:"query"`
}

func (cr *CustomerRepository) CreateAdminActivity(ctx context.Context, entry AdminActivity) (int64, error) {
	if entry.Details == nil {
		entry.Details = []AdminActivityDetail{}
	}
	details, err := json.Marshal(entry.Details)
	if err != nil {
		return 0, err
	}
	var id int64
	err = cr.pool.QueryRow(ctx, `INSERT INTO admin_activity(actor_telegram_id,actor_name,actor_role,action,category,title,target_customer_id,target_telegram_id,target_name,details,status,finished_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,CASE WHEN $11='pending' THEN NULL ELSE NOW() END) RETURNING id`,
		entry.ActorTelegramID, entry.ActorName, entry.ActorRole, entry.Action, entry.Category, entry.Title, entry.TargetCustomerID, entry.TargetTelegramID, entry.TargetName, string(details), entry.Status).Scan(&id)
	return id, err
}

func (cr *CustomerRepository) FinishAdminActivity(ctx context.Context, entry AdminActivity) error {
	if entry.Details == nil {
		entry.Details = []AdminActivityDetail{}
	}
	details, err := json.Marshal(entry.Details)
	if err != nil {
		return err
	}
	_, err = cr.pool.Exec(ctx, `UPDATE admin_activity SET title=$2,target_customer_id=$3,target_telegram_id=$4,target_name=$5,details=$6,status=$7,finished_at=NOW() WHERE id=$1 AND status='pending'`,
		entry.ID, entry.Title, entry.TargetCustomerID, entry.TargetTelegramID, entry.TargetName, string(details), entry.Status)
	return err
}

func (cr *CustomerRepository) ListAdminActivity(ctx context.Context, q AdminActivityQuery) ([]AdminActivity, bool, error) {
	// Legacy detail requests mixed real visits with background refreshes. Hide
	// these unreliable entries without removing raw history from the database.
	rows, err := cr.pool.Query(ctx, `SELECT id,actor_telegram_id,actor_name,actor_role,action,category,title,target_customer_id,target_telegram_id,target_name,details,status,created_at,finished_at
 FROM admin_activity WHERE actor_telegram_id=$1 AND action<>'users/detail' AND ($2=0 OR id<$2) AND ($3='' OR category=$3)
 AND ($4='' OR title ILIKE $5 OR target_name ILIKE $5 OR target_telegram_id::TEXT ILIKE $5 OR details::TEXT ILIKE $5)
 ORDER BY id DESC LIMIT $6`, q.TelegramID, q.BeforeID, q.Category, q.Query, "%"+q.Query+"%", q.Limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]AdminActivity, 0, q.Limit+1)
	for rows.Next() {
		var entry AdminActivity
		var details []byte
		if err := rows.Scan(&entry.ID, &entry.ActorTelegramID, &entry.ActorName, &entry.ActorRole, &entry.Action, &entry.Category, &entry.Title, &entry.TargetCustomerID, &entry.TargetTelegramID, &entry.TargetName, &details, &entry.Status, &entry.CreatedAt, &entry.FinishedAt); err != nil {
			return nil, false, err
		}
		if err := json.Unmarshal(details, &entry.Details); err != nil {
			return nil, false, err
		}
		items = append(items, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(items) > q.Limit
	if hasMore {
		items = items[:q.Limit]
	}
	return items, hasMore, nil
}
