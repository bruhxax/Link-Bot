package payment

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"link-bot/internal/database"
	"link-bot/internal/integrations"
)

// This test uses its own schema and never contacts Platega, Telegram or Remnawave.
func TestPlategaDisputeDoesNotFulfillPurchaseAgain(t *testing.T) {
	dsn := os.Getenv("PAYMENT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PAYMENT_TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("platega_test_%d", time.Now().UnixNano())
	cfg.MaxConns = 4
	cfg.ConnConfig.PreferSimpleProtocol = true
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path TO "+schema)
		return err
	}
	pool, err := pgxpool.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	files, err := filepath.Glob("../../db/migrations/*.up.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	for _, file := range files {
		raw, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("migration %s: %v", file, err)
		}
	}
	settings, err := integrations.NewService(ctx, database.NewPaymentIntegrationRepository(pool))
	if err != nil {
		t.Fatal(err)
	}
	_, err = settings.Update(ctx, integrations.ProviderPlatega, integrations.UpdateInput{
		Enabled: true, Fields: map[string]string{"merchantId": "merchant", "secretKey": "secret", "apiUrl": "https://app.platega.io"},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	repo := database.NewPurchaseRepository(pool)
	s := PaymentService{purchaseRepository: repo, integrationGateway: integrations.NewGateway(settings)}
	var purchaseID int64
	// paid_at is the durable evidence that the first CONFIRMED was fulfilled.
	err = pool.QueryRow(ctx, `WITH buyer AS (INSERT INTO customer(telegram_id) VALUES(42) RETURNING id)
		INSERT INTO purchase(customer_id,amount,currency,invoice_type,month,status,paid_at,external_payment_id)
		SELECT id,200,'RUB','platega',1,'paid',NOW(),'transaction-id' FROM buyer RETURNING id`).Scan(&purchaseID)
	if err != nil {
		t.Fatal(err)
	}
	original, err := repo.FindById(ctx, purchaseID)
	if err != nil {
		t.Fatal(err)
	}
	headers := make(http.Header)
	headers.Set("X-MerchantId", "merchant")
	headers.Set("X-Secret", "secret")
	for _, status := range []string{"CONFIRMED", "DISPUTED", "DISPUTED", "CONFIRMED", "PARTIALLY_REFUNDED", "DISPUTED", "CONFIRMED", "CHARGEBACKED", "CANCELED", "CONFIRMED"} {
		raw := []byte(fmt.Sprintf(`{"id":"transaction-id","amount":211,"currency":"RUB","status":%q,"payload":"untrusted-order"}`, status))
		if response, err := s.ProcessExternalWebhook(ctx, integrations.ProviderPlatega, headers, raw, nil); err != nil || response != "OK" {
			t.Fatalf("callback %s: %q, %v", status, response, err)
		}
		current, err := repo.FindById(ctx, purchaseID)
		if err != nil || current.Status != database.PurchaseStatusPaid || current.PaidAt == nil || !current.PaidAt.Equal(*original.PaidAt) {
			t.Fatalf("callback %s changed fulfillment: %+v, %v", status, current, err)
		}
	}
	// Reproduce a delayed cancellation that read the purchase before fulfillment.
	if err := repo.CancelIfUnpaid(ctx, purchaseID); err != nil {
		t.Fatal(err)
	}
	current, err := repo.FindById(ctx, purchaseID)
	if err != nil || current.Status != database.PurchaseStatusPaid {
		t.Fatalf("late cancellation erased paid status: %+v, %v", current, err)
	}
	// A historical status overwrite must not erase the evidence in paid_at.
	if err := repo.UpdateFields(ctx, purchaseID, map[string]interface{}{"status": database.PurchaseStatusCancel}); err != nil {
		t.Fatal(err)
	}
	if err := s.ProcessPurchaseById(ctx, purchaseID); err != nil {
		t.Fatalf("already fulfilled purchase was processed again: %v", err)
	}
	if err := repo.CancelIfUnpaid(ctx, purchaseID); err != nil {
		t.Fatal(err)
	}
	// Unpaid cancellations still work and are safe to repeat.
	var unpaidID int64
	if err := pool.QueryRow(ctx, `INSERT INTO purchase(customer_id,amount,currency,invoice_type,month,status)
		VALUES($1,200,'RUB','platega',1,'pending') RETURNING id`, original.CustomerID).Scan(&unpaidID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := repo.CancelIfUnpaid(ctx, unpaidID); err != nil {
			t.Fatal(err)
		}
	}
	unpaid, err := repo.FindById(ctx, unpaidID)
	if err != nil || unpaid.Status != database.PurchaseStatusCancel || unpaid.PaidAt != nil {
		t.Fatalf("unpaid purchase was not canceled: %+v, %v", unpaid, err)
	}
}
