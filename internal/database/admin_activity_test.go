package database

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// An isolated schema ensures these checks never read or alter application data.
func TestAdminActivityPersistenceAndStablePagination(t *testing.T) {
	dsn := os.Getenv("ADMIN_ACTIVITY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set ADMIN_ACTIVITY_TEST_DATABASE_URL to an isolated test database")
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
	schema := fmt.Sprintf("admin_activity_test_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	if _, err = pool.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000054_admin_activity.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	repo := &CustomerRepository{pool: pool}
	insert := func(actor int64, category, title string) int64 {
		t.Helper()
		id, err := repo.CreateAdminActivity(ctx, AdminActivity{ActorTelegramID: actor, ActorName: "operator", ActorRole: "Поддержка", Action: "test", Category: category, Title: title, Status: "pending"})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := insert(22, "users", "Изменил баланс")
	second := insert(22, "support", "Закрыл обращение")
	third := insert(22, "settings", "Изменил триал")
	insert(33, "users", "Другой администратор")
	// Earlier versions mislabeled background detail reads as card visits. Keep
	// their raw history, but never show this unreliable noise in the journal.
	for i := 0; i < 10; i++ {
		_, err := repo.CreateAdminActivity(ctx, AdminActivity{ActorTelegramID: 22, ActorName: "operator", Action: "users/detail", Category: "users", Title: "Открыл карточку пользователя", TargetTelegramID: 8544649953, TargetName: "tester", Status: "success"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.FinishAdminActivity(ctx, AdminActivity{ID: first, Title: "Изменил баланс", TargetTelegramID: 8544649953, TargetName: "tester", Details: []AdminActivityDetail{{Label: "Баланс", Before: "0", After: "100"}}, Status: "success"}); err != nil {
		t.Fatal(err)
	}
	items, more, err := repo.ListAdminActivity(ctx, AdminActivityQuery{TelegramID: 22, Limit: 2})
	if err != nil || !more || len(items) != 2 || items[0].ID != third || items[1].ID != second {
		t.Fatalf("first page %+v %v %v", items, more, err)
	}
	insert(22, "users", "Новое действие между страницами")
	// A fresh repository sees durable entries, and concurrent inserts do not shift the cursor.
	repo = &CustomerRepository{pool: pool}
	items, more, err = repo.ListAdminActivity(ctx, AdminActivityQuery{TelegramID: 22, BeforeID: second, Limit: 2})
	if err != nil || more || len(items) != 1 || items[0].ID != first || items[0].FinishedAt == nil || items[0].Status != "success" || items[0].Details[0].After != "100" {
		t.Fatalf("persistent second page %+v %v %v", items, more, err)
	}
	for _, query := range []string{"tester", "8544649953", "Баланс"} {
		items, _, err = repo.ListAdminActivity(ctx, AdminActivityQuery{TelegramID: 22, Limit: 20, Query: query})
		if err != nil || len(items) != 1 || items[0].ID != first {
			t.Fatalf("search %q %+v %v", query, items, err)
		}
	}
	items, _, err = repo.ListAdminActivity(ctx, AdminActivityQuery{TelegramID: 22, Limit: 20, Category: "support"})
	if err != nil || len(items) != 1 || items[0].ID != second {
		t.Fatal("category filtering failed")
	}
	// Finalized entries cannot be overwritten by a delayed second completion.
	if err = repo.FinishAdminActivity(ctx, AdminActivity{ID: first, Title: "wrong", Status: "failed"}); err != nil {
		t.Fatal(err)
	}
	items, _, err = repo.ListAdminActivity(ctx, AdminActivityQuery{TelegramID: 22, Limit: 20, Query: "tester"})
	if err != nil || len(items) != 1 || items[0].Status != "success" {
		t.Fatal("completed entry was overwritten")
	}
	raw, err = os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000054_admin_activity.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
}
