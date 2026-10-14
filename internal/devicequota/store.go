package devicequota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"link-bot/internal/remnawave"
	"link-bot/internal/trafficpolicy"
)

type Slot struct {
	Number      int                  `json:"number"`
	Device      remnawave.UserDevice `json:"device"`
	ChildID     int64                `json:"childId"`
	ChildUUID   uuid.UUID            `json:"childUuid"`
	Registered  bool                 `json:"registered"`
	RevokeHWID  string               `json:"revokeHwid,omitempty"`
	Username    string               `json:"username,omitempty"`
	Paused      bool                 `json:"paused,omitempty"`
	UserLimit   *int64               `json:"userLimit,omitempty"`
	DropPending bool                 `json:"dropPending,omitempty"`
}

type BlockedDevice struct {
	Device    remnawave.UserDevice `json:"device"`
	BlockedAt time.Time            `json:"blockedAt"`
}
type Binding struct {
	ID             int64
	ParentKey      string
	ParentID       int64
	ParentUUID     uuid.UUID
	SubscriptionID *int64
	Token          uuid.UUID
	Squads         []string
	Slots          []*Slot
	Isolated       bool
	Personal       bool
	Blocked        []BlockedDevice
}
type Store interface {
	FindParent(context.Context, string) (*Binding, error)
	FindToken(context.Context, uuid.UUID) (*Binding, error)
	Create(context.Context, *Binding) error
	Locked(context.Context, int64, func(*Binding, func() error) error) error
	List(context.Context) ([]*Binding, error)
	Plan(context.Context, *remnawave.PanelUser) (int64, string, *trafficpolicy.Terms, error)
	Candidates(context.Context, []string) ([]remnawave.PanelUser, error)
	LoadConfig(context.Context, int64, int, string, string, string) (*Configuration, error)
	SaveConfig(context.Context, int64, int, string, string, string, *Configuration) error
}
type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

const columns = `id,parent_key,parent_id,parent_uuid,subscription_id,token,squads,slots,isolated,personal,blocked`

