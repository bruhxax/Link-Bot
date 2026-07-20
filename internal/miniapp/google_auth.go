package miniapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	errGoogleAuthNotConfigured = errors.New("google auth is not configured")
	errGoogleAuthNotLinked     = errors.New("google account is not linked")
	errGoogleAlreadyLinked     = errors.New("google account is already linked")
)

type googleIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
	Nonce         string
	ExpiresAt     time.Time
}

func validateGoogleIDToken(ctx context.Context, idToken, clientID string) (*googleIdentity, error) {
	idToken = strings.TrimSpace(idToken)
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, errGoogleAuthNotConfigured
	}
	if idToken == "" {
		return nil, fmt.Errorf("missing google id token")
	}

	endpoint := "https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(idToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google token check failed: %w", err)
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()

	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode google token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google token rejected: %s", stringValue(payload["error_description"]))
	}

	if aud := stringValue(payload["aud"]); aud != clientID {
		return nil, fmt.Errorf("google token audience mismatch")
	}

	exp, ok := int64Value(payload["exp"])
	if !ok || exp <= 0 {
		return nil, fmt.Errorf("google token expiration missing")
	}
	expiresAt := time.Unix(exp, 0).UTC()
	if !expiresAt.After(time.Now().UTC()) {
		return nil, fmt.Errorf("google token expired")
	}

	identity := &googleIdentity{
		Subject:       strings.TrimSpace(stringValue(payload["sub"])),
		Email:         strings.ToLower(strings.TrimSpace(stringValue(payload["email"]))),
		EmailVerified: boolValue(payload["email_verified"]),
		Name:          strings.TrimSpace(stringValue(payload["name"])),
		Picture:       strings.TrimSpace(stringValue(payload["picture"])),
		Nonce:         strings.TrimSpace(stringValue(payload["nonce"])),
		ExpiresAt:     expiresAt,
	}
	if identity.Subject == "" {
		return nil, fmt.Errorf("google token subject missing")
	}
	if identity.Email == "" || !identity.EmailVerified {
		return nil, fmt.Errorf("google email is not verified")
	}

	return identity, nil
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func int64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case json.Number:
		result, err := typed.Int64()
		return result, err == nil
	case string:
		var result int64
		_, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &result)
		return result, err == nil
	default:
		return 0, false
	}
}
