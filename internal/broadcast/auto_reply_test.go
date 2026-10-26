package broadcast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v4/pgxpool"
	"link-bot/internal/database"
)

func TestAutoReplyCandidateExcludesCommandsBotsGroupsAndPayments(t *testing.T) {
	base := func() *models.Message {
		return &models.Message{ID: 901, From: &models.User{ID: 101}, Chat: models.Chat{ID: 101, Type: models.ChatTypePrivate}, Text: "Здравствуйте"}
	}
	if !IsAutoReplyCandidate(&models.Update{Message: base()}) {
		t.Fatal("ordinary message rejected")
	}
	for _, change := range []func(*models.Message){
		func(m *models.Message) { m.Text = "  /start" },
		func(m *models.Message) { m.Text = "/unknown" },
		func(m *models.Message) { m.Chat.Type = models.ChatTypeGroup },
		func(m *models.Message) { m.Chat.ID = 102 },
		func(m *models.Message) { m.From.IsBot = true },
		func(m *models.Message) { m.From = nil },
		func(m *models.Message) { m.SuccessfulPayment = &models.SuccessfulPayment{} },
		func(m *models.Message) { m.Text = "" },
	} {
		m := base()
		change(m)
		if IsAutoReplyCandidate(&models.Update{Message: m}) {
			t.Fatalf("ineligible message accepted: %+v", m)
		}
	}
	if IsAutoReplyCandidate(nil) || IsAutoReplyCandidate(&models.Update{}) {
		t.Fatal("non-message update accepted")
	}
	m := base()
	m.Text = ""
	m.Caption = "Вопрос"
	m.Document = &models.Document{}
	if !IsAutoReplyCandidate(&models.Update{Message: m}) {
		t.Fatal("user document question rejected")
	}
	m.Caption = "/start"
	if IsAutoReplyCandidate(&models.Update{Message: m}) {
		t.Fatal("caption command intercepted")
	}
}

func TestAutoReplySettingsValidateFormattingAndLimits(t *testing.T) {
	valid := database.BroadcastAutoReplySettings{Text: "<b>Рассылка</b> <tg-emoji emoji-id=\"5206222720416643915\">💬</tg-emoji>", WindowHours: 24}
	if err := normalizeAutoReplySettings(&valid); err != nil {
		t.Fatal(err)
	}
	if valid.SupportButtonText != "Поддержка" {
		t.Fatal("missing button default")
	}
	for _, change := range []func(*database.BroadcastAutoReplySettings){
		func(v *database.BroadcastAutoReplySettings) { v.WindowHours = 0 },
		func(v *database.BroadcastAutoReplySettings) { v.WindowHours = 169 },
		func(v *database.BroadcastAutoReplySettings) { v.Text = " " },
		func(v *database.BroadcastAutoReplySettings) { v.Text = strings.Repeat("x", 3501) },
		func(v *database.BroadcastAutoReplySettings) { v.Text = "<b>Незакрытый тег" },
		func(v *database.BroadcastAutoReplySettings) { v.SupportButtonText = strings.Repeat("x", 65) },
	} {
		value := valid
		change(&value)
		if err := normalizeAutoReplySettings(&value); !errors.Is(err, ErrAutoReplySettings) {
			t.Fatalf("invalid settings accepted: %+v err=%v", value, err)
		}
	}
}

