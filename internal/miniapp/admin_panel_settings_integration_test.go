package miniapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

func TestPanelOnlySettingsAcceptBrowserPayloadAndVerifyDatabaseBindingBeforePatch(t *testing.T) {
	dsn := os.Getenv("PANEL_SYNC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
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
	schema := fmt.Sprintf("panel_only_settings_%d", time.Now().UnixNano())
	_, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	_, err = pool.Exec(ctx, `CREATE TABLE customer(id BIGINT,trial_used BOOLEAN);
 CREATE TABLE customer_subscription(id BIGINT,customer_id BIGINT,panel_user_id BIGINT,panel_user_uuid UUID,subscription_link TEXT,is_primary BOOLEAN DEFAULT FALSE);
 CREATE TABLE purchase(id BIGINT,subscription_id BIGINT,status TEXT,month INT,days INT,plan_id TEXT,paid_at TIMESTAMPTZ,created_at TIMESTAMPTZ);`)
	if err != nil {
		t.Fatal(err)
	}
	var patches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			patches.Add(1)
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["trafficLimitBytes"] != float64(4321) {
				t.Errorf("wrong edited settings: %+v", body)
			}
		}
		if strings.HasSuffix(r.URL.Path, "/stream") {
			json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"users": []any{}, "hasMore": false}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"id": 18, "username": "panel-only", "status": "ACTIVE", "expireAt": "2028-01-01T00:00:00Z", "trafficLimitBytes": 4321, "trafficLimitStrategy": "MONTH"}})
	}))
	defer server.Close()
	h := &Handler{customerRepository: database.NewCustomerRepository(pool), remnawaveClient: remnawave.NewClient(server.URL, "token", "remote")}
	sess := &session{AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"users", "users.subscription"}}}
	body := `{"customerId":0,"panelId":18,"panelUuid":"00000000-0000-0000-0000-000000000000","settings":{"expireAt":"2028-01-01T00:00:00Z","trafficLimitBytes":4321,"trafficLimitStrategy":"MONTH"}}`
	w := httptest.NewRecorder()
	h.handlePanelUserSettings(w, httptest.NewRequest("POST", "/api/mini-app/admin/users/panel/settings", strings.NewReader(body)), sess, nil)
	if w.Code != 200 || patches.Load() != 1 || !strings.Contains(w.Body.String(), `"panelOnly":true`) {
		t.Fatalf("browser payload or direct save failed %d: %s, patches=%d", w.Code, w.Body.String(), patches.Load())
	}
	h.overviewPanel.mu.Lock()
	pending := h.overviewPanel.pending
	h.overviewPanel.mu.Unlock()
	if pending != nil {
		<-pending
	}
	// A failed ownership lookup must never become an unprotected panel write.
	pool.Exec(ctx, "DROP TABLE purchase")
	w = httptest.NewRecorder()
	h.handlePanelUserSettings(w, httptest.NewRequest("POST", "/api/mini-app/admin/users/panel/settings", strings.NewReader(body)), sess, nil)
	if w.Code != 503 || patches.Load() != 1 {
		t.Fatalf("binding lookup failure bypassed protected writes: %s", w.Body.String())
	}
}
