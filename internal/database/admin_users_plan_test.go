package database

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	"os"
	"testing"
	"time"
)

func TestAdminUserListShowsPaidPlanOfSelectedSubscription(t *testing.T) {
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
	schema := fmt.Sprintf("users_plan_test_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	_, err = pool.Exec(ctx, `CREATE TABLE customer(id BIGINT,telegram_id BIGINT,telegram_username TEXT,created_at TIMESTAMPTZ,trial_used BOOLEAN,is_blocked BOOLEAN,expire_at TIMESTAMPTZ,subscription_link TEXT);
 CREATE TABLE administrator(customer_id BIGINT,role_name TEXT,color TEXT);
 CREATE TABLE customer_subscription(id BIGINT,customer_id BIGINT,display_name TEXT,expire_at TIMESTAMPTZ,subscription_link TEXT,panel_user_id BIGINT,panel_user_uuid UUID,is_primary BOOLEAN,position INT);
 CREATE TABLE customer_subscription_selection(customer_id BIGINT,subscription_id BIGINT);
 CREATE TABLE purchase(id BIGINT,subscription_id BIGINT,status TEXT,plan_id TEXT,month INT,days INT,paid_at TIMESTAMPTZ,created_at TIMESTAMPTZ);
 INSERT INTO customer VALUES(1,111,'first','2026-10-12',FALSE,FALSE,NULL,NULL),(2,222,'second','2026-10-11',FALSE,FALSE,NULL,NULL);
 INSERT INTO customer_subscription VALUES(10,1,'Личная','2027-01-01','link1',10,NULL,TRUE,0),(11,1,'Работа','2027-01-01','link2',11,NULL,FALSE,1);
 INSERT INTO customer_subscription_selection VALUES(1,11);
 INSERT INTO purchase VALUES(1,10,'paid','primary',1,0,'2026-10-01','2026-10-01'),(2,11,'paid','selected',3,0,'2026-10-02','2026-10-02'),(3,11,'cancel','cancelled',12,0,'2026-10-03','2026-10-03'),(4,11,'paid','traffic',0,0,'2026-10-04','2026-10-04');`)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewCustomerRepository(pool)
	rows, total, err := repo.SearchAdminUsers(ctx, "", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(rows) != 1 || rows[0].PlanID != "selected" || rows[0].PlanMonths != 3 || rows[0].SubscriptionName != "Работа" || rows[0].SubscriptionIsPrimary {
		t.Fatalf("wrong selected plan/page: %d %+v", total, rows)
	}
	rows, total, err = repo.SearchAdminUsers(ctx, "", 1, 1)
	if err != nil || total != 2 || len(rows) != 1 || rows[0].CustomerID != 2 || rows[0].PlanID != "" {
		t.Fatalf("pagination or absent plan failed: %+v %v", rows, err)
	}
	rows, total, err = repo.SearchAdminUsers(ctx, "работа", 30, 0)
	if err != nil || total != 1 || len(rows) != 1 || rows[0].PlanID != "selected" {
		t.Fatalf("subscription search failed: %+v %v", rows, err)
	}
}
