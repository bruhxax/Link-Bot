package database

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v4"
	"time"
)

type BroadcastTemplate struct {
	ID        int64           `json:"id"`
	Channel   string          `json:"channel"`
	Name      string          `json:"name"`
	CreatedAt time.Time       `json:"createdAt"`
	Payload   json.RawMessage `json:"-"`
}
type BroadcastTemplatePayload struct {
	DeleteAfterMinutes int               `json:"deleteAfterMinutes,omitempty"`
	SourceChatID       *int64            `json:"sourceChatId,omitempty"`
	SourceMessageID    *int              `json:"sourceMessageId,omitempty"`
	SourceKind         string            `json:"sourceKind,omitempty"`
	SourcePreview      string            `json:"sourcePreview,omitempty"`
	SourceHTML         string            `json:"sourceHTML,omitempty"`
	Buttons            []BroadcastButton `json:"buttons,omitempty"`
	Subject            string            `json:"subject,omitempty"`
	Body               string            `json:"body,omitempty"`
	Audience           BroadcastAudience `json:"audience"`
}

func (cr *CustomerRepository) BroadcastTemplates(ctx context.Context) ([]BroadcastTemplate, error) {
	rows, err := cr.pool.Query(ctx, `SELECT id,channel,name,created_at FROM broadcast_template ORDER BY created_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []BroadcastTemplate{}
	for rows.Next() {
		var t BroadcastTemplate
		if err = rows.Scan(&t.ID, &t.Channel, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
func (cr *CustomerRepository) SaveBroadcastTemplate(ctx context.Context, channel, name string, payload BroadcastTemplatePayload, actor int64) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = cr.pool.Exec(ctx, `INSERT INTO broadcast_template(channel,name,payload,created_by) VALUES($1,$2,$3,$4)`, channel, name, string(raw), actor)
	return err
}
func (cr *CustomerRepository) BroadcastTemplate(ctx context.Context, id int64) (*BroadcastTemplate, error) {
	var t BroadcastTemplate
	err := cr.pool.QueryRow(ctx, `SELECT id,channel,name,created_at,payload FROM broadcast_template WHERE id=$1`, id).Scan(&t.ID, &t.Channel, &t.Name, &t.CreatedAt, &t.Payload)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &t, err
}
func (cr *CustomerRepository) DeleteBroadcastTemplate(ctx context.Context, id int64) error {
	_, err := cr.pool.Exec(ctx, `DELETE FROM broadcast_template WHERE id=$1`, id)
	return err
}
func (cr *CustomerRepository) RestoreEmailTemplate(ctx context.Context, p BroadcastTemplatePayload, actor int64, expected time.Time) (*EmailBroadcastDraft, error) {
	return scanEmailBroadcastDraft(cr.pool.QueryRow(ctx, `UPDATE email_broadcast SET status='draft',subject=$1,body=$2,audience=$3,
 recipient_count=0,sent_count=0,failed_count=0,last_error='',started_at=NULL,finished_at=NULL,updated_by=$4,
 updated_at=GREATEST(NOW(),updated_at+INTERVAL '1 microsecond') WHERE id=1 AND status<>'running' AND updated_at=$5
 RETURNING status,subject,body,recipient_count,sent_count,failed_count,last_error,started_at,finished_at,audience,updated_at`, p.Subject, p.Body, p.AudienceJSON(), actor, expected))
}
func (p BroadcastTemplatePayload) AudienceJSON() string {
	raw, _ := json.Marshal(p.Audience)
	return string(raw)
}
func (r *BroadcastRepository) RestoreTemplate(ctx context.Context, p BroadcastTemplatePayload, actor int64, expected time.Time) (*BroadcastDraft, error) {
	raw, err := json.Marshal(p.Buttons)
	if err != nil {
		return nil, err
	}
	return scanBroadcastDraft(r.pool.QueryRow(ctx, `UPDATE bot_broadcast_draft SET status='draft',source_chat_id=$1,source_message_id=$2,
 source_kind=$3,source_preview=$4,source_html=$5,buttons=$6,audience=$7,recipient_count=0,sent_count=0,failed_count=0,last_error='',
 started_at=NULL,finished_at=NULL,delete_after_minutes=$10,updated_by=$8,updated_at=GREATEST(NOW(),updated_at+INTERVAL '1 microsecond')
 WHERE id=1 AND status<>'running' AND updated_at=$9
 RETURNING status,source_chat_id,source_message_id,source_kind,source_preview,source_html,buttons,recipient_count,sent_count,failed_count,
 last_error,updated_by,started_at,finished_at,updated_at,audience, delete_after_minutes`, p.SourceChatID, p.SourceMessageID, p.SourceKind, p.SourcePreview, p.SourceHTML, string(raw), p.AudienceJSON(), actor, expected, p.DeleteAfterMinutes))
}
