package miniapp

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEmailBrowserSessionRoundTripAndTamperRejection(t *testing.T) {
	const botToken = "123456:test-token"
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	previousTime := currentMiniAppTime
	currentMiniAppTime = func() time.Time { return now }
	defer func() { currentMiniAppTime = previousTime }()

	encoded, err := createBrowserSessionData(&session{Provider: sessionProviderEmail, Email: "User@Example.com", User: telegramUser{ID: 8999999999999999}}, botToken, now)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseAndValidateLoginData(encoded, botToken)
	if err != nil || parsed.Provider != sessionProviderEmail || parsed.Email != "user@example.com" || parsed.User.ID != 8999999999999999 {
		t.Fatalf("email session round trip failed: session=%+v error=%v", parsed, err)
	}
	if _, err := parseAndValidateLoginData(strings.Replace(encoded, "user%40example.com", "other%40example.com", 1), botToken); err == nil {
		t.Fatal("tampered email session was accepted")
	}
}

func TestEmailAuthValidationAndCodeBinding(t *testing.T) {
	for _, raw := range []string{"", "not-an-email", "Name <user@example.com>", "user@example.com\r\nX: bad"} {
		if _, err := normalizeAuthEmail(raw); err == nil {
			t.Fatalf("accepted invalid email %q", raw)
		}
	}
	if email, err := normalizeAuthEmail("  User@Example.com  "); err != nil || email != "user@example.com" {
		t.Fatalf("normalization failed: %q, %v", email, err)
	}
	if fiveDigitCode.MatchString("1234") || fiveDigitCode.MatchString("123456") || !fiveDigitCode.MatchString("00123") {
		t.Fatal("five-digit validation failed")
	}
	id := uuid.New()
	if emailCodeHash(id, "12345") == emailCodeHash(id, "12346") || emailCodeHash(id, "12345") == emailCodeHash(uuid.New(), "12345") {
		t.Fatal("code hash is not bound to code and challenge")
	}
}
