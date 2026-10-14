package devicequota

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"link-bot/internal/remnawave"
	"link-bot/internal/trafficpolicy"
)

func TestAdministratorQuotasKeepAllControlsExceptPersonalLimit(t *testing.T) {
	s, store, panel, settings := fixture(t)
	settings.value.Features = map[string]bool{"personal_device_profiles": true}
	const hwid = "admin-device-123"
	if w := request(s, store.b, hwid); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	slot := store.b.Slots[0]
	child := panel.users[slot.ChildID]
	child.UserTraffic.UsedTrafficBytes = 7 * gb
	originalUUID, originalURL := child.UUID, child.SubscriptionURL
	ctx := context.Background()
	cap := int64(1)
	if err := s.ControlDevice(ctx, 1, panel.users[1].UUID, hwid, "limit", &cap); !errors.Is(err, ErrAdminLimit) {
		t.Fatalf("admin limit writable: %v", err)
	}
	if child.TrafficLimitBytes != 20*gb {
		t.Fatal("administrator quota changed")
	}
	if err := s.ControlDevice(ctx, 1, panel.users[1].UUID, hwid, "pause", nil); err != nil {
		t.Fatal(err)
	}
	if child.Status != "DISABLED" || len(child.ActiveInternalSquads) != 0 || panel.drops != 1 || panel.revocations != 0 {
		t.Fatal("pause must disconnect without revocation")
	}
	if w := request(s, store.b, hwid); w.Code != 200 || w.Body.String() != "device-configuration" {
		t.Fatalf("paused client lost its cached profile: %d %s", w.Code, w.Body.String())
	}
	if err := s.ControlDevice(ctx, 1, panel.users[1].UUID, hwid, "resume", nil); err != nil {
		t.Fatal(err)
	}
	if child.Status != "ACTIVE" || child.UUID != originalUUID || child.SubscriptionURL != originalURL || child.UserTraffic.UsedTrafficBytes != 7*gb || panel.revocations != 0 {
		t.Fatal("resume replaced profile or counters")
	}
	devices, handled, err := s.DeleteDevice(ctx, 1, panel.users[1].UUID, hwid)
	if err != nil || !handled || len(devices) != 0 || panel.revocations != 1 {
		t.Fatalf("delete failed: %v", err)
	}
	if w := request(s, store.b, hwid); w.Code != 403 {
		t.Fatalf("deleted HWID returned: %d", w.Code)
	}
	state := &remnawave.UserState{}
	if err = s.Observe(ctx, panel.users[1], state); err != nil {
		t.Fatal(err)
	}
	if !state.DeviceTrafficManaged || state.DeviceTrafficUserManaged || len(state.BlockedDevices) != 1 {
		t.Fatalf("admin controls lost: %+v", state)
	}
	if err = s.ControlDevice(ctx, 1, panel.users[1].UUID, hwid, "unblock", nil); err != nil {
		t.Fatal(err)
	}
	if w := request(s, store.b, hwid); w.Code != 200 {
		t.Fatalf("unblocked HWID cannot return: %d %s", w.Code, w.Body.String())
	}
	if child.UserTraffic.UsedTrafficBytes != 7*gb {
		t.Fatal("unblock granted fresh traffic")
	}
}

func TestPersonalProfilesDefaultAndAdministratorOverride(t *testing.T) {
	s, store, panel, settings := fixture(t)
	// Frozen purchase has no administrator device quota, regardless of catalog.
	store.terms = &trafficpolicy.Terms{}
	settings.value.Features = map[string]bool{"personal_device_profiles": true}
	panel.users[1].HwidDeviceLimit = nil
	const hwid = "personal-device-123"
	if w := request(s, store.b, hwid); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if !store.b.Personal {
		t.Fatal("personal mode not persisted")
	}
	child := panel.users[store.b.Slots[0].ChildID]
	child.UserTraffic.UsedTrafficBytes = 4 * gb
	cap := int64(3)
	ctx := context.Background()
	if err := s.ControlDevice(ctx, 1, panel.users[1].UUID, hwid, "limit", &cap); err != nil {
		t.Fatal(err)
	}
	if child.TrafficLimitBytes != 3*gb || child.Status != "DISABLED" {
		t.Fatal("native personal cap not enforced")
	}
	cap = 0
	if err := s.ControlDevice(ctx, 1, panel.users[1].UUID, hwid, "limit", &cap); err != nil {
		t.Fatal(err)
	}
	if child.Status != "ACTIVE" || child.UserTraffic.UsedTrafficBytes != 4*gb {
		t.Fatal("cap removal reset usage")
	}
	settings.value.Features["personal_device_profiles"] = false
	state := &remnawave.UserState{}
	if err := s.Observe(ctx, panel.users[1], state); err != nil {
		t.Fatal(err)
	}
	if state.DeviceTrafficManaged || state.DeviceTrafficUserManaged || store.b.Isolated || store.b.Personal || child.Status != "DISABLED" || len(panel.users[1].ActiveInternalSquads) == 0 || state.SubscriptionLink == nil || *state.SubscriptionLink != panel.users[1].SubscriptionURL {
		t.Fatal("turning off default did not restore the original shared profile")
	}
	// A newly purchased administrator policy takes priority over personal values.
	store.terms = &trafficpolicy.Terms{Enabled: true, Enforced: true, Rules: []trafficpolicy.Rule{{Devices: 1, TrafficGB: 20}}}
	if err := s.Observe(ctx, panel.users[1], state); err != nil {
		t.Fatal(err)
	}
	if child.TrafficLimitBytes != 20*gb || state.DeviceTrafficUserManaged || state.Devices[0].CanSetTrafficLimit {
		t.Fatal("personal quota overrides administrator")
	}
	if err := s.ControlDevice(ctx, 1, panel.users[1].UUID, hwid, "limit", &cap); !errors.Is(err, ErrAdminLimit) {
		t.Fatal("API still permits personal limits")
	}
}

func TestPausedCacheIsBoundToHWIDCredentialsAndFormat(t *testing.T) {
	s, store, panel, _ := fixture(t)
	const hwid = "cached-device-123"
	if w := request(s, store.b, hwid); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if err := s.ControlDevice(context.Background(), 1, panel.users[1].UUID, hwid, "pause", nil); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", Route+store.b.Token.String(), nil)
	r.Header.Set("X-Hwid", hwid)
	r.Header.Set("User-Agent", "AnotherClient/1")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatal("served another client's cached format")
	}
	panel.users[store.b.Slots[0].ChildID].SubscriptionURL += "/rotated"
	if w := request(s, store.b, hwid); w.Code != 503 {
		t.Fatal("served credentials after revocation")
	}
}

func TestUnlimitedSlotAllocationAndFiniteCaps(t *testing.T) {
	b := &Binding{Slots: []*Slot{{Number: 1, Device: remnawave.UserDevice{Hwid: "one"}}, {Number: 2, Device: remnawave.UserDevice{Hwid: "two"}}}}
	if availableSlot(b, nil) != 3 {
		t.Fatal("unlimited tariff cannot add third device")
	}
	zero, one := 0, 1
	if availableSlot(b, &zero) != 3 || availableSlot(b, &one) != 0 {
		t.Fatal("wrong finite/unlimited cap")
	}
}
