package remnawave

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"
)

// DeviceQuotaBridge decorates logical users without exposing technical users as
// customer subscriptions. Raw methods below deliberately bypass this observer.
type DeviceQuotaBridge interface {
	Observe(context.Context, *PanelUser, *UserState) error
	DeleteDevice(context.Context, int64, uuid.UUID, string) ([]UserDevice, bool, error)
	DecorateCatalog(context.Context, []PanelUser) error
	ControlDevice(context.Context, int64, uuid.UUID, string, string, *int64) error
}

const DeviceQuotaTag = "LB_DEVICE_QUOTA"

var ErrDropConnectionsUnsupported = errors.New("panel does not support dropping connections")

func (r *Client) ControlDevice(ctx context.Context, id int64, uid uuid.UUID, hwid, action string, limit *int64) error {
	if r.deviceQuota == nil {
		return errors.New("управление устройствами не включено")
	}
	return r.deviceQuota.ControlDevice(ctx, id, uid, hwid, action, limit)
}

func (r *Client) DropQuotaConnections(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return ErrDropConnectionsUnsupported
	}
	err := r.doAPIJSON(ctx, http.MethodPost, "/api/connections/drop", map[string]any{"dropBy": map[string]any{"by": "userIds", "userIds": []int64{userID}}, "targetNodes": map[string]any{"target": "allNodes"}}, nil)
	var apiErr *remnawaveAPIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 404 || apiErr.StatusCode == 405 || apiErr.StatusCode == 501) {
		return ErrDropConnectionsUnsupported
	}
	return err
}

func (r *Client) SetDeviceQuotaBridge(bridge DeviceQuotaBridge) { r.deviceQuota = bridge }
func (r *Client) RawUser(ctx context.Context, id int64, uid uuid.UUID) (*PanelUser, error) {
	return r.getPanelUserByIdentity(ctx, id, uid)
}

func (r *Client) RawQuotaUserByTelegramID(ctx context.Context, id int64) (*PanelUser, error) {
	return r.getPanelUserByTelegramID(ctx, id)
}

// Called after the purchase and panel identity have been persisted, before any
// customer-facing activation link is sent. Preserve the stable logical URL.
func (r *Client) PrepareQuotaUser(ctx context.Context, user *PanelUser) (*PanelUser, error) {
	if r.deviceQuota == nil {
		return user, nil
	}
	state := &UserState{}
	if err := r.deviceQuota.Observe(ctx, user, state); err != nil {
		return nil, err
	}
	copy := *user
	if state.SubscriptionLink != nil {
		copy.SubscriptionURL = *state.SubscriptionLink
	}
	return &copy, nil
}
func (r *Client) UpdateQuotaUser(ctx context.Context, user *PanelUser, fields map[string]any) (*PanelUser, error) {
	return r.patchPanelUser(ctx, user, fields)
}
func (r *Client) EnsureQuotaUser(ctx context.Context, username string, fields map[string]any) (*PanelUser, error) {
	var payload struct {
		Response PanelUser `json:"response"`
	}
	err := r.doAPIJSON(ctx, http.MethodGet, "/api/users/by-username/"+url.PathEscape(username), nil, &payload)
	if err == nil {
		if payload.Response.Username != username || payload.Response.Tag == nil || *payload.Response.Tag != DeviceQuotaTag || payload.Response.TelegramID != nil {
			return nil, fmt.Errorf("device quota username is already in use")
		}
		return &payload.Response, nil
	}
	var apiErr *remnawaveAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		return nil, err
	}
	fields["username"] = username
	fields["tag"] = DeviceQuotaTag
	return r.createPanelUser(ctx, fields)
}

func (r *Client) RegisterQuotaHWID(ctx context.Context, user *PanelUser, device UserDevice) error {
	devices, err := r.getUserHWIDDevices(ctx, user.ID, user.UUID)
	if err != nil {
		return err
	}
	for _, existing := range devices {
		if existing.Hwid == device.Hwid {
			return nil
		}
	}
	body := map[string]any{"userId": user.ID, "hwid": device.Hwid, "platform": device.Platform, "osVersion": device.OSVersion, "deviceModel": device.DeviceModel, "userAgent": device.UserAgent}
	err = r.doAPIJSON(ctx, http.MethodPost, "/api/hwid/devices", body, nil)
	if err != nil && user.UUID != uuid.Nil && isLegacyFallbackError(err) {
		delete(body, "userId")
		body["userUuid"] = user.UUID.String()
		err = r.doAPIJSON(ctx, http.MethodPost, "/api/hwid/devices", body, nil)
	}
	return err
}

// Revoke both configuration credentials and the old URL; never reset traffic.
func (r *Client) RevokeQuotaUser(ctx context.Context, user *PanelUser) error {
	identifier := strconv.FormatInt(user.ID, 10)
	if user.ID <= 0 {
		identifier = user.UUID.String()
	}
	err := r.doAPIJSON(ctx, http.MethodPost, "/api/users/"+identifier+"/actions/revoke", map[string]any{"revokeOnlyPasswords": false}, nil)
	if err != nil && user.UUID != uuid.Nil && isLegacyFallbackError(err) {
		err = r.doAPIJSON(ctx, http.MethodPost, "/api/users/"+user.UUID.String()+"/actions/revoke", map[string]any{"revokeOnlyPasswords": false}, nil)
	}
	return err
}
