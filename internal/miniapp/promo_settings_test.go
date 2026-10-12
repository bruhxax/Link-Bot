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
	"link-bot/internal/database"
)

func TestPromoSettingsKeepRewardsAndHistoryAndRejectStaleOrUnauthorizedEdits(t *testing.T) {
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
	schema := fmt.Sprintf("promo_settings_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	if _, err = pool.Exec(ctx, `CREATE TABLE customer(id BIGINT PRIMARY KEY); INSERT INTO customer VALUES(1); CREATE TABLE customer_subscription(id BIGINT PRIMARY KEY); CREATE TABLE purchase(id BIGINT PRIMARY KEY,status TEXT,customer_id BIGINT); INSERT INTO purchase VALUES(1,'paid',1);`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000009_promo_codes.up.sql", "000010_promo_code_limits.up.sql", "000040_promo_rewards.up.sql"} {
		raw, err := os.ReadFile("../../db/migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE promo_code ADD COLUMN owner_customer_id BIGINT; ALTER TABLE promo_code ADD CONSTRAINT promo_code_personal_limit CHECK(owner_customer_id IS NULL OR(max_redemptions IS NOT NULL AND max_redemptions=1));`); err != nil {
		t.Fatal(err)
	}
	repo := database.NewPromoCodeRepository(pool)
	h := &Handler{promoCodeRepository: repo, purchaseRepository: database.NewPurchaseRepository(pool)}
	access := adminAccess{IsAdmin: true, Permissions: []string{"promocodes"}}
	sess := &session{User: telegramUser{ID: 42}, AdminAccess: access}
	ctx = context.WithValue(ctx, adminAccessContextKey{}, access)
	request := func(who *session, payload any, want int) string {
		t.Helper()
		raw, _ := json.Marshal(payload)
		w := httptest.NewRecorder()
		h.handleAdminUpdatePromoSettings(w, httptest.NewRequest("POST", "/api/mini-app/admin/promocodes/update", strings.NewReader(string(raw))).WithContext(ctx), who, nil)
		if w.Code != want {
			t.Fatalf("want %d got %d: %s", want, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	promo, err := repo.Create(ctx, &database.PromoCode{Code: "WORK", RewardType: "days_traffic", RewardValue: 30, RewardTrafficGB: 10, IsActive: true, CreatedByTelegramID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO promo_code_redemption(promo_code_id,customer_id,purchase_id) VALUES($1,1,1)`, promo.ID); err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Minute)
	payload := map[string]any{"id": promo.ID, "isActive": false, "maxRedemptions": 5, "expiresAt": expiry.Format(time.RFC3339), "updatedAt": promo.UpdatedAt}
	response := request(sess, payload, 200)
	current, err := repo.FindByCode(ctx, "WORK")
	if err != nil {
		t.Fatal(err)
	}
	if current.IsActive || current.RewardType != "days_traffic" || current.RewardValue != 30 || current.RewardTrafficGB != 10 || current.RedemptionCount != 1 || current.MaxRedemptions == nil || *current.MaxRedemptions != 5 || !current.ExpiresAt.Equal(expiry) || !current.UpdatedAt.After(promo.UpdatedAt) {
		t.Fatalf("changed reward/history or failed settings: %+v", current)
	}
	if !strings.Contains(response, `"updatedAt":`) || !strings.Contains(response, `"isActive":false`) {
		t.Fatalf("missing editable metadata: %s", response)
	}
	request(sess, payload, 409)
	payload["updatedAt"] = current.UpdatedAt
	payload["isActive"] = true
	payload["maxRedemptions"] = 0
	payload["expiresAt"] = ""
	request(sess, payload, 200)
	current, _ = repo.FindByCode(ctx, "WORK")
	if !current.IsActive || current.MaxRedemptions != nil || current.ExpiresAt != nil {
		t.Fatal("unlimited settings not restored")
	}
	payload["updatedAt"] = current.UpdatedAt
	payload["rewardValue"] = 999
	request(sess, payload, 400)
	delete(payload, "rewardValue")
	request(&session{AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"users"}}}, payload, 403)
	payload["maxRedemptions"] = -1
	request(sess, payload, 400)
	payload["maxRedemptions"] = 0
	payload["expiresAt"] = "bad"
	request(sess, payload, 400)
	payload["expiresAt"] = ""
	personal, err := repo.Create(ctx, &database.PromoCode{Code: "PERSONAL", DiscountPercent: 20, IsActive: true, CreatedByTelegramID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE promo_code SET owner_customer_id=1,max_redemptions=1 WHERE id=$1`, personal.ID); err != nil {
		t.Fatal(err)
	}
	payload["id"] = personal.ID
	payload["updatedAt"] = personal.UpdatedAt
	request(sess, payload, 400)
	payload["maxRedemptions"] = 1
	request(sess, payload, 200)
	personal, _ = repo.FindByCode(ctx, "PERSONAL")
	if personal.OwnerCustomerID == nil || *personal.MaxRedemptions != 1 {
		t.Fatal("personal ownership lost")
	}
	if err = repo.Delete(ctx, personal.ID); err != nil {
		t.Fatal(err)
	}
	payload["updatedAt"] = personal.UpdatedAt
	request(sess, payload, 409)
}
