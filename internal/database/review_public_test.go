package database

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

func TestPublicReviewContainsOnlyPublicFields(t *testing.T) {
	raw, err := json.Marshal(PublicReview{ID: 1, Username: "review_user", Rating: 5, Comment: "Отлично"})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 4 {
		t.Fatalf("unexpected public fields: %s", raw)
	}
	for _, key := range []string{"id", "username", "rating", "comment"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("missing public field %s", key)
		}
	}
}

func TestListPublicReviewsIncludesAllAndExcludesDeleted(t *testing.T) {
	dsn := os.Getenv("REVIEWS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set REVIEWS_TEST_DATABASE_URL to an isolated database")
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
	schema := fmt.Sprintf("public_reviews_test_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	_, err = pool.Exec(ctx, `CREATE TABLE review(id BIGINT PRIMARY KEY,telegram_username TEXT,rating INT,comment TEXT,created_at TIMESTAMPTZ,deleted_at TIMESTAMPTZ);
 INSERT INTO review SELECT n,'review_user',1+(n%5),'Отзыв '||n,NOW(),NULL FROM generate_series(1,75) AS n;
 UPDATE review SET deleted_at=NOW() WHERE id=75;`)
	if err != nil {
		t.Fatal(err)
	}
	items, err := NewReviewRepository(pool).ListPublic(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 74 || items[0].ID != 74 || items[73].ID != 1 {
		t.Fatalf("reviews truncated, deleted or unordered: %+v", items)
	}
}
