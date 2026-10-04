package miniapp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
)

func TestAIAccountSnapshotsIncludeFactsWithoutCredentials(t *testing.T) {
	expiry := time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)
	secretLink := "https://vpn.example/sub/private-token"
	sub := database.CustomerSubscription{DisplayName: "Годовая", ExpireAt: &expiry, SubscriptionLink: &secretLink, IsPrimary: true}
	panel := &remnawave.UserState{Exists: true, Active: true, ExpireAt: &expiry, UsedTrafficBytes: 1234, TrafficLimitBytes: 9999, UsedDevices: 1, DeviceLimit: 3, DevicesLoaded: true, Devices: []remnawave.UserDevice{{Platform: "iOS", DeviceModel: "iPhone", Hwid: "private-device-id"}}}
	data := aiSubscriptionSnapshot(sub, panel)
	if data["deviceLimit"] != 3 || data["active"] != true || data["hasImportLink"] != true {
		t.Fatalf("wrong subscription facts %+v", data)
	}
	raw, _ := json.Marshal(data)
	if !strings.Contains(string(raw), "2026-12-15") || !strings.Contains(string(raw), "iPhone") {
		t.Fatal("expiry/devices missing")
	}
	if strings.Contains(string(raw), "private-token") || strings.Contains(string(raw), "private-device-id") {
		t.Fatal("credentials sent to provider")
	}
	unavailable := aiSubscriptionSnapshot(sub, nil)
	if unavailable["panelAvailable"] != false {
		t.Fatal("failed panel fetch treated as known")
	}
	if _, ok := unavailable["usedDevices"]; ok {
		t.Fatal("invented zero devices")
	}
	panel.DevicesLoaded = false
	panel.ExpireAt = nil
	unconfirmed := aiSubscriptionSnapshot(sub, panel)
	if _, ok := unconfirmed["usedDevices"]; ok {
		t.Fatal("failed device endpoint treated as zero devices")
	}
	if unconfirmed["expiresAt"] != sub.ExpireAt {
		t.Fatal("missing panel expiry erased known saved expiry")
	}
	planID := "year"
	checkout := "https://payment.example/private"
	purchase := database.Purchase{PlanID: &planID, Amount: 500, Status: database.PurchaseStatusPaid, YookasaURL: &checkout}
	raw, _ = json.Marshal(aiPurchaseSnapshot(purchase, []runtimeconfig.PlanSettings{{ID: planID, Name: "Годовая"}}))
	if !strings.Contains(string(raw), "Годовая") || !strings.Contains(string(raw), "paid") || strings.Contains(string(raw), checkout) {
		t.Fatal("payment snapshot wrong or leaked checkout")
	}
}

func TestHistoricalTextRedactionPreservesDates(t *testing.T) {
	expiry := time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)
	item := map[string]any{"createdAt": expiry, "subject": "Happ на ПК", "messages": []map[string]any{{"role": "customer", "text": "https://private.example/secret sk_live_private12345"}}}
	scrubHistoricalText(item)
	raw, _ := json.Marshal(item)
	if strings.Contains(string(raw), "private12345") || strings.Contains(string(raw), "private.example") {
		t.Fatal("historical secrets leaked")
	}
	if !strings.Contains(string(raw), "2026-12-15") {
		t.Fatal("known dates redacted")
	}
}
