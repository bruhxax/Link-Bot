// Package devicequota implements one logical subscription backed by one native
// Remnawave user per device. HWID identifies an import, not tamper-proof hardware.
package devicequota

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	planbook "link-bot/internal/plans"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
)

var (
	ErrBusy        = errors.New("device profiles are being updated; retry shortly")
	ErrUnavailable = errors.New("device subscription unavailable")
	ErrDeviceLimit = errors.New("device limit reached")
)

const Route = "/api/device-subscriptions/"

type Panel interface {
	RawUser(context.Context, int64, uuid.UUID) (*remnawave.PanelUser, error)
	RawQuotaUserByTelegramID(context.Context, int64) (*remnawave.PanelUser, error)
	UpdateQuotaUser(context.Context, *remnawave.PanelUser, map[string]any) (*remnawave.PanelUser, error)
	EnsureQuotaUser(context.Context, string, map[string]any) (*remnawave.PanelUser, error)
	RegisterQuotaHWID(context.Context, *remnawave.PanelUser, remnawave.UserDevice) error
	RevokeQuotaUser(context.Context, *remnawave.PanelUser) error
	DeleteRawHWIDDevice(context.Context, int64, uuid.UUID, string) ([]remnawave.UserDevice, error)
	DropQuotaConnections(context.Context, int64) error
}
type Settings interface{ Snapshot() runtimeconfig.Settings }
type Service struct {
	store    Store
	panel    Panel
	settings Settings
	origin   string
}

func New(store Store, panel Panel, settings Settings, publicURL string) *Service {
	u, err := url.Parse(strings.TrimSpace(publicURL))
	origin := ""
	if err == nil && u.Host != "" && (u.Scheme == "https" || u.Scheme == "http") {
		origin = u.Scheme + "://" + u.Host
	}
	return &Service{store: store, panel: panel, settings: settings, origin: origin}
}
func parentKey(u *remnawave.PanelUser) string {
	if u.UUID != uuid.Nil {
		return u.UUID.String()
	}
	return fmt.Sprintf("id:%d", u.ID)
}
func (s *Service) plan(ctx context.Context, u *remnawave.PanelUser) (int64, *runtimeconfig.PlanSettings, error) {
	id, planID, terms, err := s.store.Plan(ctx, u)
	if err != nil {
		return 0, nil, err
	}
	var plan *runtimeconfig.PlanSettings
	if terms != nil {
		plan = &runtimeconfig.PlanSettings{ID: planID, DeviceTrafficEnabled: terms.Enabled, DeviceTrafficEnforced: terms.Enforced, DeviceTrafficRules: append([]planbook.DeviceTrafficRule(nil), terms.Rules...)}
	} else {
		for _, p := range s.settings.Snapshot().Plans {
			if p.ID == planID {
				copy := p
				plan = &copy
				break
			}
		}
	}
	// Explicit purchased administrator rules always take precedence. The default
	// affects only subscriptions without those rules, including panel/trial ones.
	if enforced(plan) {
		return id, plan, nil
	}
	if id > 0 {
		b, e := s.store.FindParent(ctx, parentKey(u))
		if e != nil {
			return 0, nil, e
		}
		if s.settings.Snapshot().Features["personal_device_profiles"] || (b != nil && b.Personal) {
			return id, &runtimeconfig.PlanSettings{ID: planID, DeviceTrafficEnabled: true, DeviceTrafficEnforced: true, DeviceTrafficUserManaged: true}, nil
		}
	}
	return id, plan, nil
}
func enforced(p *runtimeconfig.PlanSettings) bool {
	return p != nil && p.DeviceTrafficEnabled && p.DeviceTrafficEnforced
}
func quota(p *runtimeconfig.PlanSettings, number int) int64 {
	if p == nil {
		return 0
	}
	for _, r := range p.DeviceTrafficRules {
		if r.Devices == number {
			return int64(r.TrafficGB) * 1024 * 1024 * 1024
		}
	}
	return 0
}
func (s *Service) binding(ctx context.Context, u *remnawave.PanelUser, p *runtimeconfig.PlanSettings, subID int64) (*Binding, error) {
	b, err := s.store.FindParent(ctx, parentKey(u))
	if err != nil || b != nil || !enforced(p) {
		return b, err
	}
	if s.origin == "" {
		return nil, errors.New("public cabinet URL is required for device quotas")
	}
	squads := squadIDs(u)
	if len(squads) == 0 {
		return nil, errors.New("device quotas require at least one internal squad")
	}
	b = &Binding{ParentKey: parentKey(u), ParentID: u.ID, ParentUUID: u.UUID, SubscriptionID: &subID, Token: uuid.New(), Squads: squads, Personal: p.DeviceTrafficUserManaged}
	if err = s.store.Create(ctx, b); err != nil {
		return nil, err
	}
	return s.store.FindParent(ctx, b.ParentKey)
}
func squadIDs(u *remnawave.PanelUser) []string {
	ids := make([]string, 0, len(u.ActiveInternalSquads))
	for _, v := range u.ActiveInternalSquads {
		ids = append(ids, v.UUID.String())
	}
	sort.Strings(ids)
	return ids
}
func isActive(u *remnawave.PanelUser) bool {
	return strings.EqualFold(u.Status, "ACTIVE") && u.ExpireAt.After(time.Now()) && (u.TrafficLimitBytes <= 0 || u.UserTraffic.UsedTrafficBytes < u.TrafficLimitBytes)
}

