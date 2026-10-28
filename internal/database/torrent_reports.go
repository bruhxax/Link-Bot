package database

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type TorrentReport struct {
	ID          int64      `json:"id"`
	EventKey    string     `json:"-"`
	UserUUID    uuid.UUID  `json:"userUuid"`
	PanelUserID int64      `json:"-"`
	Username    string     `json:"username"`
	Node        string     `json:"node"`
	IP          string     `json:"ip"`
	Blocked     bool       `json:"blocked"`
	Duration    int64      `json:"duration"`
	UnblockAt   *time.Time `json:"unblockAt"`
	OccurredAt  time.Time  `json:"occurredAt"`
	Protocol    string     `json:"protocol"`
	Network     string     `json:"network"`
	CustomerID  int64      `json:"customerId"`
}
type TorrentSummary struct {
	Total  int64 `json:"total"`
	Today  int64 `json:"today"`
	Active int64 `json:"active"`
	Users  int64 `json:"users"`
}

func (r *CustomerRepository) SaveTorrentReport(ctx context.Context, t TorrentReport) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO torrent_report(event_key,panel_user_uuid,panel_user_id,username,node_name,ip,blocked,block_duration,unblock_at,occurred_at,protocol,network)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
 ON CONFLICT(panel_user_uuid,node_name,ip,occurred_at) DO UPDATE SET event_key=EXCLUDED.event_key,panel_user_id=EXCLUDED.panel_user_id,username=EXCLUDED.username,blocked=EXCLUDED.blocked,block_duration=EXCLUDED.block_duration,unblock_at=EXCLUDED.unblock_at,protocol=EXCLUDED.protocol,network=EXCLUDED.network
 WHERE torrent_report.event_key IS DISTINCT FROM EXCLUDED.event_key`, t.EventKey, t.UserUUID, t.PanelUserID, t.Username, t.Node, t.IP, t.Blocked, t.Duration, t.UnblockAt, t.OccurredAt, t.Protocol, t.Network)
	return err
}

func (r *CustomerRepository) ListTorrentReports(ctx context.Context, query, status string, before int64) ([]TorrentReport, bool, TorrentSummary, error) {
	var summary TorrentSummary
	err := r.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE occurred_at>NOW()-INTERVAL '24 hours'),count(*) FILTER(WHERE blocked AND unblock_at>NOW()),count(DISTINCT panel_user_uuid) FROM torrent_report`).Scan(&summary.Total, &summary.Today, &summary.Active, &summary.Users)
	if err != nil {
		return nil, false, summary, err
	}
	rows, err := r.pool.Query(ctx, `SELECT t.id,t.panel_user_uuid,t.username,t.node_name,t.ip,t.blocked,t.block_duration,t.unblock_at,t.occurred_at,t.protocol,t.network,COALESCE(s.customer_id,0)
 FROM torrent_report t LEFT JOIN LATERAL(SELECT customer_id FROM customer_subscription WHERE panel_user_uuid=t.panel_user_uuid OR(panel_user_uuid IS NULL AND panel_user_id=t.panel_user_id AND t.panel_user_id>0) ORDER BY is_primary DESC,id LIMIT 1) s ON TRUE
 WHERE($1::text='' OR strpos(lower(t.username||' '||t.node_name||' '||t.ip||' '||t.panel_user_uuid::text),lower($1))>0)
 AND($2='all' OR($2='active' AND t.blocked AND t.unblock_at>NOW()) OR($2='released' AND t.blocked AND t.unblock_at<=NOW()) OR($2='observed' AND NOT t.blocked))
 AND($3::bigint=0 OR t.id<$3) ORDER BY t.id DESC LIMIT 31`, query, status, before)
	if err != nil {
		return nil, false, summary, err
	}
	defer rows.Close()
	items := make([]TorrentReport, 0, 31)
	for rows.Next() {
		var t TorrentReport
		if err := rows.Scan(&t.ID, &t.UserUUID, &t.Username, &t.Node, &t.IP, &t.Blocked, &t.Duration, &t.UnblockAt, &t.OccurredAt, &t.Protocol, &t.Network, &t.CustomerID); err != nil {
			return nil, false, summary, err
		}
		items = append(items, t)
	}
	more := len(items) > 30
	if more {
		items = items[:30]
	}
	return items, more, summary, rows.Err()
}

func (r *CustomerRepository) TorrentRecipients(ctx context.Context, user uuid.UUID, panelID int64) ([]int64, error) {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT c.telegram_id FROM customer_subscription s JOIN customer c ON c.id=s.customer_id
 WHERE(s.panel_user_uuid=$1 OR(s.panel_user_uuid IS NULL AND s.panel_user_id=$2 AND $2>0)) AND c.telegram_id>0 AND NOT c.telegram_id_is_synthetic AND NOT c.is_blocked`, user, panelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *CustomerRepository) ClaimTorrentUserAlert(ctx context.Context, key string, id int64, token uuid.UUID) (bool, bool, error) {
	_, err := r.pool.Exec(ctx, `INSERT INTO torrent_user_alert(event_key,telegram_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, key, id)
	if err != nil {
		return false, false, err
	}
	result, err := r.pool.Exec(ctx, `UPDATE torrent_user_alert SET notification_token=$3,lease_until=NOW()+INTERVAL '30 seconds' WHERE event_key=$1 AND telegram_id=$2 AND delivered_at IS NULL AND(lease_until IS NULL OR lease_until<NOW())`, key, id, token)
	if err != nil {
		return false, false, err
	}
	if result.RowsAffected() > 0 {
		return true, false, nil
	}
	var delivered bool
	err = r.pool.QueryRow(ctx, `SELECT delivered_at IS NOT NULL FROM torrent_user_alert WHERE event_key=$1 AND telegram_id=$2`, key, id).Scan(&delivered)
	return false, delivered, err
}
func (r *CustomerRepository) FinishTorrentUserAlert(ctx context.Context, key string, id int64, token uuid.UUID, delivered bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE torrent_user_alert SET delivered_at=CASE WHEN $4 THEN NOW() ELSE delivered_at END,notification_token=NULL,lease_until=NULL WHERE event_key=$1 AND telegram_id=$2 AND notification_token=$3`, key, id, token, delivered)
	return err
}
