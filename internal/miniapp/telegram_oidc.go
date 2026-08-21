package miniapp

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	telegramOIDCIssuer   = "https://oauth.telegram.org"
	telegramOIDCCacheTTL = 6 * time.Hour
)

var (
	telegramOIDCJWKSURL = "https://oauth.telegram.org/.well-known/jwks.json"
	telegramOIDCClient  = &http.Client{Timeout: 5 * time.Second}
	telegramOIDCKeys    = struct {
		sync.RWMutex
		keys      map[string]*rsa.PublicKey
		expiresAt time.Time
	}{keys: map[string]*rsa.PublicKey{}}
)

type telegramOIDCIdentity struct {
	ID        int64
	Subject   string
	FirstName string
	LastName  string
	Username  string
	Picture   string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type telegramOIDCHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
}

type telegramOIDCJWKSet struct {
	Keys []telegramOIDCJWK `json:"keys"`
}

type telegramOIDCJWK struct {
	KeyID     string `json:"kid"`
	KeyType   string `json:"kty"`
	Algorithm string `json:"alg"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

func validateTelegramIDToken(ctx context.Context, idToken, clientID string) (*telegramOIDCIdentity, error) {
	idToken = strings.TrimSpace(idToken)
	clientID = strings.TrimSpace(clientID)
	if idToken == "" {
		return nil, fmt.Errorf("missing telegram id token")
	}
	if len(idToken) > 16<<10 {
		return nil, fmt.Errorf("telegram id token is too large")
	}
	if clientID == "" {
		return nil, fmt.Errorf("missing telegram client id")
	}

	parts := strings.Split(idToken, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, fmt.Errorf("invalid telegram id token format")
	}

	var header telegramOIDCHeader
	if err := decodeTelegramJWTPart(parts[0], &header); err != nil {
		return nil, fmt.Errorf("decode telegram id token header: %w", err)
	}
	if header.Algorithm != "RS256" || strings.TrimSpace(header.KeyID) == "" {
		return nil, fmt.Errorf("unsupported telegram id token signature")
	}

	publicKey, err := telegramOIDCPublicKey(ctx, header.KeyID)
	if err != nil {
		return nil, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode telegram id token signature: %w", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return nil, fmt.Errorf("telegram id token signature mismatch")
	}

	var claims map[string]any
	if err := decodeTelegramJWTPart(parts[1], &claims); err != nil {
		return nil, fmt.Errorf("decode telegram id token claims: %w", err)
	}
	if strings.TrimSpace(stringValue(claims["iss"])) != telegramOIDCIssuer {
		return nil, fmt.Errorf("telegram id token issuer mismatch")
	}
	if !telegramAudienceContains(claims["aud"], clientID) {
		return nil, fmt.Errorf("telegram id token audience mismatch")
	}

	issuedAtUnix, ok := int64Value(claims["iat"])
	if !ok || issuedAtUnix <= 0 {
		return nil, fmt.Errorf("telegram id token issue time missing")
	}
	expiresAtUnix, ok := int64Value(claims["exp"])
	if !ok || expiresAtUnix <= 0 {
		return nil, fmt.Errorf("telegram id token expiration missing")
	}
	issuedAt := time.Unix(issuedAtUnix, 0).UTC()
	expiresAt := time.Unix(expiresAtUnix, 0).UTC()
	now := currentMiniAppTime()
	if issuedAt.After(now.Add(telegramInitDataFutureSkew)) {
		return nil, fmt.Errorf("telegram id token was issued in the future")
	}
	if !expiresAt.After(now) {
		return nil, fmt.Errorf("telegram id token expired")
	}
	if !expiresAt.After(issuedAt) {
		return nil, fmt.Errorf("telegram id token has invalid lifetime")
	}

	userID, ok := int64Value(claims["id"])
	if !ok || userID <= 0 {
		return nil, fmt.Errorf("telegram id token user id missing")
	}
	subject := strings.TrimSpace(stringValue(claims["sub"]))
	if subject == "" {
		return nil, fmt.Errorf("telegram id token subject missing")
	}

	firstName := strings.TrimSpace(stringValue(claims["given_name"]))
	lastName := strings.TrimSpace(stringValue(claims["family_name"]))
	if firstName == "" {
		firstName = strings.TrimSpace(stringValue(claims["name"]))
	}

	return &telegramOIDCIdentity{
		ID:        userID,
		Subject:   subject,
		FirstName: firstName,
		LastName:  lastName,
		Username:  strings.TrimSpace(stringValue(claims["preferred_username"])),
		Picture:   strings.TrimSpace(stringValue(claims["picture"])),
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	}, nil
}

func decodeTelegramJWTPart(part string, target any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(decoded)))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func telegramAudienceContains(value any, expected string) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == expected
	case []any:
		for _, item := range typed {
			if strings.TrimSpace(stringValue(item)) == expected {
				return true
			}
		}
	}
	return false
}

func telegramOIDCPublicKey(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	now := currentMiniAppTime()
	telegramOIDCKeys.RLock()
	key := telegramOIDCKeys.keys[keyID]
	cacheValid := now.Before(telegramOIDCKeys.expiresAt)
	telegramOIDCKeys.RUnlock()
	if cacheValid {
		if key != nil {
			return key, nil
		}
		return nil, fmt.Errorf("telegram id token signing key not found")
	}

	if err := refreshTelegramOIDCKeys(ctx, now); err != nil {
		return nil, err
	}
	telegramOIDCKeys.RLock()
	key = telegramOIDCKeys.keys[keyID]
	telegramOIDCKeys.RUnlock()
	if key == nil {
		return nil, fmt.Errorf("telegram id token signing key not found")
	}
	return key, nil
}

func refreshTelegramOIDCKeys(ctx context.Context, now time.Time) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, telegramOIDCJWKSURL, nil)
	if err != nil {
		return fmt.Errorf("create telegram jwks request: %w", err)
	}
	resp, err := telegramOIDCClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch telegram jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("fetch telegram jwks: status %d", resp.StatusCode)
	}

	var set telegramOIDCJWKSet
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&set); err != nil {
		return fmt.Errorf("decode telegram jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, jwk := range set.Keys {
		if jwk.KeyType != "RSA" || jwk.Algorithm != "RS256" || strings.TrimSpace(jwk.KeyID) == "" {
			continue
		}
		publicKey, err := rsaPublicKeyFromTelegramJWK(jwk)
		if err == nil {
			keys[jwk.KeyID] = publicKey
		}
	}
	if len(keys) == 0 {
		return errors.New("telegram jwks contains no supported signing keys")
	}

	telegramOIDCKeys.Lock()
	telegramOIDCKeys.keys = keys
	telegramOIDCKeys.expiresAt = now.Add(telegramOIDCCacheTTL)
	telegramOIDCKeys.Unlock()
	return nil
}

func rsaPublicKeyFromTelegramJWK(jwk telegramOIDCJWK) (*rsa.PublicKey, error) {
	modulusBytes, err := base64.RawURLEncoding.DecodeString(jwk.Modulus)
	if err != nil || len(modulusBytes) == 0 {
		return nil, fmt.Errorf("invalid jwk modulus")
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(jwk.Exponent)
	if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
		return nil, fmt.Errorf("invalid jwk exponent")
	}
	exponent := 0
	for _, value := range exponentBytes {
		exponent = exponent<<8 | int(value)
	}
	if exponent < 3 {
		return nil, fmt.Errorf("invalid jwk exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(modulusBytes), E: exponent}, nil
}