func scan(row pgx.Row) (*Binding, error) {
	var b Binding
	var squads, slots, blocked []byte
	err := row.Scan(&b.ID, &b.ParentKey, &b.ParentID, &b.ParentUUID, &b.SubscriptionID, &b.Token, &squads, &slots, &b.Isolated, &b.Personal, &blocked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(squads, &b.Squads); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(slots, &b.Slots); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(blocked, &b.Blocked); err != nil {
		return nil, err
	}
	return &b, nil
}
func (s *PostgresStore) FindParent(ctx context.Context, key string) (*Binding, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM device_traffic_binding WHERE parent_key=$1`, key))
}
func (s *PostgresStore) FindToken(ctx context.Context, token uuid.UUID) (*Binding, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM device_traffic_binding WHERE token=$1`, token))
}
func (s *PostgresStore) Create(ctx context.Context, b *Binding) error {
	squads, err := json.Marshal(b.Squads)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO device_traffic_binding(parent_key,parent_id,parent_uuid,subscription_id,token,squads,personal) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(parent_key) DO NOTHING`, b.ParentKey, b.ParentID, b.ParentUUID, b.SubscriptionID, b.Token, string(squads), b.Personal)
	return err
}
func (s *PostgresStore) Locked(ctx context.Context, id int64, fn func(*Binding, func() error) error) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	// Session lock spans durable checkpoints and remote API mutations. Never hold
	// an open transaction across a remote request; crashes retain recovery state.
	var locked bool
	err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(682914,$1::int)`, id).Scan(&locked)
	if err != nil {
		return err
	}
	if !locked {
		return ErrBusy
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, `SELECT pg_advisory_unlock(682914,$1::int)`, id); err != nil {
			_ = conn.Conn().Close(unlockCtx)
		}
	}()
	b, err := scan(conn.QueryRow(ctx, `SELECT `+columns+` FROM device_traffic_binding WHERE id=$1`, id))
	if err != nil {
		return err
	}
	if b == nil {
		return ErrUnavailable
	}
	save := func() error {
		squads, e := json.Marshal(b.Squads)
		if e != nil {
			return e
		}
		slots, e := json.Marshal(b.Slots)
		if e != nil {
			return e
		}
		blocked, e := json.Marshal(b.Blocked)
		if e != nil {
			return e
		}
		_, e = conn.Exec(ctx, `UPDATE device_traffic_binding SET squads=$2,slots=$3,isolated=$4,personal=$5,blocked=$6,updated_at=NOW() WHERE id=$1`, b.ID, string(squads), string(slots), b.Isolated, b.Personal, string(blocked))
		return e
	}
	return fn(b, save)
}
func (s *PostgresStore) List(ctx context.Context) ([]*Binding, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM device_traffic_binding ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*Binding
	for rows.Next() {
		b, e := scan(rows)
		if e != nil {
			return nil, e
		}
		result = append(result, b)
	}
	return result, rows.Err()
}
func (s *PostgresStore) Plan(ctx context.Context, u *remnawave.PanelUser) (int64, string, *trafficpolicy.Terms, error) {
	var id int64
	var plan string
	var terms *trafficpolicy.Terms
	// Identity takes precedence. Telegram fallback is only for an as-yet unbound
	// primary subscription; technical profiles never have a Telegram ID.
	err := s.pool.QueryRow(ctx, `SELECT cs.id,COALESCE(p.plan_id,''),p.device_traffic_terms FROM customer_subscription cs JOIN customer c ON c.id=cs.customer_id
 LEFT JOIN LATERAL (SELECT plan_id,device_traffic_terms FROM purchase WHERE customer_id=cs.customer_id AND subscription_id=cs.id AND status='paid' AND (month>0 OR days>0) ORDER BY paid_at DESC NULLS LAST,created_at DESC,id DESC LIMIT 1) p ON TRUE
 WHERE (cs.panel_user_id=$1 AND $1>0) OR (cs.panel_user_uuid=$2 AND $2<>'00000000-0000-0000-0000-000000000000'::uuid)
 OR (cs.is_primary AND cs.panel_user_id IS NULL AND cs.panel_user_uuid IS NULL AND c.telegram_id=$3)
 ORDER BY CASE WHEN (cs.panel_user_id=$1 AND $1>0) OR (cs.panel_user_uuid=$2 AND $2<>'00000000-0000-0000-0000-000000000000'::uuid) THEN 0 ELSE 1 END,cs.id LIMIT 1`, u.ID, u.UUID, u.TelegramID).Scan(&id, &plan, &terms)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", nil, nil
	}
	if err != nil {
		return 0, "", nil, fmt.Errorf("resolve device quota plan: %w", err)
	}
	return id, plan, terms, nil
}

func (s *PostgresStore) Candidates(ctx context.Context, plans []string) ([]remnawave.PanelUser, error) {
	if plans == nil {
		plans = []string{}
	}
	rows, err := s.pool.Query(ctx, `SELECT COALESCE(cs.panel_user_id,0),COALESCE(cs.panel_user_uuid,'00000000-0000-0000-0000-000000000000'::uuid),c.telegram_id
	FROM customer_subscription cs JOIN customer c ON c.id=cs.customer_id
	LEFT JOIN LATERAL (SELECT plan_id,device_traffic_terms FROM purchase WHERE customer_id=cs.customer_id AND subscription_id=cs.id AND status='paid' AND (month>0 OR days>0) ORDER BY paid_at DESC NULLS LAST,created_at DESC,id DESC LIMIT 1) p ON TRUE
	WHERE (((p.device_traffic_terms->>'enabled')::boolean AND (p.device_traffic_terms->>'enforced')::boolean) OR (p.device_traffic_terms IS NULL AND p.plan_id=ANY($1::text[])) OR '*'=ANY($1::text[]))
	AND (cs.panel_user_id>0 OR cs.panel_user_uuid IS NOT NULL OR cs.is_primary)
	AND NOT EXISTS(SELECT 1 FROM device_traffic_binding b WHERE b.subscription_id=cs.id AND ((b.parent_id=cs.panel_user_id AND b.parent_id>0) OR (b.parent_uuid=cs.panel_user_uuid AND b.parent_uuid<>'00000000-0000-0000-0000-000000000000'::uuid) OR (cs.panel_user_id IS NULL AND cs.panel_user_uuid IS NULL)))`, plans)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []remnawave.PanelUser
	for rows.Next() {
		var u remnawave.PanelUser
		if err = rows.Scan(&u.ID, &u.UUID, &u.TelegramID); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
