package database

import (
	"context"
	"encoding/json"
	"time"
)

type UserActivityDetail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type UserActivity struct {
	ID         int64                `json:"id"`
	CustomerID int64                `json:"customerId"`
	Source     string               `json:"source"`
	Action     string               `json:"action"`
	Title      string               `json:"title"`
	Status     string               `json:"status"`
	Details    []UserActivityDetail `json:"details"`
	CreatedAt  time.Time            `json:"createdAt"`
}

func (r *CustomerRepository) RecordUserActivity(ctx context.Context, entry UserActivity) error {
	if entry.Details == nil {
		entry.Details = []UserActivityDetail{}
	}
	raw, err := json.Marshal(entry.Details)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO user_activity(customer_id,source,action,title,status,details) VALUES($1,$2,$3,$4,$5,$6)`,
		entry.CustomerID, entry.Source, entry.Action, entry.Title, entry.Status, string(raw))
	return err
}

// The ID cursor is stable when new events arrive while older pages are open.
func (r *CustomerRepository) ListUserActivity(ctx context.Context, customerID, beforeID int64, limit int) ([]UserActivity, bool, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	rows, err := r.pool.Query(ctx, `SELECT id,customer_id,source,action,title,status,details,created_at FROM user_activity
        WHERE customer_id=$1 AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT $3`, customerID, beforeID, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]UserActivity, 0, limit+1)
	for rows.Next() {
		var entry UserActivity
		var raw []byte
		if err := rows.Scan(&entry.ID, &entry.CustomerID, &entry.Source, &entry.Action, &entry.Title, &entry.Status, &raw, &entry.CreatedAt); err != nil {
			return nil, false, err
		}
		if err := json.Unmarshal(raw, &entry.Details); err != nil {
			return nil, false, err
		}
		items = append(items, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	return items, more, nil
}
