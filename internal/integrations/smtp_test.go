package integrations

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	"link-bot/internal/database"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSMTPValidation(t *testing.T) {
	valid := map[string]string{"host": "smtp.example.com", "port": "587", "user": "sender", "password": "secret", "from": "Link-Bot <sender@example.com>"}
	if err := ValidateSMTP(valid, true); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, value string }{{"port", "0"}, {"port", "65536"}, {"host", "https://smtp.example.com"}, {"from", "not an email"}, {"from", "sender@example.com\r\nBcc: stolen@example.com"}, {"password", ""}, {"proxyUrl", "socks5://proxy.example.com"}} {
		fields := make(map[string]string)
		for k, v := range valid {
			fields[k] = v
		}
		fields[tc.key] = tc.value
		if err := ValidateSMTP(fields, true); err == nil {
			t.Errorf("invalid %s accepted", tc.key)
		}
	}
}

func TestSMTPEncryptedPersistenceAndSecretReplacement(t *testing.T) {
	dsn := os.Getenv("SUPPORT_AI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated test database")
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
	schema := fmt.Sprintf("smtp_test_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	if _, err = pool.Exec(ctx, `CREATE TABLE payment_integration(provider TEXT PRIMARY KEY,enabled BOOLEAN,encrypted_config TEXT,webhook_token TEXT,updated_by BIGINT,updated_at TIMESTAMPTZ)`); err != nil {
		t.Fatal(err)
	}
	block, _ := aes.NewCipher(make([]byte, 32))
	aead, _ := cipher.NewGCM(block)
	repo := database.NewPaymentIntegrationRepository(pool)
	s := &Service{repository: repo, aead: aead, records: map[string]record{}}
	fields := map[string]string{"host": "smtp.example.com", "port": "587", "user": "sender", "password": "secret-test-password", "from": "sender@example.com", "proxyUrl": "https://user:private-proxy-password@proxy.example.com"}
	view, err := s.Update(ctx, ProviderSMTP, UpdateInput{Enabled: true, Fields: fields}, 99)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), "secret-test-password") || strings.Contains(string(raw), "private-proxy-password") {
		t.Fatal("catalog exposed SMTP credentials")
	}
	stored, err := repo.Find(ctx, ProviderSMTP)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.EncryptedConfig, "secret-test-password") || strings.Contains(stored.EncryptedConfig, "private-proxy-password") {
		t.Fatal("credentials stored unencrypted")
	}
	if _, err = s.Update(ctx, ProviderSMTP, UpdateInput{Enabled: false, Fields: map[string]string{"password": "", "proxyUrl": ""}}, 99); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.Find(ctx, ProviderSMTP)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := s.decrypt(stored.EncryptedConfig)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Enabled || decoded["password"] != "secret-test-password" || decoded["proxyUrl"] != "" {
		t.Fatal("disable, blank password retention or proxy removal failed")
	}
	if _, err = s.Update(ctx, ProviderSMTP, UpdateInput{Enabled: true, Fields: map[string]string{"port": "-1"}}, 99); err == nil {
		t.Fatal("invalid update accepted")
	}
	active, enabled := s.SMTPSettings()
	if enabled || active["port"] != "587" {
		t.Fatal("failed update mutated active settings")
	}
}
