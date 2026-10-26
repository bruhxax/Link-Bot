package broadcast

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/jackc/pgx/v4/pgxpool"
	"link-bot/internal/database"
)

func TestBroadcastDeletionRetryPolicy(t *testing.T) {
	now := time.Now().UTC()
	item := database.BroadcastDeletion{SentAt: now.Add(-12 * time.Hour), Attempts: 1}
	for _, tc := range []struct {
		name   string
		err    error
		status string
		delay  time.Duration
	}{
		{"success", nil, "deleted", 0},
		{"already deleted", fmt.Errorf("%w: message to delete not found", bot.ErrorBadRequest), "deleted", 0},
		{"forbidden", bot.ErrorForbidden, "failed", 0},
		{"permanent request error", bot.ErrorBadRequest, "failed", 0},
		{"network retry", errors.New("connection reset"), "pending", time.Minute},
		{"rate limit", &bot.TooManyRequestsError{RetryAfter: 120}, "pending", 2 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, next := deletionResult(tc.err, item, now)
			if status != tc.status || next.Sub(now) != tc.delay {
				t.Fatalf("status=%s delay=%v", status, next.Sub(now))
			}
		})
	}
	item.Attempts = 20
	status, _, next := deletionResult(errors.New("offline"), item, now)
	if status != "pending" || next.Sub(now) != 32*time.Minute {
		t.Fatal("retry backoff is not bounded")
	}
	item.SentAt = now.Add(-48*time.Hour + 90*time.Second)
	status, _, next = deletionResult(errors.New("offline"), item, now)
	if status != "pending" || !next.Equal(item.SentAt.Add(48*time.Hour-time.Minute)) {
		t.Fatal("retry misses Telegram deadline")
	}
	status, _, _ = deletionResult(&bot.TooManyRequestsError{RetryAfter: 120}, item, now)
	if status != "failed" {
		t.Fatal("must not retry outside Telegram window")
	}
	for _, minutes := range []int{-1, 2821} {
		if database.ValidateBroadcastExpiration(minutes) == nil {
			t.Fatal("invalid duration accepted")
		}
	}
	for _, minutes := range []int{0, 1, 720, 2820} {
		if database.ValidateBroadcastExpiration(minutes) != nil {
			t.Fatal("valid duration rejected")
		}
	}
}

type failingDeletionStore struct{ calls int }

func (s *failingDeletionStore) ScheduleDeletion(context.Context, int64, int, time.Time, int) error {
	s.calls++
	return errors.New("database timeout")
}
func (*failingDeletionStore) ClaimDeletions(context.Context, int) ([]database.BroadcastDeletion, error) {
	return nil, nil
}
func (*failingDeletionStore) FinishDeletion(context.Context, database.BroadcastDeletion, string, string, time.Time) error {
	return nil
}

func TestTemporaryBroadcastRecordingFailureRemovesCopyWithoutResending(t *testing.T) {
	sent, deleted := 0, 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/deleteMessage") {
			deleted++
			w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		sent++
		w.Write([]byte(`{"ok":true,"result":{"message_id":701}}`))
	}))
	defer api.Close()
	b, err := bot.New("123:test", bot.WithSkipGetMe(), bot.WithServerURL(api.URL))
	if err != nil {
		t.Fatal(err)
	}
	store := &failingDeletionStore{}
	s := &Service{telegramBot: b, deletions: store}
	err = s.deliverBroadcast(context.Background(), database.BroadcastDraft{SourceKind: "html", SourceHTML: "Test", DeleteAfterMinutes: 720}, 101, nil)
	if !errors.Is(err, errDeletionRecording) || shouldRetry(err) || sent != 1 || deleted != 1 || store.calls != 3 {
		t.Fatalf("sent=%d deleted=%d record=%d err=%v", sent, deleted, store.calls, err)
	}
}

