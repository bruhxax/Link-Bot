package miniapp

import (
	"strconv"

	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

func deviceNotificationSnapshotForPanelState(state *remnawave.UserState) (database.DeviceNotificationSnapshot, bool) {
	if state == nil || !state.Exists || !state.DevicesLoaded || state.DevicesCheckedAt.IsZero() || state.UsedDevices != len(state.Devices) {
		return database.DeviceNotificationSnapshot{}, false
	}
	key := ""
	if state.UserUUID != uuid.Nil {
		key = "uuid:" + state.UserUUID.String()
	} else if state.UserID > 0 {
		key = "id:" + strconv.FormatInt(state.UserID, 10)
	}
	if key == "" {
		return database.DeviceNotificationSnapshot{}, false
	}
	hwids := make([]string, 0, len(state.Devices))
	for _, device := range state.Devices {
		hwids = append(hwids, device.Hwid)
	}
	return database.DeviceNotificationSnapshot{
		PanelUserKey: key,
		DeviceHWIDs:  hwids,
		DeviceLimit:  state.DeviceLimit,
		ObservedAt:   state.DevicesCheckedAt,
	}, true
}
