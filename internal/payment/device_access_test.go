package payment

import (
	"context"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"testing"
	"time"
)

func TestDevicePackProratedAmount(t *testing.T) {
	for _, tc := range []struct {
		name      string
		remaining time.Duration
		want      int
	}{
		{"one month", 30 * 24 * time.Hour, 50}, {"three months", 90 * 24 * time.Hour, 150},
		{"half month", 15 * 24 * time.Hour, 25}, {"one day", 24 * time.Hour, 2},
		{"partial day", 6 * time.Hour, 1}, {"expired", -time.Hour, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := DevicePackAmount(50, tc.remaining); got != tc.want {
				t.Fatalf("amount = %d, want %d", got, tc.want)
			}
		})
	}
}

type memoryDeviceStore struct {
	base   map[int64]int
	grants map[int64]database.DeviceGrant
	paid   map[int64]bool
}

func (m *memoryDeviceStore) EnsureDeviceBase(_ context.Context, id int64, initial int) error {
	if _, ok := m.base[id]; !ok {
		m.base[id] = initial
	}
	return nil
}
func (m *memoryDeviceStore) AddDeviceGrant(_ context.Context, id, sub int64, devices int, expiry time.Time) error {
	if _, ok := m.grants[id]; !ok {
		m.grants[id] = database.DeviceGrant{PurchaseID: id, SubscriptionID: sub, Devices: devices, ExpiresAt: expiry}
	}
	return nil
}
func (m *memoryDeviceStore) DeviceLimitState(_ context.Context, sub, pending int64, now time.Time) (int, int, error) {
	extra := 0
	for id, g := range m.grants {
		if g.SubscriptionID == sub && g.ExpiresAt.After(now) && (id == pending || m.paid[id]) {
			extra += g.Devices
		}
	}
	return m.base[sub], extra, nil
}

func TestDeviceGrantExpiresAfterEarlyRenewalAndRetriesDoNotExtendIt(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	expiry := now.Add(15 * 24 * time.Hour)
	extended := expiry.Add(30 * 24 * time.Hour)
	store := &memoryDeviceStore{base: map[int64]int{}, grants: map[int64]database.DeviceGrant{}, paid: map[int64]bool{}}
	subscription := &database.CustomerSubscription{ID: 1, CustomerID: 10, ExpireAt: &expiry}
	customer := &database.Customer{ID: 10}
	panel := &remnawave.UserState{Exists: true, Active: true, ExpireAt: &expiry, DeviceLimit: 5}
	pack := &database.Purchase{ID: 100, PurchaseKind: database.PurchaseKindExtraDevices, ExtraDevices: 1, DeviceExpiresAt: &expiry}
	base, limit, err := prepareSubscriptionDeviceGrant(ctx, store, customer, subscription, pack, panel, now)
	if err != nil || base != 5 || limit != 6 {
		t.Fatalf("purchase = (%d,%d,%v)", base, limit, err)
	}
	// Retrying a callback after the subscription was extended must neither add
	// the devices a second time nor shift their original deadline.
	panel.DeviceLimit = 6
	panel.ExpireAt = &extended
	base, limit, err = prepareSubscriptionDeviceGrant(ctx, store, customer, subscription, pack, panel, now.Add(time.Hour))
	if err != nil || base != 5 || limit != 6 || !store.grants[100].ExpiresAt.Equal(expiry) {
		t.Fatalf("retry changed the grant: base=%d limit=%d err=%v", base, limit, err)
	}
	store.paid[100] = true
	planLimit := 5
	renewal := &database.Purchase{ID: 101, PurchaseKind: database.PurchaseKindSubscription, DeviceLimitCount: &planLimit}
	base, limit, err = prepareSubscriptionDeviceGrant(ctx, store, customer, subscription, renewal, panel, now.Add(2*time.Hour))
	if err != nil || base != 5 || limit != 6 {
		t.Fatalf("renewal = (%d,%d,%v)", base, limit, err)
	}
	store.base[1] = base
	// A later purchase can have a later deadline and must survive the first expiry.
	second := &database.Purchase{ID: 102, PurchaseKind: database.PurchaseKindExtraDevices, ExtraDevices: 2, DeviceExpiresAt: &extended}
	_, limit, err = prepareSubscriptionDeviceGrant(ctx, store, customer, subscription, second, panel, now.Add(3*time.Hour))
	if err != nil || limit != 8 {
		t.Fatalf("second purchase limit=%d err=%v", limit, err)
	}
	store.paid[102] = true
	base, extra, _ := store.DeviceLimitState(ctx, 1, 0, expiry.Add(time.Second))
	if got := deviceLimitWithGrants(base, extra); got != 7 {
		t.Fatalf("after first expiry limit=%d, want 7", got)
	}
	base, extra, _ = store.DeviceLimitState(ctx, 1, 0, extended)
	if got := deviceLimitWithGrants(base, extra); got != 5 {
		t.Fatalf("after all grants expire limit=%d, want 5", got)
	}
	if deviceLimitWithGrants(0, 2) != 0 {
		t.Fatal("extra devices must not turn an unlimited plan into a limited plan")
	}
}
