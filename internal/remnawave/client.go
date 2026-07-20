package remnawave

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"link-bot/internal/config"
	"link-bot/utils"
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

type ProvisioningOptions struct {
	InternalSquadUUIDs   []string
	ExternalSquadUUID    string
	TrafficResetStrategy string
	Tag                  string
	ApplySquads          bool
}

type UserState struct {
	Exists            bool
	Active            bool
	ExpireAt          *time.Time
	SubscriptionLink  *string
	PanelUsername     string
	UserUUID          uuid.UUID
	TrafficLimitBytes int64
	UsedTrafficBytes  int64
	DeviceLimit       int
	UsedDevices       int
	Devices           []UserDevice
}

var ErrAdminSubscriptionNotFound = errors.New("subscription not found")

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
	_, err := r.client.Users().GetAllUsers(ctx, 1, 0)
	return err
}

func (r *Client) GetUsers(ctx context.Context) (*[]remapi.UserItemInfo, error) {
	pager := remapi.NewPaginationHelper(250)
	users := make([]remapi.UserItemInfo, 0)

	for {
		resp, err := r.client.Users().GetAllUsers(ctx, pager.Limit, pager.Offset)
		if err != nil {
			return nil, err
		}

		response := resp.(*remapi.GetAllUsersResponse).GetResponse()
		users = append(users, response.Users...)

		if len(response.Users) < pager.Limit {
			break
		}

		if !pager.NextPage() {
			break
		}
	}

	return &users, nil
}

func (r *Client) FindUserByIDOrUsername(ctx context.Context, query string) (*AdminSubscription, error) {
	users, err := r.GetUsers(ctx)
	if err != nil {
		return nil, err
	}

	user := findUserByIDOrUsername(*users, query)
	if user == nil {
		return nil, ErrAdminSubscriptionNotFound
	}

	return adminSubscriptionFromUser(user), nil
}

