package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

func TestUserActivityPaymentTransitionsAndPagination(t *testing.T) {
	dsn := os.Getenv("BROADCAST_AUDIENCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set BROADCAST_AUDIENCE_TEST_DATABASE_URL to an isolated database")
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
	schema := fmt.Sprintf("user_activity_%d", time.Now().UnixNano())
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE SCHEMA " + schema + "; SET search_path TO " + schema)
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE; SET search_path TO public")
	exec(`CREATE TABLE customer(id BIGINT PRIMARY KEY);
        INSERT INTO customer VALUES(1),(2);
        CREATE TABLE purchase(id BIGSERIAL PRIMARY KEY,customer_id BIGINT,amount NUMERIC,currency TEXT,invoice_type TEXT,plan_id TEXT,purchase_kind TEXT,status TEXT,is_auto_payment BOOLEAN DEFAULT false,external_payment_url TEXT);
        INSERT INTO purchase(customer_id,amount,currency,invoice_type,plan_id,purchase_kind,status) VALUES(1,300,'RUB','smerch','one','subscription','pending');
        CREATE FUNCTION miniapp_notify_change() RETURNS TRIGGER AS $$ BEGIN PERFORM pg_notify('miniapp_changes',TG_TABLE_NAME); RETURN NULL; END; $$ LANGUAGE plpgsql;`)
	apply := func(suffix string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000068_user_activity."+suffix+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		exec(string(raw))
	}
	apply("up")
	repo := NewCustomerRepository(pool)
	list := func(customer, before int64, limit int) ([]UserActivity, bool) {
		t.Helper()
		items, more, err := repo.ListUserActivity(ctx, customer, before, limit)
		if err != nil {
			t.Fatal(err)
		}
		return items, more
	}
	items, _ := list(1, 0, 30)
	if len(items) != 0 {
		t.Fatal("migration fabricated old events")
	}
	exec(`UPDATE purchase SET status='paid' WHERE id=1`)
	exec(`UPDATE purchase SET status='paid',external_payment_url='https://secret.example/token' WHERE id=1`)
	items, _ = list(1, 0, 30)
	if len(items) != 1 || items[0].Action != "payment.paid" || items[0].Status != "success" {
		t.Fatalf("payment event %+v", items)
	}
	paidID := items[0].ID
	exec(`INSERT INTO purchase(customer_id,amount,currency,invoice_type,plan_id,purchase_kind,status,is_auto_payment,external_payment_url) VALUES(1,10,'USD','cryptopay','two','gift','new',true,'https://secret.example/token')`)
	exec(`UPDATE purchase SET status='pending' WHERE id=2;UPDATE purchase SET status='cancel' WHERE id=2`)
	items, _ = list(1, 0, 30)
	if len(items) != 4 || items[0].Action != "payment.cancel" || items[1].Status != "pending" {
		t.Fatalf("transitions %+v", items)
	}
	for _, item := range items {
		for _, detail := range item.Details {
			if strings.Contains(detail.Value, "secret") || strings.Contains(detail.Value, "token") {
				t.Fatal("credential copied into journal")
			}
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE purchase SET status='paid' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	items, _ = list(1, 0, 30)
	if len(items) != 4 {
		t.Fatal("rolled back payment event persisted")
	}
	entry := UserActivity{CustomerID: 2, Source: "miniapp", Action: "purchase", Title: "Запросил оплату", Status: "failed", Details: []UserActivityDetail{{Label: "Причина", Value: "payment_failed"}}}
	if err := repo.RecordUserActivity(ctx, entry); err != nil {
		t.Fatal(err)
	}
	page, more := list(1, 0, 2)
	if len(page) != 2 || !more {
		t.Fatalf("first page %+v %v", page, more)
	}
	exec(`UPDATE purchase SET status='paid' WHERE id=2`)
	older, more := list(1, page[1].ID, 2)
	if len(older) != 2 || more || older[1].ID != paidID {
		t.Fatalf("unstable cursor %+v %v", older, more)
	}
	other, _ := list(2, 0, 30)
	if len(other) != 1 || other[0].CustomerID != 2 {
		t.Fatal("mixed customer histories")
	}
	exec(`DELETE FROM customer WHERE id=2`)
	other, _ = list(2, 0, 30)
	if len(other) != 0 {
		t.Fatal("history survived customer removal")
	}
	apply("down")
	exec(`UPDATE purchase SET status='pending' WHERE id=2`)
	var gone bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('user_activity') IS NULL`).Scan(&gone); err != nil || !gone {
		t.Fatalf("rollback %v %v", gone, err)
	}
}
