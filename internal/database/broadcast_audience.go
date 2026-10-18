package database

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"strings"
	"time"
)

type BroadcastAudience struct {
	IgnoreUnsubscribe bool     `json:"ignoreUnsubscribe"`
	Category          string   `json:"category"`
	InternalSquads    []string `json:"internalSquads"`
	ExternalSquads    []string `json:"externalSquads"`
	Tag               string   `json:"tag"`
	Language          string   `json:"language"`
}

func (a *BroadcastAudience) Normalize() error {
	a.Category = strings.ToLower(strings.TrimSpace(a.Category))
	if a.Category == "" {
		a.Category = "all"
	}
	switch a.Category {
	case "all", "active", "none", "expired", "trial", "trial_used", "paid":
	default:
		return errors.New("Неизвестная категория получателей")
	}
	for _, list := range []*[]string{&a.InternalSquads, &a.ExternalSquads} {
		if len(*list) > 100 {
			return errors.New("Слишком много сквадов")
		}
		seen := map[string]bool{}
		clean := []string{}
		for _, key := range *list {
			parsed, err := uuid.Parse(strings.TrimSpace(key))
			if err != nil {
				return errors.New("Некорректный сквад")
			}
			key = parsed.String()
			if !seen[key] {
				clean = append(clean, key)
				seen[key] = true
			}
		}
		*list = clean
	}
	a.Tag = strings.TrimSpace(a.Tag)
	if len(a.Tag) > 64 {
		return errors.New("Тег слишком длинный")
	}
	a.Language = strings.TrimSpace(a.Language)
	if a.Language != "" && a.Language != "ru" && a.Language != "en" && a.Language != "fa" {
		return errors.New("Неизвестный язык")
	}
	return nil
}

type BroadcastCustomer struct {
	Unsubscribed               bool
	ID, TelegramID             int64
	Synthetic, TrialUsed, Paid bool
	Email, Language            string
	ExpireAt                   *time.Time
	Subscriptions              []BroadcastSubscription
}
type BroadcastSubscription struct {
	PanelID   *int64
	PanelUUID *uuid.UUID
	ExpireAt  *time.Time
}

// One account may own several panel subscriptions. Resolve all of them and
// deduplicate recipients later, rather than consulting only the primary one.
func (r *CustomerRepository) BroadcastCustomers(ctx context.Context) ([]BroadcastCustomer, error) {
	rows, err := r.pool.Query(ctx, `SELECT c.id,c.telegram_id,c.telegram_id_is_synthetic,c.trial_used,
		EXISTS(SELECT 1 FROM purchase p WHERE p.customer_id=c.id AND p.status='paid' AND p.purchase_kind='subscription' AND p.amount>0 AND NOT p.is_free_plan),
		COALESCE(e.email,''),c.language,c.expire_at,s.panel_user_id,s.panel_user_uuid,s.expire_at,
		EXISTS(SELECT 1 FROM customer_broadcast_unsubscribe u WHERE u.customer_id=c.id)
		FROM customer c LEFT JOIN customer_email_auth e ON e.customer_id=c.id
		LEFT JOIN customer_subscription s ON s.customer_id=c.id WHERE NOT c.is_blocked ORDER BY c.id,s.position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []BroadcastCustomer{}
	var previous int64
	for rows.Next() {
		var c BroadcastCustomer
		var s BroadcastSubscription
		if err = rows.Scan(&c.ID, &c.TelegramID, &c.Synthetic, &c.TrialUsed, &c.Paid, &c.Email, &c.Language, &c.ExpireAt, &s.PanelID, &s.PanelUUID, &s.ExpireAt, &c.Unsubscribed); err != nil {
			return nil, err
		}
		if c.ID != previous {
			items = append(items, c)
			previous = c.ID
		}
		if s.PanelID != nil || s.PanelUUID != nil || s.ExpireAt != nil {
			items[len(items)-1].Subscriptions = append(items[len(items)-1].Subscriptions, s)
		}
	}
	return items, rows.Err()
}

func (r *BroadcastRepository) SaveAudience(ctx context.Context, a BroadcastAudience, by int64) (*BroadcastDraft, error) {
	if err := a.Normalize(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	draft, err := scanBroadcastDraft(r.pool.QueryRow(ctx, `UPDATE bot_broadcast_draft
		SET audience=$1,updated_by=$2,updated_at=GREATEST(NOW(), updated_at + INTERVAL '1 microsecond') WHERE id=1 AND status<>'running'
		RETURNING status, source_chat_id, source_message_id, source_kind, source_preview, source_html,
		buttons, recipient_count, sent_count, failed_count, last_error, updated_by, started_at, finished_at, updated_at, audience`, string(raw), by))
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, errors.New("Рассылка уже запущена")
	}
	return draft, nil
}
func (r *CustomerRepository) SaveEmailAudience(ctx context.Context, a BroadcastAudience, by int64) (*EmailBroadcastDraft, error) {
	a.IgnoreUnsubscribe = false
	if err := a.Normalize(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	draft, err := scanEmailBroadcastDraft(r.pool.QueryRow(ctx, `UPDATE email_broadcast
		SET audience=$1,updated_by=$2,updated_at=GREATEST(NOW(), updated_at + INTERVAL '1 microsecond') WHERE id=1 AND status<>'running'
		RETURNING status, subject, body, recipient_count, sent_count, failed_count, last_error, started_at, finished_at, audience, updated_at`, string(raw), by))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEmailBroadcastRunning
	}
	return draft, err
}
