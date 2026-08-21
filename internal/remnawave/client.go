package remnawave

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"link-bot/internal/config"
	"link-bot/utils"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"
)

type Client struct {
	client     *remapi.ClientExt
	httpClient *http.Client
	baseURL    string
	token      string
}

type SquadOption struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type SquadCatalog struct {
	Internal []SquadOption `json:"internal"`
	External []SquadOption `json:"external"`
}

type GraceAccessResult struct {
	ExpireAt         time.Time
	SubscriptionLink string
}

type ProvisioningOptions struct {
	InternalSquadUUIDs   []string
	ExternalSquadUUID    string
	TrafficResetStrategy string
	Tag                  string
	ApplySquads          bool
	UsernameTemplate     string
	UsernameSuffix       string
}

type UserState struct {
	Exists            bool
	Active            bool
	ExpireAt          *time.Time
	SubscriptionLink  *string
	PanelUsername     string
	UserID            int64
	UserUUID          uuid.UUID
	TrafficLimitBytes int64
	UsedTrafficBytes  int64
	DeviceLimit       int
	UsedDevices       int
	Devices           []UserDevice
}

var ErrAdminSubscriptionNotFound = errors.New("subscription not found")

var panelUsernameSanitizer = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)
var secondaryPanelUsernamePattern = regexp.MustCompile(`(?i)_s[0-9]+$`)

type AdminSubscription struct {
	ID               int64
	UUID             uuid.UUID
	Username         string
	Status           string
	ExpireAt         time.Time
	TelegramID       *int64
	Description      *string
	SubscriptionLink string
}

type AdminRebindResult struct {
	Subscription          *AdminSubscription
	PreviousTelegramID    *int64
	PreviousDescription   *string
	DisplacedSubscription *AdminSubscription
}

type UserDevice struct {
	Hwid        string
	UserID      int64
	UserUUID    uuid.UUID
	Platform    string
	OSVersion   string
	DeviceModel string
	UserAgent   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const (
	telegramUsernameContextKey = "username"
	telegramNameContextKey     = "telegramName"
)

type NodeStatus struct {
	Name        string
	Address     string
	CountryCode string
	IsOnline    bool
}

type headerTransport struct {
	base    http.RoundTripper
	local   bool
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())

	if t.local {
		r.Header.Set("x-forwarded-for", "127.0.0.1")
		r.Header.Set("x-forwarded-proto", "https")
	}

	for key, value := range t.headers {
		r.Header.Set(key, value)
	}

	resp, err := t.base.RoundTrip(r)
	if err != nil {
		return resp, err
	}

	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr == nil {
			body = injectMissingUserFields(body)
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
	}

	return resp, nil
}

// injectMissingUserFields walks the JSON and injects subLastUserAgent/subLastOpenedAt
// as null into any object that contains subscriptionUrl (i.e. UserItemInfo).
// This is needed because the panel removed these fields but the library still
// requires them in its decoder.
func injectMissingUserFields(data []byte) []byte {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return data
	}
	injectIntoValue(v)
	result, err := json.Marshal(v)
	if err != nil {
		return data
	}
	return result
}

func injectIntoValue(v interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
		if _, hasSubUrl := val["subscriptionUrl"]; hasSubUrl {
			if _, ok := val["subLastUserAgent"]; !ok {
				val["subLastUserAgent"] = nil
			}
			if _, ok := val["subLastOpenedAt"]; !ok {
				val["subLastOpenedAt"] = nil
			}
		}
		if uptime, ok := val["xrayUptime"]; ok {
			switch typed := uptime.(type) {
			case float64:
				val["xrayUptime"] = fmt.Sprint(typed)
			case nil:
				val["xrayUptime"] = ""
			}
		}
		for _, child := range val {
			injectIntoValue(child)
		}
	case []interface{}:
		for _, item := range val {
			injectIntoValue(item)
		}
	}
}

func NewClient(baseURL, token, mode string) *Client {
	local := mode == "local"
	headers := config.RemnawaveHeaders()

	client := &http.Client{
		Transport: &headerTransport{
			base:    http.DefaultTransport,
			local:   local,
			headers: headers,
		},
		Timeout: 12 * time.Second,
	}

	api, err := remapi.NewClient(baseURL, remapi.StaticToken{Token: token}, remapi.WithClient(client))
	if err != nil {
		panic(err)
	}
	return &Client{
		client:     remapi.NewClientExt(api),
		httpClient: client,
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
	}
}