// synchronize preserves native counters when limits, dates or profiles change.
// All slots, including removed ones, contribute to the parent's aggregate cap.
func (s *Service) synchronize(ctx context.Context, b *Binding, save func() error, u *remnawave.PanelUser, p *runtimeconfig.PlanSettings, subscriptionID int64) ([]remnawave.UserDevice, int64, int64, error) {
	if subscriptionID == 0 || b.SubscriptionID == nil || subscriptionID != *b.SubscriptionID {
		// A detached/removed logical subscription must not regain a shared
		// profile when children are cleaned up.
		if err := s.disableChildren(ctx, b); err != nil {
			return nil, 0, 0, err
		}
		_, err := s.panel.UpdateQuotaUser(ctx, u, map[string]any{"status": "DISABLED", "activeInternalSquads": []string{}})
		return nil, 0, 0, err
	}
	activeMode := enforced(p) && b.SubscriptionID != nil
	if activeMode {
		if p.DeviceTrafficUserManaged {
			b.Personal = true
		}
		ids := squadIDs(u)
		if len(ids) > 0 {
			// Persist routing before removing the shared user's data-plane access.
			b.Squads = ids
		}
		// Persist the intent too: if the process dies immediately after clearing
		// routing, opting out on restart must still restore the saved squads.
		b.Isolated = true
		if err := save(); err != nil {
			return nil, 0, 0, err
		}
		if len(ids) > 0 {
			if _, err := s.panel.UpdateQuotaUser(ctx, u, map[string]any{"activeInternalSquads": []string{}}); err != nil {
				return nil, 0, 0, err
			}
		}
	}
	total, lifetime := u.UserTraffic.UsedTrafficBytes, u.UserTraffic.LifetimeUsedTrafficBytes
	users := make(map[int]*remnawave.PanelUser, len(b.Slots))
	for _, slot := range b.Slots {
		if slot.ChildID <= 0 && slot.ChildUUID == uuid.Nil {
			continue
		}
		child, err := s.panel.RawUser(ctx, slot.ChildID, slot.ChildUUID)
		if err != nil {
			// Unknown usage must not leave the remaining profiles running beyond
			// an unverifiable shared allowance. Identities are persisted locally.
			return nil, 0, 0, errors.Join(err, s.disableChildren(ctx, b))
		}
		users[slot.Number] = child
		total += child.UserTraffic.UsedTrafficBytes
		lifetime += child.UserTraffic.LifetimeUsedTrafficBytes
	}
	var devices []remnawave.UserDevice
	for _, slot := range b.Slots {
		child := users[slot.Number]
		if child == nil {
			continue
		}
		if slot.RevokeHWID != "" {
			if _, err := s.panel.UpdateQuotaUser(ctx, child, map[string]any{"status": "DISABLED"}); err != nil {
				return nil, 0, 0, err
			}
			s.dropConnections(ctx, b, slot, save)
			if err := s.panel.RevokeQuotaUser(ctx, child); err != nil {
				return nil, 0, 0, err
			}
			if _, err := s.panel.DeleteRawHWIDDevice(ctx, child.ID, child.UUID, slot.RevokeHWID); err != nil {
				return nil, 0, 0, err
			}
			slot.RevokeHWID = ""
			slot.Registered = false
			if err := save(); err != nil {
				return nil, 0, 0, err
			}
			var err error
			child, err = s.panel.RawUser(ctx, slot.ChildID, slot.ChildUUID)
			if err != nil {
				return nil, 0, 0, err
			}
		}
		limit := quota(p, slot.Number)
		if p != nil && p.DeviceTrafficUserManaged && slot.UserLimit != nil {
			limit = *slot.UserLimit
		}
		permitted := activeMode && slot.Device.Hwid != "" && !slot.Paused && slotAllowed(slot.Number, u.HwidDeviceLimit) && isActive(u) && (u.TrafficLimitBytes <= 0 || total < u.TrafficLimitBytes) && (limit <= 0 || child.UserTraffic.UsedTrafficBytes < limit)
		if permitted && !slot.Registered {
			if err := s.panel.RegisterQuotaHWID(ctx, child, slot.Device); err != nil {
				return nil, 0, 0, err
			}
			slot.Registered = true
			if err := save(); err != nil {
				return nil, 0, 0, err
			}
		}
		status := "DISABLED"
		squads := []string{}
		if permitted {
			status = "ACTIVE"
			squads = b.Squads
		}
		fields := map[string]any{}
		if slot.Username == "" {
			slot.Username = deviceUsername(u, b, slot.Number)
			if err := save(); err != nil {
				return nil, 0, 0, err
			}
		}
		if child.Username != slot.Username {
			fields["username"] = slot.Username
		}
		description := deviceDescription(u, slot)
		if child.Description == nil || *child.Description != description {
			fields["description"] = description
		}
		if child.Status != status {
			if status == "DISABLED" && child.Status == "ACTIVE" {
				slot.DropPending = true
				if err := save(); err != nil {
					return nil, 0, 0, err
				}
			}
			fields["status"] = status
		}
		if strings.Join(squadIDs(child), ",") != strings.Join(squads, ",") {
			fields["activeInternalSquads"] = squads
		}
		if u.ExpireAt.After(time.Now()) && !child.ExpireAt.Equal(u.ExpireAt) {
			fields["expireAt"] = u.ExpireAt.UTC().Format(time.RFC3339)
		}
		if child.TrafficLimitBytes != limit {
			fields["trafficLimitBytes"] = limit
		}
		if child.TrafficLimitStrategy != u.TrafficLimitStrategy {
			fields["trafficLimitStrategy"] = u.TrafficLimitStrategy
		}
		if child.HwidDeviceLimit == nil || *child.HwidDeviceLimit != 1 {
			fields["hwidDeviceLimit"] = 1
		}
		if !sameUUID(child.ExternalSquadUUID, u.ExternalSquadUUID) {
			fields["externalSquadUuid"] = u.ExternalSquadUUID
		}
		if len(fields) > 0 {
			if _, err := s.panel.UpdateQuotaUser(ctx, child, fields); err != nil {
				return nil, 0, 0, err
			}
		}
		if slot.DropPending {
			s.dropConnections(ctx, b, slot, save)
		}
		if slot.Device.Hwid != "" {
			d := slot.Device
			used := child.UserTraffic.UsedTrafficBytes
			d.TrafficUsedBytes = &used
			d.TrafficLimitBytes = &limit
			d.Slot = slot.Number
			d.Paused = slot.Paused
			d.CanSetTrafficLimit = p != nil && p.DeviceTrafficUserManaged
			devices = append(devices, d)
		}
	}
	if !activeMode && b.Isolated {
		// Disable every child before restoring the original shared credentials.
		if _, err := s.panel.UpdateQuotaUser(ctx, u, map[string]any{"activeInternalSquads": b.Squads}); err != nil {
			return nil, 0, 0, err
		}
		b.Isolated = false
		if err := save(); err != nil {
			return nil, 0, 0, err
		}
	}
	return devices, total, lifetime, nil
}

