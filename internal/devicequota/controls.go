package devicequota

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/remnawave"
)

var (
	ErrDeviceBlocked = errors.New("устройство заблокировано для этой подписки; сначала разблокируйте его в кабинете")
	ErrAdminLimit    = errors.New("лимит задан администратором и не может быть изменён пользователем")
	ErrDeviceMissing = errors.New("устройство не найдено")
)

func slotAllowed(number int, limit *int) bool { return limit == nil || *limit <= 0 || number <= *limit }
func isBlocked(b *Binding, hwid string) bool {
	for _, item := range b.Blocked {
		if item.Device.Hwid == hwid {
			return true
		}
	}
	return false
}
func blockedDevices(b *Binding) []remnawave.UserDevice {
	result := make([]remnawave.UserDevice, 0, len(b.Blocked))
	for _, item := range b.Blocked {
		d := item.Device
		d.Blocked = true
		d.BlockedAt = item.BlockedAt
		d.CanSetTrafficLimit = false
		result = append(result, d)
	}
	return result
}
func (s *Service) dropConnections(ctx context.Context, b *Binding, slot *Slot, save func() error) {
	if !slot.DropPending {
		return
	}
	err := s.panel.DropQuotaConnections(ctx, slot.ChildID)
	if err == nil || errors.Is(err, remnawave.ErrDropConnectionsUnsupported) {
		slot.DropPending = false
		_ = save()
	}
}

func (s *Service) ControlDevice(ctx context.Context, id int64, uid uuid.UUID, hwid, action string, gb *int64) error {
	if action != "pause" && action != "resume" && action != "limit" && action != "unblock" {
		return errors.New("неизвестное действие с устройством")
	}
	if action == "limit" && (gb == nil || *gb < 0 || *gb > 1000000) {
		return errors.New("укажите лимит от 0 до 1000000 GB; 0 — без лимита")
	}
	parent, err := s.panel.RawUser(ctx, id, uid)
	if err != nil {
		return err
	}
	subID, plan, err := s.plan(ctx, parent)
	if err != nil {
		return err
	}
	b, err := s.store.FindParent(ctx, parentKey(parent))
	if err != nil {
		return err
	}
	if b == nil || !enforced(plan) {
		return errors.New("управление устройствами для этой подписки не включено")
	}
	if action == "limit" && !plan.DeviceTrafficUserManaged {
		return ErrAdminLimit
	}
	// Best-effort cache warming occurs outside the advisory-lock connection.
	// Pausing always disconnects even if a configuration endpoint is unavailable.
	if action == "pause" {
		s.warmConfiguration(ctx, b, hwid)
	}
	return s.store.Locked(ctx, b.ID, func(b *Binding, save func() error) error {
		if action == "unblock" {
			for i, item := range b.Blocked {
				if item.Device.Hwid == hwid {
					b.Blocked = append(b.Blocked[:i], b.Blocked[i+1:]...)
					return save()
				}
			}
			return nil
		}
		slot := findSlot(b, hwid)
		if slot == nil {
			return ErrDeviceMissing
		}
		switch action {
		case "pause":
			slot.Paused = true
			slot.DropPending = true
		case "resume":
			slot.Paused = false
		case "limit":
			value := *gb * 1024 * 1024 * 1024
			slot.UserLimit = &value
		}
		if err := save(); err != nil {
			return err
		}
		u, err := s.panel.RawUser(ctx, id, uid)
		if err != nil {
			return err
		}
		_, _, _, err = s.synchronize(ctx, b, save, u, plan, subID)
		return err
	})
}

func blockSlot(b *Binding, slot *Slot) {
	if !isBlocked(b, slot.Device.Hwid) {
		b.Blocked = append(b.Blocked, BlockedDevice{Device: slot.Device, BlockedAt: time.Now().UTC()})
	}
	slot.RevokeHWID = slot.Device.Hwid
	slot.Device.Hwid = ""
	slot.DropPending = true
}