func (r *Client) RebindUserTelegramID(ctx context.Context, userUUID uuid.UUID, targetTelegramID int64, targetDescription string) (*AdminRebindResult, error) {
	if userUUID == uuid.Nil || targetTelegramID <= 0 {
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

	current := findUserByUUID(*users, userUUID)
	if current == nil {
		return nil, ErrAdminSubscriptionNotFound
	}

	currentSubscription := adminSubscriptionFromUser(current)
	previousTelegramID := currentSubscription.TelegramID
	previousDescription := currentSubscription.Description
	if previousTelegramID != nil && *previousTelegramID == targetTelegramID {
		updated, err := r.updateAdminUserTelegramProfile(ctx, current, previousTelegramID, &targetDescription)
		if err != nil {
			return nil, err
		}
		return &AdminRebindResult{
			Subscription:        updated,
			PreviousTelegramID:  previousTelegramID,
			PreviousDescription: previousDescription,
		}, nil
	}

	displacedUser := findOtherUserByTelegramID(*users, targetTelegramID, userUUID)
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

func (r *Client) RestoreAdminRebind(ctx context.Context, userUUID uuid.UUID, telegramID *int64, description *string, displacedSubscription *AdminSubscription) error {
	users, err := r.GetUsers(ctx)
	if err != nil {
		return err
	}

	current := findUserByUUID(*users, userUUID)
	if current == nil {
		return ErrAdminSubscriptionNotFound
	}

	if _, err = r.updateAdminUserTelegramProfile(ctx, current, telegramID, description); err != nil {
		return fmt.Errorf("restore transferred subscription: %w", err)
	}

	if displacedSubscription == nil {
		return nil
	}
	displacedCurrent := findUserByUUID(*users, displacedSubscription.UUID)
	if displacedCurrent == nil {
		return fmt.Errorf("restore displaced subscription: %w", ErrAdminSubscriptionNotFound)
	}
	if _, err = r.updateAdminUserTelegramProfile(ctx, displacedCurrent, displacedSubscription.TelegramID, displacedSubscription.Description); err != nil {
		return fmt.Errorf("restore displaced subscription: %w", err)
	}
	return nil
}

func (r *Client) updateAdminUserTelegramProfile(ctx context.Context, current *remapi.UserItemInfo, telegramID *int64, description *string) (*AdminSubscription, error) {
	if current == nil {
		return nil, ErrAdminSubscriptionNotFound
	}

	squadIDs := make([]uuid.UUID, 0, len(current.ActiveInternalSquads))
	for _, squad := range current.ActiveInternalSquads {
		squadIDs = append(squadIDs, squad.UUID)
	}

	telegramValue := remapi.OptNilInt{}
	if telegramID == nil {
		telegramValue.SetToNull()
	} else {
		telegramValue.SetTo(int(*telegramID))
	}
	descriptionValue := remapi.OptNilString{}
	if description == nil {
		descriptionValue.SetToNull()
	} else {
		descriptionValue.SetTo(strings.TrimSpace(*description))
	}

	response, err := r.client.Users().UpdateUser(ctx, &remapi.UpdateUserRequest{
		UUID:                 remapi.NewOptUUID(current.UUID),
		TelegramId:           telegramValue,
		Description:          descriptionValue,
		ActiveInternalSquads: squadIDs,
	})
	if err != nil {
		return nil, err
	}
	if value, ok := response.(*remapi.InternalServerError); ok {
		return nil, errors.New("error while rebinding user. message: " + value.GetMessage().Value + ". code: " + value.GetErrorCode().Value)
	}

	updated, ok := response.(*remapi.UserResponse)
	if !ok {
		return nil, errors.New("unknown response type while rebinding user")
	}
	return adminSubscriptionFromUser(&updated.Response), nil
}

func findUserByIDOrUsername(users []remapi.UserItemInfo, query string) *remapi.UserItemInfo {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	requestedID, idErr := strconv.ParseInt(query, 10, 64)
	for i := range users {
		if idErr == nil && int64(users[i].ID) == requestedID {
			return &users[i]
		}
		if strings.EqualFold(strings.TrimSpace(users[i].Username), query) {
			return &users[i]
		}
	}

	return nil
}

func findUserByUUID(users []remapi.UserItemInfo, userUUID uuid.UUID) *remapi.UserItemInfo {
	for i := range users {
		if users[i].UUID == userUUID {
			return &users[i]
		}
	}
	return nil
}

func findOtherUserByTelegramID(users []remapi.UserItemInfo, telegramID int64, excludedUUID uuid.UUID) *remapi.UserItemInfo {
	if telegramID <= 0 {
		return nil
	}
	for i := range users {
		linkedTelegramID, ok := users[i].TelegramId.Get()
		if ok && int64(linkedTelegramID) == telegramID && users[i].UUID != excludedUUID {
			return &users[i]
		}
	}
	return nil
}

func adminSubscriptionFromUser(user *remapi.UserItemInfo) *AdminSubscription {
	if user == nil {
		return nil
	}

	var telegramID *int64
	if value, ok := user.TelegramId.Get(); ok && value > 0 {
		converted := int64(value)
		telegramID = &converted
	}
	var description *string
	if value, ok := user.Description.Get(); ok {
		copied := value
		description = &copied
	}

	status := ""
	if value, ok := user.Status.Get(); ok {
		status = string(value)
	}

	return &AdminSubscription{
		ID:               int64(user.ID),
		UUID:             user.UUID,
		Username:         strings.TrimSpace(user.Username),
		Status:           status,
		ExpireAt:         user.ExpireAt.UTC(),
		TelegramID:       telegramID,
		Description:      description,
		SubscriptionLink: strings.TrimSpace(user.SubscriptionUrl),
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

	if user == nil {
		return nil, nil
	}

	stats := struct {
		TrafficLimitBytes int64 `json:"trafficLimitBytes"`
		HwidDeviceLimit   *int  `json:"hwidDeviceLimit"`
		UserTraffic       struct {
			UsedTrafficBytes int64 `json:"usedTrafficBytes"`
		} `json:"userTraffic"`
	}{}

	if data, marshalErr := json.Marshal(user); marshalErr == nil {
		if unmarshalErr := json.Unmarshal(data, &stats); unmarshalErr != nil {
			slog.Warn("remnawave: decode user stats failed", "error", unmarshalErr, "telegramId", utils.MaskHalfInt64(telegramId))
		}
	} else {
		slog.Warn("remnawave: marshal user stats failed", "error", marshalErr, "telegramId", utils.MaskHalfInt64(telegramId))
	}

	deviceLimit := -1
	if stats.HwidDeviceLimit != nil {
		deviceLimit = *stats.HwidDeviceLimit
	}
	devices, deviceErr := r.getUserHWIDDevices(ctx, user.UUID)
	if deviceErr != nil {
		slog.Warn("remnawave: load user devices failed", "error", deviceErr, "telegramId", utils.MaskHalfInt64(telegramId))
	}
	usedDevices := len(devices)

	statusValue, hasStatus := user.Status.Get()
	status := strings.ToUpper(strings.TrimSpace(string(statusValue)))
	if !hasStatus {
		status = "ACTIVE"
	}

	if (status == "DISABLED" || status == "EXPIRED") || user.ExpireAt.IsZero() || !user.ExpireAt.After(time.Now().UTC()) {
		return &UserState{
			Exists:            true,
			Active:            false,
			PanelUsername:     strings.TrimSpace(user.Username),
			UserUUID:          user.UUID,
			TrafficLimitBytes: stats.TrafficLimitBytes,
			UsedTrafficBytes:  stats.UserTraffic.UsedTrafficBytes,
			DeviceLimit:       deviceLimit,
			UsedDevices:       usedDevices,
			Devices:           devices,
		}, nil
	}

	var subscriptionLink *string
	if link := strings.TrimSpace(user.SubscriptionUrl); link != "" {
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
		UserUUID:          user.UUID,
		TrafficLimitBytes: stats.TrafficLimitBytes,
		UsedTrafficBytes:  stats.UserTraffic.UsedTrafficBytes,
		DeviceLimit:       deviceLimit,
		UsedDevices:       usedDevices,
		Devices:           devices,
	}, nil
}

func (r *Client) getUserHWIDDevices(ctx context.Context, userUUID uuid.UUID) ([]UserDevice, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/api/hwid/devices/"+userUUID.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []UserDevice{}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("hwid request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Response struct {
			Total   int `json:"total"`
			Devices []struct {
				Hwid        string    `json:"hwid"`
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

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	if payload.Response.Total < 0 || len(payload.Response.Devices) == 0 {
		return []UserDevice{}, nil
	}

	devices := make([]UserDevice, 0, len(payload.Response.Devices))
	for _, item := range payload.Response.Devices {
		devices = append(devices, UserDevice{
			Hwid:        strings.TrimSpace(item.Hwid),
			UserUUID:    item.UserUUID,
			Platform:    trimNilString(item.Platform),
			OSVersion:   trimNilString(item.OSVersion),
			DeviceModel: trimNilString(item.DeviceModel),
			UserAgent:   trimNilString(item.UserAgent),
			CreatedAt:   item.CreatedAt.UTC(),
			UpdatedAt:   item.UpdatedAt.UTC(),
		})
	}

	return devices, nil
}

func (r *Client) DeleteUserHWIDDevice(ctx context.Context, userUUID uuid.UUID, hwid string) ([]UserDevice, error) {
	payload := map[string]string{
		"userUuid": userUUID.String(),
		"hwid":     strings.TrimSpace(hwid),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/api/hwid/devices/delete", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("delete hwid device failed: %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var payloadResp struct {
		Response struct {
			Devices []struct {
				Hwid        string    `json:"hwid"`
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

	if err := json.NewDecoder(resp.Body).Decode(&payloadResp); err != nil {
		return nil, err
	}

	devices := make([]UserDevice, 0, len(payloadResp.Response.Devices))
	for _, item := range payloadResp.Response.Devices {
		devices = append(devices, UserDevice{
			Hwid:        strings.TrimSpace(item.Hwid),
			UserUUID:    item.UserUUID,
			Platform:    trimNilString(item.Platform),
			OSVersion:   trimNilString(item.OSVersion),
			DeviceModel: trimNilString(item.DeviceModel),
			UserAgent:   trimNilString(item.UserAgent),
			CreatedAt:   item.CreatedAt.UTC(),
			UpdatedAt:   item.UpdatedAt.UTC(),
		})
	}

	return devices, nil
}

func (r *Client) DeleteUserHWIDDeviceByTelegramID(ctx context.Context, telegramId int64, hwid string) error {
	user, err := r.getPanelUserByTelegramID(ctx, telegramId)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("panel user not found")
	}

	_, err = r.DeleteUserHWIDDevice(ctx, user.UUID, hwid)
	return err
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

func (r *Client) CreateOrUpdateUser(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, deviceLimit int, days int, isTrialUser bool) (*remapi.UserItemInfo, error) {
	return r.CreateOrUpdateUserWithOptions(ctx, customerId, telegramId, trafficLimit, deviceLimit, days, legacyProvisioningOptions(isTrialUser))
}

func (r *Client) CreateOrUpdateUserWithOptions(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, deviceLimit int, days int, options ProvisioningOptions) (*remapi.UserItemInfo, error) {
	existingUser, err := r.getPanelUserByTelegramID(ctx, telegramId)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return r.createUserWithOptions(ctx, customerId, telegramId, trafficLimit, deviceLimit, days, options)
		}
		return nil, err
	}

	return r.updateUserWithOptions(ctx, existingUser, trafficLimit, deviceLimit, days, options)
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

func (r *Client) getPanelUserByTelegramID(ctx context.Context, telegramId int64) (*remapi.UserItemInfo, error) {
	resp, err := r.client.Users().GetUserByTelegramId(ctx, strconv.FormatInt(telegramId, 10))
	if err != nil {
		return nil, err
	}

	usersResp, ok := resp.(*remapi.UsersResponse)
	if !ok {
		return nil, errors.New("unknown response type")
	}

	users := usersResp.GetResponse()
	if len(users) == 0 {
		return nil, fmt.Errorf("user with telegramId %d not found", telegramId)
	}

	existingUser := pickTelegramUser(users, telegramId)
	if existingUser == nil {
		return nil, fmt.Errorf("user with telegramId %d not found", telegramId)
	}

	return existingUser, nil
}

func pickTelegramUser(users []remapi.UserItemInfo, telegramId int64) *remapi.UserItemInfo {
	suffix := fmt.Sprintf("_%d", telegramId)

	for i := range users {
		if strings.Contains(users[i].Username, suffix) {
			return &users[i]
		}
	}

	if len(users) == 0 {
		return nil
	}

	return &users[0]
}

func (r *Client) updateUser(ctx context.Context, existingUser *remapi.UserItemInfo, trafficLimit int, deviceLimit int, days int) (*remapi.UserItemInfo, error) {
	return r.updateUserWithOptions(ctx, existingUser, trafficLimit, deviceLimit, days, legacyProvisioningOptions(false))
}

func (r *Client) updateUserWithOptions(ctx context.Context, existingUser *remapi.UserItemInfo, trafficLimit int, deviceLimit int, days int, options ProvisioningOptions) (*remapi.UserItemInfo, error) {

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

	userUpdate := &remapi.UpdateUserRequest{
		UUID:                 remapi.NewOptUUID(existingUser.UUID),
		ExpireAt:             remapi.NewOptDateTime(newExpire),
		Status:               remapi.NewOptUpdateUserRequestStatus(remapi.UpdateUserRequestStatusACTIVE),
		TrafficLimitBytes:    remapi.NewOptInt(trafficLimit),
		HwidDeviceLimit:      remapi.NewOptNilInt(deviceLimit),
		ActiveInternalSquads: squadIDs,
		TrafficLimitStrategy: remapi.NewOptUpdateUserRequestTrafficLimitStrategy(getUpdateStrategy(strategy)),
	}

	if options.ApplySquads {
		externalSquad, err := optionalUUID(options.ExternalSquadUUID)
		if err != nil {
			return nil, err
		}
		if externalSquad == uuid.Nil {
			userUpdate.ExternalSquadUuid.SetToNull()
		} else {
			userUpdate.ExternalSquadUuid = remapi.NewOptNilUUID(externalSquad)
		}
	}

	tag := strings.TrimSpace(options.Tag)
	if tag != "" {
		userUpdate.Tag = remapi.NewOptNilString(tag)
	}

	description, username, hasProfileInfo := telegramDescriptionFromContext(ctx)
	if hasProfileInfo {
		userUpdate.Description = remapi.NewOptNilString(description)
	}

	updateUser, err := r.client.Users().UpdateUser(ctx, userUpdate)
	if err != nil {
		return nil, err
	}
	if value, ok := updateUser.(*remapi.InternalServerError); ok {
		return nil, errors.New("error while updating user. message: " + value.GetMessage().Value + ". code: " + value.GetErrorCode().Value)
	}

	tgid, _ := existingUser.TelegramId.Get()
	slog.Info("updated user", "telegramId", utils.MaskHalf(strconv.Itoa(tgid)), "username", utils.MaskHalf(username), "days", days)
	resp2 := updateUser.(*remapi.UserResponse).Response
	return &resp2, nil
}

func (r *Client) createUser(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, deviceLimit int, days int, isTrialUser bool) (*remapi.UserItemInfo, error) {
	return r.createUserWithOptions(ctx, customerId, telegramId, trafficLimit, deviceLimit, days, legacyProvisioningOptions(isTrialUser))
}

func (r *Client) createUserWithOptions(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, deviceLimit int, days int, options ProvisioningOptions) (*remapi.UserItemInfo, error) {
	expireAt := time.Now().UTC().AddDate(0, 0, days)
	username := generateUsername(customerId, telegramId)

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

	createUserRequestDto := remapi.CreateUserRequest{
		Username:             username,
		ActiveInternalSquads: squadIDs,
		Status:               remapi.NewOptCreateUserRequestStatus(remapi.CreateUserRequestStatusACTIVE),
		TelegramId:           remapi.NewOptNilInt(int(telegramId)),
		ExpireAt:             expireAt,
		TrafficLimitStrategy: remapi.NewOptCreateUserRequestTrafficLimitStrategy(getCreateStrategy(strategy)),
		TrafficLimitBytes:    remapi.NewOptInt(trafficLimit),
		HwidDeviceLimit:      remapi.NewOptInt(deviceLimit),
	}
	if externalSquad != uuid.Nil {
		createUserRequestDto.ExternalSquadUuid = remapi.NewOptNilUUID(externalSquad)
	}
	tag := strings.TrimSpace(options.Tag)
	if tag != "" {
		createUserRequestDto.Tag = remapi.NewOptNilString(tag)
	}

	description, tgUsername, hasProfileInfo := telegramDescriptionFromContext(ctx)
	if hasProfileInfo {
		createUserRequestDto.Description = remapi.NewOptString(description)
	}

	userCreate, err := r.client.Users().CreateUser(ctx, &createUserRequestDto)
	if err != nil {
		return nil, err
	}
	slog.Info("created user", "telegramId", utils.MaskHalf(strconv.FormatInt(telegramId, 10)), "username", utils.MaskHalf(tgUsername), "days", days)
	resp2 := userCreate.(*remapi.UserResponse).Response
	return &resp2, nil
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

func generateUsername(customerId int64, telegramId int64) string {
	return fmt.Sprintf("%d_%d", customerId, telegramId)
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

func getCreateStrategy(s string) remapi.CreateUserRequestTrafficLimitStrategy {
	switch s {
	case "DAY":
		return remapi.CreateUserRequestTrafficLimitStrategyDAY
	case "WEEK":
		return remapi.CreateUserRequestTrafficLimitStrategyWEEK
	case "NO_RESET":
		return remapi.CreateUserRequestTrafficLimitStrategyNORESET
	default:
		return remapi.CreateUserRequestTrafficLimitStrategyMONTH
	}
}

func getUpdateStrategy(s string) remapi.UpdateUserRequestTrafficLimitStrategy {
	switch s {
	case "DAY":
		return remapi.UpdateUserRequestTrafficLimitStrategyDAY
	case "WEEK":
		return remapi.UpdateUserRequestTrafficLimitStrategyWEEK
	case "NO_RESET":
		return remapi.UpdateUserRequestTrafficLimitStrategyNORESET
	default:
		return remapi.UpdateUserRequestTrafficLimitStrategyMONTH
	}
}

func trimNilString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
