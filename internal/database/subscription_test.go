package database

import (
	"strings"
	"testing"
)

func TestNormalizeSubscriptionDisplayName(t *testing.T) {
	t.Parallel()

	got, err := NormalizeSubscriptionDisplayName("  Домашний   VPN  ")
	if err != nil {
		t.Fatalf("NormalizeSubscriptionDisplayName() error = %v", err)
	}
	if got != "Домашний VPN" {
		t.Fatalf("NormalizeSubscriptionDisplayName() = %q", got)
	}
	if _, err := NormalizeSubscriptionDisplayName(""); err == nil {
		t.Fatal("empty subscription name must fail")
	}
	if _, err := NormalizeSubscriptionDisplayName(strings.Repeat("я", 41)); err == nil {
		t.Fatal("subscription name longer than 40 runes must fail")
	}
}
