package broadcast

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/jackc/pgx/v4/pgxpool"
	"link-bot/internal/database"
)

func TestDeliveryRechecksUnsubscribeAndOverrideWithoutResubscribing(t *testing.T) {
	dsn := os.Getenv("BROADCAST_AUDIENCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set BROADCAST_AUDIENCE_TEST_DATABASE_URL to an isolated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
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
	schema := fmt.Sprintf("broadcast_delivery_test_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	if _, err := pool.Exec(ctx, `CREATE TABLE customer(id BIGINT PRIMARY KEY,telegram_id BIGINT,is_blocked BOOLEAN,telegram_id_is_synthetic BOOLEAN);
		CREATE TABLE customer_broadcast_unsubscribe(customer_id BIGINT PRIMARY KEY REFERENCES customer(id));
		INSERT INTO customer VALUES(1,101,false,false),(2,102,true,false)`); err != nil {
		t.Fatal(err)
	}
	delivered := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer api.Close()
	b, err := bot.New("123:test", bot.WithSkipGetMe(), bot.WithServerURL(api.URL))
	if err != nil {
		t.Fatal(err)
	}
	r := database.NewCustomerRepository(pool)
	s := NewService(nil, r, nil, b)
	draft := database.BroadcastDraft{SourceKind: "html", SourceHTML: "Test broadcast"}
	if err := r.SetBroadcastSubscribed(ctx, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := s.copyTo(ctx, draft, 101, nil); !errors.Is(err, database.ErrBroadcastUnsubscribed) || delivered != 0 {
		t.Fatalf("ordinary send: delivered=%d err=%v", delivered, err)
	}
	draft.Audience.IgnoreUnsubscribe = true
	if err := s.copyTo(ctx, draft, 101, nil); err != nil || delivered != 1 {
		t.Fatalf("override send: delivered=%d err=%v", delivered, err)
	}
	if subscribed, err := r.BroadcastSubscribed(ctx, 1); err != nil || subscribed {
		t.Fatalf("override resubscribed customer: %v %v", subscribed, err)
	}
	if err := s.copyTo(ctx, draft, 102, nil); !errors.Is(err, database.ErrBroadcastUnsubscribed) || delivered != 1 {
		t.Fatalf("override bypassed block: %v", err)
	}
	draft.Audience.IgnoreUnsubscribe = false
	if err := r.SetBroadcastSubscribed(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.copyTo(ctx, draft, 101, nil); err != nil || delivered != 2 {
		t.Fatalf("resubscribe delivery: %v", err)
	}
	// The captured campaign is unchanged; a new opt-out still prevents delivery.
	if err := r.SetBroadcastSubscribed(ctx, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := s.copyTo(ctx, draft, 101, nil); !errors.Is(err, database.ErrBroadcastUnsubscribed) || delivered != 2 {
		t.Fatalf("mid-campaign opt-out ignored: %v", err)
	}
	if _, err := pool.Exec(ctx, "DROP TABLE customer_broadcast_unsubscribe"); err != nil {
		t.Fatal(err)
	}
	if err := s.copyTo(ctx, draft, 101, nil); err == nil || delivered != 2 {
		t.Fatal("preference lookup failure sent a broadcast")
	}
}