func TestTemporaryBroadcastPersistsDeliveryAndResumesDeletion(t *testing.T) {
	dsn := os.Getenv("BROADCAST_AUDIENCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set BROADCAST_AUDIENCE_TEST_DATABASE_URL to an isolated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
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
	schema := fmt.Sprintf("temporary_broadcast_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema+"; CREATE TABLE email_broadcast(id INT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE; SET search_path TO public")
	for _, name := range []string{"000017_broadcast_campaign", "000030_broadcast_html", "000055_broadcast_audience", "000066_temporary_broadcast", "000067_broadcast_auto_reply", "000070_broadcast_surveys"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", name+".up.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	repo := database.NewBroadcastRepository(pool)
	draft, err := repo.Get(ctx)
	if err != nil || draft.DeleteAfterMinutes != 0 {
		t.Fatalf("default: %+v %v", draft, err)
	}
	stale := draft.UpdatedAt
	draft, err = repo.SaveExpiration(ctx, 720, 101, draft.UpdatedAt)
	if err != nil || draft == nil || draft.DeleteAfterMinutes != 720 {
		t.Fatalf("save: %+v %v", draft, err)
	}
	if d, err := repo.SaveExpiration(ctx, 0, 101, stale); err != nil || d != nil {
		t.Fatal("stale save accepted")
	}
	if _, err = pool.Exec(ctx, "UPDATE bot_broadcast_draft SET status='running'"); err != nil {
		t.Fatal(err)
	}
	if d, err := repo.SaveExpiration(ctx, 60, 101, draft.UpdatedAt); err != nil || d != nil {
		t.Fatal("changed a running campaign")
	}
	if _, err = pool.Exec(ctx, "UPDATE bot_broadcast_draft SET status='draft'"); err != nil {
		t.Fatal(err)
	}

	messageID := 700
	deleted := map[int]bool{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = r.ParseMultipartForm(1 << 20)
		if strings.HasSuffix(r.URL.Path, "/deleteMessage") {
			var id int
			fmt.Sscan(r.FormValue("message_id"), &id)
			deleted[id] = true
			w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		messageID++
		fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d}}`, messageID)
	}))
	defer api.Close()
	b, err := bot.New("123:test", bot.WithSkipGetMe(), bot.WithServerURL(api.URL))
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(repo, nil, nil, b)
	chatID, sourceID := int64(999), 42
	for _, kind := range []string{"html", "photo"} {
		draft.SourceKind = kind
		draft.SourceHTML = "<b>Temporary</b>"
		draft.SourceChatID = &chatID
		draft.SourceMessageID = &sourceID
		if err = s.deliverBroadcast(ctx, *draft, 101, nil); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM broadcast_message_deletion WHERE message_id IN (701,702) AND chat_id=101 AND delete_at=sent_at+INTERVAL '12 hours'").Scan(&count); err != nil || count != 2 {
		t.Fatalf("recipient IDs or TTL lost: %d %v", count, err)
	}
	if jobs, err := repo.ClaimDeletions(ctx, 25); err != nil || len(jobs) != 0 {
		t.Fatal("deletion before expiry")
	}
	// A new draft must not delete or cancel outstanding jobs.
	if _, err = repo.Reset(ctx, 101); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE broadcast_message_deletion SET sent_at=NOW()-INTERVAL '13 hours',delete_at=NOW()-INTERVAL '1 hour',next_attempt_at=NOW()-INTERVAL '1 hour'"); err != nil {
		t.Fatal(err)
	}
	leased, err := repo.ClaimDeletions(ctx, 1)
	if err != nil || len(leased) != 1 {
		t.Fatalf("claim: %v %v", leased, err)
	}
	if again, err := repo.ClaimDeletions(ctx, 25); err != nil || len(again) != 1 {
		t.Fatal("leased message claimed twice")
	}
	// Simulate process death after a claim, then resume using a fresh service.
	if _, err = pool.Exec(ctx, "UPDATE broadcast_message_deletion SET lease_until=NOW()-INTERVAL '1 second'"); err != nil {
		t.Fatal(err)
	}
	restarted := NewService(database.NewBroadcastRepository(pool), nil, nil, b)
	if n, err := restarted.deleteExpiredMessages(ctx); err != nil || n != 2 {
		t.Fatalf("restart: %d %v", n, err)
	}
	if !deleted[701] || !deleted[702] || deleted[42] {
		t.Fatalf("wrong messages deleted: %v", deleted)
	}
	if n, err := restarted.deleteExpiredMessages(ctx); err != nil || n != 0 {
		t.Fatal("completed job repeated")
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM broadcast_message_deletion WHERE status='deleted'").Scan(&count); err != nil || count != 2 {
		t.Fatal("completion not persisted")
	}
	// Ordinary broadcasts do not create deletion tasks.
	draft.DeleteAfterMinutes = 0
	if err = s.deliverBroadcast(ctx, *draft, 101, nil); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM broadcast_message_deletion").Scan(&count); err != nil || count != 2 {
		t.Fatal("permanent broadcast scheduled")
	}
	// Database constraints also reject invalid settings; rollback removes only the feature.
	if _, err = pool.Exec(ctx, "UPDATE bot_broadcast_draft SET delete_after_minutes=2821"); err == nil {
		t.Fatal("invalid duration stored")
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000066_temporary_broadcast.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
}