func (r *Client) Ping(ctx context.Context) error {
	// A healthcheck must not download every panel user. One authenticated page is
	// enough to verify that the API and its database are responding.
	var payload struct{}
	err := r.doAPIJSON(ctx, http.MethodGet, "/api/users/stream?size=1", nil, &payload)
	if err == nil {
		return nil
	}
	if !isLegacyFallbackError(err) {
		return err
	}
	_, legacyErr := r.client.Users().GetAllUsers(ctx, 1, 0)
	return legacyErr
}

func (r *Client) GetUsers(ctx context.Context) (*[]PanelUser, error) {
	users, err := r.streamUsers(ctx, nil)
	if err == nil {
		return &users, nil
	}
	if !isLegacyFallbackError(err) {
		return nil, err
	}
	legacyUsers, legacyErr := r.legacyUsers(ctx)
	if legacyErr != nil {
		return nil, errors.Join(err, legacyErr)
	}
	return &legacyUsers, nil
}

func (r *Client) FindUserByIDOrUsername(ctx context.Context, query string) (*AdminSubscription, error) {
	users, err := r.GetUsers(ctx)
	if err != nil {
		return nil, err
	}

	user := findPanelUserByIDOrUsername(*users, query)
	if user == nil {
		return nil, ErrAdminSubscriptionNotFound
	}

	return adminSubscriptionFromUser(user), nil
}

func (r *Client) FindUserBySubscriptionLink(ctx context.Context, subscriptionLink string) (*AdminSubscription, error) {
	users, err := r.GetUsers(ctx)
	if err != nil {
		return nil, err
	}
	user := findPanelUserBySubscriptionLink(*users, subscriptionLink)
	if user == nil {
		return nil, ErrAdminSubscriptionNotFound
	}
	return adminSubscriptionFromUser(user), nil
}

func (r *Client) RebindUserTelegramID(
	ctx context.Context,
	userID int64,
	userUUID uuid.UUID,
	targetTelegramID int64,
	targetDescription string,
	replaceUserID int64,
	replaceUserUUID uuid.UUID,
) (*AdminRebindResult, error) {
	if (userID <= 0 && userUUID == uuid.Nil) || targetTelegramID <= 0 {
		return nil, ErrAdminSubscriptionNotFound
	}
	targetDescription = strings.TrimSpace(targetDescription)
	if targetDescription == "" {
		targetDescription = "- | -"
	}

	users, err := r.GetUsers(ctx)
	if err != nil {
		return nil, err
	}

	current := findPanelUser(*users, userID, userUUID)
	if current == nil {
		return nil, ErrAdminSubscriptionNotFound
	}

	currentSubscription := adminSubscriptionFromUser(current)
	previousTelegramID := currentSubscription.TelegramID
	previousDescription := currentSubscription.Description

	var displacedUser *PanelUser
	if replaceUserID > 0 || replaceUserUUID != uuid.Nil {
		candidate := findPanelUser(*users, replaceUserID, replaceUserUUID)
		if candidate != nil && (candidate.ID != current.ID || candidate.UUID != current.UUID) {
			displacedUser = candidate
		}
	}
	var displacedSubscription *AdminSubscription
	if displacedUser != nil {
		displacedSubscription = adminSubscriptionFromUser(displacedUser)
		unlinkedDescription := "- | -"
		if _, err := r.updateAdminUserTelegramProfile(ctx, displacedUser, nil, &unlinkedDescription); err != nil {
			return nil, fmt.Errorf("detach target account subscription: %w", err)
		}
	}

	updated, err := r.updateAdminUserTelegramProfile(ctx, current, &targetTelegramID, &targetDescription)
	if err != nil {
		if displacedUser != nil {
			rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 12*time.Second)
			_, rollbackErr := r.updateAdminUserTelegramProfile(rollbackCtx, displacedUser, displacedSubscription.TelegramID, displacedSubscription.Description)
			rollbackCancel()
			if rollbackErr != nil {
				return nil, errors.Join(err, fmt.Errorf("restore displaced subscription: %w", rollbackErr))
			}
		}
		return nil, err
	}

	return &AdminRebindResult{
		Subscription:          updated,
		PreviousTelegramID:    previousTelegramID,
		PreviousDescription:   previousDescription,
		DisplacedSubscription: displacedSubscription,
	}, nil
}

