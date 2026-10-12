package database

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

func TestPanelPlanObservationsUseLatestPaidTermPerSubscription(t *testing.T) {
	dsn := os.Getenv("PANEL_SYNC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PANEL_SYNC_TEST_DATABASE_URL to an isolated database")
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
	schema := fmt.Sprintf("panel_plan_test_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	_, err = pool.Exec(ctx, `CREATE TABLE customer_subscription(id BIGINT PRIMARY KEY,panel_user_id BIGINT,panel_user_uuid UUID,subscription_link TEXT,customer_id BIGINT DEFAULT 7);
	 CREATE TABLE purchase(id BIGINT PRIMARY KEY,subscription_id BIGINT,status TEXT,paid_at TIMESTAMPTZ,created_at TIMESTAMPTZ,month INT,days INT,plan_id TEXT);
	 INSERT INTO customer_subscription(id,panel_user_id,panel_user_uuid,subscription_link) VALUES(1,42,'11111111-1111-1111-1111-111111111111','https://example.com/a'),(2,NULL,'22222222-2222-2222-2222-222222222222',NULL),(3,NULL,NULL,NULL);
	 INSERT INTO purchase VALUES
	 (1,1,'paid','2026-09-01','2026-09-01',1,0,'old'),
	 (2,1,'paid','2026-10-01','2026-10-01',3,0,'quarter'),
	 (3,1,'cancel','2026-10-02','2026-10-02',12,0,'cancelled'),
	 (4,1,'paid','2026-10-03','2026-10-03',0,0,'traffic_topup');`)
	if err != nil {
		t.Fatal(err)
	}
	items, err := NewCustomerRepository(pool).PanelPlanObservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("unlinked subscriptions included: %+v", items)
	}
	for _, item := range items {
		if item.CustomerID != 7 || item.SubscriptionID <= 0 {
			t.Fatalf("binding identity lost: %+v", item)
		}
		if item.PanelID != nil {
			if *item.PanelID != 42 || item.Months != 3 || item.PlanID != "quarter" || item.Link != "https://example.com/a" || item.PanelUUID == nil {
				t.Fatalf("wrong paid term or identity: %+v", item)
			}
		} else if item.Months != 0 || item.Days != 0 || item.PlanID != "" || item.PanelUUID == nil {
			t.Fatalf("manual panel account lost: %+v", item)
		}
	}
}
