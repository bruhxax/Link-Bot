package config

import "testing"

func TestParseRemnawaveHeaders(t *testing.T) {
	headers := parseRemnawaveHeaders(
		"X-Test: value; X-Api-Key: old; invalid; Empty: ",
		"caddy-token",
	)

	if got := headers["X-Test"]; got != "value" {
		t.Fatalf("X-Test = %q, want value", got)
	}
	if got := headers["X-Api-Key"]; got != "caddy-token" {
		t.Fatalf("X-Api-Key = %q, want caddy-token", got)
	}
	if _, exists := headers["Empty"]; exists {
		t.Fatal("empty header value must be ignored")
	}
}