func (r *Client) RestoreAdminRebind(ctx context.Context, userID int64, userUUID uuid.UUID, telegramID *int64, description *string, displacedSubscription *AdminSubscription) error {
	users, err := r.GetUsers(ctx)
	if err != nil {
		return err
	}

	current := findPanelUser(*users, userID, userUUID)
	if current == nil {
		return ErrAdminSubscriptionNotFound
	}

	if _, err = r.updateAdminUserTelegramProfile(ctx, current, telegramID, description); err != nil {
		return fmt.Errorf("restore transferred subscription: %w", err)
	}

	if displacedSubscription == nil {
		return nil
	}
	displacedCurrent := findPanelUser(*users, displacedSubscription.ID, displacedSubscription.UUID)
	if displacedCurrent == nil {
		return fmt.Errorf("restore displaced subscription: %w", ErrAdminSubscriptionNotFound)
	}
	if _, err = r.updateAdminUserTelegramProfile(ctx, displacedCurrent, displacedSubscription.TelegramID, displacedSubscription.Description); err != nil {
		return fmt.Errorf("restore displaced subscription: %w", err)
	}
	return nil
}

func (r *Client) updateAdminUserTelegramProfile(ctx context.Context, current *PanelUser, telegramID *int64, description *string) (*AdminSubscription, error) {
	if current == nil {
		return nil, ErrAdminSubscriptionNotFound
	}

	updated, err := r.patchPanelUser(ctx, current, map[string]any{
		"telegramId":  telegramID,
		"description": description,
	})
	if err != nil {
		return nil, err
	}
	return adminSubscriptionFromUser(updated), nil
}

func adminSubscriptionFromUser(user *PanelUser) *AdminSubscription {
	if user == nil {
		return nil
	}

	return &AdminSubscription{
		ID:               user.ID,
		UUID:             user.UUID,
		Username:         strings.TrimSpace(user.Username),
		Status:           strings.TrimSpace(user.Status),
		ExpireAt:         user.ExpireAt.UTC(),
		TelegramID:       user.TelegramID,
		Description:      user.Description,
		SubscriptionLink: strings.TrimSpace(user.SubscriptionURL),
	}
}

func (r *Client) GetUserStateByTelegramID(ctx context.Context, telegramId int64) (*UserState, error) {
	user, err := r.getPanelUserByTelegramID(ctx, telegramId)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}

	return r.userStateFromPanelUser(ctx, user, "telegramId", utils.MaskHalfInt64(telegramId))
}

func (r *Client) GetUserStateByIdentity(ctx context.Context, userID int64, userUUID uuid.UUID) (*UserState, error) {
	user, err := r.getPanelUserByIdentity(ctx, userID, userUUID)
	if err != nil {
		if errors.Is(err, ErrAdminSubscriptionNotFound) || strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}
	return r.userStateFromPanelUser(ctx, user, "userId", userID)
}

func (r *Client) userStateFromPanelUser(ctx context.Context, user *PanelUser, logKey string, logValue any) (*UserState, error) {
	if user == nil {
		return nil, nil
	}

	deviceLimit := -1
	if user.HwidDeviceLimit != nil {
		deviceLimit = *user.HwidDeviceLimit
	}
	devices, deviceErr := r.getUserHWIDDevices(ctx, user.ID, user.UUID)
	if deviceErr != nil {
		slog.Warn("remnawave: load user devices failed", "error", deviceErr, logKey, logValue)
	}
	usedDevices := len(devices)

	status := strings.ToUpper(strings.TrimSpace(user.Status))
	if status == "" {
		status = "ACTIVE"
	}

	if (status == "DISABLED" || status == "EXPIRED") || user.ExpireAt.IsZero() || !user.ExpireAt.After(time.Now().UTC()) {
		return &UserState{
			Exists:            true,
			Active:            false,
			PanelUsername:     strings.TrimSpace(user.Username),
			UserID:            user.ID,
			UserUUID:          user.UUID,
			TrafficLimitBytes: user.TrafficLimitBytes,
			UsedTrafficBytes:  user.UserTraffic.UsedTrafficBytes,
			DeviceLimit:       deviceLimit,
			UsedDevices:       usedDevices,
			Devices:           devices,
		}, nil
	}

	var subscriptionLink *string
	if link := strings.TrimSpace(user.SubscriptionURL); link != "" {
		subscriptionLink = &link
	}
	panelUsername := strings.TrimSpace(user.Username)

	expireAt := user.ExpireAt.UTC()
	return &UserState{
		Exists:            true,
		Active:            true,
		ExpireAt:          &expireAt,
		SubscriptionLink:  subscriptionLink,
		PanelUsername:     panelUsername,
		UserID:            user.ID,
		UserUUID:          user.UUID,
		TrafficLimitBytes: user.TrafficLimitBytes,
		UsedTrafficBytes:  user.UserTraffic.UsedTrafficBytes,
		DeviceLimit:       deviceLimit,
		UsedDevices:       usedDevices,
		Devices:           devices,
	}, nil
}

func (r *Client) AddDeviceLimit(ctx context.Context, telegramID int64, extraDevices int) (*PanelUser, error) {
	if extraDevices <= 0 {
		return nil, errors.New("extra device count must be positive")
	}

	user, err := r.getPanelUserByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, err
	}
	return r.addDeviceLimitForUser(ctx, user, extraDevices)
}

