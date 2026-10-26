package miniapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"link-bot/internal/broadcast"
	"link-bot/internal/database"
)

func TestBroadcastTemplatesRestoreDraftsWithoutSendingAndRejectStaleState(t *testing.T) {
	dsn := os.Getenv("PANEL_SYNC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
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
	schema := fmt.Sprintf("broadcast_templates_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	for _, name := range []string{"000017_broadcast_campaign.up.sql", "000030_broadcast_html.up.sql", "000048_email_link_and_broadcast.up.sql", "000055_broadcast_audience.up.sql", "000066_temporary_broadcast.up.sql", "000059_broadcast_templates.up.sql"} {
		raw, err := os.ReadFile("../../db/migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		// The email auth tables belong to a different feature; test its broadcast table verbatim.
		if strings.HasPrefix(name, "000048") {
			raw = raw[strings.Index(string(raw), "CREATE TABLE email_broadcast"):]
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("migration %s: %v", name, err)
		}
	}
	cr, br := database.NewCustomerRepository(pool), database.NewBroadcastRepository(pool)
	svc := broadcast.NewService(br, cr, nil, nil) // No Telegram transport: applying a template cannot send.
	h := &Handler{customerRepository: cr, broadcastService: svc}
	sess := &session{User: telegramUser{ID: 42}, AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"broadcast"}}}
	request := func(who *session, payload any, want int) string {
		t.Helper()
		raw, _ := json.Marshal(payload)
		w := httptest.NewRecorder()
		h.handleBroadcastTemplates(w, httptest.NewRequest("POST", "/api/mini-app/admin/broadcast/templates", strings.NewReader(string(raw))).WithContext(ctx), who, nil)
		if w.Code != want {
			t.Fatalf("want %d, got %d: %s", want, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	_, err = pool.Exec(ctx, `UPDATE bot_broadcast_draft SET status='finished',delete_after_minutes=720,source_chat_id=42,source_message_id=27,source_kind='text',source_preview='Короткое описание',source_html=$1,buttons='[{"id":"one","type":"url","text":"Открыть","url":"https://example.com","style":"primary"}]',recipient_count=5,sent_count=4,failed_count=1`, strings.Repeat("Полный текст ", 40)+"\nВторая строка")
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	request(sess, map[string]any{"action": "save", "channel": "telegram", "name": "Сообщение", "updatedAt": d.UpdatedAt, "audience": map[string]any{"category": "active", "language": "ru"}}, 200)
	items, err := cr.BroadcastTemplates(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("list: %+v %v", items, err)
	}
	list := request(sess, map[string]any{"action": "list"}, 200)
	for _, secret := range []string{"sourceChatId", "sourceMessageId", "sourceHTML", "Полный текст"} {
		if strings.Contains(list, secret) {
			t.Fatalf("metadata disclosed %s", secret)
		}
	}
	apply := map[string]any{"action": "apply", "channel": "telegram", "id": items[0].ID, "updatedAt": d.UpdatedAt}
	request(sess, apply, 200)
	restored, err := svc.Get(ctx)
	if err != nil || restored.Status != "draft" || restored.DeleteAfterMinutes != 720 || restored.SourceMessageID == nil || *restored.SourceMessageID != 27 || restored.SentCount != 0 || restored.RecipientCount != 0 || restored.FailedCount != 0 || restored.Audience.Category != "active" || len(restored.Buttons) != 1 || !strings.Contains(restored.SourceText, "Вторая строка") || len(restored.SourceText) < 400 || !restored.UpdatedAt.After(d.UpdatedAt) {
		t.Fatalf("template did not restore whole draft safely: %+v %v", restored, err)
	}
	request(sess, apply, 409) // The first apply invalidates its nonce.
	apply["updatedAt"] = restored.UpdatedAt
	pool.Exec(ctx, `UPDATE bot_broadcast_draft SET status='running'`)
	request(sess, apply, 409)
	pool.Exec(ctx, `UPDATE bot_broadcast_draft SET status='draft'`)
	apply["channel"] = "email"
	request(sess, apply, 404)
	request(&session{User: telegramUser{ID: 43}, AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"smtp"}}}, map[string]any{"action": "list"}, 403)
	email, err := cr.SaveEmailBroadcastDraft(ctx, "Тема письма", "Текст письма\nВторая строка", 42)
	if err != nil {
		t.Fatal(err)
	}
	request(sess, map[string]any{"action": "save", "channel": "email", "name": "Письмо", "updatedAt": email.UpdatedAt, "audience": map[string]any{"category": "trial"}}, 200)
	items, _ = cr.BroadcastTemplates(ctx)
	request(sess, map[string]any{"action": "save", "channel": "email", "name": "Старый", "updatedAt": time.Unix(1, 0)}, 409)
	email, err = cr.SaveEmailBroadcastDraft(ctx, "Другое письмо", "Другой текст", 42)
	if err != nil {
		t.Fatal(err)
	}
	emailApply := map[string]any{"action": "apply", "channel": "email", "id": items[0].ID, "updatedAt": email.UpdatedAt}
	request(sess, emailApply, 200)
	e, _ := cr.EmailBroadcastDraft(ctx)
	if e.Status != "draft" || e.Subject != "Тема письма" || e.Body != "Текст письма\nВторая строка" || e.Audience.Category != "trial" {
		t.Fatalf("email restore: %+v", e)
	}
	request(sess, emailApply, 409)
	emailApply["updatedAt"] = e.UpdatedAt
	pool.Exec(ctx, `UPDATE email_broadcast SET status='running'`)
	request(sess, emailApply, 409)
	request(sess, map[string]any{"action": "delete", "id": items[0].ID}, 200)
	request(sess, emailApply, 404)
	request(sess, map[string]any{"action": "save", "channel": "wrong", "updatedAt": time.Now()}, 400)
}
