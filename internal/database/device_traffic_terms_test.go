package database

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"link-bot/internal/trafficpolicy"
)

func TestPurchaseFreezesDeviceTrafficAtInvoiceCreation(t *testing.T) {
	dsn := os.Getenv("DEVICE_QUOTA_TEST_DATABASE_URL")
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
	schema := fmt.Sprintf("purchase_traffic_test_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	_, err = pool.Exec(ctx, `CREATE TABLE app_runtime_settings(id INT PRIMARY KEY,config JSONB);
	CREATE TABLE purchase(id BIGSERIAL PRIMARY KEY,amount FLOAT8,customer_id BIGINT,subscription_id BIGINT,created_at TIMESTAMPTZ DEFAULT NOW(),month INT,days INT,plan_id TEXT,
	traffic_limit_bytes BIGINT,device_limit_count INT,purchase_kind TEXT,extra_devices INT,extra_traffic_bytes BIGINT,extra_traffic_unlimited BOOLEAN,device_expires_at TIMESTAMPTZ,is_free_plan BOOLEAN,free_plan_one_time BOOLEAN,paid_at TIMESTAMPTZ,
	currency TEXT,expire_at TIMESTAMPTZ,status TEXT,invoice_type TEXT,promo_code_id BIGINT,promo_code_snapshot TEXT,promo_code_discount_percent INT,crypto_invoice_id BIGINT,crypto_invoice_url TEXT,yookasa_url TEXT,yookasa_id UUID,
	agreement_accepted BOOLEAN,is_auto_payment BOOLEAN,parent_purchase_id BIGINT,yookasa_payment_method_id UUID,yookasa_payment_method_type TEXT,yookasa_payment_method_title TEXT,yookasa_payment_method_saved BOOLEAN,
	external_payment_id TEXT,external_payment_url TEXT,gift_recipient_username TEXT,gift_recipient_customer_id BIGINT,gift_token UUID,gift_delivered_at TIMESTAMPTZ,gift_sender_seen_at TIMESTAMPTZ);
	INSERT INTO app_runtime_settings VALUES(1,'{"plans":[{"id":"p","deviceTrafficEnabled":true,"deviceTrafficEnforced":true,"deviceTrafficRules":[{"devices":1,"trafficGb":20}]}]}');
	INSERT INTO purchase(customer_id,month,days,plan_id,status) VALUES(9,1,0,'p','paid'),(10,1,0,'missing','paid');`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/migrations/000061_purchase_device_traffic_terms.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	var legacy, disabled trafficpolicy.Terms
	if err = pool.QueryRow(ctx, `SELECT device_traffic_terms FROM purchase WHERE customer_id=9`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if !legacy.Enforced || len(legacy.Rules) != 1 || legacy.Rules[0].TrafficGB != 20 {
		t.Fatalf("current terms not preserved: %+v", legacy)
	}
	if err = pool.QueryRow(ctx, `SELECT device_traffic_terms FROM purchase WHERE customer_id=10`).Scan(&disabled); err != nil || disabled.Enabled || disabled.Enforced {
		t.Fatalf("migration enabled unknown old purchase: %+v %v", disabled, err)
	}
	repo := NewPurchaseRepository(pool)
	planID := "p"
	create := func() *Purchase {
		t.Helper()
		p := &Purchase{CustomerID: 1, Month: 1, PlanID: &planID, Status: PurchaseStatusNew, InvoiceType: InvoiceTypeFree, Currency: "RUB"}
		id, e := repo.Create(ctx, p)
		if e != nil {
			t.Fatal(e)
		}
		saved, e := repo.FindById(ctx, id)
		if e != nil {
			t.Fatal(e)
		}
		return saved
	}
	pending := create()
	if pending.DeviceTrafficTerms == nil || !pending.DeviceTrafficTerms.Enforced {
		t.Fatal("invoice omitted purchased quota terms")
	}
	if _, err = pool.Exec(ctx, `UPDATE app_runtime_settings SET config='{"plans":[{"id":"p","deviceTrafficEnabled":false,"deviceTrafficEnforced":false,"deviceTrafficRules":[]}]}'`); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.FindById(ctx, pending.ID)
	if err != nil || !reflect.DeepEqual(stored.DeviceTrafficTerms, pending.DeviceTrafficTerms) {
		t.Fatalf("catalog edit changed pending invoice: %+v %v", stored, err)
	}
	newPurchase := create()
	if newPurchase.DeviceTrafficTerms == nil || newPurchase.DeviceTrafficTerms.Enabled || newPurchase.DeviceTrafficTerms.Enforced {
		t.Fatal("new sale ignored updated terms")
	}
	if _, err = pool.Exec(ctx, `UPDATE app_runtime_settings SET config='{"plans":[]}'`); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.FindById(ctx, pending.ID)
	if err != nil || !stored.DeviceTrafficTerms.Enforced || stored.DeviceTrafficTerms.Rules[0].TrafficGB != 20 {
		t.Fatal("deleting catalog plan lost purchased terms")
	}
	if _, err = pool.Exec(ctx, `UPDATE app_runtime_settings SET config='{"plans":null}'`); err != nil {
		t.Fatal(err)
	}
	withoutPlan := create()
	if withoutPlan.DeviceTrafficTerms == nil || withoutPlan.DeviceTrafficTerms.Enabled {
		t.Fatal("absent catalog did not produce explicit disabled terms")
	}
}
