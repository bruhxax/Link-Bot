package miniapp

import (
	"link-bot/internal/database"
	"link-bot/internal/trafficpolicy"
	"testing"

	planbook "link-bot/internal/plans"
	"link-bot/internal/runtimeconfig"
)

func TestSubscriptionDeviceTrafficUsesExactPurchasedPlan(t *testing.T) {
	plans := []runtimeconfig.PlanSettings{
		{ID: "first", Enabled: true, Months: 1, DeviceTrafficEnabled: true, DeviceTrafficRules: []planbook.DeviceTrafficRule{{Devices: 1, TrafficGB: 20}}},
		{ID: "second", Enabled: false, Months: 1, DeviceTrafficEnabled: true, DeviceTrafficRules: []planbook.DeviceTrafficRule{{Devices: 2, TrafficGB: 40}}},
		{ID: "hidden", DeviceTrafficRules: []planbook.DeviceTrafficRule{{Devices: 3, TrafficGB: 60}}},
	}
	got := subscriptionDeviceTrafficRules(plans, "second")
	if len(got) != 1 || got[0].Devices != 2 || got[0].TrafficGB != 40 {
		t.Fatalf("wrong plan annotations: %+v", got)
	}
	got[0].TrafficGB = 999
	if plans[1].DeviceTrafficRules[0].TrafficGB != 40 {
		t.Fatal("result mutates settings")
	}
	for _, id := range []string{"", "missing", "hidden"} {
		if len(subscriptionDeviceTrafficRules(plans, id)) != 0 {
			t.Fatalf("unexpected annotations for %q", id)
		}
	}
}

func TestPurchasedDeviceTrafficPayloadUsesImmutableTerms(t *testing.T) {
	h := &Handler{}
	purchase := &database.Purchase{DeviceTrafficTerms: &trafficpolicy.Terms{Enabled: true, Enforced: true, Rules: []trafficpolicy.Rule{{Devices: 1, TrafficGB: 20}}}}
	payload := h.buildSubscriptionPayload(&database.Customer{}, purchase, nil)
	if !payload.DeviceTrafficEnforced || len(payload.DeviceTrafficRules) != 1 || payload.DeviceTrafficRules[0].TrafficGB != 20 {
		t.Fatalf("purchased terms disappeared without catalog plan: %+v", payload)
	}
	payload.DeviceTrafficRules[0].TrafficGB = 999
	if purchase.DeviceTrafficTerms.Rules[0].TrafficGB != 20 {
		t.Fatal("payload mutated purchased terms")
	}
	purchase.DeviceTrafficTerms = &trafficpolicy.Terms{}
	payload = h.buildSubscriptionPayload(&database.Customer{}, purchase, nil)
	if payload.DeviceTrafficEnforced || len(payload.DeviceTrafficRules) != 0 {
		t.Fatal("disabled purchased terms became active")
	}
}