func (s *Service) disableChildren(ctx context.Context, b *Binding) error {
	var result error
	for _, slot := range b.Slots {
		if slot.ChildID <= 0 && slot.ChildUUID == uuid.Nil {
			continue
		}
		_, err := s.panel.UpdateQuotaUser(ctx, &remnawave.PanelUser{ID: slot.ChildID, UUID: slot.ChildUUID}, map[string]any{"status": "DISABLED", "activeInternalSquads": []string{}})
		if err != nil && !errors.Is(err, remnawave.ErrAdminSubscriptionNotFound) {
			result = errors.Join(result, err)
		}
	}
	return result
}
func sameUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func (s *Service) Observe(ctx context.Context, u *remnawave.PanelUser, state *remnawave.UserState) error {
	if u.Tag != nil && *u.Tag == remnawave.DeviceQuotaTag {
		return nil
	}
	subID, p, err := s.plan(ctx, u)
	if err != nil {
		return err
	}
	b, err := s.binding(ctx, u, p, subID)
	if err != nil || b == nil {
		return err
	}
	return s.store.Locked(ctx, b.ID, func(b *Binding, save func() error) error {
		// Re-read under the cross-process lock: a previous request may have restored
		// routing since the caller's panel snapshot was obtained.
		parent, e := s.panel.RawUser(ctx, b.ParentID, b.ParentUUID)
		if e != nil {
			return e
		}
		devices, total, lifetime, e := s.synchronize(ctx, b, save, parent, p, subID)
		if e != nil {
			return e
		}
		link := s.origin + Route + b.Token.String()
		state.SubscriptionLink = &link
		if enforced(p) {
			state.DeviceTrafficManaged = true
			state.DeviceTrafficUserManaged = p.DeviceTrafficUserManaged
			state.BlockedDevices = blockedDevices(b)
			state.Devices = devices
			state.UsedDevices = len(devices)
			state.DevicesLoaded = true
			state.DevicesCheckedAt = time.Now().UTC()
			state.UsedTrafficBytes = total
			state.LifetimeUsedTrafficBytes = lifetime
			// Exhausting traffic does not expire the paid subscription: keep the
			// cabinet available for buying traffic. Data-plane access is checked
			// independently above and by the native child limits.
			state.Active = !strings.EqualFold(parent.Status, "DISABLED") && !strings.EqualFold(parent.Status, "EXPIRED") && parent.ExpireAt.After(time.Now())
		}
		return nil
	})
}