func (r *Client) AddDeviceLimitByIdentity(ctx context.Context, userID int64, userUUID uuid.UUID, extraDevices int) (*PanelUser, error) {
	if extraDevices <= 0 {
		return nil, errors.New("extra device count must be positive")
	}
	user, err := r.getPanelUserByIdentity(ctx, userID, userUUID)
	if err != nil {
		return nil, err
	}
	return r.addDeviceLimitForUser(ctx, user, extraDevices)
}

func (r *Client) addDeviceLimitForUser(ctx context.Context, user *PanelUser, extraDevices int) (*PanelUser, error) {
	if user == nil {
		return nil, errors.New("subscription not found")
	}
	currentLimit := 0
	if user.HwidDeviceLimit != nil {
		currentLimit = *user.HwidDeviceLimit
	}
	if currentLimit <= 0 {
		return nil, errors.New("cannot add devices to an unlimited subscription")
	}
	if currentLimit > 10000-extraDevices {
		return nil, errors.New("resulting device limit is too large")
	}

	return r.patchPanelUser(ctx, user, map[string]any{
		"hwidDeviceLimit": currentLimit + extraDevices,
	})
}

func (r *Client) getUserHWIDDevices(ctx context.Context, userID int64, userUUID uuid.UUID) ([]UserDevice, error) {
	identifier := strconv.FormatInt(userID, 10)
	if userID <= 0 {
		identifier = userUUID.String()
	}
	var payload hwidDevicesResponse
	err := r.doAPIJSON(ctx, http.MethodGet, "/api/hwid/devices/"+identifier, nil, &payload)
	if err != nil && userID > 0 && userUUID != uuid.Nil && isLegacyFallbackError(err) {
		err = r.doAPIJSON(ctx, http.MethodGet, "/api/hwid/devices/"+userUUID.String(), nil, &payload)
	}
	if err != nil {
		var apiErr *remnawaveAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return []UserDevice{}, nil
		}
		return nil, err
	}
	return userDevicesFromResponse(payload, userID, userUUID), nil
}

func (r *Client) DeleteUserHWIDDevice(ctx context.Context, userID int64, userUUID uuid.UUID, hwid string) ([]UserDevice, error) {
	request := map[string]any{"userId": userID, "hwid": strings.TrimSpace(hwid)}
	var payload hwidDevicesResponse
	err := r.doAPIJSON(ctx, http.MethodPost, "/api/hwid/devices/delete", request, &payload)
	if err != nil && userUUID != uuid.Nil && isLegacyFallbackError(err) {
		err = r.doAPIJSON(ctx, http.MethodPost, "/api/hwid/devices/delete", map[string]any{
			"userUuid": userUUID.String(),
			"hwid":     strings.TrimSpace(hwid),
		}, &payload)
	}
	if err != nil {
		return nil, err
	}
	return userDevicesFromResponse(payload, userID, userUUID), nil
}

func (r *Client) DeleteUserHWIDDeviceByTelegramID(ctx context.Context, telegramId int64, hwid string) error {
	user, err := r.getPanelUserByTelegramID(ctx, telegramId)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("panel user not found")
	}

	_, err = r.DeleteUserHWIDDevice(ctx, user.ID, user.UUID, hwid)
	return err
}

type hwidDevicesResponse struct {
	Response struct {
		Total   int `json:"total"`
		Devices []struct {
			Hwid        string    `json:"hwid"`
			UserID      int64     `json:"userId"`
			UserUUID    uuid.UUID `json:"userUuid"`
			Platform    *string   `json:"platform"`
			OSVersion   *string   `json:"osVersion"`
			DeviceModel *string   `json:"deviceModel"`
			UserAgent   *string   `json:"userAgent"`
			CreatedAt   time.Time `json:"createdAt"`
			UpdatedAt   time.Time `json:"updatedAt"`
		} `json:"devices"`
	} `json:"response"`
}

func userDevicesFromResponse(payload hwidDevicesResponse, fallbackUserID int64, fallbackUUID uuid.UUID) []UserDevice {
	devices := make([]UserDevice, 0, len(payload.Response.Devices))
	for _, item := range payload.Response.Devices {
		userID := item.UserID
		if userID <= 0 {
			userID = fallbackUserID
		}
		userUUID := item.UserUUID
		if userUUID == uuid.Nil {
			userUUID = fallbackUUID
		}
		devices = append(devices, UserDevice{
			Hwid:        strings.TrimSpace(item.Hwid),
			UserID:      userID,
			UserUUID:    userUUID,
			Platform:    trimNilString(item.Platform),
			OSVersion:   trimNilString(item.OSVersion),
			DeviceModel: trimNilString(item.DeviceModel),
			UserAgent:   trimNilString(item.UserAgent),
			CreatedAt:   item.CreatedAt.UTC(),
			UpdatedAt:   item.UpdatedAt.UTC(),
		})
	}
	return devices
}

