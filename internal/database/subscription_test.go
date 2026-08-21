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

func TestFirstFreeSubscriptionPosition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		positions []int
		want      int
		ok        bool
	}{
		{name: "first additional slot", positions: []int{1}, want: 2, ok: true},
		{name: "gap is reused", positions: []int{1, 3}, want: 2, ok: true},
		{name: "limit reached", positions: []int{1, 2, 3}, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := firstFreeSubscriptionPosition(tt.positions)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("firstFreeSubscriptionPosition() = (%d, %t), want (%d, %t)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestTransferSubscriptionDisplayName(t *testing.T) {
	t.Parallel()

	if got := transferSubscriptionDisplayName("  Home   VPN  ", 2); got != "Home VPN" {
		t.Fatalf("transferSubscriptionDisplayName() = %q", got)
	}
	if got := transferSubscriptionDisplayName("", 3); got != "Подписка 3" {
		t.Fatalf("empty transfer name = %q", got)
	}
	if got := transferSubscriptionDisplayName(strings.Repeat("я", 45), 2); len([]rune(got)) != 40 {
		t.Fatalf("long transfer name has %d runes", len([]rune(got)))
	}
}
