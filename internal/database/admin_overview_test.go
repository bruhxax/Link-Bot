package database

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	"os"
	"testing"
	"time"
)

func TestAdminOverviewCountsAllUsersAndKeepsCurrenciesSeparate(t *testing.T) {
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
	schema := fmt.Sprintf("admin_overview_test_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	_, err = pool.Exec(ctx, `CREATE TABLE customer(id BIGINT PRIMARY KEY,is_blocked BOOLEAN DEFAULT FALSE,created_at TIMESTAMPTZ DEFAULT '2026-09-01T00:00:00Z',expire_at TIMESTAMPTZ,subscription_link TEXT);
	 CREATE TABLE customer_subscription(id BIGINT PRIMARY KEY,customer_id BIGINT,expire_at TIMESTAMPTZ,subscription_link TEXT);
	 CREATE TABLE purchase(id BIGINT PRIMARY KEY,subscription_id BIGINT,status TEXT,amount FLOAT8,currency TEXT,invoice_type TEXT,paid_at TIMESTAMPTZ,created_at TIMESTAMPTZ,month INT DEFAULT 1,days INT DEFAULT 0,plan_id TEXT);
	 INSERT INTO customer(id,expire_at,subscription_link) VALUES(1,'2027-01-01','https://example.com/1'),(2,'2026-09-01','https://example.com/old'),(3,NULL,NULL),(4,'2027-01-01','https://example.com/blocked'),(5,'2026-09-01','https://example.com/5'),(6,NULL,NULL),(7,NULL,'https://example.com/unknown');
	 UPDATE customer SET is_blocked=TRUE WHERE id=4;
	 UPDATE customer SET created_at='2026-10-11T22:00:00Z' WHERE id=1;
	 UPDATE customer SET created_at='2026-10-11T20:00:00Z' WHERE id=2;
	 INSERT INTO customer_subscription VALUES(1,1,'2027-01-01','https://example.com/1'),(2,2,'2027-01-01','https://example.com/2'),(4,4,'2027-01-01','https://example.com/blocked'),(6,6,'2026-09-01','https://example.com/6');
	 INSERT INTO purchase(id,subscription_id,status,amount,currency,invoice_type,paid_at,created_at,plan_id) VALUES
	 (1,1,'paid',100,'RUB','yookasa','2026-10-11T22:00:00Z','2026-10-11T22:00:00Z','premium'),
	 (2,NULL,'paid',200,'RUB','yookasa','2026-10-11T20:00:00Z','2026-10-11T20:00:00Z','premium'),
	 (3,NULL,'paid',500,'RUB','yookasa','2026-09-15T00:00:00Z','2026-09-15T00:00:00Z','premium'),
	 (4,NULL,'cancel',300,'RUB','yookasa','2026-10-11T22:00:00Z','2026-10-11T22:00:00Z',NULL),
	 (5,NULL,'paid',50,'STARS','telegram_stars','2026-10-11T22:00:00Z','2026-10-11T22:00:00Z',NULL),
	 (6,NULL,'paid',700,'RUB','balance','2026-10-11T22:00:00Z','2026-10-11T22:00:00Z',NULL),
	 (7,NULL,'paid',0,'RUB','free','2026-10-11T22:00:00Z','2026-10-11T22:00:00Z',NULL),
	 (8,NULL,'paid',40,'USD','crypto','2026-10-11T22:00:00Z','2026-10-11T22:00:00Z',NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewCustomerRepository(pool)
	now := time.Date(2026, 10, 11, 22, 15, 0, 0, time.UTC)
	data, err := repo.LoadAdminOverview(ctx, 30, now, true, true)
	if err != nil {
		t.Fatal(err)
	}
	u := data.Users
	if u.Total != 7 || u.Active != 2 || u.Expired != 2 || u.Without != 2 || u.Blocked != 1 || u.Today != 1 || u.Week != 2 {
		t.Fatalf("wrong full-population counters: %+v", u)
	}
	if len(u.Daily) != 30 || u.Daily[29].Date != "2026-10-12" || u.Daily[29].Value != 1 || u.Daily[28].Value != 1 {
		t.Fatalf("daily Moscow range wrong: %+v", u.Daily)
	}
	r := data.Revenue
	if r.TodayRub != 100 || r.WeekRub != 300 || r.MonthRub != 300 || r.AllRub != 800 || r.AllStars != 50 || r.AverageRub != 800.0/3 {
		t.Fatalf("currencies/refunds/balance mixed: %+v", r)
	}
	if len(r.Daily) != 30 || r.Daily[29].Value != 100 || r.Daily[28].Value != 200 {
		t.Fatalf("daily revenue wrong: %+v", r.Daily)
	}
	if len(data.Subscriptions) != 2 {
		t.Fatalf("active subscription distribution wrong: %+v", data.Subscriptions)
	}
	data, err = repo.LoadAdminOverview(ctx, 7, now, true, false)
	if err != nil || data.Users == nil || data.Revenue != nil {
		t.Fatalf("users-only role received finance: %+v %v", data, err)
	}
	// Without either permission, no aggregate query needs to read the database.
	if _, err = pool.Exec(ctx, "DROP TABLE purchase"); err != nil {
		t.Fatal(err)
	}
	data, err = repo.LoadAdminOverview(ctx, 7, now, false, false)
	if err != nil || data.Users != nil || data.Revenue != nil {
		t.Fatalf("unavailable permissions queried data: %+v %v", data, err)
	}
}