func (r *Client) GetNodesStatus(ctx context.Context) ([]NodeStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/api/nodes", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("nodes request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Response []struct {
			Name        string `json:"name"`
			Address     string `json:"address"`
			CountryCode string `json:"countryCode"`
			IsConnected bool   `json:"isConnected"`
			IsDisabled  bool   `json:"isDisabled"`
		} `json:"response"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	result := make([]NodeStatus, 0, len(payload.Response))
	for _, node := range payload.Response {
		name := strings.TrimSpace(node.Name)
		if name == "" {
			name = strings.TrimSpace(node.Address)
		}

		result = append(result, NodeStatus{
			Name:        name,
			Address:     strings.TrimSpace(node.Address),
			CountryCode: strings.TrimSpace(node.CountryCode),
			IsOnline:    node.IsConnected && !node.IsDisabled,
		})
	}

	return result, nil
}

func (r *Client) DecreaseSubscription(ctx context.Context, telegramId int64, trafficLimit int, deviceLimit int, days int) (*time.Time, error) {
	existingUser, err := r.getPanelUserByTelegramID(ctx, telegramId)
	if err != nil {
		return nil, err
	}

	updated, err := r.updateUser(ctx, existingUser, trafficLimit, deviceLimit, days)
	if err != nil {
		return nil, err
	}

	return &updated.ExpireAt, nil
}

func (r *Client) CreateOrUpdateUser(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, deviceLimit int, days int, isTrialUser bool) (*PanelUser, error) {
	return r.CreateOrUpdateUserWithOptions(ctx, customerId, telegramId, trafficLimit, deviceLimit, days, legacyProvisioningOptions(isTrialUser))
}

func (r *Client) CreateOrUpdateUserWithOptions(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, deviceLimit int, days int, options ProvisioningOptions) (*PanelUser, error) {
	existingUser, err := r.getPanelUserByTelegramID(ctx, telegramId)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return r.createUserWithOptions(ctx, customerId, telegramId, trafficLimit, deviceLimit, days, options)
		}
		return nil, err
	}

	return r.updateUserWithOptions(ctx, existingUser, trafficLimit, deviceLimit, days, options)
}

func (r *Client) CreateOrUpdateUserForSubscription(ctx context.Context, customerID, telegramID, subscriptionID, userID int64, userUUID uuid.UUID, isPrimary bool, trafficLimit, deviceLimit, days int, options ProvisioningOptions) (*PanelUser, error) {
	if userID > 0 || userUUID != uuid.Nil {
		existing, err := r.getPanelUserByIdentity(ctx, userID, userUUID)
		if err != nil {
			return nil, err
		}
		return r.updateUserWithOptions(ctx, existing, trafficLimit, deviceLimit, days, options)
	}
	if isPrimary {
		return r.CreateOrUpdateUserWithOptions(ctx, customerID, telegramID, trafficLimit, deviceLimit, days, options)
	}
	options.UsernameSuffix = fmt.Sprintf("s%d", subscriptionID)
	return r.createUserWithOptions(ctx, customerID, telegramID, trafficLimit, deviceLimit, days, options)
}

func (r *Client) DeleteUser(ctx context.Context, userID int64, userUUID uuid.UUID) error {
	user, err := r.getPanelUserByIdentity(ctx, userID, userUUID)
	if err != nil {
		if errors.Is(err, ErrAdminSubscriptionNotFound) {
			return nil
		}
		return err
	}
	return r.deletePanelUser(ctx, user)
}

func legacyProvisioningOptions(isTrialUser bool) ProvisioningOptions {
	selected := config.SquadUUIDs()
	external := config.ExternalSquadUUID()
	strategy := config.TrafficLimitResetStrategy()
	tag := config.RemnawaveTag()
	if isTrialUser {
		selected = config.TrialInternalSquads()
		external = config.TrialExternalSquadUUID()
		strategy = config.TrialTrafficLimitResetStrategy()
		tag = config.TrialRemnawaveTag()
	}
	internal := make([]string, 0, len(selected))
	for value := range selected {
		if value != uuid.Nil {
			internal = append(internal, value.String())
		}
	}
	externalValue := ""
	if external != uuid.Nil {
		externalValue = external.String()
	}
	return ProvisioningOptions{InternalSquadUUIDs: internal, ExternalSquadUUID: externalValue, TrafficResetStrategy: strategy, Tag: tag, ApplySquads: true}
}

func (r *Client) getPanelUserByTelegramID(ctx context.Context, telegramId int64) (*PanelUser, error) {
	filters := make(url.Values)
	filters.Set("telegramId", strconv.FormatInt(telegramId, 10))
	users, err := r.streamUsers(ctx, filters)
	if err == nil {
		if existingUser := pickPanelTelegramUser(users, telegramId); existingUser != nil {
			return existingUser, nil
		}
		return nil, fmt.Errorf("user with telegramId %d not found", telegramId)
	}
	if !isLegacyFallbackError(err) {
		return nil, err
	}

	resp, legacyErr := r.client.Users().GetUserByTelegramId(ctx, strconv.FormatInt(telegramId, 10))
	if legacyErr != nil {
		return nil, errors.Join(err, legacyErr)
	}
	usersResp, ok := resp.(*remapi.UsersResponse)
	if !ok {
		return nil, errors.New("unknown legacy user response type")
	}
	legacyUsers := usersResp.GetResponse()
	converted := make([]PanelUser, 0, len(legacyUsers))
	for i := range legacyUsers {
		converted = append(converted, panelUserFromLegacy(&legacyUsers[i]))
	}
	if existingUser := pickPanelTelegramUser(converted, telegramId); existingUser != nil {
		return existingUser, nil
	}
	return nil, fmt.Errorf("user with telegramId %d not found", telegramId)
}

func (r *Client) GrantGraceAccess(ctx context.Context, telegramID int64, days int, internalSquadUUIDs []string) (GraceAccessResult, error) {
	if days <= 0 {
		return GraceAccessResult{}, errors.New("grace access duration must be positive")
	}
	if len(internalSquadUUIDs) == 0 {
		return GraceAccessResult{}, errors.New("grace access requires at least one internal squad")
	}

	user, err := r.getPanelUserByTelegramID(ctx, telegramID)
	if err != nil {
		return GraceAccessResult{}, err
	}
	squadIDs, err := r.resolveInternalSquads(ctx, internalSquadUUIDs)
	if err != nil {
		return GraceAccessResult{}, err
	}
	if len(squadIDs) == 0 {
		return GraceAccessResult{}, errors.New("selected grace access squads were not found")
	}

	expireAt := time.Now().UTC().AddDate(0, 0, days)
	updated, err := r.patchPanelUser(ctx, user, map[string]any{
		"expireAt":             expireAt,
		"status":               "ACTIVE",
		"activeInternalSquads": squadIDs,
	})
	if err != nil {
		return GraceAccessResult{}, err
	}

	return GraceAccessResult{
		ExpireAt:         updated.ExpireAt,
		SubscriptionLink: strings.TrimSpace(updated.SubscriptionURL),
	}, nil
}

func pickPanelTelegramUser(users []PanelUser, telegramId int64) *PanelUser {
	suffix := fmt.Sprintf("_%d", telegramId)

	for i := range users {
		if users[i].TelegramID != nil && *users[i].TelegramID == telegramId && strings.HasSuffix(users[i].Username, suffix) {
			return &users[i]
		}
	}
	for i := range users {
		if users[i].TelegramID != nil && *users[i].TelegramID == telegramId && !secondaryPanelUsernamePattern.MatchString(strings.TrimSpace(users[i].Username)) {
			return &users[i]
		}
	}
	for i := range users {
		if users[i].TelegramID != nil && *users[i].TelegramID == telegramId {
			return &users[i]
		}
	}

	if len(users) == 0 {
		return nil
	}

	return &users[0]
}

func (r *Client) updateUser(ctx context.Context, existingUser *PanelUser, trafficLimit int, deviceLimit int, days int) (*PanelUser, error) {
	return r.updateUserWithOptions(ctx, existingUser, trafficLimit, deviceLimit, days, legacyProvisioningOptions(false))
}

func (r *Client) updateUserWithOptions(ctx context.Context, existingUser *PanelUser, trafficLimit int, deviceLimit int, days int, options ProvisioningOptions) (*PanelUser, error) {

	newExpire := getNewExpire(days, existingUser.ExpireAt)
	squadIDs := make([]uuid.UUID, 0, len(existingUser.ActiveInternalSquads))
	for _, squad := range existingUser.ActiveInternalSquads {
		squadIDs = append(squadIDs, squad.UUID)
	}
	if options.ApplySquads {
		var err error
		squadIDs, err = r.resolveInternalSquads(ctx, options.InternalSquadUUIDs)
		if err != nil {
			return nil, err
		}
	}
	strategy := strings.TrimSpace(options.TrafficResetStrategy)
	if strategy == "" {
		strategy = config.TrafficLimitResetStrategy()
	}

	fields := map[string]any{
		"expireAt":             newExpire,
		"status":               "ACTIVE",
		"trafficLimitBytes":    trafficLimit,
		"hwidDeviceLimit":      deviceLimit,
		"activeInternalSquads": squadIDs,
		"trafficLimitStrategy": normalizeTrafficStrategy(strategy),
	}

	if options.ApplySquads {
		externalSquad, err := optionalUUID(options.ExternalSquadUUID)
		if err != nil {
			return nil, err
		}
		if externalSquad == uuid.Nil {
			fields["externalSquadUuid"] = nil
		} else {
			fields["externalSquadUuid"] = externalSquad
		}
	}

	tag := strings.TrimSpace(options.Tag)
	if tag != "" {
		fields["tag"] = tag
	}

	description, username, hasProfileInfo := telegramDescriptionFromContext(ctx)
	if hasProfileInfo {
		fields["description"] = description
	}

	updated, err := r.patchPanelUser(ctx, existingUser, fields)
	if err != nil {
		return nil, err
	}

	tgid := int64(0)
	if existingUser.TelegramID != nil {
		tgid = *existingUser.TelegramID
	}
	slog.Info("updated user", "telegramId", utils.MaskHalfInt64(tgid), "username", utils.MaskHalf(username), "days", days)
	return updated, nil
}

func (r *Client) createUser(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, deviceLimit int, days int, isTrialUser bool) (*PanelUser, error) {
	return r.createUserWithOptions(ctx, customerId, telegramId, trafficLimit, deviceLimit, days, legacyProvisioningOptions(isTrialUser))
}

func (r *Client) createUserWithOptions(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, deviceLimit int, days int, options ProvisioningOptions) (*PanelUser, error) {
	expireAt := time.Now().UTC().AddDate(0, 0, days)
	username := generateUsername(options.UsernameTemplate, customerId, telegramId)
	username = appendUsernameSuffix(username, options.UsernameSuffix)

	squadIDs, err := r.resolveInternalSquads(ctx, options.InternalSquadUUIDs)
	if err != nil {
		return nil, err
	}
	externalSquad, err := optionalUUID(options.ExternalSquadUUID)
	if err != nil {
		return nil, err
	}
	strategy := strings.TrimSpace(options.TrafficResetStrategy)
	if strategy == "" {
		strategy = config.TrafficLimitResetStrategy()
	}

	fields := map[string]any{
		"username":             username,
		"activeInternalSquads": squadIDs,
		"status":               "ACTIVE",
		"telegramId":           telegramId,
		"expireAt":             expireAt,
		"trafficLimitStrategy": normalizeTrafficStrategy(strategy),
		"trafficLimitBytes":    trafficLimit,
		"hwidDeviceLimit":      deviceLimit,
	}
	if externalSquad != uuid.Nil {
		fields["externalSquadUuid"] = externalSquad
	}
	tag := strings.TrimSpace(options.Tag)
	if tag != "" {
		fields["tag"] = tag
	}

	description, tgUsername, hasProfileInfo := telegramDescriptionFromContext(ctx)
	if hasProfileInfo {
		fields["description"] = description
	}

	created, err := r.createPanelUser(ctx, fields)
	if err != nil {
		return nil, err
	}
	slog.Info("created user", "telegramId", utils.MaskHalf(strconv.FormatInt(telegramId, 10)), "username", utils.MaskHalf(tgUsername), "days", days)
	return created, nil
}

func (r *Client) resolveInternalSquads(ctx context.Context, selected []string) ([]uuid.UUID, error) {
	response, err := r.client.InternalSquad().GetInternalSquads(ctx)
	if err != nil {
		return nil, err
	}
	payload, ok := response.(*remapi.InternalSquadsResponse)
	if !ok {
		return nil, errors.New("unknown internal squads response type")
	}
	wanted := map[uuid.UUID]struct{}{}
	for _, raw := range selected {
		value, err := optionalUUID(raw)
		if err != nil {
			return nil, err
		}
		if value != uuid.Nil {
			wanted[value] = struct{}{}
		}
	}
	result := make([]uuid.UUID, 0)
	responsePayload := payload.GetResponse()
	for _, squad := range responsePayload.GetInternalSquads() {
		if len(wanted) > 0 {
			if _, exists := wanted[squad.UUID]; !exists {
				continue
			}
		}
		result = append(result, squad.UUID)
	}
	return result, nil
}

func optionalUUID(raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, nil
	}
	value, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid squad UUID: %w", err)
	}
	return value, nil
}

func (r *Client) ListSquads(ctx context.Context) (SquadCatalog, error) {
	internalResponse, err := r.client.InternalSquad().GetInternalSquads(ctx)
	if err != nil {
		return SquadCatalog{}, err
	}
	internalPayload, ok := internalResponse.(*remapi.InternalSquadsResponse)
	if !ok {
		return SquadCatalog{}, errors.New("unknown internal squads response type")
	}
	externalResponse, err := r.client.ExternalSquad().GetExternalSquads(ctx)
	if err != nil {
		return SquadCatalog{}, err
	}
	externalPayload, ok := externalResponse.(*remapi.ExternalSquadsResponse)
	if !ok {
		return SquadCatalog{}, errors.New("unknown external squads response type")
	}
	result := SquadCatalog{Internal: []SquadOption{}, External: []SquadOption{}}
	internalItems := internalPayload.GetResponse()
	for _, item := range internalItems.GetInternalSquads() {
		result.Internal = append(result.Internal, SquadOption{UUID: item.UUID.String(), Name: strings.TrimSpace(item.Name)})
	}
	externalItems := externalPayload.GetResponse()
	for _, item := range externalItems.GetExternalSquads() {
		result.External = append(result.External, SquadOption{UUID: item.UUID.String(), Name: strings.TrimSpace(item.Name)})
	}
	return result, nil
}

func generateUsername(template string, customerId int64, telegramId int64) string {
	template = strings.TrimSpace(template)
	if template == "" {
		template = "{{customer_id}}_{{telegram_id}}"
	}
	username := strings.NewReplacer(
		"{{customer_id}}", strconv.FormatInt(customerId, 10),
		"{{telegram_id}}", strconv.FormatInt(telegramId, 10),
	).Replace(template)
	username = panelUsernameSanitizer.ReplaceAllString(username, "_")
	username = strings.Trim(username, "_.-")
	if username == "" {
		return fmt.Sprintf("%d_%d", customerId, telegramId)
	}
	if len(username) > 64 {
		username = username[:64]
	}
	return username
}

func appendUsernameSuffix(username, suffix string) string {
	suffix = panelUsernameSanitizer.ReplaceAllString(strings.TrimSpace(suffix), "_")
	suffix = strings.Trim(suffix, "_.-")
	if suffix == "" {
		return username
	}
	suffix = "_" + suffix
	maxBase := 64 - len(suffix)
	if maxBase < 1 {
		return suffix[len(suffix)-64:]
	}
	if len(username) > maxBase {
		username = strings.TrimRight(username[:maxBase], "_.-")
	}
	return username + suffix
}

func telegramDescriptionFromContext(ctx context.Context) (description string, usernameForLog string, hasProfileInfo bool) {
	displayName, hasDisplayName := contextStringValue(ctx, telegramNameContextKey)
	username, hasUsername := contextStringValue(ctx, telegramUsernameContextKey)
	if !hasDisplayName && !hasUsername {
		return "", "", false
	}

	return FormatTelegramDescription(displayName, username), strings.TrimSpace(strings.TrimPrefix(username, "@")), true
}

func FormatTelegramDescription(displayName, username string) string {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = "-"
	}

	username = strings.TrimSpace(strings.TrimPrefix(username, "@"))
	usernamePart := "-"
	if username != "" {
		usernamePart = "@" + username
	}

	return displayName + " | " + usernamePart
}

func contextStringValue(ctx context.Context, key string) (string, bool) {
	if ctx == nil {
		return "", false
	}

	value := ctx.Value(key)
	if value == nil {
		return "", false
	}

	switch v := value.(type) {
	case string:
		return v, true
	case fmt.Stringer:
		return v.String(), true
	default:
		return "", true
	}
}

func getNewExpire(daysToAdd int, currentExpire time.Time) time.Time {
	if daysToAdd <= 0 {
		if currentExpire.AddDate(0, 0, daysToAdd).Before(time.Now()) {
			return time.Now().UTC().AddDate(0, 0, 1)
		} else {
			return currentExpire.AddDate(0, 0, daysToAdd)
		}
	}

	if currentExpire.Before(time.Now().UTC()) || currentExpire.IsZero() {
		return time.Now().UTC().AddDate(0, 0, daysToAdd)
	}

	return currentExpire.AddDate(0, 0, daysToAdd)
}

func normalizeTrafficStrategy(strategy string) string {
	switch strings.ToUpper(strings.TrimSpace(strategy)) {
	case "DAY":
		return "DAY"
	case "WEEK":
		return "WEEK"
	case "NO_RESET":
		return "NO_RESET"
	default:
		return "MONTH"
	}
}

func trimNilString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
