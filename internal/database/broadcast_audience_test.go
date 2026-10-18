package database

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBroadcastAudienceNormalization(t *testing.T) {
	id := uuid.New().String()
	a := BroadcastAudience{Category: " ACTIVE ", InternalSquads: []string{id, id}, Tag: " PREMIUM "}
	if err := a.Normalize(); err != nil {
		t.Fatal(err)
	}
	if a.Category != "active" || len(a.InternalSquads) != 1 || a.Tag != "PREMIUM" || a.ExternalSquads == nil {
		t.Fatalf("incorrect normalization: %+v", a)
	}
	for _, a := range []BroadcastAudience{{Category: "unknown"}, {InternalSquads: []string{"not-a-uuid"}}, {Language: "invalid"}} {
		if err := a.Normalize(); err == nil {
			t.Fatal("invalid audience accepted")
		}
	}
}

// Uses only a disposable schema in the explicitly configured test database.
func TestBroadcastAudiencePersistenceAndDraftGuards(t *testing.T) {
	dsn := os.Getenv("BROADCAST_AUDIENCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set BROADCAST_AUDIENCE_TEST_DATABASE_URL to an isolated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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
	schema := fmt.Sprintf("audience_test_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	_, err = pool.Exec(ctx, "SET search_path TO "+schema)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `CREATE TABLE customer(id BIGINT PRIMARY KEY,telegram_id BIGINT,telegram_id_is_synthetic BOOLEAN,trial_used BOOLEAN,language TEXT,expire_at TIMESTAMPTZ,is_blocked BOOLEAN);
	CREATE TABLE customer_email_auth(customer_id BIGINT PRIMARY KEY,email TEXT);
	CREATE TABLE customer_subscription(customer_id BIGINT,panel_user_id BIGINT,panel_user_uuid UUID,expire_at TIMESTAMPTZ,position INTEGER);
	CREATE TABLE purchase(customer_id BIGINT,status TEXT,purchase_kind TEXT,amount BIGINT,is_free_plan BOOLEAN);
	CREATE TABLE email_auth_challenge(mode TEXT CONSTRAINT email_auth_challenge_mode_check CHECK(mode IN ('login','register')));
	INSERT INTO customer VALUES (1,101,false,true,'ru',NULL,false),(2,102,false,false,'en',NULL,true),(3,103,true,false,'en',NULL,false);
	INSERT INTO customer_email_auth VALUES(1,'one@example.com'),(3,'web@example.com');
	INSERT INTO customer_subscription VALUES(1,10,NULL,NOW()+INTERVAL '1 day',0),(1,11,NULL,NOW()+INTERVAL '2 days',1);
	INSERT INTO purchase VALUES(1,'paid','subscription',100,false),(3,'paid','devices',100,false);`)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000017_broadcast_campaign.up.sql", "000030_broadcast_html.up.sql", "000048_email_link_and_broadcast.up.sql", "000055_broadcast_audience.up.sql", "000063_broadcast_preferences.up.sql"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	c := NewCustomerRepository(pool)
	b := NewBroadcastRepository(pool)
	customers, err := c.BroadcastCustomers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(customers) != 2 || len(customers[0].Subscriptions) != 2 || !customers[0].Paid || customers[1].Paid || customers[0].Email != "one@example.com" {
		t.Fatalf("incorrect eligible accounts: %+v", customers)
	}
	t.Run("Telegram unsubscribe preserves email and filters Telegram broadcasts", func(t *testing.T) {
		if subscribed, err := c.BroadcastSubscribed(ctx, 1); err != nil || !subscribed {
			t.Fatalf("default=%v err=%v", subscribed, err)
		}
		for i := 0; i < 2; i++ {
			if err := c.SetBroadcastSubscribed(ctx, 1, false); err != nil {
				t.Fatal(err)
			}
		}
		if subscribed, err := NewCustomerRepository(pool).BroadcastSubscribed(ctx, 1); err != nil || subscribed {
			t.Fatalf("recreated repository lost opt-out: %v %v", subscribed, err)
		}
		for _, ignore := range []bool{false, true} {
			allowed, err := c.BroadcastTelegramAllowed(ctx, 101, ignore)
			if err != nil || allowed != ignore {
				t.Fatalf("Telegram ignore=%v allowed=%v err=%v", ignore, allowed, err)
			}
			allowed, err = c.BroadcastEmailAllowed(ctx, "one@example.com")
			if err != nil || !allowed {
				t.Fatalf("email ignore=%v allowed=%v err=%v", ignore, allowed, err)
			}
			ids, err := c.ListAllTelegramIDs(ctx, ignore)
			wantCount := 0
			if ignore {
				wantCount = 1
			}
			if err != nil || len(ids) != wantCount {
				t.Fatalf("legacy recipients ignore=%v: %v %v", ignore, ids, err)
			}
		}
		if allowed, err := c.BroadcastTelegramAllowed(ctx, 102, true); err != nil || allowed {
			t.Fatalf("override unblocked a blocked customer: %v %v", allowed, err)
		}
		if allowed, err := c.BroadcastTelegramAllowed(ctx, 103, true); err != nil || allowed {
			t.Fatalf("synthetic Telegram ID included: %v %v", allowed, err)
		}
		if allowed, err := c.BroadcastEmailAllowed(ctx, "missing@example.com"); err != nil || allowed {
			t.Fatalf("unknown email included: %v %v", allowed, err)
		}
		customers, err := c.BroadcastCustomers(ctx)
		if err != nil || !customers[0].Unsubscribed || customers[1].Unsubscribed {
			t.Fatalf("recipient preferences: %+v %v", customers, err)
		}
		// Sending with an override must not change the stored opt-out.
		if subscribed, err := c.BroadcastSubscribed(ctx, 1); err != nil || subscribed {
			t.Fatalf("override changed preference: %v %v", subscribed, err)
		}
		for i := 0; i < 2; i++ {
			if err := c.SetBroadcastSubscribed(ctx, 1, true); err != nil {
				t.Fatal(err)
			}
		}
		if allowed, err := c.BroadcastTelegramAllowed(ctx, 101, false); err != nil || !allowed {
			t.Fatalf("resubscribe failed: %v %v", allowed, err)
		}
		if allowed, err := c.BroadcastEmailAllowed(ctx, "one@example.com"); err != nil || !allowed {
			t.Fatalf("email resubscribe failed: %v %v", allowed, err)
		}
	})
	a := BroadcastAudience{Category: "active", Tag: "PREMIUM", IgnoreUnsubscribe: true}
	_ = a.Normalize()
	draft, err := b.SaveAudience(ctx, a, 101)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Audience.Category != "active" || draft.Audience.Tag != "PREMIUM" || !draft.Audience.IgnoreUnsubscribe {
		t.Fatal("Telegram audience lost")
	}
	if _, err = b.StartCapture(ctx, 101); err != nil {
		t.Fatal(err)
	}
	draft, err = b.SaveSource(ctx, 101, 42, "text", "Test", "Test", 101)
	if err != nil {
		t.Fatal(err)
	}
	stale := draft.UpdatedAt
	draft, err = b.SaveAudience(ctx, BroadcastAudience{Category: "none"}, 101)
	if err != nil {
		t.Fatal(err)
	}
	if started, err := b.BeginSend(ctx, 1, 101, stale); err == nil && started != nil {
		t.Fatal("Telegram accepted a stale draft")
	}
	if _, err = b.BeginSend(ctx, 1, 101, draft.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err = b.SaveAudience(ctx, a, 101); err == nil {
		t.Fatal("changed audience during Telegram send")
	}
	email, err := c.SaveEmailBroadcastDraft(ctx, "Subject", "Body", 101)
	if err != nil {
		t.Fatal(err)
	}
	stale = email.UpdatedAt
	email, err = c.SaveEmailAudience(ctx, a, 101)
	if err != nil {
		t.Fatal(err)
	}
	if email.Audience.Tag != "PREMIUM" || email.Audience.IgnoreUnsubscribe {
		t.Fatal("email audience lost")
	}
	if _, err = c.BeginEmailBroadcast(ctx, 1, 101, stale); err == nil {
		t.Fatal("email accepted a stale draft")
	}
	if _, err = c.BeginEmailBroadcast(ctx, 1, 101, email.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err = c.SaveEmailAudience(ctx, a, 101); err == nil {
		t.Fatal("changed audience during email send")
	}
	if err := b.Finish(ctx, BroadcastStatusFinished, 0, 0, "", 1); err != nil {
		t.Fatal(err)
	}
	if finished, err := b.Get(ctx); err != nil || finished.RecipientCount != 0 || finished.FailedCount != 0 {
		t.Fatalf("unsubscribed recipient counted as failure: %+v %v", finished, err)
	}
	if err := c.FinishEmailBroadcast(ctx, "finished", 0, 0, "", 1); err != nil {
		t.Fatal(err)
	}
	if finished, err := c.EmailBroadcastDraft(ctx); err != nil || finished.RecipientCount != 0 || finished.FailedCount != 0 {
		t.Fatalf("email skipped recipient counted as failure: %+v %v", finished, err)
	}
}
