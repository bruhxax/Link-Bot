package devicequota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"link-bot/internal/remnawave"
	"link-bot/internal/trafficpolicy"
)

func TestDefaultOptOutRestoresNativeURLAndKeepsOldGatewayWorking(t *testing.T) {
	s, store, panel, settings := fixture(t)
	store.terms = &trafficpolicy.Terms{}
	settings.value.Features = map[string]bool{"personal_device_profiles": true}
	const hwid = "rollback-device-123"
	if w := request(s, store.b, hwid); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	child := panel.users[store.b.Slots[0].ChildID]
	child.UserTraffic.UsedTrafficBytes = 7 * gb
	parent := panel.users[1]
	parent.UserTraffic.UsedTrafficBytes = 11 * gb
	original := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("original-profile")) }))
	defer original.Close()
	parent.SubscriptionURL = original.URL + "/original"
	settings.value.Features["personal_device_profiles"] = false
	s.reconcile(context.Background())
	state := &remnawave.UserState{}
	if err := s.Observe(context.Background(), parent, state); err != nil {
		t.Fatal(err)
	}
	if state.SubscriptionLink == nil || *state.SubscriptionLink != parent.SubscriptionURL || state.DeviceTrafficManaged || store.b.Isolated || store.b.Personal {
		t.Fatal("shared subscription did not return after opt-out")
	}
	if len(parent.ActiveInternalSquads) == 0 || child.Status != "DISABLED" || len(child.ActiveInternalSquads) != 0 {
		t.Fatal("routing not switched back safely")
	}
	if parent.UserTraffic.UsedTrafficBytes != 11*gb || child.UserTraffic.UsedTrafficBytes != 7*gb || panel.revocations != 0 {
		t.Fatal("rollback changed counters or credentials")
	}
	users := []remnawave.PanelUser{*parent}
	if err := s.DecorateCatalog(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	if users[0].SubscriptionURL != parent.SubscriptionURL {
		t.Fatal("catalog replaced restored link with a device URL")
	}
	store.b.Blocked = []BlockedDevice{{Device: remnawave.UserDevice{Hwid: hwid}}}
	// Already imported common URLs still proxy the original configuration, even
	// without HWID after switching back to the ordinary shared subscription.
	for _, value := range []string{"", hwid} {
		if w := request(s, store.b, value); w.Code != 200 || w.Body.String() != "original-profile" {
			t.Fatalf("legacy gateway broke after rollback: %d %s", w.Code, w.Body.String())
		}
	}
	settings.value.Features["personal_device_profiles"] = true
	store.b.Blocked = nil
	if w := request(s, store.b, hwid); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if len(panel.users) != 2 || panel.users[store.b.Slots[0].ChildID] != child || child.UserTraffic.UsedTrafficBytes != 7*gb {
		t.Fatal("re-enabling allocated fresh allowance")
	}
}

func TestOptOutRestoresParentWhenTechnicalUserWasDeleted(t *testing.T) {
	s, store, panel, settings := fixture(t)
	store.terms = &trafficpolicy.Terms{}
	settings.value.Features = map[string]bool{"personal_device_profiles": true}
	if w := request(s, store.b, "missing-child-123"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	delete(panel.users, store.b.Slots[0].ChildID)
	settings.value.Features["personal_device_profiles"] = false
	state := &remnawave.UserState{}
	if err := s.Observe(context.Background(), panel.users[1], state); err != nil {
		t.Fatal(err)
	}
	if len(panel.users[1].ActiveInternalSquads) == 0 || store.b.Isolated || state.SubscriptionLink == nil || *state.SubscriptionLink != panel.users[1].SubscriptionURL {
		t.Fatal("missing child stranded the original profile")
	}
}

func TestDefaultOptOutDoesNotDisablePurchasedAdministratorLimits(t *testing.T) {
	s, store, panel, settings := fixture(t)
	if w := request(s, store.b, "admin-protected-123"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	settings.value.Features = map[string]bool{"personal_device_profiles": false}
	s.reconcile(context.Background())
	child := panel.users[store.b.Slots[0].ChildID]
	state := &remnawave.UserState{}
	if err := s.Observe(context.Background(), panel.users[1], state); err != nil {
		t.Fatal(err)
	}
	if child.Status != "ACTIVE" || child.TrafficLimitBytes != 20*gb || !store.b.Isolated || !state.DeviceTrafficManaged || state.DeviceTrafficUserManaged {
		t.Fatal("default toggle disabled purchased administrator quotas")
	}
}
