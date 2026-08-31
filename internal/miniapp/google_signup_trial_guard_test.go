package miniapp

import (
	"net/http/httptest"
	"strings"
	"testing"

	"link-bot/internal/database"
)

func TestGoogleOnlyCustomerBypassesTelegramChannelCheck(t *testing.T) {
	h := &Handler{}
	customer := &database.Customer{TelegramID: 9000000000000000, TelegramIDIsSynthetic: true}
	if !h.shouldBypassRequiredChannelSubscription(customer) {
		t.Fatal("google-only customer must not require an unavailable Telegram channel check")
	}
}

func TestTrialBrowserDeviceHash(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/mini-app/trial/activate", nil)
	request.Header.Set("X-Client-Device-Fingerprint", strings.Repeat("a", 64))

	first, err := trialBrowserDeviceHash(request)
	if err != nil {
		t.Fatalf("trialBrowserDeviceHash() error = %v", err)
	}
	second, err := trialBrowserDeviceHash(request)
	if err != nil {
		t.Fatalf("trialBrowserDeviceHash() second error = %v", err)
	}
	if first == "" || first != second || first == strings.Repeat("a", 64) {
		t.Fatalf("fingerprint must be stable and server-side hashed: %q / %q", first, second)
	}
}

func TestTrialBrowserDeviceHashRejectsMalformedValue(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/mini-app/trial/activate", nil)
	request.Header.Set("X-Client-Device-Fingerprint", "too-short")
	if _, err := trialBrowserDeviceHash(request); err == nil {
		t.Fatal("expected malformed fingerprint to be rejected")
	}
}

func TestBrowserGoogleLoginCreatesDeviceFingerprint(t *testing.T) {
	raw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read embedded app.js: %v", err)
	}
	appJS := string(raw)

	required := []string{
		"X-Client-Device-Fingerprint",
		"getBrowserDeviceFingerprint",
		"trial_identity_used",
	}
	for _, fragment := range required {
		if !strings.Contains(appJS, fragment) {
			t.Fatalf("browser trial guard fragment is missing: %q", fragment)
		}
	}
	if strings.Contains(appJS, "google_not_linked") {
		t.Fatal("browser must not require pre-linking a Google account")
	}
}
