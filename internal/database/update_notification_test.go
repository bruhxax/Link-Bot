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

func TestUpdateNotificationLeaseAndPersistence(t *testing.T) {
	dsn := os.Getenv("BROADCAST_AUDIENCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated test database")
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
	schema := fmt.Sprintf("update_notification_test_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	if _, err = pool.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000065_update_notifications.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	a, b := NewUpdateNotificationRepository(pool), NewUpdateNotificationRepository(pool)
	claim := func(repo *UpdateNotificationRepository, revision string, want bool) {
		t.Helper()
		got, err := repo.Claim(ctx, revision)
		if err != nil || got != want {
			t.Fatalf("claim %s: %v, %v want %v", revision, got, err, want)
		}
	}
	finish := func(revision string, ok bool) {
		t.Helper()
		if err := a.Finish(ctx, revision, ok); err != nil {
			t.Fatal(err)
		}
	}
	claim(a, "old|head|2.7.4", true)
	claim(b, "old|head|2.7.4", false)
	claim(b, "old|next|2.7.4", false)
	finish("old|head|2.7.4", false)
	claim(b, "old|head|2.7.4", true)
	finish("old|head|2.7.4", true)
	claim(b, "old|head|2.7.4", false)
	claim(b, "old|next|2.7.4", true)
	if _, err = pool.Exec(ctx, "UPDATE app_update_notification_state SET lease_until=NOW()-INTERVAL '1 second'"); err != nil {
		t.Fatal(err)
	}
	claim(a, "old|next|2.7.4", true)
	finish("old|next|2.7.4", true)
	claim(b, "old|next|2.7.4", false)
	down, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000065_update_notifications.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
}
