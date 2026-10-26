package broadcast

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"link-bot/internal/database"
)

type pinAPICall struct {
	Method    string
	ChatID    int64
	MessageID int
	SourceID  int
	ParseMode string
	Text      string
}

type pinFixture struct {
	s       *Service
	pool    *pgxpool.Pool
	calls   []pinAPICall
	failPin bool
	nextID  int
	pinned  map[int64]int
	deleted map[int]bool
	mu      sync.Mutex
}

func newPinFixture(t *testing.T) (*pinFixture, context.Context) {
	t.Helper()
	dsn := os.Getenv("BROADCAST_AUDIENCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set BROADCAST_AUDIENCE_TEST_DATABASE_URL to an isolated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	t.Cleanup(cancel)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("pinned_test_%d", time.Now().UnixNano())
	cfg.MaxConns = 3
	cfg.ConnConfig.PreferSimpleProtocol = true
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path TO "+schema)
		return err
	}
	pool, err := pgxpool.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); pool.Close() })
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE customer(id BIGINT PRIMARY KEY,telegram_id BIGINT,is_blocked BOOLEAN,telegram_id_is_synthetic BOOLEAN);
        INSERT INTO customer VALUES(1,101,false,false),(2,102,false,false),(3,103,true,false),(4,104,false,true)`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE email_broadcast(id BIGINT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000017_broadcast_campaign", "000030_broadcast_html", "000046_direct_messages", "000055_broadcast_audience", "000063_broadcast_preferences", "000064_pinned_message", "000066_temporary_broadcast", "000067_broadcast_auto_reply", "000070_broadcast_surveys"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", name+".up.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// An opted-out user must still receive the separately managed pinned announcement.
	if _, err = pool.Exec(ctx, `INSERT INTO customer_broadcast_unsubscribe(customer_id) VALUES(2)`); err != nil {
		t.Fatal(err)
	}
	f := &pinFixture{pool: pool, nextID: 1000, pinned: map[int64]int{}, deleted: map[int]bool{}}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("decode API: %v", err)
		}
		chatID, _ := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
		messageID, _ := strconv.Atoi(r.FormValue("message_id"))
		sourceID, _ := strconv.Atoi(r.FormValue("from_chat_id"))
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		f.calls = append(f.calls, pinAPICall{method, chatID, messageID, sourceID, r.FormValue("parse_mode"), r.FormValue("text")})
		w.Header().Set("Content-Type", "application/json")
		if method == "getChat" {
			if id := f.pinned[chatID]; id != 0 {
				fmt.Fprintf(w, `{"ok":true,"result":{"id":%d,"type":"private","pinned_message":{"message_id":%d}}}`, chatID, id)
			} else {
				fmt.Fprintf(w, `{"ok":true,"result":{"id":%d,"type":"private"}}`, chatID)
			}
			return
		}
		if method == "pinChatMessage" && f.deleted[messageID] {
			fmt.Fprint(w, `{"ok":false,"error_code":400,"description":"Bad Request: message to pin not found"}`)
			return
		}
		if method == "pinChatMessage" && f.failPin {
			f.failPin = false
			w.WriteHeader(500)
			fmt.Fprint(w, `{"ok":false,"error_code":500,"description":"temporary pin failure"}`)
			return
		}
		f.nextID++
		if method == "pinChatMessage" {
			f.pinned[chatID] = messageID
		}
		if method == "unpinChatMessage" && f.pinned[chatID] == messageID {
			delete(f.pinned, chatID)
		}
		if method == "copyMessage" || method == "sendMessage" {
			fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d}}`, f.nextID)
		} else {
			fmt.Fprint(w, `{"ok":true,"result":true}`)
		}
	}))
	t.Cleanup(api.Close)
	b, err := bot.New("123:test", bot.WithSkipGetMe(), bot.WithServerURL(api.URL))
	if err != nil {
		t.Fatal(err)
	}
	f.s = NewService(database.NewBroadcastRepository(pool), database.NewCustomerRepository(pool), nil, b)
	f.s.ConfigurePinnedMessages(database.NewPinnedMessageRepository(pool))
	return f, ctx
}

func (f *pinFixture) count(method string, chatID int64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.Method == method && (chatID == 0 || c.ChatID == chatID) {
			n++
		}
	}
	return n
}

