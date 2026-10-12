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

func TestPanelOverrideRebasesFuturePurchasesWithoutLosingPaymentHistory(t *testing.T) {
	dsn := os.Getenv("PANEL_SYNC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PANEL_SYNC_TEST_DATABASE_URL to an isolated PostgreSQL database")
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
	schema := fmt.Sprintf("panel_entitlement_test_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	_, err = pool.Exec(ctx, `CREATE TABLE purchase(id BIGINT PRIMARY KEY, subscription_id BIGINT, status TEXT, amount BIGINT,
		extra_devices INT NOT NULL DEFAULT 0, extra_traffic_bytes BIGINT NOT NULL DEFAULT 0, extra_traffic_unlimited BOOLEAN NOT NULL DEFAULT FALSE);
		CREATE TABLE subscription_device_limit(subscription_id BIGINT PRIMARY KEY, base_limit INT, updated_at TIMESTAMPTZ DEFAULT NOW());
		CREATE TABLE subscription_traffic_limit(subscription_id BIGINT PRIMARY KEY, base_limit BIGINT, updated_at TIMESTAMPTZ DEFAULT NOW());
		CREATE TABLE subscription_device_grant(purchase_id BIGINT PRIMARY KEY, subscription_id BIGINT, devices INT, expires_at TIMESTAMPTZ,
		 expired_applied_at TIMESTAMPTZ, purchase_notified_at TIMESTAMPTZ, reminder_notified_at TIMESTAMPTZ, expiry_notified_at TIMESTAMPTZ);
		INSERT INTO subscription_device_limit VALUES(1,5,NOW()); INSERT INTO subscription_traffic_limit VALUES(1,100,NOW());
		INSERT INTO purchase VALUES(1,1,'paid',100,2,50,FALSE);
		INSERT INTO purchase VALUES(2,1,'new',50,1,25,FALSE);
		INSERT INTO subscription_device_grant(purchase_id,subscription_id,devices,expires_at) VALUES(1,1,2,NOW()+INTERVAL '1 day');`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000058_panel_entitlement_overrides.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	repo := NewPurchaseRepository(pool)
	now := time.Now().UTC()
	pending := &Purchase{ID: 2, PurchaseKind: PurchaseKindExtraDevices, ExtraDevices: 1, ExtraTrafficBytes: 25}
	assertLimits := func(deviceBase, deviceExtra int, trafficBase, trafficExtra int64, unlimited bool) {
		t.Helper()
		db, de, err := repo.DeviceLimitState(ctx, 1, 0, now)
		if err != nil || db != deviceBase || de != deviceExtra {
			t.Fatalf("device state (%d,%d,%v), want (%d,%d)", db, de, err, deviceBase, deviceExtra)
		}
		tb, te, un, err := repo.TrafficLimitState(ctx, 1, 0)
		if err != nil || tb != trafficBase || te != trafficExtra || un != unlimited {
			t.Fatalf("traffic state (%d,%d,%v,%v)", tb, te, un, err)
		}
	}
	for _, limits := range []struct {
		devices int
		traffic int64
	}{{7, 150}, {8, 175}} {
		if err := repo.RebasePanelEntitlements(ctx, 1, pending, limits.devices, limits.traffic, now); err != nil {
			t.Fatal(err)
		}
		assertLimits(5, 2, 100, 50, false)
	}
	// Changing only traffic must preserve active device grants.
	if err := repo.RebasePanelEntitlements(ctx, 1, pending, 7, 20, now); err != nil {
		t.Fatal(err)
	}
	assertLimits(5, 2, 20, 0, false)
	// A lower device limit is a new base, never an accidental unlimited value.
	if err := repo.RebasePanelEntitlements(ctx, 1, pending, 1, 20, now); err != nil {
		t.Fatal(err)
	}
	assertLimits(1, 0, 20, 0, false)
	grants, err := repo.DeviceGrants(ctx, 1)
	if err != nil || len(grants) != 0 {
		t.Fatalf("superseded grants still expire: %+v %v", grants, err)
	}
	// A subsequent pack applies exactly once from the manual base.
	if _, err := pool.Exec(ctx, `UPDATE purchase SET status='paid' WHERE id=2;
		INSERT INTO subscription_device_grant(purchase_id,subscription_id,devices,expires_at) VALUES(2,1,1,NOW()+INTERVAL '1 day')`); err != nil {
		t.Fatal(err)
	}
	if err := repo.RebasePanelEntitlements(ctx, 1, nil, 2, 45, now); err != nil {
		t.Fatal(err)
	}
	assertLimits(1, 1, 20, 25, false)
	// Both unlimited directions and a later finite override retain panel values.
	if err := repo.RebasePanelEntitlements(ctx, 1, nil, 0, 0, now); err != nil {
		t.Fatal(err)
	}
	assertLimits(0, 0, 0, 0, false)
	if _, err := pool.Exec(ctx, `INSERT INTO purchase(id,subscription_id,status,amount,extra_traffic_unlimited) VALUES(3,1,'paid',20,TRUE)`); err != nil {
		t.Fatal(err)
	}
	if err := repo.RebasePanelEntitlements(ctx, 1, nil, 3, 10, now); err != nil {
		t.Fatal(err)
	}
	assertLimits(3, 0, 10, 0, false)
	var count, amount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*), SUM(amount) FROM purchase WHERE status='paid'`).Scan(&count, &amount); err != nil || count != 3 || amount != 170 {
		t.Fatalf("payment history changed: %d %d %v", count, amount, err)
	}
	// First delivery must not interpret a coincidentally equal manual target
	// as proof that this new invoice was already applied.
	if _, err := pool.Exec(ctx, `INSERT INTO subscription_device_limit VALUES(2,5,NOW());
		INSERT INTO subscription_traffic_limit VALUES(2,100,NOW());
		INSERT INTO purchase(id,subscription_id,status,amount,extra_devices,extra_traffic_bytes) VALUES(4,2,'new',50,1,25)`); err != nil {
		t.Fatal(err)
	}
	if err := repo.RebasePanelEntitlements(ctx, 2, &Purchase{ID: 4, PurchaseKind: PurchaseKindExtraDevices, ExtraDevices: 1, ExtraTrafficBytes: 25}, 6, 125, now); err != nil {
		t.Fatal(err)
	}
	db, _, err := repo.DeviceLimitState(ctx, 2, 0, now)
	if err != nil || db != 6 {
		t.Fatalf("new invoice consumed manual device increase: %d %v", db, err)
	}
	tb, _, _, err := repo.TrafficLimitState(ctx, 2, 0)
	if err != nil || tb != 125 {
		t.Fatalf("new invoice consumed manual traffic increase: %d %v", tb, err)
	}
	// Verify this migration can also be rolled back without changing purchases.
	raw, err = os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000058_panel_entitlement_overrides.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
}
