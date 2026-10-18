package miniapp

import (
	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"testing"
	"time"
)

func TestBroadcastUnsubscribeAndOverrideDoNotBypassAudienceFilters(t *testing.T) {
	c := database.BroadcastCustomer{Unsubscribed: true, Language: "ru", Paid: true}
	for _, category := range []string{"all", "paid", "none", "trial_used"} {
		if matchesBroadcastCustomer(c, nil, database.BroadcastAudience{Category: category}, "", time.Now(), false) {
			t.Fatalf("unsubscribed user included in %s", category)
		}
	}
	a := database.BroadcastAudience{Category: "paid", IgnoreUnsubscribe: true}
	if !matchesBroadcastCustomer(c, nil, a, "", time.Now(), false) {
		t.Fatal("override did not include unsubscribed recipient")
	}
	a.Language = "en"
	if matchesBroadcastCustomer(c, nil, a, "", time.Now(), false) {
		t.Fatal("override bypassed language filter")
	}
	a.Language = ""
	a.Category = "active"
	if matchesBroadcastCustomer(c, nil, a, "", time.Now(), false) {
		t.Fatal("override bypassed subscription filter")
	}
}

func TestBroadcastAudienceUsesCurrentPanelState(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	tag := "FREE"
	c := database.BroadcastCustomer{TrialUsed: true, Language: "ru", ExpireAt: &past}
	users := []remnawave.PanelUser{{Status: "EXPIRED", ExpireAt: past}, {Status: "ACTIVE", ExpireAt: future, Tag: &tag}}
	for _, tc := range []struct {
		category string
		want     bool
	}{{"active", true}, {"none", false}, {"expired", false}, {"trial", true}, {"trial_used", true}, {"paid", false}} {
		if got := matchesBroadcastCustomer(c, users, database.BroadcastAudience{Category: tc.category}, "FREE", now, true); got != tc.want {
			t.Fatalf("%s: %v", tc.category, got)
		}
	}
	users[1].Status = "DISABLED"
	if matchesBroadcastCustomer(c, users, database.BroadcastAudience{Category: "active"}, "FREE", now, true) {
		t.Fatal("disabled panel user counted as active")
	}
	if !matchesBroadcastCustomer(c, users, database.BroadcastAudience{Category: "none"}, "FREE", now, true) {
		t.Fatal("disabled account should have no active subscription")
	}
	users[1].Status = "ACTIVE"
	users[1].ExpireAt = past
	if matchesBroadcastCustomer(c, users, database.BroadcastAudience{Category: "trial"}, "FREE", now, true) {
		t.Fatal("expired ACTIVE trial included")
	}
	users[1].ExpireAt = future
	paidTag := "PREMIUM"
	users[1].Tag = &paidTag
	if matchesBroadcastCustomer(c, users, database.BroadcastAudience{Category: "trial"}, "FREE", now, true) {
		t.Fatal("changed trial tag ignored")
	}
}

func TestBroadcastAudienceCombinesScopesAndLanguage(t *testing.T) {
	now := time.Now()
	a, b, external := uuid.New(), uuid.New(), uuid.New()
	tag := "PREMIUM"
	c := database.BroadcastCustomer{Language: "ru", Paid: true}
	u := remnawave.PanelUser{Status: "ACTIVE", ExpireAt: now.Add(time.Hour), Tag: &tag, ActiveInternalSquads: []remnawave.PanelSquad{{UUID: a}}, ExternalSquadUUID: &external}
	audience := database.BroadcastAudience{Category: "active", InternalSquads: []string{b.String(), a.String()}, ExternalSquads: []string{external.String()}, Tag: "premium", Language: "ru"}
	if !matchesBroadcastCustomer(c, []remnawave.PanelUser{u}, audience, "FREE", now, true) {
		t.Fatal("valid combined selection rejected")
	}
	audience.InternalSquads = []string{b.String()}
	if matchesBroadcastCustomer(c, []remnawave.PanelUser{u}, audience, "FREE", now, true) {
		t.Fatal("unrelated squad included")
	}
	audience.InternalSquads = nil
	audience.Language = "en"
	if matchesBroadcastCustomer(c, []remnawave.PanelUser{u}, audience, "FREE", now, true) {
		t.Fatal("language ignored")
	}
	audience.Language = "ru"
	audience.ExternalSquads = []string{b.String()}
	if matchesBroadcastCustomer(c, []remnawave.PanelUser{u}, audience, "FREE", now, true) {
		t.Fatal("external squad ignored")
	}
}

func TestBroadcastAudienceFallbackIncludesSecondarySubscriptions(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	c := database.BroadcastCustomer{ExpireAt: &past, Subscriptions: []database.BroadcastSubscription{{ExpireAt: &future}}, TrialUsed: true}
	if !matchesBroadcastCustomer(c, nil, database.BroadcastAudience{Category: "active"}, "FREE", now, false) {
		t.Fatal("secondary active subscription lost")
	}
	if matchesBroadcastCustomer(c, nil, database.BroadcastAudience{Category: "expired"}, "FREE", now, false) {
		t.Fatal("active multi-subscription account marked expired")
	}
	if matchesBroadcastCustomer(c, nil, database.BroadcastAudience{Category: "active"}, "FREE", now, true) {
		t.Fatal("stale local expiry used when panel configured")
	}
}
