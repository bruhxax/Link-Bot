package miniapp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateTelegramIDToken(t *testing.T) {
	now := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	key := mustTelegramOIDCTestKey(t)
	server := telegramOIDCTestJWKSServer(t, "login-key", &key.PublicKey)
	configureTelegramOIDCTest(t, server, now)

	token := signTelegramOIDCTestToken(t, key, "login-key", map[string]any{
		"iss":                telegramOIDCIssuer,
		"aud":                "8521897198",
		"sub":                "telegram-user-6402520205",
		"iat":                now.Add(-time.Minute).Unix(),
		"exp":                now.Add(time.Hour).Unix(),
		"id":                 int64(6402520205),
		"name":               "Maks Link",
		"given_name":         "Maks",
		"family_name":        "Link",
		"preferred_username": "maks",
		"picture":            "https://cdn4.telesco.pe/avatar.jpg",
	})

	identity, err := validateTelegramIDToken(context.Background(), token, "8521897198")
	if err != nil {
		t.Fatalf("validateTelegramIDToken returned error: %v", err)
	}
	if identity.ID != 6402520205 || identity.FirstName != "Maks" || identity.LastName != "Link" || identity.Username != "maks" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
	if !identity.IssuedAt.Equal(now.Add(-time.Minute)) || !identity.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected token times: %+v", identity)
	}
}

func TestValidateTelegramIDTokenRejectsInvalidClaimsAndSignature(t *testing.T) {
	now := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	key := mustTelegramOIDCTestKey(t)
	server := telegramOIDCTestJWKSServer(t, "login-key", &key.PublicKey)
	configureTelegramOIDCTest(t, server, now)

	baseClaims := map[string]any{
		"iss": telegramOIDCIssuer,
		"aud": "8521897198",
		"sub": "telegram-user-6402520205",
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(time.Hour).Unix(),
		"id":  int64(6402520205),
	}

	wrongAudience := cloneTelegramOIDCTestClaims(baseClaims)
	wrongAudience["aud"] = "9999999999"
	if _, err := validateTelegramIDToken(context.Background(), signTelegramOIDCTestToken(t, key, "login-key", wrongAudience), "8521897198"); err == nil {
		t.Fatal("expected audience mismatch")
	}

	expired := cloneTelegramOIDCTestClaims(baseClaims)
	expired["exp"] = now.Add(-time.Second).Unix()
	if _, err := validateTelegramIDToken(context.Background(), signTelegramOIDCTestToken(t, key, "login-key", expired), "8521897198"); err == nil {
		t.Fatal("expected expired token error")
	}

	otherKey := mustTelegramOIDCTestKey(t)
	if _, err := validateTelegramIDToken(context.Background(), signTelegramOIDCTestToken(t, otherKey, "login-key", baseClaims), "8521897198"); err == nil {
		t.Fatal("expected signature mismatch")
	}
}

func configureTelegramOIDCTest(t *testing.T, server *httptest.Server, now time.Time) {
	t.Helper()
	originalURL := telegramOIDCJWKSURL
	originalClient := telegramOIDCClient
	originalTime := currentMiniAppTime
	telegramOIDCJWKSURL = server.URL
	telegramOIDCClient = server.Client()
	currentMiniAppTime = func() time.Time { return now }
	telegramOIDCKeys.Lock()
	telegramOIDCKeys.keys = map[string]*rsa.PublicKey{}
	telegramOIDCKeys.expiresAt = time.Time{}
	telegramOIDCKeys.Unlock()
	t.Cleanup(func() {
		telegramOIDCJWKSURL = originalURL
		telegramOIDCClient = originalClient
		currentMiniAppTime = originalTime
		telegramOIDCKeys.Lock()
		telegramOIDCKeys.keys = map[string]*rsa.PublicKey{}
		telegramOIDCKeys.expiresAt = time.Time{}
		telegramOIDCKeys.Unlock()
		server.Close()
	})
}

func mustTelegramOIDCTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

func telegramOIDCTestJWKSServer(t *testing.T, keyID string, key *rsa.PublicKey) *httptest.Server {
	t.Helper()
	modulus := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	exponent := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kid": keyID,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   modulus,
				"e":   exponent,
			}},
		})
	}))
}

func signTelegramOIDCTestToken(t *testing.T, key *rsa.PrivateKey, keyID string, claims map[string]any) string {
	t.Helper()
	headerJSON, err := json.Marshal(map[string]any{"alg": "RS256", "kid": keyID, "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal JWT header: %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal JWT claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := header + "." + payload
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func cloneTelegramOIDCTestClaims(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
