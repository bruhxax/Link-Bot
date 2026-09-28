package database

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestDeviceNotificationTransitions(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	previous := deviceNotificationState{
		PanelUserKey: "id:1281", DeviceHWIDs: []string{"phone", "laptop"}, ObservedAt: &now,
	}
	tests := []struct {
		name     string
		previous deviceNotificationState
		hwids    []string
		key      string
		limit    int
		at       time.Time
		added    int
		reached  bool
		fresh    bool
	}{
		{name: "first read is silent", hwids: []string{"phone", "laptop"}, limit: 2, fresh: true},
		{name: "migrated count is silent", previous: deviceNotificationState{LimitReached: true}, hwids: []string{"phone", "laptop"}, limit: 2, fresh: true},
		{name: "opening cabinet again", previous: previous, hwids: []string{"laptop", "phone"}, fresh: true},
		{name: "one new device", previous: previous, hwids: []string{"phone", "laptop", "tablet"}, added: 1, fresh: true},
		{name: "replacement at unchanged count", previous: previous, hwids: []string{"phone", "tablet"}, added: 1, fresh: true},
		{name: "deletion is silent", previous: previous, hwids: []string{"phone"}, fresh: true},
		{name: "confirmed empty is silent", previous: previous, hwids: []string{}, fresh: true},
		{name: "first device after confirmed empty", previous: deviceNotificationState{PanelUserKey: "id:1281", DeviceHWIDs: []string{}, ObservedAt: &now}, hwids: []string{"phone"}, added: 1, fresh: true},
		{name: "new device reaches limit", previous: previous, hwids: []string{"phone", "laptop", "tablet"}, limit: 3, added: 1, reached: true, fresh: true},
		{name: "limit notification is not repeated", previous: deviceNotificationState{PanelUserKey: "id:1281", DeviceHWIDs: []string{"phone", "laptop"}, LimitReached: true, ObservedAt: &now}, hwids: []string{"phone", "laptop"}, limit: 2, fresh: true},
		{name: "older empty read cannot reset state", previous: previous, hwids: []string{}, at: now.Add(-time.Second)},
		{name: "older devices cannot be restored", previous: previous, hwids: []string{"phone", "laptop", "deleted"}, at: now.Add(-time.Second)},
		{name: "equal timestamp is ignored", previous: previous, hwids: []string{"phone"}, at: now},
		{name: "reissued panel user gets silent baseline", previous: previous, key: "id:1282", hwids: []string{"phone", "laptop", "tablet"}, limit: 3, fresh: true},
		{name: "old panel user read cannot undo reissue", previous: previous, key: "id:1282", hwids: []string{"tablet"}, at: now.Add(-time.Second)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, at := tt.key, tt.at
			if key == "" {
				key = "id:1281"
			}
			if at.IsZero() {
				at = now.Add(time.Second)
			}
			snapshot, valid := normalizeDeviceNotificationSnapshot(DeviceNotificationSnapshot{
				PanelUserKey: key, DeviceHWIDs: tt.hwids, DeviceLimit: tt.limit, ObservedAt: at,
			})
			if !valid {
				t.Fatal("test snapshot rejected")
			}
			added, reached, fresh := deviceNotificationChanges(tt.previous, snapshot)
			if added != tt.added || reached != tt.reached || fresh != tt.fresh {
				t.Fatalf("changes=(%d, %t, %t), want (%d, %t, %t)", added, reached, fresh, tt.added, tt.reached, tt.fresh)
			}
		})
	}
}

func TestInvalidDeviceSnapshotsNeverTouchNotificationState(t *testing.T) {
	valid := DeviceNotificationSnapshot{PanelUserKey: "id:1281", DeviceHWIDs: []string{"phone"}, ObservedAt: time.Now()}
	unknownDevices, missingUser, missingTime, missingHWID := valid, valid, valid, valid
	unknownDevices.DeviceHWIDs = nil
	missingUser.PanelUserKey = ""
	missingTime.ObservedAt = time.Time{}
	missingHWID.DeviceHWIDs = []string{"phone", " "}
	// A nil pool makes an accidental database write fail immediately.
	repository := NewCustomerRepository(nil)
	for _, snapshot := range []DeviceNotificationSnapshot{unknownDevices, missingUser, missingTime, missingHWID} {
		added, reached, err := repository.ClaimDeviceNotifications(context.Background(), 123, 1, snapshot)
		if err != nil || added != 0 || reached {
			t.Fatalf("invalid snapshot emitted notification: (%d, %t, %v)", added, reached, err)
		}
	}
}

func TestDeviceHWIDsAreDeduplicatedWithoutMutatingCaller(t *testing.T) {
	hwids := []string{" phone ", "laptop", "phone"}
	snapshot, valid := normalizeDeviceNotificationSnapshot(DeviceNotificationSnapshot{
		PanelUserKey: "id:1281", DeviceHWIDs: hwids, ObservedAt: time.Now(), DeviceLimit: -1,
	})
	if !valid || snapshot.DeviceLimit != 0 || !reflect.DeepEqual(snapshot.DeviceHWIDs, []string{"laptop", "phone"}) {
		t.Fatalf("normalized snapshot=%+v valid=%t", snapshot, valid)
	}
	if !reflect.DeepEqual(hwids, []string{" phone ", "laptop", "phone"}) {
		t.Fatal("normalization mutated caller's device list")
	}
}
