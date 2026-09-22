package miniapp

import (
	"strings"
	"testing"

	"link-bot/internal/config"
)

func TestPartnerInviteURLUsesSeparateMiniAppStartParameter(t *testing.T) {
	previous := config.BotURL()
	config.SetBotURL("https://t.me/link_bot/")
	defer config.SetBotURL(previous)

	inviteURL := partnerInviteURL("a1b2c3d4")
	if inviteURL != "https://t.me/link_bot?startapp=partner_A1B2C3D4" {
		t.Fatalf("partnerInviteURL() = %q", inviteURL)
	}
	if strings.Contains(inviteURL, "ref_") {
		t.Fatalf("partner invite must not use the regular referral parameter: %q", inviteURL)
	}
}
