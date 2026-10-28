package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
)

func TestTorrentAlertPersistenceAndDeliveryLeases(t *testing.T) {
	dsn := os.Getenv("TORRENT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TORRENT_TEST_DATABASE_URL to an isolated database")
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
	schema := fmt.Sprintf("torrent_alert_test_%d", time.Now().UnixNano())
	exec := func(query string) {
		t.Helper()
		if _, err := pool.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE SCHEMA " + schema + "; SET search_path TO " + schema)
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE; SET search_path TO public")
	exec(`CREATE TABLE user_activity(id BIGSERIAL PRIMARY KEY,customer_id BIGINT,source TEXT CHECK(source IN ('miniapp','telegram','payment')),
		action TEXT,title TEXT,status TEXT,details JSONB,created_at TIMESTAMPTZ);
		CREATE TABLE customer_subscription(id BIGINT,customer_id BIGINT,display_name TEXT,panel_user_id BIGINT,panel_user_uuid UUID,is_primary BOOLEAN);
		INSERT INTO customer_subscription VALUES
		(11,1,'Основная',42,'22222222-2222-4222-8222-222222222222',true),
		(12,1,'Дубль привязки',42,'22222222-2222-4222-8222-222222222222',false),
		(21,2,'Другой UUID',42,'33333333-3333-4333-8333-333333333333',true),
		(31,3,'Старая привязка',42,NULL,true);`)
	exec(`CREATE TABLE customer(id BIGINT,telegram_id BIGINT,telegram_id_is_synthetic BOOLEAN,is_blocked BOOLEAN);
	INSERT INTO customer VALUES(1,101,false,false),(2,102,false,false),(3,103,true,false);
	CREATE FUNCTION miniapp_notify_change() RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END; $$ LANGUAGE plpgsql;`)
	apply := func(direction string) {
		t.Helper()
		raw, err := os.ReadFile("../../db/migrations/000071_torrent_alerts." + direction + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		exec(string(raw))
	}
	apply("up")
	repo := NewCustomerRepository(pool)
	entry := UserActivity{Title: "Обнаружен торрент-трафик", CreatedAt: time.Now().UTC(), Details: []UserActivityDetail{{Label: "IP", Value: "2001:db8::1"}, {Label: "UUID пользователя", Value: "22222222-2222-4222-8222-222222222222"}, {Label: "Пользователь панели", Value: "alice"}, {Label: "Нода", Value: "Frankfurt"}, {Label: "Результат", Value: "IP заблокирован на ноде"}, {Label: "Срок блокировки", Value: "3600 сек."}}}
	identity := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	for i := 0; i < 2; i++ {
		if err := repo.RecordTorrentAlert(ctx, "report", identity, 42, entry); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ customer, want int64 }{{1, 1}, {2, 0}, {3, 1}} {
		var count int64
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_activity WHERE customer_id=$1", tc.customer).Scan(&count); err != nil || count != tc.want {
			t.Fatalf("ownership/dedup customer %d: count=%d error=%v", tc.customer, count, err)
		}
	}
	var details string
	if err := pool.QueryRow(ctx, "SELECT details::text FROM user_activity WHERE customer_id=1").Scan(&details); err != nil || !strings.Contains(details, "Основная") {
		t.Fatalf("missing subscription context: %s, %v", details, err)
	}
	claim := func(token uuid.UUID, wantClaim, wantDelivered bool) {
		t.Helper()
		claimed, delivered, err := repo.ClaimTorrentAlert(ctx, "report", token)
		if err != nil || claimed != wantClaim || delivered != wantDelivered {
			t.Fatalf("claim=(%v,%v,%v), want=(%v,%v)", claimed, delivered, err, wantClaim, wantDelivered)
		}
	}
	first, next := uuid.New(), uuid.New()
	claim(first, true, false)
	claim(next, false, false)
	// A failed send releases the lease immediately for the panel's retry.
	if err := repo.FinishTorrentAlert(ctx, "report", first, false); err != nil {
		t.Fatal(err)
	}
	claim(next, true, false)
	// A process crash makes its lease recoverable. The old token cannot finish it.
	exec("UPDATE torrent_alert SET notification_lease_until=NOW()-INTERVAL '1 minute'")
	claim(first, true, false)
	if err := repo.FinishTorrentAlert(ctx, "report", next, true); err != nil {
		t.Fatal(err)
	}
	claim(next, false, false)
	if err := repo.FinishTorrentAlert(ctx, "report", first, true); err != nil {
		t.Fatal(err)
	}
	claim(next, false, true)
	// Recreating the repository (as after restart) keeps the delivered marker.
	restarted := NewCustomerRepository(pool)
	if claimed, delivered, err := restarted.ClaimTorrentAlert(ctx, "report", uuid.New()); err != nil || claimed || !delivered {
		t.Fatalf("delivery lost after restart: %v %v %v", claimed, delivered, err)
	}
	if err := repo.RecordTorrentAlert(ctx, "unlinked", uuid.New(), 999, entry); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_activity").Scan(&count); err != nil || count != 2 {
		t.Fatalf("unlinked user assigned to an account: %d, %v", count, err)
	}
	workspace := func(direction string) {
		t.Helper()
		raw, err := os.ReadFile("../../db/migrations/000072_torrent_workspace." + direction + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		exec(string(raw))
	}
	workspace("up")
	items, _, summary, err := repo.ListTorrentReports(ctx, "alice", "active", 0)
	if err != nil || len(items) != 1 || summary.Total != 1 || items[0].CustomerID != 1 {
		t.Fatalf("history backfill: %+v %+v %v", items, summary, err)
	}
	until := entry.CreatedAt.Add(time.Hour)
	report := TorrentReport{EventKey: "report", UserUUID: identity, PanelUserID: 42, Username: "alice", Node: "Frankfurt", IP: "2001:db8::1", Blocked: true, Duration: 3600, UnblockAt: &until, OccurredAt: entry.CreatedAt}
	for i := 0; i < 2; i++ {
		if err := repo.SaveTorrentReport(ctx, report); err != nil {
			t.Fatal(err)
		}
	}
	items, _, summary, err = repo.ListTorrentReports(ctx, "2001:db8", "all", 0)
	if err != nil || len(items) != 1 || summary.Total != 1 {
		t.Fatalf("duplicate after backfill: %+v %+v %v", items, summary, err)
	}
	ids, err := repo.TorrentRecipients(ctx, identity, 42)
	if err != nil || len(ids) != 1 || ids[0] != 101 {
		t.Fatalf("wrong recipients: %v %v", ids, err)
	}
	userToken := uuid.New()
	claimed, delivered, err := repo.ClaimTorrentUserAlert(ctx, "report", 101, userToken)
	if err != nil || !claimed || delivered {
		t.Fatal("cannot claim user delivery", err)
	}
	if err := repo.FinishTorrentUserAlert(ctx, "report", 101, userToken, true); err != nil {
		t.Fatal(err)
	}
	claimed, delivered, err = repo.ClaimTorrentUserAlert(ctx, "report", 101, uuid.New())
	if err != nil || claimed || !delivered {
		t.Fatal("lost user delivery marker", err)
	}
	report.EventKey = "unlinked"
	report.UserUUID = uuid.New()
	report.PanelUserID = 999
	report.Blocked = false
	report.UnblockAt = nil
	if err := repo.SaveTorrentReport(ctx, report); err != nil {
		t.Fatal(err)
	}
	items, _, summary, err = repo.ListTorrentReports(ctx, "", "observed", 0)
	if err != nil || len(items) != 1 || items[0].CustomerID != 0 || summary.Total != 2 {
		t.Fatalf("unlinked report missing: %+v %v", items, err)
	}
	workspace("down")
	apply("down")
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_activity").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback left torrent history: %d, %v", count, err)
	}
	apply("up")
}
