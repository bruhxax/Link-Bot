package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/database/stub"
)

func rollbackMigrationSource(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "db", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	return "file://" + filepath.ToSlash(path)
}

// Startup must work both before the withdrawn release and after its migration
// has already been recorded. Deleting migration 56 breaks the latter case.
func TestBroadcastRollbackMigrationHistory(t *testing.T) {
	for _, version := range []int{55, 56} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			driver := &stub.Stub{CurrentVersion: version}
			m, err := migrate.NewWithDatabaseInstance(rollbackMigrationSource(t), "stub", driver)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			if err := m.Up(); err != nil {
				t.Fatalf("startup from version %d: %v", version, err)
			}
			if driver.CurrentVersion != 62 || driver.IsDirty {
				t.Fatalf("unexpected final state: version=%d dirty=%v", driver.CurrentVersion, driver.IsDirty)
			}
			if err := m.Up(); err != migrate.ErrNoChange {
				t.Fatalf("restart should be a no-op: %v", err)
			}
		})
	}
}

func TestBroadcastRollbackMigrationDatabase(t *testing.T) {
	dsn := os.Getenv("BROADCAST_AUDIENCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set BROADCAST_AUDIENCE_TEST_DATABASE_URL to an isolated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	admin.SetMaxOpenConns(1)
	for _, version := range []int{55, 56} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			db, err := admin.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			schema := fmt.Sprintf("rollback_test_%d_%d", time.Now().UnixNano(), version)
			if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `CREATE TABLE customer_subscription(id BIGINT PRIMARY KEY);
				CREATE TABLE app_runtime_settings(id INT,config JSONB);
				CREATE TABLE purchase(customer_id BIGINT, status TEXT, purchase_kind TEXT, amount BIGINT, is_free_plan BOOLEAN);
				INSERT INTO purchase VALUES(1,'paid','subscription',100,false);
				ALTER TABLE purchase ADD COLUMN month INT DEFAULT 0, ADD COLUMN days INT DEFAULT 0, ADD COLUMN plan_id TEXT`); err != nil {
				t.Fatal(err)
			}
			if version == 56 {
				raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000056_broadcast_paid_lookup.up.sql"))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, string(raw)); err != nil {
					t.Fatal(err)
				}
			}
			driver, err := postgres.WithConnection(ctx, db, &postgres.Config{})
			if err != nil {
				t.Fatal(err)
			}
			m, err := migrate.NewWithDatabaseInstance(rollbackMigrationSource(t), "postgres", driver)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			defer db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE; SET search_path TO public")
			if err := m.Force(version); err != nil { // Fixture state only.
				t.Fatal(err)
			}
			if err := m.Up(); err != nil {
				t.Fatal(err)
			}
			v, dirty, err := m.Version()
			if err != nil || v != 62 || dirty {
				t.Fatalf("version=%d dirty=%v err=%v", v, dirty, err)
			}
			var indexGone bool
			var amount int
			if err := db.QueryRowContext(ctx, "SELECT to_regclass('purchase_broadcast_paid_customer_idx') IS NULL").Scan(&indexGone); err != nil || !indexGone {
				t.Fatalf("index rollback: gone=%v err=%v", indexGone, err)
			}
			if err := db.QueryRowContext(ctx, "SELECT amount FROM purchase WHERE customer_id=1").Scan(&amount); err != nil || amount != 100 {
				t.Fatalf("purchase data changed: amount=%d err=%v", amount, err)
			}
			if err := m.Up(); err != migrate.ErrNoChange {
				t.Fatalf("restart should be a no-op: %v", err)
			}
		})
	}
}