func (f *pinFixture) capture(t *testing.T, ctx context.Context, text string) *database.PinnedMessage {
	t.Helper()
	if _, err := f.s.StartPinnedCapture(ctx, 900); err != nil {
		t.Fatal(err)
	}
	ok, err := f.s.CapturePinnedMessage(ctx, &models.Message{ID: 99, From: &models.User{ID: 900}, Chat: models.Chat{ID: 900}, Text: text})
	if err != nil || !ok {
		t.Fatalf("capture=%v err=%v", ok, err)
	}
	p, err := f.s.PinnedState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPinnedMessageOnlyOnStartAndNewUsers(t *testing.T) {
	f, ctx := newPinFixture(t)
	p := f.capture(t, ctx, "Объявление")
	if _, err := f.s.SetPinnedEnabled(ctx, true, p.Revision-1); !errors.Is(err, database.ErrPinnedStateChanged) {
		t.Fatal("stale toggle accepted", err)
	}
	p, err := f.s.SetPinnedEnabled(ctx, true, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.processPinnedBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if f.count("copyMessage", 101) != 0 || f.count("pinChatMessage", 0) != 0 {
		t.Fatal("enabling must not broadcast")
	}
	if err = f.s.EnsurePinnedOnStart(ctx, 101); err != nil {
		t.Fatal(err)
	}
	if f.count("copyMessage", 101) != 1 || f.count("pinChatMessage", 101) != 1 || f.count("copyMessage", 102) != 0 {
		t.Fatal("start touched another user", f.calls)
	}
	if err = f.s.EnsurePinnedOnStart(ctx, 101); err != nil {
		t.Fatal(err)
	}
	if f.count("copyMessage", 101) != 1 || f.count("pinChatMessage", 101) != 1 {
		t.Fatal("already pinned message duplicated")
	}
	// A manual unpin restores the existing copy on the next /start.
	f.mu.Lock()
	delete(f.pinned, 101)
	f.mu.Unlock()
	if err = f.s.EnsurePinnedOnStart(ctx, 101); err != nil {
		t.Fatal(err)
	}
	if f.count("copyMessage", 101) != 1 || f.count("pinChatMessage", 101) != 2 {
		t.Fatal("manual unpin caused a duplicate")
	}
	if err = f.s.EnsurePinnedOnStart(ctx, 102); err != nil {
		t.Fatal(err)
	}
	if err = f.s.EnsurePinnedOnStart(ctx, 103); err == nil {
		t.Fatal("blocked customer accepted")
	}
	if err = f.s.EnsurePinnedOnStart(ctx, 104); err == nil {
		t.Fatal("synthetic customer accepted")
	}
	if f.count("copyMessage", 103) != 0 || f.count("copyMessage", 104) != 0 {
		t.Fatal("blocked or synthetic user received announcement")
	}
	if _, err = f.pool.Exec(ctx, "INSERT INTO customer VALUES(5,105,false,false)"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.processPinnedBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if f.count("copyMessage", 105) != 0 {
		t.Fatal("new user received message without /start")
	}
	if err = f.s.EnsurePinnedOnStart(ctx, 105); err != nil {
		t.Fatal(err)
	}
	if f.count("pinChatMessage", 105) != 1 {
		t.Fatal("new user not pinned on /start")
	}
	// Saving a replacement must wait for each user's next /start.
	p = f.capture(t, ctx, "Новое объявление")
	if _, err = f.s.processPinnedBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if f.count("copyMessage", 101) != 1 || f.count("copyMessage", 102) != 1 {
		t.Fatal("replacement broadcast unexpectedly")
	}
	if err = f.s.EnsurePinnedOnStart(ctx, 101); err != nil {
		t.Fatal(err)
	}
	if f.count("copyMessage", 101) != 2 || f.count("copyMessage", 102) != 1 || f.count("unpinChatMessage", 101) != 1 {
		t.Fatal("replacement was not isolated to requesting user")
	}
	p, err = f.s.SetPinnedEnabled(ctx, false, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.processPinnedBatch(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{101, 102, 105} {
		if f.pinned[id] != 0 {
			t.Fatalf("disable left announcement pinned for %d", id)
		}
	}
	if _, err = f.s.SetPinnedEnabled(ctx, true, p.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.processPinnedBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if f.count("copyMessage", 101) != 2 {
		t.Fatal("re-enable broadcast a copy")
	}
	if err = f.s.EnsurePinnedOnStart(ctx, 101); err != nil {
		t.Fatal(err)
	}
	if f.count("copyMessage", 101) != 2 {
		t.Fatal("re-enable duplicated current copy")
	}
	for _, c := range f.calls {
		if c.Method == "unpinChatMessage" && c.MessageID == 0 {
			t.Fatal("unspecified unpin could affect another pin")
		}
	}
}

func TestPinnedNextStartRetriesAndDeletedCopy(t *testing.T) {
	f, ctx := newPinFixture(t)
	p := f.capture(t, ctx, "Важное сообщение")
	if _, err := f.s.SetPinnedEnabled(ctx, true, p.Revision); err != nil {
		t.Fatal(err)
	}
	f.failPin = true
	if err := f.s.EnsurePinnedOnStart(ctx, 101); err == nil {
		t.Fatal("pin error ignored")
	}
	if _, err := f.s.processPinnedBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if f.count("pinChatMessage", 101) != 1 || f.count("copyMessage", 102) != 0 {
		t.Fatal("failed start retried in background")
	}
	resumed := NewService(f.s.repository, f.s.customerRepository, nil, f.s.telegramBot)
	resumed.ConfigurePinnedMessages(database.NewPinnedMessageRepository(f.pool))
	if err := resumed.EnsurePinnedOnStart(ctx, 101); err != nil {
		t.Fatal(err)
	}
	if f.count("copyMessage", 101) != 1 || f.count("pinChatMessage", 101) != 2 {
		t.Fatal("next start did not reuse saved copy")
	}
	d, err := resumed.pinnedRepository.Delivery(ctx, 101)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	delete(f.pinned, 101)
	f.deleted[*d.MessageID] = true
	f.mu.Unlock()
	if err = resumed.EnsurePinnedOnStart(ctx, 101); err != nil {
		t.Fatal(err)
	}
	if f.count("copyMessage", 101) != 2 {
		t.Fatal("deleted message not recreated during same /start")
	}
	p, err = resumed.PinnedState(ctx)
	if err != nil || p.Completed != 1 || p.Failed != 0 {
		t.Fatalf("state: %+v %v", p, err)
	}
}

func TestPinnedCaptureOwnershipConflictsAndFormatting(t *testing.T) {
	f, ctx := newPinFixture(t)
	if _, err := f.s.StartPinnedCapture(ctx, 900); err != nil {
		t.Fatal(err)
	}
	for _, m := range []*models.Message{
		{From: &models.User{ID: 901}, Chat: models.Chat{ID: 901}, Text: "wrong admin"},
		{From: &models.User{ID: 900}, Chat: models.Chat{ID: -100}, Text: "group"},
		{From: &models.User{ID: 900}, Chat: models.Chat{ID: 900}, Text: "/start"},
	} {
		if captured, err := f.s.CapturePinnedMessage(ctx, m); captured || err != nil {
			t.Fatalf("wrong source captured: %v %v", captured, err)
		}
	}
	if _, err := f.s.StartCapture(ctx, 900); !errors.Is(err, ErrRunning) {
		t.Fatalf("broadcast conflicts with pin: %v", err)
	}
	if _, err := f.s.StartDirectCapture(ctx, 900, 1); !errors.Is(err, ErrRunning) {
		t.Fatalf("direct capture conflicts with pin: %v", err)
	}
	if _, err := f.s.StartPinnedCapture(ctx, 901); !errors.Is(err, database.ErrPinnedStateChanged) {
		t.Fatalf("another admin stole capture: %v", err)
	}
	bad := &models.Message{ID: 99, From: &models.User{ID: 900}, Chat: models.Chat{ID: 900}, Text: "<b>broken"}
	if captured, err := f.s.CapturePinnedMessage(ctx, bad); !captured || err != nil {
		t.Fatal(captured, err)
	}
	p, _ := f.s.PinnedState(ctx)
	if !p.AwaitingMessage || p.HasMessage {
		t.Fatal("invalid HTML was saved")
	}
	bad.Text = "Hello 😊"
	bad.Entities = []models.MessageEntity{{Type: models.MessageEntityTypeBold, Offset: 0, Length: 5}, {Type: models.MessageEntityTypeCustomEmoji, Offset: 6, Length: 2, CustomEmojiID: "52764212364458059956"}}
	if captured, err := f.s.CapturePinnedMessage(ctx, bad); !captured || err != nil {
		t.Fatal(captured, err)
	}
	p, _ = f.s.PinnedState(ctx)
	if !p.HasMessage || p.SourceMessageID == nil || *p.SourceMessageID == 99 {
		t.Fatal("source must be a bot-owned copy")
	}
	if f.count("copyMessage", 900) != 1 {
		t.Fatal("native Telegram entities must be copied intact")
	}
	if _, err := f.s.PreviewPinnedMessage(ctx, 900); err != nil {
		t.Fatal(err)
	}
	if f.count("pinChatMessage", 0) != 0 {
		t.Fatal("preview pinned a message")
	}
	if _, err := f.s.StartCapture(ctx, 900); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.StartPinnedCapture(ctx, 900); !errors.Is(err, ErrRunning) {
		t.Fatal("pin stole broadcast source", err)
	}
	if _, err := f.s.repository.Reset(ctx, 900); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.StartPinnedCapture(ctx, 900); err != nil {
		t.Fatal(err)
	}
	bad.Text = "/cancel"
	if captured, err := f.s.CapturePinnedMessage(ctx, bad); !captured || err != nil {
		t.Fatal(captured, err)
	}
	p, _ = f.s.PinnedState(ctx)
	if p.AwaitingMessage || !p.HasMessage {
		t.Fatal("cancel must preserve old source")
	}
}