func TestBroadcastAutoReplyPersistenceAndDelivery(t *testing.T) {
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
	schema := fmt.Sprintf("broadcast_reply_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE; SET search_path TO public")
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000067_broadcast_auto_reply.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	repo := database.NewBroadcastRepository(pool)
	settings, err := repo.AutoReplySettings(ctx)
	if err != nil || settings.Enabled || settings.WindowHours != 24 {
		t.Fatalf("defaults: %+v %v", settings, err)
	}
	var mu sync.Mutex
	requests, failNext := []string{}, false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = r.ParseMultipartForm(1 << 20)
		requests = append(requests, r.FormValue("text"))
		w.Header().Set("Content-Type", "application/json")
		if failNext {
			failNext = false
			w.WriteHeader(500)
			w.Write([]byte(`{"ok":false,"error_code":500,"description":"temporary error"}`))
			return
		}
		w.Write([]byte(`{"ok":true,"result":{"message_id":1500}}`))
	}))
	defer api.Close()
	b, err := bot.New("123:test", bot.WithSkipGetMe(), bot.WithServerURL(api.URL))
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(repo, nil, nil, b)
	stale := *settings
	settings.Enabled = true
	settings.Text = "<b>Это рассылка</b>. Напишите в поддержку."
	settings, err = s.SaveAutoReply(ctx, *settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveAutoReply(ctx, stale); !errors.Is(err, ErrDraftChanged) {
		t.Fatalf("stale config: %v", err)
	}
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(requests) }
	message := func(chatID int64) *models.Message {
		return &models.Message{ID: 1001, Date: int(time.Now().Unix()), From: &models.User{ID: chatID}, Chat: models.Chat{ID: chatID, Type: models.ChatTypePrivate}, Text: "Это вы мне написали?"}
	}
	for _, chat := range []int64{101, 102, 103, 104} {
		if err = repo.RecordBroadcastRecipient(ctx, chat, 700, time.Now().Add(-time.Hour).Truncate(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.replyToBroadcast(ctx, message(999)); err != nil || count() != 0 {
		t.Fatal("replied to a non-recipient")
	}
	if err = s.replyToBroadcast(ctx, message(101)); err != nil || count() != 1 {
		t.Fatalf("first reply: %v count=%d", err, count())
	}
	if err = NewService(repo, nil, nil, b).replyToBroadcast(ctx, message(101)); err != nil || count() != 1 {
		t.Fatal("restart resent auto-reply")
	}
	m := message(102)
	m.ReplyToMessage = &models.Message{ID: 13}
	if err = s.replyToBroadcast(ctx, m); err != nil || count() != 1 {
		t.Fatal("replied to unrelated bot message")
	}
	m.ReplyToMessage.ID = 700
	if err = s.replyToBroadcast(ctx, m); err != nil || count() != 2 {
		t.Fatal("explicit reply ignored")
	}
	if _, err = pool.Exec(ctx, "UPDATE broadcast_recipient_reply SET sent_at=NOW()-INTERVAL '25 hours' WHERE chat_id=103"); err != nil {
		t.Fatal(err)
	}
	if err = s.replyToBroadcast(ctx, message(103)); err != nil || count() != 2 {
		t.Fatal("expired reply window ignored")
	}
	mu.Lock()
	failNext = true
	mu.Unlock()
	if err = s.replyToBroadcast(ctx, message(104)); err == nil || count() != 3 {
		t.Fatal("send failure not reported")
	}
	if err = s.replyToBroadcast(ctx, message(104)); err != nil || count() != 4 {
		t.Fatal("failed send left recipient locked")
	}
	// Only a new campaign resets the once-per-broadcast response state.
	if err = repo.RecordBroadcastRecipient(ctx, 101, 900, time.Now().Truncate(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = repo.RecordBroadcastRecipient(ctx, 101, 700, time.Now()); err != nil {
		t.Fatal(err)
	}
	m = message(101)
	m.ReplyToMessage = &models.Message{ID: 700}
	if err = s.replyToBroadcast(ctx, m); err != nil || count() != 4 {
		t.Fatal("stale delivery replaced new campaign")
	}
	m.ReplyToMessage = nil
	m.ID = 800
	if err = s.replyToBroadcast(ctx, m); err != nil || count() != 4 {
		t.Fatal("replayed pre-campaign message received a reply")
	}
	m.ID = 1002
	if err = s.replyToBroadcast(ctx, m); err != nil || count() != 5 {
		t.Fatalf("new campaign did not reset state: %v", err)
	}
	// Lease prevents concurrent updates from sending two replies; a crashed worker can retry later.
	if err = repo.RecordBroadcastRecipient(ctx, 102, 900, time.Now().Truncate(time.Second)); err != nil {
		t.Fatal(err)
	}
	first, err := repo.ClaimBroadcastReply(ctx, 102, 1002, 0, time.Now(), 24)
	if err != nil || first != 900 {
		t.Fatal("failed to claim")
	}
	if again, err := repo.ClaimBroadcastReply(ctx, 102, 1003, 0, time.Now(), 24); err != nil || again != 0 {
		t.Fatal("claimed recipient twice")
	}
	if _, err = pool.Exec(ctx, "UPDATE broadcast_recipient_reply SET lease_until=NOW()-INTERVAL '1 second' WHERE chat_id=102"); err != nil {
		t.Fatal(err)
	}
	if err = s.replyToBroadcast(ctx, message(102)); err != nil || count() != 6 {
		t.Fatal("expired worker lease did not recover")
	}
	settings.Enabled = false
	settings, err = s.SaveAutoReply(ctx, *settings)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.RecordBroadcastRecipient(ctx, 101, 950, time.Now().Truncate(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = s.replyToBroadcast(ctx, message(101)); err != nil || count() != 6 {
		t.Fatal("disabled auto-reply sent")
	}
	// A plain message is escaped rather than silently interpreted as markup.
	settings.Enabled = true
	settings.Text = "Цена < 100 & > 10"
	if _, err = s.SaveAutoReply(ctx, *settings); err != nil {
		t.Fatal(err)
	}
	if err = s.replyToBroadcast(ctx, message(101)); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	last := requests[len(requests)-1]
	mu.Unlock()
	if last != "Цена &lt; 100 &amp; &gt; 10" {
		t.Fatalf("unsafe plain text: %s", last)
	}
	encoded, _ := json.Marshal(settings)
	if !strings.Contains(string(encoded), "updatedAt") {
		t.Fatal("missing settings version")
	}
	raw, err = os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000067_broadcast_auto_reply.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
}
