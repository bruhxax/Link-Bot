package miniapp

import (
	"os"
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

func TestPartnerPageIsOnlyVisibleWhenActive(t *testing.T) {
	styles, err := os.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read partner styles: %v", err)
	}
	css := string(styles)
	if !strings.Contains(css, ".page.partner-page { display: none; }") {
		t.Fatal("inactive partner page must remain hidden")
	}
	if !strings.Contains(css, ".page.partner-page.active { display: grid;") {
		t.Fatal("active partner page must use its grid layout")
	}
}

func TestPartnerFrontendUsesReferralLayoutAndExpandableAdminEntries(t *testing.T) {
	script, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read partner script: %v", err)
	}
	frontend := string(script)
	for _, expected := range []string{
		"referral-metrics partner-referral-metrics",
		"referral-invite-card partner-invite-card",
		"<details class=\"admin-partner-entry",
		"admin-partner-entry__details",
	} {
		if !strings.Contains(frontend, expected) {
			t.Fatalf("partner frontend must include %q", expected)
		}
	}
}
