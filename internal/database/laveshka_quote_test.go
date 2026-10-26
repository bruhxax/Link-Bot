package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

func TestLaveshkaQuotePersistsImmutableAmountsAndRollsBack(t *testing.T) {
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
	schema := fmt.Sprintf("laveshka_%d", time.Now().UnixNano())
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE SCHEMA " + schema + ";SET search_path TO " + schema)
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE; SET search_path TO public")
	exec("CREATE TABLE purchase(id BIGINT PRIMARY KEY);INSERT INTO purchase VALUES(42),(43)")
	apply := func(suffix string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000069_laveshka_quote."+suffix+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		exec(string(raw))
	}
	apply("up")
	q := LaveshkaQuote{PurchaseID: 42, InvoiceID: "inv_fixture", MerchantID: 12, RubMinor: 8900, USDMinor: 89}
	repo := NewPaymentIntegrationRepository(pool)
	for i := 0; i < 2; i++ {
		if err := repo.SaveLaveshkaQuote(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	// New repository models a restart: the quote belongs to the database.
	repo = NewPaymentIntegrationRepository(pool)
	saved, err := repo.FindLaveshkaQuote(ctx, q.InvoiceID)
	if err != nil || saved == nil || *saved != q {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}
	for _, change := range []func(*LaveshkaQuote){
		func(q *LaveshkaQuote) { q.USDMinor++ }, func(q *LaveshkaQuote) { q.RubMinor++ }, func(q *LaveshkaQuote) { q.InvoiceID = "inv_other" }, func(q *LaveshkaQuote) { q.MerchantID++ }, func(q *LaveshkaQuote) { q.PurchaseID = 43 },
	} {
		changed := q
		change(&changed)
		if err := repo.SaveLaveshkaQuote(ctx, changed); err == nil {
			t.Fatal("overwrote original quote or reused invoice")
		}
	}
	saved, err = repo.FindLaveshkaQuote(ctx, q.InvoiceID)
	if err != nil || saved == nil || *saved != q {
		t.Fatal("original quote changed")
	}
	if missing, err := repo.FindLaveshkaQuote(ctx, "inv_missing"); err != nil || missing != nil {
		t.Fatal("missing quote not nil")
	}
	exec("DELETE FROM purchase WHERE id=42")
	if missing, err := repo.FindLaveshkaQuote(ctx, q.InvoiceID); err != nil || missing != nil {
		t.Fatal("quote did not cascade with purchase")
	}
	apply("down")
	apply("up")
}
