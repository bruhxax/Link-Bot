package remnawave

import (
	"testing"

	"github.com/google/uuid"
)

func TestFindUserByIDOrUsername(t *testing.T) {
	firstUUID := uuid.New()
	secondUUID := uuid.New()
	users := []PanelUser{
		{UUID: firstUUID, ID: 1281, Username: "10204_1404001393"},
		{UUID: secondUUID, ID: 1282, Username: "Manual_Subscription"},
	}

	byID := findPanelUserByIDOrUsername(users, "1281")
	if byID == nil || byID.UUID != firstUUID {
		t.Fatalf("expected panel ID lookup to find %s", firstUUID)
	}

	byUsername := findPanelUserByIDOrUsername(users, "manual_subscription")
	if byUsername == nil || byUsername.UUID != secondUUID {
		t.Fatalf("expected case-insensitive username lookup to find %s", secondUUID)
	}

	if found := findPanelUserByIDOrUsername(users, "missing"); found != nil {
		t.Fatalf("expected missing subscription lookup to return nil")
	}
}

func TestFindPanelUserBySubscriptionLink(t *testing.T) {
	t.Parallel()

	userUUID := uuid.New()
	users := []PanelUser{{ID: 1281, UUID: userUUID, SubscriptionURL: " https://example.test/sub/one "}}
	got := findPanelUserBySubscriptionLink(users, "https://example.test/sub/one")
	if got == nil || got.UUID != userUUID {
		t.Fatalf("expected subscription URL lookup to find %s", userUUID)
	}
	if got := findPanelUserBySubscriptionLink(users, ""); got != nil {
		t.Fatal("empty subscription URL must not match")
	}
}

func TestFormatTelegramDescription(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		username    string
		want        string
	}{
		{name: "complete", displayName: "Link User", username: "exampleuser", want: "Link User | @exampleuser"},
		{name: "username with at", displayName: "Link", username: "@exampleuser", want: "Link | @exampleuser"},
		{name: "missing name", username: "exampleuser", want: "- | @exampleuser"},
		{name: "missing username", displayName: "Link", want: "Link | -"},
		{name: "missing both", want: "- | -"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatTelegramDescription(tt.displayName, tt.username); got != tt.want {
				t.Fatalf("FormatTelegramDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}
