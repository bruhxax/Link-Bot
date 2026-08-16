package config

import (
	"net/url"
	"testing"
)

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

func TestVersionedMiniAppURL(t *testing.T) {
	got := versionedMiniAppURL("https://example.com/mini-app/?page=plans&v=old#section", "fresh")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse versioned URL: %v", err)
	}
	if parsed.Query().Get("page") != "plans" {
		t.Fatalf("page query = %q, want plans", parsed.Query().Get("page"))
	}
	if parsed.Query().Get("v") != "fresh" {
		t.Fatalf("v query = %q, want fresh", parsed.Query().Get("v"))
	}
	if parsed.Fragment != "section" {
		t.Fatalf("fragment = %q, want section", parsed.Fragment)
	}
}

func TestVersionedMiniAppURLKeepsEmptyAndInvalidValues(t *testing.T) {
	if got := versionedMiniAppURL("", "fresh"); got != "" {
		t.Fatalf("empty URL = %q, want empty", got)
	}
	if got := versionedMiniAppURL("https://example.com/mini-app/", ""); got != "https://example.com/mini-app/" {
		t.Fatalf("empty version changed URL to %q", got)
	}
}