// Catalog synchronization must not replace the stable public link with a
// parent's now-isolated panel URL. No remote writes are needed for this path.
func (s *Service) DecorateCatalog(ctx context.Context, users []remnawave.PanelUser) error {
	bindings, err := s.store.List(ctx)
	if err != nil {
		return err
	}
	links := make(map[string]string, len(bindings))
	for _, b := range bindings {
		links[b.ParentKey] = s.origin + Route + b.Token.String()
	}
	for i := range users {
		if link := links[parentKey(&users[i])]; link != "" {
			users[i].SubscriptionURL = link
		}
	}
	return nil
}
func (s *Service) DeleteDevice(ctx context.Context, id int64, uid uuid.UUID, hwid string) ([]remnawave.UserDevice, bool, error) {
	u, err := s.panel.RawUser(ctx, id, uid)
	if err != nil {
		return nil, false, err
	}
	b, err := s.store.FindParent(ctx, parentKey(u))
	if err != nil || b == nil {
		return nil, false, err
	}
	subID, p, err := s.plan(ctx, u)
	if err != nil {
		return nil, true, err
	}
	if !enforced(p) {
		return nil, false, nil
	}
	var devices []remnawave.UserDevice
	err = s.store.Locked(ctx, b.ID, func(b *Binding, save func() error) error {
		found := isBlocked(b, hwid)
		for _, slot := range b.Slots {
			if slot.Device.Hwid == strings.TrimSpace(hwid) {
				blockSlot(b, slot)
				found = true
				break
			}
		}
		if !found {
			return errors.New("device not found")
		}
		if e := save(); e != nil {
			return e
		}
		parent, e := s.panel.RawUser(ctx, b.ParentID, b.ParentUUID)
		if e != nil {
			return e
		}
		devices, _, _, e = s.synchronize(ctx, b, save, parent, p, subID)
		return e
	})
	return devices, true, err
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		s.reconcile(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) reconcile(ctx context.Context) {
	// An administrator can enable quotas on a tariff whose subscribers are
	// offline. Activate those subscriptions too, without waiting for a visit.
	var planIDs []string
	for _, p := range s.settings.Snapshot().Plans {
		if enforced(&p) {
			planIDs = append(planIDs, p.ID)
		}
	}
	if s.settings.Snapshot().Features["personal_device_profiles"] {
		planIDs = append(planIDs, "*")
	}
	candidates, discoverErr := s.store.Candidates(ctx, planIDs)
	if discoverErr != nil {
		slog.Error("device quota: discover subscriptions failed")
	}
	for _, candidate := range candidates {
		itemCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		var u *remnawave.PanelUser
		var err error
		if candidate.ID > 0 || candidate.UUID != uuid.Nil {
			u, err = s.panel.RawUser(itemCtx, candidate.ID, candidate.UUID)
		} else if candidate.TelegramID != nil {
			u, err = s.panel.RawQuotaUserByTelegramID(itemCtx, *candidate.TelegramID)
		}
		if err == nil && u != nil {
			err = s.Observe(itemCtx, u, &remnawave.UserState{})
		}
		cancel()
		if err != nil && !errors.Is(err, ErrBusy) && !errors.Is(err, remnawave.ErrAdminSubscriptionNotFound) {
			slog.Error("device quota: activation failed")
		}
	}
	bindings, err := s.store.List(ctx)
	if err != nil {
		slog.Error("device quota: list bindings failed")
		return
	}
	for _, b := range bindings {
		if ctx.Err() != nil {
			return
		}
		itemCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		// Resolve through the pool before acquiring the advisory-lock connection.
		// Otherwise a busy pool could deadlock waiting on its own held sessions.
		var p *runtimeconfig.PlanSettings
		var subID int64
		preParent, preErr := s.panel.RawUser(itemCtx, b.ParentID, b.ParentUUID)
		if preErr == nil {
			subID, p, preErr = s.plan(itemCtx, preParent)
		}
		if preErr != nil && !errors.Is(preErr, remnawave.ErrAdminSubscriptionNotFound) {
			cancel()
			slog.Error("device quota: parent lookup failed", "bindingId", b.ID)
			continue
		}
		err = s.store.Locked(itemCtx, b.ID, func(b *Binding, save func() error) error {
			u, e := s.panel.RawUser(itemCtx, b.ParentID, b.ParentUUID)
			if errors.Is(e, remnawave.ErrAdminSubscriptionNotFound) {
				return s.disableChildren(itemCtx, b)
			}
			if e != nil {
				return e
			}
			_, _, _, e = s.synchronize(itemCtx, b, save, u, p, subID)
			return e
		})
		cancel()
		if err != nil && !errors.Is(err, ErrBusy) {
			slog.Error("device quota: reconciliation failed", "bindingId", b.ID)
		}
	}
}
