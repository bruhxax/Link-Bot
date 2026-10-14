package runtimeconfig

import (
	"encoding/json"
	"reflect"
	"testing"

	planbook "link-bot/internal/plans"
)

func TestPersonalDeviceProfilesOptInSurvivesNormalization(t *testing.T) {
	settings := DefaultSettings()
	if settings.Features["personal_device_profiles"] {
		t.Fatal("existing subscriptions must not be converted by default")
	}
	settings.Features["personal_device_profiles"] = true
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatal(err)
	}
	if !settings.Features["personal_device_profiles"] {
		t.Fatal("feature toggle lost")
	}
}

func TestDeviceTrafficAnnotationsRoundTripWithoutChangingLimits(t *testing.T) {
	settings := DefaultSettings()
	settings.Plans = []PlanSettings{{ID: "annotated", Enabled: true, Months: 1, PriceRub: 100, TrafficGB: 100, DeviceLimit: 5,
		DeviceTrafficEnabled: true, DeviceTrafficRules: []planbook.DeviceTrafficRule{{Devices: 2, TrafficGB: 40}, {Devices: 1, TrafficGB: 20}}}}
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	var restored Settings
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	service := &Service{}
	service.value.Store(restored)
	plan := service.CheckoutPlans()[0]
	if plan.TrafficLimitBytes != 100*gibibyte || plan.DeviceLimitCount != 5 || plan.PriceRub != 100 {
		t.Fatalf("annotations changed checkout: %+v", plan)
	}
	want := []planbook.DeviceTrafficRule{{Devices: 1, TrafficGB: 20}, {Devices: 2, TrafficGB: 40}}
	if !reflect.DeepEqual(plan.DeviceTrafficRules, want) {
		t.Fatalf("rules = %+v", plan.DeviceTrafficRules)
	}
	plan.DeviceTrafficRules[0].TrafficGB = 999
	if service.CheckoutPlans()[0].DeviceTrafficRules[0].TrafficGB != 20 {
		t.Fatal("checkout leaked mutable settings")
	}
	restored.Plans[0].DeviceTrafficEnabled = false
	service.value.Store(restored)
	if len(service.CheckoutPlans()[0].DeviceTrafficRules) != 0 {
		t.Fatal("disabled annotations exposed to customers")
	}
}

func TestDeviceTrafficRejectsInvalidAndDuplicateRows(t *testing.T) {
	for _, rules := range [][]planbook.DeviceTrafficRule{
		nil, {{Devices: 0, TrafficGB: 20}}, {{Devices: 1001, TrafficGB: 20}},
		{{Devices: 1, TrafficGB: 0}}, {{Devices: 1, TrafficGB: -1}}, {{Devices: 1, TrafficGB: 1000001}},
		{{Devices: 1, TrafficGB: 20}, {Devices: 1, TrafficGB: 40}}, make([]planbook.DeviceTrafficRule, 101),
	} {
		settings := DefaultSettings()
		settings.Plans = []PlanSettings{{ID: "test", Enabled: true, Months: 1, DeviceTrafficEnabled: true, DeviceTrafficRules: rules}}
		if err := NormalizeAndValidate(&settings); err == nil {
			t.Fatalf("accepted invalid rows: %+v", rules)
		}
	}
}

func TestDeviceTrafficEnforcementRequiresFiniteDeviceSlots(t *testing.T) {
	for _, plan := range []PlanSettings{
		{DeviceTrafficEnforced: true, DeviceLimit: 2},
		{DeviceTrafficEnforced: true, DeviceTrafficEnabled: true, DeviceTrafficRules: []planbook.DeviceTrafficRule{{Devices: 1, TrafficGB: 20}}},
		{DeviceTrafficEnforced: true, DeviceTrafficEnabled: true, DeviceLimit: 1, DeviceTrafficRules: []planbook.DeviceTrafficRule{{Devices: 2, TrafficGB: 40}}},
	} {
		if err := validateDeviceTraffic(&plan); err == nil {
			t.Fatalf("accepted ambiguous enforcement: %+v", plan)
		}
	}
	plan := PlanSettings{DeviceTrafficEnforced: true, DeviceTrafficEnabled: true, DeviceLimit: 2, DeviceTrafficRules: []planbook.DeviceTrafficRule{{Devices: 1, TrafficGB: 20}, {Devices: 2, TrafficGB: 40}}}
	if err := validateDeviceTraffic(&plan); err != nil {
		t.Fatal(err)
	}
}
