package miniapp

import (
	"context"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

func TestIncompletePanelStateDoesNotClaimDeviceNotifications(t *testing.T) {
	// With a nil database pool and an unconfigured Telegram client, this test
	// fails if an incomplete read attempts either a state write or a message.
	h := &Handler{customerRepository: database.NewCustomerRepository(nil), telegramBot: &bot.Bot{}}
	customer := &database.Customer{TelegramID: 123}
	subscription := &database.CustomerSubscription{ID: 1}
	for _, state := range []*remnawave.UserState{
		nil,
		{Exists: true, UserID: 1281, DevicesCheckedAt: time.Now()},                                      // Failed HWID read.
		{Exists: true, UserID: 1281, DevicesLoaded: true},                                               // No observation time.
		{Exists: true, UserID: 1281, DevicesLoaded: true, DevicesCheckedAt: time.Now(), UsedDevices: 5}, // Partial list.
		{DevicesLoaded: true, DevicesCheckedAt: time.Now()},                                             // Missing user.
	} {
		h.trackDeviceNotifications(context.Background(), customer, subscription, state)
	}
}

func TestDeviceNotificationSnapshotUsesExactSubscriptionIdentity(t *testing.T) {
	now := time.Now()
	state := &remnawave.UserState{
		Exists: true, UserID: 1281, DevicesLoaded: true, DevicesCheckedAt: now,
		UsedDevices: 1, Devices: []remnawave.UserDevice{{Hwid: "phone"}}, DeviceLimit: 5,
	}
	snapshot, valid := deviceNotificationSnapshotForPanelState(state)
	if !valid || snapshot.PanelUserKey != "id:1281" || len(snapshot.DeviceHWIDs) != 1 || snapshot.DeviceHWIDs[0] != "phone" || snapshot.DeviceLimit != 5 || !snapshot.ObservedAt.Equal(now) {
		t.Fatalf("numeric snapshot=%+v valid=%t", snapshot, valid)
	}
	state.UserUUID = uuid.MustParse("00000000-0000-4000-8000-000000000001")
	snapshot, valid = deviceNotificationSnapshotForPanelState(state)
	if !valid || snapshot.PanelUserKey != "uuid:"+state.UserUUID.String() {
		t.Fatalf("legacy snapshot=%+v valid=%t", snapshot, valid)
	}
	state.Devices, state.UsedDevices = []remnawave.UserDevice{}, 0
	snapshot, valid = deviceNotificationSnapshotForPanelState(state)
	if !valid || snapshot.DeviceHWIDs == nil || len(snapshot.DeviceHWIDs) != 0 {
		t.Fatalf("confirmed empty snapshot=%+v valid=%t", snapshot, valid)
	}
}
