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

// This opt-in suite creates and drops its own schema. It works with PostgreSQL
// or an isolated PGlite socket server, without accessing application records.
func TestAISupportPersistentFlow(t *testing.T) {
	dsn := os.Getenv("SUPPORT_AI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set SUPPORT_AI_TEST_DATABASE_URL to an isolated test database")
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
	schema := fmt.Sprintf("support_ai_test_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	if _, err := pool.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE customer(id BIGINT PRIMARY KEY); INSERT INTO customer VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000006_support_tickets.up.sql", "000035_support_message_media.up.sql", "000050_support_ai.up.sql", "000051_support_ai_knowledge.up.sql", "000053_support_operator.up.sql"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("migration %s: %v", name, err)
		}
	}
	repo := NewSupportRepository(pool)
	create := func() *SupportTicket {
		t.Helper()
		ticket, err := repo.CreateTicket(ctx, &SupportTicket{CustomerID: 1, Subject: "VPN", CustomerName: "tester"}, &SupportMessage{Body: "Не подключается", AuthorTelegramID: 10})
		if err != nil {
			t.Fatal(err)
		}
		return ticket
	}
	claim := func() *SupportAIClaim {
		t.Helper()
		item, err := repo.ClaimAIMessage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if item == nil {
			t.Fatal("expected queued message")
		}
		return item
	}
	empty := func() {
		t.Helper()
		item, err := repo.ClaimAIMessage(ctx)
		if err != nil || item != nil {
			t.Fatalf("unexpected claim %+v %v", item, err)
		}
	}
	add := func(ticketID int64, body string) {
		t.Helper()
		_, err := repo.AddCustomerMessage(ctx, ticketID, 10, body, "tester", "", "")
		if err != nil {
			t.Fatal(err)
		}
	}

	ticket := create()
	assertThinking := func(id int64, expected bool) {
		t.Helper()
		actual, err := repo.AIThinking(ctx, id)
		if err != nil || actual != expected {
			t.Fatalf("thinking state: got %v, want %v: %v", actual, expected, err)
		}
	}
	assertThinking(ticket.ID, true)
	first := claim()
	if first.TicketID != ticket.ID {
		t.Fatal("wrong ticket claimed")
	}
	empty() // An active lease prevents duplicate work.
	add(ticket.ID, "Дополнительные сведения")
	if committed, err := repo.FinishAIMessage(ctx, *first, "Старый ответ", false); err != nil || committed {
		t.Fatalf("stale answer committed: %v %v", committed, err)
	}
	second := claim()
	if second.MessageID == first.MessageID {
		t.Fatal("newer message was lost")
	}
	if committed, err := repo.FinishAIMessage(ctx, *second, "Уточните приложение", false); err != nil || !committed {
		t.Fatalf("AI answer failed: %v %v", committed, err)
	}
	if committed, err := repo.FinishAIMessage(ctx, *second, "Дубль", false); err != nil || committed {
		t.Fatalf("duplicate committed: %v %v", committed, err)
	}
	messages, err := repo.ListMessagesByTicket(ctx, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 || messages[2].AuthorRole != SupportAuthorRoleAI {
		t.Fatalf("wrong conversation %+v", messages)
	}
	updated, err := repo.FindTicketByID(ctx, ticket.ID)
	if err != nil || updated.CustomerUnreadCount != 1 || updated.AdminUnreadCount != 2 {
		t.Fatalf("unread counts incorrect: %+v %v", updated, err)
	}
	empty()
	assertThinking(ticket.ID, false)

	add(ticket.ID, "Позови оператора")
	assertThinking(ticket.ID, true)
	pending := claim()
	if handed, err := repo.HandoffAI(ctx, ticket.ID, "Позвал администратора"); err != nil || !handed {
		t.Fatalf("handoff: %v %v", handed, err)
	}
	if committed, err := repo.FinishAIMessage(ctx, *pending, "Опоздавший ответ", false); err != nil || committed {
		t.Fatalf("answered after handoff: %v %v", committed, err)
	}
	if handed, err := repo.HandoffAI(ctx, ticket.ID, "Дубль передачи"); err != nil || handed {
		t.Fatalf("duplicate handoff: %v %v", handed, err)
	}
	add(ticket.ID, "Ещё вопрос для администратора")
	assertThinking(ticket.ID, false)
	empty() // Handoff persists for all subsequent customer messages.
	notifyID, err := repo.ClaimAINotification(ctx)
	if err != nil || notifyID != ticket.ID {
		t.Fatalf("missing notification: %d %v", notifyID, err)
	}
	if duplicate, err := repo.ClaimAINotification(ctx); err != nil || duplicate != 0 {
		t.Fatalf("duplicate notification lease: %d %v", duplicate, err)
	}
	if err := repo.CompleteAINotification(ctx, ticket.ID); err != nil {
		t.Fatal(err)
	}
	if id, err := repo.ClaimAINotification(ctx); err != nil || id != 0 {
		t.Fatal("completed notification replayed")
	}

	ticket = create()
	pending = claim()
	if err := repo.SetOperator(ctx, ticket.ID, 99, "Оператор", false, false); err != nil {
		t.Fatal(err)
	}
	if committed, err := repo.FinishAIMessage(ctx, *pending, "Ответ после взятия в работу", false); err != nil || committed {
		t.Fatalf("AI replied after claim: %v %v", committed, err)
	}
	if err := repo.SetOperator(ctx, ticket.ID, 100, "Другой", false, false); err != ErrSupportOperatorConflict {
		t.Fatalf("second operator stole ticket: %v", err)
	}
	if err := repo.SetOperator(ctx, ticket.ID, 100, "Другой", true, false); err != ErrSupportOperatorConflict {
		t.Fatalf("second operator released ticket: %v", err)
	}
	loaded, err := repo.FindTicketByID(ctx, ticket.ID)
	if err != nil || loaded.OperatorTelegramID != 99 || loaded.OperatorName != "Оператор" {
		t.Fatalf("operator not persisted: %+v %v", loaded, err)
	}
	listed, err := repo.ListTicketsForAdmin(ctx, SupportTicketStatusOpen)
	if err != nil || len(listed) == 0 || listed[0].OperatorTelegramID != 99 {
		t.Fatalf("operator missing from list: %+v %v", listed, err)
	}
	assertThinking(ticket.ID, false)
	add(ticket.ID, "Оператор ещё разбирается")
	empty()
	if err := repo.SetOperator(ctx, ticket.ID, 99, "", true, false); err != nil {
		t.Fatal(err)
	}
	add(ticket.ID, "Вернулся в очередь")
	empty()
	if err := repo.CloseTicket(ctx, ticket.ID); err != nil {
		t.Fatal(err)
	}

	ticket = create()
	pending = claim()
	if _, err := repo.AddAdminMessage(ctx, ticket.ID, 99, "Отвечает человек"); err != nil {
		t.Fatal(err)
	}
	if committed, err := repo.FinishAIMessage(ctx, *pending, "Опоздавший ответ", false); err != nil || committed {
		t.Fatalf("answered after admin: %v %v", committed, err)
	}
	add(ticket.ID, "Спасибо")
	empty()

	ticket = create()
	pending = claim()
	if err := repo.CloseTicket(ctx, ticket.ID); err != nil {
		t.Fatal(err)
	}
	assertThinking(ticket.ID, false)
	if committed, err := repo.FinishAIMessage(ctx, *pending, "Ответ в закрытый тикет", false); err != nil || committed {
		t.Fatalf("answered after close: %v %v", committed, err)
	}
	empty()

	ticket = create()
	pending = claim()
	if _, err := pool.Exec(ctx, `UPDATE support_ai_state SET lease_until = NOW() - INTERVAL '1 second' WHERE ticket_id = $1`, ticket.ID); err != nil {
		t.Fatal(err)
	}
	reclaimed := claim()
	if reclaimed.MessageID != pending.MessageID {
		t.Fatal("expired lease did not recover")
	}
	if committed, err := repo.FinishAIMessage(ctx, *reclaimed, "Нужна проверка администратором", true); err != nil || !committed {
		t.Fatalf("model handoff: %v %v", committed, err)
	}
	if id, err := repo.ClaimAINotification(ctx); err != nil || id != ticket.ID {
		t.Fatal("model handoff did not enqueue notification")
	}

	if _, err := pool.Exec(ctx, `INSERT INTO customer VALUES (2), (3);
	 CREATE TABLE purchase(customer_id BIGINT, status TEXT, amount NUMERIC, currency TEXT);
	 INSERT INTO purchase VALUES (1, 'paid', 200, 'RUB'), (1, 'new', 900, 'RUB'), (2, 'paid', 5000, 'RUB')`); err != nil {
		t.Fatal(err)
	}
	makeCase := func(owner int64, answer string, closed bool) *SupportTicket {
		t.Helper()
		item, err := repo.CreateTicket(ctx, &SupportTicket{CustomerID: owner, Subject: "Happ Windows импорт", CustomerName: "private owner", CustomerUsername: "private_owner"}, &SupportMessage{Body: "Happ на Windows не импортирует", AuthorTelegramID: owner})
		if err != nil {
			t.Fatal(err)
		}
		if answer != "" {
			if _, err := repo.AddAdminMessage(ctx, item.ID, 99, answer); err != nil {
				t.Fatal(err)
			}
		}
		if closed {
			if err := repo.CloseTicket(ctx, item.ID); err != nil {
				t.Fatal(err)
			}
		}
		return item
	}
	makeCase(2, "Импортируйте ссылку через буфер обмена", true)
	makeCase(3, "Открытый тикет не является решением", false)
	makeCase(2, "", true)
	ownCase := makeCase(1, "Собственная история", true)
	examples, err := repo.FindAISupportExamples(ctx, 1, ticket.ID, "happ OR windows")
	if err != nil || len(examples) != 1 || examples[0].Answer != "Импортируйте ссылку через буфер обмена" {
		t.Fatalf("invalid resolved cases: %+v %v", examples, err)
	}
	history, err := repo.AICustomerHistory(ctx, 1, ticket.ID, "happ OR windows")
	if err != nil || history.TotalTickets != 6 || len(history.Recent) != 5 || history.Recent[0]["subject"] != ownCase.Subject {
		t.Fatalf("invalid owner history: %+v %v", history, err)
	}
	msgs, ok := history.Recent[0]["messages"].([]map[string]any)
	if !ok || len(msgs) != 2 || msgs[1]["role"] != "admin" || msgs[1]["text"] != "Собственная история" {
		t.Fatalf("history messages not decoded: %+v", history.Recent[0])
	}
	totals, err := repo.AIAccountTotals(ctx, 1)
	if err != nil || totals["totalPurchases"] != 2 || totals["successfulPurchases"] != 1 || totals["paidRub"] != float64(200) {
		t.Fatalf("account totals leaked another owner: %+v %v", totals, err)
	}
	knowledgeDown, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000051_support_ai_knowledge.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(knowledgeDown)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000050_support_ai.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(raw)); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM support_message WHERE author_role = 'ai'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback lost compatibility: %d %v", count, err)
	}
}
