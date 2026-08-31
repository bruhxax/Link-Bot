package miniapp

import (
	"net/url"
	"strings"
	"testing"

	"link-bot/internal/config"
)

func TestReferralInviteURLStartsBotWithReferrer(t *testing.T) {
	previousBotURL := config.BotURL()
	config.SetBotURL("https://t.me/link_bot/")
	defer config.SetBotURL(previousBotURL)

	inviteURL := buildReferralInviteURL(123456789, true)
	if inviteURL != "https://t.me/link_bot?start=ref_123456789" {
		t.Fatalf("buildReferralInviteURL() = %q", inviteURL)
	}

	shareURL := buildReferralShareURL(123456789, true)
	if !strings.Contains(shareURL, "url="+url.QueryEscape(inviteURL)) {
		t.Fatalf("share URL does not contain direct referral target: %q", shareURL)
	}
}

func TestReferralInviteURLDisabled(t *testing.T) {
	previousBotURL := config.BotURL()
	config.SetBotURL("https://t.me/link_bot")
	defer config.SetBotURL(previousBotURL)

	if got := buildReferralInviteURL(123, false); got != "" {
		t.Fatalf("disabled referral invite URL = %q", got)
	}
	if got := buildReferralInviteURL(0, true); got != "" {
		t.Fatalf("invalid referral invite URL = %q", got)
	}
}
