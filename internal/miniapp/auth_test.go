package miniapp

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseAndValidateLoginData(t *testing.T) {
	const botToken = "123456:test-token"
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	originalTime := currentMiniAppTime
	currentMiniAppTime = func() time.Time { return now }
	defer func() { currentMiniAppTime = originalTime }()

	values := url.Values{}
	values.Set("id", "6402520205")
	values.Set("first_name", "Maks")
	values.Set("username", "maks")
	values.Set("auth_date", strconv.FormatInt(now.Add(-time.Minute).Unix(), 10))
	values.Set("hash", signTelegramLoginValues(values, botToken))

	sess, err := parseAndValidateLoginData(values.Encode(), botToken)
	if err != nil {
		t.Fatalf("parseAndValidateLoginData returned error: %v", err)
	}
	if sess.User.ID != 6402520205 {
		t.Fatalf("user id = %d, want 6402520205", sess.User.ID)
	}
	if sess.User.Username != "maks" {
		t.Fatalf("username = %q, want maks", sess.User.Username)
	}
}

func TestParseAndValidateLoginDataRejectsBadHash(t *testing.T) {
	values := url.Values{}
	values.Set("id", "6402520205")
	values.Set("first_name", "Maks")
	values.Set("auth_date", strconv.FormatInt(time.Now().UTC().Unix(), 10))
	values.Set("hash", strings.Repeat("0", 64))

	if _, err := parseAndValidateLoginData(values.Encode(), "123456:test-token"); err == nil {
		t.Fatal("expected bad hash error")
	}
}

func signTelegramLoginValues(values url.Values, botToken string) string {
	var pairs []string
	for key, items := range values {
		if key == "hash" || len(items) == 0 {
			continue
		}
		pairs = append(pairs, key+"="+items[0])
	}
	sort.Strings(pairs)
	secret := sha256.Sum256([]byte(botToken))
	return hex.EncodeToString(hmacSHA256(secret[:], []byte(strings.Join(pairs, "\n"))))
}
