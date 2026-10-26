package integrations

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"link-bot/internal/database"
)

func TestLaveshkaPaymentSurvivesSettingsSaveRestartAndKeyRotation(t *testing.T) {
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
	schema := fmt.Sprintf("lave_payment_%d", time.Now().UnixNano())
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE SCHEMA " + schema + ";SET search_path TO " + schema)
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE; SET search_path TO public")
	exec("CREATE TABLE purchase(id BIGINT PRIMARY KEY,invoice_type TEXT);INSERT INTO purchase VALUES(42,'laveshka')")
	for _, name := range []string{"000019_payment_integrations.up.sql", "000069_laveshka_quote.up.sql"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		exec(string(raw))
	}
	repo := database.NewPaymentIntegrationRepository(pool)
	settings, err := NewService(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled(ProviderLaveshka) {
		t.Fatal("new provider enabled without credentials")
	}
	fields := map[string]string{"apiKey": "LAVE-12-fixture", "webhookSecret": "whsec_fixture", "rubPerUsd": "100"}
	if _, err := settings.Update(ctx, ProviderLaveshka, UpdateInput{Enabled: true, Fields: fields}, 1); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Find(ctx, ProviderLaveshka)
	if err != nil || stored == nil || strings.Contains(stored.EncryptedConfig, "fixture") {
		t.Fatal("credentials not encrypted")
	}
	attachAPI := func(g *Gateway, status string) {
		g.httpClient = &http.Client{Transport: laveTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(laveResponse(laveFixture(status)))), Header: make(http.Header)}, nil
		})}
	}
	gateway := NewGateway(settings)
	attachAPI(gateway, "active")
	created, err := gateway.Create(ctx, CreatePaymentRequest{Provider: ProviderLaveshka, PurchaseID: 42, Amount: 89})
	if err != nil || created.URL != laveTestURL {
		t.Fatalf("create %+v %v", created, err)
	}
	// Blank secret inputs preserve stored credentials; a changed key for the
	// same merchant and a new exchange rate must not invalidate an old invoice.
	if _, err := settings.Update(ctx, ProviderLaveshka, UpdateInput{Enabled: true, Fields: map[string]string{"apiKey": "LAVE-12-rotated", "webhookSecret": "", "rubPerUsd": "200"}}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Update(ctx, ProviderLaveshka, UpdateInput{Enabled: true, Fields: map[string]string{"rubPerUsd": "0"}}, 1); err == nil {
		t.Fatal("invalid rate saved")
	}
	settings, err = NewService(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	config, ok := settings.Config(ProviderLaveshka)
	if !ok || config["rubPerUsd"] != "200.000000" || config["webhookSecret"] != "whsec_fixture" || config["apiKey"] != "LAVE-12-rotated" {
		t.Fatal("saved credentials/rate lost on restart")
	}
	gateway = NewGateway(settings)
	attachAPI(gateway, "paid")
	raw, headers := laveEvent(laveFixture("paid"), "invoice.paid")
	for i := 0; i < 2; i++ {
		event, err := gateway.HandleWebhook(ctx, ProviderLaveshka, headers, raw, nil)
		if err != nil || !event.Paid || event.Amount != 89 || event.PurchaseID != 42 {
			t.Fatalf("payment after restart: %+v %v", event, err)
		}
	}
}
