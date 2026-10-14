package devicequota

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
	"link-bot/internal/remnawave"
)

func TestPostgresBindingCheckpointsAndPlanOwnership(t *testing.T) {
	dsn := os.Getenv("DEVICE_QUOTA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	cfg.ConnConfig.PreferSimpleProtocol = true
	pool, err := pgxpool.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema := fmt.Sprintf("device_quota_test_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	_, err = pool.Exec(ctx, `CREATE TABLE customer(id BIGINT PRIMARY KEY,telegram_id BIGINT);
 CREATE TABLE customer_subscription(id BIGINT PRIMARY KEY,customer_id BIGINT,panel_user_id BIGINT,panel_user_uuid UUID,is_primary BOOLEAN);
 CREATE TABLE purchase(id BIGINT,customer_id BIGINT,subscription_id BIGINT,plan_id TEXT,status TEXT,month INT,days INT,paid_at TIMESTAMPTZ,created_at TIMESTAMPTZ);
 INSERT INTO customer VALUES(1,101),(2,202);
 INSERT INTO customer_subscription VALUES(5,1,11,NULL,true),(6,2,22,NULL,true);
 INSERT INTO purchase VALUES(1,1,5,'old','paid',12,0,NOW()-INTERVAL '1 day',NOW()),(2,1,5,'new','paid',1,0,NOW(),NOW()),(3,2,6,'other','paid',12,0,NOW(),NOW());`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/migrations/000060_device_traffic_profiles.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	store := NewPostgresStore(pool)
	tg := int64(101)
	parent := &remnawave.PanelUser{ID: 11, UUID: uuid.New(), TelegramID: &tg}
	// A lower-ID unbound primary must not steal the secondary's purchased plan
	// merely because both panel profiles belong to the same Telegram account.
	if _, err = pool.Exec(ctx, `UPDATE customer_subscription SET is_primary=false WHERE id=5; INSERT INTO customer_subscription VALUES(4,1,NULL,NULL,true)`); err != nil {
		t.Fatal(err)
	}
	subID, plan, err := store.Plan(ctx, parent)
	if err != nil || subID != 5 || plan != "new" {
		t.Fatalf("wrong purchased plan %d %q %v", subID, plan, err)
	}
	candidates, err := store.Candidates(ctx, []string{"new"})
	if err != nil || len(candidates) != 1 || candidates[0].ID != 11 {
		t.Fatalf("wrong discovery candidates %+v %v", candidates, err)
	}
	token := uuid.New()
	b := &Binding{ParentKey: parentKey(parent), ParentID: parent.ID, ParentUUID: parent.UUID, SubscriptionID: &subID, Token: token, Squads: []string{uuid.NewString()}}
	if err = store.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	candidates, err = store.Candidates(ctx, []string{"new"})
	if err != nil || len(candidates) != 0 {
		t.Fatalf("already managed subscription discovered again: %+v %v", candidates, err)
	}
	b, err = store.FindToken(ctx, token)
	if err != nil || b == nil {
		t.Fatalf("cannot load token: %v", err)
	}
	err = store.Locked(ctx, b.ID, func(b *Binding, save func() error) error {
		b.Isolated = true
		b.Slots = []*Slot{{Number: 1, Device: remnawave.UserDevice{Hwid: "first-device-123"}, ChildID: 901, ChildUUID: uuid.New(), Registered: true}}
		return save()
	})
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewPostgresStore(pool)
	restored, err := restarted.FindParent(ctx, parentKey(parent))
	if err != nil || restored == nil || !restored.Isolated || len(restored.Slots) != 1 || restored.Slots[0].ChildID != 901 || restored.Token != token {
		t.Fatalf("checkpoint not durable: %+v %v", restored, err)
	}
	// A concurrent creator must not replace the first bearer URL or routing.
	b.Token = uuid.New()
	b.Squads = []string{uuid.NewString()}
	if err = store.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	stable, _ := store.FindToken(ctx, token)
	if stable == nil || stable.Squads[0] != restored.Squads[0] {
		t.Fatal("idempotent creation replaced routing/token")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM customer_subscription WHERE id=5`); err != nil {
		t.Fatal(err)
	}
	orphan, _ := store.FindToken(ctx, token)
	if orphan == nil || orphan.SubscriptionID != nil || len(orphan.Slots) != 1 {
		t.Fatal("deleting owner lost child cleanup identities")
	}
	if err = store.Locked(ctx, b.ID, func(b *Binding, save func() error) error { return nil }); err != nil {
		t.Fatalf("session lock was not released: %v", err)
	}
}
